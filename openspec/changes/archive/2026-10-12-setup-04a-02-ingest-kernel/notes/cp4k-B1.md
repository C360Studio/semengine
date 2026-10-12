# Codex B1 notes: an interrupted input counts once in Health's error count

## Replaced (owner ruling, #91 comment 6124130342)

The fix recorded below, `9fc8b0c`, was rejected. It carried a per-input count (`heldErrors`) to the write sites as a
`context` value, and the owner ruled: "ctx as bag of values is an anti pattern we are not going to start now." No
`context.WithValue` or `ctx.Value` carries state in graph-ingest: `git grep -n 'WithValue\|ctx.Value' --
processor/graph-ingest/` prints nothing.

The ruling settles B1 another way: an interruption is counted in `work_interrupted_total{cause}`, not in the error
count `Health` reads. The commit that replaces `9fc8b0c`, on `e362446`:

- removes `heldErrors`, `heldErrorsKey`, `countError` and their use in `processIngest`; the three write sites count
  with `atomic.AddInt64(&c.errors, 1)` again (`component.go:2240`, `:2343`, `:2354`), as on `93dfe06`;
- removes `recordInterruptedWork`'s `atomic.AddInt64(&c.errors, 1)` (`keyed_ingest.go:346`). It still logs the
  interruption at ERROR and adds one to `work_interrupted_total` under its cause; the input is still left unsettled
  and the lane keeps running (ruling 6109278240, item 1, which otherwise stands);
- so a write the interruption cut counts once, at its own site, and an interruption anywhere else adds nothing. Until
  #183 lands, an interruption alone no longer makes graph-ingest report unhealthy, and `DataFlow`'s `ErrorRate`, which
  reads the same counter (`component.go:905`), no longer sees it;
- changes the texts that stated the old count: the `graph-ingest-recovery` delta's requirement and its "Application
  panics" scenario, design D8's "*An interrupted apply does not stop ingest*", and the doc comments of
  `recordInterruptedWork` and `newWorkInterruptedMetric`. `processor/graph-ingest/README.md` never said an
  interruption raises the error count, so it is unchanged. `tasks.md` (task 4.20, and the line of task 4.4's
  description that names the count) is not edited here.

### Tests

`TestInterruptedApplicationIsRedeliveredNotAStop` (`processor/graph-ingest/interrupted_work_test.go:68`): in every
case `work_interrupted_total` rises by exactly one under the case's cause and the input is unsettled, as before. The
error-count assertion (`:226`) now wants no rise for "application panics", "in-progress signal fails" (the guard-read
case) and "delivery has no metadata", and a rise of one for "in-progress signal fails during the entity write" and
"during a container birth": the cut write's own count, beside its own ERROR line. The case field `writeLogged` is
renamed `writeFailed`, since it now sets both.

`TestOrdinaryLaneWriteErrorCountsOnce` (`:375`) and `TestDirectWriteErrorCountsAtOnce` (`:404`) are kept: each
still holds behaviour the code has (an ordinary write failure on the lane counts once and is not interrupted work; a
write called directly counts its failure once before it returns). Their doc comments no longer speak of a held count.

Fail before, on `e362446` with only the test changed:

```text
$ go test -race -count=1 -timeout=60s -run '^(TestInterruptedApplicationIsRedeliveredNotAStop|TestOrdinaryLaneWriteErrorCountsOnce|TestDirectWriteErrorCountsAtOnce)$' ./processor/graph-ingest/
--- FAIL: TestInterruptedApplicationIsRedeliveredNotAStop (0.02s)
    --- FAIL: TestInterruptedApplicationIsRedeliveredNotAStop/application_panics (0.01s)
        interrupted_work_test.go:226: Health's error count rose by 1, want 0
    --- FAIL: TestInterruptedApplicationIsRedeliveredNotAStop/in-progress_signal_fails (0.00s)
        interrupted_work_test.go:226: Health's error count rose by 1, want 0
    --- FAIL: TestInterruptedApplicationIsRedeliveredNotAStop/delivery_has_no_metadata (0.00s)
        interrupted_work_test.go:226: Health's error count rose by 1, want 0
FAIL
FAIL    github.com/c360studio/semengine/processor/graph-ingest    0.379s
FAIL
```

Exit 1. The two write cases passed on the base, where the held count made them rise by one.

### Mutation records

Each `task mutate:check` ran the named subtest three times on the unchanged code, three times on the wrong change
(copied to `/tmp/cp4k-b1b-mutants/`, outside the repository, on this machine only) and once on the unchanged code
again. Both exited 0 with the verdict `detection: each of the 3 mutant runs failed the named test with an expected
line, and the after-run passed`.

1. `count-interruption.go` puts `atomic.AddInt64(&c.errors, 1)` back in `recordInterruptedWork`.
   `-file processor/graph-ingest/keyed_ingest.go -test
   'TestInterruptedApplicationIsRedeliveredNotAStop/in-progress_signal_fails' -expect interrupted_work_test.go:226`.
   Each mutant run printed `interrupted_work_test.go:226: Health's error count rose by 1, want 0`.
2. `uncount-cut-write.go` removes the count at the compare-and-set's failure (`component.go:2240`).
   `-file processor/graph-ingest/component.go -test
   'TestInterruptedApplicationIsRedeliveredNotAStop/in-progress_signal_fails_during_the_entity_write' -expect
   interrupted_work_test.go:226`. Each mutant run printed `interrupted_work_test.go:226: Health's error count rose by
   0, want 1`.

No survivors and no inconclusive runs.

### Not covered

- The encoding failure at `component.go:2343` still has no test of its own; the container-birth case holds `:2354`.
- The race in which the application returns a cut write's error in the same tick the heartbeat fails has no test.
  It no longer changes the count: the write counts once at its site whichever case the helper's `select` takes, and
  the interruption adds nothing.

The sections below are the record of the rejected fix, unchanged.

The finding is Codex's review record on PR #93, comment 6123602840, B1 (blocking). The base is `93dfe06`. The
contract is the change's `specs/graph-ingest-recovery/spec.md`, "An interrupted application is redelivered, not a
stop" (`:105-110`): an interrupted input is counted once in the error count its health reads.

## Failing first

The test gained two cases in `TestInterruptedApplicationIsRedeliveredNotAStop`
(`processor/graph-ingest/interrupted_work_test.go`): "in-progress signal fails during the entity write" (`:115`), the
reviewer's case, which holds the compare-and-set's read of the entity, and "in-progress signal fails during a
container birth" (`:135`), which turns hierarchy inference on and holds the first container's create. The
guard-read case stays. On the base, with only the test changed:

```text
$ go test -race -count=1 -timeout=60s -run '^TestInterruptedApplicationIsRedeliveredNotAStop$' ./processor/graph-ingest/
--- FAIL: TestInterruptedApplicationIsRedeliveredNotAStop (0.02s)
    --- FAIL: TestInterruptedApplicationIsRedeliveredNotAStop/in-progress_signal_fails_during_the_entity_write (0.00s)
        interrupted_work_test.go:220: Health's error count rose by 2, want 1
    --- FAIL: TestInterruptedApplicationIsRedeliveredNotAStop/in-progress_signal_fails_during_a_container_birth (0.00s)
        interrupted_work_test.go:220: Health's error count rose by 2, want 1
FAIL
FAIL    github.com/c360studio/semengine/processor/graph-ingest    0.298s
FAIL
```

Exit 1. The other four cases passed.

## The counted sites

`grep -n 'atomic.AddInt64(&c.errors' processor/graph-ingest/*.go` on the base lists ten sites. Callers were read with
`gopls references` and `gopls call_hierarchy`. "Reachable" means `applyIngest`, run by
`natsclient.ConsumeDeliveryWithHeartbeat` in `processIngest`, can reach the site while the helper can cancel it.
Line numbers are the base's; the edit kept them.

| Site | What it counts | Reachable from the application | Now |
| --- | --- | --- | --- |
| `component.go:1559` | a delivery with no metadata, in the consume closure | no: the closure runs before the input is submitted to a lane | unchanged |
| `component.go:1590` | a poison message, in the consume closure | no: same closure, before submission | unchanged |
| `component.go:1718` | `handleMessage`'s decode failure | no: `handleMessage` has no production caller (tests only) | unchanged |
| `component.go:2239` | `mergeEntityOnLane`'s compare-and-set failure | yes: `applyIngest` → `ingestEntity` → `mergeEntityOnLane`; the reviewer's case | `c.countError(ctx)` |
| `component.go:2342` | `createEntityWithReceipt`'s encoding failure | yes, with hierarchy on: `mergeEntityOnLane` → `addToContainers` → `HierarchyInference.AddToContainers` → `hierarchyStore.CreateEntity` → `CreateEntity` (`graph/inference/hierarchy.go:258`) | `c.countError(ctx)` |
| `component.go:2353` | `createEntityWithReceipt`'s refused create | yes, with hierarchy on: same path; the container-birth case | `c.countError(ctx)` |
| `component.go:2393` | `deleteEntityAtRevision`'s refused delete | no: only `handleCanonicalDelete` calls it (`canonical_mutations.go:492`) | unchanged |
| `component.go:2600` | `addTriplesLane`'s cancelled batch | no: only `handleCanonicalAppend` calls it (`canonical_mutations.go:394`) | unchanged |
| `component.go:2606` | `addTriplesLane`'s failed subject | no: same | unchanged |
| `keyed_ingest.go:347` | `recordInterruptedWork`, the interruption's own count | it is the one count | unchanged |

No request handler shares a reachable site: the handlers write through `createEntity`, `deleteEntityAtRevision` and
`addTriplesLane`, none of which the application calls. The reachable sites' other callers are direct: `ingestEntity`
(through `handleMessage`, tests only) and the exported `CreateEntity`, whose only production caller is the hierarchy
store, plus any consumer that calls it.

## Mechanism

`processIngest` gives each application a `heldErrors` (`keyed_ingest.go:361-383`) in its context. The three reachable
sites call `c.countError(ctx)`, which adds to the held count when the context carries one and to `c.errors` when it
does not. After `ConsumeDeliveryWithHeartbeat` returns, `processIngest` drops the held count if the helper reports an
interruption (`recordInterruptedWork` is that input's one count) and otherwise adds it to `c.errors`
(`keyed_ingest.go:225-232`). The helper joins the application on every path before it returns
(`natsclient/delivery_settlement.go:336-374`), so the held count is final when it is read.

Why this one:

- An interrupted input gets exactly one count, however far its application got: the held count is dropped whole. That
  also covers what a narrower check would miss: two counted failures in one application (a container create refused
  before a second one is interrupted), and the race where the application has already returned a counted error in the
  tick the heartbeat fails (the helper's `select` takes either ready case).
- An ordinary write error on the lane is added once, as before; a direct call counts at once, as before.
- The context is the only thing that reaches the container sites: the application reaches them through hierarchy
  inference, whose `EntityStore.CreateEntity` takes a context and an entity. The rejected alternatives:
  - Skip `recordInterruptedWork`'s count when the application "already counted". This needs the same per-application
    channel and leaves two counts when two sites count in one application.
  - Skip the count at a site when `ctx.Err()` is set. This changes direct-call behaviour, still counts a write cancelled
    at Stop, and misses the race above.
  - Mark counted errors with a wrapper type. This changes the dynamic type of errors the exported `CreateEntity` returns.
- `natsclient`, the decision mapping, the settlement order, and `recordInterruptedWork`'s metric and log are unchanged.

The four questions of the developer contract, "Before adding anything new":

1. Who counts today: `c.errors`, added to directly at the ten sites above; nothing counted per application.
2. The premise was measured: the failing run above reproduces the reviewer's line on the base.
3. Consumer at birth: `processIngest` is the only reader of `heldErrors`; `countError` is called at the three
   reachable sites.
4. Nothing is asked of a caller: the held count and its key are unexported, and a direct caller sees no change.

`heldErrors` is a package-local mechanism, not a cross-package primitive, so no package is listed to adopt it.

The hole class is any site reachable from `applyIngest` that adds to `c.errors` directly instead of through
`countError`. The table enumerates every site there is today. A new reachable site that adds directly would reopen the
double count; nothing but review catches that.

## Tests on the other side

- `TestOrdinaryLaneWriteErrorCountsOnce` (`interrupted_work_test.go:373`): the entity bucket refuses the create, with
  no interruption. The input is negatively acknowledged, Health's error count rises by one, and no cause of
  `work_interrupted_total` rises. A refused create ends the compare-and-set at once (`natsclient/kv.go:434-447`), so
  the test waits on no retry. It passed on the base.
- `TestDirectWriteErrorCountsAtOnce` (`:402`): `ingestEntity` and the exported `CreateEntity`, both called directly
  with a refused create, raise the count by one before they return. It passed on the base.

The test now accepts one more ERROR line in the two write cases: `ingestEntity`'s own "Failed to merge entity",
naming the entity. Other ERROR lines are still refused. The case field `writeLogged` (`:76`) says when the line is
expected, and the test requires it exactly once then (`:222-246`). The reviewer saw this line in the reproduction and
did not file it as a separate finding. The log is unchanged: scope is frozen to the count.

Generated checks: not used. The input cases are where the interruption lands (before the write, in the entity's
read, in a container's create) and whether a write fails ordinarily. Each count site the application reaches has a
named case. There is no stated law, and no history beyond one input and the next on the same lane.

## Mutation records

Each `task mutate:check` ran the named test on the unchanged code, three times on the wrong change (copied to
`/tmp/cp4k-b1-mutants/`, outside the repository) and on the unchanged code again. All three exited 0.

1. `count-twice.go` adds the held count in the interrupted branch too, before `recordInterruptedWork`.
   `-test TestInterruptedApplicationIsRedeliveredNotAStop -expect interrupted_work_test.go:220`. Verdict:
   `detection: each of the 3 mutant runs failed the named test with an expected line, and the after-run passed`.
   Each run printed `interrupted_work_test.go:220: Health's error count rose by 2, want 1` twice, once for each
   write case.
2. `drop-ordinary.go` removes the line that adds the held count after a settled input.
   `-test TestOrdinaryLaneWriteErrorCountsOnce -expect interrupted_work_test.go:389`. Verdict: detection, the same
   wording. Each run printed `interrupted_work_test.go:389: Health's error count rose by 0, want 1`.
3. `drop-direct.go` removes `countError`'s direct count. `-test TestDirectWriteErrorCountsAtOnce -expect
   interrupted_work_test.go:412`. Verdict: detection, the same wording. Failure locations `:412` and `:424`, both
   subtests.

No survivors and no inconclusive runs.

## Not covered

- The race in which the application returns a counted error in the same tick the heartbeat fails has no test: the
  helper's `select` picks one of two ready cases at random, so a test of it could not be made deterministic. The
  mechanism covers it because the held count is dropped on every interruption, whatever the application returned.
- The encoding failure at `component.go:2342` has no test of its own. It goes through `countError` as `:2353` does,
  and the container-birth case holds `:2353`.
- A panic after a counted failure in the same application has no test. The held count is dropped on the panic's
  interruption as on any other.
- Timing: on the lane, an ordinary write error is now added after the helper settles the input, not before. A Health
  read between the Nak and `processIngest`'s return sees the count one input late. No test reads it in that window.
- Seen, not changed: an interrupted write logs two ERROR lines, `ingestEntity`'s and `recordInterruptedWork`'s. A write
  cancelled at Stop (the helper's own context ends, not an interruption) still counts once through the held count, as
  before. `DataFlow`'s `ErrorRate` reads the same counter (`component.go:905`, `:921`) and now sees one count per
  interrupted input.
