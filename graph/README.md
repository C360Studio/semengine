# Graph Package

The data model and the wire types of SemEngine's entity graph. The package imports no NATS client: a reader of a
graph type does not pull in the transport to get it (`graph-transport-boundary`, "The graph root imports no
transport"; `TestGraphImportsNoTransport`).

## What the package holds

- **The data model.** `EntityState` is one entity's stored state: its 6-part ID
  (`org.platform.system.domain.type.instance`, the key in the `ENTITY_STATES` bucket), its statements
  (`message.Triple`), an optional `StorageRef`, the `MessageType` that last wrote it, and `UpdatedAt`, the store's
  write time. It carries no version number of its own: the bucket's key-value revision is the only concurrency fence
  (`graph-entity-writes`, "The revision is the only fence"). A stored value written with a `version` key still
  decodes.
- **The replace rule**, a pure function with no I/O. `ReplaceBySource` treats the statements of one (subject,
  predicate, source) as one set: an arriving set replaces only the stored set of its own key, and a set older than the
  stored one (by `Timestamp`) is not applied and is reported in `ReplaceResult.Stale`. `Confidence` and `Context`
  never decide whether a write applies.
- **The single-value reads.** A predicate may hold statements from several sources. `EntityState.GetTriple`,
  `EntityState.GetPropertyValue` and `GetPropertyValue` pick the same statement every time: the latest `Timestamp`;
  on equal timestamps, the `Source` that sorts first; on equal sources, the first stored. A reader that wants another
  choice reads `Triples`.
- **The reserved sources.** `SourceIndexingProfile`, `SourceHierarchy` and `SourceLifecycle` are the names
  graph-ingest and `pkg/lifecycle` write their own statements under; `IsReservedSource` recognises them.
- **The state contract.** `MarshalEntityState` is the validating encoder for stored entities and
  `UnmarshalEntityState` the validating decoder; `StateContractError` and its classifiers carry the "graph state reset
  required" contract across a request and its reply. Entity-ID prefixes and the predicate token codec sit beside them.
- **Bucket names.** The `Bucket*` constants name the graph's buckets. The catalog that describes and acquires them is
  `graph/kvcatalog`.
- **The wire types.** The query request and reply types, the mutation requests and responses
  (`ReconcilePredicatesRequest` names the one source it reconciles), `ExactEntity` and `ExactEntityReader` for the
  authority read, and `IndexStatusResponse`, the readiness envelope a producer writes to `GRAPH_STATUS`, with its
  `IndexState*` constants. The computation that fills the envelope is in `graph/readiness`.
- **The query reply envelope.** Replies on `graph.ingest.query.*` are not enveloped.
- **The verb table.** `QueryVerbs` declares the request/reply verbs graph-ingest answers (`entity`, `batch` and
  `prefix` on `graph.ingest.query.*`), each with its subject, request type and reply type.

## What is elsewhere

- `graph/kvcatalog`: the bucket catalog, its acquisition, the owned-bucket retention check and `IsKVTombstone`.
- `graph/readiness`: the readiness computation and the GRAPH_STATUS publisher.
- `graph/inference`: hierarchy inference (its hierarchy slice).

## Import

```go
import "github.com/c360studio/semengine/graph"
```
