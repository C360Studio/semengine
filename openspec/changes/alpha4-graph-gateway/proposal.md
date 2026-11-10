# Proposal: alpha4-graph-gateway

Approved-by:

Track: `design` (re-admits a package the ledger excludes; adds capability `graph-gateway`; modifies `graph-query`,
`graph-transport-boundary` and `composed-stack`).

## Why

The owner re-admitted `gateway/graph-gateway` as a trimmed Adapt for `v0.1.0-alpha.4` (#567, ruling 2026-11-10).
SemSource's UI and smoke test (semsource PR #259), SemTeams' UI (semteams #406) and SemConnect's UI read the graph
through its `/graph-gateway/graphql` route; without it SemSource cannot pin SemEngine, and each consumer would keep a
private copy.

## What Changes

- New `gateway/graph-gateway`, a read-only GraphQL endpoint with a declared schema: `entity`, `entitiesByPrefix`,
  `entitiesByPredicate`, `pathSearch`. Requests are parsed and validated (`github.com/vektah/gqlparser/v2`, MIT, and its
  `github.com/agnivade/levenshtein`, MIT: two new modules); the answer holds only the selected fields, and entity
  replies are checked as the exact reader checks them. The pin's substring router is not carried.
- `trajectory`: no SemEngine verb serves it (its pin responder is the excluded agentic loop). Recommended: an exported
  seam lets a composition root add a root field with its own schema and subject, declared as an output port; SemTeams
  adds `trajectory` with it. Owner question 1.
- graph-query forwards `predicate` to graph-index as `graph.query.predicate` (as it forwards `byName`), so the gateway
  reads one family, `graph.query.*`.
- Lifecycle on `component.Lifecycle`, requests admitted through `internal/httpadmission`; strict config.
- Not carried: agentic decode, `gateway` parent, `/mcp` stub, playground stub, standalone server, readiness surface,
  inference API, classifier, introspection, OpenAPI `init` registration, test hooks, the other 14 operations.
- The composed smoke stack gains graph-gateway and asks the four operations through the real HTTP port.
- Ledger row flips `defer-exclude` → `adapt`.

## Migration notes (consumer breaks, owner 2026-11-02)

- `entitiesByPredicate` needs a selection: `entitiesByPredicate(...) { entities }` (answer shape unchanged).
- `pathSearch` arguments are `startEntity`, `maxDepth`, `maxNodes`, `direction`, `predicates` (no `start`/`depth`);
  `entities` are `{ id depth }`; `paths[k]` leads to `entities[k]` as at the pin, each step `{ from predicate to }`
  naming the edge as the graph holds it; no `edges`, `type`, `score` or entity `triples`.
- `entitiesByPrefix` answers `{ entities { … } total next_cursor }`; SemConnect's selection of `id triples` directly
  on it (`ui/src/lib/services/graphGateway.ts:107-117`) moves under `entities`. `globalSearch` is not served.
- Not in the schema (can be added later without a break): entity `storage_ref`, `message_type`, `updated_at`; triple
  `context`, `datatype`, `expires_at`.
- Config keys left: `ports` (one `graph_queries` output) and `query_timeout_ms`. Refused at boot, naming the key:
  `graphql_path`, `mcp_path`, `bind_address`, `standalone_server`, `enable_playground`, `enable_inference_api`,
  `query_timeout`, `domain_examples_path`, `enable_embedding_classifier`, `readiness_keys`, `readiness_path`; the
  `graph_index_queries` and `agentic_queries` ports are refused.
- Introspection (`__schema`, `__type`) is refused; `__typename` answers.

## Capabilities

- Added: `graph-gateway`. Modified: `graph-query`, `graph-transport-boundary`, `composed-stack`.

## Impact

`gateway/graph-gateway` (new), `graph/query_verbs.go`, `processor/graph-query`, `processor/graph-index` (request type),
`internal/smoke`, `go.mod`, `docs/admission-ledger.yaml`, `.agents/skills/query-pattern/SKILL.md`. No overlap with #587.

## Rulings

Needed: owner question 1 (`owner-questions.md`). Covered: scope and re-admission (#567, 2026-11-10); consumer breaks as
migration notes (2026-11-02).
