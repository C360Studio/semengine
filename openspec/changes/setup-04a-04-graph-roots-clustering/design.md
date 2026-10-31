# Design: setup-04a-04-graph-roots-clustering

Base: `f3b48f6`. Inventory: `inventory.md` (round 2; this design waits on its `INVENTORY PASS`). Change 4, PR 9 of 10
(#9 comment 6386856657, item 8). Tier 0 only (#9 comment 6386603659). Draft PR #483. Governing records: ADR-099 and
the owner rulings behind it (inventory 6b), ADR-102 d6 (ported; binds the levels), ADR-085 (ported, kept), ADR-087
(not ported; tier-2 lane). Probes: inventory, Pin probe.

Coined terms: a *group* is one community: every entity whose ID shares a prefix of a given length; a *level* is that
length (0 = four positions, org.platform.system.domain; 1 = three, the source; 2 = two, the deployment; ADR-102 d6);
the *base partition* is the set of group records in `COMMUNITY_INDEX` that graph-query serves; an *overlay* is any
other partition, written somewhere graph-query does not read.

## The facts the choice rests on

- Label propagation (LPA), the algorithm `graph/clustering` runs at the pin, was removed by your own SemStreams ruling
  of 2026-08-23 (ADR-099; gh606 Q1 "RULED: REMOVE", naming `lpa.go`). The reason, in that ruling: over these entity
  IDs it re-derives the IDs. SemStreams never carried the removal out, so the pin still has LPA (inventory 6b).
- SETUP 03B admitted "clustering on explicit edges" at tier 0 on 2026-10-01 without reading ADR-099.
- SemEngine has already ported ADR-102, which fixes what a community level is (level 0 = the four-position prefix)
  and retires the `graph/inference` containers together with the ADR-099 partition (`docs/adr/102-…:103`, `:129`).
  An LPA level 0 is a second, contradicting meaning of "level".
- ADR-099 allows detection back only as an overlay: explicit edges in, written where graph-query does not read,
  default off, never writing the base partition, never a readiness dependency of graph-query, and shipping only once
  a fixture that can fail measures it. The pin's quality check never scored better than 1 of 3.
- Who uses communities today: semboids (its flock view; LPA over explicit edges, `configs/flock.json`); semsource only
  behind `enable_clustering` (default off) and never asks for a level above 0 (inventory, Open evidence questions).

## Options

- **A. Port the pin's tier-0 half whole** (4,231 pin lines; waiver). Carries everything ADR-099 removed, plus a sibling
  cache that never refreshes (P1) and levels that are not a hierarchy (P2).
- **B. LPA over explicit edges writing `COMMUNITY_INDEX`** (2,503 lines). Keeps semboids' flock view working today.
  Contradicts ADR-099 twice (LPA stays; detection writes the base partition graph-query serves) and ADR-102 (level 0
  would not be the four-position prefix). semboids' two-clique unit test is not the quality fixture ADR-099 asks for.
- **B′. LPA as a default-off overlay** in its own bucket, which graph-query never reads, shipped only with a fixture
  that can fail (for example, flocks with known membership scored against the result). Conforms to ADR-099 and could
  serve semboids, but alone it gives graph-query no base partition, adds a new bucket (a new durable primitive, owner
  decision), and the fixture does not exist yet.
- **C. The ADR-099 base partition (recommended).** Groups by ID prefix at levels 0-2, bounded records, write on change,
  the statistical summary per group. Conforms to ADR-099 and ADR-102; deterministic with no algorithm to tune; about
  1,350 pin lines carried plus a small partition step the pin never wrote, from a design you already ruled (gh606
  §3.3, Q5, Q6, Q8). Cost: every boid lands in one community (all share the prefix `<org>.<platform>.sim.flock`,
  semboids `internal/boidgraph/payload.go:42`), and semboids' flock view shows no members until semboids reads members
  by prefix: it decodes `members` from each record (`internal/api/graphstream_decode.go:78`, `:86`), which C records
  do not carry. Flock clusters return only with B′, its own change with its fixture. graph-query (item 9) is ported
  to the bounded record.
- **D. Do nothing until change 7.** Item 9 cannot build its community path.

Trade-off in plain words: B keeps one demo working and ports code you already threw out; C follows your own ruling
and leaves semboids without flock clusters until an overlay is built and measured.

## D1. Files, edges, exclusions (option C)

Carried into `graph/clustering/`: `types.go` (the group record and storage interface), `storage.go`, `summarizer.go`
`:1-498`, a new `doc.go` and `README.md` (the product contract line), and a new `partition.go`. Edges: `graph`,
`natsclient`, `pkg/errs`, `pkg/types`. Ledger: one `adapt` row and `defer-exclude` rows:

- tier 2, naming #32: `summarizer.go:499-863`, `enhancement_worker.go`, `summary_store.go`, `metrics.go`;
- ADR-099, naming question 1: `lpa.go`, `entityid_provider.go`, `semantic_edge_provider.go`, `pagerank.go` (its only
  reader is representative selection, which Q5 replaces; it is quadratic in group size);
- no reader: `provider.go` (P4).

`MembershipHash` stays: the record carries it (gh606 §3.3). One line in `.agents/skills/entity-or-bucket/SKILL.md`
item 2 (the pin skill's "community detection" clause, `pin:.agents/skills/entity-or-bucket/SKILL.md:47`), reworded to
community grouping.

## D2. The partition step (new exported surface, owner approval needed)

`clustering.NewPartitioner(requester, storage, summarizer)` and `(*Partitioner).Refresh(ctx) (Partition, error)`.
`requester` is the one-method interface `graph.NewExactEntityReader` takes (`RequestClassified`, satisfied by
`*natsclient.Client`); the partitioner reads the graph through graph-ingest's `prefix` verb with an empty prefix, paged
by cursor, its subject taken from `graph.QueryVerbs()` (query-pattern skill, read 2). No entity list is passed in.

Who it is for: framework API. The contract line is the `community-grouping` spec and the package `README.md`; a
product-style test (`TestRefreshAsAProduct`, package `clustering_test`, on `natsfixture` with graph-ingest serving the
verb) calls it as a product would. `processor/graph-clustering` (change 7) is its first framework caller. Observable
behaviour:

- The constructor refuses a nil requester, storage or summarizer, and `Refresh` a nil context, each with an error.
- Each entity read belongs to exactly one group per level, its ID prefix at that level; singletons get a record (Q6).
- Each record holds level, prefix ID, member count, `MembershipHash` of the members, keywords, statistical summary
  and at most `clustering.MaxRepEntities` (a framework constant, 5 as at the pin; gh606 `:220`) representative IDs; no
  member list. Summaries for groups of two or more members.
- A record equal to the stored one is not written. Every changed record is written before any removal; a read or
  write failure returns `(nil, err)` and removes nothing.
- Incomplete runs are errors the caller cannot miss: when an ID the canonical parser refuses was left out, or removing
  stale records failed, `Refresh` returns the partition **and** a non-nil `*IncompleteError` (fields `Skipped
  []string`, `Prune error`; `Unwrap` returns the prune error). `err == nil` means complete.
- Each entity state is read once per refresh. A page boundary is keyset: an entity created mid-refresh with an ID
  before the cursor waits for the next refresh (`graph/query_prefix_types.go:24-28`).

## D3. Storage

`NATSCommunityStorage` over `*natsclient.KVStore` keeps `SaveCommunity`, `Prune`, `GetAllCommunities`. Required: S1
the nil-KV in-memory store goes (#186); S2 triples go; S3 key listing uses `KVStore.Keys`, which already maps nats.go's
empty-bucket sentinel by identity (`natsclient/kv.go:582`), so no error text is searched (AGENTS § 2 uses `errors.Is`
for library sentinels); S4 an unreadable entry fails `GetAllCommunities`, naming the key; S5 a cancelled context
returns its error. Key `{level}.{prefix}`; no `entity.{level}.{id}` keys. Unread methods dropped (inventory 4).

## D4. Summary

`CommunitySummarizer` and `StatisticalSummarizer` port. Required: S9, ties break by term or ID; representative
selection is deterministic, prefers members with text, and needs no graph index (Q5), replacing PageRank. A summary
depends on the members and on the corpus statistics of that refresh (`BuildCorpusDF`, `summarizer.go:74`).
`Community`'s pin fields `Members`, `ParentID`, `SummaryStatus` (stored `"statistical"`, `summarizer.go:126`, read only
by the e2e harness), `LLMSummary`, `SummaryTruncated`, `Metadata` give way to the bounded record.

## D5. ADRs

ADR-099 ported with a port note: its decision is applied (base partition derived; LPA, edge synthesis and semantic
edges not carried); the overlay contract stands for any later detection change. ADR-085's note gains the package.
ADR-087 stays out with the tier-2 lane (#32); ADR-086 and ADR-061 are not ported.

## D5a. Spec delta

`specs/community-grouping/spec.md` is written against option C. If the owner rules otherwise on question 1, or
answers question 3 by asking for the overlay in this change, it is rewritten before review continues.

## D6. Proving tests

Unit, on a fake requester serving prefix pages and a fake storage: `TestRefreshRefusesNilContext`,
`TestGroupsArePrefixes` (each level), `TestSingletonRecorded`, `TestUnparseableIDIsAnError` (`errors.As` finds
`*IncompleteError` naming the ID), `TestRecordIsBounded`, `TestUnchangedRecordNotRewritten`,
`TestWriteFailureRemovesNothing`, `TestPruneFailureIsAnError`, `TestEachStateReadOnce` (pages across several
cursors), `TestSummaryDeterministic`. Generated: `TestPartitionMatchesModel`, seeded random ID sets; the model splits
each ID string on `.` and joins the first 4, 3 and 2 parts itself, never calling the `types.EntityID` prefix methods
the code uses; seed and run count recorded. Product-style: `TestRefreshAsAProduct` (D2). Integration on `natsfixture`:
`TestUnchangedRefreshAddsNoRevision`, `TestGetAllCommunitiesFailsOnCorruptEntry`,
`TestPruneReportsCancel`, `TestGetAllReportsCancel`. Mutation checks: tie-break removed; equality skip removed;
removal before writes; the `*IncompleteError` replaced by `nil`. Coverage floor 80 %.

## D7. Issues and merge order

Retires none. Stay open: #32, #68, #141, #477 (sites leave with the tier-2 lane; a comment on each). No open PR
overlaps; if #464 merges first, the integration run repeats at nats.go 1.55.0. Item 9 ports after this, onto the
bounded record (gh606 §3.3: membership by prefix query, summary join by stored hash). Change 7's
`processor/graph-clustering` calls `Refresh` and drops its LPA config keys as refused keys (gh606 Q4). A semboids
overlay (B′) is its own later change, with its fixture.
