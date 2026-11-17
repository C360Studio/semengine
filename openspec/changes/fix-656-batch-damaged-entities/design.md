# Design: fix-656-batch-damaged-entities

The premises are pinned in `inventory.md` (base `e9d8b536`).

## Context

Today a batch read refuses whole when one requested entity is damaged (`processor/graph-ingest/query.go:733-734`).
Fusion works around this by reading each ID alone (`pkg/fusion/fusionnats/client.go:276-342`). graph-query's word
search passes the refusal on, so one damaged hit refuses a search. Owner rulings on #657: the caller named the IDs,
so the batch returns the rest and lists each damaged ID in `missing` with reason `damaged`; fusion's re-read goes.

## Goals / Non-Goals

- Goal: the batch answers good entities and names damaged ones, shown by an integration test that fails on `main`.
- Goal: fusion's fallback and its spec scenarios go.
- Non-goals:
  - the prefix page, which keeps refusing (ruling 1);
  - refusal detail, graph-index, the embedding count and ADR-109 wording (#657 follow-ups);
  - the three components that stop serving (#658);
  - de-duplicating requested IDs.

## D1. graph-ingest's batch names each damaged ID

- **A (chosen, ruled).** Add `graph.MissingDamaged = "damaged"`. It is documented as "the stored value under this ID
  is refused by the stored-value rule; its entity read refuses fatal `graph_state_reset_required`".
  - Batch behaviour:
    - every readable entity is in `entities`;
    - each damaged ID is in `missing` with `damaged`;
    - not-found and empty IDs keep `not_found` and `error`;
    - when every ID is damaged, the reply is `{"entities":[],"missing":[…]}` and is not a refusal;
    - a read fault that is not damage still fails the whole batch transient `internal`, so damage no longer outranks
      a fault;
    - a damaged ID requested twice is listed twice, as a not-found ID is today.
  - The damaged-entity record, its one ERROR log and `Health()` are unchanged, because the record is made by
    `readEntity` (`mutation_runtime.go:150-167`).
  - The counter seeds and counts `damaged`, and the debug log gives each reason's count (`query.go:199` names
    `not_found` for every reason today).
  - The prefix page keeps its refusal. The developer chooses how the shared fetch tells the two callers apart; the
    prefix tests prove it.
  - Cost: about 20 production lines; one closed-set value added.
- **B. Do nothing.** #656 stays open and fusion keeps its fallback. This goes against the ruling.

**Cache.** A damaged value is never cached (the cache is filled after a good read, `query.go:694`), and detecting
damage evicts the entry (`poison_inventory.go:107`). So every batch that names a damaged ID reads it from KV again and
re-records it. The record logs once per entity. A good entry still in the cache is served as before. This change
does not alter how the cache behaves around a write made from outside graph-ingest.

## D2. Fusion drops the re-read

- `Entities` sends one batch and reconciles it. On any refusal it returns the error and sends no `entity` request.
  `entitiesAlone` and its branch are deleted (about 50 lines; most of `poison_test.go` goes with them).
- **O2 (owner).** What should a damaged seed show in fusion's `unhydrated`?
  - **A (recommended):** `damaged` maps to `fusion.UnhydratedError`, so a fusion consumer sees exactly what it sees
    today (`retrieval.go:110-112`, `README.md:114-120`), and fusion's closed set is unchanged. semsource reads this
    list.
  - **B:** a new `fusion.UnhydratedDamaged`, which is new product-facing surface, and a consumer's exhaustive switch
    must add it.

  The ruling's "in a per-ID list" was asked about the batch's `missing`. Whether it reaches fusion's list is the
  owner's call.
- Without a mapping, `damaged` would fall through to `unknown` (`client.go:353-354`).
- If every seed is damaged, the answer is `deferred` `all_seeds_unhydrated`, as today.

## D3. graph-query's word search

`readMatches` counts every `missing` entry as a hit "gone when read back" (`matches.go:113-131`;
`graph/query_local_search_types.go:52-53`). With D1 alone, a damaged hit would become `missing: 1, truncated: true`,
labelled as gone.

- **O1 (owner).**
  - **B (recommended):** a hit the batch names `damaged` makes graph-query refuse the search, fatal
    `graph_state_reset_required` naming the ID. This is today's observable behaviour, and it fits ruling 1's second
    case: the read finds its own records, so it refuses.
  - **A:** let the `missing` path take damaged hits, as the lexical design said (`…graph-query-lexical/design.md:54-56`).
    That needs no code, but the `Missing` doc and spec change to "gone or damaged", and the caller cannot tell which.
- Stakes: graph-lexical stops on the first damaged entity it is sent (`processor/graph-lexical/component.go:240-251`),
  so this path is reached only in the window before graph-lexical sees the damaged write. B costs about 5 lines and a
  test.

## Adopter seam

- A consumer must know one fact: `damaged` can appear in `missing`.
- Doing nothing, a caller that reads `entities` alone gets the good entities and loses nothing silently.
- semconnect's exhaustive switch (`gateway/cs-api/systems.go:896`) refuses the batch loudly. This needs a migration
  note, not an owner question.
- semsparql already fails a query on any reason other than `not_found` (`sparql/executor.go:1564`).
- Mixed versions (fusion new, graph-ingest old) return the old refusal from `Fuse` as fatal. Both ship in one module
  version.

## Proving tests (failing first)

- **graph-ingest, integration (`TestIntegration_BatchQuery_NamesDamagedEntities`)** over real NATS:
  - Setup: write A and C canonically, then Put damaged bytes under B through the KV handle (as
    `client_integration_test.go:432-436` does).
  - Request `[A, B, C, absent]` on `graph.ingest.query.batch` with `RequestClassified`.
  - Expect: entities {A, C}, `missing` {B `damaged`, absent `not_found`}, the entity verb on B refusing fatal
    `graph_state_reset_required`, and graph-ingest `Healthy` false.
  - On `main` the request is refused `graph_state_reset_required`, which shows the test can fail. The output goes in
    `notes/2.1.md`.
- **All damaged (`TestIntegration_BatchQuery_AllDamaged`):** `[B]` answers `entities` `[]` and `missing`
  `[{B damaged}]`, with no error.
- **Damage plus fault (`TestBatchDamageBesideFaultIsTransient`):** a mock bucket where B is damaged and D fails
  transient refuses transient `internal`. The existing `TestBatchBackendFailureReturnsNoPartialEntities` stays.
- **Prefix unchanged:** the `prefix` subtest of `TestAggregateReadNamesEveryPoisonedEntity` and the prefix site of
  `TestEveryEntityReadRefusesTheSameStoredPoison` pass untouched.
- **Generated check (rapid; record the seed and the completed runs).**
  - Draw requested IDs with repeats. Each ID is good, good-and-cached, damaged, absent or empty.
  - The reference model maps each requested occurrence to an outcome from its drawn state alone: entity, `damaged`,
    `not_found` or `error`.
  - Law: the reply's entities and `missing` together match the model's multiset exactly, and every damaged ID is
    recorded.
  - Named cases run first as their own tests.
- **Counter:** after the integration request, `batch_query_missing_total{reason="damaged"}` has risen by 1, and the
  series exists at 0 before any request.
- **fusionnats:**
  - Unit: the batch reply names B `damaged`, giving A and C hydrated in request order and B unhydrated `error`.
  - Unit: a batch refused `graph_state_reset_required` is returned with no `entity` request.
  - `reconcile_prop_test.go` draws `damaged` too.
  - Integration: `TestPoisonedSeedOverRealResponders` now asserts as its precondition that the batch names poison
    `damaged`, which fails on `main`. Its `Entities` assertions stay.
- **graph-query (if B):** `TestSearchRefusesDamagedHit`. A stub batch names one hit `damaged`, and `localSearch` and
  `globalSearch` refuse fatal `graph_state_reset_required` naming it. `TestSearchDropsMissingHits` stays for
  `not_found`.

## Merge order

No other open PR overlaps. #692 merges alone.

## Open Questions

- O1 (owner): graph-query refuses a search with a damaged hit (B, recommended) or counts it in `missing` (A).
- O2 (owner): fusion maps `damaged` to `error` (A, recommended) or gains `UnhydratedDamaged` (B).
