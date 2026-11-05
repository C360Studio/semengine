# Design: `processor/graph-query` step 3, community reads and local search

Base `b940e29`; pin `8b99efe9`. Evidence: archived `inventory.md` §§ 1, 11, 13; `inventory-delta.md`;
`notes/probes/step3-pin-probe.*`. Binding: PR #504 comments 6582751320 (ruling 1), 6582751783 (ruling 2), 6582752155
(ruling 3), 6585009466, 6588079803. **Goal.** A community read answers from the latest record, says when it searched a
sample, or refuses. **Non-goals:** steps 4 and 5, enumeration, change 7's writer wiring. Pin about 825 + 820 lines,
this about 450 + 1,000 (estimate); one PR.

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

**(b).** `entity_revision` is the highest revision of an `Upsert` or `Delete` the maintainer applied that changed a
contribution in the group, so a touched group's record differs and is written at the next tick. *Truth:* when the
maintainer is fed one watch in ascending revision order, every write at or below it is reflected. A KV watch delivers
that order (ported ADR-066, "Why it is correct"; `internal/revlag` rests on it). Fed per key only, as the spec now
allows, counts stay right and the field is a best effort. *Costs, declared:* a write that changes no count, word or
representative now writes its three records once (`TestUnchangedRecordAddsNoRevision` changes its second half); a tick
with nothing applied still writes nothing. A refused first value touches no group, so nothing advances, and `Tick`
names it. After a restart a group whose last write was a purged delete comes back lower: lower, still true. *Proof:*
`TestRecordNamesItsLatestWrite` (upsert, same-content upsert, delete, obsolete delivery, refused first value);
`TestMaintainerMatchesModel`, model extended from the spec; `TestHistoryOrderIrrelevant` unchanged.

## D2. `indexed_revision` and `min_revision` on `localSearch`

`indexed_revision` is the record's `entity_revision`. The page is read after the record from the authority, so it
reflects at least as much (ruling 1). A `min_revision` above it is refused `revision_not_reached`, transient, detail
`indexed_revision`, before the page is read. **The stuck case:** a `min_revision` from a write to no member of E's
group at that level stays above the record until the group's next write. A level-2 group takes every write of its
deployment, so it catches up; a quiet level-0 group can refuse indefinitely. The doc says to send the revision of a
write to the group searched. Owner question 1; (e) is the fix if a caller needs it. *Proof:*
`TestLocalSearchReadsItsWriteBack` (write at W, before and after a tick); `localSearch` joins
`TestMinRevisionRefusalThroughGraphQuery`.

## D3. The community view

graph-query gets its first run context, from `Start` and ended in `cleanup` (ADR-113), as graph-index's
(`processor/graph-index/component.go:258-283`). What a caller sees:

- `Start` neither waits for nor fails on a missing `COMMUNITY_INDEX`: its writer is graph-clustering's, not yet wired
  (`graph/kvcatalog/kvcatalog.go:188`). Until it exists `localSearch` refuses `index_not_ready` and every other verb
  answers. `Health` is the shell's.
- Once the bucket exists, `localSearch` answers without a restart. When to reopen is the developer's: on an interval
  or on demand, never retrying without waiting, no config key.
- A lost watch (the bucket deleted) refuses `index_not_ready`, never the last projection (ADR-081); a new view serves
  once caught up.
- `Stop` drains the subscriptions, then returns only after the view's `Run` has; a `Stop` whose context ends first
  returns its error and the next resumes.
- Readiness is per request through the view, never `GRAPH_STATUS` or `COMMUNITY_SUMMARIES` (ADR-087 Decision 5).

*Proof:* `TestLocalSearchBeforeCommunityIndexExists`, `TestLocalSearchAfterCommunityIndexAppears`,
`TestLocalSearchRefusesAfterWatchLoss` (deleted bucket, as `pkg/graphview`'s `TestRunReportsLostWatch`),
`TestStopEndsTheCommunityView`, and the lifecycle suite with the view running, all under `-race`.

## D4. Request and reply

`graph.LocalSearchRequest` decodes strictly: `entity_id` passes `ValidateEntityID`; `level` 0 to 2, omitted 0
(`GroupKey` refuses others); `query` lowercased, split on white space, repeats removed, 1 to `MaxSearchTerms` (16)
terms; `min_revision`. Each failure is `invalid_request` naming the field, before any read. *16:* each term is a
substring scan of every entity's text, so a 1 MB page costs about 16 MB of scanning; the pin took any length
(arithmetic). Raising later breaks nobody.

`data` is `graph.LocalSearchData`: `community_id`, `level`, `member_count`, `community_summary`, `keywords` (the
record's); `entities`, the matches, best first, each the page's bytes unchanged, so a value graph-query does not read
survives; `entities_scanned`, distinct entities read; `entities_truncated`; `answer`. Dropped from the pin:
`communityId` (snake case now), `count` (`len(entities)`), `durationMs`, `entity_digests` (the entities' ID and type
again; digests return with step 5's summary mode), `answer_model`, `degraded`, `degraded_reason` (nothing degrades at
tier 0). Enveloped (ruling 1).

## D5. One page, ranked

After the record, one `graph.ingest.query.prefix` request at `MaxPrefixQueryLimit` for the record's ID (ruling 2),
through step 2's page checks; a repeated ID (#512) counts once. The score is the pin's (`pin:graphrag.go:2360-2397`):
per term 1.0 if it equals the type position, else 0.5 if it is a substring of a predicate or a string value, over the
term count; kept at 0.3 or above (constant; `min_text_relevance` stays refused). Probe: two terms with one text match
score 0.25 and are dropped; a repeated term counted twice (0.33), now once; a predicate name matches (carried,
optional). Order: score, then entity ID; the pin's `sort.Slice` is not stable (the probe kept input order on 40 ties;
not promised). A reply over `max_payload` (`natsclient.Client.MaxPayload`) leaves out the lowest-ranked matches until it
fits and sets `entities_truncated`; only a byte-full page that nearly all matches reaches it. The answer is a plain
function (owner, Q6), literally `Found {n} matching entities among {entities_scanned} read from {community_id}, which
has {member_count} members.`, then the record's summary if set, then `Key themes:` and the first five keywords. The
pin printed "(7 entities, 0% match)" (probe), a relevance local search never computes. *Proof:*
`TestLocalSearchReadsOnePage` (ruling 2's: one prefix request; a match past the page is absent, truncated; a group that
fits equals an independent filter), `TestLocalSearchScoresAsThePin` (literals from `graphrag_test`),
`TestLocalSearchTermsAreDistinctAndBounded`, `TestLocalSearchOrderIsDeterministic`, `TestLocalSearchAnswerTemplate`,
`TestLocalSearchEntitiesPassUnchanged` (a large integer), `TestLocalSearchFitsMaxPayload` (own server, small limit).

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

A present record costs no entity read; a level with no records falls in rows 3 and 4 (ruling 3). One input per
request. A truncated answer adds one to a `verb`-labelled counter on the injected registry; none given, none registered
(`metric-registry`). *Proof:* `TestLocalSearchRefusals`, `TestLocalSearchRefusesBadRequests`,
`TestTruncatedSearchIsCounted`.

## D7. What leaves step 3, and ADRs

Not carried: generation, lease, fence and the community cache (ruling 3; #406 retires); the `graph.clustering.query.entity`
fallback, which swallowed every failure (S15); the semantic fallback, whose trigger, an entity with no community, is
gone (ADR-099); the answer interface (Q6). Moved, each to its reader: the poison-counting `List` and labels and digests
(#31) to step 5. A keyed `Get` already reports poison (`pkg/graphview/view.go:403-412`). ADR-087 ported with a note:
Decision 2 (one writer) and 5 kept, the store's decisions left with #485. ADR-081 (first production view), ADR-085
(level in the key: a level-1 request reads the level-1 record, `TestLocalSearchReadsTheAskedLevel`), ADR-066, ADR-109
D4 and ADR-113 applied. ADR-099's "shape-for-shape" yields to the owner's 2026-11-02 ruling, now cited in its port note.

## Scope

Retires #406 and #522. Shape findings, issues, overlap, exports: `inventory-delta.md`.
