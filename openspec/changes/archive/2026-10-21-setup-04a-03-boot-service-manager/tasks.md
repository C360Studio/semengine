# Tasks: setup-04a-03-boot-service-manager

Each task names its outcome and the gate that proves it. This change ports the service manager and the metrics service
of `service` (PR 2 of 7 in the split the owner accepted, #9 comment 6296023694). Tests come first. Record in
`notes/<task>.md` how each test fails on the base commit, or which mutant it detects. Review stops (Codex) come after
2.2 and 3.2. Each task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Shape sweep (`sweep.md`) and design pass the pre-owner review (round 2 at `daae8e8`, fixes `0a65778`) and
  are accepted by the owner, who answered Q-A yes and Q-B yes (#9 comment 6317003800); the probe note (#353
  comment 6315789814) and its round 2 addendum are on #353. Gate: `task spec:check` and `task docs:check`; each
  comment read back.

## 2. Port (D1-D7)

- [x] 2.1 Tests first, in `service/`, `internal/keycase/` and `internal/httpadmission/` (`metric/admission_test.go`
  moved, D2): the pin test files D7 lists, adapted (dead tests out,
  runtime-stop and base-context tests flipped to D2, `t.Skip` in the `rapid` action made a no-op, the `StopAll` model
  rewritten, `errs.ErrAlreadyStopped` in the idempotency test, the two pause tests changed as D2 says, and the
  refused-dial assertion added to `TestStartAllRollsBackFailedChildStartWithDetachedBoundedContext`, which the pin never
  checked); `gaugeValue` and `requireGauge` in a helper file; the two lifecycle-suite adapters; every new test D2-D6
  names; `keycase.Check`'s own tests (one per shape, design § Generated checks) and the guard and its sensitivity test
  moved from `config/load_test.go`. The notes cite the probe comment on #353. Gate: the tests fail on the base (the
  types do not exist; note the compile errors).
- [x] 2.2 The nine files from the pin with the module path rewritten, D1's surface, D2's lifecycle, D3's interface and
  default composition, D4's metrics service, D5's moved check (`config` calling it), the admitted-request counter moved
  to `internal/httpadmission` (`metric` and `service` calling it, D2), D6's routes and the #349 fix in
  `service/schema.go`. Gate:
  `go test -race ./service/ ./internal/keycase/ ./internal/httpadmission/ ./config/ ./metric/` and
  `go test -tags integration ./service/ ./config/` pass, `TestManagerPassesTheLifecycleSuite` and
  `TestMetricsPassesTheLifecycleSuite` included; `go build ./...`; `task verify` passes on a clean tree (review
  stop).
- [x] 2.3 Sensitivity: each mutant D7 lists makes its named test fail and the revert makes it pass, through
  `task mutate:check` for the unit tests and by hand with `-tags integration` where a test needs NATS (#129),
  recorded in `notes/2.3.md` with the `rapid` seeds and check counts. Gate: `task verify` passes on a clean tree.

## 3. Text, ledger, coverage, ADRs (D8)

- [x] 3.1 `service/doc.go` and `service/README.md` for the manager and the metrics service, with the contract lines D1
  and D8 name; pin ADR-030 and ADR-094 in `docs/adr/` with port notes (design, Decision records); the
  `/graph/triples` row in `docs/contract.md`'s operator table. Gate:
  `go doc ./service` lists only the kept surface; `task docs:check`.
- [x] 3.2 The `service` ledger row edited in place, the ADR-030 and ADR-094 rows, the `config` and `metric` rows'
  sentences; the `docs/repository-map.md` rows (`service/`, `docs/adr/`, `internal/keycase/`, `internal/httpadmission/`,
  and the ledger count, 88). Gate:
  `task verify` passes on a clean tree, which runs `ledger:check`, `cover:check` (`service` at or above its floor, the
  number recorded on the PR) and `docs:check` (review stop).
- [x] 3.3 #344 told that service payloads now refuse letter-case variants and that the heartbeat's decode (PR 6)
  closes it; #196 told that the manager and the metrics service compose the guard (or why not, from 2.2's notes).
  Any defect found and not fixed is filed (`status:blocked` on #353, linked without a closing keyword). Gate: each
  comment and issue read back.

## 4. Verify, review and close

- [x] 4.1 `task verify` passes on a clean tree at the implementation head `f1ced98` (`verify: ok`, 2026-10-21); the
  coverage of `service` (89.4%, merged unit and integration profile, floor 80%) is recorded on the PR (#353 comment
  6322449603).
- [x] 4.2 Cross-agent review of record at both stops, as PR comments (Codex: stop 1 at `66a5d30`, #353 comments
  6319900945 and 6321007206; stop 2 at `dfb3418`, comment 6322221725, two documentation findings fixed at `f1ced98`);
  `reviewed-by:` in the PR body.
- [x] 4.3 The change is archived (`openspec archive setup-04a-03-boot-service-manager`) in this commit, the last
  content commit, syncing `service-lifecycle` and `operator-surface` into `openspec/specs/`. Gate: `task spec:check`.
