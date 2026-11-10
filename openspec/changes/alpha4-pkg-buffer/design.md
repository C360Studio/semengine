# Design: alpha4-pkg-buffer

Inventory: `inventory.md` (base `b10a32e`). Ledger row: `pkg/buffer`, `adapt` (task 3.1).

## Size and split

1,235 production and 1,224 test lines at the pin; one package with no lifecycle owner; one PR.

## D1. What is carried unchanged

The ring and its three policies, as at the pin:

- **DropOldest** (the default) evicts the oldest item and returns nil.
- **DropNewest** discards the new item and returns nil.
- **Block** makes `Write` wait until a read frees space or `Close`.

Also carried unchanged:

- `Read`, `ReadBatch`, `Peek`, `Size`, `Capacity`, `IsFull`, `IsEmpty`, `Clear` and `Stats`.
- `Close`: after it, `Write` returns an `ErrorInvalid` error wrapping `errs.ErrAlreadyStopped`, while `Read` and
  `ReadBatch` still drain.
- `Statistics` with its counters, rates and `Summary`.
- The error classes.
- `WriteWithTimeout` and `WriteWithContext` under the other two policies behave as `Write`.

A zero timeout or an already-ended context refuses under Block even when there is space, as at the pin (probe).

## D2. Required corrections

| # | Pin site | Basis | Observable behaviour | Proving test |
| --- | --- | --- | --- | --- |
| R1 | `buffer.go:90-107` | owner ruling; #91 | `WithStats`, `StatsFromContext`, `ContextKeyStats` do not exist | build |
| R2 | `metrics.go:26`, `:77-97` | build (no `RegisterCounter` on main); `metric-registry` spec; #69 | collectors `semengine_buffer_{writes,reads,peeks,overflows,drops}_total`, `_size`, `_utilization` with label `component=<prefix>` gathered from the injected registry; a second buffer under one prefix shares them; a registry that holds another collector type under a key refuses construction | `TestMetricsOnInjectedRegistry` |
| R3 | `circular.go:353` | AGENTS §1 row 1; ADR-113; admission gate (context ownership) | `WriteWithTimeout(item T, timeout time.Duration) error` stays on the returned buffer; with a full Block buffer it returns `context.DeadlineExceeded` itself once `timeout` has passed, and nothing is written; it succeeds when a read frees space first; no context root is created | `TestWriteWithTimeoutAssertable`, `TestWriteWithTimeoutExpires`, `TestWriteWithTimeoutSucceedsWhenSpaceFrees` (behaviour pins); no root: read of `circular.go` |
| R4 | `circular.go:383-415` | `background-work` spec, "Background work takes one of three shapes" | when `WriteWithContext` returns (success, cancel, deadline, close), no goroutine it started is running | `TestWriteWithContextLeavesNothing` (behaviour pin); the join: read of `circular.go` |
| R5 | `circular.go:361`, `:373` | `.agents/contracts/semengine-developer.md:229` (exported context-taking boundaries reject nil when they can return an error) | `WriteWithContext(nil, item)` returns an `ErrorInvalid` error under every policy and writes nothing | `TestWriteWithContextRefusesNilContext` (fails first) |
| R6 | `circular.go:93-96`, `:110-112`, `:296-300` | defects in ported surface are fixed or tracked (#9, 2026-10-03); the comment states it | a drop callback runs after the lock is released, so it may call any buffer method (DropOldest, DropNewest, Clear) | `TestDropCallbackMayUseBuffer` (fails first) |
| R7 | `buffer_test.go:438-449`, `:467`, `:471-482`, `:508`, `:643-703` | no `time.Sleep` (verify); wait on the ordering event (#392); synctest proof (`background-work`) | no sleeps or elapsed bounds; blocked-writer and timeout tests run in `synctest.Test` and use `synctest.Wait` | `task test:unit`, `task test:repeat` |
| R8 | `doc.go:46-49`; `README.md:116-128`, `:470-471` | surface audit (c); `docs:check` | the examples compile as written (they assert the method); the README passes markdownlint | `task docs:check`; an example test `ExampleNewCircularBuffer_writeWithTimeout` |
| R9 | `circular.go:32-34`; `options.go:39` | AGENTS §1, "any degrade is declared" | doc comments say capacity below 1 becomes 1 and that a nil registry or empty prefix records no metrics | `TestCapacityBelowOneIsOne` (behaviour pin) |
| R10 | `statistics.go:23`, `:68-73`, `:114-119`, `:215`, `:233` | surface audit (a) and (c): nothing sets it, so it always reads 0; precedent `docs/admission-ledger.yaml:851-853` (#9, #220) | `UpdateMemoryUsage`, `MemoryUsage` and `StatsSummary.MemoryUsage` (`memory_usage`) do not exist | build |

The developer chooses the mechanism. The pin copy does not compile on main (`metrics.go:77` calls `RegisterCounter`),
so the port lands R1 and R2 first, under the new module path, and the tests run on that copy:

- R5 and R6 are defects a test can see, so their tests fail first on that copy. R6's failing-first evidence is a hang
  that ends at `go test -timeout`.
- R3 and R4 keep the pin's observable behaviour. Their defects (the invented root, the goroutine not waited for)
  cannot be seen by a deterministic test. The R3, R4 and R9 tests are behaviour pins, shown able to fail by
  `task mutate:check`. The missing root and the join are proved by reading `circular.go`, cited in the review.

Examples suffice and no generated checks are used: the ring arithmetic is carried unchanged, and the
corrections are in waiting and lock release, which a sequence model does not exercise.

## D3. Optional findings, kept

All of these are under Bounded port scope; none is tied to a binding requirement.

- **O1** `Buffer[T]` has one implementation and hides the bounded writes (shape 3). It is kept, and owner question Q1
  asks whether to change that. `TestWriteWithTimeoutAssertable` asserts exactly SemSource's local interface on every
  policy's buffer, so a rename fails here, not silently in SemSource. It cannot protect a wrapper:
  SemSource's `notifyingBuffer` (`internal/entitypub/stop_settlement_test.go:14-17`, bbfb3c6) embeds `Buffer[T]`, so
  the assertion at `publisher.go:269` fails and that test runs plain `Write`.
- **O2** `notEmpty` is signalled and never waited on. It is kept: dead unexported code with no behaviour.
- **O3** The `Statistics` mutators and `NewStatistics` have no outside reader. They are kept so the package ports whole.
  `*Statistics` is named by SemSource (`publisher.go:307`).
- **O4** `WithDropCallback` has no production caller at the pin or in a consumer. It is kept so the package ports
  whole, and the README offers it (`README.md:80-90`, `:147-152`), which is its contract line. R6 is the chosen fix
  for its deadlock. No owner ruling removes it; the eviction callbacks the owner removed from `internal/cache`
  (#9 comment 5950725772) were that package's, not this one's.
- **O5** Stats and metrics both count every event (shape 1). This is kept: "stats always on" is the package's stated
  design, as in `internal/cache`.
- **O6** Under Block, `Write` has no deadline. It is kept: a caller that needs a bound uses `WriteWithTimeout`.
- **O7** A metric registration error is classed Transient. It is kept, matching `internal/cache/cache.go:109`.
- **O8** `BenchmarkBufferWithStatistics` runs two identical subtests. Kept.
- **O9** `service/message_logger.go:225` keeps its own ring. It has different semantics (sequence slots, newest
  first) and is not consolidated.

## D4. Issues, ADRs, guidance, order

- Issues: #566 is retired (task 3.1). #34 stays: it is the importer's port.
- ADRs: none at the pin governs the package (inventory § 6), so none is ported. ADR-113 is cited for R3 and R4.
- Guidance: none at the pin.
- Merge order: before #34's PR. #588 does not overlap.
- Adoption sweep: not applicable; the package is carried, not an established primitive.
