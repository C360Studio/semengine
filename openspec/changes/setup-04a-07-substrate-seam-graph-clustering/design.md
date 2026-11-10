# Design: `processor/graph-clustering`, the community writer

Base `9992088` on `057bc8b`; pin `8b99efe9`. Inventory: `inventory.md` (§ n below). Ruling: #9 comment 6724212898
item 1. **Goal.** With graph-clustering in a composition, `COMMUNITY_INDEX` holds one bounded record per entity-ID
prefix group, kept current from `ENTITY_STATES` changes at tier 0, so `localSearch` and the text tier of
`searchGraph` answer. **Non-goals:** tier-2 summaries (#485), the review worker (#488), label propagation (ADR-099),
and, pending Q1, anomaly detection.

## Options for the feed (inventory § 2, § 6)

| Option | Cost | Verdict |
| --- | --- | --- |
| (a) Extend the follower: `Entry.Value` carries the delivered bytes | one field on plumbing; ADR-108 D4/D5 change for this plug-in (Q2) | **recommended** |
| (b) Own `WatchAll` in graph-clustering | re-writes the follower's wait, lost feed, ordering check and replay end (shape 2, copied shell) | no |
| (c) Re-read current state per key, as graph-index | pairs a newer value with an older revision across keys; breaks `entity_revision` (`community-grouping` `:195-197`) | no |
| (d) The pin's timer rescan | ruled out: "kept current from changes, never by rescanning" (PR #483 comment 6543795826) | no |
| (e) Do nothing | localSearch and text questions refuse for ever; semsource's graph search tool goes through the text tier | no |

Why (a) is safe where ADR-108 D4 warns against applying the delivered value: graph-clustering has one apply path, on
the follower's goroutine, with no repair lane, and the maintainer ignores a delivery at or below an entity's applied
revision (`community-grouping`, "Applying a change depends only on the latest revision").

## D1. Shell and feed

The component composes `component.Lifecycle` and `follower.Open` with `Key: readiness.KeyGraphClustering`
(`"graph-clustering"`). `Deliver` decodes `Entry.Value` with `graph.UnmarshalEntityState` and calls `Upsert` (a value
that does not decode is `Upsert(id, rev, nil)`, the maintainer's refusal) or `Delete` for a marker, then `Complete`s
the revision at once: the envelope counts a revision indexed when the maintainer holds it; `COMMUNITY_INDEX` follows
within one interval, and each record's `entity_revision` is the reader's mark. `Failed` returns the refused-entity
count with reasons; `Withhold` returns 0, so a refused entity is reported, not withholding.

## D2. Publication

A ticker every `detection_interval` (default 30 s, the pin's) runs `Tick` under the run context while
`follower.QueryReady` answers nil; the first such tick calls `BootstrapDone` first. So: nothing is published before the
replay is applied; after a lost feed (a watch closed, a bucket recreated, a revision out of order, which the watermark
refuses) or a latched reset no tick publishes, the records stay at the last honest publication, and `Health` names the
state. A stored entity the replay refuses latches the follower's reset, so nothing is published until the
operator clears it: the pin's sticky whole-view reset (`pin:doc.go:21-22`), as graph-index refuses every query.
A failed tick (write error, `*IncompleteError`) is logged and retried on the next tick; it is not an error count in
`Health` (ADR-109 D2). `Stop` joins the ticker and the follower; nothing is left.

## D3. Config, ledger, removed surface

Strict decoding. Kept: `ports` (one `ENTITY_STATES` input; outputs refused, as graph-embedding),
`detection_interval`, `startup_attempts`, `startup_interval_ms`. Refused at boot, naming the key with guidance
(ADR-099): `batch_size` (unread at the pin, `pin:doc.go:85`), `enable_llm`, `enhancement_workers` (tier 2, #485),
`min_community_size`, `max_iterations`, `entity_id_edges`, `semantic_edges` (ADR-099), `allow_ungated_reads` (no gate,
ADR-099), and pending Q1 `enable_anomaly_detection`, `anomaly_config`. Not carried: the four `graph.clustering.query.*`
verbs (dead, § 3), the readiness gate on graph-index and graph-embedding (ADR-082, ADR-085's one `FreshnessWithin`),
the enhancement and review workers, the indexing-profile check (ADR-054 Phase 1, as graph-embedding), Prometheus
globals (`metric-registry`). New `adapt` ledger row; `processor/graph-clustering/MIGRATION.md` for semsource and semboids.

## D4. #525 and the seam

Issue #525 retires with two proofs: a probe at the pinned nats.go and server image that writes interleaved keys and asserts
the watch delivers ascending revisions across keys, and the D2 rule that a refused order stops publication. The seam
(#32): after this port no tier-0 package at the pin imports `graph/llm` or `model/wire`; a contract test asserts
`go list -deps ./...` holds neither nor `go-openai` (04A D8 exit condition).

## Proving tests

`TestConfigRefusesRemovedKeys` (one case per key), `TestNothingPublishedBeforeReplay`, `TestRecordsFollowChanges`
(upsert, edit, delete, group emptied), `TestRefusedValueReportedNotWithheld`, `TestLostFeedStopsPublication`,
`TestWatchDeliversAscendingAcrossKeys` (integration, #525), `TestStopLeavesNothing` (lifecycle suite, `synctest`),
`TestHealthIgnoresTickFailures`, `TestTreeImportsNoProviderPlumbing` (contract), and the composed stack: graph-clustering
joins, `localSearch` answers the lead's level-0 group, a text question answers, and the restart scenarios keep them
(`composed-stack` delta).

## Size and split

Pin 4,117 prod / 6,716 test (§ 1); almost none carries as is. This change, hand estimate: 650 + 1,300 package,
~40 + 80 follower, ~250 smoke, ~60 contract. One PR: one lifecycle owner, under the guide. If Q1 admits anomaly
detection now it is three more changes after this one, in order: `graph/structural` (883 / 736), `graph/inference`'s
detectors, storage and applier (3,568 / 2,097; #198), then graph-clustering's anomaly lane (786 / 363). Merge #583 first.

## Guidance carried

The pin's contracts have no graph-clustering section (`grep -n -i "graph-clustering\|community\|anomal"
pin:.agents/contracts/*.md`: none). `.agents/skills/entity-or-bucket/SKILL.md:44-45` already says it. Edited:
`.agents/skills/query-pattern/SKILL.md:77` and `:99`, which say localSearch and text questions refuse until change 7
writes `COMMUNITY_INDEX`, become "until graph-clustering's first publication".
