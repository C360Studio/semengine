# Consumer migration: `gateway/graph-gateway`

For developers of projects that read the graph through SemStreams' graph-gateway and move to SemEngine's. It says what
you change at your cutover. The port breaks old shapes where that gives a better result (owner, 2026-11-02). Nothing
here was run against your code: it comes from reading SemSource, SemTeams and SemConnect as inventoried for this
change.

The biggest change is how a request is read. SemStreams' gateway picked an operation by searching the query text for
a field name and sent back the reply it got, whatever the request selected. SemEngine's parses each request and checks
it against a declared schema (`schema.graphql`, listed in `README.md`) before it asks the graph anything, then answers
with exactly the fields you selected. A field or argument the schema does not declare is refused with HTTP 400, naming
it. A query that worked because nothing checked it fails now, loudly. The route is the same:
`POST /graph-gateway/graphql` for a component named `graph-gateway`.

## Query changes

- **`entitiesByPredicate` needs a selection.** Write `entitiesByPredicate(predicate: $p, value: $v) { entities }`. The
  answer is the same shape as before, a list of entity IDs under `entities`; a query with no selection is refused.
  SemSource's UI test and smoke script send it with none.
- **`pathSearch` takes new argument names and answers a narrower shape.**
  - Arguments are `startEntity`, `direction`, `maxDepth`, `maxNodes` and `predicates`. `start` and `depth` are gone.
    `startEntity`, `direction` and `maxDepth` are required. `direction` is `outgoing`, `incoming` or `both`, in any
    letter case. `maxDepth` is 1 to 10. `maxNodes` is at most 500; left out, null or 0 means 100; a negative value or
    one over 500 is refused, not clamped. `predicates` is `[String]`, as SemSource declares it; a null item is refused.
  - `entities` are `{ id depth }`, with the start first at depth 0. `paths[k]` is the run of edges from the start to
    `entities[k]`, as before, so `paths[0]` is `[]`. Each step is `{ from predicate to }` naming the edge as the graph
    holds it, whichever way the walk crossed it.
  - There is no `edges`, `type`, `score` or entity `triples` on the answer. For an entity's triples, ask `entity` or
    `entitiesByPrefix` afterwards.
  - SemSource: rename `start` to `startEntity` and `depth` to `maxDepth`. SemTeams: its query selects
    `entities { id triples }` and `edges { subject predicate object }`, which SemStreams never answered either; select
    `entities { id depth }` and `paths { from predicate to }`, and add `direction`.
- **`entitiesByPrefix` answers a page.** The answer is `{ entities { id triples } total next_cursor }`; send
  `next_cursor` back as `cursor` for the next page. An empty `prefix` still matches every entity. SemConnect selects
  `id triples` directly on the field (`ui/src/lib/services/graphGateway.ts:107-117`); move that selection under
  `entities` and read the list from `entitiesByPrefix.entities`.
- **`entity` is unchanged:** `entity(id: $id) { entity { id triples { ... } } kvRevision }`, with `kvRevision` a
  JSON number. An entity whose stored value has revision 0 is refused, not answered.
- **Fields not in the schema.** Entity `storage_ref`, `message_type` and `updated_at`, and triple `context`,
  `datatype` and `expires_at`, are not declared, so selecting one is refused. A triple has `subject`, `predicate`,
  `object`, `source`, `timestamp` and `confidence`. Any of the six can be added later without breaking a query that
  works today.
- **One root field per request.** Ask `entity` and `pathSearch` in two requests. `__typename` may sit beside the one
  field.

## Operations not served

- **`globalSearch`** is not served; SemConnect calls it (`ui/src/lib/services/graphGateway.ts:49-51`). graph-query
  answers the same question on its NATS verb `globalSearch` (see `processor/graph-query/README.md`).
- **`trajectory`** is refused with HTTP 400 (owner, 2026-11-10). Its answer is agent detail that SemTeams now owns, so
  SemTeams answers it. Write a component of your own with a method `RegisterHTTPHandlers(prefix string, mux
  *http.ServeMux)`: the service manager mounts every such component on the same HTTP server under `/<instance>`, the way
  it mounts this gateway at `/graph-gateway`. Point the UI's one `trajectory` call (`ui/src/lib/services/agentApi.ts`)
  at that route.
- **Introspection** (`__schema`, `__type`) is refused with HTTP 400; `__typename` answers. The only caller found is a
  SemTeams test that semteams #406 deletes. Read `schema.graphql` for the schema.
- The other 14 SemStreams operations, the `/mcp` route and the playground are not served.

## Configuration changes (stop boot)

Each of these fails when the component is created, naming the key. Delete the line before the first start.

- **Keys left:** `ports` and `query_timeout_ms` (default 60000). An empty configuration is enough.
- **Refused:** `graphql_path` (the route is always `/<instance>/graphql`), `mcp_path`, `bind_address` and
  `standalone_server` (the service manager's HTTP server serves the route), `enable_playground`,
  `enable_inference_api`, `domain_examples_path`, `enable_embedding_classifier`, `readiness_keys` and
  `readiness_path`. **`query_timeout`** is refused: write `query_timeout_ms` in milliseconds. SemSource sets
  `bind_address` and `enable_playground`; SemTeams' `configs/flow-bootstrap.json` sets `graphql_path`, `mcp_path`,
  `enable_playground` and `query_timeout`.
- **Ports:** the one output is `graph_queries`, a NATS request port on `graph.query.*` (README, "Ports"). The
  `graph_index_queries` and `agentic_queries` ports are refused, and so is any input. SemSource declares three
  outputs, `agentic_queries` among them; leave `ports` out to take the default. `entitiesByPredicate` now reaches
  graph-index through graph-query, so the composition needs graph-query but no port of its own on graph-index.

## Answers that change without an error

- **Only what you select comes back.** SemStreams copied the whole reply into `data.<field>`. Code that read a field
  it did not select gets nothing now: select every field you read.
- **A failed field is null; the rest of the answer stands.** A refusal from the graph, such as an entity that does
  not exist, answers HTTP 200 with `data.<field>` null and `class` and `code` in `errors[0].extensions`. A reply
  that does not fit the schema answers 200 with the field null and an error naming the path; SemStreams answered 500.
- **No responder names the subject.** When graph-query is not running, the answer is HTTP 500 naming the subject,
  such as `graph.query.entity`; SemStreams said `query failed`. Past `query_timeout_ms` the answer is 504.
- **503 before start and during stop.** Until the component has started, and once it begins stopping, every request
  answers 503 `graph-gateway is stopping`. SemStreams' readiness route is not carried.
- **Names inside arguments no longer route.** An ID such as `x.y.pathsearch.trajectory.a.b` passed to `entity` is
  read as an ID; SemStreams could send it to the wrong operation.
