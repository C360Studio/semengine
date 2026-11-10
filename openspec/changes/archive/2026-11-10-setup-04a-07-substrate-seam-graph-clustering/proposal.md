# Proposal: setup-04a-07-substrate-seam-graph-clustering

Approved-by: owner (Coby Leuschke), 2026-11-10, PR #582 comment 6768832111 sha256:c8ee3104ece2

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
  `min_community_size`, `max_iterations`, `entity_id_edges`, `semantic_edges`, `allow_ungated_reads`,
  `enable_anomaly_detection`, `anomaly_config`.
- Anomaly detection is dropped for good (ruling above): the rest of `graph/inference` and `graph/structural` are never
  ported, and `ANOMALY_INDEX` (`graph.BucketAnomalyIndex` and its catalog row), which nothing would ever write, leaves
  the tree. The admission ledger records the drop.
- Not carried: label propagation and edge providers (ADR-099), the readiness gate (ADR-082), the four
  `graph.clustering.query.*` verbs (no reader), tier-2 summaries (#485).
- The composed smoke test adds graph-clustering: `localSearch` and a text question answer, across restarts.
- A contract test proves no package in the tree reaches `graph/llm`, `model/wire` or `go-openai` (04A D8).

## Rulings

Owner, 2026-11-10 (PR #582 comment 6768832111): anomaly detection (the k-core, missing-link and missing-transitive
detectors, `ANOMALY_INDEX`, the review worker) is never ported, and its config keys are refused at construction,
amending #97 design D1a, the ledger's `graph/inference` row and #198; #32's exit condition becomes "tier 0 builds
without the LLM plumbing, proven by the contract test", and #32 closes with this PR; the design is approved.

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
  spike expecting two communities from explicit edges reads one prefix group per source; `enable_anomaly_detection`
  and `anomaly_config` are refused; `graph.BucketAnomalyIndex` is gone (semsource names `ANOMALY_INDEX` only as a
  string in its cutover bucket list, `internal/cutover/buckets.go:35`).
- Merge order: #583 first (task 3.1 holds on it).
- Closes: #525 (task 2.3), #32 (task 3.2, ruling 2), #198 (task 4.1: its work is the rest of `graph/inference`, which
  ruling 1 drops).
- Stays: #488 (the tier-2 LLM review worker; ruling 1 is about tier-0 anomaly detection, not the tier-2 slot).
- Stays: #141 (its spatial and temporal sites remain; this PR removes only its `graph/inference/storage.go:159` site),
  #485, #477, #195, #196, #581.
