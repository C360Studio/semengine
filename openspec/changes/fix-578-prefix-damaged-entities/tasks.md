# Tasks: fix-578-prefix-damaged-entities

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md` or `task mutate:check`. Premises are in `inventory.md` and `design.md`. One pull request: #655,
which closes #578. #656, #657 and #658 stay open.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`, spec deltas). Needs INVENTORY PASS, a
  passing pre-owner design review, and the owner filling `Approved-by:`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 graph-ingest, tests first. Write the design's graph-ingest tests: damaged entity listed, exact prefix,
  first key too large, the refusal-code cases, and the generated cursor walk with its seed and completed runs
  recorded. Then `graph.PrefixIDsResponse`, the shared listing function, and the `prefixIDs` handler in
  `queryHandlers` (design D1). The prefix verb's tests pass untouched, including `poison_scoping_test.go:296-302`
  and `:432-434`. `notes/2.1.md` shows each new test failing on today's code.
- [ ] 2.2 Table and forward. Add the two rows (design D2) and graph-query's forward entry. Update the tests that
  list rows (inventory § 3) and `composition/reserved_test.go:134-135`'s comment. Update the doc comments of
  `QueryVerbs` and `PrefixQueryRequest`.
- [ ] 2.3 fusionnats, tests then code (design D3). Write the unit tests, then
  `TestDamagedEntityUnderPrefixOverRealResponders`, shown failing on `main`.
- [ ] 2.4 Docs:
  - Gap statements: `pkg/fusion/README.md:120-121`, `pkg/fusion/MIGRATION.md:89-90` and `:133`.
  - Verb lists: `pkg/fusion/README.md:101-109`, `pkg/fusion/fusionnats/doc.go:11-19`, `graph/README.md:48-55`,
    `processor/graph-ingest/README.md:108-114`, `processor/graph-query/README.md:26`, `processor/graph-query/doc.go:9`,
    `docs/repository-map.md:90`, `processor/graph-ingest/README.md:19`.
- [ ] 2.5 Before the PR is marked ready: post on #578 what counts as done (the listing lets a caller narrow the read
  and find the damaged entity; the full prefix page keeps refusing by ruling), and make the PR title and body match
  the change.

## 3. Close

- [ ] 3.1 Codex review of record at the head; `reviewed-by:` in the PR body.
- [ ] 3.2 Archive the change and sync the specs as the last content commit.
