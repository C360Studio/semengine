# Proposal: test-557-composed-stack-smoke

Approved-by:

## Why

Issue #557. Nothing in SemEngine starts the real components together the way a product would: from one JSON
configuration, through `config.Manager`, `ServiceManager` and `ComponentManager`. The one composed test
(`processor/graph-query/composed_integration_test.go:66-107`) wires three components by hand and has no
graph-embedding. The owner chose use case A, "ingest facts and ask questions", and made restart safety a main goal:
"much of the reason semstreams failed was the debt around resiliance so we need to ensure we start off right in
semengine" (#557 comment 6729015840). The four restart cases are required to pass, a broker restart under a running
stack included (comment 6730981995).

Design track: the change adds a capability spec, `composed-stack`, with one requirement per promise.

## What Changes

- A new test-only package, `internal/smoke`, whose integration tests are the composition root: one JSON
  configuration boots graph-ingest, graph-index, graph-embedding and graph-query on one NATS server.
- A producer publishes a fixed set of entities on `entity.>`; every check reads only what a consumer can read:
  `graph.query.*` answers (entity, relationships, path, summary, alias), the three `GRAPH_STATUS` readiness keys,
  component health over HTTP, and embedding records read from `EMBEDDING_INDEX` (no query verb serves them until
  PR #558). `localSearch` is checked only as its refusal: nothing in this stack writes community records. Every
  wait is ordered after the write it checks, and each restart is followed by new writes, so stale state cannot pass.
- Restart safety, each case its own test: (1) stop and start the whole stack; (2) kill the process mid-ingest;
  (3) stop the stack, restart the NATS server, start the stack; (4) restart the NATS server under the running stack.
  After each, the same fixed answers, and every acknowledged publish present exactly once.
- Case 4 was probed before the design (nats.go v1.55.0): the follower's watch survives a file-storage broker restart
  and stalls about 20 s (inventory, "Probe"). If case 4 still fails on main, the fix is part of this work (design D8).
- No production code, unless case 4 needs a fix. No binary, Compose, fixed port, sleep, own deadline or skip.
- `semantic` and `similar` checks are added when PR #558 merges (owner, comment 6730981995).

## Owner decisions this proposal asks for

1. The stack runs in a child process for every boot (D3), not in the test process.
2. Ports: the test picks a free port and writes `http_port` into the configuration (D4), instead of a production
   change that lets the configuration ask for a kernel-picked port and exposes the bound address.
3. Four declared reads besides the public paths (D5): `EMBEDDING_INDEX`; graph-ingest's consumer info and the
   `ENTITY_STATES` status, to order each wait after its write; graph-ingest's applied-sequence bucket, to prove the
   case-2 kill happened mid-ingest.
4. The new package's home and name, `internal/smoke` (D1).
5. `localSearch` checked only as its refusal until community records have a writer.

## Capabilities

### New Capabilities

- `composed-stack`: the tier-0 graph stack boots from one configuration and keeps its answers across the four
  restarts.

### Modified Capabilities

None.
