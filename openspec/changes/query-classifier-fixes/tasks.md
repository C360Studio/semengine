# Tasks: query-classifier-fixes

Each task names its outcome and the gate that proves it. Tests come first; each failing test is shown failing on
main (`d80acfd`) in `notes/<task>.md` before its fix. Each step ends when `task verify` passes on a clean tree at its
commit (#171). One task is one developer's unit.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review; owner
  acceptance and the answer to design question 2 (question 1 answered: host zone, #479 comment 6525906611). Gate: `task
  spec:check`, `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Read-only query embedding (#96, design D3)

- [ ] 2.1 `TestQueryEmbeddingIsReadOnly` (scores and vectors before thresholding, with and without intervening
  queries; construction still learns), failing on main (premise `classifier_embedding.go:104`; inventory Q2).
- [ ] 2.2 `FindBestMatch` uses `GenerateQuery`; doc and README "Limits" line updated. Gate:
  `go test -race -count=1 ./graph/query`.

## 3. Typed tier-1 hints (#478, design D4)

- [ ] 3.1 `TestTierOneHintsAreTyped` and `TestEveryHintNameIsTyped` (the test walks the `SearchOptions` JSON tags by
  reflection, so a field without a `case` fails; unknown names unchanged; each refusal of design D4, naming domain,
  example, hint), failing on main (premise `classifier_chain.go:89`, `examples.go:43`). Rewrite
  `TestFindBestMatch_PreservesOptions` (`"time_range": "week"`), recorded in notes.
- [ ] 3.2 `NewEmbeddingClassifier` decodes and copies; README hint table loses its tier-1 column. Gate: as 2.2.

## 4. Field binding and ordinary phrases (#94, design D1)

- [ ] 4.1 `TestAggregationFieldBinding` and `TestOrdinaryAggregationPhrases` through `ClassifierChain.ClassifyQuery`
  (R14, R17, R21, R23, R24, R26, R27, exact type and field; the descriptive mentions of design § Proving tests
  activate nothing), failing on main (premise `classifier.go:57-77`; inventory Q1).
- [ ] 4.2 Patterns and field binding. Existing `TestKeywordClassifier_*` pass unchanged. Gate: as 2.2.

## 5. Exclusions (#95, design D2; after 4)

- [ ] 5.1 `TestKeywordExclusions` (R28-R31 and the controls) and `TestChainKeepsExclusions` (design § Proving
  tests: `FindBestMatch` shows the unwanted example is eligible; the full chain gives the default result and never
  calls the fake LLM; affirmative controls give tier 0), the chain part failing on main (premise
  `classifier_chain.go:69-102`).
- [ ] 5.2 `FuzzKeywordClassify` with the two laws of D2's spec requirement; seed, run count and any failure in
  notes. Gate: `task mutate:check` on one exclusion rule, survivors reported as survivors.
- [ ] 5.3 Keyword exclusions and the chain rule; doc comments state D2's table. Gate: as 2.2.

## 6. Clock and bounds (#479 items 1-2, design D5-D6)

- [ ] 6.1 `TestTimeRangesFromOneInstant` (fixed instants and zones, week and month edges, a DST day) and
  `TestOutOfRangeNumbersAreRefused` (inventory Q3 cases: no result and an invalid-input error naming the phrase;
  pole clamp answers), the second failing on main (premise `classifier.go:223-357`). The temporal tests that assert
  `NotNil` only gain exact values.
- [ ] 6.2 One `now` per question; bounds and refusal as D6; existing chain callers in the tests take the error.
  Gate: as 2.2 and `go test -count=5 -cpu 1 -shuffle=on ./graph/query`.

## 7. Records and docs

- [ ] 7.1 Ledger row `graph/query`: contract and `known_risks` say what ships (D1-D6). README: hint table, zone,
  bounds, exclusion rule and its limits; package doc. Gate: `task ledger:check`, `task docs:check`.

## 8. Evidence and review

- [ ] 8.1 `task verify` passes on a clean tree at the last commit before the archive commit; notes linked from a
  PR comment.
- [ ] 8.2 Cross-agent review; `reviewed-by:` in the PR body; body says `Closes #94`, `Closes #95`, `Closes #96`,
  `Closes #478`, and names #479 without a closing keyword.
- [ ] 8.3 Comment on #479 that PR #480 at head `<SHA>` (the reviewed head) carries items 1 and 2, and that
  item 3 stays with change 4 PR 9. Read back.

## 9. Close

- [ ] 9.1 Archive (`openspec archive query-classifier-fixes`) as the last content commit, syncing
  `query-classification`. Gate: `task spec:check`, `task docs:check`.
