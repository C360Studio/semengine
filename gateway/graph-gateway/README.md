# graph-gateway

A read-only GraphQL endpoint over the graph, for web front ends and other HTTP callers.

## Overview

`graph-gateway` answers `POST /<instance>/graphql` on the service manager's HTTP server: a component named
`graph-gateway` answers at `/graph-gateway/graphql`. It answers four read operations by asking graph-query, which asks
graph-ingest and graph-index. It writes nothing, holds no NATS state and runs no goroutine of its own: each request
makes its one NATS request on the HTTP handler's goroutine.

Its schema, `schema.graphql` in this package, is the contract. A request is parsed and validated against it before
anything is asked of the graph, and the answer holds exactly the fields the request selected, under their aliases and
through fragments. Nothing is chosen by searching the query text, so a word such as `pathsearch` inside an argument
routes nowhere.

`MIGRATION.md` lists what a consumer of the SemStreams gateway changes at its cutover.

## Operations

| Root field | Arguments | Answer | Asks |
| --- | --- | --- | --- |
| `entity` | `id: String!` | `{ entity { id triples } kvRevision }` | `graph.query.entity` |
| `entitiesByPrefix` | `prefix: String!`, `limit: Int`, `cursor: String` | `{ entities { id triples } total next_cursor }` | `graph.query.prefix` |
| `entitiesByPredicate` | `predicate: String!`, `value: String`, `limit: Int` | `{ entities }`, a list of entity IDs | `graph.query.predicate` |
| `pathSearch` | `startEntity: String!`, `direction: String!`, `maxDepth: Int!`, `maxNodes: Int`, `predicates: [String]` | `{ entities { id depth } paths { from predicate to } truncated }` | `graph.query.pathSearch` |

A triple is `{ subject predicate object source timestamp confidence }`; `object` is any JSON value and `timestamp` is
RFC 3339. `kvRevision` is the entity's `ENTITY_STATES` revision, a JSON number. Each subject is read from
`graph.QueryVerbs()` when the component is created, and none is spelled in this package.

- **`entity`** decodes the stored value with `graph.UnmarshalEntityState` and refuses a reply with no entity or a
  revision of 0, the checks `graph.NewExactEntityReader` makes (ADR-091, section 4, at the pin).
- **`entitiesByPrefix`** checks the page with `graph.ValidateDecodedEntityStates`. An empty `prefix` matches every
  entity. Send `next_cursor` back as `cursor` for the next page; graph-ingest caps `limit` at
  `graph.MaxPrefixQueryLimit`.
- **`entitiesByPredicate`** lists the entities holding `predicate`, only those whose value is `value` when it is given.
  A match on nothing answers `[]`.
- **`pathSearch`** walks from `startEntity`. `direction` is `outgoing`, `incoming` or `both`, in any letter case.
  `maxDepth` is 1 to 10 and `maxNodes`, the start included, is 1 to 500, 100 when left out; a value outside a limit is
  refused, never clamped. `predicates` keeps only edges holding one of them; a null item in it is refused with HTTP
  400. `entities[0]` is the start at depth 0. `paths[k]` is the run of edges from the start to `entities[k]`, so
  `paths[0]` is `[]`, and each step names the edge as the graph holds it (`from` holds `predicate` pointing at `to`),
  whichever way the walk crossed it. `truncated` is true exactly when the walk found an entity it had no room for.
  For an entity's triples, ask `entity` or `entitiesByPrefix`.

One request names one root field, besides `__typename`. Ask several in several requests. There is no mutation, no
subscription and no introspection: `__schema` and `__type` are refused, and `__typename` answers from the schema.

A request body is `{"query": "...", "variables": {...}, "operationName": "..."}`; `variables` and `operationName` are
optional, and `operationName` is required when the document holds more than one operation. In the example,
`$HOST` is the address and port of the service manager's HTTP server.

```bash
curl -s "http://$HOST/graph-gateway/graphql" -H 'Content-Type: application/json' \
  -d '{"query":"query($id: String!) { entity(id: $id) { kvRevision entity { id } } }","variables":{"id":"c360.demo.fleet.ops.drone.001"}}'
```

## Answers and errors

| Case | HTTP | Body |
| --- | --- | --- |
| A method other than POST | 405, `Allow: POST` | `errors` |
| A body that is not one JSON object; a query that does not parse or does not validate; a mutation or subscription; two root fields; `__schema` or `__type`; a null `predicates` item | 400 | `errors` naming the field or argument, no `data` |
| A refusal from the verb behind the field, such as an entity that does not exist | 200 | `data.<field>` null; `errors[0].extensions` holds `class` and `code` |
| No responder, or a lost connection | 500 | `data.<field>` null; the error names the subject, such as `graph.query.entity` |
| No answer within `query_timeout_ms` | 504 | `data.<field>` null; the error says `request timeout` |
| A reply that does not fit the schema or fails the checks above | 200 | `data.<field>` null; the error names the path |
| Before `Start`, and from the moment `Stop` begins | 503 | plain text `graph-gateway is stopping` |

Every root field is nullable, so a refused field answers null and the rest of `data`, such as `__typename`, stands. No
partial value of a field is returned: a reply that does not fit anywhere inside it nulls the whole field.

## Configuration

An empty configuration is enough. This is the same configuration written out:

```json
{
  "type": "gateway",
  "name": "graph-gateway",
  "enabled": true,
  "config": {
    "ports": {
      "outputs": [
        {
          "name": "graph_queries",
          "required": true,
          "config": {
            "kind": "nats-request",
            "subject": "graph.query.*",
            "interface": {"type": "graph.query", "version": "v1"}
          }
        }
      ]
    },
    "query_timeout_ms": 60000
  }
}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the port above | A `ports` block that is present is used as written; one that is absent takes the default |
| `query_timeout_ms` | int | 60000 | How long one request waits for graph-query's reply, in milliseconds; past it the answer is HTTP 504. `0` means the default. Negative is refused, and so is anything above 9223372036854, which does not fit a Go duration. |

The configuration is decoded strictly: a key not listed here is refused when the component is created, naming it.
The route is always `<prefix>/graphql`, and the HTTP server, its address and any middleware are the service manager's.

## Ports

| Direction | Name | Kind | Subject |
| --- | --- | --- | --- |
| output | `graph_queries` | nats-request | `graph.query.*` |

The one output is required and has to be this port: a different name, subject, kind or interface, a `timeout` other
than `1s` (the value it takes when none is written), `retries`, a second output, or any input is refused. Its
`description` and `external` are yours to set.

## Health

`Health` is running or stopped, from `component.Lifecycle`; the gateway keeps no state that could leave it degraded.
`DataFlow` counts each POST as one input, with the body length the client declared (0 when it declared none).
`ErrorCount` counts each 500 and 504: a request that got no answer from graph-query, so an outage behind it shows here.
A refusal the verb sent is an answer and is not counted. The gateway registers no metrics of its own.

## Starting and stopping

`Start` needs a NATS client and acquires nothing else. Until it returns, and from the moment `Stop` begins, every
request answers 503. `Stop` refuses new requests, then returns once every admitted request has answered; if its
context ends first, it returns that error and a later `Stop` waits again.

## Dependencies

- Registered by a composition root through `Register` (type `gateway`); importing the package registers nothing.
- graph-query must run in the same composition, with graph-ingest and graph-index behind it.
- `github.com/vektah/gqlparser/v2` parses and validates requests.
