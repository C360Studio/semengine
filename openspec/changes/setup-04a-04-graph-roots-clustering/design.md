# Design: setup-04a-04-graph-roots-clustering

Base: `f3b48f6`. Inventory: `inventory.md` (`INVENTORY PASS` at `5c2f610`; round-4 premises in its § 13). Change 4,
PR 9 of 10 (#9 comment 6386856657, item 8). Tier 0 only (#9 comment 6386603659). Draft PR #483. Governing records:
ADR-099 and the owner rulings behind it (inventory 6b), ADR-102 d6 (ported; binds the levels), ADR-085 (ported,
kept), ADR-087 (not ported; tier-2 lane), and the owner's rulings of 2026-10-31 (proposal, Rulings).

Coined terms. Entity IDs are `org.platform.system.domain.type.instance`. A *group* is every entity sharing an ID
prefix of one length; its *level* is that length: 0 = four positions (`org.platform.system.domain`), 1 = three (the
source, `org.platform.system`), 2 = two (the deployment, `org.platform`) (ADR-102 d6). The *base partition* is the
set of group records in `COMMUNITY_INDEX`. An entity's *contribution* is what it adds to each of its three groups:
one member, one count for its type, and its capped term set. A *tick* publishes the groups changed since the last.

## What was ruled (2026-10-31)

The base partition is the entity-ID prefix, kept current from changes, never by rescanning; no label propagation.
This pull request is the library: a maintainer in `graph/clustering` with an apply path (one entity's upsert or
delete) and a tick path. Change 7's `processor/graph-clustering` wires the `ENTITY_STATES` watch, as graph-index does
(`processor/graph-index/config.go:136`, `graph/follower`); bootstrap is the watch's initial replay fed through the
apply path. Rejected in round 3: a full-scan `Refresh` over the `prefix` verb (cost grows with the graph on every run).

## D1. Files, edges, exclusions

New or carried in `graph/clustering/`: `types.go` (the bounded `Community` record), `storage.go`, `maintainer.go`
(new), `terms.go` (the pin's term extraction and summary template, `pin:graph/clustering/summarizer.go:130-163`,
`:388-480`), `doc.go`, `README.md`. Edges: `graph`, `natsclient`, `pkg/errs`, `pkg/types`. Ledger: one `adapt` row
and `defer-exclude` rows:

- tier 2, naming #485 (tier-2 epic #484): `summarizer.go:499-863`, `enhancement_worker.go`, `summary_store.go`,
  `metrics.go`, and `MembershipHash` (`storage.go:42-48`; its only remaining reader is the deferred summary store, and
  re-hashing a large group on every insert is linear work);
- ADR-099, ruled 2026-10-31: `lpa.go`, `entityid_provider.go`, `semantic_edge_provider.go`, `pagerank.go`;
- replaced by the maintainer: `StatisticalSummarizer` and `CommunitySummarizer` (both summarize a list of entity
  states, which the incremental path never holds; the tier-2 seam returns with #485), corpus document
  frequency (`BuildCorpusDF`);
- no reader: `provider.go` (P4).

Size (#159): about 990 pin lines considered (`types.go` 126, `storage.go` 526, `summarizer.go` `:130-163` and
`:388-480` about 140, `doc.go` 198) plus the new maintainer; under the ~3,000 bound without a waiver.

One line in `.agents/skills/entity-or-bucket/SKILL.md` item 2 (the pin skill's community clause,
`pin:.agents/skills/entity-or-bucket/SKILL.md:47`), reworded to community grouping.

## D2. The maintainer (new exported surface, owner approval needed)

`clustering.NewMaintainer(storage *NATSCommunityStorage) (*Maintainer, error)`; `(*Maintainer).Upsert(entityID string,
revision uint64, state *graph.EntityState) error`; `Delete(entityID string, revision uint64) error`;
`BootstrapDone()`; `Tick(ctx) error`. Framework API: the contract line is the `community-grouping` spec and the package
`README.md`; `TestMaintainerAsAProduct` (package `clustering_test`, `natsfixture`) drives it as change 7 will. Every
method is safe to call from the watch goroutine and a ticker goroutine at once (`-race` test). Observable behaviour:

- **Apply.** Each entity keeps its last-applied revision and its contribution. `Upsert` with a revision at or below
  the kept one, and `Delete` likewise, change nothing (an obsolete delivery). A newer `Upsert` subtracts the old
  contribution and adds the new one; `Delete` subtracts it and keeps the revision as a tombstone, so a late older
  `Upsert` cannot resurrect the entity. Messages are never counted: the state after any history equals the state
  built from the final values alone (watch deliveries coalesce).
- **Refusals.** `Upsert` refuses, with a typed invalid error naming the entity, a nil state, an ID the canonical
  parser refuses (which includes the 256-byte bound, `pkg/types/entity_id.go:13`), and a state whose ID differs from
  `entityID`. A refused entity contributes nothing and is held in a refused set until a valid `Upsert` or a `Delete`
  for it is applied. An entity with more than `MaxEntityTerms` distinct terms is not refused: its term set is capped
  to the `MaxEntityTerms` highest counts, ties broken lexically.
- **Tick.** `Tick` refuses a nil context, and returns `ErrBootstrapIncomplete` and writes nothing before
  `BootstrapDone`. The first tick after bootstrap lists `COMMUNITY_INDEX` keys once and removes keys of groups that do
  not exist. Every tick then recomputes only the groups touched since the last successful publication of each: per
  group, member count, type counts, the top `MaxKeywords` terms by the group's own counts (lexical ties), the summary
  text, and `MaxRepEntities` representative IDs (the lexically smallest members that carry text). It writes a record
  only when it differs from the last one published, removes the records of groups that became empty, and reads no
  value from `COMMUNITY_INDEX` or `ENTITY_STATES`.
- **Failure signals.** A write or removal failure returns an error; the affected groups stay pending and are retried
  on the next tick. When the refused set is not empty, a publication that otherwise succeeds returns
  `*IncompleteError` naming the refused IDs. `Tick` returns nil only when every group is published and nothing is
  refused. A partial update is never reported as complete.

## D3. Storage

`NATSCommunityStorage` over `*natsclient.KVStore` keeps `SaveCommunity`, `Prune` (by group key), `GetAllCommunities`
(read by the anomaly half, change 7) and gains `Keys` for the first tick. Required from the inventory: S1 the nil-KV
store goes (#186); S2 triples go; S3 key listing uses `KVStore.Keys`, which maps nats.go's empty-bucket sentinel by
identity (`natsclient/kv.go:582`); S4 an unreadable entry fails `GetAllCommunities`, naming the key; S5 a cancelled
context returns its error. Key `{level}.{prefix}`; no `entity.{level}.{id}` keys.

## D4. The record

`Community{ID, Level, MemberCount, TypeCounts (top MaxKeywords types), Keywords, StatisticalSummary, RepEntities}`,
every field bounded by the constants; no member list, no membership hash. `MaxRepEntities` = 5 and `MaxKeywords` = 10
as at the pin (`summarizer.go:53-57`); `MaxEntityTerms` = 32 (new; the cap that bounds memory per entity).

## D5. ADRs

ADR-099 ported with a port note: its base partition is applied, kept from changes; its levels as amended by ADR-102;
LPA and edge synthesis not carried. ADR-085's note gains the package. ADR-087 stays out with the tier-2 lane (#485).

## D6. Proving tests

Generated, against an independent model: `TestMaintainerMatchesModel` applies seeded random histories of upserts,
deletes, obsolete revisions and refusals, then compares every published record with a from-scratch computation over
the final entity set; the model splits each ID on `.` itself and counts terms itself. `TestHistoryOrderIrrelevant`:
two shuffled histories with the same final values give byte-identical records. Seed, run count and completed checks
in notes. `TestTickCost`, on a fake KV that counts calls: after bootstrap, a tick after k changed entities makes zero reads
and writes only the groups those k touched; an unchanged graph writes nothing. Unit: `TestTombstoneBlocksOlderUpsert`,
`TestRefusedEntityMakesTickIncomplete`, `TestTickBeforeBootstrap`, `TestFailedWriteRetried`, `TestRecordIsBounded`,
each refusal and obsolete-revision case. Integration on
`natsfixture`: `TestUnchangedRecordAddsNoRevision`, `TestGetAllCommunitiesFailsOnCorruptEntry`,
`TestPruneReportsCancel`, `TestGetAllReportsCancel`, `TestMaintainerAsAProduct`. Mutation checks: subtraction removed
on replace; obsolete-revision check removed; tombstone dropped; equality skip removed; `*IncompleteError` replaced by
nil. Coverage floor 80 %.

## D7. Issues, merge order, held items

Retires none. Stay open: #485 (home of the clustering tier-2 lane), #32 (now only the provider plumbing: `graph/llm`,
`model/wire`, `go-openai`), #68, #141, #477 (their sites leave with the tier-2 lane to #485; a comment on each). The
tracker rows already in the ledger move to their new homes in this PR: `graph/embedding/http_embedder.go` → #489,
`graph/query/classifier_llm_adapter.go` → #486 (their home text in `known_risks`). No open PR
overlaps. Held for item 9 (`processor/graph-query`), not designed here: the reader's consistency contract (which
committed generation of `COMMUNITY_INDEX` a reader sees during a tick), and graph-query's own prefix-page reads,
which rescan keys per page (`graph/query_prefix_types.go:30-33`). Held for change 7: the watch wiring, and its
refused-config keys. Out of scope, a later change: a detection overlay (connected components over explicitly chosen
relationship predicates, default off, its own namespace graph-query never reads, never a readiness input), gated by an
exact-membership fixture that includes a one-edge bridge, and by a semboids-style precision/recall and resource gate.
semboids is a load generator; its member decoder breaks under bounded records.
