# Tasks: config-key-letter-case

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit, or which mutant it detects. A review stop (Codex) comes after 3.1; the change is small enough
for one. Each task ends only when `task verify` passes on a clean tree at its commit.

## 1. Design

- [x] 1.1 Proposal, design and spec delta pass pre-owner review and are accepted by the owner. Gate:
  `task spec:check` and `task docs:check`.

## 2. Refusal (D1-D3)

- [x] 2.1 Tests first, in `config/load_test.go`, through both loaders:
  - D3's refused cases, each error holding the path as written and as declared;
  - D3's accepted controls;
  - the coverage test over every declared key path;
  - the guard that `Config` reaches no slice or array of structs or maps, no embedded struct, and no other
    `json.Unmarshaler`.

  Gate: the refused cases and the coverage test fail on the base; record how in `notes/2.1.md`.
- [ ] 2.2 `rejectCaseVariantKeys` in `config/config.go` per D1, called from `loadRawJSONFromBytes` after the two
  named refusals (D2). Gate: `go test -race ./config/` passes, with no edit to an existing test; `task verify`
  passes on a clean tree.
- [ ] 2.3 Sensitivity: `task mutate:check` detects D3's mutants (a)-(d), recorded in `notes/2.3.md`. Gate:
  `task verify` passes on a clean tree.

## 3. Docs (D4)

- [ ] 3.1 `config/README.md` and `config/doc.go` per D4. Gate: `task verify` passes on a clean tree, including
  `docs:check` and the README example test (review stop).

## 4. Verify, review and close

- [ ] 4.1 `task verify` passes on a clean tree at the implementation head. A Codex review of record is on the PR
  (`reviewed-by:`). The PR body carries `Closes #325` and `implemented-by:`. The change is archived and the
  `config-loading` spec synced as the last content commit.
