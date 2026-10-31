# Design: setup-04a-04-graph-roots-clustering

Base: `f3b48f6`. Inventory: `inventory.md` (§ 13 holds the round-4 premises). Change 4, PR 9 of 10 (#9 comment
6386856657, item 8); tier 0 only (#9 comment 6386603659); draft PR #483. Governing records: ADR-099 and its rulings
(inventory 6b), ADR-102 d6 (ported; binds the levels), ADR-085 (ported, kept; decision 7 orders the rebuild), ADR-087
(not ported; tier 2), and the owner's rulings of 2026-10-31 (proposal, Rulings).

Coined terms. Entity IDs are `org.platform.system.domain.type.instance`. A *group* is every entity sharing an ID
prefix of one length; its *level* is that length: 0 = `org.platform.system.domain`, 1 = the source
`org.platform.system`, 2 = the deployment `org.platform` (ADR-102 d6). An entity's *contribution* is what it adds to
each of its three groups: one member, one count for its type, and its capped word set. A *word* is a run of 3 to
`MaxWordBytes` ASCII `a-z` and `0-9` characters in a string value, not a stop word. A *tick* publishes the groups changed
since the last.

## What was ruled (2026-10-31), and what change 7 must supply

The base partition is the entity-ID prefix, kept current from changes, never by rescanning; no label propagation. This
pull request is the library: a maintainer with an apply path (one entity's upsert or delete) and a tick path;
bootstrap is the watch's initial replay fed through the apply path. `CommunitySummarizer` and `StatisticalSummarizer`
leave tier 0 ("Drop it; tier 2 (#485) brings back what it needs.").

Change 7's `processor/graph-clustering` supplies, per delivery, the exact value, its revision and whether it is a
delete or purge, and a signal when the replay ends. A raw `WatchAll` gives all four (the nil entry ends the replay).
`graph/follower` does not today: its `Entry` has no value (`graph/follower/follower.go:46-51`), it keeps the replay
end to itself (`:227-235`), and graph-index re-reads current state per key (`processor/graph-index/index.go:155-163`),
which would pair a newer value with an older revision. Change 7 runs its own watch or extends the follower.

## D1. Files, edges, exclusions, size

New or carried in `graph/clustering/`: `types.go` (the bounded `Community` record), `storage.go`, `maintainer.go`
(new), `terms.go` (word extraction and summary template from `pin:graph/clustering/summarizer.go:388-490`), `doc.go`,
`README.md`. Edges: `graph`, `natsclient`, `pkg/errs`, `pkg/types`. Ledger: one `adapt` row and `defer-exclude` rows:
tier 2, naming #485 (epic #484): `summarizer.go:499-863`, `enhancement_worker.go`, `summary_store.go`, `metrics.go`,
`MembershipHash` (`storage.go:42-48`); ADR-099: `lpa.go`, `entityid_provider.go`, `semantic_edge_provider.go`,
`pagerank.go`; the 2026-10-31 ruling: `CommunitySummarizer`, `StatisticalSummarizer` (`summarizer.go:27-129`,
`:165-387`) with `BuildCorpusDF`; no reader: `provider.go` (P4). Size (#159): about 950 pin lines considered
(`types.go` 126, `storage.go` 526, `summarizer.go:388-490` about 100, `doc.go` 198) plus the new maintainer; no
waiver. One line in `.agents/skills/entity-or-bucket/SKILL.md` item 2
(`pin:.agents/skills/entity-or-bucket/SKILL.md:47`).

## D2. The maintainer (new exported surface)

`NewMaintainer(storage *NATSCommunityStorage) (*Maintainer, error)`; `Upsert(entityID string, revision uint64, state
*graph.EntityState) error`; `Delete(entityID string, revision uint64) error`; `BootstrapDone()`; `Tick(ctx) error`.
Framework API: contract line the `community-grouping` spec and the package `README.md`; `TestMaintainerAsAProduct`
drives it as change 7 will. `Upsert`'s revision is that of the exact value decoded. Precondition, owned by change 7
and not a library promise: each key's deliveries arrive in increasing revision order, as from one ordered watch.
Methods are safe from the watch goroutine and a ticker goroutine at once; `Upsert` and `Delete` never wait on a `Tick`
write.

- **Apply.** Each present entity keeps its last-applied revision and contribution. An `Upsert` or `Delete` at or below
  it changes nothing; a newer `Upsert` subtracts the old contribution and adds the new; `Delete` subtracts and forgets.
  Records after any history equal those computed from the final values.
- **Refusals.** `Upsert` refuses, with a typed invalid error naming the entity, a nil state, an ID the canonical parser
  refuses, and a state whose ID differs from `entityID`. A refused entity contributes nothing and stays in a refused
  set until a valid `Upsert` or a `Delete` for it.
- **Words.** Only words in string values count; terms from ID positions and predicate names do not. A run longer than
  `MaxWordBytes` (32) is not a word, by definition, so a long literal adds no keyword; nothing is truncated. Each
  entity contributes its `MaxEntityTerms` (32) most frequent words, lexical ties. Keywords are therefore the top words
  over capped contributions: a word outside every member's top 32 cannot be a keyword, an approximation the spec and
  the reference model state.
- **Memory.** Per entity: ID, revision, type, at most 32 words. Per group: member count, type counts, word counts, the
  ordered set of text-bearing members, and a copy of its last published record (at most `MaxRecordBytes`). A restart
  rebuilds from the replay.
- **Tick.** Refuses a nil context; returns `ErrBootstrapIncomplete` and writes nothing before `BootstrapDone`. It
  recomputes only groups touched since their last successful publication: member count, the top `MaxKeywords` types and
  words (lexical ties), the summary text, and the `MaxRepEntities` lexically smallest text-bearing members (from the
  ordered set, no scan). The cost per changed group, paid while `Upsert` and `Delete` wait, is proportional to its
  distinct words (a top-k selection, no full sort). It writes a record only when it differs from the last published, and
  only after every changed record is written does it remove anything: the records of groups that became empty and, on
  the first tick after bootstrap, the keys of groups that do not exist (one key listing; ADR-085 decision 7, the pin's
  order, `pin:graph/clustering/lpa.go:243-250`). If any write fails, nothing is removed and both the writes and the
  cleanup stay pending for the next tick, so the old records stay readable. It reads nothing from `COMMUNITY_INDEX`
  after the first tick and holds no `ENTITY_STATES` handle.
- **Failure signals.** A write or removal failure, or the context ending before pending work is done, returns an error
  and leaves the work pending. With a non-empty refused set, an otherwise complete tick returns `*IncompleteError`
  naming the refused IDs. `Tick` returns nil only when everything is published and nothing is refused.

## D3. Storage

`NATSCommunityStorage` over `*natsclient.KVStore`. Exported: `NewNATSCommunityStorage` (change 7 and products build
the store) and `GetAllCommunities` (change 7's anomaly half reads every record,
`pin:processor/graph-clustering/anomaly.go:342`). Save, remove and key listing are unexported. Required from the
inventory: S1 the nil-KV store goes (#186); S2 triples go; S3 key listing uses `KVStore.Keys`
(`natsclient/kv.go:582`); S4 an unreadable entry fails `GetAllCommunities`, naming the key; S5 a cancelled context
returns its error. Key `{level}.{prefix}`; no `entity.{level}.{id}` keys.

## D4. The record and its bound

`Community{ID, Level, MemberCount, TypeCounts, Keywords, StatisticalSummary, RepEntities}`; no member list, no
membership hash. Every field is bounded by construction: ID and representative IDs at most 256 bytes
(`pkg/types/entity_id.go:13`), at most 10 types (each a bounded ID position) and 10 keywords of at most 32 bytes, 5
representatives, and a template summary of 3 types and 5 keywords. The encoded record is therefore under
`MaxRecordBytes` (8 KiB), far below the 1 MiB NATS limit, with no runtime truncation; `TestRecordIsBounded` asserts it
with long literals. Exported names and their need: `Maintainer` and its methods (change 7's watch loop);
`IncompleteError`, `ErrBootstrapIncomplete` (`errors.As`, `errors.Is`); `Community` (item 9 and products decode it);
`MaxRepEntities` = 5 and `MaxKeywords` = 10 (as at the pin, `summarizer.go:53-57`), `MaxEntityTerms` = 32,
`MaxWordBytes` = 32, `MaxRecordBytes` (the bounds a reader or producer can rely on).

## D5. Proving tests

Generated: `TestMaintainerMatchesModel` applies seeded random histories (upserts, deletes, obsolete revisions,
refusals) and compares every record with a from-scratch model over the final set; the generator chooses each entity's
words, so the model takes them by construction (never running `terms.go`), splits IDs on `.` itself, and applies the
same 32-word cap. `TestHistoryOrderIrrelevant`: two histories that interleave entities differently, keep each entity's
revision order, and end with the same values give byte-identical records; obsolete deliveries only for entities still
present. Seed, run count and completed checks in notes.

`TestTickCost` runs on a counting fake `jetstream.KeyValue` behind a real `KVStore` (`natsclient.NewClient` does not
dial, `natsclient/client.go:312-341`; `NewKVStore` takes any `jetstream.KeyValue`, `natsclient/kv.go:55`): after the
first tick, k changed entities cause no `COMMUNITY_INDEX` read and writes only to their groups; an unchanged graph
writes nothing. Mutants: list keys every tick; read before write; drop the equality skip. The `ENTITY_STATES` half
cannot fail a test (no handle).

Unit: `TestObsoleteDeliveryIgnored`, `TestRefusedEntityMakesTickIncomplete`, `TestTickBeforeBootstrap`,
`TestFailedWriteRetried`, `TestFirstTickFailedPublicationKeepsOldRecords` (a stale key and a failing write: the stale
record survives and is removed on the next successful tick), `TestRecordIsBounded`, `TestLongLiteralIsNotAWord`,
`TestUpsertDuringBlockedWrite`, `TestKeywordsIgnoreIDAndPredicates`, `TestRepresentativesWithoutScan`,
`TestTickReportsCancel`. Integration on `natsfixture`: `TestUnchangedRecordAddsNoRevision`,
`TestGetAllCommunitiesFailsOnCorruptEntry`, `TestGetAllReportsCancel`, `TestMaintainerAsAProduct`. Mutation checks:
subtraction removed on replace; obsolete-revision check removed; equality skip removed; removal before writes;
`*IncompleteError` replaced by nil. Coverage floor 80 %.

## D6. ADRs, issues, merge order, held items

ADR-099 ported with a port note: its base partition applied, kept from changes, levels as amended by ADR-102; LPA and
edge synthesis not carried. ADR-085's note gains the package (decision 7 kept). ADR-087 stays out (#485). Retires
none. Stay open: #485, #32 (now only `graph/llm`, `model/wire`, `go-openai`), #68, #141, #477 (sites leave with the
tier-2 lane to #485; a comment on each). This PR re-points the ledger rows `graph/embedding/http_embedder.go` → #489
and `graph/query/classifier_llm_adapter.go` → #486. No open PR overlaps. Held for item 9, not designed here: the
reader's consistency contract (which committed generation of `COMMUNITY_INDEX` a reader sees during a tick) and its
prefix-page reads, which rescan keys per page (`graph/query_prefix_types.go:30-33`). Held for change 7: the watch
wiring and its refused config keys. A later change: a detection overlay (connected components over chosen relationship
predicates, default off, its own namespace, never a readiness input), gated by an exact-membership fixture with a
one-edge bridge and a semboids-style precision/recall and resource gate. semboids is a load generator; its member
decoder breaks under bounded records.
