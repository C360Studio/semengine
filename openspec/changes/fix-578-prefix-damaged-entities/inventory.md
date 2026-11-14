# Inventory: fix-578-prefix-damaged-entities

base: 88c7deec08652edc26d6dfa8186b8044243ea40a (main 80525179 + claim and docs commits; no code change)

Question, after the owner's rulings on #657 (comment 6877432505): what does a listing of the entity IDs under a
prefix need, so fusion's prefix search can read around a damaged entity while the full-entity prefix page keeps
refusing? Repositories read: this one only. Consumers reach the prefix search through `pkg/fusion`, not the verbs
(earlier inventory § 4). No pin package is carried, so § 6 is one line.

## 1. The claimed gap

- No verb lists IDs only. `git grep -n -i 'prefixIDs\|PrefixIDs\|ids_only\|IDsOnly\|KeysOnly' -- '*.go'`: no hit.
  The only `json:"ids"` is the batch request (`graph/query_batch_types.go:45` — ``IDs []string `json:"ids"` ``).
  Key listing exists below the verbs only: `natsclient/kv.go:595` — `func (kv *KVStore) KeysByPrefix(`.
- The prefix verb decodes every key on the page and refuses the page if one is damaged:
  `processor/graph-ingest/query.go:330` — `entities, _, err := c.fetchEntitiesConcurrent(ctx, pageKeys, defaultMaxConcurrent)`;
  `query.go:598` — `if errors.As(r.err, &contractErr) {`; damage wins over other errors at `query.go:613` —
  `if len(poisoned) > 0 {` (before `:616` — `if firstOtherErr != nil {`); aggregate at `query.go:629` —
  `func aggregateEntityPoisonError(`.
- Everything a listing needs is done before that decode: request decode `query.go:237` —
  `if err := json.Unmarshal(data, &req); err != nil {`; key listing `:262-292`; sort and compact `:299-300`;
  total `query.go:302` — `total := len(keys)`; cursor `:305-317`; limit `query.go:320` — `pageKeys := keys`.
  One stored value is read before `:330`: `query.go:286` —
  `if _, getErr := c.entityBucket.Get(ctx, req.Prefix); getErr == nil {` (exact six-part prefix; existence only, no
  decode).
- The damaged-entity record and health come from the decode only: `processor/graph-ingest/mutation_runtime.go:159` —
  `c.inventoryEntityPoison(ctx, contractErr, entry.Revision)`; spec `openspec/specs/graph-ingest-recovery/spec.md:225`.
- #578's text says a caller "cannot tell which entity is damaged". The refusal names the IDs in its text
  (`query.go:629-643`), but not as structured detail (unclaimed item on #657). It does not list the other keys.

## 2. Every spelling of the facts modelled

"The keys under a prefix, in order, paged":

- graph-ingest's prefix verb, above (`query.go:226-350`), and its fit and cursor `query.go:359` —
  `func marshalFittingPrefixPage(`; cursor codec `query.go:649` — `func encodeCursor(key string) string {`, `:655`.
- graph-query reads only IDs from the full page: `processor/graph-query/counts.go:32-75` (`readCountPage`);
  spec `openspec/specs/graph-query/spec.md:333` — "`hierarchyStats` and `summary` SHALL read one prefix page, IDs only".
- graph-index lists its own buckets by prefix (`processor/graph-index/query.go:176`, `:260`, `:624`), but not entities.

"A damaged entity on a read": unchanged by this change. The aggregate is at `query.go:629`, the record at
`mutation_runtime.go:159`, and fusion's per-seed mapping at `pkg/fusion/fusionnats/client.go:265-270` and `:292-334`.

## 3. Adjacent claims

- Owner rulings: #657 comment 6877432505 (ruling 4: "The prefix fix is a listing of IDs only; the full-entity prefix
  page keeps refusing"). Design session recommendation: PR #655 comment 6877450943 (premises re-checked in design.md).
- Related, not in this change: #656 (batch names damaged entities as `damaged`; fusion's re-read goes with it), #658,
  and the unclaimed items on #657.
- Verb table and its readers (`git grep -n 'QueryVerbs()'`):
  - The table: `graph/query_verbs.go:70` — `func QueryVerbs() []QueryVerb {`. Its prefix rows are at `:75` and `:107`.
  - Decode by verb: `graph/query_verbs.go:15` — "A caller decodes by verb; nothing decides a reply's shape from its".
  - Spec, the same rule: `openspec/specs/graph-transport-boundary/spec.md:433-437` ("A caller SHALL decode by verb").
  - Tests that list rows and fail when a row is added:
    - `graph/query_verbs_test.go:22` — `want := []QueryVerb{`
    - `processor/graph-ingest/query_verbs_integration_test.go:236` — `requests := map[string]any{`
    - `processor/graph-query/query_integration_test.go:43`, `:576` — `want := []string{"entity", "batch", "prefix", ...`
    - `processor/graph-query/revision_integration_test.go:230` (the requests map, refusing unknown fields)
  - Tests that read rows by name and need no change:
    - `gateway/graph-gateway/gateway_integration_test.go:96`
    - `internal/smoke/smoke_integration_test.go:158`
    - `pkg/fusion/fusionnats/client_test.go:174`
    - `processor/graph-query/lifecycle_test.go:146`, `:204`
    - `processor/graph-ingest/failed_start_callbacks_integration_test.go:60`
  - Contract guard: `internal/harness/contract/reservedsubjects_test.go:32` — `for _, verb := range graph.QueryVerbs() {`
    (reads the table, matches by `strings.Contains` at `:150`).
  - Composition: `composition/analyze.go:132`, `:253`. Generated check `composition/reserved_test.go:139`:
    `var enumerationTokens = []string{"graph", "ingest", "query", ... "prefix", ...}`, and its comment `:134-135`
    claims "every token of graph-ingest's ... reserved subjects".
- Who answers each verb:
  - graph-ingest: `processor/graph-ingest/query.go:29-35` (`queryHandlers`), registered from the table at `:45-58`.
    The not-started gate covers every subscription (`processor/graph-ingest/component.go:506-510`).
  - graph-query: `processor/graph-query/query.go:27-37` — `forwards := map[string]struct{ responder, name string }{`;
    the prefix forward is at `:30`, and the relay itself at `:137-141` (`forward`).
  - graph-gateway maps root fields by name: `gateway/graph-gateway/resolve.go:17-22`. No change.
- Operator and OpenAPI surfaces: `git grep -ln 'ResponseTypes\|OpenAPI' -- '*.go'` finds only `service/`. None of
  those files names a graph verb (`git grep -n 'QueryVerb\|ingest' -- service/openapi*.go service/*_http.go`: none).
- Specs that state the verbs:
  - graph-transport-boundary:
    - `:13-47` (every verb in the table)
    - `:254-264` (the verbs whose refusals carry a code)
    - `:152-208` (prefix request rules)
    - `:468-483` (prefix total)
  - graph-query `:21-30` (the forwards).
  - graph-entity-writes `:393-396`: names the prefix verb as a decoder; the new verb decodes nothing.
  - fusion:
    - `:22-32`: "A fatal seed read fails the call" names "`graph_state_reset_required` from a prefix resolve".
    - `:262-275`: subjects come from the table.
    - `:315-330`: a poisoned seed is unhydrated.
- Fusion:
  - Prefix resolve: `pkg/fusion/fusionnats/client.go:123` — `func (c *Client) resolvePrefix(`; seeds at `:140`.
  - Rows the client looks up: `:25` and `:39` — `{"prefix", &v.prefix},`.
  - ID check used by the semantic resolve: `:167` — `graph.ValidateDecodedEntityIDs(ids)`.
  - Seed limit: `pkg/fusion/engine_lens.go:31` — `resolveLimit        = 40`.
  - Hydration: `pkg/fusion/engine_lens.go:135` — `hydration, err := e.graph.Entities(ctx, SeedIDs(seeds))`.
  - Fusion unit tests that fake the prefix reply:
    - `pkg/fusion/fusionnats/client_test.go:25`, `:102`, `:110`, `:125`, `:148`, `:242`, `:265`, `:367`, `:444`
    - `pkg/fusion/fusionnats/poison_test.go:52`, `:159`
  - Real-responder model: `pkg/fusion/fusionnats/client_integration_test.go:372` —
    `func TestPoisonedSeedOverRealResponders(t *testing.T) {` (its `poison` is under the `alpha` prefix, `:59-63`).
- The batch reply has no byte fit: `processor/graph-ingest/query.go:150` — `return json.Marshal(resp)`. A reply that
  is too large is refused `response_too_large` at `natsclient/request.go:390` —
  `if errors.Is(respErr, nats.ErrMaxPayload) {`.
- Tests that pin the whole-page refusal stay unchanged: `processor/graph-ingest/poison_scoping_test.go:296-302`,
  `:432-434`.
- Docs that state the prefix gap:
  - `pkg/fusion/README.md:120-121`
  - `pkg/fusion/MIGRATION.md:89-90`, `:133`
  - Verb lists: `pkg/fusion/README.md:101-109`, `pkg/fusion/fusionnats/doc.go:11-19`, `graph/README.md:48-55`,
    `processor/graph-ingest/README.md:108-114`, `processor/graph-query/README.md:26`, `processor/graph-query/doc.go:9`.
- Open pull requests (`gh pr list --state open`, 2026-11-14):
  - #655: this claim.
  - #654: draft, `setup-04b-03` graph-query lexical, 13 files, all under its change directory today. Its
    graph-query delta MODIFIES and REMOVES requirements other than "Authority and name replies are forwarded
    unchanged". Its code will edit `processor/graph-query/query.go` wiring and tests.
  - #597, #598: `go.mod` only, no overlap.
- Open issues (`gh issue list --state open --search prefix`, `--search fusionnats`):
  - #578: this change.
  - #656, #657, #658: related; they stay open.
  - #300 item D: `marshalFittingPrefixPage`'s double marshal; optional, stays.
  - #506, #624, #621, #570, #195, #110, #11, #9, #10, #60, #68: they use the word, not this surface.
  - `--search response_too_large`: no issue on the batch reply's size.

## 4. Who it is for

- Rows `prefixIDs` on graph-ingest and on graph-query. The graph-ingest row is plumbing for graph-query's forward
  (`processor/graph-query/query.go:27-37`, the row added there). The graph-query row is plumbing for
  `pkg/fusion/fusionnats/client.go:123` (`resolvePrefix`). It is also framework API for products listing IDs under
  a prefix: the contract line is the graph-transport-boundary requirement this change adds, and the test is
  `TestDamagedEntityUnderPrefixOverRealResponders`.
- Type `graph.PrefixIDsResponse`: the same callers.

## 5. Problem shape

The shape is "list what exists without reading it, then read it with per-item accounting". The closest instance is
fusion's resolve-then-hydrate: `resolveSemantic` lists IDs (`client.go:150-176`) and `Entities` reads them through
batch with per-ID accounting (`client.go:258-282`). The `nl` mode already works this way. This change makes the prefix
mode match it.

## 6. Pin decision records

No package is carried from the pin. The decisions in force are ADR-109 D1 (`docs/adr/109-ready-after-boot.md:77`) and
the owner rulings on #657.

## Collision table (new request subject)

| Dimension | Evidence |
| --- | --- |
| Semantic class | The keys under a prefix, sorted and paged. Owners: graph-ingest's prefix verb (`query.go:226-350`). Partial owner: graph-query's ID-only read of that page (`counts.go:32-75`). |
| Catalogs | The verb table `graph/query_verbs.go:70`; composition's reserved subjects `composition/analyze.go:130-136`. |
| Status | None. Neither verb sets health. The prefix verb only through the decode record (`mutation_runtime.go:159`). |
| Lifecycle | Subscribed at Start from the table (`processor/graph-ingest/query.go:45-58`; `processor/graph-query/query.go:111-135`); gated until Start completes (`component.go:506-510`). |
| Ownership | One responder per verb: composition refuses a second (`composition/analyze.go:246-265`). |
| Readers | fusionnats (this change). graph-query counts stay on `prefix` (ruling 1). |
| Writers | None. The listing reads keys only. |
| Recovery | None. Nothing is stored. |

## Adopter seams

- A fusion user (a product with a prefix lens) has to know nothing new. Before: one damaged entity under the prefix
  fails `Fuse` with fatal `graph_state_reset_required`. After: the answer stands, and the damaged entity is in
  `unhydrated` with reason `error`. Two side effects show up in the reply:
  - A key deleted between the listing and the read is listed `not_found`. Before, it silently dropped out.
  - The prefix mode now sends up to 40 seeds, whatever the entity size, as `nl` and symbol do. If those 40
    entities exceed the server's maximum payload, the batch is refused `response_too_large` and the call fails
    (§ 3, batch reply). Before, the prefix page's own byte fit held fewer seeds. The caller finds out from a typed
    runtime error.
- A direct caller of `prefixIDs` must know two things: a listed ID may be damaged (reading it tells you), and a
  cursor goes back to the verb that gave it. A caller who knows neither still gets an honest listing.
- Mixed versions: a graph-query without the row has no responder on `graph.query.prefixIDs`, so fusion's prefix
  resolve fails with a transient "no responders" error. That is loud. A flag would have been silently ignored
  (§ 3, `query.go:237`).
- A direct caller of `graph.query.prefix` and the gateway's `entitiesByPrefix`: nothing changes.
