# Tasks: setup-04a-foundation

Each task names the outcome that proves it. An unchecked task that says "hold" is one `task spec:queue` reports as
blocked until the named review or ruling exists. "This pull request" is PR #47. Evidence is recorded there as a
comment unless a task says otherwise.

## 1. Inventory

- [x] 1.1 `inventory.md` records the architect's inventory at base `38b184b` (tree `9286055`), with every entry
      pinned and no options, slice order, or design.
- [x] 1.2 Hold: independent inventory review. The reviewer's verdict on `inventory.md` is `INVENTORY PASS`, recorded
      on PR #47 with the reviewed file's checksum.

## 2. Options and design

- [x] 2.1 `design.md` frames the options for cutting the tier-0 set into changes with their costs,
      including doing nothing (one change) and extending an existing surface, states every premise with its
      measurement, and names one recommendation.
- [x] 2.2 Independent design review: `DESIGN PASS` on re-check 3 (BLOCKING → CHANGES REQUESTED ×2 → PASS), recorded
      on PR #47 with the reviewed file's checksum.
- [x] 2.3 Owner acceptance on #9 (2026-10-01): the chain and rulings (a)–(d), (f)–(h) are posted on #9 and recorded
      in `design.md`, "Owner rulings".

## 3. Landing

- [x] 3.1 This change is design-only (owner ruling on #9): the spec deltas named in D10 are drafted in each of the seven
      changes' own OpenSpec change, `setup-04a-01-floor` first, so `.openspec.yaml` keeps `skip_specs: true` and no
      `specs/` folder exists here.
- [ ] 3.2 `openspec archive setup-04a-foundation` is this pull request's last content commit; `docs/repository-map.md`
      names the archived change and the seven planned changes; a bounded reviewer check of the archive commit is
      recorded on PR #47.
