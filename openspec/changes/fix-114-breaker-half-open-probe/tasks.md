# Tasks: fix-114-breaker-half-open-probe

Quick track: proposal and tasks, with no design document and no spec delta, because no requirement changes. Codex
reviews the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [x] 1.1 Proposal and tasks committed (`26b5c0e`); owner approval of the lost-probe rule (2026-11-08, #114 comment
  6722829046). Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [x] 2.1 `TestBreaker_HalfOpenAdmitsOneProbe`, `TestBreaker_ProbeSlotAfterFailedAndPassingProbe`,
  `TestBreaker_LostProbeFreesSlotAfterOneCooldown`, `TestBreaker_StatusReadsReserveNothing` (subtests
  `EndpointStatus`, `EndpointStats`) and `TestBreaker_ConcurrentProbeAdmitsExactlyOne` in `model/breaker_test.go`
  (`c2b3436`). On the base commit `63f9317` each fails under `go test -race`: a second caller is admitted while the
  probe is outstanding (the first four), and 32 of 32 concurrent callers are admitted past the cooldown, where one is
  wanted. Examples suffice, no generated checks: the proposal gives the reason. Gate: each fails on the base commit.
- [x] 2.2 `IsHealthy` takes the probe slot and refuses other callers until `RecordResult` reports or one `Cooldown`
  has passed since the probe was admitted; `EndpointStatus` keeps the lazy Open to HalfOpen move and reserves nothing;
  the struct, `probeInFlight` and `HealthPolicy.IsHealthy` doc comments say so and the "best-effort serialization"
  comment is gone; the `model` ledger row names the change (`da8a524`). The existing breaker tests needed no change:
  they observe half-open through `EndpointStatus` and decide the probe with `RecordResult`, which still holds.
  `task mutate:check` detects dropping the probe-slot guard (`TestBreaker_HalfOpenAdmitsOneProbe` and
  `TestBreaker_ConcurrentProbeAdmitsExactlyOne`), `<` to `<=` in the lost-probe comparison and not recording when the
  probe was admitted (`TestBreaker_LostProbeFreesSlotAfterOneCooldown`), and a status read that reserves the slot
  (`TestBreaker_StatusReadsReserveNothing`); no survivors. Gate: the tests in 2.1 pass under `-race`; `task verify`
  passed on a clean tree at `da8a524`.

## 3. Code review

- [x] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open: approve at `797e6d1` (PR #556
  comment 6723152105), one NIT (stale pending-approval wording in `proposal.md` and the PR body), fixed. The PR body
  carries `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive --skip-specs`). Gate: `task spec:check` and
  `task docs:check`.
