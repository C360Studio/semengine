# Proposal: fix-114-breaker-half-open-probe

Approved-by: owner, 2026-11-08 in Enjoy, approved the lost-probe rule (one `Cooldown`); recorded on #114 (comment 6722829046)

## Why

Issue #114. `model.RollingWindowBreaker` says that once the cooldown has passed, half-open lets exactly one probe
through and refuses every other caller until the probe reports (`model/breaker.go:73-75`). It does not do that:
`IsHealthy` is `EndpointStatus(endpoint) != StatusOpen` (`model/breaker.go:117-119`), so every caller is admitted
while the first probe is still outstanding, and `probeInFlight` is set but never read. The first status read past
the cooldown also sets `probeInFlight`, so a caller that only reads the status would take the probe slot if anything
read it. The file is carried unchanged from the pin `8b99efe9`.

Measured 2026-11-08 on `8195f3e`: no production code calls `IsHealthy`, `EndpointStatus` or `RecordResult` on a
breaker (`git grep`; the only callers are `model/breaker_test.go` and `model/health_test.go`). MEDIUM by #185: no
current trigger. Owner chose 2026-11-08 (in Enjoy) to fix it now rather than wait for the first caller.

Overlap: no open PR touches `model/` or `docs/admission-ledger.yaml` (checked 2026-11-08).

Quick track: proposal and tasks. No spec delta: no spec names `model`; the code is brought in line with its own doc
comment. Codex reviews the code.

## What Changes

- `IsHealthy` admits one probe per half-open period: past the cooldown, the first caller moves the endpoint to
  half-open and takes the probe slot (true); every other caller gets false until `RecordResult` closes the slot.
- **Lost-probe rule (the one new rule, needs the owner's approval):** a probe that has not reported within one
  `Cooldown` of being admitted counts as lost, and the next `IsHealthy` caller takes the slot. Without a rule, a probe
  whose caller crashed or dropped the request would hold the endpoint half-open, refusing everyone, for ever.
  `Cooldown` is reused so there is no new config field.
- `EndpointStatus` and `EndpointStats` read the state and reserve nothing. `EndpointStatus` still reports half-open
  once the cooldown has passed.
- The `HealthPolicy.IsHealthy` doc comment (`model/health.go:118-120`) drops "Equivalent to EndpointStatus(name) !=
  StatusOpen"; the stale comment in `EndpointStatus` about "best-effort serialization" goes.
- Unchanged: any `RecordResult` while half-open decides the probe, even one from a request admitted before the
  endpoint opened. The breaker has no request identity to tell them apart, and the pin behaves the same way.
- Tests with the existing fake clock: a second caller is refused while the probe is outstanding; the slot reopens
  after a failed probe's cooldown and the breaker closes after a passing one; a lost probe frees the slot after one
  `Cooldown`; status and stats reads reserve nothing; concurrent `IsHealthy` calls past the cooldown admit exactly one
  (run under `-race`). Each fails on the base commit.
- Examples suffice, no generated checks (`docs/testing.md`): one state machine with four named transitions, each an
  example; the concurrent test checks the one-probe law directly.
- Ledger row `model` (already `adapt`): the contract and changed-from-the-pin text name this fix.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None.
