# Proposal: setup-04a-03-boot-health

## Why

Change 3 of Slice 04A (`setup-04a-03-boot`) ports the packages the first booting process needs: `service` and what
it imports. The owner started it on 2026-10-19 with `health` first (#9 comment 6270875286). From change 3 on, each
pull request ports one package under its own OpenSpec change (#159; architect contract, "One package per pull
request"). `health` is 625 non-test lines at the pin `8b99efe9` and imports nothing else in change 3, so it goes first.

`health` holds the health report a service gives (`health.Status`) and the two functions `service` builds its
operator answers from: `Aggregate`, which rolls several reports into one, and `FromComponentHealth`, which turns a
component's own report into a `health.Status`. At the pin, `service` serves those answers at `/health`,
`/services/health` and `/components/health`, and publishes each service's report on `health.service.<name>`. The 03B
design keeps those endpoints as at the pin
(`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:722-725`).

## What Changes

- Port `health` from the pin to `health`, with its tests, under an `adapt` ledger row.
- Drop the surface nothing reads at the pin or in any named consumer (inventory § 1, S4): `Monitor` and its 11 methods,
  plus `NewMonitor`; `Status.WithMetrics`; `Status.WithSubStatus`; `Status.IsHealthy`; `Metrics.MessagesProcessed`.
  Their tests go with them, including the two `time.Sleep` calls in `monitor_test.go` that `task verify` refuses.
- Rewrite `doc.go` and `README.md` so they describe only what the code does (inventory § 1, S5).
- Behaviour is as at the pin, including the owner's 2026-10-19 ruling: a component with `Healthy=false` is reported
  `unhealthy`, whatever its own status word says (#318 comment 6271450376). Pending owner question Q3 (design), one
  message changes: an unhealthy component with no error text reads "Component unhealthy", not "Component healthy".

## Capabilities

- Added: `health-status` (how reports roll up, and how a component's report becomes a `health.Status`).

## Impact

- New package `health`; it imports `component` (already in the tree). Nothing in the tree imports it yet. It waits
  for `service`, the last pull request of change 3 (architect contract: a package nothing imports yet is carried).
- No wire change: the JSON field names of `health.Status` and `health.Metrics` are as at the pin, and so are the
  values, except the one message Q3 changes if the owner says fix. The dropped `messages_processed` field was
  `omitempty` and never set, so it never appeared in output.
- `docs/admission-ledger.yaml`: one new row, `health`.
- No coverage floor: the 03B design lists `health` as not gated (`design.md:461`).
- Retires no issue (inventory § 3). Addresses #9.
