# Tasks: setup-04b-01-lexical-index

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is `task mutate:check` or
failing-first output in `notes/<task>.md`. The design may split or reorder the code tasks.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`): the package's surface, the tokenizer's rules,
  how a phrase is checked, the name weight, the scorecard's corpus and floor, and the four extension points the
  proposal names. Pre-owner design review passes; the owner fills `Approved-by:`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 Tokenizer: lower-casing, word splitting (code identifiers included), stopwords, plural folding.
- [ ] 2.2 Index and BM25 scoring: write, replace and delete an entity; search with scope before limit, ties by ID.
- [ ] 2.3 Query forms: exact phrase and excluded word.
- [ ] 2.4 Scorecard: the fixed corpus and questions, the floor, and the generated checks of the laws.

## 3. Land

- [ ] 3.1 `graph/lexical/README.md` and `docs/repository-map.md`.
- [ ] 3.2 Archive and spec sync.
