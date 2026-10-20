# Tasks: setup-04a-03-boot-service-base

Each task names its outcome and the gate that proves it. This change ports `BaseService` and the shared files of `service`
(PR 1 of 7 in the split the owner accepted, #9 comment 6296023694). Sections 2 onward arrive with the accepted design.

## 1. Inventory

- [x] 1.1 Inventory reviewed (`INVENTORY PASS` from an independent SemEngine inventory review, round 2 at
  `ad946b7`).
  Gate: `task spec:check` and `task docs:check`.
- [x] 1.2 The split by lifecycle owner and its merge order accepted by the owner on #9, with Q1 to Q3 of
  `inventory.md` § 10 ruled: #9 comment 6296023694 (option B; `PredicateReconciler` restored in `pkg/projection` by
  PR 3; `MaybeStartPProf` gains a stop, PR 1).
- [ ] 1.3 Proposal and design for PR 1 pass the pre-owner review and are accepted by the owner. Gate:
  `task spec:check` and `task docs:check`.
