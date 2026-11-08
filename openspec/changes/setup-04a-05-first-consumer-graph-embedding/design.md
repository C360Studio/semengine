# Design: `processor/graph-embedding` step 1, the write path

Base `73a25e1` (#548 merged as `9e3246f`); pin `8b99efe9`. Owner rulings: PR #550 comment 6716972354 (split, G10,
fusion readiness, probes). Probes: `inventory.md` § 0. **Goal.** With graph-embedding in a composition, every entity
with text gets a BM25 record in `EMBEDDING_INDEX`, the `graph-embedding` readiness key tells the truth, and nothing
reaches a provider. **Non-goals:** query verbs and the vector cache (PR B), fusion (C, D), tier 1 (#489).

Pin evidence: "unit" ran in the owner's probes (`-race` once and 5x shuffled, rc=0); "integration" is
`//go:build integration` and did not run. Paths are `pin:processor/graph-embedding/`.

## D1. Shell and follower (ADR-108, #109, #196)

The component composes `component.Lifecycle` and opens `follower.Open` with `Key: readiness.KeyGraphEmbedding`, as
graph-index does (`processor/graph-index/component.go:236-245`). Hop 1 is `Deliver`: it reads the entity, decoded
with `graph.UnmarshalEntityState`, and writes a pending record. A revision is `Complete`d when the Worker reports its
terminal outcome for that source revision, or at once when hop 1 ends it (no text, deleted, refused): the pin's
two-hop rule (`readiness.go:38-55`, ADR-066 § 3; pin test
`readiness_test.go:TestCompleteEmbedding_TerminalOutcomesAdvanceWatermark`,
unit; `embedding_lifecycle_test.go:TestEntityTombstoneCompletesWatermarkWhenDeleteFails`, unit). With
`coalesce_ms > 0`, completing a coalesced entity at the revision read drains every lower revision of that key
(`internal/revlag/watermark.go:100-110` on main; pin `component.go:1619`; pin test
`derived_state_convergence_integration_test.go:TestIntegration_CoalescedLane_TombstoneConverges`, integration).
After #548 the follower refuses a non-ascending revision and declares the feed lost (`graph/follower/follower.go:267`);
the plug-in sees no change. *Proof:* `TestReadinessWaitsForHopTwo`, `TestCoalescedRevisionsComplete`, `-race`.

**When the envelope reads degraded (owner question).** The follower reads degraded when the floor has not risen for
30 s while behind (`graph/follower/follower.go:77`, `:471-479`). The pin's embedding producer instead read degraded
only after 120 s with no terminal completion at all (pin `readiness.go:23-27`, `:192-212`; ADR-066 § 3,
`docs/adr/066-honest-graph-index-readiness.md:312-315`; pin test
`readiness_test.go:TestTrackEmbeddingProgress_CompletionsBasedStuckDetector`, unit). The difference shows when one
low revision waits, say on a slow offloaded-body read, while later entities keep finishing: the floor stands still,
work does not. Options:

- **(a1) recommended:** the follower counts its own `Complete` calls and treats any completion in the window as
  progress, for every plug-in; no new config field, threshold stays 30 s (the pin's 120 s was for the external model
  hop, which tier 0 does not have; tier 1 revisits it, #489). Cost: graph-index changes the same way; a held low
  revision there reads not ready with `staleness_ms` rising (the oldest unapplied write's age since #548), but not
  degraded.
- **(a2):** an optional plug-in progress signal in `follower.Config`; graph-embedding passes its completion count,
  graph-index keeps the floor rule. Cost: one more field and two meanings of "stuck".
- **(b):** keep the floor rule for graph-embedding, a named ADR-066 § 3 deviation. Cost: during an ingest with
  offloaded bodies, one read slower than 30 s marks the embedding key degraded while work flows, and SemSource's MCP
  gateway shows that to agents.

*Proof (any option):* `TestHeldLowRevisionWithProgress` (one revision held, later ones completing past the threshold:
not ready; degraded under (b) only) and `TestNoCompletionsReadsDegraded` (nothing completes past the threshold:
degraded).

## D2. The Worker under the run context; a lost watch

Ledger `graph/embedding` known_risks. `Worker.Run` runs under the one run context `Start` creates; `Stop` returns
after `Run` has. If `Run` returns while that context is live, `Health` names `embedding worker watch lost`, `Healthy`
is false until the process restarts; nothing rebinds the watch, as for the follower's lost feed
(`graph/follower/follower.go:11-13`). No pin counterpart (the pin Worker had `Start`/`Stop`). *Proof:*
`TestHealthNamesLostWorkerWatch` (integration), `TestStopJoinsWorker`.

## D3. The failed map and its breakdown (owner, G10)

Per entity it holds the latest terminal outcome's source revision and its reason. A failure for revision r is stored
only when r is at or above it; a success at r clears any failure at or below r. A failed delete of an absent
entity and a failed entity read have no source revision, so they are stranded at the highest revision
(`^uint64(0)`): no terminal outcome from the Worker clears them, only a later successful hop-1 convergence of that
entity
by the repair lane: a pending record written, a guarded result that finds it already generated, or the record
removed (pin `component.go:1628-1650`, `:1694-1701`, SemStreams #722 B1). A read that succeeds while the following
pending write or stale-vector delete fails re-marks the failure (pin `:1864-1884`, `:1911-1941`). Seeded from
`Storage.ScanFailed`.
`follower.Config.Failed` becomes `func() (count int64, reasons map[string]uint64)`, read once per envelope so count
and reasons agree (pin `readiness.go:149-152`). `Failed` returns a fresh map the follower owns; the plug-in keeps
no reference to it, since the follower reads it on its heartbeat while Worker callbacks change the failed map. A
second optional `FailedReasons` field would leave existing adopters untouched but could pair a count with reasons
from a different moment; the signature change costs each adopter one line returning `nil` reasons (graph-index here,
spatial and temporal when they port, #109); the follower sets `FailedReasons` when count > 0. graph-index returns
`nil` reasons, so its envelope is unchanged. `first_failure_at` is not set, and this PR removes the field:
`graph/index_status.go:85` and its test fill at
`index_status_test.go:59` (owner, #110 2026-11-08 and PR #550 comment 6716972354; `MIGRATION.md`). Pin tests:
`failed_map_test.go:TestCurrentFailedMap_TracksReasonsAndGauge` (unit);
`integration_test.go:TestIntegration_EmbeddingReadiness_DependencyDownDegradesThenRecovers` (integration). The repair
lane (`component.go:1266-1340`) retries `delete_failed`, `pending_write_failed`, `entity_read_failed` every 30 s and
stops with `Stop` (pin tests `derived_state_convergence_test.go:TestFailedDelete_MarksStrandedAndRepairs`,
`TestRepairScope_EmbedderSideReasonNotReDriven`, `TestObsoleteTerminal_CannotClearStranding_PendingWriteSite`, unit).
*Proof:* `TestFailedMapHistoryMatchesModel` (generated histories against a reference model, seed recorded),
`TestEnvelopeCarriesFailedReasons` (under `-race`, with outcomes reported concurrently with heartbeats),
`TestGraphIndexEnvelopeUnchanged`, `TestRepairConvergesStranded` including read success with write failure,
`TestStrandedWithoutRevisionClearsOnlyOnRepair` (ported from
`TestObsoleteTerminal_CannotClearStranding_PendingWriteSite` and `_TombstoneSite`), `TestRepairConvergesStranded`.

## D4. Config

Kept, each with a test that fails when ignored: `ports`, `embedder_type` (`bm25` only; other values refused naming
the value and tier 0), `workers`, `max_text_len`, `text_suffixes`, `startup_attempts`, `startup_interval_ms`,
`coalesce_ms`. Dropped: `batch_size`; the pin reads it nowhere (`grep -n BatchSize`: `component.go:77,133,170-171,212`
only; its one pin test, `workers_config_test.go`, checks only a round-trip). Unknown keys refused, naming the key.
*Proof:* `TestConfigRefusesUnknownKey`, `TestConfigRefusesHTTPEmbedder`, one test per kept key.

## D5. Text lanes

Inline text from `text_suffixes`; offloaded bodies through `storage/storeregistry` with the identity text ahead of
the body (ADR-063, `component.go:1954-2062`; pin tests
`storageref_fallback_test.go:TestQueueEntityForEmbedding_UnresolvedBodyWithoutInlineTextSkipsAndDeletesStaleVector`,
`TestReportOffloadedContentExcluded_LoudNotSilent`, unit;
`offloaded_identity_integration_test.go:TestIntegration_OffloadedEntityEmbedsIdentityAlongsideBody`, integration).
The indexing-profile check is not carried: every path returns true (`component.go:1769-1779`; pin test
`indexing_profile_test.go:TestIndexingEligible_Phase1IsAlwaysEligible`, unit); ADR-054 is not ported.
*Proof:* `TestStorageRefLaneEmbedsIdentityThenBody`, `TestOffloadedBodyMissingIsDeclared`.

## D6. Health, readiness, tier 0

`Health` is `Lifecycle.Health(stuck...)` with the follower's `Stuck()` and D2's state; no error-count term (ADR-109
D2; pin `component.go:567`). No status verb (ADR-083, ADR-109 D3; pin `query.go:39`). No `model`, no `go-openai`.
*Proof:* `TestHealthIgnoresErrorCount`, `TestNoStatusVerb`, `TestGraphEmbeddingImportsNoProviderModule`.

## D7. Tests carried

Rewritten on `natsfixture.Open` (#195), with no `time.Sleep`, `Skip` or deadline of their own; owner-proof lifecycle
tests are not carried.

## D8. ADRs, issues, order

Applied: ADR-063, 066 § 3 (two-hop completion; its stuck criterion is the owner question in D1), 083 (D3 as
amended by #548), 108, 109 D2/D3, 113. Not ported: ADR-054 (D5), 055 (comment
only). Changed by owner ruling: the envelope's failure breakdown keeps reasons and loses `first_failure_at` (G10,
beside #110's 2026-11-08 ruling). Issues: inventory § 9 and `proposal.md`. Order: #548 is merged; this PR next.
Guidance: SemStreams' "semantic identity and graph" and "storage and retention" contract sections and the
`entity-or-bucket` skill apply.

Integration-only pin evidence (not run by the probes): `TestIntegration_CoalescedLane_TombstoneConverges`,
`TestIntegration_EmbeddingReadiness_DependencyDownDegradesThenRecovers`,
`TestIntegration_OffloadedEntityEmbedsIdentityAlongsideBody`.
