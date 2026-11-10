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
| R3 | `circular.go:353` | AGENTS §1 row 1; ADR-113; admission gate (context ownership) | `WriteWithTimeout(item T, timeout time.Duration) error` stays on the returned buffer; with a full Block buffer it returns `context.DeadlineExceeded` itself once `timeout` has passed, and nothing is written; it succeeds when a read frees space first; no context root is created | `TestWriteWithTimeoutAssertable`, `TestWriteWithTimeoutExpires`, `TestWriteWithTimeoutSucceedsWhenSpaceFrees` |
| R4 | `circular.go:383-415` | `background-work` spec, "Background work takes one of three shapes" | when `WriteWithContext` returns (success, cancel, deadline, close), no goroutine it started is running | `TestWriteWithContextLeavesNothing` |
| R5 | `circular.go:361`, `:373` | `background-work` "nil refused"; AGENTS §1 row 4 | `WriteWithContext(nil, item)` returns an `ErrorInvalid` error under every policy and writes nothing | `TestWriteWithContextRefusesNilContext` |
| R6 | `circular.go:93-96`, `:110-112`, `:296-300` | defects in ported surface are fixed or tracked (#9, 2026-10-03); the comment states it | a drop callback runs after the lock is released, so it may call any buffer method (DropOldest, DropNewest, Clear) | `TestDropCallbackMayUseBuffer` |
| R7 | `buffer_test.go:445-447`, `:467`, `:480-482`, `:508`, `:643-703` | no `time.Sleep` (verify); wait on the ordering event (#392); synctest proof (`background-work`) | no sleeps or elapsed bounds; blocked-writer and timeout tests run in `synctest.Test` and use `synctest.Wait` | `task test:unit`, `task test:repeat` |
| R8 | `doc.go:46-49`; `README.md:116-128`, `:470-471` | surface audit (c); `docs:check` | the examples compile as written (they assert the method); the README passes markdownlint | `task docs:check`; an example test `ExampleCircularBuffer_writeWithTimeout` |
| R9 | `circular.go:32-34`; `options.go:39` | AGENTS §1, "any degrade is declared" | doc comments say capacity below 1 becomes 1 and that a nil registry or empty prefix records no metrics | `TestCapacityBelowOneIsOne` |

Mechanism is the developer's, settled by failing-first tests under `-race`. Each R3 to R6 test is shown to fail on
the pin's code ported with only the import path changed. R6's failing-first evidence is a hang that ends at `go test
-timeout`. Examples suffice and no generated checks are used: the ring arithmetic is carried unchanged, and the
corrections are in waiting and lock release, which a sequence model does not exercise.

## D3. Optional findings, kept

All of these are under Bounded port scope; none is tied to a binding requirement.

- **O1** `Buffer[T]` has one implementation and hides the bounded writes (shape 3). It is kept, and owner question Q1
  asks whether to change that. `TestWriteWithTimeoutAssertable` asserts exactly SemSource's local interface on every
  policy's buffer. A rename then fails here, not silently in SemSource.
- **O2** `notEmpty` is signalled and never waited on. It is kept: dead unexported code with no behaviour.
- **O3** The `Statistics` mutators and `NewStatistics` have no outside reader. They are kept so the package ports whole.
  `*Statistics` is named by SemSource (`publisher.go:307`).
- **O4** `MemoryUsage` always reads 0. It is kept and declared in its doc comment (Q2).
- **O5** Stats and metrics both count every event (shape 1). This is kept: "stats always on" is the package's stated
  design, as in `internal/cache`.
- **O6** Under Block, `Write` has no deadline. It is kept: a caller that needs a bound uses `WriteWithTimeout`.
- **O7** A metric registration error is classed Transient. It is kept, matching `internal/cache/cache.go:109`.
- **O8** `BenchmarkBufferWithStatistics` runs two identical subtests. Kept.
- **O9** `service/message_logger.go:225` keeps its own ring. It has different semantics (sequence slots, newest
  first) and is not consolidated.

## D4. Issues, ADRs, guidance, order

- Issues: #566 is retired (task 3.1). #34 stays: it is the importer's port.
- ADRs: none at the pin governs the package (inventory § 6), so none is ported. ADR-113 is cited for R3 to R5.
- Guidance: none at the pin.
- Merge order: before #34's PR. #588 does not overlap.
- Adoption sweep: not applicable; the package is carried, not an established primitive.
