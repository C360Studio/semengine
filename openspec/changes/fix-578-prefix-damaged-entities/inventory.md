# Inventory: fix-578-prefix-damaged-entities

base: ecbd4b923741290d14e8fa89084a5565598ef5ac (origin/main 80525179 + empty claim commit)

Question: what must change so graph-ingest's prefix read returns the undamaged entities on a page and names each
damaged one, and who reads that reply. Repositories read: this one; consumers semsource, semconnect, semteams
(symbol-level grep, `docs/inventory-scope.md` rule 2). No pin package is carried; § 6 is one line.

## 1. The claimed gap

- The prefix reply has no per-entity list: `graph/query_prefix_types.go:71` — `type PrefixQueryResponse struct {`;
  fields `Entities`, `Total`, `NextCursor` only (`:73-79`).
- The prefix handler refuses the page on any error from the fetch: `processor/graph-ingest/query.go:330-333` —
  `entities, _, err := c.fetchEntitiesConcurrent(ctx, pageKeys, defaultMaxConcurrent)` / `return nil,
  classifyEntityQueryError(err)`.
- The fetch turns any damaged entity into one aggregate error: `query.go:603-604` — `if len(poisoned) > 0 { return
  nil, nil, aggregateEntityPoisonError(poisoned)`; built at `:629-643`, fatal `graph_state_reset_required`.
- Premise in the brief "mirror the batch reply's Missing": the batch reply never names a damaged entity. The batch
  handler refuses the same way (`query.go:130-137`); `graph/query_batch_types.go:32-36` — "the only current emitter
  is a malformed requested ID (the empty string)"; `query.go:158-162` (`batchMissingReason`). So only the shape is
  mirrored.
- Decode is the only source of a `*graph.StateContractError` on this path:
  `processor/graph-ingest/mutation_runtime.go:155-161` (`decodeStoredEntity`, then `inventoryEntityPoison`); the
  fetch sorts it from other errors with `errors.As` at `query.go:594-598`.

## 2. Every spelling of "an entity on this read is damaged"

- Aggregate error, batch and prefix: `query.go:629` `aggregateEntityPoisonError`.
- Per-entity record and health: `mutation_runtime.go:159` `c.inventoryEntityPoison(...)`; spec
  `openspec/specs/graph-ingest-recovery/spec.md:225` (Healthy false while a record is held). Unchanged by this change.
- Wire per-ID reason: `graph/query_batch_types.go:22-36` `MissingReason` {`not_found`, `error`}, `MissingEntity` `:39-42`.
- Fusion's mapping: `pkg/fusion/fusionnats/client.go:338-346` `unhydratedReason` (`MissingError` -> `UnhydratedError`);
  fallback on a reset refusal: `client.go:268-270`, `:319-321`.
- graph-query's own declared count of damaged records (community records, not prefix):
  `openspec/specs/graph-query/spec.md:806`.

No new spelling is needed: the prefix reply reuses `MissingEntity` / `MissingError`.

## 3. Adjacent claims

Readers of `PrefixQueryResponse` (`gopls references graph/query_prefix_types.go:71:6`, production only):

- `processor/graph-ingest/query.go:440` (builds it).
- `pkg/fusion/fusionnats/client.go:131` (prefix resolve; seeds from `resp.Entities`, `:138-141`).
- `gateway/graph-gateway/resolve.go:141` (`entitiesByPrefix`; answers `{entities,total,next_cursor}`, `:156`).
- `graph/query_verbs.go:76`, `:108` (reply type of `graph.ingest.query.prefix` and `graph.query.prefix`).

Readers gopls cannot see (anonymous struct, `git grep -n 'readCountPage' -- processor/graph-query`):

- `processor/graph-query/counts.go:32-75` `readCountPage` decodes `entities`/`total`/`next_cursor`, ignores unknown
  keys; callers `counts.go:99` (hierarchyStats), `summary.go:50`, `localsearch.go:275`, `textsearch.go:74`
  (globalSearch).
- `processor/graph-query/query.go:137-141` `forward` relays `graph.query.prefix` bytes unchanged.

Helpers: `gopls references query.go:477:22` (`fetchEntitiesConcurrent`) -> `query.go:130` (batch), `:330` (prefix);
`gopls references query.go:457:6` (`classifyEntityQueryError`) -> `:137`, `:332`.

Specs that state today's behaviour:

- `openspec/specs/graph-transport-boundary/spec.md:270` (codes: `graph_state_reset_required` for a refused stored
  value); `:468-483` (prefix total).
- `openspec/specs/graph-entity-writes/spec.md:393-396` (prefix verb applies the decode rule).
- `openspec/specs/fusion/spec.md:22-32` ("A fatal seed read fails the call", names a prefix resolve's reset);
  `:315-330` (poisoned seed unhydrated).
- `openspec/specs/graph-query/spec.md:331-355` ("Counts declare a partial page").
- `openspec/specs/graph-gateway/spec.md:58-74` ("Answers follow the gateway's schema and are checked").

Tests that pin today's whole-page refusal:

- `processor/graph-ingest/poison_scoping_test.go:296-302` (prefix subtest of
  `TestAggregateReadNamesEveryPoisonedEntity`).
- `poison_scoping_test.go:432-434` (prefix site of `TestEveryEntityReadRefusesTheSameStoredPoison`).

ADRs: `docs/adr/109-ready-after-boot.md:77` (D1: "poison refuses only the poisoned entities"); `:196` (pin ADR-079 D3
kept, "the refusal stays per entity"); pin ADR-079 D3 "aggregate reads failing loudly" quoted at
`openspec/changes/archive/2026-11-09-setup-04a-05-fusionnats/inventory.md:55-56`, § 2 `:63-64` finds this gap.

Open pull requests (`gh pr list --state open`, 2026-11-14): #655 (this claim); #654 draft, `setup-04b-03`
graph-query lexical, 0 files, overlaps `processor/graph-query` (`textsearch.go:74` reads `readCountPage`); #597, #598
(go.mod only, no overlap). No open PR touches `processor/graph-lexical` together with these files.

Open issues: #578 (this); #300 item D names `marshalFittingPrefixPage`'s double marshal (optional, stays); #621, #563
(no overlap with the reply). `gh issue list --state open --search prefix`: #506, #624 name the word, not this reply.

## 4. Who it is for

- New field `PrefixQueryResponse.Missing`: plumbing for `pkg/fusion/fusionnats/client.go:131`, graph-query
  `counts.go:32`, graph-gateway `resolve.go:141`; contract line `graph/query_prefix_types.go` doc plus the
  graph-transport-boundary requirement this change adds.
- Consumers reading the reply directly (grep of the sister checkouts, read-only): semsource
  `test/setup03a/removal_test.go:91`, `removal_replay_helpers_test.go:417` (read `graph.query.prefix` entities);
  semconnect `ui/src/lib/services/graphGateway.ts:109`, semteams `ui/src/lib/services/graphApi.ts:259` (read the
  gateway's `entitiesByPrefix.entities`). semboids: no hit.

## 5. Problem shape

"Return what was read, list each item not read, with a reason; fail the call only for faults that are not per item."
Closest instances: batch reply `query.go:146-149` (shape, for not-found and empty IDs only);
`fusionnats.entitiesAlone` `client.go:292-334` (same rule per seed, for damaged entities). This change adopts the
batch shape and fusion's rule.

## 6. Pin decision records

No package is carried from the pin. The governing decisions are in this tree: ADR-109 D1 (`:77`) and the pin
ADR-079 D3 it keeps (`:196`).

## Adopter seams

- A caller of `graph.query.prefix` who never reads `missing` gets fewer entities and no error (silent). Found out:
  doc only. In this repository every reader is changed in the same pull request; outside it, semsource's tests read
  `entities` only (§ 4). Migration note required (owner rule 2026-11-02: a consumer break is a migration note).
- A gateway client (semconnect, semteams UI) selects `entities` only. If the gateway passed the page through, damaged
  entities would vanish from the UI with no error. Hence the gateway question in the design.
- The ideal is that an adopter has to know nothing new. That holds only if the in-repository readers that hide
  partial pages (graph-query counts, gateway) keep refusing. The design does that.
