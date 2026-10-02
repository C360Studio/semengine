# Tasks: setup-04a-01-floor

Each task names the outcome that proves it and the gate that checks it. An unchecked task that says "Hold:" is one
`task spec:queue` reports as blocked until the named review or ruling exists. "This pull request" is PR #48. Where
there is code, the failing test is written and shown failing before the code that makes it pass. Owner of each
task: **D** developer (`semengine-developer`), **R** reviewer (`semengine-reviewer`), **W** technical writer
(`semengine-technical-writer`), **A** architect. Evidence (test output, coverage, `go list` listings) is recorded on
this pull request as a comment unless a task says otherwise. No task asserts a fact that exists only after merge.

## 1. Design and spec acceptance

- [ ] 1.1 (A) `design.md` and the six deltas under `specs/` match the accepted foundation design (D2 row 1, D3–D7,
      D9, D10.1) and the #9 rulings; every requirement has at least one scenario; `task spec:check` passes with no
      `skip_specs`.
- [x] 1.2 Independent design review (PASS on re-check 2, 2026-10-01). The reviewer's verdict on `design.md` and `specs/`
      is a pass, recorded on this pull request with the reviewed files' checksums.
- [x] 1.3 Owner acceptance of this change on #9 (2026-10-01) (the chain is accepted; this is the first change's own
      scope).
- [x] 1.4 Owner ruling on design D7 (2026-10-01, on #9). Superseded by task 1.6.
- [x] 1.5 Owner rulings of 2026-10-01 on #9 (comment 5941920346): Q1 a repaired test file makes its row `adapt`;
      Q2 `TestIntegration_Reconnection` is rewritten on `natsfixture.Restart` and proves re-dial; Q3 the three
      `test_client_*_test.go` files are not ported (`defer-exclude` with `test_client.go`) and counts are restated
      (the exclusion of `monitoring_consumers_test.go` follows from Q3 and is not part of the ruling, design D1);
      Q4 the seven doc-comment sleeps are carried as they are. Its ruling 5 is superseded by task 1.6.
- [x] 1.6 Owner ruling of 2026-10-02 on #9 (comment 5950234192): the lifecycle suite applies to services only
      (`metric.Server`, `natsclient.Client`); helpers that run background work get plain unit tests; the
      no-fallible-start exemption and its pinned list are removed; every real probe defect is a failing-first
      `adapt` item. Recorded in `design.md` D2, D3, D7.
- [x] 1.7 Owner ruling of 2026-10-02 on #9 (comment 5950482163), replacing rule 2.3 of 1.6: background work takes
      one of three shapes (`Run(ctx)`, `Close()` that joins, `Shutdown(ctx)`), with no fixed shutdown timeout, as the
      engine's standing rule. Recorded in `design.md` D7 and the `background-work` delta; tasks 3.1, 3.6, 3.7, 6.4
      follow.

## 2. Harness extension (first task, #9 item 6)

- [x] 2.0 (D) Pre-port probe: in a throwaway test against the pin snapshot (not committed), run the suite's seven
      existing checks through a minimal adapter against each of the five owners (`metric.Server`, `natsclient.Client`,
      `TemporalResolver`, `CoalescingSet`, `resource.Watcher`) and record which checks fail on this pull request;
      every failure becomes a named, `adapt` item with a test written first on the owner's row before its port task
      runs; the probe's source is attached to the comment on this pull request, not committed. Known before the
      probe: `Client.Close(nil)` panics and `Connect(nil)` is unchecked (design P13).
- [x] 2.0b (D) Pre-port repeat probe: against the pin snapshot (not committed), `go test -race -count=1 -cpu 1` and
      at least three runs of `go test -count=5 -cpu 1 -shuffle=on` over the 14 tested packages, each run's seed and
      failing tests recorded on this pull request. Recorded in PR #48 comment 5942307713 (race 14/14; four shuffle
      runs, two failing in `pkg/cache` with seeds `1790895004660367000` and `1790895074311951000`). Still to post:
      the seeds of the two green runs and the design D8 hit list (P18). The three known `pkg/cache` failures
      (`TestCoalescingSet_EntityUpdateScenario`, `TestAttack_ConcurrentAddRemove`,
      `TestCoalescingSet_ContextCancellation`; design P19) are repair items on task 3.6.
- [x] 2.1 (D) `Fixture.Restart` and `CreateMemoryStream`: an integration test, written first and shown to fail on the
      base
      (no `Restart` method), creates one file-backed stream with `CreateStream` and one memory-backed stream with
      `CreateMemoryStream`, publishes one message to each, restarts, and asserts the file-backed message is readable
      through the new `JetStream()` and the memory-backed one is not; a second test asserts `URL()` dials after the
      restart; a third asserts a consumer with a running handler is ended before `Restart` returns and no handler runs
      afterwards; a fourth asserts `Restart` before `Start` returns an error with no Docker call. Gate: `task
      test:integration -- ./internal/harness/natsfixture`. The durability premise (design P3) is proven here, not
      assumed.
      - Note: the fixture's `connect` was fixed first to wait for its dial to finish before returning (commit
        f1b046d). The pin's dial goroutine outlived the call, which the background-work rule forbids, and `Restart`
        re-runs `connect`.
- [x] 2.2 (D) Restart fault matrix: with the `stopContainer` and `startContainer` hooks made to return an error in turn,
      `Restart`
      returns a `FixtureError` naming the phase, no second container exists, and `Stop` observes the container gone;
      the sensitivity test trips exactly its phase. The one-replacement rule is unchanged (`maxAttempts` untouched).
- [x] 2.3 (D) `FaultKV`: unit tests written first show `FailAfter(Update)` returns the injected error while a fresh
      read sees the new revision and `Calls()[Update] == 1`, and `FailBefore(Create)` leaves the key absent with
      count zero; `go vet` confirms the wrapper satisfies `jetstream.KeyValue`; `go list -deps
      ./internal/harness/natsfixture` shows no package of this module outside `internal/harness/`.
- [x] 2.4 (D) `internal/harness/prochost`: tests written first show (a) the helper test is a no-op without the
      marker, (b) a started helper is in its own process group with output under the evidence directory, (c) a kill
      between two checkpoints leaves the first checkpoint's file and not the second's and `Wait` returns the killed
      status within its bound, (d) after `Pause` the process is observed stopped (`ps` state `T`, read through
      `probe.Await`) and the checkpoint count read while stopped is unchanged after the test writes the helper's
      next request, and after `Resume` the checkpoint arrives, (e) a test that returns with the helper running
      leaves no process behind, checked by start identity (`runner_test.go:255,293`) and not by pid alone; output
      goes under `SEMENGINE_EVIDENCE_DIR` when set and under `t.TempDir()` otherwise, and (b) asserts the fallback;
      no file in the package contains a sleep (`TestNoSleepsInTests` scans its non-test files). Gate: `task
      test:unit` and `task test:repeat -- ./internal/harness/prochost` (no Docker needed).
- [ ] 2.5 (D) `lifecycletest.Run(t, factory, mustFail, promise)` and `CheckFailedStartHoldsNothing`: a test written
      first shows `Run` failing before any check, naming the argument, when `mustFail` is nil. The `refowner` double
      gains a must-fail construction mode (design D8 note): Start returns its error after `o.startAttempted = true`
      (`refowner_test.go:72`); the `checks` entry for the new check is marked must-fail and every test iterating
      `checks` — `TestEachFailpointTripsExactlyItsCheck` (`:382`) and `TestChecksPassAgainstCleanDouble`
      (`:239-249`) — builds the must-fail double for it; the clean must-fail double passes the new check. A
      `failpointTable` row `startFailsButHolds` (want `FailedStartHoldsNothing`) acts only in must-fail mode, is
      caught by the new check naming the unresolved item, trips no other check, and is ended by `finalize` with no
      exemption; `TestAbortStopThenFinishJoinsWorker` (`:414-418`) runs it as inert. A must-fail factory whose Start
      succeeds makes the check fail stating so. Suite gap (`lifecycletest-panic-is-not-refusal`): a `failpointTable`
      row `nilStartPanics` (want `NilContextsRefused`), written first, passes on the base because `call` turns the
      panic into an error (`lifecycletest.go:291-297`); then `call` marks a recovered panic and no check counts it as
      a refusal. Both existing callers (`refowner_test.go:491-492`, `natsfixture/fixture_integration_test.go:519`)
      compile against the new signature. Gate: `task test:unit`; `task test:repeat -- ./internal/harness/lifecycletest`;
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

For each of the 15 packages (one task line per package below): the ledger row is written first and `task
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
      at the pin"; `resource.Watcher` takes shape 1 (design D7): `Run(ctx) error` replaces `StartBackgroundCheck` and
      `Stop`, behind tests written first — (1) `Run` inside `synctest.Test` returns `ctx.Err()` once its context is
      cancelled, with the check called on each tick and nothing left running; (2) `Run(nil)` returns an error and
      calls no check; the `cancel` and `wg` fields and the doc references to the removed methods (`watcher.go:108,
      :138-140`; `doc.go:21, :57, :66, :85, :149, :151, :159`) go with them.
- [ ] 3.2 (D) `pkg/errs`, `vocabulary` (level 1): rows `carry`; `pkg/errs`'s row names `ErrAlreadyStopped`
      (`errs.go:47`) as the one sentinel and change 3 as the SS#1218 proof's home; the two `pkg/errs` context roots
      are triaged.
- [ ] 3.3 (D) `pkg/types` (level 2): row `carry`.
- [ ] 3.4 (D) `pkg/projection/contract`, `pkg/tlsutil` (level 3): `pkg/projection/contract` row `carry`; `pkg/tlsutil`
      row `adapt`: ported without `LoadServerTLSConfigWithACME`, `LoadClientTLSConfigWithACME` and `initACMEClient`
      (`tlsutil.go:186-365`), the `context`, `time` and `pkg/acme` imports only they use (`:5, :10, :12`), and the
      ACME parts of `doc.go` (`:7, :10, :13-14, :27, :93-126, :151, :153-154, :160, :166`); no tlsutil test calls
      them (pin grep, design D1). The row's `known_risks` records the deferral to change 5 with both defects
      (design D7). `pkg/tlsutil`'s integration-tagged test runs in the integration lane.
- [ ] 3.5 (D) `metric`, `payloadregistry` (level 4): `metric` row `adapt` with its two roots triaged; `metric.Server`
      adapter lists the listener, the `http.Server` and the serve goroutine; the suite runs with the bound-port
      must-fail factory (#38's first real service). Two items (design D3), each written first:
      `metric-abort-stop-reports-context` — an in-package test starts a server, replaces `s.serveDone` with a
      buffered channel already holding a value, and calls `Stop` with an ended context, so both cases of the
      `select` at `handler.go:222-228` are ready before it runs; over 30 such servers the pin's `Stop` returns nil
      for at least one (each run is a fair coin), and after the fix every `Stop` returns the context's error;
      `metric-forced-join-without-timer` — `forcedServeJoinTimeout` (`:23, :243-249`) is removed and the forced path
      waits on `serveDone`. `payloadregistry` row `adapt` (`testing.go` rehomed, task 2.8);
      `prometheus/client_golang` becomes direct.
- [ ] 3.6 (D) `message`, `pkg/cache` (level 5): `message` row `carry` (`google/uuid` direct; its two tests that used
      `internal/semantictest` import the harness copy); `pkg/cache` row `adapt`: its 26 sleeps are repaired under
      `synctest` with its 30 `t.Parallel()` calls removed (D8 R1); `TestCoalescingSet_EntityUpdateScenario`,
      `TestAttack_ConcurrentAddRemove` and `TestCoalescingSet_ContextCancellation` are shown failing first under a
      recorded `test:repeat` seed from task 2.0b, and after the repair `task test:repeat -- ./pkg/cache` passes on
      that seed and on three further runs whose seeds are recorded; per design D7, each test written first:
      `CoalescingSet` takes shape 3 — inside `synctest.Test` it is constructed, used and shut down with nothing left
      running; `NewCoalescingSet(nil, …)` panics at the call with no goroutine started (today it panics in the
      goroutine, `coalescing_set.go:136`); `Shutdown(ctx)` returns `ctx.Err()` when its context ends while a planted
      callback blocks (the pin's `Close()` waits forever, `:116-126`), and once the callback returns the goroutine
      exits and a second `Shutdown` returns nil. The TTL and hybrid caches take shape 2 — inside `synctest.Test` each
      is constructed, used and closed with nothing left running, and `Close()` returns with no fixed wait
      (`ttl.go:258-262`, `hybrid.go:276-292`); `cache.NewTTL` and `cache.NewFromConfig` return an error on a nil
      context (today the TTL goroutine panics, `ttl.go:276`). By owner ruling (#9 comment 5950725772),
      `WithEvictionCallback`
      (`options.go:44`), `EvictCallback` (`cache.go:51-53`) and the eviction plumbing in the simple, LRU, TTL and
      hybrid caches are removed, with `TestEvictCallback` (`cache_test.go:444-503`, design D8) and the examples in
      `doc.go:39, :157, :162, :234` and `README.md:64, :117-125, :200, :410`. The row's `known_risks` names the later
      callers (design D5) as port-refactor rows; the one `pkg/cache` root is triaged.
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
      files under `task test:integration -- ./natsclient`; the `Client` adapter (lists conn, JetStream handle,
      subscriptions, consumer claims, the health monitor, the metrics poller, callback and claim-release goroutines)
      passes the suite with a refused-URL must-fail factory after three items (design D3), each written first and
      shown to fail on the pin: `natsclient-nil-context-refused` (`Close(nil)` panics at `client.go:685`;
      `Connect(nil)` dials at `:494`, panics at `:495` and leaks the connection — both refuse with an error before
      acting); `natsclient-connect-refuses-second-start` (`:547` overwrites the connection, `:566` starts a second
      poller); `natsclient-close-joins-its-goroutines`, in-package tests: (a) `handleDisconnect` (`:1496`) with a
      planted `onDisconnect` that blocks until released: the pin's `Close` returns nil while the callback runs; after
      the fix `Close` under an ended context returns `ctx.Err()`, a second `Close` with a live context does not
      return until the callback is released and then returns nil (the pin's second `Close` returns nil at once,
      `:583-585`); (b) a callback delivered after `Close` has begun (`handleClosed` and `handleDisconnect` called
      once the closing flag is set) and a connection-loss timer that fires after `Close` start nothing and are not
      counted, `go test -race` passes, and the drop is logged at debug level; (c) two concurrent `Close` calls while
      the planted callback blocks: the one whose context ends returns `ctx.Err()` while the other is still waiting,
      and the other returns nil once the callback is released; (d) `Close` with a bounded context called from inside
      `onDisconnect` returns `ctx.Err()`; the review checks that all eleven sites of design D3 go through the one
      helper). `TemporalResolver` takes shape 2 by
      delegation (design D7): constructed and closed inside `synctest.Test` with no broker and nothing left running,
      and `NewTemporalResolver(nil, …)` and `NewTemporalResolverWithCache(nil, …)` return an error, each test
      written first; its empty eviction callbacks (`kv_temporal.go:27,59`) go with `WithEvictionCallback`. Each item is
      recorded on the row as changed behaviour (`adapt`).
- [ ] 3.8 (D) `task cover:check` targets `natsclient`, `message`, `payloadregistry` at 80%: the first measurement
      of each is recorded on this pull request (design P8: the pin baseline is unmeasured). If any of the three
      measures below 80%, a new task asking the owner to rule on that package's coverage is added to this file at
      that moment, written so `task spec:queue` reads it, and the gate stays unwaived (plan `:206-209`); nothing in
      this file waits on the owner until the measurement exists.
- [ ] 3.9 (D) `stretchr/testify` is a direct requirement; `task tidy:check` passes; the ported test-file count is 121
      (127 at the pin less the four excluded files and `pkg/acme`'s two) and the line count after repair is recorded
      with `wc` next to the pin's 33,279 less the 60 lines of `TestEvictCallback` (design D8), with a diff stat
      against the pin per package.
- [ ] 3.10 (R) Port review per package group (3.1–3.7): the two service adapters list every retained kind (the review
      checklist of the `lifecycle-suite` delta); each helper has the shape design D7 gives it, its `synctest` test and
      its nil-context refusal, and no fixed shutdown timeout remains (`background-work` delta); rows validate, no
      file beyond the pin's was added except adapters and the rewritten `NewTestClient` sites, and every D8 repair is
      on its row; every real-clock timer in a repaired file is either an R1b failure bound sized per D8 or a site in
      D8's disposition table with that disposition (a review check the text check cannot make). Verdict on this pull
      request.
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
      the lifecycle suite for services with its required must-fail factory and the adapter checklist, the three
      shapes of background work with their `synctest` test, and the repair classes for a ported test (design D8);
      written for a working developer, each coined term defined at first use.
- [ ] 6.3 (W) The `openspec/specs/` sync: the six deltas applied to `harness-boundaries`, `nats-fixture`,
      `lifecycle-suite` and the three new capabilities (`process-host`, `transport-client`, `background-work`),
      verified against the code as landed; `task spec:check` passes.
- [ ] 6.4 (W) `.agents/contracts/semengine-developer.md` and `.agents/contracts/semengine-reviewer.md` gain the
      "Background work" subsection after "Context ownership", and their detach bullets the no-join-by-timer clause,
      as drafted on this pull request; `task docs:check` passes.

## 7. Review and archive

- [ ] 7.1 Hold: independent change review. The reviewer's verdict on the full diff (harness, 15 packages, rows,
      gates, docs) is a pass recorded on this pull request with the reviewed commit; a critical-stage read applies
      because this change adds new exported harness surface and changes `lifecycletest.Run`.
- [ ] 7.2 (D) `task verify` green on the final commit; the integration lane green under the host lock, evidence
      directory attached to this pull request; `implemented-by:` in the pull request body.
- [ ] 7.3 (W) The change archived under `openspec/changes/archive/` as the last content commit before squash merge;
      `task spec:queue` shows no open hold.
