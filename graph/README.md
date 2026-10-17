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
- **The single-value reads.** A predicate may hold statements from several sources. `EntityState.GetTriple` and
  `EntityState.GetPropertyValue` pick the same statement every time: the latest `Timestamp`; on equal timestamps, the
  `Source` that sorts first; on equal sources, the first stored. A reader that wants another choice reads `Triples`.
- **The reserved sources.** `SourceIndexingProfile`, `SourceHierarchy` and `SourceLifecycle` are the names
  graph-ingest and `pkg/lifecycle` write their own statements under; `IsReservedSource` recognises them.
- **The state contract.** `MarshalEntityState` is the validating encoder for stored entities and
  `UnmarshalEntityState` the validating decoder. `StateContractError` carries the "graph state reset required" failure
  across a request and its reply, and `IsStateContractError` recognises it. Entity-ID prefixes and the predicate token
  codec sit beside them.
- **Bucket names.** The `Bucket*` constants name the graph's buckets. The catalog that describes and acquires them is
  `graph/kvcatalog`. A product that needs the full list of framework buckets calls `kvcatalog.KVCatalog()`: it returns
  a copy of every descriptor, so editing the result changes nothing the framework resolves.
- **The wire types.** The query request and reply types, the mutation requests and responses
  (`ReconcilePredicatesRequest` names the one source it reconciles), `ExactEntity` and `ExactEntityReader` for the
  authority read, and `IndexStatusResponse`, the readiness envelope a producer writes to `GRAPH_STATUS`. Its state
  field is an `IndexState`, one of the four `IndexState*` constants that `IndexStates()` lists. The computation that
  fills the envelope is in `graph/readiness`.
- **The query reply envelope.** Replies on `graph.ingest.query.*` are not enveloped.
- **The verb table.** `QueryVerbs` declares the request/reply verbs graph-ingest answers (`entity`, `batch` and
  `prefix` on `graph.ingest.query.*`), each with its subject and responder; `QueryVerbs`' doc comment names each
  verb's request and reply types.

### Entity Identification

All entity IDs use the 6-part federated format defined in `types.EntityID` (`pkg/types`) (canonical order, ADR-102:
`org.platform` = the minting deployment authority, `system` = the source, `domain.type` = a delegated taxonomy):

```text
org.platform.system.domain.type.instance
```

Example: `c360.telemetry.mavlink.robotics.drone.42`

To parse entity IDs and extract type information:

```go
eid, err := message.ParseEntityID(state.ID)
if err != nil {
    return fmt.Errorf("invalid entity ID: %w", err)
}

entityType := eid.Type      // "drone"
system := eid.System        // "mavlink"
instance := eid.Instance    // "42"
```

## Graphable Interface

The `Graphable` interface enables domain payloads to self-declare their entities and relationships:

```go
type Graphable interface {
    EntityID() string                  // Returns 6-part federated ID
    Triples() []message.Triple        // Returns all facts about this entity
}
```

**Why it's here**: Components implementing graph integration naturally import `graph/` to find contracts.
This interface defines how payloads transform into graph-compatible data.

### Example Implementation

```go
type PositionPayload struct {
    SystemID  uint8   `json:"system_id"`
    Latitude  float64 `json:"latitude"`
    Longitude float64 `json:"longitude"`
    Altitude  float32 `json:"altitude"`
}

func (p *PositionPayload) EntityID() string {
    return fmt.Sprintf("acme.telemetry.robotics.mavlink.drone.%d", p.SystemID)
}

func (p *PositionPayload) Triples() []message.Triple {
    entityID := p.EntityID()
    return []message.Triple{
        {Subject: entityID, Predicate: "geo.location.latitude", Object: p.Latitude},
        {Subject: entityID, Predicate: "geo.location.longitude", Object: p.Longitude},
        {Subject: entityID, Predicate: "geo.location.altitude", Object: p.Altitude},
    }
}
```

## Design Principles

### One-Way Transformation

`Graphable` → `EntityState` is a one-way transformation at message ingestion time:

1. Domain payload implements `Graphable` interface
2. `Graphable.Triples()` generates triples at runtime from payload fields
3. `EntityState` persists those triples in NATS KV storage
4. No reverse transformation (storage → payload) is supported or needed

### Triple-Based Storage

Properties, relationships, and domain-specific data are all stored as RDF-like triples:

```go
Triple{
    Subject:   "c360.telemetry.robotics.mavlink.drone.42",
    Predicate: "geo.location.latitude",
    Object:    34.052235,
}
```

This provides maximum flexibility while maintaining semantic clarity.

## What is elsewhere

- `graph/kvcatalog`: the bucket catalog, its acquisition, the owned-bucket retention check and `IsKVTombstone`.
- `graph/readiness`: the readiness computation and the GRAPH_STATUS publisher.
- `graph/inference`: hierarchy inference (its hierarchy slice).

## Import

```go
import "github.com/c360studio/semengine/graph"
```
