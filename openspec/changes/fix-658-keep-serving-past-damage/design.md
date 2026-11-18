# Design: fix-658-keep-serving-past-damage

Premises are pinned in `inventory.md` (base `be77e6b5`).

## Context

One damaged entity (a stored value `graph.UnmarshalEntityState` refuses) latches the follower's reset for the life of
the process (`graph/follower/follower.go:363-365`), and every verb is then refused fatal (`:413-416`). graph-index,
graph-embedding and graph-lexical latch it on any damaged value; the follower latches it itself for a damaged value in
the startup replay (`:299-309`), which also stops graph-clustering's publication (`TestResetStopsPublication`). The
owner ruled the components keep serving (#657 ruling 3, moved up on #658) and that a damaged record is never left out
of a reply without the reply saying so (#657 ruling 1). Each of the four checks behind a refusal
(`graph/state_contract.go:43-52`) is about one stored value, so none is evidence that the whole store is under another
rule; a whole store under another rule shows as every entity damaged, which health names.

## Goals / Non-Goals

- Goals: #658's done-when list, for all four follower plug-ins; every reply that may have left a damaged entity out
  carries a count, through graph-query and the relays Q3 names.
- Non-goals: graph-ingest's verbs and its poison inventory; the write path; refusal detail and the scope of
  `unreadable_records` (#657 follow-ups); removing `reset_required` from the readiness vocabulary (follow-up issue).

## D1. The follower records damaged entities, one entry per key

- **A (recommended).** `Reset` becomes a per-entity report: the plug-in passes the key, the revision it read and the
  refusal. The follower keeps the latest entry per key and drops it when the plug-in completes a higher revision of
  that key or a delete is delivered for it. The follower no longer decodes in the replay. The latch, `resetRemedy`, the
  `reset_required` envelope and the fatal `QueryReady` branch go. Cost: the same four call sites change; one rule for
  clearing. No extra decode.
- **B.** The follower decodes every delivered value, live too, and plug-ins report nothing. Exact by construction, but
  one more decode per write in each of four followers (not measured).
- **C.** Each plug-in keeps its own set. Four copies of one fact; rejected.
- Two read methods for the plug-ins' handlers: the count of damaged entities, optionally kept by an ID filter, and
  the refusal stored for one ID (nil when it is not damaged). Production callers: the query handlers (D3).
- A damaged entity never withholds a query. graph-index's `noncanonical_entity_id` case leaves its failed set, and every
  damaged revision completes, so the first build finishes with damage stored.

## D2. Rows built from the entity's last good value (Q1)

- **A (recommended): removed.** The entity is answered as if absent, and the count says one was left out. Each
  plug-in already has the removal path: graph-index `deleteFromIndexes` (`index.go:520`), graph-embedding
  `removeRecord` (`hop1.go:121`), graph-lexical's delete path (`apply.go:152-153`), graph-clustering `maintainer.Delete`
  in place of `Upsert(nil)` so the follower counts the entity once, not also as `refused_entity`. A removal that fails
  is the plug-in's existing failed write: withheld and retried by its repair loop.
- **B: kept until a good value arrives.** A reply would then hold facts the store no longer holds, under an
  `indexed_revision` that claims every write at or below it is reflected (`graph/query_response.go:11-13`).

## D3. A reply says how many it left out

- **A (recommended):** `graph.QueryResponse` gains `damaged_entities` (int, omitted at 0), the damaged entities the
  producer held when it answered. One field covers every enveloped verb. A count follows ruling 1's third case (a
  skipped derived record is counted); the IDs go to health (D5), so a reply stays bounded.
  - Verbs with a `scope` (word search, embedding `search`) count only damaged IDs in scope; others count all.
  - A verb naming one entity that is damaged refuses it as the entity verb does (fatal
    `graph_state_reset_required`, the stored refusal): graph-index `outgoing` and embedding `similar`'s source.
    `incoming` answers, since its rows are the other entities' facts.
- **B:** a field on each of 12 data types. More surface, and graph-query merges them per type.
- **C:** health only. Breaks ruling 1.

## D4. Relays carry the count

- graph-query: a composed reply carries the largest `damaged_entities` among the enveloped replies it read, beside the
  existing "smallest revision" rule (graph-transport-boundary). Forwards pass bytes unchanged already.
- Q2: `localSearch`, `globalSearch` and `searchGraph` set `truncated` while the count is above 0.
- Q3: fusion (`fusionnats.decodeReply`, `client.go:446-465`) and graph-gateway (`resolve.go:82-86`) keep only `data`.

## D5. Health, envelope, readiness, repair

- `Stuck` gains one sentence naming up to 10 damaged IDs with their reasons, the count, and the repair (write a valid
  value or delete the key), as graph-ingest's poison sample does (`poison_inventory.go:227-241`). `Healthy` is false
  while one is stored (ADR-109 D2, settled).
- The envelope reads `degraded` with the count in `failed_count` and `failed_reasons["damaged_entity"]`; no ID list on
  the watched key (`graph/index_status.go:76-78`).
- A valid write or a delete is picked up without a restart: the rows return, the count and the sentence go.

## D6. Records

ADR-108 D4 and ADR-109 D1/D2 are amended in place citing the rulings; ADR-074 and ADR-084 get a port-note bullet
saying the stop is replaced here. No new ADR; no owner question, since the ruling covers the change.

## Adopter seam

- A caller must know one fact: `damaged_entities` above 0 means rows may be missing. Doing nothing, a caller gets
  correct answers about every other entity and the gap is in the reply.
- Migration notes: semsource's mcp-gateway reads `truncated` only (`graph_matches.go:130`), covered by Q2; semconnect's
  `systems.go:993-997` should read the count; its readiness tool's `reset_required` check (`main.go:155`) is
  never true again and the envelope reads `degraded` instead.

## Proving tests (failing first, real NATS, `t.Context()`, no sleeps)

- Each component, replay and live: A good, B damaged (stored before start; overwritten after). Expect: A's answers
  (graph-index `byName`, `alias`, `outgoing`; embedding `search`; lexical `search`; clustering records), B absent with
  `damaged_entities` 1, `outgoing`/`similar` on B fatal, `Health` naming B, the envelope `degraded` with
  `damaged_entity` 1; then a valid B: B answers, count gone, `Healthy`. On `main` each request is refused fatal.
  The oracle is the test's own fixture.
- Follower: generated check (rapid, seed recorded) over reports, completions and deletes on a few keys, against a
  model where a key is damaged when its highest-revision event is a report; named cases first.
- graph-query: stubbed index replies at 2 and 3 give a composed reply at 3; on `main` the field is absent.

## Size and merge order

About 350 production and 1,000 test lines. PR 1: `graph` and graph-query (D3, D4). PR 2: fusion and graph-gateway, if
Q3 is yes. PR 3, on draft #693: the follower and the four plug-ins (D1, D2, D5), records and docs, in one PR because
the follower's contract and its four callers change together. Merge 1, 2, 3, so no reply loses a record unsaid.

## Risks

- Embedding can complete a key's higher revision after a failed read; the entry then clears until the repair lane
  reads the value again. Stated in the spec as a limit.
- Health lowers `Healthy` while damage is stored, so `/readyz` reads not ready, as graph-ingest's poison already does.

## Open Questions (owner)

- Q1 (D2), Q2 (D4), Q3 (D4): in the proposal.
