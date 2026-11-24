# Design: fix-740-warm-restart-replay

## Context

Facts are from `inventory.md` and probe740 (scripts and outputs: PR #747 comments 7137819740, 7137820382 and
7146304046). The server is nats-server 2.15.0 (the `natsfixture/nats-image` pin), the client nats.go v1.55.0.
`pending_bytes` is sampled every 250 ms, so each peak is a lower bound.

- Boot replays: up to about ten whole-bucket watches at tier 1, all on the process's one connection (inventory § 1.1).
  They overlap because components start at once (`service/component_manager_lifecycle.go:74-85`).
- A lost connection fails `Start`: Subscribe refuses at once while reconnecting (`natsclient/client.go:1176`).
- P1, P2: at the default `max_pending` (64 MiB), five watches on one connection close it 7 to 10 times at 230 MB.
  Failures begin between 17 and 33 MB.
- P1b, P5: one watch peaks at 33 to 47 MB whatever the store size. That is the server's flow-control window
  (`server/consumer.go:607` at 2.15.3, a source read), not the store.
- P3: after a close, each watch resumes where it stopped. Replays finish; it is `Start` that fails during the drops.
- P8, `max_pending` 512 MiB: five watches peak at 185 MB (230 MB store) and 180 MB (460 MB store); ten watches peak at
  344 MB at both sizes. There were no closes, and every watch delivered every key once and in order.

## Decision

**D1. A required server setting, `max_pending: 536870912`.** `docs/boot.md` states it with its reason, the way it
states that JetStream must be enabled. Ten readers peaked at 344 MB of 512 MiB in P8. That margin is measured, not
promised. Not chosen (Fable read, #740 comment 7146176783): a connection per reader, which does not reduce the load and
adds 5 to 8 connections; replays one at a time, which would make tier 0 boot wait on the embedding provider
(`graph/embedding/worker.go:363`); and a byte-bounded pull reader, which would re-implement nats.go's KV watch.

**D2. The proving test runs with the setting.** `natsfixture.WithMaxPending(n)` writes `max_pending` into the broker's
config file beside `max_payload` (`internal/harness/natsfixture/fixture.go:201-205`).
`TestSmoke_WarmRestartOverALargeStore` starts its own broker with 512 MiB and keeps its 132 MB store. It asserts that
`Start` returns nil, that every reader reports its snapshot applied, and that the account's slow-consumer count does
not change. It was red at the default (CI run 45443875456) and must be green with the setting.

**D3. Revert `NewSibling`.** Nothing calls it once the per-reader work is dropped. It is reverted with a revert commit;
the branch was pushed, so there is no force-push.

## Risks

- A deployment on a default NATS server still fails a warm restart; the boot error does not name the setting.
- The number must be revisited when the framework adds a whole-bucket reader, or if nats-server changes its window.
- 2.14.10, the reporter's server, was not probed.

## Rulings

Owner, 2026-11-24: #740 comment 7146268745 replaces the per-connection rulings in comment 7136495793.
`Approved-by:` is to be re-filled by the owner.
