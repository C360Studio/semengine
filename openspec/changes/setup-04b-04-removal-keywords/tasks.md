# Tasks: setup-04b-04-removal-keywords

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is `task mutate:check` or
failing-first output in `notes/<task>.md`. Premises cite `inventory.md` (INVENTORY PASS at d797b547), `design.md`
and `notes/probes/`. Each code task names its pull request (design D10): A, B, C, then D, all after #660.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`, `notes/probes/keywords-probe.*`). Pre-owner
  design review passes; the owner fills `Approved-by:`, rules on design questions 1 to 3, and approves the changed
  ADR decisions named in the proposal. Hold: OPEN-Q proposal awaits owner approval.

## 2. Code

- [ ] 2.1 PR A. The example classifier goes (design D1). Premise: `EmbeddingClassifier` has no production caller and
  its constructor builds the hashed embedder (`graph/query/classifier_embedding.go:27,47,71`); the chain holds it as
  tier 1 (`classifier_chain.go:30,37,126`); graph-query passes `nil` (`processor/graph-query/globalsearch.go:326`).
  Accept: new `TestChainAnswersFromFirstTier` and `TestChainOptionsAreCopies` (an edit to a returned `Options` map
  changes no later result); `TestChainKeepsExclusions` (LLM tier only) and `TestChainEndedContextReturnsNil` pass;
  new `TestQueryImportsNoEmbedding` lists no `graph/embedding` under
  `go list -deps ./graph/query`; `classifier_embedding.go`, its two test files, `Example.Vector` and the tier 1
  scenarios are deleted; `graph/query/README.md`, `doc.go` and the `graph/query` ledger row (`:3927-4000`) say what
  the chain is now.
- [ ] 2.2 PR B. `nl` asks the word index (design D2). Premise: fusionnats sends `nl` to graph-query's `semantic` row
  and looks rows up among graph-query's only (`pkg/fusion/fusionnats/client.go:30-55,151-177`); the word index takes
  `query`, `limit` and `scope` (`graph/query_lexical_types.go:10-15`). Accept: new `TestNLResolveAsksTheWordIndex`
  (subject from graph-lexical's row, the request's bytes, seeds in hit order with their scores), new
  `TestNLWordIndexRefusalReturned` (an invalid refusal from the word index reaches the caller with class, code and
  detail), new `TestNLIndexNotReadyDefers` and new `TestNLResolveWithoutWordIndex` (integration: no graph-lexical,
  transient `internal` naming `graph.lexical.query.search`);
  `TestVerbLookupNamesMissingRow` refuses a table without graph-lexical's `search`; `TestSubjectsComeFromVerbTable`
  updated; the integration test resolves `nl` against a running graph-lexical (`client_integration_test.go:116`
  composes graph-lexical, not graph-embedding).
- [ ] 2.3 PR B. What an `nl` answer reports, and its provenance (design D3, D4; owner question 2). Premise: the counts
  are the embedding cache's (`pkg/fusion/contract.go:180-186`, `retrieval.go:66-73`); `nl` is `embedding`
  (`lens.go:195-204`). Accept: `TestNLAnswerCarriesWordIndexCounts` (`bodies_not_indexed` and `excluded` beside the
  nodes and on a miss), new `TestWordIndexCountsReachResolution`, `TestSymbolAnswerOmitsCounts`,
  `TestProvenanceForMode`; smoke's word question
  (`internal/smoke/smoke_integration_test.go:563-573`) wants provenance `lexical` and the live holders, and the
  no-embedder stack composes graph-lexical and answers it; a mutant mapping `nl` to `embedding` is shown to fail.
  `pkg/fusion/MIGRATION.md`, ADR-062's and ADR-071's port notes and the `pkg/fusion`, fusionnats ledger rows
  (`:4668`, `:4782-4863`) say what changed.
- [ ] 2.4 PR C. graph-embedding takes a product's embedder (design D5; owner question 3). Premise: the component
  builds the hashed embedder itself (`processor/graph-embedding/component.go:87,325`) and accepts only `embedder_type:
  bm25` (`config.go:15,69-71,121-122`). Accept: new `TestRegisterWithoutEmbedderRefuses`, `TestProductEmbedderIsUsed`
  (the registered name is the dedup type and `search`'s `embedder_type`), `TestEmbedderClosedOnStop` (built and never
  started: `newEmbedder` not called; started and stopped: one close; a `Start` failing after the build: closed, or
  kept and closed by `Stop`); `TestConfigRefusesUnknownKey` refuses `embedder_type`; the lifecycle suite and the 33
  integration tests pass on a test embedder registered through `RegisterWithEmbedder`; `TestEmbeddingTextUnchanged`
  unchanged.
- [ ] 2.5 PR C. The hashed embedder is deleted (plan item 4). Premise: its only callers are graph-embedding and the
  example classifier (inventory §4), gone after 2.1 and 2.4. Accept: `bm25_embedder.go` and its test are deleted;
  `TestEmbeddingImportsNoProviderModule` and the `graph/embedding merged` floor (`scripts/cover-check.sh:54`) pass;
  `doc.go`, `embedder.go`, `dedup.go:25-41` and `README.md:25` name no BM25 embedder.
- [ ] 2.6 PR C. `embedding_enabled` reads false when left out (design D6). Premise: `processor/graph-query/
  component.go:69-70`. Accept: `TestConfigEmbeddingEnabled` (left out reads false; true still forwards),
  `TestNoRespondersIsTransient` with the key set true.
- [ ] 2.7 PR C. Smoke on tier 0 (design D7; #628). Premise: smoke composes graph-embedding with `bm25`
  (`internal/smoke/stack_integration_test.go:71,90-91,152,182,734`) and asserts it (`smoke_integration_test.go:544`).
  Accept: `TestSmoke_OneConfigAnswers`, `TestSmoke_StackRestartKeepsAnswers`,
  `TestSmoke_KillMidIngestKeepsAcknowledgedWrites`, `TestSmoke_BrokerRestartBetweenRuns` and
  `TestSmoke_BrokerRestartUnderRunningStack` pass on seven components, `TestSmoke_OneConfigAnswers` running the
  symbol, prefix and unknown-name fusion checks from `noembedder_integration_test.go:134,146,174` unchanged; `git grep
  -n -i "bm25\|semantic\|similar" internal/smoke` finds no check; `noembedder_integration_test.go` is deleted and
  `TestFusionNATSImportsNoEmbedding` passes. PR body: `Closes #628`.
- [ ] 2.8 PR D. Text through `graph/entitytext` (design D9; #650). Premise: `entityWords` reads every string value
  (`graph/clustering/terms.go:57`); graph-clustering has no `text_suffixes` (`processor/graph-clustering/
  config.go:46-53`). Accept: new `TestKeywordTextMatchesLexical`, `TestTextSuffixesIsRead`;
  `TestRepresentativesWithoutScan` and `TestRecordIsBounded` pass; `processor/graph-clustering/MIGRATION.md` says
  to set `text_suffixes` to graph-lexical's list. PR body: `Closes #650`.
- [ ] 2.9 PR D. Distinctive keywords (design D8; owner question 1). Premise: keywords are the most counted words
  (`terms.go:89`), 27.9% of them corpus-common on the probe (`notes/probes/keywords-probe.log`). Accept: new
  `TestKeywordsScorecard` and `TestKeywordsScoredWhenWritten`; `TestTickCost` and the maintainer's property test pass
  unchanged; a mutant that drops the graph-wide count is shown to fail the scorecard. The floors and the first run's
  numbers are recorded in `notes/2.9.md`.

## 3. Docs and close-out

- [ ] 3.1 Each PR updates what it changes: `processor/graph-embedding/README.md`, `MIGRATION.md`;
  `processor/graph-query/README.md`, `MIGRATION.md`; `.agents/skills/query-pattern/SKILL.md:30-35`;
  `docs/repository-map.md:74,76,91`; `docs/contract.md:33`; the ledger rows named in inventory §4; ADR-099's port note
  (PR D); `task docs:check` passes.
- [ ] 3.2 PR D. Sync the specs (the Purpose lines of graph-embedding, embedding-index, query-classification and
  composed-stack name the hashed embedder) and archive the change as the last content commit; `task spec:check`
  passes.
