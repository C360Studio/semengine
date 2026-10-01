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

## 2. Harness extension (first task, #9 item 6)

- [ ] 2.0 (D) Pre-port probe: in a throwaway test against the pin snapshot (not committed), run the suite's seven
      existing checks through a minimal adapter against each of the five owners (`metric.Server`, `natsclient.Client`,
      `TemporalResolver`, `CoalescingSet`, `resource.Watcher`) and record which checks fail on this pull request;
      every failure becomes a named, `adapt` item with a test written first on the owner's row before its port task
      runs; the probe's source is attached to the comment on this pull request, not committed. Known before the
      probe: `Client.Close(nil)` panics and `Connect(nil)` is unchecked (design P13).
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
      status within its bound, (d) pause stops progress and resume restores it, (e) a test that returns with the
      helper running leaves no pid behind; output goes under `SEMENGINE_EVIDENCE_DIR` when set and under
      `t.TempDir()` otherwise, and (b) asserts the fallback. Gate: `task test:unit` (no Docker needed).
- [ ] 2.5 (D) `lifecycletest.Run(t, factory, mustFail StartFailure, promise)` with `MustFail(f)`, `NoFallibleStart()`
      and `CheckFailedStartHoldsNothing`: a test written first shows `Run` rejecting the zero `StartFailure` value
      before any check, naming the argument, and `MustFail(nil)` panicking at the call site naming the argument, and a
      compile-check test (a `_test.go` that must not build, run through
      `go vet` on a planted file) shows a bare `Factory` is not accepted in its place; the `refowner`
      double gains a `startFails` failpoint declared in its table with the new check as its expected check, and
      `TestEachFailpointTripsExactlyItsCheck` trips exactly it; a second failpoint `startFailsButHolds` is caught by
      the new check naming the unresolved item; a must-fail factory whose Start succeeds makes the check fail stating
      so; per the D7 ruling (task 1.4), `NoFallibleStart()` makes the start-failure subtest report "no fallible start"
      and run every other check. Both existing callers (`refowner_test.go:303-304`,
      `natsfixture/fixture_integration_test.go:519`) compile against the new signature. Gate: `task test:unit`;
      `task cover:check` keeps `lifecycletest` at 80%.
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
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128` with only the module path rewritten; `task verify` passes; the carried
tests pass in their lane; the row's `proving_tests` names them; context roots in the package are triaged in
`known_risks`.

- [ ] 3.1 (D) `pkg/platform`, `pkg/resource`, `pkg/retry`, `pkg/security`, `pkg/timestamp` (level 0): rows `carry`
      except `pkg/resource` (`adapt`, SS#1415-class ender); `pkg/platform` and `pkg/security` rows record "no tests
      at the pin"; `resource.Watcher.Stop(ctx) error` is written behind a suite run that first fails on the
      unbounded wait — the test, written first and shown to fail on the pin, plants a check function that ignores
      its context and blocks until the test releases it, because `Stop` cancels the loop's context before `wg.Wait()`
      (`watcher.go:218-227`) and the
      wait is bounded otherwise; with that shape the adapter's `Stop` under an expired context never returns on the
      pin's code, and returns the context error after the change; the adapter lists the check goroutine and its
      cancel; the must-fail factory follows the D7 ruling (`NoFallibleStart()` assumed, recorded on the row).
- [ ] 3.2 (D) `pkg/errs`, `vocabulary` (level 1): rows `carry`; `pkg/errs`'s row names `ErrAlreadyStopped`
      (`errs.go:47`) as the one sentinel and change 3 as the SS#1218 proof's home; the two `pkg/errs` context roots
      are triaged.
- [ ] 3.3 (D) `pkg/acme`, `pkg/types` (level 2): rows `carry`; `go-acme/lego/v4` enters `go.mod` as direct;
      `task vuln` output recorded; `pkg/acme`'s one integration-tagged test runs in the integration lane.
- [ ] 3.4 (D) `pkg/projection/contract`, `pkg/tlsutil` (level 3): rows `carry`; `pkg/tlsutil`'s integration-tagged
      test runs in the integration lane.
- [ ] 3.5 (D) `metric`, `payloadregistry` (level 4): `metric` row `carry` with its two roots triaged; `metric.Server`
      adapter lists the listener, the `http.Server` and the serve goroutine; the suite runs with the bound-port
      must-fail factory (#38's first real owner) and passes; `payloadregistry` row `adapt` (`testing.go` rehomed, task
      2.8); `prometheus/client_golang` becomes direct.
- [ ] 3.6 (D) `message`, `pkg/cache` (level 5): `message` row `carry` (`google/uuid` direct; its two tests that used
      `internal/semantictest` import the harness copy); `pkg/cache` row `adapt`: `CoalescingSet.Close(ctx) error`
      written behind a suite run that first fails on the unbounded `<-c.done` — the test, written first and shown
      to fail on the pin, plants a callback that blocks until released, the only shape under which the wait is
      unbounded — and then passes; the adapter lists the ticker and the `run` goroutine; the must-fail factory follows
      the D7 ruling
      (`NoFallibleStart()` assumed, recorded on the row); the row's `known_risks` names the two later callers
      (`processor/graph-embedding/component.go:875`, `processor/rule/entity_watcher.go:980`) as change 5 and 6
      port-refactor rows; the one `pkg/cache` root is triaged.
- [ ] 3.7 (D) `natsclient` (level 6): row `adapt`; `test_client.go` and `test_options.go` are not ported and their
      file rows are updated (`evidence` on the `adapt` row); the 69 `NewTestClient` sites are rewritten to
      `natsfixture` (count recorded: 69 before, 0 after); the nine roots are triaged; the nats.go v1.52→v1.54
      difference is recorded in `known_risks` with the carried tests as the only regression evidence; the 49 unit
      test files pass under `task test:unit` and the 29 integration-tagged files under `task test:integration --
      ./natsclient`; adapters for `Client` (lists conn, JetStream handle, subscriptions, consumer claims, health and
      metrics goroutines) and `TemporalResolver` (lists the history cache) pass the suite — `Client` with a
      refused-URL must-fail factory, `TemporalResolver` per the D7 ruling (`NoFallibleStart()` assumed); `Close(nil)`
      and `Connect(nil)` are refused behind tests written first and shown to fail on the pin (the pin's
      `Close(nil)` panics on a connected client, `client.go:686`), recorded on the row as changed behaviour
      (`adapt`) alongside every other probe finding from task 2.0.
- [ ] 3.8 (D) `task cover:check` targets `natsclient`, `message`, `payloadregistry` at 80%: the first measurement
      of each is recorded on this pull request (design P8: the pin baseline is unmeasured). If any of the three
      measures below 80%, a new task asking the owner to rule on that package's coverage is added to this file at
      that moment, written so `task spec:queue` reads it, and the gate stays unwaived (plan `:206-209`); nothing in
      this file waits on the owner until the measurement exists.
- [ ] 3.9 (D) `stretchr/testify` is a direct requirement; `task tidy:check` passes; the test-file count ported is 127
      and the line count 34,887 (recorded with `wc`).
- [ ] 3.10 (R) Port review per package group (3.1–3.7): adapters list every retained kind (the review checklist of
      the `lifecycle-suite` delta), rows validate, no file beyond the pin's was added except adapters and the
      rewritten `NewTestClient` sites. Verdict on this pull request.

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
- [ ] 5.4 (D) `scripts/cover-check.sh` reads its targets from a list that this and later changes extend, with the
      three new targets; the contract test over the script (flake-defense 4.4, if merged first; else written here)
      still passes.

## 6. Docs

- [ ] 6.1 (W) `AGENTS.md:43-44` lists exactly `scripts/verify.sh:10-11`'s steps; `task docs:check` passes.
- [ ] 6.2 (W) `docs/testing.md` (PR #39) or its successor gains: the `Restart` contract (`URL()` valid until the next
      restart; stop the owner before, start after), `FaultKV`'s before/after semantics, the helper-process pattern,
      the required must-fail factory, and the adapter checklist; written for a working developer, each coined term
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
