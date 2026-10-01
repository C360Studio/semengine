# Tasks: setup-04a-01-floor

Each task names the outcome that proves it and the gate that checks it. An unchecked task that says "Hold:" is one
`task spec:queue` reports as blocked until the named review or ruling exists. "This pull request" is PR #48. Where
there is code, the failing test is written and shown failing before the code that makes it pass. Owner of each
task: **D** developer (`semengine-developer`), **R** reviewer (`semengine-reviewer`), **W** technical writer
(`semengine-technical-writer`), **A** architect. Evidence (test output, coverage, `go list` listings) is recorded on
this pull request as a comment unless a task says otherwise. No task asserts a fact that exists only after merge.

## 1. Design and spec acceptance

- [ ] 1.1 (A) `design.md` and the five deltas under `specs/` match the accepted foundation design (D2 row 1, D3–D7,
      D9, D10.1) and the #9 rulings; every requirement has at least one scenario; `task spec:check` passes with no
      `skip_specs`.
- [x] 1.2 Independent design review (PASS on re-check 2, 2026-10-01). The reviewer's verdict on `design.md` and `specs/`
      is a pass, recorded on this pull request with the reviewed files' checksums.
- [x] 1.3 Owner acceptance of this change on #9 (2026-10-01) (the chain is accepted; this is the first change's own
      scope).
- [x] 1.4 Owner ruling on design D7 (2026-10-01, on #9: typed `NoFallibleStart()`) (the required must-fail factory where
      an owner's start cannot fail: a typed `NoFallibleStart()` value of the `StartFailure` type, a fallible start under
      an adapt row, or exclusion). Recorded on #9 and in `design.md`; tasks 2.5, 3.1, 3.6 and 3.7 follow the ruling.
- [x] 1.5 Owner rulings of 2026-10-01 on #9 (comment 5941920346): Q1 a repaired test file makes its row `adapt`;
      Q2 `TestIntegration_Reconnection` is rewritten on `natsfixture.Restart` and proves re-dial; Q3 the three
      `test_client_*_test.go` files are not ported (`defer-exclude` with `test_client.go`) and counts are restated
      (the exclusion of `monitoring_consumers_test.go` follows from Q3 and is not part of the ruling, design D1);
      Q4 the seven doc-comment sleeps are carried as they are; D7 refined — `NoFallibleStart()` is reported through a
      pinned owner list in a contract test. Recorded in `design.md` D1, D5, D7, D8; tasks 2.5a, 3.1, 3.3, 3.6, 3.7
      follow.

## 2. Harness extension (first task, #9 item 6)

- [ ] 2.0 (D) Pre-port probe: in a throwaway test against the pin snapshot (not committed), run the suite's seven
      existing checks through a minimal adapter against each of the five owners (`metric.Server`, `natsclient.Client`,
      `TemporalResolver`, `CoalescingSet`, `resource.Watcher`) and record which checks fail on this pull request;
      every failure becomes a named, `adapt` item with a test written first on the owner's row before its port task
      runs; the probe's source is attached to the comment on this pull request, not committed. Known before the
      probe: `Client.Close(nil)` panics and `Connect(nil)` is unchecked (design P13).
- [ ] 2.0b (D) Pre-port repeat probe: against the pin snapshot (not committed), `go test -race -count=1 -cpu 1` and
      at least three runs of `go test -count=5 -cpu 1 -shuffle=on` over the 14 tested packages, each run's seed and
      failing tests recorded on this pull request. The three known `pkg/cache` failures
      (`TestCoalescingSet_EntityUpdateScenario`, `TestAttack_ConcurrentAddRemove`,
      `TestCoalescingSet_ContextCancellation`; design P19) and every other failing test become repair items on
      their package's port task; the design D8 hit list (P18) is attached alongside.
- [ ] 2.1 (D) `Fixture.Restart` and `CreateMemoryStream`: an integration test, written first and shown to fail on the
      base
      (no `Restart` method), creates one file-backed stream with `CreateStream` and one memory-backed stream with
      `CreateMemoryStream`, publishes one message to each, restarts, and asserts the file-backed message is readable
      through the new `JetStream()` and the memory-backed one is not; a second test asserts `URL()` dials after the
      restart; a third asserts a consumer with a running handler is ended before `Restart` returns and no handler runs
      afterwards; a fourth asserts `Restart` before `Start` returns an error with no Docker call. Gate: `task
      test:integration -- ./internal/harness/natsfixture`. The durability premise (design P3) is proven here, not
      assumed.
- [ ] 2.2 (D) Restart fault matrix: with the `stopContainer` and `startContainer` hooks made to return an error in turn,
      `Restart`
      returns a `FixtureError` naming the phase, no second container exists, and `Stop` observes the container gone;
      the sensitivity test trips exactly its phase. The one-replacement rule is unchanged (`maxAttempts` untouched).
- [ ] 2.3 (D) `FaultKV`: unit tests written first show `FailAfter(Update)` returns the injected error while a fresh
      read sees the new revision and `Calls()[Update] == 1`, and `FailBefore(Create)` leaves the key absent with
      count zero; `go vet` confirms the wrapper satisfies `jetstream.KeyValue`; `go list -deps
      ./internal/harness/natsfixture` shows no package of this module outside `internal/harness/`.
- [ ] 2.4 (D) `internal/harness/prochost`: tests written first show (a) the helper test is a no-op without the
      marker, (b) a started helper is in its own process group with output under the evidence directory, (c) a kill
      between two checkpoints leaves the first checkpoint's file and not the second's and `Wait` returns the killed
      status within its bound, (d) after `Pause` the process is observed stopped (`ps` state `T`, read through
      `probe.Await`) and the checkpoint count read while stopped is unchanged after the test writes the helper's
      next request, and after `Resume` the checkpoint arrives, (e) a test that returns with the helper running
      leaves no process behind, checked by start identity (`runner_test.go:255,293`) and not by pid alone; output
      goes under `SEMENGINE_EVIDENCE_DIR` when set and under `t.TempDir()` otherwise, and (b) asserts the fallback;
      no file in the package contains a sleep (`TestNoSleepsInTests` scans its non-test files). Gate: `task
      test:unit` and `task test:repeat -- ./internal/harness/prochost` (no Docker needed).
- [ ] 2.5 (D) `lifecycletest.Run(t, factory, mustFail StartFailure, promise)` with `MustFail(f)`, `NoFallibleStart()`
      and `CheckFailedStartHoldsNothing`: a test written first shows `Run` rejecting the zero `StartFailure` value
      before any check, naming the argument, and `MustFail(nil)` panicking at the call site naming the argument, and a
      compile-check test (a `_test.go` that must not build, run through
      `go vet` on a planted file) shows a bare `Factory` is not accepted in its place. The `refowner` double gains a
      must-fail construction mode (design D8 note): Start returns its error after `o.startAttempted = true`
      (`refowner_test.go:72`); the `checks` entry for the new check is marked must-fail and every test iterating
      `checks` — `TestEachFailpointTripsExactlyItsCheck` (`:382`) and `TestChecksPassAgainstCleanDouble`
      (`:239-249`) — builds the must-fail double for it; the clean must-fail double passes the new check. A
      `failpointTable` row `startFailsButHolds` (want `FailedStartHoldsNothing`) acts only in must-fail mode, is
      caught by the new check naming the unresolved item, trips no other check, and is ended by `finalize` with no
      exemption; `TestAbortStopThenFinishJoinsWorker` (`:414-418`) runs it as inert. A must-fail factory whose Start
      succeeds makes the check fail stating so. `Run` given `NoFallibleStart()` runs no failed-start subtest, calls no
      skip, logs no line in its place, and runs every other check. Both existing callers
      (`refowner_test.go:491-492`, `natsfixture/fixture_integration_test.go:519`) compile against the new signature.
      Gate: `task test:unit`; `task test:repeat -- ./internal/harness/lifecycletest`; `task cover:check` keeps
      `lifecycletest` at 80%.
- [ ] 2.5a (D) Pinned owner list (`lifecycle-suite` delta, "Pinned owners without a fallible start"): in
      `internal/harness/contract`, a sensitivity test written first, over planted trees, fails (a) on a
      `NoFallibleStart()` call missing from the pinned list, naming `file:line` and its key, (b) on a list entry with
      no call, naming the entry, (c) on any reference to `NoFallibleStart` other than a direct call in `Run`'s third
      argument — a variable, a function value, a helper, a dot import — naming `file:line`, (d) on two calls with
      the same key, naming both, or anywhere nested inside a `for` or `range` body (including a func literal passed
      to `t.Run`), naming `file:line`, (e) with a method's call keyed by its receiver type, (f) with one function
      running the same owner under two promises (as `refowner_test.go:491-492` does) as two keys, not a duplicate,
      and (g) with the bare identifiers `Run` and `NoFallibleStart` recognised inside package `lifecycletest`;
      then the check passes on the real tree with the list holding only
      `lifecycletest`'s own test of `NoFallibleStart()`. Port tasks 3.1, 3.6 and 3.7 each show the check failing
      on the new adapter's call before adding that owner's entry. Gate: `task test:unit`.
- [ ] 2.6 (D) The fixture's own must-fail factory: `TestS1_7Restart` passes `Run` a fixture whose `deps.start` fails;
      the start-failure check passes (nothing unresolved, Stop nil, no Docker call counted). Gate: `task
      test:integration -- ./internal/harness/natsfixture`.
- [ ] 2.7 (D) I8 and T-B8 in `internal/harness/contract`: a test over `go list -deps ./...` and `go.mod` fails on
      any `github.com/c360studio/semstreams` path; a tree-shape sensitivity test (like `TestImportGraphSensitivity`)
      shows the aggregator rule rejecting a package importing `Register` from two component packages and the
      SemStreams rule rejecting a planted import; both pass on the real tree. Gate: `task test:unit`.
- [ ] 2.8 (D) `internal/harness/semantictest` and `internal/harness/payloadfixture` carry the pin's helpers
      byte-for-byte except package path and import rewrites; T-B1 passes with them in the harness and a sensitivity
      case shows it rejecting the same files planted outside `internal/harness/`; `go list -deps` of each helper
      lists only pure-library packages of the set, and no test in those packages imports the helper (design P5).
- [ ] 2.9 (R) Harness review: the five additions against the deltas, the fault matrices' completeness, and the
      `natsfixture` import list. Verdict recorded on this pull request before any ported package lands.

## 3. Port mechanics, per package in design D1's order

For each of the 16 packages (one task line per package below): the ledger row is written first and `task
ledger:check` passes; the package and its `_test.go` files are copied from the pin at
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128` with the module path rewritten and the design D8 repairs applied, each
repair written so it fails first where the pin's test fails (P19) and listed on the row's `evidence` as pin
`file:line` → SemEngine `file:line`; a row with any repaired test file is `adapt` (task 1.5, Q1); `task verify`
passes; `task test:repeat -- ./<pkg>` passes; the carried tests pass in their lane; the row's `proving_tests` names
them; context roots in the package are triaged in `known_risks`.

- [ ] 3.1 (D) `pkg/platform`, `pkg/resource`, `pkg/retry`, `pkg/security`, `pkg/timestamp` (level 0): rows `carry`
      except `pkg/resource` (`adapt`, SS#1415-class ender and D8 repairs) and `pkg/retry` (`adapt`, D8 repair:
      `retry_test.go:68`); `pkg/resource`'s six sleeps (`watcher_test.go:151,208,324,333,335,363`) are repaired
      under `synctest` (D8 R1); `pkg/platform` and `pkg/security` rows record "no tests
      at the pin"; `resource.Watcher.Stop(ctx) error` is written behind a suite run that first fails on the
      unbounded wait — the test, written first and shown to fail on the pin, plants a check function that ignores
      its context and blocks until the test releases it, because `Stop` cancels the loop's context before `wg.Wait()`
      (`watcher.go:218-227`) and the
      wait is bounded otherwise; with that shape the adapter's `Stop` under an expired context never returns on the
      pin's code, and returns the context error after the change; the adapter lists the check goroutine and its
      cancel; the adapter passes `NoFallibleStart()` (D7) and its entry is added to the pinned list (task 2.5a),
      recorded on the row.
- [ ] 3.2 (D) `pkg/errs`, `vocabulary` (level 1): rows `carry`; `pkg/errs`'s row names `ErrAlreadyStopped`
      (`errs.go:47`) as the one sentinel and change 3 as the SS#1218 proof's home; the two `pkg/errs` context roots
      are triaged.
- [ ] 3.3 (D) `pkg/acme`, `pkg/types` (level 2): `pkg/types` row `carry`; `pkg/acme` row `adapt` (D8 R3: the
      `// +build integration` line at `integration_test.go:2` deleted, `TestNoHiddenTests` failing on it first);
      `go-acme/lego/v4` enters `go.mod` as direct; `task vuln` output recorded; `pkg/acme`'s one integration-tagged
      test runs in the integration lane.
- [ ] 3.4 (D) `pkg/projection/contract`, `pkg/tlsutil` (level 3): rows `carry`; `pkg/tlsutil`'s integration-tagged
      test runs in the integration lane.
- [ ] 3.5 (D) `metric`, `payloadregistry` (level 4): `metric` row `carry` with its two roots triaged; `metric.Server`
      adapter lists the listener, the `http.Server` and the serve goroutine; the suite runs with the bound-port
      must-fail factory (#38's first real owner) and passes; `payloadregistry` row `adapt` (`testing.go` rehomed, task
      2.8); `prometheus/client_golang` becomes direct.
- [ ] 3.6 (D) `message`, `pkg/cache` (level 5): `message` row `carry` (`google/uuid` direct; its two tests that used
      `internal/semantictest` import the harness copy); `pkg/cache` row `adapt`: its 26 sleeps are repaired under
      `synctest` with its 30 `t.Parallel()` calls removed (D8 R1); `TestCoalescingSet_EntityUpdateScenario`,
      `TestAttack_ConcurrentAddRemove` and `TestCoalescingSet_ContextCancellation` are shown failing first under a
      recorded `test:repeat` seed from task 2.0b, and after the repair `task test:repeat -- ./pkg/cache` passes on
      that seed and on three further runs whose seeds are recorded; `CoalescingSet.Close(ctx) error`
      written behind a suite run that first fails on the unbounded `<-c.done` — the test, written first and shown
      to fail on the pin, plants a callback that blocks until released, the only shape under which the wait is
      unbounded — and then passes; the adapter lists the ticker and the `run` goroutine; the adapter passes
      `NoFallibleStart()` (D7) and its entry is added to the pinned list (task 2.5a), recorded on the row; the row's
      `known_risks` names the two later callers
      (`processor/graph-embedding/component.go:875`, `processor/rule/entity_watcher.go:980`) as change 5 and 6
      port-refactor rows; the one `pkg/cache` root is triaged.
- [ ] 3.7 (D) `natsclient` (level 6): row `adapt`; `test_client.go` and `test_options.go` are not ported and their
      file rows are updated (`evidence` on the `adapt` row); four test files are not ported and get `defer-exclude`
      file rows with design D1's reasons: `test_client_factory_test.go`, `test_client_integration_test.go` and
      `test_client_readiness_test.go` by owner ruling (task 1.5, Q3), and `monitoring_consumers_test.go` as forced by
      Q3 (`WithMonitoring` is at `test_client.go:448`, not ported; not an owner ruling); the 56
      `NewTestClient` sites in the ported files are rewritten to `natsfixture` (count recorded: 56 before, 0 after);
      the 37 sleeps (8 unit in `client_test.go`, 29 integration) are repaired per design D8 R1;
      `TestIntegration_Reconnection` (`integration_test.go:63`) is rewritten on `natsfixture.Restart` with no skip
      call: the disconnect callback is observed through a channel, the client is observed unhealthy through
      `probe.Await`, and a client dialled from the new `URL()` is observed healthy; its name and comment say it
      proves re-dial, not nats.go's automatic reconnect (D8 R2); the nine roots are triaged; the nats.go v1.52→v1.54
      difference is recorded in `known_risks` with the carried tests as the only regression evidence; the 46 unit
      test files pass under `task test:unit` and `task test:repeat -- ./natsclient`, and the 28 integration-tagged
      files under `task test:integration -- ./natsclient`; adapters for `Client` (lists conn, JetStream handle,
      subscriptions, consumer claims, health and metrics goroutines) and `TemporalResolver` (lists the history cache)
      pass the suite — `Client` with a refused-URL must-fail factory, `TemporalResolver` with `NoFallibleStart()`
      (D7) and its entry added to the pinned list (task 2.5a); `Close(nil)`
      and `Connect(nil)` are refused behind tests written first and shown to fail on the pin (the pin's
      `Close(nil)` panics on a connected client, `client.go:686`), recorded on the row as changed behaviour
      (`adapt`) alongside every other probe finding from task 2.0.
- [ ] 3.8 (D) `task cover:check` targets `natsclient`, `message`, `payloadregistry` at 80%: the first measurement
      of each is recorded on this pull request (design P8: the pin baseline is unmeasured). If any of the three
      measures below 80%, a new task asking the owner to rule on that package's coverage is added to this file at
      that moment, written so `task spec:queue` reads it, and the gate stays unwaived (plan `:206-209`); nothing in
      this file waits on the owner until the measurement exists.
- [ ] 3.9 (D) `stretchr/testify` is a direct requirement; `task tidy:check` passes; the ported test-file count is 123
      (127 at the pin less the four excluded files) and the line count after repair is recorded with `wc` next to
      the pin's 33,494, with a diff stat against the pin per package.
- [ ] 3.10 (R) Port review per package group (3.1–3.7): adapters list every retained kind (the review checklist of
      the `lifecycle-suite` delta), rows validate, no file beyond the pin's was added except adapters and the
      rewritten `NewTestClient` sites, and every D8 repair is on its row; every real-clock timer in a repaired file
      is either an R1b failure bound sized per D8 or a site in D8's disposition table with that disposition (a
      review check the text check cannot make); the owner named in each pinned-list entry (task 2.5a) is the owner
      the adapter's factory builds. Verdict on this pull request.
- [ ] 3.11 (D) CI time: the wall time of `task verify` per step (`scripts/verify.sh` prints it) and of the CI `verify`
      job with every package ported are recorded on this pull request, with the integration lane's time against its
      `-timeout 10m` (`scripts/test-integration.sh:402`). If the job exceeds its 15-minute limit (`merge-gate`
      "Required needs both jobs"; `ci.yml:22`) or the lane exceeds its timeout, a new task asking the owner to rule is
      added to this file at that moment, written so `task spec:queue` reads it, and the limit stays unchanged: the
      limit is spec, and neither the developer nor the reviewer may raise it.

## 4. Repair evidence this change can produce (ruling g)

- [ ] 4.1 (D) Settlement, `natsclient` half: the 18 unit and 3 integration settlement tests pass against
      `natsfixture`; the `transport-client` "Settlement follows the decision" scenarios map to named tests (long work
      with heartbeat → `TestIntegrationConsumeDeliveryWithHeartbeatHealthyRenewalPreventsOverlap`; semantic retry →
      `TestIntegrationSemanticRetryProducesDurableRedelivery`; work panics →
      `TestConsumeDeliveryWithHeartbeatControlLossNormalizesInvalidAndPanic` and
      `TestConsumeDeliveryWithHeartbeatPanicAndZeroPolicyFailClosed`, `delivery_settlement_test.go:690,739`); the row
      names change 2 for the graph-ingest half.
- [ ] 4.2 (D) Acknowledged is not durable, `natsclient` half: an integration test publishes through the ported client
      to a memory-backed and a file-backed stream, restarts the fixture (task 2.1's primitive), and asserts absence
      and presence respectively; the `natsclient` row names it and change 2 for the graph-ingest scenarios.
- [ ] 4.3 (D) `Close`/`Drain` bounded: tests show `Drain(nil)` refused without a server call (carried), `Close(nil)`
      and `Connect(nil)` refused with the connection untouched (new, test written first, task 3.7), `Close` under a
      short
      deadline returning within it with the connection closed, and a second `Close` returning nil (carried where
      the pin has them; new ones where the row records a gap).

## 5. Ledger items that need no port, and boundary gates

- [ ] 5.1 (W) Eight `defer-exclude` rows for the D4 ten-out packages never carried (`agentic`, `agentic/agentrun`,
      `gateway`, `gateway/graph-gateway`, `internal/agentterminal`, `internal/deliverylane`, `internal/looptoken`,
      `vocabulary/agentic`) at the pin SHA with the D4 reason each; `task ledger:check` passes.
- [ ] 5.2 (A) The Tier-1 cross-check re-measured on the ruled 65-package set (#9 item 1), recorded next to the
      ledger as an inventory with its command and result; it changes no row.
- [ ] 5.3 (D) Package-doc lint sensitivity: `package-comments` is already on (`revive.toml:22`); `task lint` passes
      over the ported tree (every package in the set has a package comment at the pin, inventory §3.3), and a run
      with one public package's comment removed fails naming it; the eight public destinations (design D5) are the
      ones the check protects.
- [ ] 5.4 (D) `scripts/cover-check.sh` reads its targets from a list that this and later changes extend, adding
      `natsclient`, `message` and `payloadregistry`, which lie outside the `internal/harness` base the script
      hard-codes today (`cover-check.sh:14`), and `natsclient`'s statements come from the merged unit and integration
      profiles; `TestCoverCheckSensitivity` (`cover_test.go:37`), which today names only the three harness packages,
      gains a below-80% and a missing-from-profile case for a package outside `internal/harness`, written first and
      failing; `TestCoverCheckPrintsFailingTest` (`cover_test.go:134`, flake-defense 4.4, merged) still passes.

## 6. Docs

- [ ] 6.2 (W) `docs/testing.md` (PR #39) or its successor gains: the `Restart` contract (`URL()` valid until the next
      restart; stop the owner before, start after), `FaultKV`'s before/after semantics, the helper-process pattern,
      the required must-fail factory and the pinned list of owners without a fallible start, the adapter checklist,
      and the repair classes for a ported test (design D8); written for a working developer, each coined term
      defined at first use.
- [ ] 6.3 (W) The `openspec/specs/` sync: the five deltas applied to `harness-boundaries`, `nats-fixture`,
      `lifecycle-suite` and the two new capabilities, verified against the code as landed; `task spec:check`
      passes.

## 7. Review and archive

- [ ] 7.1 Hold: independent change review. The reviewer's verdict on the full diff (harness, 16 packages, rows,
      gates, docs) is a pass recorded on this pull request with the reviewed commit; a critical-stage read applies
      because this change adds new exported harness surface and changes `lifecycletest.Run`.
- [ ] 7.2 (D) `task verify` green on the final commit; the integration lane green under the host lock, evidence
      directory attached to this pull request; `implemented-by:` in the pull request body.
- [ ] 7.3 (W) The change archived under `openspec/changes/archive/` as the last content commit before squash merge;
      `task spec:queue` shows no open hold.
