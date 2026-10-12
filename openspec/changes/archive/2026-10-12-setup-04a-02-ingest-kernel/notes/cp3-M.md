# Checkpoint 3 M1, M2 and M3 notes

Nil contexts are refused at the write entries; two tests own their waits.

The findings are Codex's checkpoint 3 record, PR #93 comment 6099921473, M1, M2 and M3, and its portable probe
`TestReviewCreateEntityRefusesNilContext`. The task is 4.14. The base is `385c244`.

## Choices the findings do not spell out

1. **M1, the error.** Both guards return `errs.WrapInvalid(errs.ErrInvalidData, <type>, <method>, "nil context")`,
   the spelling `internal/lifecycleguard/guard.go:87` and `metric/handler.go:75` already use. No new sentinel: no
   caller tells a nil context apart from other invalid input.
2. **M1, where the guards sit.** In `CreateEntity` itself (`component.go:2069`), not in `createEntityWithReceipt`:
   the private helper relies on its callers, as the developer contract § Context ownership allows. In
   `AddToContainers` (`graph/inference/hierarchy.go:128`), first, before the zero-time check and the three skips
   (container, foreign authority, invalid ID), so a nil context is refused whichever path the ID would take, not
   only where storage is called.
3. **M1, the class.** The exported context-taking methods of the two packages are `Component.Start` and
   `Component.Stop` (both through `lifecycleguard`, which refuses nil), `Component.CreateEntity` and
   `HierarchyInference.AddToContainers`. After this change every one refuses nil.
4. **M1, the graph-ingest test** keeps the probe's entity and adds a control: the same entity is stored under a real
   context, so the refusal cannot pass because the entity was invalid for another reason.
5. **M2, the fix** is the precedent's one line (`t.Cleanup` of `Close`, as at `poison_scoping_test.go:424`); the
   cache keeps `context.Background()`, which `Close` does not depend on.
6. **M2, no committed failing test.** graph-ingest has no goroutine-leak check, and adding one is beside the finding.
   The evidence is a probe kept outside the repository (below).
7. **M3, the barrier.** graph-ingest has no seam inside the apply itself. The test wraps the real handler through
   the `consumeStream` acquisition seam (`component.go:399`, used at `:1518`), which
   `lifecycle_integration_test.go:123` and `failed_start_callbacks_integration_test.go:254` already wrap. The first
   delivered message waits on a channel before the real handler runs. A held message is not acked, so the producer's
   outstanding count (`readiness.go:222-225`, pending plus ack-pending) is at least one on every status tick, and
   `ComputeBacklogStatus` (`graph/readiness/index_status.go:111`) cannot report ready. The status loop starts
   (`component.go:1062`) after the consumers bind (`:1006`), so no tick reads an empty consumer set.
8. **M3, what stays.** The 200-message backlog, the `probe.Await` predicate and the catch-up and scope assertions are
   unchanged; the backlog is no longer what makes the not-ready window observable. The release is deferred as well,
   so a failed assertion cannot hold `Stop` (defers run before `owner.finish`).

## Evidence

- Fail before, on `385c244`: `TestComponent_CreateEntity_RefusesNilContext` panicked with a nil-pointer dereference
  at `component.go:2261` (`createEntityWithReceipt`); `TestAddToContainersRefusesNilContext` failed all four rows
  at `hierarchy_test.go:268` with `= <nil>, want an error classified invalid`.
- M2 probe, `TestProbeM2CacheGoroutineEnds`, added by `go test -overlay` as `zz_probe_m2_test.go`: it runs
  `TestDetectionInvalidatesCachedEntry` as a subtest, then reads the goroutine profile up to 1,000 times (with
  `runtime.Gosched` between) for `internal/cache.(*hybridCache[...]).cleanup`. On `385c244` it failed: the goroutine
  was alive. After the fix it passed with `-count=10`.
- M3 fail before, by hand: a demonstration block (never committed) that waits up to 10s for the envelope to read
  caught-up before the first poll, standing in for a test goroutine descheduled while ingest finishes. The base test
  with it failed at its `require.True(t, sawNotReady, ...)` (HEAD line 206) reading `Ready:true ... Lag:0`; the new
  test with it passed (12.4s: nothing reads caught-up while a message is held).
- After the fix: `go test -race -count=1 -cpu=1` of the two M1 tests, and `task test:integration -- -run
  '^TestIntegration_ReadinessEnvelope_BacklogIsNotReady$' ./processor/graph-ingest/`, pass.

## Mutation records

M1, each `task mutate:check -- -pkg <pkg> -test <test> -file <file> -mutant <copy> -expect <line>`, the copy kept
outside the repository. Each printed `verdict: detection: each of the 3 mutant runs failed the named test with an
expected line, and the after-run passed` and exited 0.

Tests: C is `TestComponent_CreateEntity_RefusesNilContext`, A is `TestAddToContainersRefusesNilContext`.

| Wrong change | Test | Expected line | Verdict |
| --- | --- | --- | --- |
| The `CreateEntity` guard returns nil | C | `component_test.go:817` | detection |
| The `CreateEntity` guard returns `errors.New` (unclassified) | C | `component_test.go:817` | detection |
| The `CreateEntity` guard writes the entity, then refuses | C | `component_test.go:823` | detection |
| The `AddToContainers` guard returns `nil, nil` | A | `hierarchy_test.go:268` | detection |
| The `AddToContainers` guard moved after the skips | A | `hierarchy_test.go:268` | detection |

M3, by hand (#129: each edit in place after a `cp` backup, restored, SHA-256 equal before and after; one run each
through `task test:integration`; `:238` is `require.True(t, sawNotReady, ...)`):

| Wrong change | Failed at | Observed |
| --- | --- | --- |
| The barrier removed (the first message signals, unheld), under the forced deschedule | `:238` | fail, `Ready:true` |
| `observeOutstandingWork` counts pending, not ack-pending (`readiness.go:225`) | `:238` | fail, `Ready:true` |

M2 has no mutation record: its fix is in a test file. The probe's before and after runs are its evidence.

## Not covered

- M2's leak check is a probe, not a committed test; nothing fails if a later test in the package leaks a cache.
- The M3 mutants ran once each, not three times.
- Candidate follow-up, not in this commit: `TestIntegration_ReadyImpliesTheWritesAreDurable`
  (`readiness_integration_test.go`) states that "only a backlog" lets a status tick sample a not-ready state; it was
  not read for the same scheduling dependency.
