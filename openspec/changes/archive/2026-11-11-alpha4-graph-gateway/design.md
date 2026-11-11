# Design: alpha4-graph-gateway

## Context

Inventory: `inventory.md` (base 366f136, pin 8b99efe9). The pin gateway is 2,676 production lines and 5,315 test lines.
It imports `agentic`, `gateway`, `graph/inference`, `graph/query` and `graph/readiness`, routes by substring over the
lower-cased query text (`component.go:1095-1211`) and writes reply bytes as `data.<field>` whatever was selected
(`:1520-1545`). The pin's guidance names conformant GraphQL as the target (ADR-090 decision 6; pin `query-pattern` skill
:17,72). `entitiesByPredicate` is only on `graph.index.query.*`; `trajectory` has no verb (inventory § 2).

## Goals / Non-Goals

Goals: the four reads through one validated schema; read-only; failures loud;
~600 lines. Non-goals: the other 14 pin operations, MCP, introspection, auth (the mux owns middleware, ADR-030).

## Decisions

**D1. Routing: parse and validate, not substring.** (a) Keep the router: no new module, but four spellings (S1),
collisions (gh#206, `:1130-1160`), selections ignored, a hand parser (`:1547-1732`). (b) A hand root-field parser:
fixes routing only. (c) `github.com/vektah/gqlparser/v2` (MIT, v2.5.64, 2026-10-22; adds `agnivade/levenshtein`
v1.2.2, MIT; its yaml is already in go.sum) validates against SDL, coerces variables, resolves fragments; a small
executor projects one root field by its selection (aliases, fragments, `__typename`). (d) `graphql-go/graphql`: last
release 2023-04. (e) gqlgen: code generation for four fields. Pick (c); it removes S1, S2, S11, S15.

**D2. The schema is the contract, not the wire JSON.** Each resolver decodes the verb's typed reply by the table's
`Enveloped` flag, never by sniffing content (`graph/query_verbs.go:14-16`), checks it, then builds a value keyed by
GraphQL names. Root fields are nullable so a refused field is `data.<field>: null`, not `data: null`:

```graphql
scalar JSON
scalar Uint64
type Query {
  entity(id: String!): ExactEntity
  entitiesByPrefix(prefix: String!, limit: Int, cursor: String): EntityPage
  entitiesByPredicate(predicate: String!, value: String, limit: Int): PredicateMatches
  pathSearch(startEntity: String!, direction: String!, maxDepth: Int!, maxNodes: Int,
             predicates: [String]): PathSearchResult
}
type ExactEntity { entity: Entity!  kvRevision: Uint64! }
type Entity { id: String!  triples: [Triple!]! }
type Triple { subject: String! predicate: String! object: JSON source: String timestamp: String confidence: Float }
type EntityPage { entities: [Entity!]!  total: Int!  next_cursor: String }
type PredicateMatches { entities: [String!]! }
type PathSearchResult { entities: [PathEntity!]!  paths: [[Edge!]!]!  truncated: Boolean! }
type PathEntity { id: String!  depth: Int! }
type Edge { from: String!  predicate: String!  to: String! }
```

Checks: `entity` decodes the stored value with `graph.UnmarshalEntityState` and refuses a missing entity or a zero
`kv_revision`, the checks of `graph/exact_entity.go:88-101` (ADR-091 §4); `entitiesByPrefix` runs
`graph.ValidateDecodedEntityStates`, as the pin (`component.go:2064`) and `pkg/fusion/fusionnats/client.go:135` do. A
failed check is D6's last row. Pin names stay where consumers read them: `kvRevision`, `next_cursor`,
`paths{from predicate to}`. `paths` keeps the pin's alignment (pin `processor/graph-query/pathrag.go:139-140`):
`paths[k]` is the run of edges from the start to `entities[k]`, `paths[0]` empty, built by following `edges` back
(`graph/query_path_types.go:36-39`); each step names the edge as the graph holds it (graph-query spec :260-268).
`predicates` is `[String]` as at the pin (semsource sends `$predicates: [String]`); a null item is refused.
`direction` is case-insensitive and an empty `prefix` matches all, as at the pin (`:1342-1344`, `:2245-2247`).
Alternative, bytes passed through: breaks 2 of 4 consumer reads anyway and keeps the misreads of inventory § 3.

**D3. `entitiesByPredicate` via a graph-query forward.** Verb row `predicate` on `graph.query.*` (enveloped
`PredicateData`), forwarded to `graph.index.query.predicate` like `byName` (`processor/graph-query/query.go:31`);
`graph.PredicateRequest` replaces graph-index's inline struct (`processor/graph-index/query.go:288-293`), one spelling.
Alternative: a second gateway port on `graph.index.query.*`, as the pin; a second family, and it bends "over
`graph.query.*`".

**D4. `trajectory` is not served (owner, 2026-11-10, #567).** Its pin responder is the agentic loop SemTeams takes
over, and its answer is agent detail. SemTeams serves it from a component of its own: any registered component with
`RegisterHTTPHandlers(prefix, mux)` is mounted at `/<instance>` (`service/service_manager_http.go:268-289`), the way
this gateway is. SemTeams' UI points its one `trajectory` call at that route. Rejected: a hook adding consumer root
fields to this schema (new exported surface for one consumer), and a trajectory schema in the engine (agent meaning).

**D5. Component.** `component.Lifecycle`; no run context, no goroutine, no NATS state. Requests go through
`internal/httpadmission.Requests` (`admission.go:28-43`, as `metric/handler.go:143` and
`service/service_manager_http.go:88,157`): the route is wrapped with `Wrap`; cleanup calls `CloseAdmission` then
`Wait(ctx)`; before `Start` the handler refuses while the lifecycle reports `stopped`. No counter of its own. Config:
`ports` with exactly one `graph_queries` NATS-request output on `graph.query.*`, interface `graph.query v1`, checked as
graph-query checks its input (`processor/graph-query/config.go:30-64`), and `query_timeout_ms` (default 60000, the
pin's 60 s, `:133-135`). Unknown keys are refused, naming the key. Route `<prefix>/graphql`, mounted by the service
manager (`service/service_manager_http.go:270-289`). No OpenAPI spec: component paths never reach `/openapi.json`
(`:340-354`); the SDL is the contract.

**D6. Observable answers.** Pin mapping (`:2123-2234`) kept except where marked:

| Case | HTTP | Body |
| --- | --- | --- |
| not POST | 405 | `errors` |
| parse/validation error, mutation, two root fields, introspection | 400 | `errors` naming field or argument |
| classified refusal | 200 | `data.<field>: null`, `errors[0].extensions` `class`, `code` |
| no responder (changed: pin said `query failed`, `:2229`) | 500 | transient error naming the subject |
| deadline | 504 | `request timeout` |
| before `Start`, once `Stop` begins | 503 | `graph-gateway is stopping` |
| reply not fitting the schema or failing D2's checks (changed: pin 500, `:2064-2067`) | 200 | `data.<field>: null`, error naming the path |

`Stop` returns once no admitted request runs or its context ends.

## Shape sweep, ADRs, risks, migration

Required: S1/S2/S11/S15 (D1; fail-closed, "the design decides"), S6/S7 (back doors), S8/S9 (lifecycle, ADR-113), S10
(OpenAPI rule). Dropped with the surface: S3-S5, S13, S14. Fixed: S12. Pin tests not carried (dropped surface,
test-text rules). ADRs kept: 090 decision 6 (D1), 091 §4 (D2), 060 (D6), 095/108/113 (D5); 090 and 091 are not ported,
the spec carries the one decision of each. Risks: two new modules (govulncheck in `verify`); a ~150-line executor whose
rule (exactly the selected names) gets a `rapid` check (`go.mod:26`); one-time consumer query edits. `MIGRATION.md`
lists every break; task 3.1 tells semsource #251, semteams #406 and semconnect before the merge.

## Open Questions

None. Owner question 1 is answered in `owner-questions.md`.
