# Proposal: simplify-04a-18-natsclient-metrics

## Why

PR 6d of epic #223 (#223 comment 6234160898): the last bullet of #222. Inventory § 1 measured each item:

- A subscription the server refuses is only logged (#169). The owner ruled on 2026-10-12 to count it and keep the
  return.
- Two clients on one metrics registry write the same series (#75), and so would two graph-ingest instances. No
  program or config measured builds either.
- The stream message is built in three places. Two publish calls and the asynchronous path repeat the same steps.
- Keys are listed by filter in two ways that complete differently. `KeysByFilter` refuses a partial snapshot;
  `FilteredKeys` returns what it gathered.

## What Changes

- A permission or subscription-limit refusal adds one to a new counter, `semengine_nats_server_refusals_total`
  (`reason`), beside the log line. `Subscribe` still returns the subscription and nil (Q1).
- No metric mechanism is added for #75. One metrics owner per registry is written down for natsclient and
  graph-ingest; #75 closes (Q2).
- One private path builds and publishes a stream message. Each exported call keeps its errors and its metric. The
  false `AutoCreate` line goes.
- `FilteredKeys` runs on `KeysByFilter`'s code. Its reader argument needs `WatchFiltered`. A dead branch goes (Q3).

## Capabilities

- Modified: `transport-client`. Two requirements are added.

## Impact

- Files:
  - `natsclient/client.go`, `stream.go`, `kv.go`, `options.go`, `jetstream_metrics.go`, `README.md`, `doc.go` and
    their tests;
  - `processor/graph-ingest/component.go` (the comment at `:723-725`; after #289);
  - `docs/adr/112-natsclient-contract.md` (§2 amendment);
  - `docs/admission-ledger.yaml` (natsclient and graph-ingest rows).
- Behaviour that callers can see:
  - the new counter rises on a refusal;
  - `FilteredKeys` returns an error where it could return a partial list, and waits up to 5 s for its watcher to
    close.
- Breaking at cutover, found by the compiler: a reader passed to `FilteredKeys` without `WatchFiltered`. semteams
  passes a `jetstream.KeyValue` and is unaffected. The pin's narrow readers change when ported; agentic-tools'
  reader is in a package that is not admitted.
- Closes #75, #169, #222. Addresses #223.
