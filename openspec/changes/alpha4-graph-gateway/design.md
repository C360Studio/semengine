# Design: alpha4-graph-gateway

## Context

Inventory: `inventory.md` (base 366f136, pin 8b99efe9). The pin gateway is 2,676 production lines and 5,315 test lines.
It imports `agentic`, `gateway`, `graph/inference`, `graph/query` and `graph/readiness`. It routes by substring over the
lower-cased query text (`component.go:1095-1211`) and writes reply bytes as `data.<field>` whatever the client selected
(`:1520-1545`). The pin's own guidance names conformant GraphQL as the target (ADR-090 decision 6; pin `query-pattern`
skill :17,72). Four of the five operations have verbs on main. `entitiesByPredicate` is only on `graph.index.query.*`,
and `trajectory` has no verb at all (inventory § 2).

## Goals / Non-Goals

Goals: the four graph reads, plus `trajectory` per owner question 1, through one parsed and validated schema. Read-only.
Failures are loud. The port is about 600 production lines. Non-goals: the other 14 pin operations, MCP, introspection,
subscriptions, auth (the service manager's mux owns middleware, ADR-030), and any agentic type.

## Decisions

**D1. Routing: parse and validate (recommended), not substring.** (a) Keep the substring router. No new dependency, but
four spellings of one operation set (S1), known collisions (gh#206, `:1130-1160`), the selection set ignored, and a
hand-written argument parser (`:1547-1732`). (b) A hand-written root-field parser. It fixes routing only and still
ignores selections. (c) `gqlparser/v2` parses and validates the document against SDL text, coerces variables, and
resolves fragments. The gateway writes a small executor: one root field, then a projection of the answer by the
selection set, with aliases, fragments and `__typename`. One new module, MIT, maintained (v2.5.64, 2026-10-22); its
`yaml` dependency is already in go.sum. (d) `graphql-go/graphql`: last release 2023-04, and its types would leak into
any extension seam. (e) gqlgen: code generation, a large toolchain for four fields. Pick (c). It removes S1, S2, S11,
S15, the router and the pin's introspection code. Forecloses: nothing a consumer reads.

**D2. The schema is the contract, not the wire JSON.** Each resolver decodes the verb's typed reply. It follows the
`Enveloped` flag in the verb table and never sniffs the reply's content (`graph/query_verbs.go:14-16`). It then builds a
value keyed by GraphQL names:

```graphql
scalar JSON
scalar Uint64
type Query {
  entity(id: String!): ExactEntity
  entitiesByPrefix(prefix: String!, limit: Int, cursor: String): EntityPage!
  entitiesByPredicate(predicate: String!, value: String, limit: Int): PredicateMatches!
  pathSearch(startEntity: String!, direction: String!, maxDepth: Int!, maxNodes: Int,
             predicates: [String!]): PathSearchResult!
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

The pin's field names stay where consumers read them: `kvRevision`, `next_cursor`, `paths{from predicate to}`.
`paths[k]` is the run of edges from the start to `entities[k+1]`, found by following `edges` back
(`graph/query_path_types.go:36-39`). Each step names the edge as the graph holds it (graph-query spec :260-268), so
`from`/`to` follow the graph's direction, not the walk's. `direction` is case-insensitive, as at the pin (`:1342-1344`),
because SemSource sends `OUTGOING`. An empty `prefix` matches every entity, as at the pin (`:2245-2247`). Alternative:
pass bytes through as the pin did. Rejected: it breaks 2 of 4 consumer reads anyway (`kv_revision`, `edges`) and keeps
the silent misreads of inventory § 3.

**D3. `entitiesByPredicate` via a graph-query forward (recommended).** Add verb row `predicate` on `graph.query.*`
(enveloped `PredicateData`) and a forward to `graph.index.query.predicate`, the same as `byName`
(`processor/graph-query/query.go:31`). Add `graph.PredicateRequest`; graph-index decodes into it in place of its inline
struct (`processor/graph-index/query.go:288-293`). That leaves one spelling of the request. Alternative: a second
gateway port on `graph.index.query.*`, as the pin had. It costs a second family and a second port to validate, and it
bends the ruling's "over `graph.query.*`".

**D4. `trajectory` (owner question 1).** (a) Recommended: `RegisterWith(registry, ops ...Operation)`; `Register` is
`RegisterWith(registry)`. `Operation{SDL, Field, Subject}` adds one root field: SDL text declaring `extend type Query {
… }` and the types it returns. The request payload is the field's coerced arguments as a JSON object, and the reply is
projected by name. Registration refuses, naming the problem: an SDL that fails to parse with the base schema, a `Field`
it does not declare, a built-in field, a duplicate, or a `Subject` that is invalid, wildcarded, in `graph.QueryVerbs()`
or under `graph.mutation.`. SemTeams adds `trajectory` → `agentic.query.trajectory` with the pin's SDL (`:1906-1950`,
whose names are already the JSON keys). (b) SemEngine owns the trajectory SDL and forwards it. That puts agentic meaning
in the engine and keeps an `agentic.query` dependency. (c) Drop `trajectory`, against the ruling.

**D5. Component.** The component runs on `component.Lifecycle`, has no run context, starts no goroutine and holds no
NATS state. Config: `ports` with exactly one `graph_queries` NATS-request output on `graph.query.*`, interface
`graph.query v1`, checked the way graph-query checks its input (`processor/graph-query/config.go:30-64`). Also
`query_timeout_ms` (default 60000, the pin's 60 s, `:133-135`), the bound on each request. Unknown keys are refused,
naming the key. The route is `<prefix>/graphql`, mounted by the service manager
(`service/service_manager_http.go:270-289`). There is no OpenAPI spec: component paths never reach `/openapi.json`
(`:340-354`), and the SDL is the contract.

**D6. Observable answers** (pin mapping kept, `:2123-2234`). These are proven by tests named in the spec:

| Case | HTTP | Body |
| --- | --- | --- |
| not POST | 405 | `errors` |
| parse/validation error, mutation, more than one root field, introspection | 400 | `errors` naming the field or argument |
| classified refusal (not found, invalid) | 200 | `data.<field>: null`, `errors[0].extensions` `class`, `code` |
| no responder | 500 | error naming the subject, class `transient` |
| deadline | 504 | `request timeout` |
| before `Start`, after `Stop` | 503 | `service stopping` |
| reply not fitting the schema | 200 | `data.<field>: null`, error naming the path |

`Stop` returns once no request of the gateway is running or its context has ended. After `Stop`, every request gets 503.

## Shape sweep dispositions

Required: S1/S2/S11/S15 (D1; fail-closed rule, the owner's "the design decides"), S6/S7 (back-door rule), S8/S9
(lifecycle and context rules, ADR-113), S10 (AGENTS § 1 OpenAPI rule). Dropped with the trimmed surface: S3, S4, S5,
S13, S14. Fixed in passing: S12 (`errors.Is`). Pin tests are not carried: they cover dropped surface and fail the
test-text rules.

## ADRs

The design keeps each decision these ADRs make: ADR-090 decision 6 (D1 is GS-12's conformance), ADR-091 §4
(`kvRevision`), ADR-060 (D6 class and code), ADR-095/108/113 (D5). Do not port 090 or 091. Their other decisions govern
packages this change does not carry; the spec carries the one decision from each that applies.

## Risks / Trade-offs

- A new dependency to track (govulncheck runs in `verify`).
- The executor is about 150 lines we own. Its rule (an answer holds exactly the selected names) gets a generated check
  (`pgregory.net/rapid`, already in `go.mod:26`).
- SemSource and SemTeams make one-time query edits (proposal, Migration notes).

## Migration Plan

Per the proposal. `gateway/graph-gateway/MIGRATION.md` lists every break; semsource #251 and semteams #406 are told on
merge.

## Open Questions

1. **Owner:** the source of `trajectory` (D4), in `owner-questions.md`.
