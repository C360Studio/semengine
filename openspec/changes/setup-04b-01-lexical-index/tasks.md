# Tasks: setup-04b-01-lexical-index

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is `task mutate:check` or
failing-first output in `notes/<task>.md`. Premises cite `inventory.md` (passed at `a1783aa1`) and `design.md`.

## 1. Design

- [x] 1.1 Architect inventory and design (`inventory.md`, `design.md`): the package's surface, the tokenizer's rules,
  how a phrase is checked, the name weight, the scorecard's corpus and floor, and the four extension points the
  proposal names. Pre-owner design review passes; the owner fills `Approved-by:`.

## 2. Code

- [x] 2.1 Tokenizer (`Tokens`, stopwords, plural rules; design D3). Premise: three tokenizers exist and none is
  shared (inventory §2: `graph/embedding/bm25_embedder.go:242`, `graph/clustering/terms.go:28`,
  `processor/graph-query/localsearch.go:60`). The rules and their order were measured in
  `notes/probes/tokenizer-probe-2.log`, `word-shape-probe.log` and `tokenizer-law-probe.log`. Accept: `TestTokens`
  (with the `wills does haves ands ares thes fors withs` and long-run witnesses), `TestPluralRules`,
  `TestSingularFindsPlural`,
  `TestPropFoldIsIdempotent`, `TestJoinedIdentifierIsNotHeld` and `FuzzTokens` pass; a mutant that skips identifier
  splitting, one that drops rule 1, and one that tests only the unfolded word against the stopwords are each shown
  to fail.
- [ ] 2.2 Index, BM25F and scope (`NewIndex`, `Put`, `Delete`, `Len`, `Search`, `NameWeight`; design D1, D2, D6).
  Premise: no word index exists (inventory §1); scope matching is `graph/id_prefix.go:26` and ties go by ID
  (`graph/embedding/storage.go:132`). Accept: `TestPutReplacesAndDeletes`, `TestSearchRanksByBM25`,
  `TestNameOutranksBody`, `TestScopeBeforeLimit`, `TestSearchDuringWrites` (race detector), and the properties
  `TestPropIndexMatchesModel` (plain words), `TestPropRankingIgnoresHistory` and `TestPropAbsentWordsMatchNothing`.
  Each property draws from a closed vocabulary (lower case, no `s` or `x`, never ending in `e`, no stopword, so
  tokenizing changes nothing) against a naive BM25F model over a `map[id]Document`. Named witnesses run before
  `rapid.Check`, and the seed and completed checks go in `notes/2.2.md`.
- [ ] 2.3 Query forms (`ParseQuery`, phrases, exclusion and reserved syntax; design D4, D4b, D5, D7). Premise:
  SemSource's R2 and R3 (inventory §5); per-entity word sequences cost 472 B and 1,590 B per entity at one million
  (`notes/probes/sequence-probe.log`); requiring every word reaches 6.5% of known-item answers
  (`notes/probes/and-query-probe.log`). Accept: `TestParseQuery`, `TestParseQueryReservesForms`, `FuzzParseQuery`,
  `TestCombinedQuery`, `TestUnmatchedPhraseFindsNothing`, `TestPhraseNeedsAdjacentWords`,
  `TestPhraseStaysInOneField` and `TestExcludedWordRemovesEntities`. `TestPropIndexMatchesModel` is extended so that
  its model and generated queries carry up to two phrases and two exclusions, with seed and checks in `notes/2.3.md`.
- [ ] 2.4 Scorecard and name weight (design D6, D8). Premise: the probes' known-item method
  (`notes/probes/README.md`); 2,000 sections come to 0.97 MB gzipped (`notes/probes/corpus-size-probe.log`); the
  unit-test time budget (`openspec/specs/merge-gate/spec.md:15-19`). Accept: the frozen corpus and questions, among
  them one that pins `withretry` finding nothing, are in `graph/lexical/testdata/scorecard/`, with their source
  commit in `notes/2.4.md`. The sweep of `NameWeight` over 2, 3 and 5, with 1 for comparison, is recorded there; the
  chosen value is set in the constant and written into design D6. The floor (measured R@10 minus one point) is set
  as a constant in `TestScorecardHoldsItsFloor` and written into design D8. `TestScorecardHoldsItsFloor` and
  `TestScorecardBehaviour` pass, and an IDF-is-one mutant is shown to fail the floor.
- [ ] 2.5 Guards and coverage (design D10). Premise: the graph root guard covers `./graph` only
  (`internal/harness/contract/graphtransport_test.go:18`). Accept: `TestLexicalImportsNoNATS` and its sensitivity
  test pass in `internal/harness/contract`; `"graph/lexical unit"` is in `scripts/cover-check.sh` and
  `task cover:check` passes.

## 3. Land

- [ ] 3.1 `graph/lexical/README.md` with `ExampleIndex`, and `docs/repository-map.md`. Premise: inventory §4 (read
  paths that need no text) and `processor/graph-index/query.go:366` (exact string value match). Accept: the README
  says word search helps only entities with text; telemetry and other numeric data are reached through prefix,
  predicate, relationships, name and alias, and groups by ID; the value filter matches the value written as a
  string, exactly, and nothing offers numeric ranges; the time-range and bounds verbs have no responder in the
  tree. It also states the query rules: a single leading `-` excludes (`go test -race` excludes `race`, while
  `--force` is text; quote the flag to search for it), quoting makes any text literal, which syntax is reserved,
  and that `withretry` finds nothing.
- [ ] 3.2 Archive and spec sync.
