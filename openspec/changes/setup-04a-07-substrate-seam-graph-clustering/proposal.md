# Proposal: setup-04a-07-substrate-seam-graph-clustering

Approved-by:

Track: `design` (adds a capability, `graph-clustering`; modifies `entity-state-follower` and `composed-stack`).

## Why

The owner's ruling of 2026-11-08 (#9 comment 6724212898, item 1) makes `processor/graph-clustering` the next port,
ahead of the rule core. Today nothing writes `COMMUNITY_INDEX`, so graph-query's `localSearch` and the text tier of
`searchGraph` refuse `index_not_ready` for ever, and semsource's graph search tool goes through that tier. The library
that keeps the records, `graph/clustering`'s maintainer, is on main (#483) with no caller. This change gives it one.

## What Changes

- New `processor/graph-clustering` on `component.Lifecycle` and `graph/follower`: every `ENTITY_STATES` change is
  applied to the maintainer in delivery order, and changed groups are published every `detection_interval`. Tier 0,
  no provider, no graph-index or graph-embedding dependency (ADR-099).
- `follower.Entry` carries the delivered value, so the maintainer applies the exact value at its revision.
- No tick starts while the feed is lost or a reset is latched. A tick that keeps failing is a named stuck state, a
  `degraded` envelope and a counter, not just a log line.
- New `graph-clustering` readiness key in `GRAPH_STATUS` (information, ADR-088 decision 1). Its indexed revision
  counts what the maintainer holds; each record's `entity_revision` says what is published.
- Removed config keys refused at boot with guidance: `batch_size`, `enable_llm`, `enhancement_workers`,
  `min_community_size`, `max_iterations`, `entity_id_edges`, `semantic_edges`, `allow_ungated_reads`.
- Not carried: label propagation and edge providers (ADR-099), the readiness gate (ADR-082), the four
  `graph.clustering.query.*` verbs (no reader), tier-2 summaries (#485) and review worker (#488).
- The composed smoke test adds graph-clustering: `localSearch` and a text question answer, across restarts.
- A contract test proves no package in the tree reaches `graph/llm`, `model/wire` or `go-openai` (04A D8).

## Questions for the owner

1. **Anomaly detection.** The pin's graph-clustering can also look for odd structure (k-core, missing links, missing
   transitive links) and write `ANOMALY_INDEX`. It is off by default, no consumer turns it on, the pin itself switched
   it off (pin ADR-054 "Move 1", ADR-061), its only reader is the gateway products own, and it reads community member
   lists that records no longer keep. Port it now (three more PRs before the rule core), later (its own issue, after
   the rule core), or not at all? Recommended: later. Then the config keys are refused and #198 and #141 stay open.
2. **When the LLM plumbing issue closes (#32).** Your comment of 2026-10-24 says #32 closes when tier 1 admits a
   provider, or when you rule that summaries and answer synthesis live in semteams. After this port nothing at tier 0
   uses that plumbing, and a test proves it. Change the closing condition to "tier 0 builds without it, proven by
   the test", closing #32 with this PR and leaving the plumbing to the tier-2 issues (epic #484)? Or keep it?
   Recommended: change it. If kept, #32 stays open and nothing else in this change moves.

## Capabilities

- New `graph-clustering`. Modified `entity-state-follower` (the value), `composed-stack` (five components).
  `community-grouping`: unchanged, first production caller.

## Impact

- Pin lines: 4,117 production, 6,716 test; this PR about 650 + 1,300 for the package, plus about 430 elsewhere
  (follower, smoke, contract), hand estimate. One PR.
- Exported: `follower.Entry.Value`, `readiness.KeyGraphClustering`; the package's `Register`, `CreateGraphClustering`,
  `DeclarePorts`, `Config` (`Validate`, `ApplyDefaults`), `DefaultConfig`; the collector
  `graph_clustering_publication_failures_total`. Changed text: the follower's `Stuck` sentence for failures.
- Consumer migration (`processor/graph-clustering/MIGRATION.md`): semsource's `enable_llm` and `entity_id_edges`, and
  semboids' `batch_size`, `min_community_size`, `entity_id_edges` and output ports, are refused at boot; semboids'
  spike expecting two communities from explicit edges reads one prefix group per source.
- Merge order: #583 first (task 3.1 holds on it).
- Retires #525, and #32 if question 2 is yes. Stays: #198, #141 (question 1), #485, #488, #477, #195, #196, #581.
