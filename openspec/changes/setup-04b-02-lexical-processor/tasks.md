# Tasks: setup-04b-02-lexical-processor

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is `task mutate:check` or
failing-first output in `notes/<task>.md`. Premises cite `inventory.md` (INVENTORY PASS at `5582eb9e`, pin fixed in
`f7a1b001`), `design.md` and `notes/probes/`. Each code task names its pull request (design D10): A, B or C.

## 1. Design

- [x] 1.1 Architect inventory and design (`inventory.md`, `design.md`, `notes/probes/`): placement, mechanism, the
  text rule, the verb, readiness and bodies, the word-record extension point, memory, smoke. Pre-owner design review
  passes; the owner fills `Approved-by:`. Approved 2026-11-13 (#10 comment 6852149945): first-build budget 60 s for
  32,000 offloaded entities, default cap 16,000 characters.

## 2. Code

- [x] 2.1 PR A. `graph/entitytext`, and graph-embedding onto it (design D3, D5). Premise: four text rules,
  graph-embedding's at `processor/graph-embedding/hop1.go:228` with defaults `config.go:27` and cap `config.go:23`,
  its body read at `graph/embedding/worker.go:744` with no deadline (inventory §2a; #551); `storage` reaches no NATS
  module (`go list -deps ./storage | grep -c nats-io` is 0); the NATS object store's read returns at its context's
  deadline at nats.go v1.55.0, with `nats: timeout`, which wraps no context error
  (`notes/probes/objectstore-watch-probe.log`, A1 and A2); a body read failure fails the record `content_error`
  (`graph/embedding/worker.go:417-421`). The fakes in `TestBodyReadDeadline` and `TestHungStoreFreesWorkerLane`
  wait for their context, then return an error that wraps no context error (A2's shape). Accept:
  `TestEmbeddingTextUnchanged` written and passing against today's extractor before the move, then after;
  `TestNameAndBodyRoles`, `TestDocumentCap`, `TestBodyReadDeadline` (synctest), `TestBodyReadOutcomes`,
  `TestHungStoreFreesWorkerLane` (synctest), `TestEntityTextImportsNoNATS` and its sensitivity check pass;
  graph-embedding's existing tests pass unchanged; a mutant that selects label predicates in `Values`, one that
  cuts Name, and one that decides the deadline with `errors.Is(err, context.DeadlineExceeded)` are each shown to
  fail. PR body: `Closes #551`.
- [x] 2.2 PR B. Index memory before real data (design D7, #623). Premise: about 60 B per held word, 9.5 KB per
  entity at 157 words (#623; inventory §2c); the word-number layout measured 472 B per 20-word entity
  (`openspec/changes/archive/2026-11-12-setup-04b-01-lexical-index/notes/probes/sequence-probe.log`); SemSource's
  code bodies average about 600 characters (`notes/probes/body-size-probe.log`). Accept: the harness from #623 run
  before and after at 50,000 entities, and once at SemSource's sizes, with heap, per word, build time and the four
  search times in `notes/2.2.md`; every `graph/lexical` test, the generated checks and
  `TestScorecardHoldsItsFloor` pass unchanged; the figures, and the entity count and text size they are accepted
  for, are in `notes/2.2.md`. PR body: `Closes #623`.
- [ ] 2.3 PR C. The component and its apply rule (design D1, D2). Premise: the follower plug-in contract and the
  Value-apply rule (`graph/follower/follower.go:62-88`, `:75-76`;
  `openspec/specs/entity-state-follower/spec.md:204-205`); the graph-clustering shape
  (`processor/graph-clustering/component.go:192-203`, `:276`); `lexical.Index` holds no revision
  (`graph/lexical/index.go:80-84`); the shell and run-context rules (inventory §6). Generated checks are used, not
  examples alone, because two paths write one key and the order of their outcomes is the risk (design D2). Accept:
  `TestStaleBodyReadDropped` (the named witness, the floor passing the dropped revision), then
  `TestPropApplyMatchesModel` against the reference model, which also checks that every delivered revision
  completes, seed and completed checks in `notes/2.3.md`; `TestRestartRebuildsSameAnswers`, `TestOlderDeliveryIgnored`,
  `TestDeleteRemovesEntity`, `TestConfigRefusesUnknownKey`, `TestIndexGauges`, the `lifecycle-suite` run through
  the component's adapter, and `TestGraphLexicalImportsNoProviderModule` (added to
  `internal/harness/contract/embeddingprovider_test.go`, with the sensitivity loop) pass;
  `readiness.KeyGraphLexical` is declared; a mutant that applies a body read whatever the key's revision, one that
  drops the delivery guard, one that calls `Complete` before `Put`, and one that never completes a dropped read's
  revision are each shown to fail.
- [ ] 2.4 PR C. The body path (design D5). Premise: SemSource offloads every passage and code body (inventory §9);
  a missing object is `storage.ErrObjectNotFound`, not transient (`storage/objectstore/store.go:148-153`); store
  providers start and register before other components (`service/component_manager_lifecycle.go:37-54`, `:123`);
  `Failed` above 0 reads degraded (`graph/follower/follower.go:508-512`). Accept: `TestBlockingStoreBoundsFirstBuild`
  (10,000 entities, synctest), `TestUnreadBodyCountedThenRepaired`, `TestMissingAndUnresolvedBodies` and
  `TestBodyCutAtCap` pass; `TestUnreadBodyCountedThenRepaired` and `TestMissingAndUnresolvedBodies` also run as
  integration tests against an objectstore component on the shared NATS server, while the blocking case stays a
  unit test with a store that waits for its context and then returns its own error (A2's shape), since no harness
  holds the server down (#574); a mutant that keeps reading a stalled store, one that stalls only on a context
  error, and one that retries a missing body are each shown to fail. `TestBodyCutAtCap` checks the gauge and the
  one warning line.
- [ ] 2.5 PR C. The verb (design D4). Premise: subjects only in `graph/query_verbs.go` and the guard reads the table
  (inventory §3); graph-index's `min_revision` order (`openspec/specs/graph-index/spec.md:240-245`). Accept: the
  row, `graph.LexicalSearchRequest`, `graph.LexicalSearchData` and `graph.LexicalHit`;
  `TestSearchWithheldUntilBuilt`, `TestSearchMinRevisionBoundary`, `TestSearchScopeLimitMatched`,
  `TestSearchLimitRange`, `TestSearchRefusesParseErrors`, `TestQueryVerbsDeclareEachRespondersVerbs` (extended) and
  a served-exactly-its-verbs test pass; `AGENTS.md` § 2 and `.agents/rules-index.md` name `graph.lexical.query.*` in
  the reserved-subject row, and `task ateam:doctor` passes.
- [ ] 2.6 PR C. First-build cost (design D5, D6; open question 1). Premise: a healthy read takes about 0.46 ms
  (`notes/probes/objectstore-watch-probe.log`); 32k offloaded entities come to about 15 s of reads by arithmetic.
  Accept: a probe, its source and log in `notes/2.6.md`, starts the component over 20,000 and 100,000 entities on
  one NATS server, inline and with bodies in an object store, and records time to the first answer; the result is
  compared with the budget the owner set and posted on #10.
- [ ] 2.7 PR C. Smoke (design D8; #628 first item). Premise: the composition lists, factories and `indexKeys`
  (`internal/smoke/stack_integration_test.go:150-152`, `:177-185`; inventory §4); the running-broker test counts
  seven components (`openspec/specs/composed-stack/spec.md:77-90`). Accept: `TestSmoke_OneConfigAnswers`,
  `TestSmoke_StackRestartKeepsAnswers`, `TestSmoke_BrokerRestartBetweenRuns` and
  `TestSmoke_BrokerRestartUnderRunningStack` hold the `composed-stack` deltas (eight components, graph-lexical's key,
  no lost feed); `TestSmoke_KillMidIngestKeepsAcknowledgedWrites` waits on graph-lexical's key through `indexKeys`,
  its requirement unchanged because it names no keys; a mutant
  that skips the delete in the component is shown to fail the smoke check; the graph-only and no-embedder tests pass
  unchanged. PR body: `Addresses #628`.

## 3. Land

- [ ] 3.1 PR C. Docs (design D9). Premise: `graph/lexical/README.md:9-11` says no component fills an index; its "Who
  word search helps" section. Accept: `processor/graph-lexical/README.md` states the verb, config, the four body
  states, the cap and its cut gauge, file paths, memory and restart cost, and says in plain words that sensor or
  drone telemetry and other entities without text gain nothing from word search and are reached by structure;
  `graph/lexical/README.md` and the `lexical-search` purpose line name the component;
  `processor/graph-embedding/README.md` says its text comes from `graph/entitytext`; `docs/repository-map.md` and
  `scripts/cover-check.sh` gain the two packages; `docs/setup-plan.md:429-430` says SemSource's mainline builds on
  SemEngine (owner ruling on PR #629). The adoption list (design D3) is filed as one issue.
- [ ] 3.2 PR C. Archive and spec sync.
