# Tasks: natsclient-contract

Each task names the outcome and the gate that proves it. "This pull request" is PR #177; evidence is recorded there
as a comment. (W) is the technical writer and (O) the orchestrating session. Every task can be ticked in or before the
archive commit. What follows the archive is not a task: the check of the archive, marking the pull request ready,
`task merge:check -- 177` and the merge.

## 1. Design

- [ ] 1.1 Inventory revision 2 and this change's proposal, design, tasks, delta, ADR-112 draft and rule text are
      reviewed together by semengine-reviewer (the owner's time-box, `design.md` Status). Gate: `INVENTORY PASS` and
      `DESIGN REVIEW PASS`, recorded on this pull request with each file's sha256.
- [ ] 1.2 The owner accepts the design and answers the ten questions in `owner-questions.md`. Gate: the acceptance
      and answers, recorded on #170 or on this pull request.
- [ ] 1.3 P10 to P16 and the reviewer's p15b are posted on this pull request, each output beside its source (with
      its sha256) and its command. Gate: the comment ids, written into ADR-112's Status.

## 2. Documents (W)

- [ ] 2.1 This change's directory, `openspec/changes/natsclient-contract/`, holds `proposal.md`, `design.md`,
      `tasks.md`, `inventory.md` (revision 2) and `specs/transport-client/spec.md` as accepted. Gate: each file's
      sha256, recorded on this pull request, and `task spec:check`.
- [ ] 2.2 `docs/adr/112-natsclient-contract.md` matches the accepted draft.
- [ ] 2.3 The rule is in place, as `rule.md` gives it: the `AGENTS.md` row after `:99`, the architect contract bullet
      after `:350` and the reviewer contract bullet after `:151`.
- [ ] 2.4 `docs/repository-map.md`'s `docs/adr/` row names ADR-112. If #176 merged first, merge `main` before this
      edit.

## 3. Issues (O)

- [ ] 3.1 #168's body is corrected at the sentences inventory §0 pins, and its scope narrows to the list in design
      D1. Gate: the body, read back after the write.
- [ ] 3.2 A one-line comment on #110: ADR-110:192 (a watcher sees every revision) against the pin's ADR-066
      `:408-409` (a watcher that lags is not delivered most revisions), for ADR-109 to measure (design D7). Gate: the
      comment id, read back.

## 4. Gate and archive

- [ ] 4.1 `task verify` passes on the head, on a clean tree.
- [ ] 4.2 The change is archived with `openspec archive natsclient-contract -y`. In the same commit, `transport-client`
      Purpose line `:8` is edited by hand as design D2 gives it, and `docs/repository-map.md` gains the archived
      change's row. This is the last content commit.
