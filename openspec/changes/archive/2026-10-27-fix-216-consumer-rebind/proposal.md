# Proposal: fix-216-consumer-rebind

## Why

A running graph-ingest stops applying input when its input stream is created again under it, and nothing in the
process recovers (#216). The inventory (`inventory.md`, INVENTORY PASS, committed at `6d500e0`) measured it at the
pinned nats.go v1.54.0: after a broker restart that loses a memory stream, the consume loop keeps pulling against a
consumer that no longer exists and never delivers again (probe case a); a stream deleted while the server stays up
ends the loop for good (case d). It also found a worse case: once the loop has ended, a consumer created again under
the same durable name makes `Health()` read healthy while nothing is delivered (cases e and f).

The owner ruled on 2026-10-27 (#216 comment 6443457897): "Yes, restart is the recovery (small fix)." Nothing rebinds
the consumer, in natsclient or in graph-ingest. graph-ingest reports a lost input binding as a stuck state that holds
until its process is restarted, and the text that promises otherwise is corrected. A restart reads the stream's
generation again, so re-ingestion after a restart keeps working (#15's behaviour).

## What Changes

- **A lost input binding is stuck until restart.** graph-ingest's `Health()` reads unhealthy, and stays unhealthy
  until the process is restarted, once a consume loop it started has ended without its own `Stop`. The readiness
  envelope reports the same tick as degraded. A memory stream lost at a broker restart already reads unhealthy through
  today's consumer lookup; a stream and consumer both created again around the restart stays the declared
  limitation. Nothing is rebound, stopped or deleted.
- **Text corrected.** The `graph-ingest-recovery` scenario "Memory transport lost at broker restart" and
  `docs/contract.md:79` say a process restart is the recovery and that operators restart on `/readyz`, not
  `/healthz`; `processor/graph-ingest/README.md:188` and the graph-ingest ledger row follow.
- **No natsclient change.** natsclient already returns the native handle graph-ingest keeps; nothing is added to it
  (#170 ruling; ADR-112).

## Capabilities

### Modified Capabilities

- `graph-ingest-recovery`: modified "Acknowledged is not durable" (its memory-restart scenario) and "graph-ingest's
  health reads its stuck states"; added "A lost input binding is stuck until the process restarts".

## Impact

- Code: `processor/graph-ingest/readiness.go`, `processor/graph-ingest/consume.go`, and their tests.
- Docs: `docs/contract.md`, `processor/graph-ingest/README.md`, `docs/admission-ledger.yaml` (graph-ingest row).
- Retires #216 (`Closes #216`), with the healthy-but-not-consuming case folded in. #15, #301 and #83 stay open
  (design, Dispositions).
