# Proposal: fix-656-batch-damaged-entities

Approved-by:

Track: `design`. Capabilities modified: `graph-transport-boundary`, `fusion`, `graph-query`.

## Why

When one requested entity is damaged (graph-ingest's stored-value rule refuses its stored bytes), graph-ingest refuses
the whole batch read with `graph_state_reset_required`. The good entities in the same request are never returned
(`processor/graph-ingest/query.go:733-734`, `:136`). The owner's rule (#657, rulings 1 and 2) says: "The caller named
the IDs (entity, batch): return the rest and name each damaged ID in the reply's per-ID list". A damaged entity gets
its own reason, `damaged`, and `error` keeps its present meaning. For #656 the ruling says: "the batch reply returns
the good entities and names each damaged one in `missing` with reason `damaged`. Fusion's one-ID-at-a-time re-read
goes with it."

## What Changes

- `graph` gains `MissingDamaged MissingReason = "damaged"`. The closed set becomes `not_found`, `error` and `damaged`.
- graph-ingest's batch verb returns every readable entity and lists each damaged ID in `missing` with reason
  `damaged`. This holds when every ID is damaged too: the reply then has an empty `entities` list and no refusal. A
  read fault that is not damage still fails the whole batch (transient `internal`). The damaged-entity record and
  graph-ingest's health do not change. The `batch_query_missing_total` counter gains a `damaged` series.
- The prefix page keeps refusing (ruling 1). The entity verb does not change.
- `fusionnats.Client.Entities` drops its re-read of a refused batch one ID at a time. It maps `damaged` to
  `fusion.UnhydratedError`, which is what a damaged seed shows today (owner question O2).
- graph-query's `localSearch` and `globalSearch` keep refusing a search with a damaged hit, now from the reply's
  `damaged` entry and not from the batch's refusal (owner question O1).
- Docs and migration notes: listed in tasks 2.4.

## Out of scope

The other follow-ups of #657 (structured detail on refusals, graph-index's value-filter code, skipped-row counts,
the scope of the embedding count, ADR-109 wording), #658, and #659 stay out.

## Impact

Packages touched: `graph`, `processor/graph-ingest`, `pkg/fusion/fusionnats`, `processor/graph-query`, plus docs. No
other open PR overlaps (only #692, this claim). One PR, #692, which closes #656.
