# Tasks: fix-578-prefix-damaged-entities

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is failing-first output in
`notes/<task>.md` or `task mutate:check`. Premises cite `inventory.md` and `design.md`. One pull request (#655,
`Closes #578`).

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`, spec deltas); INVENTORY PASS, pre-owner design
  review passes, owner fills `Approved-by:`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 graph-ingest tests first (`poison_scoping_test.go`, `query_prefix_unit_test.go`): the cases in design
  "Proving tests" for graph-ingest, plus the generated cursor-walk check against a sorted-key model (record seed and
  completed checks). Replace the prefix halves of the two named tests; the batch halves are unchanged.
  `notes/2.1.md` shows each failing on today's code.
- [ ] 2.2 `graph`: add `PrefixQueryResponse.Missing` and update the doc comments of `PrefixQueryResponse` and
  `MissingError`. graph-ingest: the prefix read lists damaged keys, the fit covers them, and other read errors win
  (design D1). Batch behaviour is unchanged: its tests pass untouched.
- [ ] 2.3 fusionnats test then code: damaged prefix IDs become seeds (design D2); fusion over a reply with `missing`
  answers with the rest.
- [ ] 2.4 graph-query and graph-gateway tests then code: a page with `missing` is refused (design D2).
- [ ] 2.5 Docs: `processor/graph-ingest/README.md:114-120`, `processor/graph-query/MIGRATION.md` (direct
  `graph.query.prefix` readers read `missing`), `gateway/graph-gateway/README.md:34`.

## 3. Close

- [ ] 3.1 Codex review of record at the head; `reviewed-by:` in the PR body.
- [ ] 3.2 Archive the change and sync the specs as the last content commit.
