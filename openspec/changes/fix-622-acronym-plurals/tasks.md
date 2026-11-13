# Tasks: fix-622-acronym-plurals

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is failing-first output
in `notes/<task>.md` or `task mutate:check`. Premises cite `proposal.md` and `notes/probes/`. Single PR (#653,
`Closes #622`).

## 1. Proposal

- [x] 1.1 Architect proposal and spec delta (`proposal.md`, `specs/lexical-search/spec.md`, `notes/probes/`): rule S
  in the splitter, rule F in plural rule 5, with corpus and scorecard counts. `task spec:check` and
  `task docs:check` pass; the owner ruled option (a) and approved 2026-11-13 (`sha256:5ac000fd360e`).

## 2. Code

- [x] 2.1 Tests first (`graph/lexical/tokens_test.go`, and an index test). Premise: `splitIdentifier` splits before
  the last capital of a capital run followed by lower case, so `IDs` gives `i ds` (`graph/lexical/tokens.go`,
  `splitIdentifier`); `fold` rule 5 needs over 3 bytes (`fold`, last case), pinned by the `"its": "its"` witness in
  `TestPluralRules`. Accept: a new test puts one entity per form (`ID`, `id`, `IDs`, `ids`, and the same four for
  URL, API, PR, ADR, UUID) and asserts that a query of each form returns exactly that acronym's four entities; the
  expected sets are written by hand, never computed with `Tokens`. `TestTokens` / `TestPluralRules` gain the
  witnesses `IDs`→`id`, `IDS`→`id`, `URLS`→`url`, `CPUs`→`cpu`, `ids`→`id`, `HTTPServer`→`http server`,
  `EntityIDsByType`→`entity i ds type` (inside a run, unchanged), `bus`→`bus`, `os`→`os` (2 bytes), and rule 5's new
  bound replaces `"its": "its"`. `FuzzTokens` gains `IDs ids URLs` as a seed. `notes/2.1.md` records the new test
  failing on today's code, and failing again with only rule S and with only rule F, so each half is shown needed.
  Generated checks: the laws the change must keep (folding twice equals folding once; tokenizing the joined words
  gives them back) are already `TestPropFoldIsIdempotent` and `FuzzTokens`; the acronym rule adds no new law, so the
  table of examples suffices.
- [x] 2.2 Implement rules S and F in `graph/lexical/tokens.go`; update the `Tokens`, `splitIdentifier` and `fold`
  comments, the sentence in `graph/lexical/doc.go:5-7` on when a query word and an indexed word agree (it must name
  acronym plurals and the merges rule F adds), and the README's "How text becomes words", naming the merges rule F
  adds and stating the length limits in bytes (plain words, no history). Accept: 2.1's tests pass;
  `TestScorecardHoldsItsFloor` logs R@1, R@10 and MRR no lower than 24.6%, 56.0% and 0.356, recorded in
  `notes/2.2.md`; every `TestScorecardBehaviour` question passes; full `task verify` passes on a clean tree.
- [ ] 2.3 Archive: `openspec archive fix-622-acronym-plurals --yes` promotes the delta; `task spec:check` and
  `task docs:check` pass.
