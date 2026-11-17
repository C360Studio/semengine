# Tasks: fix-686-embedder-input-type

Tests come first. Each code task ends at a clean-tree local `task verify`. Single PR, `Closes #686`.

## 1. Proposal

- [ ] 1.1 Proposal and spec delta (`proposal.md`, `specs/embedding-index/spec.md`); `task spec:check` and
  `task docs:check` pass.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 Tests first (`graph/embedding/httpembedder/httpembedder_test.go`): `TestRequestShape` wants the document
  body exactly `{model, input, input_type: "document"}`; new `TestInputTypeNamesTheText` reads both bodies, and with
  `OmitInputType` true wants neither to carry the field. Shown to fail on main (`notes/2.1.md`).
- [ ] 2.2 Implement in `httpembedder.go`: `Config.OmitInputType`, the request's `input_type` field, the doc comments
  on `Config`, `Generate` and `GenerateQuery`; update `processor/graph-embedding/README.md`, `MIGRATION.md`, the ledger
  row and `docs/repository-map.md`. Accept: 2.1 passes; full `task verify` passes on a clean tree.
- [ ] 2.3 Evidence: `task -x scorecard:tier1` against semembed `c4cfb309` (arctic-embed-s) with no `-query-prefix`;
  the vector row is within a point of the prefixed run's R@10 52.0% (`notes/2.3.md`).
- [ ] 2.4 Archive: `openspec archive fix-686-embedder-input-type --yes`; `task spec:check` and `task docs:check` pass.
