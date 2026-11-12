# Tasks: setup-04b-01-lexical-index

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is `task mutate:check` or
failing-first output in `notes/<task>.md`. Premises cite `inventory.md` (passed at `a1783aa1`) and `design.md`.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`): the package's surface, the tokenizer's rules,
  how a phrase is checked, the name weight, the scorecard's corpus and floor, and the four extension points the
  proposal names. Pre-owner design review passes; the owner fills `Approved-by:`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 Tokenizer (`Tokens`, stopwords, plural rules; design D3). Premise: three tokenizers exist and none is
  shared (inventory §2: `graph/embedding/bm25_embedder.go:242`, `graph/clustering/terms.go:28`,
  `processor/graph-query/localsearch.go:60`). The rules were measured in `notes/probes/tokenizer-probe-2.log` and
  `notes/probes/word-shape-probe.log`, which are committed with this task. Accept: `TestTokens`, `TestPluralRules`,
  `TestPropFoldIsIdempotent`, `TestJoinedIdentifierIsNotHeld` and `FuzzTokens` pass; a mutant that skips
  identifier splitting and one that drops rule 1 are each shown to fail.
- [ ] 2.2 Index, BM25F and scope (`NewIndex`, `Put`, `Delete`, `Len`, `Search`; design D1, D2, D6). Premise: no word
  index exists (inventory §1); scope matching is `graph/id_prefix.go:26` and ties go by ID
  (`graph/embedding/storage.go:132`). Accept: `TestPutReplacesAndDeletes`, `TestSearchRanksByBM25`,
  `TestNameOutranksBody`, `TestScopeBeforeLimit`, `TestSearchDuringWrites` (race detector) and the properties
  `TestPropIndexMatchesModel`, `TestPropRankingIgnoresHistory` and `TestPropAbsentWordsMatchNothing`, each with
  named witnesses before `rapid.Check`, seed and completed checks recorded in `notes/2.2.md`.
- [ ] 2.3 Query forms (`ParseQuery`, phrases and exclusion; design D4, D5). Premise: SemSource's R2 and R3
  (inventory §5); per-entity word sequences cost 472 B and 1,590 B per entity at one million
  (`notes/probes/sequence-probe.log`, committed with this task). Accept: `TestParseQuery`, `FuzzParseQuery`,
  `TestPhraseNeedsAdjacentWords`, `TestExcludedWordRemovesEntities`.
- [ ] 2.4 Scorecard and name weight (design D6, D8). Premise: the probes' known-item method
  (`notes/probes/README.md`); the unit-test time budget (`openspec/specs/merge-gate/spec.md:15-19`). Accept: the
  frozen corpus and questions, among them one that pins `withretry` finding nothing, are in
  `graph/lexical/testdata/scorecard/`, with their size and source commit in `notes/2.4.md`; the sweep of `w` over
  1, 2, 3 and 5 is recorded and the chosen value is written into the spec; `TestScorecardHoldsItsFloor` and
  `TestScorecardBehaviour` pass; an IDF-is-one mutant is shown to fail the floor.
- [ ] 2.5 Guards and coverage (design D10). Premise: the graph root guard covers `./graph` only
  (`internal/harness/contract/graphtransport_test.go:18`). Accept: `TestLexicalImportsNoNATS` and its sensitivity
  test pass in `internal/harness/contract`; `"graph/lexical unit"` is in `scripts/cover-check.sh` and
  `task cover:check` passes.

## 3. Land

- [ ] 3.1 `graph/lexical/README.md` with `ExampleIndex`, including what word search does not help with and where those
  entities are reached (design D9; inventory §4), and `docs/repository-map.md`.
- [ ] 3.2 Archive and spec sync.
