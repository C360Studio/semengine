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
delete) and a tick path. Change 7's `processor/graph-clustering` wires the `ENTITY_STATES` watch; bootstrap is the
watch's initial replay fed through the apply path. `CommunitySummarizer` and `StatisticalSummarizer` leave tier 0
("Drop it; tier 2 (#485) brings back what it needs."). Rejected in round 3: a full-scan `Refresh` over the `prefix`
verb.

What change 7 must supply, and what it costs (inventory § 13): for each delivery, the exact value, its revision, and
whether it is a delete or purge; and a signal when the initial replay has ended. A raw `WatchAll` gives all four (the
nil entry marks the end of the replay). `graph/follower` does not today: its `Entry` carries no value
(`graph/follower/follower.go:46-51`), it keeps the end-of-replay marker to itself (`:227-235`), and graph-index
re-reads current state per key (`processor/graph-index/index.go:155-163`), which here would label a newer value with
an older revision. So change 7 either runs its own watch or extends the follower to pass values and announce the end
of replay. Either way one watch serves the maintainer.

## D1. Files, edges, exclusions

New or carried in `graph/clustering/`: `types.go` (the bounded `Community` record), `storage.go`, `maintainer.go`
(new), `terms.go` (the pin's word extraction and summary template, `pin:graph/clustering/summarizer.go:388-490`),
`doc.go`, `README.md`. Edges: `graph`, `natsclient`, `pkg/errs`, `pkg/types`. Ledger: one `adapt` row
and `defer-exclude` rows:

- tier 2, naming #485 (tier-2 epic #484): `summarizer.go:499-863`, `enhancement_worker.go`, `summary_store.go`,
  `metrics.go`, and `MembershipHash` (`storage.go:42-48`; its only remaining reader is the deferred summary store, and
  re-hashing a large group on every insert is linear work);
- ADR-099, ruled 2026-10-31: `lpa.go`, `entityid_provider.go`, `semantic_edge_provider.go`, `pagerank.go`;
- owner ruling 2026-10-31 ("Drop it; tier 2 (#485) brings back what it needs."): `CommunitySummarizer` and
  `StatisticalSummarizer` (`summarizer.go:27-129`, `:165-387`), with corpus document frequency (`BuildCorpusDF`);
- no reader: `provider.go` (P4).

Size (#159): about 950 pin lines considered (`types.go` 126, `storage.go` 526, `summarizer.go` `:388-490`
about 100, `doc.go` 198) plus the new maintainer; under the ~3,000 bound without a waiver.

One line in `.agents/skills/entity-or-bucket/SKILL.md` item 2 (the pin skill's community clause,
`pin:.agents/skills/entity-or-bucket/SKILL.md:47`), reworded to community grouping.

## D2. The maintainer (new exported surface, owner approval needed)

`clustering.NewMaintainer(storage *NATSCommunityStorage) (*Maintainer, error)`; `(*Maintainer).Upsert(entityID string,
revision uint64, state *graph.EntityState) error`; `Delete(entityID string, revision uint64) error`;
`BootstrapDone()`; `Tick(ctx) error`. Framework API: the contract line is the `community-grouping` spec and the package
`README.md`; `TestMaintainerAsAProduct` (package `clustering_test`, `natsfixture`) drives it as change 7 will.
`Upsert`'s revision SHALL be the revision of the exact value `state` was decoded from. Precondition, owned by change
7 and not a library promise checked here: each key's deliveries reach the maintainer in increasing revision order, as
from one ordered watch. Every method is safe to call from the watch goroutine and a ticker goroutine at once, and
`Upsert` and `Delete` never wait on a `Tick`'s storage write (`-race` test, a blocked write). Observable behaviour:

- **Apply.** Each present entity keeps its last-applied revision and contribution. An `Upsert` or `Delete` at or below
  the kept revision changes nothing. A newer `Upsert` subtracts the old contribution and adds the new one; `Delete`
  subtracts it and forgets the entity. The records after any history equal those computed from the final values.
- **Refusals.** `Upsert` refuses, with a typed invalid error naming the entity, a nil state, an ID the canonical
  parser refuses, and a state whose ID differs from `entityID`. A refused entity contributes nothing and stays in a
  refused set until a valid `Upsert` or a `Delete` for it.
- **Memory.** Per entity: its ID, revision, type, and at most `MaxEntityTerms` terms (the highest counts, lexical
  ties). Per group: member count, type counts, term counts, and the ordered set of its text-bearing members. Nothing
  persists across a restart: a new maintainer is rebuilt from the watch's replay.
- **Tick.** `Tick` refuses a nil context, and returns `ErrBootstrapIncomplete` and writes nothing before
  `BootstrapDone`. The first tick after bootstrap lists `COMMUNITY_INDEX` keys once and removes keys of groups that do
  not exist. Every tick then recomputes only the groups touched since their last successful publication: member count,
  type counts, the top `MaxKeywords` terms by the group's own counts (lexical ties), the summary text, and the
  `MaxRepEntities` lexically smallest text-bearing members, read from the ordered set without scanning membership. It
  writes a record only when it differs from the last one published, removes the records of groups that became empty,
  and reads nothing from `COMMUNITY_INDEX` (and holds no `ENTITY_STATES` handle).
- **Terms.** Keywords come only from words in an entity's string values (the pin's `extractTerms`,
  `pin:graph/clustering/summarizer.go:464-480`, with its stop words); terms from ID positions and predicate names are
  not counted, so the shared vocabulary of a source does not fill every group's keywords. Types are counted
  separately, from the ID's type position.
- **Failure signals.** A write or removal failure returns an error; the affected groups stay pending and are retried
  on the next tick. When the refused set is not empty, a publication that otherwise succeeds returns
  `*IncompleteError` naming the refused IDs. `Tick` returns nil only when every group is published and nothing is
  refused.

## D3. Storage

`NATSCommunityStorage` over `*natsclient.KVStore`. Exported, each with its need: `NewNATSCommunityStorage` (change 7
and products build the maintainer's store) and `GetAllCommunities` (the anomaly half of change 7 reads every record,
`pin:processor/graph-clustering/anomaly.go:342`). Unexported, called only by the maintainer: save, prune by group key,
and key listing for the first tick. Required from the inventory: S1 the nil-KV store goes (#186); S2 triples go; S3
key listing uses `KVStore.Keys`, which maps nats.go's empty-bucket sentinel by identity (`natsclient/kv.go:582`); S4 an
unreadable entry fails `GetAllCommunities`, naming the key; S5 a cancelled context returns its error. Key
`{level}.{prefix}`; no `entity.{level}.{id}` keys.

## D4. The record

`Community{ID, Level, MemberCount, TypeCounts (top MaxKeywords types), Keywords, StatisticalSummary, RepEntities}`,
every field bounded by the constants; no member list, no membership hash. The other exported names and their need: `Maintainer`
and its methods (change 7's watch loop); `IncompleteError`, `ErrBootstrapIncomplete` (callers branch with `errors.As`
and `errors.Is`); `Community` (item 9 and products decode records); the three constants (the record's size bound a
reader can rely on). `MaxRepEntities` = 5 and `MaxKeywords` = 10
as at the pin (`summarizer.go:53-57`); `MaxEntityTerms` = 32 (new; the cap that bounds memory per entity).

## D5. ADRs

ADR-099 ported with a port note: its base partition is applied, kept from changes; its levels as amended by ADR-102;
LPA and edge synthesis not carried. ADR-085's note gains the package. ADR-087 stays out with the tier-2 lane (#485).

## D6. Proving tests

Generated, against an independent model: `TestMaintainerMatchesModel` applies seeded random histories of upserts,
deletes, obsolete revisions and refusals, then compares every published record with a from-scratch computation over
the final entity set. The generator builds each entity's string values from words it chose, so the model takes each
entity's terms from the generator by construction and never runs the extraction in `terms.go`; it splits each ID on
`.` itself. `TestHistoryOrderIrrelevant`: two histories that interleave entities differently, keep each entity's revision
order (change 7's precondition), and end with the same final values give byte-identical records; obsolete
deliveries are generated only for entities still present, since with no tombstones an older upsert after a delete
would bring the entity back. Seed, run count and completed checks in notes.

`TestTickCost` runs on a counting fake `jetstream.KeyValue` behind a real `KVStore` (`natsclient.NewClient` does not
dial, `natsclient/client.go:312-341`; `(*Client).NewKVStore` takes any `jetstream.KeyValue`, `natsclient/kv.go:55`):
after bootstrap, a tick after k changed entities makes no `COMMUNITY_INDEX` read and writes only the groups those k
touched; an unchanged graph writes nothing. Its mutants: list keys on every tick; read before each write; drop the
equality skip. The `ENTITY_STATES` half cannot fail a test: the maintainer holds no handle to that bucket.

Unit: `TestObsoleteDeliveryIgnored`, `TestRefusedEntityMakesTickIncomplete`, `TestTickBeforeBootstrap`,
`TestFailedWriteRetried`, `TestRecordIsBounded`, `TestUpsertDuringBlockedWrite` (a write blocked in the fake; `Upsert`
returns), `TestKeywordsIgnoreIDAndPredicates`, `TestRepresentativesWithoutScan` (the smallest text-bearing member
leaves; the next one is chosen). Integration on `natsfixture`: `TestUnchangedRecordAddsNoRevision`,
`TestGetAllCommunitiesFailsOnCorruptEntry`, `TestPruneReportsCancel`, `TestGetAllReportsCancel`,
`TestMaintainerAsAProduct`. Mutation checks: subtraction removed on replace; obsolete-revision check removed; equality
skip removed; `*IncompleteError` replaced by nil. Coverage floor 80 %.

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
