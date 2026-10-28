# Proposal: fix-280-lifecycle-one-field-structs

## Why

Issue #280 item 3. Two unexported structs in `pkg/lifecycle/manager.go` each wrap one value:
`createOutcome` holds `Entity *graph.EntityState`, and `transitionOutcome` holds `committedRevision uint64`. Each can
be its plain value. Simplification wave PR 9b (#272) merged without them because its inventory read #153 as naming
the word, not the package. Items 1 and 2 of #280 are already done: PR #432 fixed the storage doc, and the owner
ruled on 2026-10-22 to keep `component.Declaration`'s parallel lists, which service PR 4 (#367) documented. This
change finishes the issue.

Light process, owner 2026-10-28: proposal and tasks; no design document and no spec delta, because no requirement
changes; Codex reviews the code.

## What Changes

- `createWithSource` and `createWithRegistration` return `(*graph.EntityState, error)`; `createOutcome` is deleted.
  `CreateFromOperator`, the one reader of the entity, reads the returned value.
- `transitionWithOutcome` returns `(uint64, error)`, the committed revision; `transitionOutcome` is deleted.
  `DespawnWith`, the one reader of the revision, reads the returned value.
- Nothing else changes: no exported identifier, no error, no behaviour. Every file outside `pkg/lifecycle/manager.go`
  stays as it is.

## Capabilities

None. No requirement under `openspec/specs/` changes.
