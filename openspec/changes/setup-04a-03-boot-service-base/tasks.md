# Tasks: setup-04a-03-boot-service-base

Each task names its outcome and the gate that proves it. This change ports `BaseService` and the shared files of
`service` (PR 1 of 7 in the split the owner accepted, #9 comment 6296023694). Tests come first. Record in
`notes/<task>.md` how each test fails on the base commit, or which mutant it detects. Review stops (Codex) come after
2.2 and 3.2. Each task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Inventory

- [x] 1.1 Inventory reviewed (`INVENTORY PASS` from an independent SemEngine inventory review, round 2 at
  `ad946b7`).
  Gate: `task spec:check` and `task docs:check`.
- [x] 1.2 The split by lifecycle owner and its merge order accepted by the owner on #9, with Q1 to Q3 of
  `inventory.md` § 10 ruled: #9 comment 6296023694 (option B; `PredicateReconciler` restored in `pkg/projection` by
  PR 3; `MaybeStartPProf` gains a stop, PR 1).
- [ ] 1.3 Proposal and design for PR 1 pass the pre-owner review and are accepted by the owner. Gate:
  `task spec:check` and `task docs:check`.

## 2. Port (D1-D6)

- [ ] 2.1 Tests first, in `service/`: `base_lifecycle_test.go` and `base_test.go` carried and adapted per D6 (unit
  where no NATS is needed, no sleeps, the dropped options' subtests removed, the two abort tests and the one-shot Stop
  test flipped); the `natsfixture.Shared` `TestMain` file; the two tests moved from the pin's
  `service_composition_test.go`; the new tests D2, D3, D4 and D6 name, including the lifecycle-suite adapter and
  `TestCanonicalServiceJSONIsIdempotent`. The notes cite the probe comment on #343 (H1 to H6, design, Probes). Gate:
  the tests fail on the base (the package does not exist; note the compile error).
- [ ] 2.2 The nine files from the pin with the module path rewritten, D1's surface, D2's lifecycle, D3's
  `MaybeStartPProf`/`PProfServer`, D4's `kvcatalog` call, and Q1 as ruled. Gate: `go test -race ./service/` and
  `go test -tags integration ./service/` pass, `TestBaseServicePassesTheLifecycleSuite` and
  `TestPProfShutdownWaitsForAnInFlightRequest` included; `go build ./...`; `task verify` passes on a clean tree
  (review stop).
- [ ] 2.3 Sensitivity: each mutant D6 lists makes its named test fail and the revert makes it pass, through
  `task mutate:check` for the unit tests and by hand with `-tags integration` for the retention mutant (#129),
  recorded in `notes/2.3.md`. Gate: `task verify` passes on a clean tree.

## 3. Text, ledger, coverage, guidance (D7)

- [ ] 3.1 `service/doc.go` and `service/README.md` for this surface, with the contract lines D1 and D3 name (the
  embedding calls, `MaybeStartPProf` then `Shutdown` with a fresh bounded context, `WireGraphRuntime`). Pin ADR-058
  in `docs/adr/` with its port note (design, Decision records). Gate: `go doc ./service` lists only the kept surface;
  `task docs:check`.
- [ ] 3.2 The `service` and ADR-058 ledger rows; the `docs/repository-map.md` row; `"service merged"` in
  `scripts/cover-check.sh`; the OpenAPI-registry sentences in `.agents/contracts/semengine-developer.md` and
  `semengine-reviewer.md`. Gate: `task verify` passes on a clean tree, which runs `ledger:check`, `cover:check`
  (`service` at or above 80%, the number recorded on the PR) and `docs:check` (review stop).
- [ ] 3.3 File the follow-up issue for PR 2: refuse letter-case variants in service configuration payloads through
  `config`'s checker (D5), linked from the PR without a closing keyword; comment on #195 that `service`'s shared
  client is replaced. Gate: the issue and comment read back.

## 4. Verify, review and close

- [ ] 4.1 `task verify` passes on a clean tree at the implementation head; the coverage of `service` (merged
  profile) is recorded on the PR.
- [ ] 4.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 4.3 Any defect found and not fixed is filed. Then the change is archived
  (`openspec archive setup-04a-03-boot-service-base`) as the last content commit, syncing `service-lifecycle` into
  `openspec/specs/`. Gate: `task spec:check`.
