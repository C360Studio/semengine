# Proposal: simplify-04a-18-natsclient-metrics

## Why

PR 6d of epic #223 (#223 comment 6234160898): the last bullet of #222. Inventory § 1 measured each item:

- A subscription the server refuses is only logged (#169). The owner ruled on 2026-10-12 to count it and keep the
  return.
- Two clients on one metrics registry write the same series (#75). No program builds that: every production
  `WithMetrics` caller makes one client per registry.
- The stream message is built in three places. Two publish calls and the asynchronous path repeat the same steps.
- Keys are listed by filter in two ways that complete differently. `KeysByFilter` refuses a partial snapshot;
  `FilteredKeys` returns what it gathered.

## What Changes

- A permission or subscription-limit refusal adds one to the JetStream error counter (`permission_violation`,
  `max_subscriptions_exceeded`) beside the log line. `Subscribe` still returns the subscription and nil (Q1).
- No metric mechanism is added for #75. One client per registry is written down; #75 closes (Q2).
- One private path builds and publishes a stream message. Each exported call keeps its errors and metric. The false
  `AutoCreate` line goes.
- `FilteredKeys` runs on `KeysByFilter`'s code. Its reader argument needs `WatchFiltered` (Q3).

## Capabilities

- Modified: `transport-client`. Two requirements are added.

## Impact

- Files:
  - `natsclient/client.go`, `stream.go`, `kv.go`, `options.go`, `README.md`, `doc.go` and their tests;
  - `docs/adr/112-natsclient-contract.md` (§2 amendment);
  - `docs/admission-ledger.yaml` (natsclient row).
- Behaviour that callers can see:
  - the counter rises on a refusal;
  - `FilteredKeys` returns an error where it could return a partial list, and waits up to 5 s for its watcher to
    close.
- Breaking at cutover, found by the compiler: a reader passed to `FilteredKeys` without `WatchFiltered`. semteams
  passes a `jetstream.KeyValue` and is unaffected; the pin's narrow readers change when ported.
- Closes #75, #169, #222. Addresses #223.
