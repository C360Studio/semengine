# Inventory: fix-656-batch-damaged-entities

base: e9d8b5362f486b5a5cad5964159941bed8746c7c (main 6b72b61f + claim commit; no code change)

Question: what changes when a batch read names each damaged entity in `missing` with reason `damaged`, as #657's
rulings 1–2 (comment 6877432505) and #656 ask, and who reads that reply? Repositories read: this one; consumer
checkouts semsource `e1e62fd`, semteams `c8571b34`, semsparql `e7f827c`, semconnect `dff1265` (local heads, not
fetched). No pin package is carried, so § 6 is one line.

## 1. The claimed gap

- No `damaged` reason exists: `git grep -n '"damaged"\|MissingDamaged' -- '*.go'`: no hit. The closed set is
  `graph/query_batch_types.go:28` — `MissingNotFound MissingReason = "not_found"` and `:34` —
  `MissingError MissingReason = "error"`.
- The batch refuses whole on damage: `processor/graph-ingest/query.go:733-734` — `if len(poisoned) > 0 {` /
  `return nil, nil, aggregateEntityPoisonError(poisoned)`. Damage wins over another fault (`:736` follows it). The
  handler returns it at `query.go:136` — `return nil, classifyEntityQueryError(err)`.
- The same function serves the prefix page: `query.go:253` —
  `entities, _, err := c.fetchEntitiesConcurrent(ctx, pageKeys, defaultMaxConcurrent)`. Ruling 1 keeps that page
  refusing, so the change must not move the prefix page.
- Empty IDs are reported `error`: `query.go:159-163` (`batchMissingReason`). Not-found IDs are reported
  `not_found`: `query.go:683-688`. Another read fault fails the batch: `query.go:736-738` — `if firstOtherErr != nil {`.
- Duplicate requested IDs are not de-duplicated before the reads (`query.go:614-631`), so a repeated not-found ID is
  listed twice today.

## 2. Every spelling of the facts modelled

"A requested ID absent from the batch reply, and why":

- Wire type: `graph/query_batch_types.go:20-56`.
- Producer: `query.go:147-150`; counter by reason `query.go:173-185`; counter seeds
  `processor/graph-ingest/metrics.go:362` —
  `for _, reason := range []graph.MissingReason{graph.MissingNotFound, graph.MissingError} {`. The debug log names
  one fixed reason whatever was missing: `query.go:199` — `"reason", string(graph.MissingNotFound),`.
- Readers in this repository:
  - `pkg/fusion/fusionnats/client.go:347-356` (`unhydratedReason`; an unknown reason becomes
    `fusion.UnhydratedUnknown`, `:353-354`) and `:365-392` (`reconcileHydration`).
  - `processor/graph-query/matches.go:70-73` (`batchReply`) and `:113-131`: every `missing` entry, whatever its
    reason, is counted in `missing` and left out.
- Readers in consumers:
  - semconnect `gateway/cs-api/systems.go:896` — `case graph.MissingNotFound, graph.MissingError:`, with
    `default:` refusing the batch as an "invalid missing reason". semconnect is still on the SemStreams pin
    (`go.mod:6`).
  - semsparql `reader/nats.go:287-317` passes reasons through. `sparql/executor.go:1564` —
    `if m.Reason != "" && m.Reason != graph.MissingNotFound {` fails the query naming the ID and reason. semsparql is
    on semengine `v0.1.0-alpha.2`.
  - semsource and semteams: no reader of the batch verb or of `MissingReason`. Searched with
    `git grep -e 'graph\.query\.batch' -e 'ingest\.query\.batch' -e 'MissingReason' -e 'EntityBatchResponse'`.
    semsource reads `fusion.Unhydrated` (`source/fusion/fusiontest/memgraph.go:99-110`;
    `processor/code-context/exact_seed.go:56`).

"A damaged seed in fusion": `client.go:276-282` (the refusal sends it to `entitiesAlone`), `:293-342`
(`entitiesAlone`; a damaged ID becomes `fusion.UnhydratedError` at `:328-330`), and `pkg/fusion/retrieval.go:110-112`
(the doc of `UnhydratedError` names "fusionnats lists a poisoned seed this way").

"A damaged entity is recorded": `processor/graph-ingest/mutation_runtime.go:150-167` (`readEntity` records it at the
revision it read). Detection evicts the cached entry: `poison_inventory.go:107` —
`c.invalidateEntityCacheEntry(id)`. A damaged value is never cached, because the cache is filled only after a good
read (`query.go:694`).

## 3. Adjacent claims

- Owner rulings: #657 comment 6877432505 (rulings 1, 2, and "#656: ..."). Owner on 2026-11-18: design track, #656 alone.
- Accepted design that differs (O1): `openspec/changes/archive/2026-11-14-setup-04b-03-graph-query-lexical/design.md:54-56`
  — "after which graph-query's `missing` path takes it unchanged (chosen)".
- Specs:
  - fusion `:22-30` (cross-references "A poisoned seed is unhydrated, not a failed read"), `:297-312`, `:314-329`,
    `:331-339`.
  - graph-query `:671-688` ("A hit the batch names missing SHALL be left out and counted in `missing`").
  - graph-transport-boundary `:261-291`: refusal codes and the empty batch, with no statement of the `missing`
    reasons; `git grep -n 'not_found' -- openspec/specs/graph-transport-boundary` gives only mutation outcomes.
  - graph-entity-writes `:395-397` lists the batch among the validated reads (unchanged).
- Tests that assert today's batch refusal:
  - `processor/graph-ingest/poison_scoping_test.go:289-294` and `:429-430`.
  - `pkg/fusion/fusionnats/poison_test.go:113-295` (every test but `TestBatchFaultNotReadAlone` uses
    `batchErr: resetRefusal()`).
  - `pkg/fusion/fusionnats/client_integration_test.go:439-445`.
- Stays as is: `processor/graph-ingest/query_contract_guard_test.go:513` (`TestBatchBackendFailureReturnsNoPartialEntities`).
- Admission ledger rows describing the refusal: `docs/admission-ledger.yaml` graph-query `:4407`, `:4436`;
  fusionnats contract item (6) `:4828-4834`, side effects `:4844`, `proving_tests` `:4875-4878`.
- Docs stating the refusal: `processor/graph-ingest/README.md:126-128`, `processor/graph-query/README.md:150-152`,
  `processor/graph-query/MIGRATION.md:259-261`, `pkg/fusion/README.md:114-120`, `pkg/fusion/MIGRATION.md:96-100`,
  `docs/repository-map.md:91` ("One damaged hit refuses a search until it is fixed").
- Open PRs: `gh pr list --state open` gives #692 only (this claim, 0 files).
- Open issues: `gh issue list --state open --search graph-ingest` and `--search fusionnats`.
  - Related: #657 (stays: its other follow-ups), #658 (stays), #659 (stays: 40 large seeds still exceed max payload),
    #630 (test messages in graph-ingest; stays, though this change edits `poison_scoping_test.go`).
  - The rest name the word, not this change: #270, #502, #300, #141, #195, #196, #602, #111, #109, #110, #134, #9,
    #570, #108, #641, #10, #350, #24, #621.

## 4. Who it is for

- `graph.MissingDamaged` (new exported constant).
  - Production callers in this repository: graph-ingest's batch path (`query.go:147-150`), fusionnats
    `unhydratedReason`, graph-query `readMatches` (if O1 is ruled B).
  - Consumers: semconnect `systems.go:896` and semsparql `executor.go:1564`.
- No new subject, bucket, config field or verb.

## 5. The problem shape

The shape is "a multi-ID read that lists per-ID absences beside what it returns". The closest instance is the same
verb's not-found path, `query.go:683-688` with `:147-150`. This change adopts it: damage takes the not-found path
with its own reason. The prefix-IDs listing (#578, archived) is the same rule's other half.

## 6. Pin decision records

No pin package is carried. ADR-109 `:77` and `:196` (in this repository): "poison refuses only the poisoned
entities". This change brings the batch in line with them. Their rewording is out of scope by the owner's choice.

## Adopter seam inventory

The surface is the `missing` list of `graph.query.batch` and `graph.ingest.query.batch`, and fusion's `unhydrated`.

1. **What must they know?** A batch reply can name an ID `damaged`, and that ID's entity read refuses fatal
   `graph_state_reset_required`. One fact.
2. **What happens if they do nothing?**
   - A caller that reads `entities` only gets the good entities, where today it gets a refusal. That loses nothing:
     the reply names the ID.
   - A caller that switches on the reason exhaustively, such as semconnect, refuses the batch loudly with
     "invalid missing reason".
   - A fusion consumer sees no change if O2 is ruled A.
3. **Where do they find out?** From the reply itself, a typed runtime value, or from a refusal raised by the caller's
   own code. Nothing is lost silently.
4. **What should they have to know?** Nothing beyond item 1. The gap is the semconnect switch, a migration note.

Collision table: not triggered. No durable, communication or coordination primitive is proposed.
