# Design: fix-658-keep-serving-past-damage

Premises are pinned in `inventory.md` (base `be77e6b5`).

## Context

One damaged entity (a stored value `graph.UnmarshalEntityState` refuses) latches the follower's reset for the life of
the process (`graph/follower/follower.go:363-365`; every verb refused at `:413-416`). graph-index, graph-embedding and
graph-lexical latch it on any damaged value, and the follower latches it for one in the startup replay (`:299-309`),
which also stops graph-clustering. Rulings: the components keep serving (#657 ruling 3, moved up on #658); a damaged
record is never left out of a reply without the reply saying so (#657 ruling 1); #658's done-when: "each reply says
that records were left out". Each of the four refusal reasons (`graph/state_contract.go:43-52`) is about one value, so
none should still stop a component: a whole store under another rule shows as every entity damaged, named in health.

## Goals / Non-Goals

- Goals: #658's done-when for all four plug-ins; every reply that may omit a damaged entity counts it, relays too.
- Non-goals: graph-ingest's verbs and poison inventory; the write path; refusal detail; `unreadable_records` scope.

## D1. The follower records damaged entities, one entry per key

- **A (recommended).** `Reset` becomes a per-entity report (key, revision read, refusal). One entry per key, kept at
  the highest reported revision, dropped when the plug-in completes a higher revision of the key or a delete is
  delivered above it. Completions at or below the report change nothing (graph-index reports the revision `Get` read,
  `index.go:193`, which can be above the delivered one it completes). No decode in the follower; the latch,
  `resetRemedy`, the fatal `QueryReady` branch and the `reset_required` envelope go. Two read methods: the count (an
  optional ID filter) and one ID's recorded refusal.
- **B.** The follower decodes every value: a second decode per write in four followers. **C.** A set per plug-in.
- Order: a plug-in reports before it removes rows (D2), and a key clears only on a completion that follows writing its
  valid rows, so no reply lacks B's rows while counting 0. A report never withholds; damaged revisions complete, so
  the first build finishes. A report that newly records a key counts one `ErrorCount`, as one reset does today.
- `reset_required`, `IndexStatusResponse.Code` and `Reason` lose their only producer; PR 3 removes them and the gate's
  branch (graph-readiness delta); a stale key holding it reads `unrecognized_state`, still a defer.

## D2. Rows built from the entity's last good value (Q1)

- **A (recommended): removed**, with each plug-in's existing path: graph-index `deleteFromIndexes` (`index.go:520`),
  graph-embedding `removeRecord` (`hop1.go:121`), graph-lexical's delete (`apply.go:152-153`), graph-clustering
  `maintainer.Delete` in place of `Upsert(nil)`. A failed removal is the plug-in's failed write, as today.
- **B: kept.** Replies would hold facts the store no longer holds under an `indexed_revision` that claims every write
  at or below it (`graph/query_response.go:11-13`).
- graph-clustering's refused set and `IncompleteError` (`maintainer.go:143-147`) become unreachable from the component,
  since every value it upserts decoded. Kept: they are `graph/clustering`'s own contract for any caller.

## D3. A reply counts what it left out

- **A (recommended):** `graph.QueryResponse.DamagedEntities` (`damaged_entities`, omitted at 0). A count, as ruling 1
  gives skipped derived records; the IDs go to health. Verbs with a `scope` count damage in scope; others count all.
- **B:** a field on 12 data types. **C:** health only, against ruling 1.
- Against ruling 1, verb by verb. Its second case (refuse) was ruled for graph-ingest reads that decode entities; these
  replies come from derived rows (third case: count), and #658's done-when asks every reply to answer and say so.

| Reply | Case | This change |
| --- | --- | --- |
| index `outgoing` of B; embedding `similar` from B | caller named B | refuse fatal, recorded refusal |
| index `incoming` to B | others' facts | answer, count |
| index `alias`, `byName`, `predicate` (value filter too), `predicateList`, `predicateStats`, `predicateCompound` | finds own; figures | answer, count (#658 done-when) |
| embedding `search`, `similar` results; lexical `search` | finds own | answer, count in scope |
| graph-query forwards | as above | bytes unchanged |
| `relationships` of B | caller named B | refuse (index `outgoing`) |
| `pathSearch` reaching B | path search | refuse: the hop ends the walk (`pathsearch.go:102-120`) |
| `summary`, `entityByAlias`, `localSearch`, `globalSearch`, `searchGraph` | finds own; figures | answer, largest count |
| `hierarchyStats`, graph-ingest verbs | unchanged | as today (#656) |

- graph-query's `data.damaged` (#656) names hits the batch found damaged, the window before graph-lexical removes
  them; `damaged_entities` counts what a component left out. Both can name one entity; neither is a sum.
- `COMMUNITY_INDEX` records leave B out too; `localSearch` scopes its word search to the group's prefix, so the count
  covers a damaged member of the group whose record it attaches.

## D4. Relays

- graph-query: the largest `damaged_entities` among the enveloped replies read, beside the smallest-revision rule.
- Q2: `localSearch`, `globalSearch`, `searchGraph` set `truncated` while the count is above 0. Cost: unscoped search
  counts every damaged entity, so one anywhere makes every `globalSearch` and `searchGraph` reply `truncated`.
- Q3, fusion: `fusion.Response.DamagedEntities` (`damaged_entities`, omitempty), the largest count any read of the
  answer reported. Not `Truncated`, which means the budget cut nodes (`README.md:71`); a count beside
  `bodies_not_indexed`. `Resolution` gains the field; `Neighbors` returns `(fusion.Neighbors, error)` with `Edges` and
  `DamagedEntities`, `Names` `(fusion.Names, error)` with `Names` and `DamagedEntities` (`retrieval.go:41,43`).
  semsource's `fusiontest.MemGraph` no longer compiles until it returns the structs.
- Q3, graph-gateway: a response whose resolvers read a reply with a count carries top-level
  `extensions.damagedEntities` (largest); no schema change, so "exactly the selected fields" holds.

## D5. Health, envelope, repair

- `Stuck` names up to 10 damaged IDs with reasons, the count and the repair (write a valid value or delete the key),
  as graph-ingest's sample (`poison_inventory.go:227-241`). `Healthy` false while stored (ADR-109 D2, settled).
- Envelope: `degraded`, `failed_count` plus `failed_reasons["damaged_entity"]`, no IDs (`graph/index_status.go:77`).
- A valid write or a delete is picked up with no restart.
- Records: ADR-108 D4 and ADR-109 D1/D2 amended citing the rulings; ADR-074 and ADR-084 get a note. No new ADR.

## Adopter seam

One fact: `damaged_entities` above 0 means rows may be missing. Doing nothing, a caller's answers about every other
entity are right and the gap is in the reply. Migration notes: semsource reads only `truncated` (Q2) and must update
`MemGraph` (Q3); semconnect's `systems.go:993-997` should read the count, and its readiness tool's
`IndexStateResetRequired` (`main.go:155`) no longer compiles.

## Proving tests

Failing first, over real NATS, with `t.Context()` and no sleeps; the oracle is the test's own fixture.

- Each component, replay and live: A valid, B damaged (stored before start; overwritten after); then B repaired.
  Waits: graph-index and graph-lexical, a request with `min_revision` B's revision; graph-embedding (replies carry
  `indexed_revision` 0), the follower's floor, then the vector cache past B's delete; graph-clustering, the floor,
  then a tick by hand. On `main` each request is refused fatal.
- Follower: a generated check (rapid, seed and runs recorded) drawing reports, completions at, below and above the
  report, and deletes, against a model; named cases run first.
- graph-query, fusion, gateway: stubbed replies at 2 and 3 give 3; on `main` the count is absent.

## Size and merge order

About 450 production and 1,200 test lines. PR 1: `graph`, graph-query. PR 2 (Q3): fusion, graph-gateway. PR 3, on
draft #693: follower, four plug-ins, readiness vocabulary, records, docs; one PR, as the follower contract and its
callers change together. Merge 1, 2, 3.

## Risks and open questions

- Embedding can complete a higher revision after a failed read, clearing B until its repair lane reads it again;
  meanwhile the `entity_read_failed` hold refuses queries transient `index_not_ready`, so B is never left out unsaid.
- A damaged entity makes `/readyz` not ready while stored, as graph-ingest's poison does today.
- Owner: Q1 (D2), Q2 (D4), Q3 (D4), in the proposal.
