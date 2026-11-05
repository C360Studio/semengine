# Design: `processor/graph-query` step 3, community reads and local search

Base `b940e29`; pin `8b99efe9`. Evidence: archived `inventory.md` §§ 1, 11, 13; `inventory-delta.md`; `notes/probes/`.
Binding: PR #504 comments 6582751320, 6582751783, 6582752155 (rulings 1-3), 6585009466, 6588079803. **Goal.** A
community read answers from the latest record, says when it searched a sample, or refuses. **Non-goals:** steps 4, 5,
enumeration, change 7's writer wiring. Pin about 880 + 820 lines, this about 450 + 1,000 (estimate); one PR.

## D1. The record's revision (the one new field)

Ruling 1 left the community verbs' `indexed_revision` open: no record carries an `ENTITY_STATES` revision
(`graph/clustering/types.go:26-51`), and the view's `Applied()` is `COMMUNITY_INDEX`'s.

| Option | Cost |
| --- | --- |
| (a) The maintainer's high mark at build | True and larger, but a record's bytes depend on when ticks ran, so no model computes it from a history |
| **(b) The latest write applied to the group** | A function of the history alone; the model carries it |
| (c) The highest current member's revision | A delete is invisible: after deleting E at W the group may never reach W, so read-your-delete refuses for ever |
| (d) No field; `localSearch` reports 0 | Claims nothing, against ruling 1's list; a caller comparing never sees it catch up |
| (e) A published tick watermark | No stuck case (D2), but ruling 3 says no tick marker; a key outside `{level}.{id}` that `GetAllCommunities` refuses |

**(b).** `entity_revision` is the highest revision of an `Upsert` or `Delete` applied for any entity whose ID parses
into the group, held or not, refused or not; a delivery at or below the entity's last-applied revision is not applied.
So a delete of an entity never held, such as a replayed delete marker after a restart, still raises it, unlike (c).
The mark ends with the group: a recreated group starts at the write that recreated it. A raised record differs, so the
next tick writes it. *Truth:* the caller SHALL feed the maintainer in ascending revision order across keys; then every
write at or below `entity_revision` is reflected. That a KV watch delivers that order is not promised here: ADR-066
states it as carried text, unprobed at the pinned versions (ADR-112); change 7, which wires the watch, probes it.
*Costs:* a write that changes no count, word or representative now writes its records once; a purged delete comes back
lower after a restart, still true. *Proof:* `TestRecordNamesItsLatestWrite` (upsert, same-content upsert, held and
unheld delete, obsolete, refused, a group emptied and recreated); `TestUnchangedRecordAddsNoRevision` (nothing applied,
no write); `TestMaintainerMatchesModel` on its ascending histories, model from the spec; `TestHistoryOrderIrrelevant`,
whose histories keep only each entity's order, comparing records apart from `entity_revision`.

## D2. `indexed_revision` and `min_revision` on `localSearch`

`indexed_revision` is the record's `entity_revision`; the page, read after it from the authority, reflects at least as
much (ruling 1). A `min_revision` above it is refused `revision_not_reached` (transient, detail `indexed_revision`)
before the page is read. **Stuck case:** a `min_revision` from a write outside E's group stays above the record until
the group's next write; a level-2 group catches up, a quiet level-0 one may never. Docs: send a revision of a write to
the group searched. Owner question 1; (e) fixes it. *Proof:* `TestLocalSearchReadsItsWriteBack`; `localSearch` joins
`TestMinRevisionRefusalThroughGraphQuery`.

## D3. The community view

graph-query gets its first run context, from `Start` and ended in `cleanup` (ADR-113), as graph-index's
(`processor/graph-index/component.go:258-283`). What a caller sees:

- `Start` neither waits for nor fails on a missing `COMMUNITY_INDEX`, whose writer is not yet wired
  (`graph/kvcatalog/kvcatalog.go:188`). Until it exists `localSearch` refuses `index_not_ready`, the other verbs answer,
  and `Health` is the shell's.
- Once the bucket exists, `localSearch` answers without a restart. As at the pin (`pin:component.go:563-576`,
  `:710-750`), one supervisor opens the reader, runs a view until it returns, waits 5 s (the pin's `recheck_interval`
  default, `:100-103`; the key stays refused) and opens again. Not `internal/resource.Watcher`: it watches the bucket,
  not the watch, so a view lost while the bucket stays would never reopen, and its callbacks need a second goroutine.
- A lost watch refuses `index_not_ready`, never the last projection (ADR-081); a new view serves once caught up. During
  a tick each group answers from its latest record, without waiting or refusing (ruling 3).
- `Stop` drains the subscriptions and returns after the view's `Run` has; a `Stop` whose context ends first returns
  its error and the next resumes.
- Readiness is per request through the view, never `GRAPH_STATUS` or `COMMUNITY_SUMMARIES` (ADR-087 Decision 5).

*Proof:* `TestLocalSearchBeforeCommunityIndexExists`, `TestLocalSearchAfterCommunityIndexAppears` (waits up to one
interval), `TestLocalSearchRefusesAfterWatchLoss`, `TestStopEndsTheCommunityView`, the lifecycle suite, under `-race`;
`TestLocalSearchDuringBlockedTick`: one group's next record written straight to the bucket, as a tick held between two
writes leaves it (only `clustering`'s fake holds a real tick, `maintainer_test.go:515`); the written group answers from
its new record, the other from its old.

## D4. Request and reply

`graph.LocalSearchRequest` decodes strictly: `entity_id` passes `ValidateEntityID`; `level` 0 to 2, omitted 0
(`GroupKey` refuses others); `query` lowercased, split on white space, repeats removed, 1 to `MaxSearchTerms` (16)
terms; `min_revision`. Each failure is `invalid_request` naming the field, before any read. *16:* each term scans every
entity's text, about 16 MB at a 1 MB page; the pin took any length (arithmetic). `data` is `graph.LocalSearchData`:
the record's `community_id`, `level`, `member_count`, `community_summary`, `keywords`; `entities`, the matches, best
first, each the page's bytes unchanged; `entities_scanned`, distinct entities read; `entities_truncated`; `answer`.
Dropped from the pin: `communityId` (now snake case), `count`, `durationMs`, `entity_digests` (the entities' ID and
type again; back with step 5's summary mode), `answer_model`, `degraded`, `degraded_reason` (nothing degrades at tier
0). Enveloped (ruling 1).

## D5. One page, ranked

After the record, one `graph.ingest.query.prefix` request at `MaxPrefixQueryLimit` for the record's ID (ruling 2),
through step 2's page checks; a repeated ID (#512) counts once. The score is the pin's (`pin:graphrag.go:2360-2397`):
per term 1.0 if it equals the type position, else 0.5 if it is a substring of a predicate or a string value, over the
term count; kept at 0.3 or above (constant; `min_text_relevance` stays refused). Probe: two terms with one text match
score 0.25 and are dropped; a repeated term counted twice (0.33), now once; a predicate name matches (carried). Order:
score, then entity ID; the pin's `sort.Slice` is not stable (40 ties kept input order once; not promised). A reply over
`max_payload` (`natsclient.Client.MaxPayload`) leaves out the lowest-ranked matches until it fits and sets
`entities_truncated`. The answer is a plain function (owner, Q6), literally `Found {n} matching entities among
{entities_scanned} read from {community_id}, which has {member_count} members.`, `{n}` counting the entities in the
reply, after any cut; then the record's summary if set, then `Key themes:` and the first five keywords. The pin printed
"(7 entities, 0% match)" (probe), a relevance local search never computes. *Proof:* `TestLocalSearchReadsOnePage`
(ruling 2's: one prefix request; a later match absent, truncated; a fitting group equals an independent filter), the
scoring, terms, order and answer tests, `TestLocalSearchEntitiesPassUnchanged`, `TestLocalSearchFitsMaxPayload`.

## D6. Refusals and counting

| Case | Caller receives | Error in `Health` |
| --- | --- | --- |
| Bad request | `invalid_request` | no |
| No view, catching up, lost, stopped | transient `index_not_ready` | no |
| No record, E exists (entity verb) | transient `index_not_ready` | no |
| No record, E absent | the entity verb's refusal as it arrived | no |
| Record unreadable (poison) | transient `internal` naming the key; heals on a valid write | yes |
| `min_revision` above the record | transient `revision_not_reached`, detail | no |
| Page broken or no responder | transient `internal`, as step 2 | yes |

A present record costs no entity read. A keyed read follows ruling 3's sentence for one entity; its empty-level
sentence is enumeration's (step 5). On an empty level an existing entity is still `index_not_ready`; an absent one
gets the entity verb's refusal, since no index state makes it exist. A truncated answer adds one to
`semengine_graph_query_search_truncated_total{verb}` through `metric.RegisterOrGet`'s returned collector (no registry,
none); the pin's `metrics.go` (`sync.Once`, global fallback: #68's site) is not carried. *Proof:* tasks 3.2, 3.3.

## D7. What leaves step 3, and ADRs

Not carried: generation, lease, fence, community cache (ruling 3; #406 retires); the `graph.clustering.query.entity`
fallback that swallowed failures (S15); the semantic fallback, since ruling 3 decides its case, no record (D6); the
answer interface (Q6). Moved to their reader, step 5: the poison-counting `List` (`Get` reports poison,
`pkg/graphview/view.go:403-412`), labels and digests (#31). ADR-087 ported with a note, Decisions 2 and 5 kept, the
store's with #485. ADR-081, ADR-085 (`TestLocalSearchReadsTheAskedLevel`), ADR-109 D4, ADR-113 applied. ADR-099's
"shape-for-shape" yields to the 2026-11-02 ruling, cited in its port note. Retires #406, #522.
