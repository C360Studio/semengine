# Tasks: setup-04a-01-floor

Each task names the outcome that proves it and the gate that checks it. An unchecked task that says "Hold:" is one
`task spec:queue` reports as blocked until the named review or ruling exists. "This pull request" is PR #48. Where
there is code, the failing test is written and shown failing before the code that makes it pass. Owner of each
task: **D** developer (`semengine-developer`), **R** reviewer (`semengine-reviewer`), **W** technical writer
(`semengine-technical-writer`), **A** architect. Evidence (test output, coverage, `go list` listings) is recorded on
this pull request as a comment unless a task says otherwise. No task asserts a fact that exists only after merge.

## 1. Design and spec acceptance

- [x] 1.1 (A) `design.md` and the eight deltas under `specs/` (six, plus `metric-registry` and `message-codec` by owner
      ruling, #9 comment 5983188211) match the accepted foundation design (D2 row 1, D3–D7,
      D9, D10.1) and the #9 rulings; every requirement has at least one scenario; `task spec:check` passes with no
      `skip_specs`.
      - Done. The architect's corrections were applied in `e1ada76` and `2f46546`. The reviewer's check of them
        (PR #48 comment 5983766238) passed the criteria above with three findings, closed in the commit that ticks
        this task.
        1.1-a: `transport-client` states F26's consumer half (a consumer on a replaced connection is stopped when
        `Close` begins and its handlers are joined; its claim is released only after they return). Its scenario
        "A consumer on a replaced connection" is mapped in D3's ownership table to
        `TestClientCloseEndsConsumerOnReplacedConnection` and
        `TestClientCloseAttributesConsumerToItsHandlesConnection`. 1.1-b: `TestRegisterOrGetRefusesSameKeyWithDifferentLabelNames`
        is the example for a held key met with other label names, which `TestPropRegisterOrGetHistory` cannot reach.
        It passes on the code as it stands. A mutant that drops label names from the descriptor comparison fails it
        ("An error is expected but got nil"). The other `RegisterOrGet` examples pass under that mutant, and so did
        one default run of the property test. The scenario is retitled "Same key, another type, help or label
        names", and D9's map names the test. 1.1-c: `proposal.md`'s Capabilities list gains `metric-registry` and
        `message-codec`, and its `transport-client` line names the four added requirements.
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
- [x] 2.5 (D) `lifecycletest.Run(t, factory, mustFail, promise)` and `CheckFailedStartHoldsNothing`: a test written
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
- [x] 2.6 (D) The fixture's own must-fail factory: `TestS1_7Restart` passes `Run` a fixture whose `deps.start` fails;
      the start-failure check passes (nothing unresolved, Stop nil, no Docker call counted). Gate: `task
      test:integration -- ./internal/harness/natsfixture`.
- [x] 2.7 (D) I8 and T-B8 in `internal/harness/contract`: a test over `go list -deps ./...` and `go.mod` fails on
      any `github.com/c360studio/semstreams` path; a tree-shape sensitivity test (like `TestImportGraphSensitivity`)
      shows the aggregator rule rejecting a package importing `Register` from two component packages and the
      SemStreams rule rejecting a planted import; both pass on the real tree. Gate: `task test:unit`.
- [x] 2.8 (D) `internal/harness/semantictest` and `internal/harness/payloadfixture` carry the pin's helpers
      byte-for-byte except package path and import rewrites and, in `payloadfixture`, the `payloadregistry.New`,
      `payloadregistry.Registry` and `payloadregistry.Registration` qualifications the move out of `package
      payloadregistry` requires (recorded as `adapt` items on the `payloadregistry` row, task 3.5); T-B1 passes
      with them in the harness and a sensitivity case shows it rejecting the same files planted outside
      `internal/harness/`; `go list -deps` of each helper lists only pure-library packages of the set, and no test
      in those packages imports the helper (design P5).
      Lands after tasks 3.3 (`pkg/types`), 3.2 (`vocabulary`) and 3.5 (`payloadregistry`): at the pin
      `internal/semantictest/fixtures.go:13-14` imports `pkg/types` and `vocabulary`, and
      `payloadregistry/testing.go:6` imports `pkg/types`. It is therefore held by task 3.0 like section 3.
      Done in 62d9f47 and 8d2a01a (`payloadfixture` package comment): `semantictest` is a `carry` row
      (`internal/semantictest` → `internal/harness/semantictest`, matched to the pin by `task ledger:check`);
      `payloadfixture/testing.go`'s qualifications are on the `payloadregistry` row by pin line.
      `TestImportGraphRejectsHarnessHelpersOutsideTheHarness` plants both files at their pin paths and in the harness:
      T-B1 refuses the first and admits the second (implementer-reported: it failed first with the helpers absent, and
      with `testing` dropped from the forbidden list it failed naming `internal/semantictest/fixtures.go`). `go list
      -deps`, implementer-reported at `62d9f47`: `semantictest` reaches `pkg/retry`, `pkg/errs`, `pkg/types`,
      `pkg/platform`, `vocabulary` (at `887f41b`, measured by the 3.10 review, the same without `pkg/platform`, which
      left when task 3.6b removed `vocabulary.EntityIRI`); `payloadfixture` those and `pkg/projection/contract`,
      `payloadregistry`; none has a `go` statement in a non-test file, and no test of any of them imports either
      helper (their `TestImports` and `XTestImports`), as P5 says of the pin.
- [x] 2.9 (R) Harness review of tasks 2.1–2.7 and 2.10: the additions against the deltas, the fault matrices'
      completeness, and the `natsfixture` import list. Verdict recorded on this pull request before any ported
      package lands. Task 2.8 is not in this review; it is reviewed with section 3's port review (task 3.10).
- [x] 2.10 (D) Public-signature contract test (owner ruling, #9 comment 5953477174; `harness-boundaries` › "Public
      signatures name no internal type"): `TestPublicSignatures` in `internal/harness/contract` loads the module's
      non-test packages with `golang.org/x/tools/go/packages` (already direct, as in `TestNoRetainedContext`,
      `context_test.go:224`; no new dependency, no ledger row) and walks with `go/types` from each exported
      identifier of every public package, modelled on the design review's `go/types` walk of design D5 (attached to
      this pull request). `TestPublicSignaturesSensitivity`, written first and shown failing against a check that
      reports nothing, plants a fixture module (`writeTree`, as `TestNoRetainedContextSensitivity` does) with one
      violation per reach: a direct result, an exported method, an embedded field (an internal type, and an
      unexported type whose exported method is promoted), an exported non-embedded struct field, an interface method
      set, a type argument, a generic constraint on a function and on a type, an alias, an exported variable and
      constant, an exported function returning an unexported type whose exported method names the internal type,
      and an internal package nested below a public one. Each is reported naming the identifier and the internal
      type; a mutation that skips unexported named types fails the unexported-type plant; a clean package that uses
      the internal type only in unexported identifiers and bodies reports nothing. The real
      tree passes; it has no public package until section 3, whose `task verify` runs then hold each ported package
      to it. Gate: `task test:unit`. Not gated by #52 (task 3.0).
- [x] 2.11 (D) No bare select (owner-approved on this pull request; `harness-boundaries` › "No bare select"):
      `TestNoBareSelect` in `internal/harness/contract/testtext_test.go` parses every Go file in the module, test
      and non-test, `package main` included, and fails on a `select` with no cases, naming the file and line; a file
      that does not parse fails it. `TestNoBareSelectSensitivity`, written first and shown failing against a check
      that reports nothing, plants a bare `select` in a test file, a non-test file and a `main`, in four spellings
      (no space, spaces inside, across lines, a comment inside) and inside a goroutine, and shows a `select` with
      cases and mentions in comments and strings passing. Reverting the prochost helper to a bare `select` fails
      the real-tree test. Gate: `task test:unit`.

## 3. Port mechanics, per package in design D1's order

For each of the 15 packages (one task line per package below, named by its pin path): the ledger row is written
first and `task ledger:check` passes; the package and its `_test.go` files are copied from the pin at
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128` to the row's `destination` (design D5: eleven keep their pin paths, four
move from `pkg/<name>` to `internal/<name>`), with every pin import path rewritten to its destination and the design
D8 repairs applied, each repair written so it fails first where the pin's test fails (P19) and listed on the row's
`proving_tests` as pin `file:line` → SemEngine `file:line`; a row with any repaired test file is `adapt` (task 1.5, Q1);
`task verify` passes; `task test:repeat -- ./<destination>` passes; the carried tests pass in their lane; the row's
`proving_tests` names them; context roots in the package are triaged in `known_risks`.

- [x] 3.0 Hold: section 3 waits for #52 (PR #54) to merge, per the owner's placement on #52 ("Scope and placement",
      2026-10-02). Lifted: #54 merged as `39badc4` (2026-10-02). Section 2 is not held, except task 2.8, which needs
      section 3's packages. The #52 command's output, each package's difference from the pin read through its
      row's `source_path` → `destination` (design D5), is task 7.1's review input.

- [x] 3.1 (D) `pkg/platform`, `pkg/resource`, `pkg/retry`, `pkg/security`, `pkg/timestamp` (level 0): rows `carry`
      except `pkg/resource` (`adapt`, SS#1415-class ender and D8 repairs) and `pkg/retry` (`adapt`, D8 repair:
      `retry_test.go:68`); `pkg/resource`'s six sleeps (`watcher_test.go:151,208,324,333,335,363`) are repaired
      under `synctest` (D8 R1); `pkg/platform` and `pkg/security` rows record "no tests
      at the pin"; `resource.Watcher` takes shape 1 (design D7): `Run(ctx) error` replaces `StartBackgroundCheck` and
      `Stop`, behind tests written first — (1) `Run` inside `synctest.Test` returns `ctx.Err()` once its context is
      cancelled, with the check called on each tick and nothing left running; (2) `Run(nil)` returns an error and
      calls no check; the `cancel` and `wg` fields and the doc references to the removed methods (`watcher.go:108,
      :138-140`; `doc.go:21, :57, :66, :85, :149, :151, :159`) go with them. Done in 084935e; evidence PR #48
      comment 5954727965.
- [x] 3.2 (D) `pkg/errs`, `vocabulary` (level 1): rows `carry`; `pkg/errs`'s row names `ErrAlreadyStopped`
      (`errs.go:47`) as the one sentinel and change 3 as the SS#1218 proof's home; the two `pkg/errs` context roots
      are triaged. Done in 4555515; evidence PR #48 comment 5954727965. Corrected in 358a01e: `vocabulary`'s row is
      `adapt`, because its `README.md` is ported with markdownlint fixes (owner ruling, #9 comment 5955265930, since
      replaced by 5957221949, which still allows them), and `task ledger:check` does not compare READMEs; `pkg/errs`
      stays `carry`.
- [x] 3.3 (D) `pkg/types` (level 2): row `carry`. Done in dcdf6dd; evidence PR #48 comment 5954727965. Corrected in
      358a01e: the row is `adapt`, because its `README.md` is ported with markdownlint fixes (owner ruling, #9
      comment 5955265930, since replaced by 5957221949, which still allows them).
- [x] 3.3a (W) The identity documents ride with `pkg/types` (#72 ruling D, comment 5969505488; relayed in PR #48
      comments 5969296459 §2 and 5969508639 §2), ported from the pin at
      `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, each with an `adapt` ledger row naming its corrections and a port
      note naming what SemEngine has not yet ported: `docs/adr/102-entity-id-segment-semantics.md` (lint fixes
      only), `docs/adr/104-unique-platform-authority.md`, `docs/concepts/16-federation.md` and the pin's
      `openspec/specs/entity-id-contract/spec.md` (`:490` keeps `DeploymentPrefix` as `MUST export`). The spec is
      ported to `docs/specs/entity-id-contract.md` as a reference contract, not to `openspec/specs/`, which holds
      only current truth verified against code: most of its requirements describe packages not yet ported, and each
      moves into an `openspec/specs/` capability when its code is (#72 ruling of 2026-10-03, comment
      5970020197). Its opening note says so; its lint fixes and the path notes in ADR-102, ADR-104 and
      `16-federation.md` (pin `:26`) are on their ledger rows. Corrections,
      each cited in the document to the pin line it supersedes: ADR-104 decision 7 (`:94-100`) kept as history and
      marked superseded (SemStreams #1188; the ADR's own `:127-128`; pin
      `openspec/specs/component-runtime-config/spec.md:369-371`), with its restatements at `:115-118`, `:123` and
      `:145`; `16-federation.md:56-58` rewritten to ADR-104 decisions 1 and 4 (the minted suffix) and `:60-64` to
      the pin's authority gate (`processor/graph-ingest/authority_gate.go:51-52` and its ten call sites); "SemStreams"
      to "SemEngine" at `16-federation.md:3, :190, :194` and `spec.md:211`; the `16-federation.md:190` link to an
      unported concept page reworded; `pkg/platform/platform.go:1-4` (the package comment named `message` and
      `vocabulary` as readers) now names `config` and positions 1-2 of the ID, an `adapt` item on the `pkg/platform`
      row beside `:27-28`. `docs/repository-map.md` lists `docs/adr/`, the concept page and `docs/specs/`. Gates:
      `task docs:check`, `task spec:check`, `task ledger:check`, `task verify`.
- [x] 3.4 (D) `pkg/projection/contract`, `pkg/tlsutil` (level 3): `pkg/projection/contract` row `carry`, destination
      `pkg/projection/contract` (public: SemConnect imports it; #9 comment 5953295358); `pkg/tlsutil` row `adapt`,
      destination `internal/tlsutil` (design D5; #9 comment 5952661571): ported without
      `LoadServerTLSConfigWithACME`, `LoadClientTLSConfigWithACME` and `initACMEClient` (`tlsutil.go:186-365`), the
      `context`, `time` and `pkg/acme` imports only they use (`:5, :10, :12`), and the ACME parts of `doc.go` (the
      ACME clauses of `:7` and `:10`; `:13-14, :27, :93-126, :151-154, :160, :166`, where `:152` is the blank line
      between the two ACME error-handling parts); no tlsutil test calls them (pin grep, design D1). The row's
      `known_risks` records the deferral to change 5 with both defects (design D7). Pin `pkg/security/doc.go:138`
      links `internal/tlsutil` (the `pkg/security` carry check expects it once the `pkg/tlsutil` row maps the move).
      Neither package has a `README.md` at the pin. `internal/tlsutil`'s integration-tagged test runs in the
      integration lane. Done in 0031f26 and f1cc210; `task verify` ok on f1cc210.
- [x] 3.5 (D) `metric`, `payloadregistry` (level 4), both at their pin paths (public: SemSource imports both; design
      D5, #9 comment 5953295358): `metric` row `adapt` with its two roots triaged; `metric.Server`
      adapter lists the listener, the `http.Server`, the serve goroutine and, since aa94acf, the requests its handler
      admitted and has not returned from (`Stop` waits for them within its context). Since a19f393 (Codex review,
      PR #48 comment 5959412053, finding 1), `Stop` closes admission before `Shutdown`: a request that reaches the
      handler afterwards gets 503 with `Connection: close` and its handler never runs, and the closed flag and the
      running count share one mutex, so a nil `Stop` is final (`metric/admission_test.go`); the suite runs with the bound-port
      must-fail factory (#38's first real service). Two items (design D3), each written first:
      `metric-abort-stop-reports-context` — an in-package test starts a server, replaces `s.serveDone` with a
      buffered channel already holding a value, and calls `Stop` with an ended context, so both cases of the
      `select` at `handler.go:222-228` are ready before it runs; over 30 such servers the pin's `Stop` returns nil
      for at least one (each run is a fair coin), and after the fix every `Stop` returns the context's error;
      `metric-forced-join-without-timer` — `forcedServeJoinTimeout` (`:23, :243-249`) is removed and the forced path
      waits on `serveDone`. `payloadregistry` row `adapt` (`testing.go` rehomed, task 2.8; `adapt` items for its
      unqualified `New`, `Registry` and `Registration` written as `payloadregistry.X` in `payloadfixture`);
      `prometheus/client_golang` becomes direct. Done in ea46a68 (`payloadregistry`, without `testing.go`, which no
      package test uses; the row records the rehome and gains the `payloadfixture` qualification items when task
      2.8 lands) and 41e1d86 (`metric`; at the pin 13, 12, 11 and 16 of 30 `Stop`s returned nil, and the forced join
      gave up on its timer; both green after the fix; `README.md` lint fixes recorded on the row); `task verify` ok
      on 41e1d86.
- [x] 3.5a (D) `metric` registration, design D9 (Codex review finding 4, PR #48 comment 5956732582; owner-accepted
      2026-10-02): `RegisterOrGet[C]` replaces `MetricsRegistrar`, `RegisterOrGetGaugeVec` and the six `Register*`
      methods; tests 1-8 of D9 written first, each with the oracle D9 names (test 4 asserts errors, zero returns and
      the key map, not gathered values), plus D9's generated check `TestPropRegisterOrGetHistory`; the `metric` row
      records the contract sentence, the adapt items (`registry.go:16-25, :27-60, :127-278, :246`; `doc.go:14,
      :75-97, :111, :125, :136, :196-209, :236-253, :372-377`, the design's `:245` widened to the error list beside
      it; the README registration examples under #9 comment 5957221949), the tests replaced, and the consumer
      impact. Done in e78d0ce. Failing first, implementer-reported, at the pin's methods behind the new signature:
      every same-key case gathered 1 where 2 was written; the type, help, nil-`Counter`, alias and direct-collision
      cases were accepted; the typed-nil `*GaugeVec` and the core collision were already refused at the pin (test 6
      failed only on the returned candidate). The generated check and the corrected evidence landed with the third
      review's fixes (PR #48 comment 5959412053, findings 2 and 4). `task verify` ok on e78d0ce.
- [x] 3.6 (D) `message`, `pkg/cache` (level 5), destinations by design D5 (#9 comment 5953295358, refining
      5952661571): `message` stays public at `message` (SemSource imports it), `pkg/cache` moves to `internal/cache`.
      `message` row `adapt`, not `carry`: its `README.md` needs a lint fix at the pin (line 7, MD013; #9 comment
      5957221949) (`google/uuid` direct; its two tests that used
      `internal/semantictest` import the harness copy); `pkg/cache` row `adapt`: its 26 sleeps are repaired under
      `synctest` with its 30 `t.Parallel()` calls removed (D8 R1); `TestCoalescingSet_EntityUpdateScenario`,
      `TestAttack_ConcurrentAddRemove` and `TestCoalescingSet_ContextCancellation` are shown failing first under a
      recorded `test:repeat` seed from task 2.0b, and after the repair `task test:repeat -- ./internal/cache` passes on
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
      callers (design D5) as port-refactor rows; the one `pkg/cache` root is triaged. `pkg/cache` adapt (design D9):
      `metrics.go:69-84` registers its six collectors through `RegisterOrGet` and keeps the returned collectors; a
      test with two caches under one prefix on one registry asserts the gathered sum.
      - `pkg/cache` done in 70fe194 (port, `RegisterOrGet`), b137ab3 (eviction callback removed), 5fc2b63 (sleep
        repair), c6d7108 (shapes 2 and 3), 420063f (generated check), ed40a15 (close-ordering example), e7cd3ae (ledger,
        design and task record) and the lint fix after it. Two of the 26 sleeps went with `TestEvictCallback`; the other
        24 are repaired, each on the row. The generated-check decision is in design D7. Failing first,
        implementer-reported, on a copy of the pin (PR #48): under seed `1790895004660367000`, 48 of 48 runs of
        `-count=5 -cpu 1` with 24 in parallel failed `TestCoalescingSet_EntityUpdateScenario`, and 11 of them hung in
        `BatchCleared`'s deferred `Close`; under seed `1790895074311951000`, 6 of 48 failed
        `TestAttack_ConcurrentAddRemove` (and 9 `TestCoalescingSet_CallbackFiresAfterWindow`).
        `TestCoalescingSet_ContextCancellation` did not fail in those 96 runs nor in 2,400 runs of it alone at 12 in
        parallel: not reproduced. After the repair, `task test:repeat -- ./internal/cache` passed on both seeds and on
        `1790972252212655000`, `1790972253482840000` and `1790972254702897000`, and 96 of 96 runs of the seeds at 24 in
        parallel passed.
      - Every `internal/cache` constructor that returns `Cache` with an error (`NewSimple`, `NewLRU`, `NewTTL`, the
        hybrid constructor, and `NewFromConfig` through them) returns a nil `Cache` on error, not a nil pointer
        inside a non-nil interface (`TestCacheConstructorsReturnNilCacheOnError`). Implementer-reported: it failed
        first for `NewSimple`, `NewLRU` and `NewFromConfig`'s simple and lru paths; the TTL and hybrid ones were
        fixed in c6d7108.
      - Also 479ce24 and a7dbf9d (`NewSimple` and `NewLRU` return a nil `Cache` on error, above).
      - `message` done in c08b5c7 and 8d2a01a (gofmt of a rewritten import), after task 2.8 (62d9f47): its `.go` files
        match the pin apart from import paths (`task ledger:diff -- message`: 25 files, 0 differ); its two tests that
        used `internal/semantictest` import `internal/harness/semantictest`; `google/uuid` v1.6.0, the pin's version, is
        direct; the row is `adapt` for the README line 7 wrap.
- [x] 3.6a (D) Codex's review of the cache half (PR #48 comment 5968489295, at `a7dbf9d`) and the owner rulings of
      2026-10-03 (#9 comment 5968830525: an admitted package is ported whole, and only dead surface is removed; #9
      comment 5968665464: ported docs name SemEngine paths). Each finding with a test written first, failing first
      implementer-reported: F1 (18424c3) a `CoalescingSet` callback panic ends the process (superseded below) and
      a nil callback panics at the call (an error since task 3.13); `prochost` gains `Process.StderrPath` with a
      `process-host` scenario (cut below); the pin's `CallbackPanic` and `NilCallback` tests are replaced, and
      their three sleeps with them: of the pin's 26 sleeps, 21 remain, repaired (two went with
      `TestEvictCallback`). F2 (5647994) concurrent `Close` of the TTL and hybrid caches. F3 (68f10f9) the direct
      constructors refuse invalid dimensions; `NewTemporalResolverWithCache`, its one `natsclient` caller, is
      dropped with `TemporalResolver` (task 3.7). F4 (2e2060a) `StatsInterval` removed, with the dead surface the
      pin-wide search found (f56fcf6: `Entry`, `IsExpired`, `Touch`, `WithStats`, `StatsFromContext`,
      `ContextKeyStats`, `Statistics.MemoryUsage`); the one outside reference, `processor/rule/config.go:245` and
      its docs, is a port-refactor item on the row. F5 (65d9740, d62fdd9) unknown keys refused;
      `FuzzConfigUnmarshalJSON`, whose `null` seed found a pin panic, now fixed; the generated-check decision is
      in design D7. F6 (4fcfc46) `TestAttack_CallbackLatency` fails and ends when an `Add` blocks. F7 (58ffe7b)
      `doc.go` and `README.md` name `./internal/cache` and drop `cache.NewHybrid`, which does not exist; the
      `message` hold no longer applies (task 3.6 is done) and is gone. README import paths (3485af3) in `message`,
      `pkg/retry`, `pkg/types` and `vocabulary`, each an `adapt` item on its row. Rows: `pkg/cache` (contract,
      `known_risks`, `proving_tests`), `message`, `pkg/retry`, `pkg/types`, `vocabulary`.
      - F1's process-ending panic is superseded by owner ruling 5994720412 item 3: the panic is recovered, logged and
        counted, the batch dropped and later batches fired
        (`TestCoalescingSet_CallbackPanicIsRecoveredAndLaterBatchesFire`; the `internal/cache` row).
      - `Process.StderrPath` and its `process-host` scenario "A test reads what the helper wrote to stderr" are cut as
        dead surface: their one consumer, the process-ending `CoalescingSet` test, went with item 3 (reviewer early
        check at `5ecc016`, MEDIUM). `prochost`'s own test reads the stderr file in-package.
- [x] 3.6b (D) Codex's sixth record (PR #48 comment 5969256728, at `c64ac33`: F8–F10), #72 ruling 1 (comment 5969293525)
      as extended to `vocabulary.EntityIRI` (comment 5969505488, relayed in PR #48 comment 5969508639) with ruling D's
      `pkg/platform` comment, and #9 ruling 7 (comment 5969522395). Each with a test written first, failing first
      implementer-reported. Federation family removed (b9e216c): `TestMetaCarriesOnlyWhatTheWireCarries` failed first
      naming `DefaultFederationMeta` and `FederationMeta`. F9 and F10 (0aefed9): the `message` decode examples call
      `NewDecoder(reg).Decode`; the `pkg/cache` row drops the recovered-panic claim. `EntityIRI` removed (bb004ef) with
      its doc example and test lines; `TestNoDeploymentAuthorityNames` (`internal/harness/contract`) fails on an
      exported name matching `Federation|GlobalID|EntityIRI` in any production package and failed first naming
      `vocabulary.EntityIRI`; its sensitivity test failed against a check that reports nothing, and a mutation skipping
      methods fails it. Nothing in this change imports `pkg/platform` now; its reader is `config` (change 3), so it
      stays public; its row is `adapt` for `platform.go:27-28`. Ruling 7 (5e31950): `BaseMessage` decodes both
      timestamps strictly as integer milliseconds and refuses any other form; `TestBaseMessageTimestampsAreMilliseconds`
      failed first (1999-01-01 decoded as year 30969, 1960-01-01 as -8032) and
      `TestBaseMessageRefusesTimestampsThatAreNotMilliseconds` failed first (all eight forms accepted). The owner's
      rulings of 2026-10-03 (#9 comment 5969776736): item 1 confirms that refusal, the RFC 3339 and number-in-a-string
      forms the pin accepted included, with a missing value, `null` or 0 giving the zero time, which 5e31950 already
      does (no further change); item 2, `BaseMessage` encoding refuses a `source` that is not valid UTF-8 instead of
      writing U+FFFD in its place (1ee70d0): `TestBaseMessageRefusesSourceThatIsNotUTF8` failed first (all eight
      marshals succeeded), and `FuzzDecoderRoundTrip` drops its U+FFFD exception, asserting refusal for an invalid
      source and full equality otherwise. F8 (dbe98c5, seeds as of 1ee70d0): `FuzzDecoderDecode` (25 seeds, 8 accepted),
      `FuzzDecoderRoundTrip` (9, one added in 1ee70d0: a valid source holding U+FFFD itself) and
      `FuzzGenericJSONPayloadUnmarshalJSON` (13), with no timestamp carve-out; the generated-check decision is in design
      D7. Mutants, implementer-reported, each detected on seed replay: decoder always refuses, decoder drops `source`,
      the pin's `timestamp.Parse` heuristic restored. Exploration, separate from replay, implementer-reported (logs
      local only): `go test -run '^$' -fuzz '^<target>$' -fuzztime 60s ./message` ran 1,970,895, 1,121,935 and
      10,033,878 executions with no failing input, all run before 1ee70d0. Rows: `vocabulary`, `pkg/platform`,
      `message`, `internal/semantictest`. The owner's ruling of 2026-10-03 (#9 comment 5970334875, extending ruling
      2; Codex F12, comment 5970321028): invalid UTF-8 is refused wherever `message` encodes a string. `Type.Validate`
      refuses a component that is not valid UTF-8 (`TestTypeValidateRefusesInvalidUTF8` failed first: all four
      accepted); `BaseMessage.MarshalJSON` refuses such a type, and `GenericJSONPayload.MarshalJSON` such a string, key
      or value at any depth of `Data` (`TestBaseMessageRefusesTypeThatIsNotUTF8` and
      `TestGenericJSONRefusesStringsThatAreNotUTF8` failed first: 3 and 15 marshals succeeded with U+FFFD on the wire;
      Codex's `TestReviewerRetainedUTF8Loss` probe, run unchanged and not committed, observed both losses before the fix
      and fails on the refusal after). `FuzzDecoderStrings` (10 seeds, an invalid byte in each of the five positions)
      generates the three type components and a generic key and value, asserting refusal or full equality through
      `NewDecoder`. Mutants on the final code, implementer-reported: 14, of which 13 detected (the cycle guard's by the
      test process being killed after unbounded recursion, the rest by named assertions) and 1 survived: removing the
      `[]byte` skip, equivalent because byte elements hold no string. A reflect `CanInterface` guard whose mutant
      survived was removed: its only reachable input makes `encoding/json` panic too. Exploration, separate from replay,
      implementer-reported (log local only): `-fuzz '^FuzzDecoderStrings$' -fuzztime 30s` ran 320,542 executions with no
      failing input on the final code (2,453,552 on an earlier revision of the walk). Rows: `pkg/types`, `message`.
      The owner's rulings of 2026-10-03 (#9 comments 5972117486 and 5972208367; Codex F15 and F16, PR #48 comment
      5970889953): `GenericJSONPayload.Data` holds JSON-shaped values only, and `time.Time` is refused. Measured first
      (tarballs): 28 `NewGenericJSON` calls at the pin, 6 in production, one storing `time.Time`
      (`processor/rule/message_handler.go:376-384`, left to the `rule` port); semboids 2, JSON-shaped; semsource,
      semconnect and semteams none. The walk is replaced by a check over the ruled kinds by exact type, run before
      encoding. `TestGenericJSONRefusesValuesThatAreNotJSONShaped` failed first: all 23 subtests (17 accepted, 4
      refused by `encoding/json` without an invalid-data error, the two F16 cases refused at a field path);
      `TestGenericJSONRefusesInvalidUTF8AtDepth` failed first on the path in all six; `TestGenericJSONRefusesCycles`
      failed first (a self-referencing map gave `encoding/json`'s cycle error, not invalid data; the F16
      self-embedding case alone overflowed the stack); `TestGenericJSONAcceptsJSONShapedValues` is a control and
      passed before and after. `FuzzGenericJSONShapes` (30 seeds: one per generator kind, invalid UTF-8 in a key and
      at depth, refused kinds at depth, a NaN, empty containers; on replay 19 accepted, 10 refused as invalid data, 1
      refused for the NaN). Mutants on the final code, each detected: no refusal of another kind (shape test, cycle
      test, fuzz replay), no key UTF-8 check and no string UTF-8 check (UTF-8 test, both fuzz targets), no cycle
      guard (the cycle test's child overflowed its stack). Exploration, separate from replay (log local only):
      `-fuzz '^FuzzGenericJSONShapes$' -fuzztime 30s` ran 3,401,171 executions with no failing input. Row: `message`.
- [x] 3.7 (D) `natsclient` (level 6), port and unit lane: row `adapt`, its `source_sha` and the existing file rows'
      at the pin. Not ported, with file rows: `test_client.go` (`adapt → natsfixture`, evidence in `proving_tests`)
      and `test_options.go` (`defer-exclude`); six test files `defer-exclude` with design D1's reasons —
      `test_client_factory_test.go`, `test_client_integration_test.go`, `test_client_readiness_test.go` (owner
      ruling, task 1.5, Q3), and, forced by Q3 and not owner rulings, `monitoring_consumers_test.go` (`WithMonitoring`,
      `test_client.go:448`), `test_options_test.go` (tests `test_options.go`) and `mapped_port_retry_test.go` (tests
      `test_client.go` internals). Not ported as dropped surface, recorded on the package row (design D8, "A test of
      a removed feature"): `kv_temporal.go` with `kv_temporal_integration_test.go` and
      `TestTemporalResolver_ErrorBoundaries` (`kv_error_integration_test.go:353-450`) (owner ruling, PR #48 comment
      5969522395, item 1); `typed.go` with `typed_test.go` (item 2). This commit compiles and `task verify` passes on
      it: the production files with imports rewritten; `jetstream_metrics.go:128-161` registers all 11 collectors
      through `RegisterOrGet` and keeps the returned collectors (design D9; the pin's `Register*` calls no longer
      exist, task 3.5a); `github.com/nats-io/nats-server/v2` becomes a direct test requirement at v2.14.7, the
      `.nats-image` line (the pin requires v2.12.4, `go.mod:11`), for the embedded broker at
      `client_connect_test.go:104`, recorded in the row's `known_risks` as a pin difference with the carried tests as
      the only regression evidence (T-B3 cannot see an embedded server, so one broker version across both lanes is
      review only); and the 43 unit test files, of which: the census tests in `consumer_policy_callsite_test.go`
      (`TestConsumerPolicyProductionCallsiteCensus` `:217`, `TestConsumerPolicyDirectCreationCallCensus` `:588`) are
      rewritten for SemEngine as an `adapt` item (item 3) — their expected maps hold what SemEngine's tree has at this
      commit, measured and not copied, a caller planted in a `t.TempDir()` tree is shown to fail each, and the row's
      `known_risks` says each later port that adds a caller updates the maps; the 8 sleeps in `client_test.go` are
      repaired under design D8 R1a; the 47 fixed-address lines in 8 unit files are repaired (D8 R5;
      `stream_visibility_test.go:82` is an error string the guard does not match and stays); `testStreamMaxAge` and
      `testStreamMaxBytes` (`test_client.go:912-913`, read at `client_test.go:373-374`) are defined in the one test-side
      helper file that replaces `test_client.go`. `task test:unit`, `task test:repeat -- ./natsclient` and
      `task tidy:check` pass. The nats.go v1.52→v1.54 difference is in `known_risks` with the carried tests as the only
      regression evidence. Context roots: the pin's two live production roots are `client.go:566` (the metrics poller,
      triaged in 3.7c) and `trace.go:56` (leaves with `DetachContextWithTrace`, 3.7a); the rest of the "nine" are
      comments or `test_client.go`.
      - Done in 9cda760 (port: 25 production files, 10,986 lines, and the 43 unit files, imports rewritten;
        `RegisterOrGet` for the 11 collectors; `nats-server/v2` v2.14.7 direct; ledger rows), 369d21e (census),
        e5a5f0e (D8 repairs) and eb03773. The work is split into commits for review: 9cda760 compiles and passes
        `go vet`, and its census tests, sleep guard, fixed-address guard and `cleanup-roots:check` fail until
        369d21e and e5a5f0e. `task verify` passes at eb03773 (implementer-reported, PR #48), with
        `task test:repeat -- ./natsclient` and `task tidy:check`. Census maps measured on SemEngine's tree: no
        caller of the four entry points; direct creations `natsclient/stream.go` ×2 and
        `internal/harness/natsfixture/fixture.go` ×1. The two planted-caller tests name `planted.go` in all seven
        subtests, and all seven fail when the checks return nil. Sleeps 8 → 0 (R1a, `synctest`); fixed-address
        lines 47 → 0 (`"nats://unused"` for clients that never dial, `refusedNATSURL` for the one that dials).
        Also repaired, though the list above does not name it: three `Close(context.Background())` calls that
        `cleanup-roots:check` rejects (`client_connect_test.go:70, :98`, `client_test.go:700`) now run under a
        10 s `WithTimeout`. `task ledger:diff -- natsclient`: 68 files compared, 12 differ, 39 only at the pin
        (4 production files; 5 excluded unit files and `typed_test.go`; 29 integration-tagged files: task
        3.7b's 27, `test_client_integration_test.go` and `kv_temporal_integration_test.go`), 1 only in the tree
        (`test_helpers_test.go`). `doc.go:512`'s import path is the one `doc.go` difference (task 3.7e).
- [x] 3.7a (D) `natsclient` surface audit (#9 comment 5968830525; owner rulings, #9 comment 5969522395, items 1–2),
      each drop an `adapt` item on the row: `options.go` `WithPingInterval`, `WithRequestHandlerTimeout`,
      `WithDisconnectCallback`, `WithReconnectCallback`, `WithHealthChangeCallback`, `WithCircuitBreakerThreshold`,
      `WithMaxBackoff`, `WithToken`, `WithTLS`, `WithDrainTimeout`, `WithCompression`; `client.go` `OnHealthChange`,
      `WithHealthCheck`, `ErrConnectionTimeout`, `ConnectionOptions`, `PublishToStreamAsync`,
      `PublishToStreamAsyncWithMsgID`, `PublishAsyncComplete`, `PublishAsyncPending`, `MaxReconnects()`,
      `ReconnectWait()`, `PingInterval()`; `request.go` `RequestReady`; `errors.go` `ReplyError`; `DeliveryResult`'s
      `ControlError`, `SettlementError`, `SettlementAttempted`, `SettlementMethodSucceeded`;
      `StorageResource.Undescribable`; `trace.go` `DetachContextWithTrace` with its root at `:56`.
      `PublishBatchToStream` stays (semboids `boidgraph/publisher.go:32`). State that the drops leave written by
      nothing goes with them, so no branch remains that no input reaches: the `token`, `tlsCertFile`, `tlsKeyFile`,
      `tlsCAFile`, `tlsEnabled`, `compression`, `onDisconnect`, `onReconnect` and `onHealthChange` fields, and exactly
      these lines — `client.go:423-425` (token option), `:427-435` (TLS options), `:442-445` (compression), `:569-572`
      (health change on connect), `:615` (token clear), `:1500-1510` (disconnect and health callbacks), `:1520-1530`
      (reconnect and health callbacks), `:1585-1591` (health callback on close), `:1658-1661` (the monitor's health
      callback). Kept: the `clientName` option (`:437-440`), `armConnectionLossTimer` (`:1512`), the reconnect path's
      status, circuit reset and timer cancel (`:1516-1518`), and each handler's `setStatus`. `publishToStreamAsync`'s
      `msgID` parameter and its stamp go too (only `PublishToStreamAsyncWithMsgID` passed one; `PublishBatchToStream`
      passes "" at `:1177`). Fields still read by behaviour keep their defaults with no setter (`pingInterval`,
      `drainTimeout`, `circuitThreshold` 15, `maxBackoff` one minute; `requestHandlerTimeout` from the environment
      variable, `request.go:37-55`, whose name is unchanged here, #69). Audit item (c): `testCircuit`'s comment
      (`client.go:352-358`) promises a reconnect; the function only moves the status from open to disconnected, and
      the comment is corrected to say so. Tests, in the unit files here and in the integration files as 3.7b lands
      them: a test that reaches kept behaviour through a dropped symbol is retargeted, not deleted — it calls the
      unexported path the symbol wrapped (`publishToStreamAsync`, as `PublishBatchToStream` does at `:1177`;
      `requestMsgReady`, as `RequestReadyClassified` does at `errors.go:318`), reads the fields a dropped getter
      returned (`DeliveryResult`'s `controlErr`, `settlementErr`, `settlementTried`; `StorageResource`'s three unknown
      states; the client's reconnect and ping fields), uses the JetStream handle a dropped accessor forwarded, or sets
      the request-handler timeout through the environment variable; a test whose only subject is dropped behaviour is
      deleted (`TestConnectionOptions`, `client_test.go:393`; `TestRequestHandlerTimeout_Option` and `_OptionBeatsEnv`;
      `TestReplyError_*`, `errors_test.go:211, :221`; `TestPublishAsyncComplete_JetStreamUnavailable`;
      `TestIntegration_PublishToStreamAsyncWithMsgID_Dedup` and the message-ID half of `_StampsTraceAndMsgID`; the
      typed-subject cases at `subscription_integration_test.go:54, :62`; `TestIntegration_HealthMonitoring`,
      `integration_test.go:236-285`, which retires design D8's rows `:262` and `:278`). The row lists each as pin
      `file:line` → SemEngine `file:line` or "deleted".
      - Done in d6f6d18, unit lane (implementer-reported, PR #48): the 30 listed symbols, the nine fields and the
        listed lines dropped, with `lastHealthy` (`client.go:1630, :1663`), which only the monitor's health callback
        read and the compiler refuses once nothing reads it. Retargeted: `TestPublishToStreamAsync_NotConnected`,
        `_CancelledContext`, `_CircuitOpen` (`publishToStreamAsync`); the 18 `DeliveryResult` getter calls in
        `delivery_settlement_test.go` (the fields); the 7 `Undescribable()` calls in `storage_inventory_test.go`
        (the three unknown states); `TestConnectionOptions` (the reconnect and ping fields: its subject is kept
        options, so the retarget rule wins over its place in the deletion list); and
        `TestClientHandleErrorDoesNotMutateRuntimeStateOrCallbacks`, which planted the three dropped callback
        fields and is not in the list. Deleted: `TestRequestHandlerTimeout_Option`, `_OptionBeatsEnv`,
        `TestReplyError_NilErr_NoOp`, `_EmptyReplyTo_NoOp`, `TestPublishAsyncComplete_JetStreamUnavailable`. Each
        retarget fails on a mutant of the behaviour it covers; `Err()` without `controlErr` in its join survives,
        as at the pin. `requestMsgReady` has no unit test; a scratch test over the embedded broker, not committed,
        caught two mutants. The integration-file retargets and deletions this task names carry to 3.7b, listed on
        the row. `task verify` passes. Every pin `file:line` → SemEngine `file:line` is on the row.
- [x] 3.7b (D) `natsclient` integration lane: the 27 integration-tagged files land on `natsfixture`, 3.7a's rule applied
      to their dropped-symbol uses. The 51 `NewTestClient` sites (56 at the pin less five in tests removed by 3.7 and
      3.7a) are rewritten (51 before, 0 after), with the other `test_client.go` and `test_options.go` uses:
      `TestStreamConfig` and `WithStreams`, `WithKV`, `WithJetStream`, `WithFileStorage`, `WithTestTimeout`,
      `WithNATSVersion`; `WithMinimalFeatures` (`test_options.go:55-61`; called at
      `client_close_integration_test.go:16`, `client_async_error_integration_test.go:28`,
      `subscription_integration_test.go:84`) → a plain fixture, which always runs JetStream, and the review checks that
      none of the three asserted JetStream's absence; `TestClient.Terminate()` (4 files at the pin, 3 after the drops)
      and the `.Terminate(` calls through the container wrappers (14 files, 13 after the drops; 40 of the calls through
      `integration_test.go:286-296` and 3 through `client_integration_test.go:197-200`) → `Fixture.Stop`;
      `GetNativeConnection()` (3) → a connection dialled from `URL()` in `stream_visibility` and `kv_watcher_ownership`,
      and `client.GetConnection()` (what the pin's helper returned) at `subscription_integration_test.go:98, :106`,
      which closes the connection its own subscription is on; and the testcontainers imports and `testClient.container`
      uses (`client_integration_test.go:14, :190-200`; `integration_test.go:18, :24-34, :70, :240, :286-296`).
      Afterwards no `natsclient` test imports testcontainers. `natsfixture` gains one option that sets the broker's
      `max_payload` (item 5), its consumer the four `WithTestMaxPayload` tests in
      `request_response_bounds_integration_test.go`, with a fixture test written first (a publish above the set limit is
      refused, one at or below it accepted) and the `nats-fixture` delta's requirement "Broker max payload is settable".
      The 8 fixed-address lines in 4 integration files are repaired (D8 R5). The 23 integration sleeps (29 at the pin
      less six in the removed `TemporalResolver` tests) are repaired per D8 R1, and the lane's real-clock timers are
      re-counted after the deletions and each checked against D8's table. `TestIntegration_Reconnection`
      (`integration_test.go:62`) is rewritten on `natsfixture.Restart` with no skip call (D8 R2; item 2): the loss is
      observed through `WithConnectionLostCallback` and the client's `Status` through `probe.Await`, and a client
      dialled from the new `URL()` is observed healthy; its name and comment say it proves re-dial, not nats.go's
      automatic reconnect. The two tests that pin NATS 2.14.4 (`kv_key_contract_integration_test.go:38-39`,
      `kv_watcher_ownership_integration_test.go:48`) run on `.nats-image` (2.14.7) with the version override dropped and
      the log line corrected (item 6); T-B3 (`imagepin_test.go:78`) cannot see a version and digest held in separate
      constants, so this is review only. `kv_key_contract_test.go:30`'s v1.52.0 constant becomes v1.54.0 and its comment
      cites `kv.go:504-506` (the three regexes are byte-identical). `task test:integration -- ./natsclient` passes. Done
      in `edfd431`, `9637826`, `8177a64`, `ce528b4`; CI run 37138705673 green at `ce528b4`. The 27 files are ported;
      `NewTestClient` sites 51 → 0; `go list` test imports show no testcontainers. `TestMaxPayloadIsSettable` failed
      first ("broker announces max_payload 1048576, want 4096"); the option writes a config file and passes `--config`,
      since nats-server has no flag for it. Sleeps 23 → 0 (10 flushes, waits on observed events or counts, one removed
      as waiting for nothing; the old Reconnection test's 3 went with its rewrite); fixed addresses 8 → 0; 31 failure
      bounds under 10 s widened to 10 s and six hand-made polls moved to `probe.Await`. Beyond the listed scope: about
      56 deferred `Close` calls go through a bounded `closeClient`; the refusal test's 150 ms request timeout became an
      event-driven wait, so it asserts `context.Canceled` where the pin asserted a timeout, after an observer on
      `_INBOX.>` shows no reply was published (review F19); two late-responder tests raise `MaxRetries` 5 → 10; the
      `publish_msgid` test is split into a dedup proof that finishes inside a 60 s window and a separate 250 ms expiry
      check (review F18; the first repair widened the window to 2 s).
      `TestIntegration_ReconnectionIsRedial` fails on a mutant that removes the loss-timer arming. Added from the 3.7a
      survivor: `TestConsumeDeliveryWithHeartbeatErrCarriesControlLoss` fails on the mutant that drops `controlErr` from
      `Err()`'s join. Census maps unchanged. Text corrected after the fact: the `Terminate` and `GetNativeConnection`
      mappings above, and D8's new row for `client_close_integration_test.go:77`.
- [x] 3.7c (D) Lifted: Codex's checkpoint review of 3.7–3.7b passed at `f97803f` (PR #48 comment 5974824034).
      `Client` lifecycle (design D3): the test-side adapter lists the `nats.Conn`, JetStream handle, subscriptions,
      internal consumer claims, the health monitor, the metrics poller, the claim-release goroutines and the two timers,
      and the client passes the suite with a refused-URL must-fail factory after these items, each test written first
      and shown to fail on the code as 3.7b leaves it: `natsclient-nil-context-refused` (`Close(nil)` panics at
      `client.go:685`; `Connect(nil)` dials at `:494`, panics at `:495` and leaks the connection — both refuse with an
      error before acting); `natsclient-connect-refuses-second-start` (`:547` overwrites the connection, `:566` starts a
      second poller); `natsclient-close-joins-its-goroutines`, in-package tests, each registering the planted
      callback's release in `t.Cleanup` before planting it and bounding every wait: (a) a connection-loss timer armed
      by `handleDisconnect` (`:1512`) fires a planted `onConnectionLost` that blocks until released: the pin's `Close`
      returns nil while it runs; after the fix `Close` under an ended context returns `ctx.Err()`, and a second `Close`
      with a live context does not return until the callback is released and then returns nil (the pin's second
      `Close` returns nil at once, `:583-585`); (b) a disconnect delivered once `Close` has begun and a loss timer that
      fires after it start nothing and are not counted, `go test -race` passes, and the drop is logged at debug level;
      (c) two concurrent `Close` calls while the planted callback blocks: the one whose context ends returns
      `ctx.Err()` while the other still waits, and the other returns nil once the callback is released; (d) `Close`
      with a bounded context called from inside `onConnectionLost` returns `ctx.Err()`; (e) the Close/Connect race,
      made deterministic: `Connect` is given, through `WithLogger`, a handler that blocks on the "Successfully
      connected to NATS" record (`:555`) until released, which holds `Connect` open after it releases `closeMu`
      (`:553`) and before it starts the monitor (`:560`) and the poller (`:566`); `Close` runs and returns while
      `Connect` is held, the handler is released, and once both have returned neither the monitor nor the poller runs
      and the adapter lists nothing (the pin starts both, and writes `metricsCancel` with no lock that `Close` reads at
      `:595`). The review checks that all six sites of design D3 go through the one helper. `jetstream.New`'s error
      (`:525`, dropped at the pin) is returned by `Connect` with the dialled connection closed; no failing-first test
      exists, because nats.go v1.54.0 `jetstream.New` fails only when an option does and
      `WithPublishAsyncErrHandler` never does (`jetstream/jetstream.go:471-492`, `jetstream_options.go:41-46`), and the
      row says so. The root at `client.go:566` is triaged on the row: `Close` cancels the poller's in-flight work and
      joins it. Each item is changed behaviour on the row.
      - Done in `1be7abb`, losing-Connect fix in `eaf95a8` (implementer-reported, PR #48). The helper is
        `startBackground(kind, fn) bool` (`client.go:171`); the six sites are the monitor (`:1745`), the poller
        (`:682`), the claim releases (`stream.go:545, :652`) and the two timer bodies (`client.go:440, :1658`); arming
        either timer also refuses once `Close` has begun. Failing first at `29249fb` (3.7b's code): `Close(nil)` and
        `Connect(nil)` panicked; the second `Connect` returned nil; (a) `Close` under an ended context returned nil; (b)
        the late disconnect armed the timer; (c) Close A returned nil while the callback ran; (d) nil where
        `DeadlineExceeded` was wanted; (e) the adapter held the JetStream handle, the monitor and the poller, and
        `Connect` returned nil; the suite failed `NilContextsRefused`, `ControlledStopUnderLiveStartAuthority` and
        `SecondStartRefusedOrRestartCycle`. Added beyond the five items, each failing first the same way: a stopped
        circuit timer, both claim-release joins, and an ended-context `Close` after the join (64 calls). 19 mutants, one
        per fix, all detected; `go test -race -count=20` on the 13 tests and `task test:repeat` pass. Beyond the text,
        on the row: `Connect` overtaken by `Close` returns `nats.ErrConnectionClosed`; a consumer started once `Close`
        has begun is stopped and refused; re-arming the circuit timer stops a pending one; a `Close` whose context has
        ended returns `ctx.Err()` even after the join; a `Connect` that loses admission to a concurrent one leaves the
        winner's status (`TestClientConnectThatLosesAdmissionLeavesStatus`, failing first with `Disconnected`; two
        mutants detected). Task 4.3's `Close(nil)`, `Connect(nil)` and second-`Close` items are proven here; 4.3 stays
        open. Codex's checkpoint review at `7ce1940` (PR #48 comment 5980134911) requested changes F21–F25; the
        guarantees above that they correct (the claim-release refusal, "no callback runs once Close has begun", the
        losing-Connect status) are superseded by 3.7c2.
- [x] 3.7c2 (D) Lifted: Codex's re-review of the fix commits, Codex APPROVE at `4e44422`, comment 5982063929.
      Codex F21–F25 (PR #48 comment 5980134911; design D3 `natsclient-close-is-final`,
      `natsclient-close-honours-each-context`, `natsclient-status-ownership`,
      `natsclient-close-reports-drain-timeout`, and the generated-checks decision). Tests written first and shown to
      fail at `7ce1940`, Codex's reproductions ported with their assertions unchanged: the 20 tests of the D3 revision
      (forced-close Subscribe and SubscribeForRequests join; refused consumer keeps ownership, internal and port; Close
      during another drain; losing dial error, cancel and the two held-write cases; monitor commit after Close; early
      native Closed; Connect after Close; failures after Close; Connect during drain; event handler commit after
      Close; event handlers joined; losing Connect's candidate handler; async publish error after Close; the operation
      table; the suite with a held handler; late delivery refused per ack policy; setup context ending while a handler
      runs; a running async publish error handler joined; native drain timeout reported; a subscription on a replaced
      connection ended and joined). One mutant per fix, each detected or reported as a survivor; `go test -race
      -count=20 -run 'Lifecycle|Close|Connect|Subscribe|Consume' ./natsclient`, `task test:integration --
      ./natsclient` and `task test:repeat` pass. D3, the ledger row and the `Close` doc comment state the corrected
      wording (F24). The late-delivery refusal loses one AckNone message, logged at warn level and counted as
      `late_delivery_refused`; D3 and the ledger row declare it, and the owner accepted it
      (question 4, #9 comment 5980769459).
      - Done in `91832ab`, with the implementation early check's findings I-1 to I-4 fixed in the next commit
        (implementer-reported, PR #48). Failing first at `7ce1940`, each for its stated reason: tests 1, 6, 11, 18 and
        20, a live `Close` returned nil while the handler was held; 2 and 17, the refused setup returned at once with
        its claim released; 3 and 9, the second `Close` and the `Connect` waited 10 s on the drain; 4, the loser left
        `Disconnected` or `Connecting`; 7, `Connect` after `Close` dialled; 8 and 13, the status became `CircuitOpen`
        and `Failures()` 15; 12, the loser returned while its candidate's closed handler ran; 14, 16 rows of the
        closing and closed states; 15, the adapter listed nothing; 16, the handler ran; 19, nil and an unwrapped
        timeout. Tests 5, 10 and 4's held-failure-write case needed the private commit seam, added alone to `7ce1940`
        for that run; 16's core case needs the new subscribe seam and is shown by mutant M6. Those failing runs used
        the short-deadline checks that I-1 replaced; the state checks that replaced them are shown able to fail by
        mutants M1, M2, M9, M11 and M12 below. `go test -race -count=20 -run
        'Lifecycle|Close|Connect|Subscribe|Consume' ./natsclient/` exit 0; `task verify` ok, its integration and
        repeat steps included.
      - Mutants, each applied alone to the code at the evidence commit and restored by checksum, run with `go test
        -race` on the named tests (one run each unless a rate is given; logs are local only). None survived
        outright and none was inconclusive; M2's detection by the JetStream case alone is partial (below).
        - M1, `ownedDelivery.run` runs the handler without counting it: detected by
          `TestClientCloseJoinsConsumerHandlerWhenClosedReportsEarly` (jetstream 10/10, fake) and
          `TestClientLifecycleAdapterListsHeldHandler`. Test 1 does not detect it: a core subscription's end
          already comes from the native closed handler, after the handler returned.
        - M2, the consumer's ownership goroutine releases on `Closed()` alone: detected deterministically by test 6's
          synctest twin (fake, 10/10). Test 6's JetStream case alone is partial: it detected 8 of 10 runs and the
          mutant survived 2.
        - M3, a mutex held across the first `Close`'s cleanup and taken by every other `Close` and by `Connect`:
          detected by `TestClientCloseHonoursItsContextDuringAnotherDrain` and
          `TestClientConnectDuringCloseDrainReturnsPromptly`.
        - M4a, `connectFailed` writes without its ownership check: detected by
          `TestClientLosingConnectLeavesWinnerStatus` (dial-error, canceled-dial, held-before-connecting).
        - M4b, `Connecting` written unconditionally: detected by its held-before-connecting case.
        - M4c, `handleDisconnect` checks, releases `mu`, then writes: detected by
          `TestClientEventHandlerCannotCommitAfterClose/handleDisconnect`.
        - M4d, the monitor commits without its ownership check: detected by
          `TestClientHealthMonitorCannotOverwriteClosedStatus`.
        - M4e, `recordFailure` without its `closing` check: detected by
          `TestClientFailuresAfterCloseLeaveStatusDisconnected` and `TestClientAsyncPublishErrorAfterCloseRecordsMetricOnly`.
        - M4f, `connectFailed` releases `mu` between its check and its write: detected by the held-before-failure-write
          case.
        - M5, the async publish error handler not admitted: detected by
          `TestClientCloseJoinsRunningAsyncPublishErrorHandler`.
        - M6, a refused late delivery runs the handler: detected by `TestClientRefusesLateDeliveryAfterRecordedEnd`, all
          three cases.
        - M7, the refusal not counted on the metric: detected by the same three cases.
        - M8a, the native drain timeout not reported: detected by `TestClientCloseReportsDrainTimeout/native-first`.
        - M8b, the client's drain-timeout error formatted with `%v`, not `%w`: detected by its client-first case.
        - M9, the replaced-connection watcher does not unsubscribe: detected by
          `TestClientCloseEndsSubscriptionOnReplacedConnection/replaced-connection-open`, 10/10.
        - M10, `Connect` without its entry `closing` check: detected by `TestClientConnectAfterCloseRefusesBeforeDial`.
        - M11, a losing `Connect` does not await its candidate's closed handler: detected by
          `TestClientLosingConnectLeavesNoCandidateHandler`, 10/10.
        - M12, the join does not await the event handlers: detected by `TestClientCloseJoinsConnectionEventHandlers`,
          10/10.
        - M13, `SetConnection` installs after `Close` began: detected by `TestClientLifecycleOperationTable` (closing
          and closed `SetConnection` rows).
        - M14, `Subscribe` not refused once `Close` began: detected by the table's closing and closed subscribe rows.
        - M15, the consumer APIs not refused at entry once `Close` began: detected by the table's consumer rows.
        - M16, a refused setup returns before its handlers: detected by
          `TestClientRefusedConsumerKeepsOwnershipUntilHandlersReturn` and
          `TestClientRefusedConsumerSetupContextEndsWhileHandlerRuns`, both cases each.
      - Codex's re-review at `f8fa86d` (PR #48 comment 5981562076) requested F26 and F27. F26: an idle
        client-created consumer left on a connection replaced through `SetConnection` kept `Close` from finishing.
        `TestClientCloseEndsConsumerOnReplacedConnection`, Codex's diagnostic with its assertions unchanged plus a
        port-API case, failed first at `f8fa86d` in both cases: `Close=context deadline exceeded`, the consumer still
        open, one claim. Fixed in the consumer ownership goroutine shared by both APIs: when `Close` begins it stops a
        consumer whose connection is not the drained one, leaves that connection open, and releases the claim only
        once the handler count is zero. Mutant M17 removes that stop: detected, both cases. F27 corrected the ledger
        (owned-work count, status frozen until cleanup, admission as the refusal boundary), D3's consumer-setup
        bullet, and the spec's refusal scenario (the setup context may end first).
      - The early check of `064d38d` (APPROVE) left M-a and N-d. M-a: D3's admission bullet still said one
        `sync.WaitGroup`; it now states the owned-work count. N-d: the consumer's connection was read at admission,
        not with the JetStream handle, so a `SetConnection(nil)` and a second `Connect` between the two reads
        mis-attributed the consumer. The setup now reads the handle and its connection in one critical section
        (`jetStreamWithConn`) and passes the connection to the ownership goroutine.
        `TestClientCloseAttributesConsumerToItsHandlesConnection` holds the setup between the two reads, runs
        `SetConnection(nil)` and a second `Connect`, and requires `Close` to stop the consumer. Run against the
        attribution read at admission (mutant M18), both cases failed: `Close did not stop the consumer on the first
        connection (stopped=false)`.
      - The early check of `a2f975a` bounded every receive on the test goroutine of `client_close_final_test.go`
        (`awaitValue`, `await`, `awaitErr`). Codex's approval at `4e44422` left F28: test 22 released its fake holds
        only on the success path. Each fake hold in the file released only there (test 22's gate and native end;
        the native end of tests 6 and 16; the native return of tests 2 and 17) is now an idempotent release that
        `t.Cleanup` also runs (`releaseInCleanup`), and test 22's cleanup joins its setup goroutine within
        `lifecycleBound`. Shown under mutant M18, which fails before both releases: a goroutine stack probe in the
        parent test's cleanup found a goroutine blocked in the fake at `4e44422` and none after the fix.
      - Departures from the design, each recorded here:
        - Test 6 runs real JetStream on an embedded server in the unit lane, not the Docker lane.
        - Test 15 checks the lifecycle adapter directly, not through the suite.
        - The replaced-connection unsubscribe is done by a per-subscription watcher, because the carried guard
          `TestClientHasNoChildLifecycleSurfaceOrCatalog` forbids a subscription catalog (D3 records it).
        - D3 gains `natsclient-close-reports-drain-timeout` as a named item; the design revision carried it only in
          its implementation section, the spec scenario and the ledger.
        - The spec scenario "Status is final once Close begins" says the status keeps its value until `Close`'s
          cleanup has finished and reads `Disconnected` after; the revision's draft said `Disconnected` from the start,
          which contradicted its own status-ownership text.
        - Carried tests changed with the behaviour, each named on the ledger row or here: `TestContextAwareMethods`
          expects `nats.ErrConnectionClosed`; `TestClientCloseStopsCircuitTimer` arms the circuit timer directly,
          since failures after `Close` no longer open the circuit; `TestClientCloseJoinsClaimRelease` expects the late
          consumer refused before native `Consume`; `subscription_test.go` follows `newSubscription`'s new argument.
        - `handleDisconnect` once `Close` has begun writes no status but still offers the connection-loss timer, so
          the refusal is logged as dropped work, as 3.7c requires.
        - The tests check that a call is still waiting through in-package state (`requireJoinPending`,
          `requireJoinOpen`, the closed-handler wait in test 12), observed at a named wait point under a 10 s failure
          bound, never with a short real-clock wait (design D8, R1b; implementation early check, finding I-1).
- [x] 3.7d (D) Lifted: Codex's checkpoint review of 3.7c and 3.7c2 (concurrency), Codex APPROVE at `4e44422`, comment
      5982063929; the metrics work below remains. `natsclient`
      metrics (design D9; item 4): the three consumer collectors that `Add` the server's current values on every poll
      (`jetstream_metrics.go:305-307`) become gauges `Set` from server state — `consumer_delivered_total` →
      `consumer_delivered_stream_sequence` (`Delivered.Stream`), `consumer_acked_total` →
      `consumer_ack_floor_stream_sequence` (`AckFloor.Stream`), `consumer_redelivered_total` →
      `consumer_redelivered_messages` (`NumRedelivered`) — with namespace and subsystem unchanged pending #69; the name
      and type changes are consumer impact on the row. No client label. Tests written first: (1) two polls of an
      unchanged consumer leave each of the three gathered values equal to the server's (the pin doubles them); (2) two
      clients on one registry: each of the 11 collectors is the canonical one, and a value written through the second
      client is gathered (fails at the pin for the 8 registered through `Register*`, `:128-146, :158-160`);
      `jetstream_metrics_test.go:16` covers all 11. The shared-series ownership defects (`stream_state` set to 0 by one
      client's failed `Info`, `:278`; `forgetConsumer` deleting series another client still writes, `:232-243`) are
      tracked by #75, named on the row's `known_risks` (#9 comment 5968830525, rule 4; owner ruling #9 comment
      5969776736, item 5), not fixed here.
      - Done (owner, 2026-10-04: "yes, carry on with 3.7d"). The three collectors are `GaugeVec`s `Set` from
        `Delivered.Stream`, `AckFloor.Stream` and `NumRedelivered` (`jetstream_metrics.go:309-311`). Test (1) is
        `TestJetStreamConsumerMetricsReportServerStateAcrossPolls`; test (2) is `jetstream_metrics_test.go:16`, extended
        to all 11 collectors and renamed `TestJetStreamMetricsShareCanonicalCollectorsAcrossOwners`. The row's
        contract names the changes and their consumer impact; its `known_risks` names #75.
        `integration_test.go`'s delivered-metric read uses the new name as a gauge. gopls `references` on the three
        fields found no other reader; `doc.go` names no metric, and `README.md` is not yet carried (3.7e).
      - Failing first at `82e979b`. Test (1) failed for its stated reason: two polls gathered 10, 8 and 4 for the
        server's 5, 4 and 2; it also failed on the three new names. Test (2) failed only on the three new names: its
        canonical-collector checks already passed at `82e979b`, because task 3.7 had moved registration to
        `RegisterOrGet`. Its stated reason, the pin's orphaned 8, is shown by mutant M20 instead.
      - Mutants, each restored by checksum. M19, `Add` in place of `Set` for the three: detected by test (1), all
        three values doubled, and the third poll, where `NumRedelivered` falls from 2 to 1, gathered 16, 14 and 5
        for the server's 6, 6 and 1. M20, the pin's shape, each of the 8 collectors the pin registered through `Register*`
        keeping its own candidate instead of the collector `RegisterOrGet` returns: detected by test (2), identity
        differs for all 8.
      - Early check of `112854b` (APPROVE): M-b, `NumRedelivered` counts messages redelivered and not yet
        acknowledged, and falls on acknowledgement (nats.go `consumer_config.go:53-57`). The gauge's help text says
        so, the row's consumer impact says the other two values are stream sequences, not counts, and test (1) has
        a third poll in which `NumRedelivered` falls. N-e: a comment says the registration key is the registry
        handle, not the metric name. N-f: test (1) polls under `t.Context()`.
      - Codex APPROVE at `c69d7ac` (comment 5982926823) left F29: the collector comment, D9, the row and this task
        called the server's values cumulative, but pending and outstanding redeliveries fall. Each now says the
        gauge is set from the server's current state.
      - Choices the design and the task text leave open, recorded here: the three gauges' help texts are new
        wording; the three registry keys (`consumer_delivered`, `consumer_acked`, `consumer_redelivered`) are kept
        unchanged, so `Unregister` by those keys still works.
      - Generated checks: not used. A poll overwrites each gauge with the server's value and keeps no state of
        its own, so the history to cover is "more than one poll", which test (1)'s two polls are; the registration
        history is D9's `TestPropRegisterOrGetHistory`.
- [x] 3.7e (D) `natsclient` `README.md` (375 lines at the pin) and `doc.go`: import paths at `README.md:14, :312`
      and `doc.go:512` name the SemEngine module, each its own `adapt` item (#9 comment 5968665464); the lint fixes
      (12 × MD013, 1 × MD032); behaviour edits by line (#9 comment 5957221949): `NewTestClient` (`README.md:277-281`,
      `doc.go:352-354`) → `natsfixture`; `WithClosedCallback` (`README.md:231`), which does not exist; the defaults at
      `README.md:213, :216`; `doc.go:339` (`WithTLS(true)`) and `doc.go:252` (a method shown as an option); every
      passage that shows a 3.7a drop; `Close` and `Connect` as 3.7c leaves them; the request-handler-timeout comments
      that name the dropped option (`client.go:100, :168`, `request.go:29`). `task docs:check` passes.
      - Done for every item above. `natsclient/README.md` is
        ported from the pin and `doc.go` edited; each edit is on the `natsclient` row by pin line (import paths
        `README.md:14, :312` and `doc.go:512`; lint: 12 × MD013, the two long headings split as on the `metric`
        row, 1 × MD032; behaviour edits; the three timeout comments, which are at the pin's `client.go:99, :169`
        and `request.go:29`). Beyond the lines named: `README.md:21` and `doc.go:12` state the same
        circuit-breaker default as `README.md:216` (5; the code's is 15, at the pin too), and `README.md:25` and
        `doc.go:17` promise the state-change callbacks 3.7a dropped. `doc.go:377`'s "subsequent calls are no-ops"
        is the `Close` passage 3.7c changes. Neither file teaches `Closed()` as the completion proof: the README
        does not mention it, and `doc.go`'s Consume example keeps the #83 wording (a nil `Close` is the proof).
        `task docs:check` passes.
      - Settled by owner ruling (#9 comment 5984291337, extending 5957221949): a passage that describes API the
        code does not have, including API that never existed at the pin, is corrected to the real API, and links
        to packages SemEngine does not have are removed. Applied, each an `adapt` item on the `natsclient` row
        by pin line: `README.md:61-64` and `doc.go:46` (`Subscribe`), `README.md:111` and `doc.go:427`
        (`Request`), `README.md:160` (`ClientOption`), `README.md:150-155` (`KVOptions`), `README.md:182-190`
        and `doc.go:318` (`WithLogger` takes `*slog.Logger`), `README.md:318` and `doc.go:518`
        (`slog.Default()`), `doc.go:320` (`WithName`), `README.md:369-371` (the related-packages links removed).
- [x] 3.8 (D) `task cover:check` targets `natsclient`, `message`, `payloadregistry` at 80%: the first measurement
      of each is recorded on this pull request (design P8: the pin baseline is unmeasured). If any of the three
      measures below 80%, a new task asking the owner to rule on that package's coverage is added to this file at
      that moment, written so `task spec:queue` reads it, and the gate stays unwaived (plan `:206-209`); nothing in
      this file waits on the owner until the measurement exists.
      - Done; all three are above 80%, so no owner task is added. First measurement, `task cover:check` inside
        `task verify` at `ed3bcd0` (local) and in CI run 37232527143's `Verify` job at the same commit, both
        green with the same figures: `natsclient` 87.8% (merged unit and integration profiles), `message` 91.9%,
        `payloadregistry` 96.3% (unit profile); the harness packages `lifecycletest` 92.4%, `probe` 97.7%,
        `natsfixture` 91.5%. Recorded here and not as a PR comment, by the instruction for this session. The CI
        log is the artifact; the local profiles are local only.
- [x] 3.9 (D) `stretchr/testify` and `pgregory.net/rapid` v1.3.0 are direct requirements (design D1; Rapid by owner
      ruling, PR #48 comment 5951926492), with `github.com/nats-io/nats-server/v2` v2.14.7 (design D1; task 3.7), and
      `pkg/types/entity_id_prop_test.go`, the floor's one Rapid file (design P23), is ported with its property test;
      `task tidy:check` passes; the ported test-file count is recorded with its arithmetic: the pin's 127, less the six
      excluded `natsclient` files, `pkg/acme`'s two, and the files removed with dropped surface (`typed_test.go`,
      `kv_temporal_integration_test.go`, and any the other surface audits name on their rows), plus files this change
      adds (each on its row, e.g. `message/meta_wire_test.go`, `message/decoder_fuzz_test.go`). The line count after
      repair is recorded with `wc` against the pin's 33,279, less each deleted test the rows list, with a diff stat
      against the pin per package.
      - Done; already true, nothing to change in `go.mod`. Its first `require` block lists
        `github.com/stretchr/testify v1.12.1` (the pin has v1.11.1, `go.mod:18`), `pgregory.net/rapid v1.3.0` and
        `github.com/nats-io/nats-server/v2 v2.14.7`, none `// indirect`. `task tidy:check` exit 0.
        `pkg/types/entity_id_prop_test.go` is ported; it differs from the pin only in its six spec-path comments
        (`entity-id-contract` → `docs/specs/entity-id-contract.md`, task 3.3a), and `go test -run Prop -v
        ./pkg/types/` passes its three property tests. Rapid is also imported by two files this change adds,
        `metric/registerorget_prop_test.go` and `internal/cache/coalescing_set_prop_test.go`.
      - Counts are over `*_test.go` in each package directory (not subdirectories), the pin from the GitHub tarball
        of `8b99efe9` with `find` and `wc -l`, the tree at `a95c84a`, the commit that fixes the 3.10 review's
        HIGH-1 and MEDIUM-2 (re-measured after tasks 4.2 and 4.3 added tests, and again after that fix). Test files: the
        pin's 127 (the 15 packages' 125 and `pkg/acme`'s 2) − 6 excluded `natsclient` files
        − 2 `pkg/acme` − 2 removed with dropped
        surface (`typed_test.go`, `kv_temporal_integration_test.go`; no other surface audit removed a file) =
        117, + 17 added = 134, which is the tree's count. The 17, each named on its row: `metric` 5
        (`admission_test.go`, `lifecycle_test.go`, `registerorget_prop_test.go`, `registerorget_test.go`,
        `tls_test.go`); `message` 4 (`decoder_fuzz_test.go`, `generic_json_fuzz_test.go`,
        `generic_json_shape_test.go`, `meta_wire_test.go`); `internal/cache` 4 (`background_test.go`,
        `coalescing_set_prop_test.go`, `config_fuzz_test.go`, `helper_process_test.go`); `natsclient` 4
        (`client_close_final_test.go`, `client_lifecycle_test.go`, `test_helpers_test.go`,
        `test_helpers_integration_test.go`). `helper_process_test.go` and `client_close_final_test.go` were not
        named on their rows; this commit names them.
      - Lines: the pin's 33,279 checks out (34,887 over the 16 directories − `pkg/acme`'s 215 −
        `test_client_factory_test.go` 312, `test_client_integration_test.go` 361, `test_client_readiness_test.go`
        581, `monitoring_consumers_test.go` 139). Less the four other removed files (`test_options_test.go` 55,
        `mapped_port_retry_test.go` 276, `typed_test.go` 364, `kv_temporal_integration_test.go` 238): 32,346. The
        tree has 38,476. Tests deleted inside carried files (`TestTemporalResolver_ErrorBoundaries`, 3.7a's
        deletions) are counted in the diff stat, not subtracted one by one. Diff stat of the test files against the
        pin (plain `diff -U0`, added and removed lines; a removed file counts all its lines): `internal/resource`
        +329 −237; `pkg/retry` +119 −110; `internal/timestamp` +1 −1; `vocabulary` +0 −80; `pkg/types` +41 −9;
        `pkg/projection/contract` +1 −1; `internal/tlsutil` +2 −2; `metric` +1,137 −314; `payloadregistry` +3 −3;
        `message` +1,370 −6; `internal/cache` +1,770 −937; `natsclient` +5,209 −4,478; `pkg/errs`, `pkg/platform`,
        `pkg/security` unchanged; total +9,982 −6,178, and 34,887 − 215 + 9,982 − 6,178 = 38,476.
- [x] 3.10 (R) Port review per package group (3.1–3.7e) and of task 2.8's rehomed helpers: the two service adapters list
      every retained kind (the review checklist of the `lifecycle-suite` delta); each helper has the shape design D7
      gives it, its `synctest` test and its nil-context refusal, and no fixed shutdown timeout remains
      (`background-work` delta); rows validate, no file beyond the pin's was added except: the two adapters; the two
      test-side helper files that replace `test_client.go` (`test_helpers_test.go`, `test_helpers_integration_test.go`);
      `natsclient/owned_delivery.go` and `client_close_final_test.go` (D3 F21–F28); `metric/admission_test.go`,
      `registerorget_test.go`, `registerorget_prop_test.go`, `tls_test.go`; the four `message` test files (3.6b); the
      four `internal/cache` test files (D7); `internal/harness/payloadfixture/doc.go`; the `natsfixture` max-payload
      option and its tests; and the rewritten `NewTestClient` sites (corrected per the 3.10 review, PR #48 comment
      5984567497, MEDIUM-3); the census tests' expected maps match the tree; and every D8 repair is on its
      row; every real-clock timer in a repaired file is either an R1b failure bound sized per D8 or a site in D8's
      disposition table with that disposition (a review check the text check cannot make);
      `go test -v -run TestPublicSignatures ./internal/harness/contract` shows "public packages checked: 11" and passes,
      and a run with a temporary exported `natsclient` function returning `*resource.Watcher` (from `internal/resource`,
      used at `client.go:1394`) fails naming it (design D5, task 2.10). Verdict on this pull request.
      - Done: APPROVE on re-check 2 at `5f7ee90`. The Claude port-review records on PR #48: comment 5984567497
        (the review at `887f41b`, CHANGES REQUESTED: HIGH-1, MEDIUM-2 to MEDIUM-6, NIT-7), comment 5984883313
        (re-check at `db7d037`, CHANGES REQUESTED: HIGH-R1, MEDIUM-R2, NIT-R3) and comment 5985118144 (re-check 2
        at `5f7ee90`, APPROVE, NIT-R4). Fix commits: `a95c84a` (HIGH-1, MEDIUM-2, -3, -4, -6, NIT-7), `db7d037`
        (MEDIUM-5, the technical writer's), `5f7ee90` (HIGH-R1, MEDIUM-R2, NIT-R3) and the commit that ticks this
        task (NIT-R4: `TestIntegration_CircuitBreakerWithRealConnection` requires the 15 dials reported through
        `opHook` before the circuit-open `Connect`; a mutant that stops `connectWith` reporting its dial fails it,
        "the 15 failed Connects each reported a dial through opHook"). These are Claude's own reviewer's records;
        the cross-agent review of the code is task 7.1.
- [x] 3.11 (D) CI time: the wall time of `task verify` per step (`scripts/verify.sh` prints it) and of the CI `verify`
      job with every package ported are recorded on this pull request, with the integration lane's time against its
      `-timeout 10m` (`scripts/test-integration.sh:402`). If the job exceeds its 15-minute limit (`merge-gate`
      "Required needs both jobs"; `ci.yml:22`) or the lane exceeds its timeout, a new task asking the owner to rule is
      added to this file at that moment, written so `task spec:queue` reads it, and the limit stays unchanged: the
      limit is spec, and neither the developer nor the reviewer may raise it.
      - Done; both are within their limits, so no owner task is added. At `ed3bcd0`, every package ported. CI run
        37232527143, `Verify` job: 4 min 43 s (20:33:02 to 20:37:45 UTC) against `timeout-minutes: 15`; its
        step timings: spec:check 1 s, docs:check 1, fmt:check 0, tidy:check 1, cleanup-roots:check 0, build 0,
        vet 1, lint 1, vuln 2, ledger:check 3, test:unit 41, test:integration 122, cover:check 4, test:repeat 85.
        The integration lane's step took 122 s against its `-timeout 10m`. Local `task verify` (exit 0, on a
        shared host): spec:check 1 s, docs:check 1, fmt:check 0, tidy:check 0, cleanup-roots:check 0, build 0,
        vet 1, lint 2, vuln 1, ledger:check 4, test:unit 57, test:integration 142, cover:check 4, test:repeat
        140, 353 s in all; the runner recorded `go_test_ms=132550` for the lane (local evidence directory).

- [x] 3.12 (D) Surface audit of vocabulary, message capability interfaces and pkg/security (owner rulings
      5985697767, 5985900154): the architect's inventory, inventory-reviewed PASS, is committed as
      `openspec/changes/setup-04a-01-floor/surface-audit.md` (lint fixes only) and cited from the `vocabulary`,
      `message`, `pkg/security` and `pkg/tlsutil` rows; what it marks dead is removed and recorded on those rows by
      pin line, with its readers.
      - Done. `b902520` the audit file; `3b7c8aa` `vocabulary` keeps 92 of 258 exported identifiers (162 removed;
        README and `doc.go` corrected, including the SSN/SOSA constants that never existed);
        `TestGetRelationshipPredicates` asserts `graph.rel.contains` is the only `graph.rel.*` registration (a
        mutant re-registering `graph.rel.near` fails it); `a89555c` the ten `message` capability interfaces removed,
        `IndexingProfiler` kept, the runtime-discovery and spatial-indexing docs corrected; `6248299` `pkg/security`
        drops 4 `Default*` and 7 `Validate` (row now `adapt`, its "no I/O" claim corrected), gains
        `config_test.go` (mutants: a renamed JSON key, an uncovered new field), and `internal/tlsutil` refuses a
        `MinVersion` other than "1.2", "1.3" or empty (failed first: 18 of 18 near-miss subtests accepted; mutants:
        the restored 1.2 fallback, a fatal class for the refusal); `a2cf43d` `PredicateAuthority` cut. The tree has
        no mention of a removed name outside the audit file and the ledger's records of it (`git grep` over code,
        docs, specs, design and ledger). `TestPublicSignatures` passes with its count unchanged; no census test
        names these packages. The README sub-package sections (`bfo/`, `cco/`, `agentic/`, `export/`) and the
        `docs/vocabulary/` links in `vocabulary/README.md`, and `message/README.md`'s References links to packages
        SemEngine does not have, are removed (owner ruling, #9 comment 5994720412, item 5; both rows).
- [x] 3.12a (D) The `RequireClientCert` fail-open (owner ruling 5985900154, item 3): with mTLS enabled,
      `RequireClientCert` false and `AllowedClientCNs` empty, `internal/tlsutil` admitted a client with no
      certificate, and the schema tag (`pkg/security/config.go:91`) called the default true while the Go zero
      value was false. Lifted: the owner chose on #9 (comment 5994720412, item 4): `RequireClientCert` is replaced
      by `ClientCertOptional` (JSON `client_cert_optional`), so the zero value requires a client certificate and
      optional mode stays when it is set.
      - Done. `TestServerMTLSRequiresClientCertificateByDefault` (`internal/tlsutil/mtls_default_test.go`): neither
        field set and no CN allowlist, a client with no certificate is refused and one the client CA signed is
        served, in real handshakes on TLS 1.2 and 1.3. Failed first on both versions (the client with no
        certificate was served); a mutant inverting the default fails it. The optional-mode tests set
        `ClientCertOptional: true`; the `default:true` tag is gone; the exported-field change is on the
        `pkg/security` row.

- [x] 3.13 (D) The owner's independent read (PR #48 comment 5985648705), owner rulings #9 comment 5994720412 items
      1-5, and the reviewer early check at `5ecc016` (Claude's own `semengine-reviewer`, PR #48; not the review of
      record). Each with a test written first; failing-first runs and mutants are implementer-reported unless named.
      - `765a9d9`: `safeHandleMessage` logs, counts (`handler_panic`) and Naks a recovered handler panic and logs a
        failed Nak (`TestSafeHandleMessageRecoversPanicNaksLogsAndCounts`); a consumer whose `Info` fails has its four
        gauges deleted, `consumer_info` counted and one Warn per transition
        (`TestJetStreamConsumerMetricsDropSeriesWhenInfoFails`). Both failed first; the reviewer's mutants at `5ecc016`
        (`DeleteLabelValues` removed, the log-once guard removed) each fail the second. Three tests that could not fail
        now can: the Nak test's AckWait exceeds its await bound, the native-drain test asserts the durable's claim is
        released, and the cache concurrency test checks `Set` errors and reads every write.
      - Item 1, no default Nak: the promise is removed in `765a9d9`; no text still makes it.
      - Item 4, `cd06a08`: task 3.12a.
      - Item 5, `4cae56d`: task 3.12's closing note.
      - Item 3, `9e5b53a` and `2a9909e`: task 3.6a's sub-bullet and design D7.
        `TestCoalescingSet_CallbackPanicIsRecoveredAndLaterBatchesFire` failed first (the binary died with the
        callback's panic). Its mutants (no count, no log, no recover, the registry ignored) each fail it.
      - Re-check MEDIUM at `a517c8c`: `NewCoalescingSet` returns an error, like the cache constructors.
        - `TestCoalescingSet_RefusesARegistryThatRefusesThePanicCounter` replaces the logged-degrade test: a refused
          panic counter is a transient error, with no goroutine started. It failed first against the degrade, and a
          mutant that swallows the refusal fails it.
        - A nil context or nil callback is an invalid-argument error, not a panic (design D7: an error where the entry
          returns one): `TestCoalescingSetNilContextRefusedAtTheCall` and `TestCoalescingSet_NilCallbackRefusedAtTheCall`
          failed first (the binary panicked). A mutant accepting a nil callback fails its test.
      - Item 2, `5ecc016`: the caller's context bounds setup only. Handlers run under a context the client owns per
        consumer, cancelled when Close begins, and Close joins them (design D3 `natsclient-consume-handler-context`;
        `transport-client` requirement).
        - `TestConsumeSetupDeadlineDoesNotEndLaterHandlers` and `TestConsumeHandlerContextEndsWhenCloseBegins` cover
          the internal and port paths. Both failed first on both.
        - Their mutants each fail them: no cancel on Close, port handlers under the setup context, and internal
          handlers under the setup context.
      - Early check HIGH, `b620a4f`: Close cancels a running handler's context after delivery has ended.
        - `TestConsumeHandlerContextEndsWhenCloseBeginsAfterDeliveryEnded` covers the internal and port paths, with
          `DisableMessageTimeout`. It failed first on both ("not within 10s").
        - The cancel removed from that wait fails it.
      - Early check MEDIUM, `b620a4f`: `TestConsumeHandlerContextEndsWhenCloseBeginsBeforeTheHandle`.
        - The cancel removed from the closing case before the handle survived the whole unit suite at `5ecc016`
          (reviewer). It now fails this test on both paths.
      - Early check MEDIUM, `3a10d6f`: `prochost.Process.StderrPath` and its `process-host` scenario are cut as dead
        surface (task 3.6a).

## 4. Repair evidence this change can produce (ruling g)

- [x] 4.1 (D) Settlement, `natsclient` half: the 18 unit and 3 integration settlement tests pass against `natsfixture`;
      the `transport-client` "Settlement follows the decision" scenarios map to named tests (long work with heartbeat →
      `TestIntegrationConsumeDeliveryWithHeartbeatHealthyRenewalPreventsOverlap`; semantic retry →
      `TestIntegrationSemanticRetryProducesDurableRedelivery`; work panics →
      `TestConsumeDeliveryWithHeartbeatControlLossNormalizesInvalidAndPanic` and
      `TestConsumeDeliveryWithHeartbeatPanicAndZeroPolicyFailClosed`, `delivery_settlement_test.go:690,739`); the row
      names change 2 for the graph-ingest half. The tests that read `DeliveryResult`'s dropped getters read its
      unexported fields instead, with the same assertions (task 3.7a).
      - Done; no code changed, so no failing-first test. The unit tests are the pin's 18 in
        `delivery_settlement_test.go` plus `TestConsumeDeliveryWithHeartbeatErrCarriesControlLoss` (task 3.7b);
        `go test -race -count=1 -v` on the 19 shows 19 PASS. The three integration tests run on `natsfixture`
        (`newFixtureClient`) and pass under `task test:integration -- -v -run '…' ./natsclient` (exit 0; the
        runner's evidence directory is local only). The scenarios map as listed above, plus "Stopped heartbeat"
        → `TestIntegrationConsumeDeliveryWithHeartbeatStoppedRenewalUsesBackOff`; the two panic tests are now at
        `delivery_settlement_test.go:725, :778`. No test calls a dropped `DeliveryResult` getter (grep: none;
        task 3.7a retargeted the 18 calls). The `natsclient` row's `proving_tests` names the tests, the scenario
        map and change 2 for the graph-ingest half.
- [x] 4.2 (D) Acknowledged is not durable, `natsclient` half: an integration test publishes through the ported client
      to a memory-backed and a file-backed stream, restarts the fixture (task 2.1's primitive), and asserts absence
      and presence respectively; the `natsclient` row names it and change 2 for the graph-ingest scenarios.
      - Done: `TestIntegration_AcknowledgedIsNotDurableOnMemoryStream` (`natsclient/integration_test.go`). It
        creates both streams through the fixture, checks through the client that the memory stream reports
        `MemoryStorage`, publishes one message to each with `PublishToStream`, reads both back at sequence 1,
        closes the client, restarts the fixture, and with a client on the new URL reads the file-backed message
        and finds the memory-backed one gone (the stream was not found after the restart; the test also accepts
        an empty re-created stream, as `natsfixture`'s own restart test does). Passed on its first run under
        `task test:integration`: it tests behaviour already in place (the broker's and task 2.1's), so there is
        no failing-first run. Mutants through the runner, each restored by checksum: `CreateMemoryStream` making
        a file-backed stream: detected by the storage-class assertion; the same with that assertion disabled in
        the test: detected by the absence check ("memory-backed message after the restart": got nil, want
        `ErrMsgNotFound`); `publishToStream` skipping `PublishMsg`: detected by the read-back before the restart.
        The `natsclient` row names the test and change 2 for the graph-ingest scenarios.
- [x] 4.3 (D) `Close`/`Drain` bounded: tests show `Drain(nil)` refused without a server call (carried), `Close(nil)` and
      `Connect(nil)` refused with the connection untouched (new, test written first, task 3.7c), `Close` under a short
      deadline returning within it with the connection closed, and a second `Close` returning nil (carried where the pin
      has them; new ones where the row records a gap).
      - Done. Each clause and its test (all in `natsclient`, unit lane):
        - `Drain(nil)`: `TestSubscriptionDrainRefusesNilContext` (`subscription_test.go`), new. The refusal is
          carried (the pin's `client.go:798`); a test of it is not: the pin's `natsclient` tests have none, so
          "carried" in this task holds for the behaviour only. The error names the nil context, the fake native
          subscription's `Drain` is never called and it stays valid.
        - `Close(nil)`, `Connect(nil)`: `TestClientCloseRefusesNilContext` (the connection stays connected) and
          `TestClientConnectRefusesNilContext` (no dial), both written first in task 3.7c.
        - `Close` under a short deadline: `TestClientCloseUnderShortDeadlineClosesConnection`
          (`client_lifecycle_test.go`), new, the pin having no such test. A handler holds the drain; case
          `deadline` gives `Close` 200 ms, case `cancelled-during-drain` cancels once `DRAINING_SUBS` is observed.
          `Close` returns the context's error while the handler is held, and the connection is closed.
        - A second `Close` returning nil: the same test, where a second `Close` with a live context is shown
          waiting on its context (a test context that records its first `Done` call), the join still open, and
          then nil once the handler is released; also `TestClientCloseUnderEndedContextAlwaysReportsIt` and
          `TestClientCloseJoinsConnectionLossCallback` (3.7c). The pin has none.
      - Failing first: not possible for either new test, since the code already behaved as required; both passed
        on their first run. They are shown able to fail by mutants of `client.go`, each applied alone and
        restored by checksum, run with `go test -race -v -count=N` on the two tests (logs local only): D1, the
        nil check in `Subscription.Drain` removed: detected 5/5 (`Drain(nil)` panicked). C1, the deadline clamp
        and the context case of `drainAndCloseConnection` removed: detected 3/3 in both cases (`Close` did not
        return within 10 s). C2, the context case's `conn.Close()` removed: detected 10/10 by
        `cancelled-during-drain`; the `deadline` case alone is partial (2 of 10, and 1 of 10 after the change
        below), because the clamp sets the drain timer to the same deadline and that timer's branch also closes
        the connection. C3, a later `Close`
        returning nil at once (the pin's `:583-585`): detected 10/10 in both cases. `go test -race -count=20` on
        both tests passes. The row's `proving_tests` records the two gaps and the new tests.
      - Corrected after `task verify` at `38a1074` failed `test:repeat` once (the `deadline` case, 1 failure in 5
        shuffled runs at one CPU): the test read `IsClosed()` the moment `Close` returned, but nats.go's own drain
        goroutine, finding no subscriptions once `Close` has force-closed the connection, moves the status to
        `DRAINING_PUBS` and then closes again (nats.go v1.54.0 `nats.go:6378-6390`), so `IsClosed` can read false
        for that moment. The test now waits for `IsClosed` with `probe.Await` under `lifecycleBound`; without the
        force close the native drain keeps the connection open for the client's 30 s drain timeout, past that
        bound. `go test -count=50 -cpu 1 -shuffle=on` and `go test -race -count=20` on the test pass; mutants C1,
        C2 and C3 re-run with the outcomes above.

## 5. Ledger items that need no port, and boundary gates

- [x] 5.1 (W) Eight `defer-exclude` rows for the D4 ten-out packages never carried (`agentic`, `agentic/agentrun`,
      `gateway`, `gateway/graph-gateway`, `internal/agentterminal`, `internal/deliverylane`, `internal/looptoken`,
      `vocabulary/agentic`) at the pin SHA with the D4 reason each; `task ledger:check` passes.
      Added at the end of `docs/admission-ledger.yaml` (46 entries), each `destination: none`, citing SETUP 03B
      design D4 and the scope ruling (#8 comment 5930898291); `task ledger:check` passes.
- [x] 5.2 (A) The Tier-1 cross-check re-measured on the ruled 65-package set (#9 item 1), recorded next to the
      ledger as an inventory with its command and result; it changes no row.
      Recorded in `docs/tier1-cross-check.md`: 39 in both, 23 Tier-1-only, 26 tier-0-only. These are the
      first-pass counts with six packages swapped each way between "both" and "Tier-1-only" and four within
      "tier-0-only". Each Tier-1-only package has a disposition from a ruling or this measurement. Neither
      first-wave consumer imports one without a ruling. No row changes. Side check: the ruled set sums to
      140,842 non-test lines (03B D4).
- [x] 5.3 (D) Package-doc lint sensitivity: `package-comments` is already on (`revive.toml:22`); `task lint` passes
      over the ported tree (every package in the set has a package comment at the pin, inventory §3.3), and a run
      with one public package's comment removed fails naming it; the eleven public destinations (design D5) are the
      ones the check protects.
      - Done; no code changed. `task lint` exit 0, and `go list -f '{{.Doc}}'` is non-empty for all 15 packages.
        For each of the eleven public packages in turn (`pkg/platform`, `pkg/retry`, `pkg/security`, `pkg/errs`,
        `vocabulary`, `pkg/types`, `pkg/projection/contract`, `metric`, `payloadregistry`, `message`,
        `natsclient`), the `//` comment block attached to the `package` clause was removed from every non-test
        file that had one (1 to 5 files: `natsclient` 5, `message` 4, five packages 2, four packages 1), and
        `task lint`'s revive command (`scripts/gopkgs.sh go tool revive -config revive.toml -formatter friendly
        ./...`) run; then every file was restored and checked by checksum (the script is local only). All eleven
        runs exit 1 with one finding, "should have a package comment" (`package-comments`), at a file in that
        package's directory, e.g. `natsclient/backing_stream_prefix.go:1:1`, `pkg/platform/platform.go:1:1`. The
        named file can be a test file (`pkg/errs/classified_is_test.go`, `metric/admission_test.go`,
        `payloadregistry/attributes_test.go`); the path names the package either way. Where a package carries its
        comment in several files, removing it from one file alone does not fail: revive asks for one package
        comment per package, so the check protects the package, not each file.
- [x] 5.4 (D) `scripts/cover-check.sh` reads its targets from a list that this and later changes extend, adding
      `natsclient`, `message` and `payloadregistry`, which lie outside the `internal/harness` base the script
      hard-codes today (`cover-check.sh:14`), and `natsclient`'s statements come from the merged unit and integration
      profiles; `TestCoverCheckSensitivity` (`cover_test.go:37`), which today names only the three harness packages,
      gains a below-80% and a missing-from-profile case for a package outside `internal/harness`, written first and
      failing; `TestCoverCheckPrintsFailingTest` (`cover_test.go:134`, flake-defense 4.4, merged) still passes.
      - Done in the commit that ticks this task. The list is the `targets` array at the top of
        `scripts/cover-check.sh`, one `"<package directory> <profile>"` line each, profile `unit`, `integration`
        or `merged`; a later port adds its line. It holds the three harness packages as before, `message unit`,
        `payloadregistry unit` and `natsclient merged`. The no-argument mode's one `go test` run writes the unit
        profile over every `unit` and `merged` target; a `merged` target is measured over both profiles at once,
        a block counting once and covered if either run covered it. Messages name the package directory, with
        `internal/harness/` dropped, so the harness lines read as before.
      - Failing first, against the script before this change: `TestCoverCheckSensitivity` gained five subtests and
        all five failed with `err=<nil>`, because the script measured none of the three packages: `message
        below` (want "message 70.0%"), `payloadregistry missing` (want "payloadregistry: no statements"),
        `natsclient below in both profiles` (want "natsclient 70.0%"), `natsclient missing from both profiles`
        (want "natsclient: no statements"), and `natsclient merged from both profiles` (unit and integration
        each cover 40%, their union 80%; want "natsclient 80.0%" and exit 0). The four harness subtests and
        `TestCoverCheckPrintsFailingTest` passed then and pass now.
      - Mutants of the script, each restored by checksum: `merged` measured from the unit profile only: detected
        by the merged subtest; the `natsclient merged` line removed: detected by the three `natsclient`
        subtests; `merged` targets left out of the unit run: survived the sensitivity test, which passes both
        profiles in, so `TestCoverCheckUnitRunCoversUnitAndMergedTargets` was added (written after the script
        change; a fake `go` records its arguments) and detects it. Not covered by a test: the refusal of an
        unknown profile word in the list.

## 6. Docs

- [x] 6.2 (W) `docs/testing.md` (PR #39) or its successor gains: the `Restart` contract (`URL()` valid until the next
      restart; stop the owner before, start after), `FaultKV`'s before/after semantics, the helper-process pattern,
      the lifecycle suite for services with its required must-fail factory and the adapter checklist, the three
      shapes of background work with their `synctest` test, and the repair classes for a ported test (design D8);
      written for a working developer, each coined term defined at first use.
      In `docs/testing.md`: the `Restart`, `FaultKV` and `prochost` entries under "Helper packages", and new sections
      "Services and the lifecycle suite", "Background work" and "Porting a test from SemStreams"; the stale
      `Run(t, factory, promise)` signature and the three-target coverage lists here and in `docs/repository-map.md`
      corrected to the code. `task docs:check` passes.
- [ ] 6.3 (W) The `openspec/specs/` sync: the eight deltas applied to `harness-boundaries`, `nats-fixture`,
      `lifecycle-suite` and the five new capabilities (`process-host`, `transport-client`, `background-work`,
      `metric-registry`, `message-codec`),
      verified against the code as landed; `task spec:check` passes.
- [x] 6.4 (W) `.agents/contracts/semengine-developer.md` and `.agents/contracts/semengine-reviewer.md` gain the
      "Background work" subsection after "Context ownership", and their detach bullets the no-join-by-timer clause, as
      drafted on this pull request; `AGENTS.md`'s "Rules and what enforces them" table carries the background-work row,
      added in commit b09374a; `task docs:check` passes; and, because guidance returns with the package (architect
      contract § Extraction slices): the pin's `.agents/contracts/semstreams-developer.md` "NATS RPC" (`:226-233`) and
      "Storage and retention contracts" (`:176-193`), `.agents/contracts/semstreams-reviewer.md` "NATS RPC error
      contract" (`:217-226`) and "Storage, retention, and cutover review" (`:170-186`), and
      `.agents/skills/kv-or-stream`. Each is read at the pin and carried in adapted form, or named as not applying with
      its reason.
      Both contracts carry "Background work" after "Context ownership" and the no-timer-in-place-of-a-join clause in
      their detach bullets, and `AGENTS.md` carries the background-work row. Read at the pin with `gh api
      …?ref=8b99efe9` (`docs/inventory-scope.md` rule 4):
      - "NATS RPC" and "NATS RPC error contract": carried, adapted to `natsclient`, as § NATS RPC (developer) and
        § NATS RPC error contract (reviewer); the pin's "repository RPC contract" is `natsclient/doc.go`'s, and
        "gateways" became "code that passes a reply on" (no gateway is ported). `AGENTS.md` gains their row.
      - "Storage and retention contracts" and "Storage, retention, and cutover review": the first two bullets
        carried, adapted to `natsclient`, as § Storage and retention (developer) and § Storage and retention review
        (reviewer), because `natsclient` enforces them: `BucketClass` and `RetentionPolicy` (`kvspec.go`),
        `CheckNoLifecycleRetention` and `AssertNoLifecycleRetention` (`kv.go`), `CheckStreamBounds`
        (`stream_bounds.go`); port review MEDIUM-5, PR #48 comment 5984567497. The other bullets do not apply
        here: they govern the `storage` package's `Store` and `StorageReference` and retained deployed state, which
        this change does not port, and return with the change that ports `storage` and the graph processors.
      - `kv-or-stream`: not applying here. It decides how components and processors communicate; no component or
        processor is ported, and the concept docs and buckets it cites (`docs/concepts/02-kv-twofer.md`,
        `03-streams-vs-kv-watches.md`, `ENTITY_STATES`) do not exist here. It returns with the first change that
        ports a component.
      `task docs:check` passes.

## 7. Review and archive

- [ ] 7.1 Hold: independent change review. The reviewer's verdict on the full diff (harness, 15 packages, rows,
      gates, docs) is a pass recorded on this pull request with the reviewed commit; a critical-stage read applies
      because this change adds new exported harness surface and changes `lifecycletest.Run`.
- [ ] 7.2 (D) `task verify` green on the final commit; the integration lane green under the host lock, evidence
      directory attached to this pull request; `implemented-by:` in the pull request body.
- [ ] 7.3 (W) The change archived under `openspec/changes/archive/` as the last content commit before squash merge;
      in that commit `openspec/specs/background-work/spec.md`, `process-host/spec.md`, `transport-client/spec.md`,
      `metric-registry/spec.md` and `message-codec/spec.md` each carry a real `## Purpose` in place of the placeholder
      `openspec archive` writes, which OpenSpec 1.13.2's strict validation rejects (PR #48 comment
      5951371629, item 2); `task spec:check` passes on that commit; `task spec:queue` shows no open hold.
