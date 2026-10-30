# Tasks: setup-04a-04-graph-roots-query

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171). One task is one developer's unit.

## 1. Design and measurement

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review; owner
  acceptance. Gate: `task spec:check` and `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval
- [ ] 1.2 Owner answers on the design's four questions (`UpgradeVectors` dropped; `NewEmbeddingClassifier`'s new
  signature; hint types left to the classifier follow-up; `located_in` dropped here), recorded on PR #475.
- [ ] 1.3 File the two issues of design D9 (S13; the keyword tier's inputs), linked from PR #475 without a closing
  keyword. Gate: each issue
  read back.

## 2. Tests first

- [ ] 2.1 Port the pin's keyword, chain, example, LLM-parse and loader tests less those of dropped surface
  (`TestUpgradeVectors*`, `TestKeywordClassifier_InferStrategy_Aggregation`, `TestLoadAllDomainExamples_*`,
  `TestCosineSimilarity`), with no `time.Sleep` (two of the pin's four go with `UpgradeVectors`, two become an
  already-ended context); record each dropped or rewritten test in `notes/2.1.md`. Gate: in 3.1.
- [ ] 2.2 The new tests of design D8 (`TestChainEndedContextReturnsNil`, `TestChainLLMFailureGivesDefault`,
  `TestChainKeywordHintStopsTheLadder`,
  `TestLLMClassifierRefusesNilContext`, `TestNewEmbeddingClassifierRefusesEndedContext`,
  `TestFindBestMatchEndedContext`, `TestResultEditsDoNotReachClassifier`, `TestZoneQueryAddsNoPredicate`,
  `TestEmbeddingClassifierAsAProduct`), each changed behaviour shown failing
  against the pin's shape in `notes/2.2.md`; `TestQueryImportsNoProviderModule` in `internal/harness/contract`
  (the embedding guard's helper on `./graph/query`, its own requirement name).
  Gate: in 3.1.

## 3. The package

- [ ] 3.1 `graph/query` less the adapter (design D1-D5): `NewEmbeddingClassifier` with a context and an error, the
  BM25 embedder held directly, `embedding.CosineSimilarity`, D4's outcomes, copies on every result, no `located_in`,
  dead surface not carried. Gate: `go test -race -count=1 ./graph/query`, `go test -count=5 -cpu 1 -shuffle=on ./graph/query`,
  `go test ./internal/harness/contract`; `go mod tidy` changes nothing.

## 4. Ledger, records and docs

- [ ] 4.1 Ledger rows: `graph/query` (`adapt`; contract design D2-D5; dependencies `graph/embedding` and `pkg/errs`,
  nats.go through `graph/embedding`; no goroutine, no provider; known risks: `Generate` learns from queries until
  #96's follow-up, hint types by tier (S13), host clock and unbounded numbers (S14, S15)) and
  `graph/query/classifier_llm_adapter.go` (`defer-exclude`, naming #32; its test, the live test and
  `DefaultClassificationTimeout` go with it). Gate: `task ledger:check`.
- [ ] 4.2 Docs (design D6): package doc (what it classifies, tiers 0, 1, 3, no store); the chain doc states design
  D4, including the LLM failure; `EmbeddingClassifier`: BM25 only, examples fixed; `LLMClassifier`: an unknown
  strategy and an unparsable time range are dropped (S8c); `README.md`: the framework API, hint names with each
  value's type per tier (P4), nested `Options` values not copied, and that hints PR 9 does not read still stop the
  ladder at tier 0. ADR-084 and ADR-085 port notes: "`graph/query`" becomes "`graph/query`'s client (retired before
  the pin)". A `docs/repository-map.md` row. Gate: `task docs:check`.

## 5. Evidence and review

- [ ] 5.1 `task verify` passes on a clean tree at the last commit before the archive commit; notes linked from a PR
  comment.
- [ ] 5.2 Cross-agent review stop; `reviewed-by:` in the PR body. The body retires no issue and names #96 as advanced.
- [ ] 5.3 Comment on #96 ("context half implemented in PR #475 at `<SHA>`, pending merge"), read back.

## 6. Close

- [ ] 6.1 Archive (`openspec archive setup-04a-04-graph-roots-query`) as the last content commit, syncing
  `query-classification` into `openspec/specs/`. Gate: `task spec:check` and `task docs:check` on the archive commit.
