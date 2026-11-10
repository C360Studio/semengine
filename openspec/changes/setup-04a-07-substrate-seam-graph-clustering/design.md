# Design: `processor/graph-clustering`, the community writer

Base `4cb8d58` on `057bc8b`; pin `8b99efe9`. Inventory: `inventory.md` (§ n below). Ruling: #9 comment 6724212898
item 1. **Goal.** With graph-clustering in a composition, `COMMUNITY_INDEX` holds one bounded record per entity-ID
prefix group, kept current from `ENTITY_STATES` changes at tier 0, so `localSearch` and the text tier of
`searchGraph` answer. **Non-goals:** tier-2 summaries (#485), the review worker (#488), label propagation (ADR-099),
and, pending Q1, anomaly detection.

## Options for the feed (inventory § 2, § 6)

| Option | Cost | Verdict |
| --- | --- | --- |
| (a) Extend the follower: `Entry.Value` carries the delivered bytes | one field on plumbing | **chosen** |
| (b) Own `WatchAll` in graph-clustering | re-writes the follower's wait, lost feed, ordering check and replay end (shape 2, copied shell) | no |
| (c) Re-read current state per key, as graph-index | pairs a newer value with an older revision across keys; breaks `entity_revision` (`community-grouping` `:195-197`) | no |
| (d) The pin's timer rescan | ruled out: "kept current from changes, never by rescanning" (PR #483 comment 6543795826) | no |
| (e) Do nothing | localSearch and text questions refuse for ever; semsource's graph search tool goes through the text tier | no |

Why this is decided, not an owner question: the owner-approved 04A-04 design leaves exactly (a) or (b) to this change
("Change 7 runs its own watch or extends the follower",
`archive/2026-10-31-setup-04a-04-graph-roots-clustering/design.md:22-27`). ADR-108 D4's warning against applying the
delivered value (`108-processor-shell.md:111-118`) is about plug-ins with a repair lane racing the watch; this one has
one apply path, on the follower's goroutine, and the maintainer ignores a delivery at or below an entity's applied
revision. D5's "graph-clustering ... has no follower" (`:125-126`) describes the pin's timer loop, which the PR #483
ruling replaced. The `graph-clustering` key follows from ADR-088 D1 (one key per producer, as information).

## D1. Shell, feed and what the key means

The component composes `component.Lifecycle` and `follower.Open` with `Key: readiness.KeyGraphClustering`
(`"graph-clustering"`). `Deliver` decodes `Entry.Value` with `graph.UnmarshalEntityState` and calls `Upsert` (a value
that does not decode is `Upsert(id, rev, nil)`, the maintainer's refusal) or `Delete` for a marker, then `Complete`s
the revision at once.

So the key's `indexed_revision` means **held by the maintainer**, not published; the record's `entity_revision` is the
reader's mark, exact per group. The spec says so, and so does `IndexStatusResponse.IndexedRevision`'s doc
(`graph/index_status.go:62-65`), which today tells every reader to gate on `IndexedRevision >= myRev`. Completing only
after a successful publication was rejected: the follower's bootstrap latch sets when the floor reaches the replay
target (`entity-state-follower`, "The initial build completes at the snapshot's revision"), and the first publication
waits for that latch (D2), so the floor would never move; breaking the cycle needs a new follower signal for the replay
end, and buys a global floor less precise than the per-group mark readers already have.

## D2. Publication and failures

A ticker every `detection_interval` (default 30 s, the pin's) runs `Tick` under the run context. A tick starts only
while `follower.QueryReady` answers nil; the first such tick calls `BootstrapDone` first. So nothing is published
before the replay is applied, and once the follower reports a lost feed (a watch closed, a bucket recreated, a
revision out of order) or a latched reset, no later tick starts and `Health` names the state. Not promised: a tick
already running when the feed is lost finishes, and records published before the loss was detected may already lack
a write. A stored entity the replay refuses latches the follower's reset (the pin's sticky reset, `pin:doc.go:21-22`).

The component keeps its last tick's outcome. `Failed` returns the count and reasons from it, with reasons the
component names itself: `refused_entity` (the length of `*IncompleteError.Refused`, no new maintainer surface) and
`publication_failed` (1 while the last tick returned any other error). `Withhold` returns 0, so neither withholds
ticks. A tick that fails every time therefore reads `degraded` in the envelope, is a stuck state in `Health`
(`/readyz` not ready), and counts on `graph_clustering_publication_failures_total` on the injected registry, until a
tick succeeds (ADR-109 D2: a persistent failure is stuck while it lasts, `109:88-96`). The follower's `Stuck` sentence
becomes plug-in neutral: "N unresolved failures", followed by the reasons when the plug-in reports any
(`follower.go:373` said "index writes"). `Stop` joins the ticker and the follower.

## D3. Config, ledger, removed surface

Strict decoding. Kept: `ports` (one `ENTITY_STATES` input; outputs refused, as graph-embedding),
`detection_interval`, `startup_attempts`, `startup_interval_ms`. Refused at boot, naming the key with guidance
(ADR-099): `batch_size` (unread at the pin, `pin:doc.go:85`), `enable_llm`, `enhancement_workers` (tier 2, #485),
`min_community_size`, `max_iterations`, `entity_id_edges`, `semantic_edges` (ADR-099), `allow_ungated_reads` (no gate,
ADR-099), and pending Q1 `enable_anomaly_detection`, `anomaly_config`. Exported surface per inventory § 8 item 4. Not
carried: the four `graph.clustering.query.*` verbs, the readiness gate on graph-index and graph-embedding (ADR-082,
ADR-085), the enhancement and review workers, the indexing-profile check (ADR-054 Phase 1), Prometheus globals. New
`adapt` ledger row; `processor/graph-clustering/MIGRATION.md` for semsource and semboids.

## D4. #525 and the seam

Issue #525 is answered by what the follower does, not by a library promise: the watermark refuses a revision at or below
the highest observed and the follower declares the feed lost (existing requirement), which stops publication (D2). An
integration probe at the pinned nats.go and server image writes interleaved keys and records the order it observes in
`notes/2.3.md`, as an observation at that version (ADR-112). The seam: a tree-wide contract test in
`internal/harness/contract/embeddingprovider_test.go` asserts `go list -deps ./...` reaches no `graph/llm`,
`model/wire` or `go-openai`, reusing its listing and reader. It replaces none of the per-package guards, which also
forbid `model` (`:47-48`); it adds processor/graph-clustering to the direct-`model` guard graph-embedding has (`:51`).

## Proving tests

`TestConfigRefusesRemovedKeys`, `TestConfigStartupWait`, `TestDetectionIntervalIsRead`, `TestRecordsFollowChanges`,
`TestNothingPublishedBeforeReplay`, `TestRefusedValueReportedNotWithheld`, `TestPersistentPublicationFailureIsStuck`,
`TestLostFeedStopsPublication`, `TestStopLeavesNothing`, `TestEntryCarriesDeliveredValue`,
`TestTreeImportsNoProviderPlumbing`, and the composed stack with graph-clustering (`composed-stack` delta).

## Size and split

Pin 4,117 prod / 6,716 test (§ 1); almost none carries as is. This change, hand estimate: 700 + 1,400 package,
~60 + 120 follower, ~300 smoke, ~80 contract. One PR: one lifecycle owner, under the guide. If Q1 admits anomaly
detection now it is three more changes after this one, in order: `graph/structural` (883 / 736), `graph/inference`'s
detectors, storage and applier (3,568 / 2,097; #198), then graph-clustering's anomaly lane (786 / 363). #583 merges
first (task 3.1's hold).

## Guidance carried

The pin's contracts have no graph-clustering section (`grep -n -i "graph-clustering\|community\|anomal"
pin:.agents/contracts/*.md`: none). `.agents/skills/entity-or-bucket/SKILL.md:44-45` already says it. Edited:
`.agents/skills/query-pattern/SKILL.md:77` and `:99`, which say localSearch and text questions refuse until change 7
writes `COMMUNITY_INDEX`, become "until graph-clustering's first publication".
