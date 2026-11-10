# Inventory: `alpha4-graph-gateway`

Package: `gateway/graph-gateway` (ledger row `docs/admission-ledger.yaml:1929-1943`, `defer-exclude`).

base: 366f136cd3291bbfc3db0ee0766ef100f0cf9e77 on origin/main b7da56ff5c03973d84e3333fa38f88f317f936b0.
pin: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128, copy `/tmp/bufpin.zL1e` (local only). Every blob of
`gateway/graph-gateway/` and `docs/adr/` was **measured** equal to `gh api …/contents/<path>?ref=8b99efe9` SHAs.

Question: what must the trimmed gateway carry, change and drop to serve `entity`, `entitiesByPrefix`,
`entitiesByPredicate`, `pathSearch` and `trajectory` over SemEngine's verbs, with no `agentic`, `gateway` or
`agentic_queries` (owner ruling on #567, 2026-11-10)? Repositories (`docs/inventory-scope.md`): this tree, the pin,
semsource (`386c9ea`, `71fea1d8`), semteams (`4df44ae8`), semconnect (`d0d06e00`), all via `gh api`. Labels:
**measured** (command run), **read**.

## 0. Pin probe

**Measured** (go1.26.10): `go test ./gateway/graph-gateway/ -count=5 -shuffle=on -cpu=1` ok; `-count=1 -race` ok.
Lifecycle suite not run, as for the change-4/5/7 ports (`archive/2026-11-10-setup-04a-07-…/inventory.md:16-22`).
Test-text lines: `t.Skip` `component_test.go:298,372,394,411,433,768`; `time.Sleep` `component_test.go:307,779`,
`query_test.go:888`; `//go:build integration` `readiness_surface_integration_test.go:1`.
Size: production 2,676 (`component.go` 2,309, `openapi.go` 166, `readiness_surface.go` 116, `doc.go` 85); tests 5,315.

## 1. Shape sweep (pin `gateway/graph-gateway/`)

| # | Shape | Pin | Finding |
| --- | --- | --- | --- |
| S1 | second spelling | `component.go:1095-1211`, `:1213-1257`, `:1259-1308`, `:1832-1904` | one operation set spelled four times: substring router, field map, argument transforms, introspection schema; `:1130-1160` records the gh#206 collision |
| S2 | second spelling | `:1321-1364` | `start`/`startEntity`/`start_entity`, `depth`/`maxDepth`/`max_depth` synonyms |
| S3 | second spelling | `:139-154` vs `:156-162` | default ports and the "canonical contract" are two copies of one list |
| S4 | advertised-absent | `:2268-2284` | `/mcp` answers `{"message":"MCP endpoint"}` (`:2278`), no MCP |
| S5 | advertised-absent | `:2287-2309` | playground is a static page naming the path |
| S6 | back door | `:299-302` | `requestHook`, `prepareInference`, `beforeRuntimeCancel`, `beforeStartDoneClose` |
| S7 | one-impl interface | `:54-62` | `natsRequester` "mockable"; one production implementation |
| S8 | own lifecycle copy | `:284-298`, `:684-901` | hand-built guard; AGENTS § 1 requires `component.Lifecycle` |
| S9 | context retained | `:721` | `BaseContext: func(net.Listener) context.Context { return runCtx }` (standalone server) |
| S10 | init registration | `openapi.go:16-18` | `service.RegisterOpenAPISpec` in `init`, refused by AGENTS § 1 |
| S11 | selection ignored | `:1520-1545` | reply bytes written as `data.<field>`; selection set never read |
| S12 | error compare | `:2213` | `err == context.DeadlineExceeded` instead of `errors.Is` |
| S13 | copied shell | `:981-988`, `:995-1002` | the double-slash loop twice |
| S14 | defensive dead branch | `:1734`, `:2018-2023` | PubAck sniff; SemEngine refuses overlapping streams (`graph-transport-boundary` spec:366-377) |
| S15 | hand parser | `:1547-1732` | inline-argument parser outside any GraphQL grammar |

Out of scope by ruling: inference API (`:1054-1093`, `graph/inference`), classifier (`:470-551`, `graph/query`),
readiness surface (`readiness_surface.go:42-116`), standalone server (`:713-741`), the 14 other operations
(`:1832-1904`). No `NewGenericJSON`, `context.Background`/`TODO` in production (**measured**, grep).

## 2. The claimed gaps, on main

| Operation | Pin route | Main verb | Reply type on main |
| --- | --- | --- | --- |
| `entity` | `graph.query.entity` `:1206-1208` | `graph/query_verbs.go:95` forward `processor/graph-query/query.go:28` | `graph.ExactEntity` `{entity, kv_revision}` `graph/exact_entity.go:17-20` (pin `kvRevision`, pin `graph/exact_entity.go:20-23`) |
| `entitiesByPrefix` | `graph.query.prefix` `:1113-1115` | `query_verbs.go:99`, forward `query.go:30` | `PrefixQueryResponse` `graph/query_prefix_types.go:70-78`, same as pin |
| `entitiesByPredicate` | **`graph.index.query.predicate`** `:1189-1191` | **none on `graph.query.*`**; `query_verbs.go:77` (graph-index), handler `processor/graph-index/query.go:285` | enveloped `PredicateData{entities}` `graph/query_index_types.go:43-45` |
| `pathSearch` | `graph.query.pathSearch` `:1175-1177` | `query_verbs.go:107`, `query.go:53` | enveloped `PathSearchData{entities[{entity_id,depth}], edges, truncated}` `graph/query_path_types.go:43-47`; pin `{entities[{id,type,score}], paths[][{from,predicate,to}]}` (pin `processor/graph-query/pathrag.go:36-54`); pin `paths[k]` leads to `entities[k]`, `paths[0]` empty (`:139-140`) |
| `trajectory` | **`agentic.query.trajectory`** `:1102-1104` | **none**; `git grep -n -i trajectory` (no `.go` hit) | pin `agentic.TrajectoryPage` (pin `agentic/trajectory_query.go:36-44`), decoded at `:2092-2121` |

`trajectory` has no data source on SemEngine. Its responder is the pin's agentic loop, excluded here and absorbed by
SemTeams (semteams `openspec/changes/archive/2026-11-09-semengine-port-adaptation-design/design.md:10,35`).

Reply checks at the pin and on main: the pin validates prefix replies with `graph.ValidateDecodedEntityStates`
(`component.go:2064`) and answers 500 when they fail (`:2064-2067`); main does the same in
`pkg/fusion/fusionnats/client.go:135`; the exact reader refuses a missing entity or a zero `kv_revision` and decodes
with `graph.UnmarshalEntityState` (`graph/exact_entity.go:88-101`). The pin's no-responder answer is `query failed`,
naming no subject (`component.go:2229`).

## 3. Consumer evidence (read)

- semsource PR #259 merged 2026-11-10; ui-smoke-dev **failed** at `386c9ea` (run 42837254478: `Received: {"entities":
  [...]}` for `entitiesByPredicate`), **passed** at `71fea1d8` (run 42842351349) against the pin. At `71fea1d8`
  `test/ui/ui-profile.spec.cjs:124-275` pins: `entitiesByPredicate(predicate[,value])` with **no selection**, reading
  `.entities`; `entity{entity{id triples{predicate object}} kvRevision}` with `kvRevision` a Number;
  `entitiesByPrefix{entities{id triples} next_cursor}`; `pathSearch(start, depth, direction "OUTGOING", predicates)`
  reading `entities[].id` and `paths` steps `{from,predicate,to}`. `scripts/core-profile-smoke.sh:415-486` greps the
  same, declaring `$predicates: [String]` (`:474`); `:199-209` wants GET not 404 and <500.
- semsource `cmd/semsource/run.go:897-939` (`71fea1d8`) configures `ports` (3 outputs incl. `agentic_queries`),
  `bind_address`, `enable_playground`; Caddy rewrites `/graphql` to `/graph-gateway/graphql` (`Caddyfile:24-27`).
- semteams `ui/src/lib/services/graphApi.ts:174-198`
  `pathSearch(startEntity,maxDepth,maxNodes){entities{id triples{…}} edges{subject predicate object}}`, never served by
  the pin (no `edges`, no entity `triples`); `:217-230` `entity{entity{…} kvRevision}`; `:257-271`
  `entitiesByPrefix(prefix,limit)` (empty prefix, `DataView.svelte:85`); `agentApi.ts:41-61`
  `trajectory(loopId,limit,cursor)` selecting `TrajectoryPage` fields. `configs/flow-bootstrap.json` sets
  `graphql_path`, `mcp_path`, `enable_playground`, `query_timeout`.
- semconnect (first-wave, `docs/inventory-scope.md:16`) `ui/src/lib/services/graphGateway.ts`: `globalSearch(query,
  level, maxCommunities)` (`:49-51`, outside the ruled scope) and `entitiesByPrefix(prefix, limit) { id triples{subject
  predicate object} }` with no `entities` wrapper (`:107-117`), read as `BackendEntity[]`.
- Introspection (`__schema`): only `ui/src/lib/server/ai/toolExecutors.attack.test.ts` in semteams, a file #406 deletes
  (`gh api search/code`, semsource/semconnect/semboids empty).

## 4. HTTP mount surface on main

`service/service_manager_http.go:270-289` mounts any component with `RegisterHTTPHandlers(prefix, mux)` at
`/<instance>`, so `/graph-gateway/graphql` needs no `gateway` package. `/openapi.json` paths come only from services
(`:340-354`); a component spec adds schemas only (`:356-367`); `RegisterOpenAPISpec` is composition-root only
(`service/openapi_registry.go:11-13`). GraphQL library: none in `go.mod` (**measured**, grep `graphql|gqlparser`).
`gqlparser` v2.5.64 requires `github.com/agnivade/levenshtein` v1.2.2 (MIT; not in go.sum, **measured**) and
`go.yaml.in/yaml/v3` v3.0.5, already indirect (`go.mod:92`).

## 5. Open issues naming the package

`gh issue list --state open --search <q>` for `graph-gateway`, `graphgateway`, `gateway`, `graphql`: #567 (this change);
Issues #563, #350 and #9 name the word, not the package (#350's hit is semconnect `gateway/cs-api`).

## 6. Open PRs

`gh pr list --state open`: #588 (this claim), #587 `pkg/buffer` (files only in its own change directory and package).
No file or capability overlap; they merge in either order.

## 7. Pin decision records

Searched `graph-gateway|graphgateway|graphql` over the 85 verified pin ADRs: 045, 047, 049, 065, 090, 091, 102.
Governing: **ADR-090** decision 6 (remote reads use the GraphQL gateway's admitted operations; GS-12 makes it
conformant, `:47-51`); **ADR-091** §4 (exact read carries the KV revision, consumed through GraphQL, `:72-75`);
**ADR-060** (classified errors; gateways read `ce.Code`, `:183`) and lifecycle ADR-095/108/113 (in `docs/adr/`). Mention
only: 045, 047, 049, 065, 102.

## 8. Guidance

Pin `.agents/skills/query-pattern/SKILL.md:17,25,72` ("provisional until GS-12 makes it conformant GraphQL"; "not a
schema executor or selection-set projector"). SemEngine `.agents/skills/query-pattern/SKILL.md:139` says the gateway is
not in the tree.

## 9. Problem shape, collision table, intent

Shape: classified-refusal-preserving HTTP→NATS request adapter. Closest instance: `service/graph_triples_http.go:1-9`
(operator GET, reads KV directly, no classified mapping); graph-query's forward
(`processor/graph-query/query.go:27-36`).

| Dimension | Evidence |
| --- | --- |
| Semantic class | remote read of the graph over HTTP |
| Owners | graph-gateway (new); `service/graph_triples_http.go` (operator triples); semsparql `/sparql` (external, inventory-scope row) |
| Catalogs / status | ports judged by `composition.Analyze` (composition-validation spec :206-215, :311); health via `component.Lifecycle` (`component/lifecycle_type.go:14-27`) |
| Lifecycle / ownership | stateless; no KV, no stream, no lease; HTTP admission at stop is owned by `internal/httpadmission.Requests` (`admission.go:1-9,28-43`; used `metric/handler.go:143`, `service/service_manager_http.go:88,157`): 503 once stop begins, `Wait` for admitted requests |
| Readers / writers | semsource UI + smoke, semteams UI, semconnect UI; writes nothing |
| Recovery | none needed: nothing persisted |

Intent check: the gateway is a consumer adapter outside both halves; it is admitted by owner ruling (#567, 2026-11-10).
Query (the graph half) is admitted (epic #8). No capability changes membership.

## 10. Adopter seam inventory

SemSource developer, never opened this package: (1) must know that the instance is named `graph-gateway`, which
config keys remain, the query edits in § 3 and that graph-query/index/ingest are composed. (2) Do nothing: an old key
is refused at boot naming it; an old query gets 400 naming the field or argument; no graph-query gives a transient
error naming the subject. (3) All loud: boot or typed runtime error. (4) Should know: nothing past composing
graph-query; the query edits are a one-time migration. SemTeams developer: `trajectory` is refused with 400 naming it
(owner, 2026-11-10); SemTeams serves it from its own component. SemConnect
developer: the `entitiesByPrefix` selection moves under `entities`, and `globalSearch` is refused with 400 naming it
(loud).
