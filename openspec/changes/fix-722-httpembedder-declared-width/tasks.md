# Tasks: fix-722-httpembedder-declared-width

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md`. One pull request (#743), `Closes #722`.

## 1. Proposal

- [ ] 1.1 The owner fills `Approved-by:`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. httpembedder: a declared width

- [ ] 2.1 Tests first, shown failing (`notes/2.1.md`), in `graph/embedding/httpembedder/httpembedder_test.go`:
  - `Config{Dimensions: 3}`: `Dimensions()` is 3 before any request, and the test server sees no request at `New`.
  - A reply of width 3 is accepted; a reply of width 2 is refused with an error naming both widths and returns no
    vectors, and `Dimensions()` stays 3.
  - `Dimensions: -1` is refused by `New`.
  - `Dimensions: 0` keeps the learned-width behaviour (`TestRefusesUntrustedReply` and the existing learned-width
    cases still pass).

  Premises: `httpembedder.go:33-59` (`Config`), `:80-104` (`New`), `:143-146` (`Dimensions`), `:275-304` (width check
  and `CompareAndSwap`).
  Accept: the new cases fail on the base commit for the right reason.
- [ ] 2.2 Implement: the field and its doc comment, the check in `New`, and the configured width stored in
  `dimensions` at construction, so the existing check refuses a mismatched reply. Update the package doc comment
  (`httpembedder.go:1-10`) and the doc comment on `New`, which says the width is learned from the first reply.
  Accept: 2.1 passes; `task verify`.
- [ ] 2.3 A restart keeps stored vectors when the width is declared. Extend the `same-provider` case of
  `TestProviderSwitchRestart` (`processor/graph-embedding/provider_switch_integration_test.go:73`), or add a sibling
  test there with `httpembedder` and a declared width against a fake provider. Run a restart on the same state and
  assert that the provider sees no request for an entity whose stored vector is unchanged. Record in `notes/2.3.md`
  which one was chosen and why.
  Accept: the test fails with the declared width removed (shown in `notes/2.3.md`); `task verify`.

## 3. Docs and spec

- [ ] 3.1 `processor/graph-embedding/README.md:198-205`: tell a product on `httpembedder` to set `Dimensions` to its
  model's width, so a restart keeps every stored vector. Accept: `task docs:check`.
- [ ] 3.2 The delta `specs/embedding-index/spec.md` matches the code. Accept: `task spec:check`.

## 4. Close

- [ ] 4.1 Archive the change and sync the spec as the last content commit. Accept: `task spec:check`,
  `task merge:check -- 743`.
