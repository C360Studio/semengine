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

- [ ] 2.1 Hold: task 1.2. `design.md` frames the options for cutting the tier-0 set into changes with their costs,
      including doing nothing (one change) and extending an existing surface, states every premise with its
      measurement, and names one recommendation.
- [ ] 2.2 Hold: independent design review. The reviewer's verdict on `design.md` is a pass, recorded on PR #47 with
      the reviewed file's checksum.
- [ ] 2.3 Hold: owner acceptance on #9. The owner's acceptance of the slicing and the first change's scope is posted
      on #9 and recorded in `design.md`.

## 3. Specs and the first change

- [ ] 3.1 Hold: task 2.3. The spec changes for the first change are under `specs/`, `.openspec.yaml` no longer sets
      `skip_specs`, and `task spec:check` passes.
