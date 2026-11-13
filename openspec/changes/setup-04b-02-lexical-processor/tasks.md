# Tasks: setup-04b-02-lexical-processor

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is `task mutate:check` or
failing-first output in `notes/<task>.md`. Premises cite `inventory.md` (INVENTORY PASS at `5582eb9e`, pin fixed in
`f7a1b001`) and `design.md`.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`): placement, mechanism, the text rule, the verb,
  readiness and bodies, the word-record extension point, memory, smoke. Pre-owner design review passes; the owner
  fills `Approved-by:`. Hold: owner approval (OPEN-Q)

## 2. Code

- [ ] 2.1 `graph/entitytext`, and graph-embedding onto it (design D3, D5). Premise: four text rules, graph-embedding's
  at `processor/graph-embedding/hop1.go:228` with defaults `config.go:27` and cap `config.go:23`, its body read at
  `graph/embedding/worker.go:744` with no deadline (inventory §2a; #551); `storage` reaches no NATS module
  (`go list -deps ./storage | grep -c nats-io` is 0). First, a probe test in `storage/objectstore` records whether
  its read returns when its context ends at the pinned nats.go (`notes/2.1.md`); D5's sentence on it is set from
  the result. Accept: `TestEmbeddingTextUnchanged` written and passing against today's extractor before the move,
  then after; `TestNameAndBodyRoles`, `TestBodyReadDeadline` (synctest), `TestEntityTextImportsNoNATS` and its
  sensitivity check pass; graph-embedding's existing tests pass unchanged; a mutant that marks every value Body is
  shown to fail. PR body: `Closes #551`.
- [ ] 2.2 Index memory before real data (design D7, #623). Premise: about 60 B per held word, 9.5 KB per entity at
  157 words (#623; inventory §2c); the word-number layout measured 472 B per 20-word entity
  (`openspec/changes/archive/2026-11-12-setup-04b-01-lexical-index/notes/probes/sequence-probe.log`). Accept: the
  harness from #623 run before and after at 50,000 entities, and once more at SemSource's passage size (about 300
  words), with heap, per word, build time and the four search times in `notes/2.2.md`; every `graph/lexical` test,
  the generated checks and `TestScorecardHoldsItsFloor` pass unchanged; design D7 records the measured figures and
  the entity count and text size they are accepted for. PR body: `Closes #623`.
- [ ] 2.3 The component (design D1, D2, D5 apply path). Premise: the follower plug-in contract and the Value-apply
  rule (`graph/follower/follower.go:62-88`; `openspec/specs/entity-state-follower/spec.md:204-205`); the
  graph-clustering shape (`processor/graph-clustering/component.go:192-203`, `:276`); `lexical.Index` holds no
  revision (`graph/lexical/index.go:80-84`); the shell and run-context rules (inventory §6). Accept:
  `TestRestartRebuildsSameAnswers`, `TestOlderDeliveryIgnored`, `TestDeleteRemovesEntity`,
  `TestConfigRefusesUnknownKey`, `TestIndexGauges`, the `lifecycle-suite` run through the component's adapter, and
  `TestGraphLexicalImportsNoProviderModule` (added to `internal/harness/contract/embeddingprovider_test.go`, with
  the sensitivity loop) pass; `readiness.KeyGraphLexical` is declared; a mutant that drops the revision guard and
  one that calls `Complete` before `Put` are shown to fail.
- [ ] 2.4 Offloaded bodies (design D5). Premise: SemSource offloads every passage and code body (inventory §9);
  graph-embedding's unregistered-store path counts and warns once (`processor/graph-embedding/hop1.go:214`).
  Accept: `TestUnreadBodyCountedThenRepaired` and `TestHungStoreBounded` (integration, against an objectstore
  component on the shared NATS server) pass; the envelope names `body_unread`; the verb is not withheld by it.
- [ ] 2.5 The verb (design D4). Premise: subjects only in `graph/query_verbs.go` and the guard reads the table
  (inventory §3); graph-index's `min_revision` order (`openspec/specs/graph-index/spec.md:240-245`). Accept: the row,
  `graph.LexicalSearchRequest`, `graph.LexicalSearchData` and `graph.LexicalHit`; `TestSearchWithheldUntilBuilt`,
  `TestSearchMinRevisionBoundary`, `TestSearchScopeLimitMatched`, `TestSearchLimitRange`, `TestSearchRefusesParseErrors`,
  `TestQueryVerbsDeclareEachRespondersVerbs` (extended) and a served-exactly-its-verbs test pass; `AGENTS.md` § 2 and
  `.agents/rules-index.md` name `graph.lexical.query.*` in the reserved-subject row, and `task ateam:doctor` passes.
- [ ] 2.6 Rebuild cost (design D5, D6; open question 1). Premise: rebuild time with offloaded bodies is not measured
  (inventory §10). Accept: a probe, its source and log in `notes/2.6.md`, starts the component over 20,000 and
  100,000 entities on one NATS server, inline and with bodies in an object store, and records time to first
  answer; the result is compared with the budget the owner set and posted on #10.
- [ ] 2.7 Smoke (design D8; #628 first item). Premise: the composition lists, factories and `indexKeys`
  (`internal/smoke/stack_integration_test.go:150-152`, `:177-185`; inventory §4). Accept: `TestSmoke_OneConfigAnswers`
  and `TestSmoke_StackRestartKeepsAnswers` hold the `composed-stack` deltas; a mutant that skips the delete in the
  component is shown to fail the smoke check; the graph-only and no-embedder tests pass unchanged. PR body:
  `Addresses #628`.

## 3. Land

- [ ] 3.1 Docs (design D9). Premise: `graph/lexical/README.md:9-11` says no component fills an index; its "Who word
  search helps" section. Accept: `processor/graph-lexical/README.md` states the verb, config, the body rule, memory
  and restart cost, and says in plain words that sensor or drone telemetry and other entities without text gain
  nothing from word search and are reached by structure; `graph/lexical/README.md` and the `lexical-search` purpose
  line name the component; `processor/graph-embedding/README.md` says its text comes from `graph/entitytext`;
  `docs/repository-map.md` and `scripts/cover-check.sh` gain the two packages; `docs/setup-plan.md:429-430` only if
  the owner rules yes on open question 3. The adoption list (design D3) is filed as one issue.
- [ ] 3.2 Archive and spec sync.
