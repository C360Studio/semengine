# Proposal: fix-740-warm-restart-replay

Approved-by: cglusky (2026-11-24) <https://github.com/C360Studio/semengine/issues/740#issuecomment-7146621360> sha256:2011f05ca40e

Track: `design`. Capability: `composed-stack` (gains one Requirement). Packages: `internal/harness/natsfixture` (a
broker option for tests), `internal/smoke` (the warm-restart test); docs in `docs/boot.md`. No production code.

## Why

A warm restart over a large store fails to start on a NATS server with default settings (#740). At boot every
whole-bucket reader replays its bucket at once over the process's one NATS connection. The server lets each replay
run about one flow-control window (32 MiB) ahead of the client, and closes a connection with more than `max_pending`
(64 MiB by default) waiting to be sent. Any component `Start` that runs during the drop fails.

The load is the number of readers replaying at once times that window, not the store size. Probe P8 (pinned
nats-server 2.15.0, nats.go v1.55.0; outputs on PR #747) ran with `max_pending` at 512 MiB:

| Watches on one connection | Store | Server closes | Peak waiting bytes |
| --- | --- | --- | --- |
| 5 | 230 MB | 0 | 185 MB |
| 5 | 460 MB | 0 | 180 MB |
| 10 | 230 MB | 0 | 344 MB |
| 10 | 460 MB | 0 | 344 MB |

Doubling the store left the peak unchanged, so one fixed setting covers any store size. SemSource already runs this
value (C360Studio/semsource#351).

## What changes

The owner ruled on 2026-11-24 (#740, comment 7146268745): "if the setting fixes it, we use it and document it."

- `docs/boot.md` states a required NATS server setting, `max_pending: 536870912` (512 MiB), with its reason and the
  sum it covers: a tier 1 stack runs about ten whole-bucket readers at boot.
- `natsfixture` gains `WithMaxPending`, written to the broker config file the way `WithMaxPayload` already is.
- `TestSmoke_WarmRestartOverALargeStore` runs on its own broker with the documented setting and must pass.
- `natsclient.Client.NewSibling` is reverted with a revert commit. The per-reader-connection work stays on a local
  branch, unmerged.

## Non-goals

- A connection per reader, replays one at a time, or a retry of a failed `Start` (Fable read, #740 comment
  7146176783).
- graph-index's client-side buffer overflow, which is #750.

## Impact

A deployment's NATS server must set `max_pending` to at least 512 MiB. A default server still fails a warm restart
once a handful of readers replay at once. The setting must be revisited when the framework adds a whole-bucket
reader. One PR (#747): `Closes #740`, `Closes #741`.
