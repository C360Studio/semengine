# Tasks: flake-defense

Each task names the outcome that proves it. An unchecked task that says "hold" is one `task spec:queue` reports as
blocked until the named review or ruling exists.

## 1. Inventory

- [x] 1.1 `inventory.md` records the architect's inventory at base `4d96860`, with every entry pinned and no options
      or design.
- [x] 1.2 Hold: independent inventory review. The reviewer's verdict on `inventory.md` is `INVENTORY PASS`, recorded
      on PR #44 with the reviewed file's checksum.

## 2. Options and ruling

- [ ] 2.1 Hold: task 1.2. `design.md` frames the options with their costs, including doing nothing and extending an
      existing guard, and names one recommendation.
- [ ] 2.2 Hold: owner ruling on #42. The owner's ruling on the options is posted on #42 and recorded in `design.md`.

## 3. Design, specs, and implementation

- [ ] 3.1 Hold: task 2.2. The spec changes for the ruled design are under `specs/`, and `.openspec.yaml` no longer
      sets `skip_specs`.
