# pkg/fusion

A deterministic engine that answers one question about the knowledge graph with one response. You give it a query and
a *lens*; it finds the matching entities, reads the structure around them, ranks and trims the result, and returns
nodes keyed by what a human reads (a name, a path, a line), never by an entity ID. No language model is involved:
the same graph and the same query give the same answer. The decision record is
[ADR-062](../../docs/adr/062-deterministic-graph-fusion.md).

A *lens* is the one piece a product writes. It says how a query is resolved (`nl`, `symbol` or `prefix`), which edges
to walk and what to call them, and how to name, classify and locate an entity. A *seed* is an entity the resolve step
returned; everything else in a response grows from the seeds.

Migrating from SemStreams' `pkg/fusion`: read [MIGRATION.md](MIGRATION.md).

## Use

```go
engine, err := fusion.NewEngine(client, fusion.NewBodyResolver(storeRegistry), logger,
    fusion.WithSignals(fusionvocab.New(cco.Parents)), // optional: ontology and salience ranking
    fusion.WithMetrics(registry),                     // optional: the two counters below
)
if err != nil {
    return err // a nil client or logger, or a registry that refused a counter
}
req := fusion.Request{Query: "ParseConfig", Want: []fusion.Want{fusion.WantBody, fusion.WantRelations}}
resp, err := engine.Fuse(ctx, req, lens)
```

`client` is any `fusion.RetrievalClient` (in this repository, a fake in tests; the NATS client is not in the tree
yet). `lens` is your `fusion.Lens`. `NewBodyResolver` may be given `nil` if you never ask for bodies; a node whose
lens names a body then reports `body_reason` `error`.

## What a response holds

| Part | Request it with | Meaning |
| --- | --- | --- |
| `nodes` | always | one per seed, ranked, within `budget` (`max_nodes` 20, `max_bytes` 60000 by default) |
| `body` | `WantBody` (the default) | the stored text of the node, read through the lens's storage handle |
| `relations` | `WantRelations` (the default) | refs to neighbours, by role the lens names, up to 12 per role |
| `paths` | `WantPaths` | up to 10 outgoing paths of names, up to 4 hops |
| `impact` | `WantImpact` | counts of entities and files that depend on the seeds (reverse closure, up to 200 nodes) |
| `graph` | `WantGraph` | typed facts, directed edges and their evidence, with caps of its own |

Ranking starts from the order the resolve step returned, then moves a node by how well its name matches the query. If
you pass `WithSignals`, a more specific ontology class and a heavier predicate weight move it a little more; a
negative weight moves it down. Entities with equal scores keep resolve order.

## The response says what it left out

An empty or short answer is never ambiguous. Each of these is in the response, and each is omitted when it is zero
or false:

- `deferred` with `defer_reason`: the engine withheld the answer. `index_not_ready` means a read replied that the
  index is still catching up; `all_seeds_unhydrated` means seeds resolved but none could be read (`unhydrated` lists
  them). A deferred response has no nodes and no `misses`: it is not "nothing found".
- `misses`: the graph answered and found nothing, with near-matches in `did_you_mean`. A miss does not prove the
  entity is absent.
- `unreadable_records` and `failed_records`: a word search (`nl` resolve) counts the records it could not read and the
  records whose embedding failed. They ride on every response built after that search answered, including a miss.
  Symbol and prefix resolves never set them.
- `relations_truncated` (on a node): a role had more than 12 relations, a relations read failed, or a target could not
  be read.
- `paths_truncated`, and `truncated` on a path: more than 10 paths existed, or a path stopped at a node that could not
  be read. A path never names an unread node.
- `impact.truncated` and `graph.truncated`: the cap was reached or a read was skipped.
- `body_reason` (on a node): the body was asked for and could not be loaded (`not_found` or `error`). The node still
  ships.
- `truncated` (top level): the budget cut nodes off.

`contract_version` is `"2"`. When a `Fuse` call returns an error instead, either the context ended, or `Resolve` or
`Entities` failed with something other than `index_not_ready` (a graph reset included). A seed read or a relations
neighbour read that replies `index_not_ready` makes the whole response `deferred`. Every other failed read after the
seeds (relations neighbours with any other error, relation targets, paths, impact, graph) never fails the call: it is
skipped and shown as one of the labels above.

## Logs and counters

Each skipped read adds one to `semengine_fusion_skipped_reads_total{facet}` (`relations`, `paths`, `impact`,
`graph`), and each `Fuse` call logs at most one Warn line per facet that skipped, with the count and the first error.
A body that cannot be loaded logs a Warn line and adds one to
`semengine_fusion_body_hydration_failures_total{reason}`. Both counters exist only on the registry passed to
`WithMetrics`; without it there are no counters and the logs and response labels remain.

## Packages

- `pkg/fusion`: the engine, `Lens`, `RetrievalClient`, `BodyResolver`, the response types. It imports no NATS client
  and no ontology package.
- `pkg/fusion/fusionvocab`: `New(parents)` returns the ranking signals. `parents` is a function from a class IRI to
  its ancestors; the BFO/CCO hierarchy lives in the semvocab module, which depends on SemEngine, so the caller passes
  semvocab's `cco.Parents`. Pass `nil` to rank with no ontology signal. Predicate weights come from this
  repository's `vocabulary` registry.

## Not in this package

- A `RetrievalClient` over NATS (`fusionnats`) is not in the tree.
- The SemStreams sub-query engine (`engine.go`, `subquery.go`, `evidence.go`, `client.go`) and `lensregistry` are not
  carried. A product creates its lens and passes it to `Fuse`.
