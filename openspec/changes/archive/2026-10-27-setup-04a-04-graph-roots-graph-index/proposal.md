# Proposal: setup-04a-04-graph-roots-graph-index

## Why

Change 4 (`setup-04a-04-graph-roots`) of epic #9 ports the graph roots, one package per pull request, in the order
the owner set on 2026-10-24 (#9 comment 6386856657). Item 3 is `processor/graph-index` with the ENTITY_STATES follower
(#107, #108, #110, #103; the ADR-109 reader). graph-index keeps the reverse indexes (outgoing, incoming, alias,
predicate, name) that graph-query and graph clustering read, and it is the first component that projects
`ENTITY_STATES`. ADR-108 D2 lands the follower with it, so the watch, startup replay, reset latch, watermark and
readiness envelope are written once, in a graph package, instead of once per projecting component. The owner's bar:
"graph is in many ways our secret sauce. it needs to be solid… i would rather not bring in a pkg that causes instant
debt". graph-index has no external-provider code, so the tier-0 ruling (#9 comment 6386603659) removes nothing.

## What Changes

- Port `processor/graph-index` onto `component.Lifecycle` (ADR-108), with the run context created in `Start`
  (ADR-113), a second `Start` refused, `Health` read from named stuck states only (ADR-109 D2), strict configuration
  and `batch_size` removed, metrics on the injected registry (#68), and `internal/dispatch.KeyedPool` in place of its
  private dispatcher (#107).
- Add the follower package under `graph/` (name proposed `graph/follower`), built from graph-index's watch loop and
  readiness glue, with the validating phase and the declared lost feed ADR-108 D4 asks for.
- graph-index's queries answer `index_not_ready` during the startup replay and while the feed is lost, in process
  (ADR-109 D1; owner ruling, PR #414 comment 6426254115, item 2), and every reply
  carries `indexed_revision` and `producer` (`graph.QueryResponse`, ingest-kernel D18; #103's producer half).
- graph-index's eight verbs join `graph.QueryVerbs()` (#106's graph-index half).
- `ComputeIndexStatus` and the revision gauges move into `graph/readiness`, without the legacy fields (#110 item 1).
- Drop dead surface: six exported index writers, test-only paths and hooks (`subscribeForRequests`, `statusInterval`).
- Carry ADR-074, ADR-077 and ADR-078 into `docs/adr/`; add ledger rows; measure rebuild-on-restart for #108.

## Capabilities

- New `graph-index`, new `entity-state-follower`.
- Modified `graph-readiness` (the index projection, revision gauges) and `graph-transport-boundary` (verb table and
  responder rules per responder; index replies carry their revision).

## Impact

- New `processor/graph-index` (about 3,250 tree lines) and `graph/follower` (about 450); `graph`, `graph/readiness`
  about 130. About 4,660 pin lines carried: over the one-package bound (#159), waived by count (design D1).
- New subjects `graph.index.query.*` (8), declared in the verb table. New buckets: none (the five are in
  `graph/kvcatalog` already). New config keys: none (`batch_size` removed). New exported surface: the follower,
  `QueryResponse` fields, `ResponderGraphIndex`, the revision gauge constructor.
- Consumers: removing `batch_size` fails semconnect's two configs at boot until they delete the line; eight new reserved
  subjects make a stream on `graph.index.query.*` fail at boot (none found in consumer configs) (design D9, D17).
- Owner ruling (PR #414 comment 6426254115): one PR with a waiver by count; refuse queries while the feed is lost;
  the alias index keeps the pin's behaviour and #415 (stale aliases, `status:blocked`) stays open for its own PR before
  PR 9.
- Addresses #107, #109, #110, #103, #106, #68, #195, #196. Retires no issue by itself; #415, #108, #408, #300 stay open
  (design D17).
