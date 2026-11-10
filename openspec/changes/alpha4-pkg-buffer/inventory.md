# Inventory: alpha4-pkg-buffer

base: b10a32e428f430dfe259f524776469e54fe35993 (claim branch on origin/main b7da56ff5c03973d84e3333fa38f88f317f936b0)

Pin: SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read from `git archive 8b99efe9` unpacked to a scratch
directory. Pin paths are marked "(pin)".

## Lines at the pin

| File | Lines |
| --- | --- |
| `pkg/buffer/buffer.go` | 116 |
| `pkg/buffer/circular.go` | 459 |
| `pkg/buffer/doc.go` | 216 |
| `pkg/buffer/metrics.go` | 135 |
| `pkg/buffer/options.go` | 69 |
| `pkg/buffer/statistics.go` | 240 |
| production total | 1,235 |
| `pkg/buffer/buffer_test.go` | 716 |
| `pkg/buffer/benchmark_test.go` | 508 |
| test total | 1,224 |
| `pkg/buffer/README.md` | 505 |

## Pin probe

- `go test -race -count=1 -p 1 ./pkg/buffer/`: ok (1.8 s).
- `go test -count=5 -shuffle=on -cpu 1 -p 1 ./pkg/buffer/`: ok (2.8 s).
- `go test -cover`: 57.5% of statements. Not on the 80% list: `pkg/buffer` is named "not gated at 80%" in
  `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:462`.
- Lifecycle suite: not applicable, no service or component.
- Lines the test-text rules reject: `pkg/buffer/buffer_test.go:467` — `time.Sleep(50 * time.Millisecond)`;
  `:508` — `time.Sleep(50 * time.Millisecond)`; `:663` — `time.Sleep(100 * time.Millisecond)`; `:699` —
  `time.Sleep(50 * time.Millisecond)`. No `Skip`, no build tag, no `t.Parallel`
  (`grep -n -E 'Skip|//go:build|\+build|t\.Parallel'` on both test files: empty).
- Behaviour probes (a throwaway test file in the scratch copy, removed afterwards):
  - A `WithDropCallback` callback that calls `buf.Size()` during a DropOldest `Write` deadlocks (no return in 2 s).
  - `WriteWithTimeout(1, 0)` on an empty Block buffer returns `context deadline exceeded`, size 0.
  - `NewCircularBuffer[int](0)` returns no error, `Capacity() == 1`.
  - After `Close`, `Read` returns the queued item; `Write` returns `Buffer.Write: buffer closed failed: component
    already stopped`.

## Shape sweep (pin)

1. Second spelling: `pkg/buffer/circular.go:84-90` (pin) — `cb.stats.Overflow()` ... `cb.metrics.recordOverflow()`:
   every event is counted twice, in `Statistics` and in the optional collectors (same shape in `internal/cache`).
2. Copied shell: none (no Start/Stop/Health; `Close` only sets a flag and wakes waiters).
3. Layered interface with one implementation: `pkg/buffer/buffer.go:19` (pin) — `type Buffer[T any] interface {`,
   implemented only by `circularBuffer[T]`. `WriteWithTimeout`/`WriteWithContext` (`circular.go:348`, `:360`) are not
   on it, so a caller reaches them only by type assertion.
4. Exported surface with no reader:
   - `pkg/buffer/buffer.go:95` (pin) — `ContextKeyStats contextKey = "buffer-stats"`, `:99` `WithStats`, `:104`
     `StatsFromContext`: no caller at the pin outside the package, none in semsource/semconnect/semteams/semboids.
   - `pkg/buffer/statistics.go:69` (pin) — `func (s *Statistics) UpdateMemoryUsage(usage int64) {`: no caller
     anywhere, so `MemoryUsage()` (`:115`) and `StatsSummary.MemoryUsage` (`:233`) always read 0.
   - `Statistics` mutators `Write`, `Read`, `Peek`, `Overflow`, `Drop`, `UpdateSize`, `Reset`, and `NewStatistics`
     (`statistics.go:27-73`, `:191`): no caller outside the package.
   - `pkg/buffer/options.go:48` (pin) — `func WithDropCallback[T any]`: no production caller at the pin or in a
     consumer; offered by `README.md:80-90`, `:147-152`.
   - Unexported: `pkg/buffer/circular.go:24` (pin) — `notEmpty *sync.Cond`: signalled (`:143`, `:456`), never waited.
     `:384-385` — `var ctxWg sync.WaitGroup` / `ctxWg.Add(1)`: never waited.
5. Classification by error text: none (`grep -n -E 'Error\(\)|strings\.'` on production files: empty).

Further defects found in the read:

- `pkg/buffer/circular.go:93-96` (pin) — `// Call dropCallback outside the lock to avoid deadlock` /
  `defer cb.opts.dropCallback(droppedItem)`: deferred after `defer cb.mu.Unlock()` (`:68`), so it runs first, under
  the lock; same at `:110-112` and `:296-300` (Clear). The probe above confirms the deadlock.
- `pkg/buffer/circular.go:353` (pin) — `ctx, cancel := context.WithTimeout(context.Background(), timeout)`: an
  invented root. The pin lists it itself:
  `openspec/changes/archive/2026-08-21-simplify-one-shot-lifecycle-ownership/next-tag-closeout-inventory.md:526`
  (pin).
- `pkg/buffer/circular.go:373-374` (pin) — `case <-ctx.Done():` with no nil check: a nil ctx panics under Block;
  under the other policies (`:361-362`) the ctx is ignored.
- `pkg/buffer/circular.go:32-34` (pin) — `if capacity <= 0 { capacity = 1 // Minimum capacity`: undeclared in the
  constructor's doc comment (`buffer.go:109-112`).
- `pkg/buffer/options.go:39` (pin) — `if registry != nil && prefix != "" {`: an empty prefix is ignored silently.
- `pkg/buffer/doc.go:49` (pin) — `// err := buf.WriteWithContext(ctx, event)` and `README.md:124`, `:128`, `:471`:
  called on a `Buffer[T]`, which does not have the method; the examples do not compile.
- `pkg/buffer/metrics.go:77` (pin) — `registry.RegisterCounter(prefix, "buffer_writes", m.writes)`: the pin's
  `RegisterCounter` returns nil for an existing key and the buffer keeps its own candidate
  (`metric/registry.go:135-138` (pin)), so a second buffer under one prefix counts into ungathered collectors.

## 1. The claimed gap

"Nothing on main offers a bounded queue":
`git grep -n -i -E 'ErrQueueFull|BoundedDispatcher|ring buffer|circular|DropOldest|overflow policy' -- '*.go'`
finds only `service/message_logger.go:225` — `// Message storage (circular buffer); a slot belongs to the entry with
the highest sequence.` (a sequence-slot log, not a FIFO with policies). `internal/dispatch.KeyedPool` is a worker
pool with lanes (`openspec/specs/keyed-dispatch/spec.md:4`), not a queue a caller reads. `pkg/buffer` is absent:
`ls pkg` lists errs, fusion, graphview, lifecycle, lifecyclecleanup, platform, projection, retry, rulepack, security,
types. The gap holds.

## 2. Spellings of the fact (a bounded FIFO with overflow policy)

- `service/message_logger.go:225` (above): its own ring, newest-first reads by sequence; different semantics.
- `internal/dispatch` (ledger `pkg/dispatch`, `docs/admission-ledger.yaml:2248`): bounded lanes,
  `SubmitBlocking(ctx,...)`.
- `pkg/worker` and `BoundedDispatcher`: not ported (`docs/admission-ledger.yaml:2567-2583`, owner ruling E).

## 3. Adjacent claims

- Issues (`gh issue list --state open --search buffer`): #566 (this port, its ruling comment 6772860154); #34
  (output/websocket port; body: "`pkg/buffer` (carried in alpha.4 per #566; the port imports the engine copy, so #566
  merges first)"); #574 and #9: name the word, not the package.
- Open PRs (`gh pr list --state open`): #587 (this claim, 0 files); #588 (`openspec/changes/alpha4-graph-gateway/`
  proposal, spec, tasks): no overlap.
- Ledger: no `pkg/buffer` row (`grep -n 'source_path: pkg/buffer' docs/admission-ledger.yaml`: empty).
  `scripts/port-status.sh:99` already lists `pkg/buffer` under change 5.
- 04A D2: `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:171` lists `output/websocket` and
  `pkg/buffer`.
- Specs: none name the package (`git grep -n 'pkg/buffer' -- openspec/specs`: empty).

## 4. Who it is for

Nothing new; every symbol is the pin's. Callers:

- Pin production: `input/udp/udp.go:270-297`, `:635`, `:661`, `:478`; `input/websocket/websocket_input.go:363-384`,
  `:1300`, `:1311-1312`, `:1339`, `:1361`, `:700`; `output/websocket/websocket.go:1369-1370`, `:1654`.
- semsource `bbfb3c6`: `internal/entitypub/publisher.go:83` — `buf buffer.Buffer[*graph.EntityPayload]`; `:196` —
  `buffer.WithOverflowPolicy[*graph.EntityPayload](buffer.Block)`; `:198` `WithMetrics`; `:201` `NewCircularBuffer`;
  `:250-252` — `type timeoutWriter interface { WriteWithTimeout(item *graph.EntityPayload, timeout time.Duration)
  error }`;
  `:269-272` — `if tw, ok := p.buf.(timeoutWriter); ok { ... } else { err = p.buf.Write(owned) }`; `:307` —
  `Stats() *buffer.Statistics`; test `internal/entitypub/stop_settlement_test.go:15` embeds `buffer.Buffer`.
- semconnect `dff1265`, semteams `c8571b34`, semboids `37dbdb0`: no import.

## 5. Problem shape

A bounded in-memory queue with a declared overflow policy and a caller-bounded wait. Closest instance in the tree:
`internal/dispatch` `SubmitBlocking(ctx, key, work)` (keyed-dispatch spec): the wait is bounded by the caller's ctx.
The port adopts that rule for `WriteWithContext`. The metrics follow `internal/cache/metrics.go:22` (RegisterOrGet,
`semengine` namespace).

## 6. Pin decision records

Every record fetched at the pin (85 files).
`grep -l -i -E` over the pattern `pkg/buffer`, `CircularBuffer`, `NewCircularBuffer`, `OverflowPolicy`, `DropOldest`,
`DropNewest`, `WriteWithTimeout`, `backpressure`, `ring buffer`, `circular buffer`:
ADR-031, ADR-039, ADR-048, ADR-072, ADR-081. ADR-039 (tool-call governance) names `pkg/buffer/` only as "existing
buffer infrastructure for future batch evaluation" (`:281`, `:579-581`), and decides nothing about it. The others
match "backpressure" about other packages (BoundedDispatcher, graph-ingest, graphview). Titles on observability
(033, 098) do not govern a queue. Result: none governs `pkg/buffer`. SemEngine's ADR-113 governs R3 to R5.

Guidance: `grep -rln -i -E 'pkg/buffer|CircularBuffer|overflow polic'` over the pin's `.agents`, `.claude`,
`docs/contracts`: empty, so there is no guidance to bring.

## Adopter seam (SemSource, the websocket output port)

1. Must know: (a) the bounded write is reached by asserting `WriteWithTimeout` on the concrete type; (b) if the
   assertion fails, their fallback `Write` under Block waits until space or `Close`, with no bound; (c) each buffer
   needs its own metrics prefix, or two share one series; (d) a zero timeout or an ended ctx refuses even when there
   is space.
2. Do nothing: SemSource's assertion compiles against any type. If the method were renamed or removed, it would
   silently switch to unbounded blocking. A wrapper already does: `internal/entitypub/stop_settlement_test.go:14-17`
   (semsource bbfb3c6) — `type notifyingBuffer struct { buffer.Buffer[*graph.EntityPayload]; sealed chan struct{} }`
   has no `WriteWithTimeout`, so the assertion at `publisher.go:269` fails and that test runs plain `Write`.
3. Found out: nowhere for (b) (no compile or runtime signal); doc for (c), (d).
4. Should know: nothing; the gap is that `Buffer[T]` hides the bounded write (owner question Q1).
