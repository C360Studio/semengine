# Tasks: fix-723-fusion-merge-answer

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md`. One pull request (#744), `Closes #723`.

## 1. Proposal

- [x] 1.1 The owner fills `Approved-by:`.

## 2. The merge answer on the response

- [x] 2.1 Tests first, shown failing (`notes/2.1.md`):
  - `pkg/fusion/fusionnats/client_test.go`, beside `TestMergedResolveProvenance`: a reply with an honoured `merge`
    (`detail` `merged`) fills `Resolution.Merge` with it; a reply with a `merge` not honoured fills `honoured: false`
    with its `detail`, `reason` and `code`; a reply with no `merge` leaves `Resolution.Merge` nil.
  - `pkg/fusion`, beside `TestNLAnswerCarriesWordIndexCounts`: an NL resolve with a `Merge` puts it on a response
    with nodes, on a miss, and on an all-seeds-unhydrated deferral, each checked on the marshalled JSON (`"merge"`
    and its keys); a symbol resolve's response and an NL resolve with no `Merge` have no `merge` key.

  Premises: `pkg/fusion/fusionnats/client.go:157-196` (`resolveMerged`), `pkg/fusion/retrieval.go:81-87`
  (`Resolution`), `pkg/fusion/contract.go:145-200` (`Response`), `pkg/fusion/engine_lens.go:116-150` (the resolve
  and the NL copy), `graph/query_global_search_types.go:47-53` (`GlobalSearchHint`).
  Accept: the new cases fail on the base commit for the right reason (missing field or key).
- [x] 2.2 Implement: the `Merge` type with a doc comment that says what each field means to a caller, the two
  fields, the copy in `resolveMerged`, and the copy in the NL branch of `engine_lens.go`.
  Accept: 2.1 passes; `task verify`.

## 3. Docs and spec

- [x] 3.1 The delta `specs/fusion/spec.md` matches the code and names the tests. Accept: `task spec:check`.

## 4. Close

- [x] 4.1 Archive the change and sync the spec as the last content commit. Accept: `task spec:check`, and
  `task merge:check` on this PR.
