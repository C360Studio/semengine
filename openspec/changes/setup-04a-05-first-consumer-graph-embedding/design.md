# Design: `processor/graph-embedding` step 1, the write path

DRAFT. Not reviewable until `inventory.md` has INVENTORY PASS and its § 0 pin probes are run. Base `d4d9eae`; pin
`8b99efe9`. **Goal.** With graph-embedding in a composition, every entity with text gets a BM25 record in
`EMBEDDING_INDEX`, the `graph-embedding` readiness key tells the truth, and nothing reaches a provider. **Non-goals:**
query verbs and the vector cache (PR B), fusion (C, D), tier 1 (#489).

## D1. Shell and follower (ADR-108, #109, #196)

The component composes `component.Lifecycle` and opens `follower.Open` with `Key: readiness.KeyGraphEmbedding`, as
graph-index does (`processor/graph-index/component.go:236-245`). Hop 1 is `Deliver`: it reads the entity, decoded
with `graph.UnmarshalEntityState` (#109 comment 2026-10-17), and writes a pending record. A revision is `Complete`d
when the Worker reports its terminal outcome for that source revision, or at once when hop 1 ends it (no text,
deleted, refused): the pin's two-hop rule (`pin:readiness.go:38-55`, ADR-066 § 3). With `coalesce_ms > 0`, a
coalesced entity completes every revision of its key up to the one read, as the pin did (`pin:component.go:1619`).
*Proof:* `TestReadinessWaitsForHopTwo` (envelope not caught up until the Worker's terminal callback),
`TestCoalescedRevisionsComplete`, `-race`.

## D2. The Worker under the run context; a lost watch

Ledger: `graph/embedding` known_risks.

`Worker.Run` runs under the one run context `Start` creates; `Stop` returns after `Run` has. If `Run` returns any
error while that context is live, `Health` names the stuck state `embedding worker watch lost`, `Healthy` is false
and stays false until the process restarts; nothing rebinds the watch. This mirrors the follower's lost feed
(`graph/follower/follower.go:11-13`). Option not taken: restart `Run` in a loop, which adds a retry policy no other
watch in the tree has. *Proof:* `TestHealthNamesLostWorkerWatch` (integration; the trigger is the developer's),
`TestStopJoinsWorker`.

## D3. The failed map

Per entity it holds the latest terminal outcome's source revision. A failure for revision r is stored only when r is
at or above it; a success at r clears any failure below or at r. Seeded from `Storage.ScanFailed` at start.
`follower.Config.Failed` reads its size. The
per-reason breakdown and first-failure time (pin `readiness.go:152-171`) follow the owner's answer on G10: either
the follower's `Failed` returns them and the envelope carries them, or they are dropped with #110's ruling and named
in `MIGRATION.md`; never dropped silently. The repair lane (`pin:component.go:1266-1340`) retries entities whose
reason is `delete_failed`, `pending_write_failed` or `entity_read_failed`, every 30 s, and stops with `Stop`.
*Proof:* `TestFailedMapHistoryMatchesModel` (generated histories of interleaved outcomes against a reference model,
seed recorded), `TestRepairConvergesStranded`.

## D4. Config

Kept, each with a test that fails when ignored: `ports`, `embedder_type` (`bm25` only; any other value refused at
construction, naming the value and tier 0), `workers`, `max_text_len`, `text_suffixes`, `startup_attempts`,
`startup_interval_ms`, `coalesce_ms`. Dropped: `batch_size` (inventory G4); unknown keys refused, naming the key.
*Proof:* `TestConfigRefusesUnknownKey`, `TestConfigRefusesHTTPEmbedder`, one test per kept key.

## D5. Text lanes

Inline text from `text_suffixes`; offloaded bodies through `storage/storeregistry` with the identity text ahead of
the body (ADR-063, `pin:component.go:1954-2062`). The indexing-profile check is not carried: every pin path returns
true (`:1769-1779`), so behaviour is unchanged; ADR-054 is not ported, since no carried code implements it.
*Proof:* `TestStorageRefLaneEmbedsIdentityThenBody`, `TestOffloadedBodyMissingIsDeclared`.

## D6. Health and readiness

`Health` is `Lifecycle.Health(stuck...)` with the follower's `Stuck()` and D2's state. No error-count term (ADR-109
D2). No status verb: the follower publishes the key (ADR-083, ADR-109 D3). *Proof:* `TestHealthIgnoresErrorCount`,
`TestNoStatusVerb` (`graph.embedding.query.status` reports no responders).

## D7. Tier 0

The package reaches neither `model` nor `go-openai`. *Proof:* `TestGraphEmbeddingImportsNoProviderModule`, modelled on
`embedding-index` "Tier 0 imports no provider code".

## D8. Tests carried

Pin tests are rewritten on `natsfixture.Open` (#195), without `time.Sleep`, `Skip` (inventory § 0) or a deadline of
their own. Owner-proof lifecycle tests are not carried; the lifecycle suite covers the shell.

## D9. ADRs, issues, order

Applied: ADR-063, 066 § 3, 083, 108, 109 D2/D3, 113. Not ported: ADR-054 (D5), 055 (comment only). Changed for the
owner: the failure breakdown (G10, beside #110's 2026-11-08 ruling). Issues: inventory § 9 and `proposal.md`.
Merge order: #548, then this PR (merging `origin/main`; the
follower's `Observe` error and the oldest-unapplied staleness are #548's). Guidance: SemStreams' "semantic identity
and graph" and "storage and retention" contract sections and the `entity-or-bucket` skill apply; the developer
carries any part not yet in `.agents/contracts/semengine-developer.md`.
