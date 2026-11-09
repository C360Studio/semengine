# Inventory: change 5 PR D, `setup-04a-05-fusionnats`

Package: `pkg/fusion/fusionnats` (`Client`, the NATS `fusion.RetrievalClient`), and the no-embedder proof
(`docs/setup-plan.md:439-444`).

base: 468bac4f93fbaef511cf0b8d0a753d2dcb67fdc9 (claim commit on origin/main 7901bfda7c5893e69b70845e22131e54737a37f9)
pin: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128 (copy at `/tmp/c5pin/src`, local only; the six fusionnats files were
compared byte for byte with `gh api repos/C360Studio/semstreams/contents/pkg/fusion/fusionnats/<file>?ref=8b99efe9`:
all six identical)

Question: what must PR D carry, change and drop to port `fusionnats` onto this tree's `pkg/fusion` (PR C, #565) and
`graph` verb table, and how is a composition without graph-embedding proven? Repositories read
(`docs/inventory-scope.md`): this tree, the pin copy, semsource (symbol level, local checkout at `a084ab67`,
`git grep` only). Labels: **measured** (command run here), **read** (code read).

## 0. Pin probes (measured, on the pin copy, owner approval to run the pin's tests)

| Run | Command | Result |
| --- | --- | --- |
| race, once | `go test -count=1 -race ./pkg/fusion/fusionnats/` | ok, rc=0 |
| repeat | `go test -count=5 -cpu=1 -shuffle=on ./pkg/fusion/fusionnats/` | ok, rc=0 |
| integration | `go test -count=1 -tags integration -run Integration -v ./pkg/fusion/fusionnats/` (Docker) | 11 subtests PASS, rc=0 |

Logs: `/tmp/change5d-probe-repeat.log`, `/tmp/change5d-probe-integ.log` (local only). No lifecycle suite: the
package has no `Start`/`Stop` owner (`Client` has an optional `Close`, `client.go:89`). Test-text rules
(**measured**, `grep` over `*_test.go`): `time.Sleep` 0; `Skip` 0; build tags: `integration` only
(`client_integration_test.go:1`); `t.Parallel` 5, all in the unit file `client_test.go` (`:425`, `:442`, `:505`,
`:523`, `:645`), none in an integration file. The integration test uses `natsclient.NewTestClient`
(`client_integration_test.go:33`), which this tree does not carry (#195; owner ruling on #568) and fake handlers, not
the real responders (`:52-79`).

## 1. Sizes (measured, `wc -l`, pin)

| File | Prod | Test |
| --- | --- | --- |
| `client.go`, `doc.go` | 624 + 34 = 658 | |
| `client_test.go`, `client_integration_test.go`, `reconcile_test.go`, `slice_e_contract_test.go` | | 934 + 238 + 126 + 105 = 1,403 |

Of production, the readiness watch (`client.go:49-65` fields, `:84-247` `Close`, `statusWatcher`, `awaitFirstStatus`,
`Status`, `unknownCause`) is about 190 lines; of tests, the nine `TestStatus_*` tests (`client_test.go:193-378`,
`:839-934`) about 280. Pin imports (**measured**): `graph`, `graph/readiness`, `pkg/errs`, `pkg/fusion`, `pkg/types`;
tests add `message`, `natsclient`, testify. No `natsclient` import in production (`client.go:38-44`, a local
`requester` interface).

## 2. Category 1, claimed gaps

- "fusionnats is not in the tree": **measured** `ls pkg/fusion` lists no `fusionnats`; `pkg/fusion/README.md:95` and
  `pkg/fusion/MIGRATION.md:105` say so.
- "the batch read refuses the whole batch on one poisoned entity": **read**, true.
  `processor/graph-ingest/query.go:618` — `if len(poisoned) > 0 {` returns
  `aggregateEntityPoisonError(poisoned)` (`:619`), fatal `graph_state_reset_required` (`:646`); the handler passes it
  through `classifyEntityQueryError` (`:141`, `:462-469`); graph-query forwards a refusal unchanged
  (`processor/graph-query/query.go:154-156`). The IDs travel only in the message text (`:647-648`, at most
  `entityPoisonSampleCap`); `StateContractError.EntityID` is in-process only (`graph/state_contract.go:64-67`). This
  is a decision at the pin: ADR-079 D3, "aggregate reads failing loudly naming every poisoned entity" (pin
  `docs/adr/079-per-entity-poison-response-authoritative-surface.md:42-43`; not in this tree's `docs/adr`).
- "the single-entity read refuses only that entity": **read**, true. `processor/graph-ingest/query.go:96-97` returns
  the stored-value rule's own refusal for the one ID.
- "on the batch verb, `graph_state_reset_required` means poison and nothing else": **read**. In graph-ingest's batch
  path the code comes only from `StateContractError` (`query.go:463`, `:603-605`); graph-ingest has no latched
  reset gate on its query handlers (`git grep -n 'QueryReady' -- processor/graph-ingest`: 0 hits). The other producer
  of the code, `graph/follower/follower.go:391`, answers graph-index's and graph-embedding's verbs, not `batch`.
- "the prefix read has the same whole-page refusal": **read**, true. `processor/graph-ingest/query.go:335-337` uses
  `fetchEntitiesConcurrent` and `classifyEntityQueryError`; the prefix reply carries no per-ID missing list.
- "`Fuse` calls `Entities` with zero seeds": **read**, true. `pkg/fusion/engine_lens.go:136` calls
  `e.graph.Entities(ctx, SeedIDs(seeds))` for any seed count; pin `client.go:421` — `if len(ids) == 0 {` returns an
  empty `Hydration` with no request (`TestEntities_EmptyShortCircuits`, `client_test.go:628`, passed in § 0).
- "`graph.UnwrapQueryResponse` exists": **false** in the tree. The ledger's `graph` row lists it as removed
  (`docs/admission-ledger.yaml:2144`); `git grep -n 'func UnwrapQueryResponse'`: 0 hits.

## 3. Category 2, spellings of the modelled facts

- Subjects. Pin literals `client.go:26` — `subjectByName        = "graph.query.byName"` through `:31`
  (`subjectRelationships`). Tree home: `graph/query_verbs.go:95-120` (graph-query's rows `entity`, `batch`, `prefix`,
  `byName`, `relationships`, `semantic`); the rule: `openspec/specs/graph-transport-boundary/spec.md`, "Servers and
  clients take subjects from the declarations" and "A contract test refuses other spellings". Lookup today: private
  `subjectOf` in `processor/graph-query/query.go:79-86`; `graph/exact_entity.go:79` uses the unexported `entityVerb`.
  No exported lookup in `graph` (**measured** `git grep -n 'func QueryVerbs\|func .*Subject' -- graph/query_verbs.go`:
  `QueryVerbs` only).
- Reply shape. Pin `client.go:594` — `response, _ = graph.UnwrapQueryResponse(response)` (decides by content, error
  dropped). Tree: `graph/query_verbs.go:13-17` — "A caller decodes by verb; nothing decides a reply's shape from its
  content"; `QueryVerb.Enveloped`; envelope `graph/query_response.go:19`.
- Relationship row. Pin decode `client.go:508-512` reads `edge_type`; tree wire `graph/query_index_types.go:19-23`
  (`RelationshipEdge`, `json:"predicate"`), built at `processor/graph-query/query.go:292`, `:313`. **read**: on the
  tree's reply the pin's decode leaves every predicate empty, so a predicate filter (`:526`) drops every edge.
- Requests. Pin maps `client.go:375` — `map[string]string{"id": id}`, `:424` —
  `map[string]any{"ids": ids}`, `:316` — `body := map[string]any{"query": query, "limit": limit}`. Tree types:
  `graph.EntityRequest` (`graph/exact_entity.go:23`), `graph.EntityBatchRequest` (`graph/query_batch_types.go:44`),
  `graph.SemanticRequest` (`graph/query_search_types.go:33`, `Scope` omitempty); the spec: "Entity and batch
  requests SHALL be Go types declared in `graph`" (graph-transport-boundary, "The verb table declares each reply").
  Prefix already uses
  `graph.PrefixQueryRequest` (`:286`). byName and relationships requests have no `graph` type (graph-index decodes a
  local struct, `processor/graph-index/query.go:610-614`; graph-query `processor/graph-query/query.go:242-245`).
- Semantic reply. Pin `client.go:327-332` decodes `results` only; tree `graph.SemanticData`
  (`graph/query_search_types.go:55-60`) carries `unreadable_records`, `failed_records`, which
  `fusion.Resolution` holds (`pkg/fusion/retrieval.go`, `Resolution`).
- Default timeout. Pin `client.go:36` — `const defaultTimeout = 5 * time.Second`; tree
  `natsclient/request.go:18` — `const DefaultRequestTimeout = 5 * time.Second`; `graph.NewExactEntityReader` passes a
  zero timeout through to it and refuses a negative one (`graph/exact_entity.go:46-59`).
- Entity decode. Pin `client.go:382-397` (`json.Unmarshal`, then `ValidateDecodedEntityState`, ID check); tree
  `graph.UnmarshalEntityState` (`graph/entity_predicate_contract.go:288`) is "the one decoder ... for every reader of
  one entity's reply", used by `ExactEntityReader` (`graph/exact_entity.go:100`).
- Readiness. Pin `client.go:201` — `func (c *Client) Status(ctx context.Context) (fusion.IndexStatus, error) {`; tree
  `fusion.RetrievalClient` has no `Status` (`pkg/fusion/retrieval.go:15-45`; PR C design D1).
- Hydration reconciliation: pin `client.go:459` (`reconcileHydration`); no other home in the tree (**measured**
  `git grep -n 'reconcile' -- pkg graph processor | grep -v _test`: graph-index's unrelated replacement reconcile only).

## 4. Category 3, adjacent claims

- Open PRs (**measured**, `gh pr list --state open`): only #577 (this claim, draft, 0 files).
- Rulings: split row D (`openspec/changes/archive/2026-11-08-setup-04a-05-first-consumer-graph-embedding/split.md:12`,
  accepted PR #550 comment 6716972354; ruling 4 there, readiness from the reply); tag after PR D (#9 comment
  6727586862); consumer fixtures are consumer-owned (#568); consumer breaks are migration notes (2026-11-02, PR #504).
- Ledger: `graph/readiness` known_risks (`docs/admission-ledger.yaml:2195-2198`, "Reading.Raw has no reader ...
  whether fusionnats still watches ... is PR D's question"); `pkg/fusion` known_risks (`:4627`, "fusionnats, PR D,
  decides the batch handling"); `graph` (`:2142-2144`).
- Specs: `fusion` (`openspec/specs/fusion/spec.md:22-37`, a fatal seed read fails the call; `:39-48`, all unhydrated
  defers); `composed-stack` (`openspec/specs/composed-stack/spec.md:11-28`, the stack includes graph-embedding);
  `graph-transport-boundary` (subjects).
- Stale text naming fusionnats (**measured**, `git grep -n fusionnats -- '*.go' '*.md' '*.yaml'
  ':!openspec/changes/archive'`): ledger `docs/admission-ledger.yaml:2198`, `:4592`, `:4600`, `:4627`, `:4631`,
  `:4694`; ADR-062 port note `docs/adr/062-deterministic-graph-fusion.md:17`; `fusion` spec
  `openspec/specs/fusion/spec.md:22-31` (a reset as the fatal seed-read example);
  `graph/readiness/watcher.go:22-25`, `pkg/fusion/README.md:95`, `pkg/fusion/MIGRATION.md:105-106`,
  `pkg/fusion/retrieval.go:37`, `processor/graph-ingest/query.go:481`, `pkg/fusion/retrieval.go:109-110`
  (`UnhydratedError` "Reserved while the handler's first-error contract stands"), port notes ADR-071 `:9-14`
  ("The fusion wiring that would send a scope is not present"), ADR-084 `:16-17`.
- Pin ADR-082 is not in the tree (ADR-084 port note `:13-14`).

## 5. Category 4, who it is for

Ported framework API with consumer callers (semsource, **measured** `git grep -n 'fusionnats\.'`): `fusionnats.New`
(`semsource:processor/code-context/component.go:136` — `graph = fusionnats.New(deps.NATSClient, 0)`, and 11
integration tests under `internal/governance`, `processor/code-context`); `*fusionnats.Client.Neighbors`
(`internal/governance/fusion_gateway_integration_test.go:304`); `Client.Status`
(`fusion_gateway_integration_test.go:80`, test only). No semsource call of `Close` (**measured**, same grep: 0).
New surface: none planned beyond changed signatures (§ 9 of the design lists them).

## 6. Category 5, problem shapes

- Classified refusal read from the reply: `isNotFound` (pin `client.go:621-624`, `errors.As` + `Code`); tree
  `graph.IsStateContractError` (`graph/state_contract.go:18-28`) reads the reset code the same way. Adopted.
- Scope a whole-batch refusal down to the refused items, without parsing text: **measured** no instance
  (`git grep -n -i 'one at a time\|each alone\|fall back' -- '*.go'`: none on entity reads). The tree's only
  production caller of the batch verb is fusionnats (**measured** `git grep -n 'EntityBatchRequest{'`: 0 production
  hits). Not a reusable primitive (one caller), so no adoption sweep.
- One-entity read: `graph.ExactEntityReader` (`graph/exact_entity.go:64-103`) owns the exact read and its decode.
  Not adopted as the client: its subject is fixed to `graph.ingest.query.entity` (`:79`), and ADR-062 routes
  fusionnats through the public `graph.query.*` verbs (`doc.go:12-21`); its decoder, `graph.UnmarshalEntityState`, is
  adopted (N10).
- Constructor that refuses bad wiring, zero timeout meaning the transport's default: `graph.NewExactEntityReader`
  (`graph/exact_entity.go:46-59`). Adopted.
- Subjects from the table with a failure naming a missing row: `processor/graph-query/query.go:79-95`
  (`subjectOf`, `handlerFor`). Adopted in shape.
- Composed stack without one component: `internal/smoke/stack_integration_test.go:66-89` (`stackConfig`,
  `stackComponents`), `:225-235` (registration list); `processor/graph-query/composed_integration_test.go:66-107`
  wires graph-ingest and graph-index by hand.
- Import-closure guard: `internal/harness/contract/embeddingprovider_test.go:20-24`
  (`TestEmbeddingImportsNoProviderModule`). Adopted for the no-embedder import check.
- A verb with no responder: graph-query refuses transient `internal` naming the subject
  (`processor/graph-query/query.go:158-159`; `doc.go:9-12`). Fusion returns any non-`index_not_ready` resolve error
  (`pkg/fusion/engine_lens.go:118-119`, `openspec/specs/fusion/spec.md:10-14`).

## 7. Category 6, pin ADRs

**Measured**: `grep -l -e fusionnats -e RetrievalClient -e reconcileHydration -e graph.query.batch` over the pin's
`docs/adr` (85 files): 062, 066, 071, 084; then titles read: 060 (classified RPC errors), 079 (poison scope), 083
(readiness as KV state, the watch fusionnats binds, `client.go:23-27`), 109 (SemEngine).

| ADR | Decides, for fusionnats | In tree |
| --- | --- | --- |
| 060 unified RPC error contract | `RequestClassified`; `entity_not_found` is the one code read as absence (`client.go:29-33`, `:370-373`) | yes |
| 062 deterministic graph fusion | the production `RetrievalClient` over public `graph.query.*` (increment B2, `doc.go:1-4`) | yes |
| 066 honest readiness | item 3 (pin `:413`): fusionnats decodes every envelope field | yes (fusion part not carried, port note) |
| 071 domain-scoped NL retrieval | `resolveSemantic` sends `scope` only when non-empty (pin `:82`) | yes |
| 079 per-entity poison | D3: the read surface refuses exactly the poisoned entities; aggregate reads fail naming them (pin `:38-46`) | no (cited by ADR-109) |
| 082 bounded-lag readiness | exact consumers, "fusion honesty envelope" among them, gate on `Ready` (pin `:65`); changed in PR C under ADR-109 D1, D3 (`split.md:22-23`); nothing left for PR D | no |
| 083 readiness as distributed state | consumers watch `GRAPH_STATUS`; fusionnats' lazy watch (`client.go:107-147`) | yes |
| 084 readiness licenses health | D6 / gh#597: `Entities` reconciles against the requested set (pin `:63-65`; `client.go:401-419`) | yes |
| 109 (SemEngine) | D1 readiness rides the reply, poison refuses only the poisoned entities (`:77`); D3 no consumer reads a sign first | yes |
| 112, 113 (SemEngine) | natsclient's timeout default; no root context outside a `Start` | yes |

Guidance: SemStreams' NATS RPC section is in the tree's contracts (`AGENTS.md` § 2, classified RPC row). Coverage:
`pkg/fusion/fusionnats` is on the 03B critical list
(`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:459`); not yet in `scripts/cover-check.sh`
(`:61-63` has `pkg/fusion unit` only).

## 8. Shape sweep (read on the pin copy, `pin:pkg/fusion/fusionnats/`)

| # | Shape | Pin | Disposition |
| --- | --- | --- | --- |
| N1 | second spelling | `client.go:26` — `subjectByName        = "graph.query.byName"` (and `:27-31`) | required (graph-transport-boundary; its contract test fails on the literal) |
| N2 | shape by content, error dropped | `client.go:594` — `response, _ = graph.UnwrapQueryResponse(response)` | required (`QueryVerb.Enveloped`, `graph/query_verbs.go:13-17`; function removed, ledger `:2144`) |
| N3 | wire drift | `client.go:511` — ``EdgeType     string `json:"edge_type"` `` | required (build need: tree wire is `predicate`) |
| N4 | removed read | `client.go:201` — `func (c *Client) Status(ctx context.Context) (fusion.IndexStatus, error) {` with `:49-65`, `:89-247` | required (ruling 4; ADR-109 D1, D3; no `Status` on the SPI) |
| N5 | detached continuing work | `client.go:140` — `if err := watch.Start(context.WithoutCancel(ctx)); err != nil {` | required, goes with N4 (ADR-113; architect contract, "Detachment is terminal and bounded") |
| N6 | dropped counts | `client.go:327` — `var resp struct {` (results only) | required (`fusion.Resolution`, PR C D3) |
| N7 | second spelling | `client.go:375` — `map[string]string{"id": id}`; `:424` — `map[string]any{"ids": ids}` | required (graph-transport-boundary, request types declared in `graph`) |
| N8 | second spelling | `client.go:316` — `body := map[string]any{"query": query, "limit": limit}` | optional; adopted (`graph.SemanticRequest` exists, `Scope` omitempty keeps ADR-071's rule) |
| N9 | second spelling, nil default | `client.go:36` — `const defaultTimeout = 5 * time.Second`; `:77` — `func New(nats requester, timeout time.Duration) *Client {` accepts nil | required for the nil requester (AGENTS.md § 1, no nil defaults); the timeout adopts `NewExactEntityReader`'s rule |
| N10 | second decoder | `client.go:382` — `var exact graph.ExactEntity` then `:392` validate | optional; adopted (`graph.UnmarshalEntityState`, one decoder for one entity's reply) |
| N11 | whole-batch refusal surfaces as a failed answer | `client.go:424-426` returns the batch error | required (ADR-109 D1 `:77`; PR C design D1 hands it to PR D) |
| N12 | one implementation | `client.go:42` — `type requester interface {` | retained: keeps `natsclient` out of production imports and lets unit tests fake the wire |
| N13 | duplicate client-side check | `client.go:357` — `func validateScope(scope []string) error {` (responder also refuses) | optional; kept (fails before the wire, pin tests `:504`, `:522`) |

Shape 5 (error text): **measured** `grep -n 'err.Error()\|strings.Contains' client.go`: 0. Context:
**measured** `grep -n 'context.Background\|context.TODO\|WithoutCancel\|go func' client.go`: `:140` only (N5). No
goroutine besides the watch's (`readiness.Watcher.Start`).

## 9. Edge values (read, against the tree's responders)

| Input | Pin `fusionnats` | Tree responder |
| --- | --- | --- |
| zero seeds | no request, empty `Hydration` (`client.go:421-423`) | batch answers `{}` for no IDs (`processor/graph-ingest/query.go:119-121`) |
| duplicate seeds | one entity, reported once (`client.go:476-480`; `TestReconcileHydration`) | batch reads each listed ID |
| one seed poisoned | whole call fails (N11) | fatal `graph_state_reset_required` for the batch (`query.go:618-619`) |
| all seeds poisoned | same | same |
| empty graph, symbol | byName answers no matches; zero seeds; engine miss | `processor/graph-index/query.go:606` ("An unknown name is an empty answer") |
| empty query, symbol | byName refused `invalid_request` | `processor/graph-index/query.go:618-620` |
| NL with no graph-embedding | error returned | graph-query: transient `internal`, "request to graph.embedding.query.search" (`query.go:158-159`) |

## 10. Open issues naming the package

**Measured**, `gh issue list --state open --search <term> --limit 200` for `fusionnats`, `fusion`,
`RetrievalClient`, `embedder`, `no-embedder`, `readiness`, `Watcher`, `poisoned`, `poison batch`, `no responders`.
Kept: #195 (natsfixture adoption; lists `pkg/fusion/fusionnats` 1 site at change 5); #110 (names the pkg/fusion copy;
item 2 done in PR C, item 3 is temporal's). Names the word, not the package: #9, #109, #32, #155, #489, #11,
issues #571, #551, #108, #502, #16 and #300. Also read: #563 (searchGraph fallback, graph-query; fusionnats calls no
`searchGraph`).

## 11. Adopter seam

Who: SemSource's code-context author (`processor/code-context/component.go:136`) and its integration-test authors.

1. Must know: `New` returns an error; there is no `Status` and no `Close`; a poisoned seed is now listed as unhydrated
   instead of failing the answer.
2. Do nothing: compile errors at `component.go:136` and the 11 test sites; `gc.Status(ctx)` at
   `fusion_gateway_integration_test.go:80` fails to compile.
3. Find out: compile errors; `pkg/fusion/MIGRATION.md`.
4. Should know: nothing more. Readiness is observed from each reply (prefer observation to prediction): the pin made
   the caller depend on a watched key a deployment without graph-index never writes (`client.go:149-169`).

Collision table: not triggered (the change removes a watch and adds no durable, communication or coordination
primitive). Intent check: not triggered (no boundary moves; graph-embedding stays admitted, the proof only composes
without it).
