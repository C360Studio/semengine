# Inventory delta: setup-04a-04-graph-roots-graph-query-communities

base: `b940e29`. Adds to the archived `inventory.md` (reviewed on PR #504) and step 2's `inventory-delta.md`; repeats
nothing they settle. Pins are at base unless marked `pin:` (`8b99efe9`).

## Surface: what step 3 meets

- `graph/clustering/types.go:26` — `type Community struct {`: level, ID, counts, keywords, summary, representatives;
  no revision (`git grep -n -i revision graph/clustering/types.go`: no line).
- `graph/clustering/maintainer.go:71` — `revision     uint64`: the maintainer holds each member's last revision.
- `graph/clustering/maintainer.go:255` — `if reflect.DeepEqual(record, m.published[key]) {`: an unchanged record is not
  written; a revision field makes every touched group's record differ.
- `graph/clustering/maintainer.go:390` — `groups: [levels]string{eid.TaxonomyPrefix(), eid.SourcePrefix(),
  eid.DeploymentPrefix()},`: the one home of level-to-prefix, unexported.
- `graph/clustering/storage.go:64` — `if own := communityKey(community.Level, community.ID); own != key {`: the record
  check graph-query must share; `:108` — `func communityKey(level int, id string) string {`, unexported.
- `graph/clustering/maintainer_integration_test.go:41` — `mustUpsert(t, m, id, 2, "beta alpha")`: the same-content
  upsert asserted to write nothing; D1 changes it.
- `pkg/graphview/view.go:403` — `func (v *View[T]) Get(key string) (T, uint64, error) {`: readiness, poison and absence
  each a distinct error; `:425` `List` drops poisoned keys (ruling 3's reason for a counting list, step 5).
- `graph/kvcatalog/kvcatalog.go:188` — `derived(graph.BucketCommunityIndex, "graph-clustering", "Community detection
  index"),`: owner-creates; `:296` `OpenCatalogReader` never creates and refuses an absent bucket as not ready.
- `processor/graph-query/component.go:143` — `// ctx bounds the subscribing only and is not kept. Nothing outlives a
  handler, so graph-query`: no run context at base.
- `processor/graph-query/counts.go:30` — `func (c *Component) readCountPage(`: the page checks; IDs only today.
- `processor/graph-query/pathsearch.go:78` — `func (c *Component) readPathStart(`: the entity read whose refusal passes
  through, reused for the missing-record case.
- `natsclient/client.go:365` — `func (m *Client) MaxPayload() (int64, error) {`.
- Searches closed empty at base: `git grep -n -i localSearch -- '*.go'`; `git grep -n 'entity_revision\|EntityRevision'`;
  `git grep -n graphview -- '*.go' ':!pkg/graphview' ':!*_test.go'` (no production reader of the view yet); production
  callers of `NewMaintainer`, `NewNATSCommunityStorage`, `GetAllCommunities`: none (change 7 wires them).

## Pin facts added

Probe, one run on the pin copy, Go 1.26.4, source and output in `notes/probes/step3-pin-probe.*`: the local answer
reads `(7 entities, 0% match)`; two terms with one text match score 0.25 (dropped at 0.3); `alpha alpha zzz` scores
0.33 (kept); a type term 1.0; `title` matches a predicate name, 0.5; 40 tied scores kept input order. Reads, not runs:
`pin:graphrag.go:308-337` semantic fallback when no community; `:2264-2316` the clustering query that logs and swallows
every failure (S15); `:2341` `sort.Slice` (not stable); `component.go:56` `min_text_relevance`, set by no consumer
(SemConnect `deploy/semstreams.json` at `d0d06e00` sets only `max_traversal_*`). No consumer calls `localSearch`
(archived `inventory.md` § 3; SemSource `query_tools.go` at `e4febc0d` has no `localSearch`).

Pin lines carried or replaced: production `graphrag.go:66-124` (lease, 59), `:126-158` (types, 33), `:279-400`
(handler, 122), `:2218-2397` (lookup and scoring, 180), `community_cache.go` (431): about 825. Tests
`community_cache_test` 321, `community_generation_test` 217, `community_summary_join_test` 51, `graphrag_test.go:347-481`
135, `community_query_behavior_test.go:19-50`, `:400-464` 97: about 820.

## Same-class collision: how current is a community record

| Dimension | Evidence |
| --- | --- |
| Semantic class | The `ENTITY_STATES` revision a group record reflects |
| Owners | Maintainer (writer, `maintainer.go:50`); graph-query (reader, new) |
| Spellings today | `graphview.View.Applied` (`view.go:392`, `COMMUNITY_INDEX`'s revision, wrong bucket); a view entry's KV revision (the record's own); graph-index's `IndexedRevision` (`graph/index_status.go:65`, another index); the pin's generation (`community_cache.go`, not carried). None is this fact |
| Status | `indexed_revision` on the reply; no `GRAPH_STATUS` key (ADR-087 Decision 5) |
| Lifecycle, recovery | Rebuilt from replay on a maintainer restart; can come back lower, still true |
| Writers, readers | Maintainer only (ADR-087 Decision 2); graph-query's `localSearch`; step 5's text tier |

Problem shape: a revision watermark of a derived index, refused under `min_revision`. Closest instance:
`processor/graph-index/query.go:79-95` (`queryRevision`), adopted; its watermark is the bucket's, so it cannot stick.
The view's supervision: closest `processor/graph-index/component.go:258-283` (run context, join) and
`graph/follower/follower.go:135` (`resource.NewWatcher`), which fails `Open` on an absent bucket; not adopted, since
`COMMUNITY_INDEX` has no writer until change 7. No new primitive for reuse, so no adoption list.

## Open pull requests

`gh pr list --state open`, 2026-11-05: #524 (this claim, no files); #523 draft, ateam v0.3.7, `openspec/changes/
adopt-ateam-v037-probe/` (specs `harness-boundaries`, `merge-gate`); #517 `golang.org/x/net`, #516 `nats-server`
2.15.2, each `go.mod` and `go.sum`. No overlap in files or capabilities; any merges first, and this branch then merges
`origin/main`. No PR has over 100 files.

## Open issues

Searches, each `gh issue list --state open --search <term>`: `community`, `localSearch`, `graphview`, `clustering`,
`COMMUNITY_INDEX`, `graph-query`, `graphquery`, `truncated`, `ADR-087`, `label`.

| Issue | Disposition, as Bounded port scope classes it |
| --- | --- |
| #406 | **Retired**: the lease is not carried (ruling 3), so no state rides a context value; task 3.2 |
| #522 | **Retired**: three docs corrections scheduled for "a later graph-query step"; task 4.2 |
| #31 | Stays; labels come with step 5's digests, which then drop the agentic predicates |
| #477 | Stays; graph-query's site adopts the view here (task 3.2, comment on the issue), other sites remain |
| #506 | Stays; the counter of task 3.3 is its measurement |
| #485 | Stays; the summary store and ADR-087's store decisions |
| #32 | Stays; the owner's Q6 removes the answer interface its 2026-10-24 line kept; comment on the issue |
| #15 | Stays; a recreated `ENTITY_STATES` restarts revisions, for every `indexed_revision` |
| #68 | Stays; the new counter uses the injected registry, so no site here |
| #111, #106, #103, #479, #405 | Stay; steps 4 and 5 (`split.md`) |
| #195, #196 | Stay; no site in this step's code |
| #484, #486, #487, #9 | Tier 2 and the epic; stay |

Names the word, not the package: #11, #141, #198, #482, #105, #16, #300, #350, #24, #7, #108, #109, #155, #512 (read:
the page dedup of D5 covers its effect here), #521.

## Shape findings for what step 3 carries

| # | Disposition |
| --- | --- |
| S1 | Required (`AGENTS.md` § 1, #406): the lease is not carried |
| S5 | Required (§ 4 back doors): `localSearchBeforeFinalize`, `open*Reader` not carried |
| S9 | Required (`graph-transport-boundary`): `graph.clustering.query.entity` not spelled; `localSearch` enters the table |
| S15 | Required (§ 1 fail closed): the swallowing lookup goes; D6 |
| S16 | Moves with labels to step 5 (#31) |
| S20 | Optional; the handler builds typed data and marshals once (Q5(b)) |
| S22 | Required by owner ruling (Q6): plain function |
| New L1 | `pin:graphrag.go:2341` unstable sort before a cut: required (`AGENTS.md` § 2, sort before limit); D5 |
| New L2 | `pin:graphrag.go:2327` a repeated term changes who matches: required (§ 1, an answer must not depend on typing); D4 |
| New L3 | Predicate names score: optional, carried; recorded |

## Who each new export is for

| Export | Reader |
| --- | --- |
| `LocalSearchRequest`, `LocalSearchData`, `localSearch` row | graph-query's handler; products, contract line `graph-query` "Local search answers from one group's record and one page", with a test calling it as a product |
| `MaxSearchTerms` | the handler; step 5's text query; products, same contract line |
| `Community.EntityRevision` | graph-query's `indexed_revision`; contract line `community-grouping` "A record names the latest entity write it reflects" |
| `GroupKey`, `DecodeCommunity` | graph-query's view (production); `NATSCommunityStorage` and the maintainer; contract line `community-grouping` "A reader finds and checks a record as storage does" |
| Truncation counter | the owner's measurement for #506 (ruling 2); contract line `graph-query` "Truncated searches are counted", README |

## Adopter seam

**1. What must they know?** Decode by the table; read `entities_truncated`, since the page is the lexically smallest
IDs; send a `min_revision` from a write to the group searched. Three facts, a finding: the third is owner question 1.
**2. If they do nothing?** Before change 7: transient `index_not_ready` on every `localSearch` for an existing entity, so
a retry loop waits for change 7 (the split's stepping stone). Ignoring `entities_truncated`: a sample read as the group,
though `entities_scanned` and `member_count` sit beside it. A foreign `min_revision`: refused until the group changes.
A pin caller sending `communityId` or reading `count`: refused by name, or reads 0; `MIGRATION.md`. **3. Where do they
find out?** Refusals: typed runtime error. Sample: a field. Stuck revision: the doc only, a finding. **4. What should
they have to know?** Which verb to call. **Gaps:** the sample, which #506 removes; the stuck revision, which a tick
watermark removes. **Prefer observation:** the caller predicts no readiness, level population or index position.

## Intent check

No boundary moves: community-aware search at tier 0 is admitted (#9; ADR-099; PR #483 rulings); archived `inventory.md`
§ 11's table stands.
