# Tasks: setup-04a-03-boot-service

Each task names its outcome and the gate that proves it. This change holds the `service` inventory and the split
proposal; once the owner accepts the split it is renamed to the first owner's porting change, and that change's
tasks replace these. `skip_specs` stays only until then.

## 1. Inventory

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS` from an independent SemEngine inventory review, cited by commit).
  Gate: `task spec:check` and `task docs:check`.
- [ ] 1.2 The split by lifecycle owner and its merge order accepted by the owner on #9, with Q1 to Q4 of
  `inventory.md` § 10 ruled. Gate: the ruling comment's id recorded here.
