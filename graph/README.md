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
- **The query reply envelope.** Replies on `graph.ingest.query.*` are not enveloped. Replies on `graph.index.query.*`
  are a `QueryResponse`: the verb's result in `data`, the `ENTITY_STATES` revision it reflects in `indexed_revision`,
  and the answering component in `producer`; `NewQueryResponse` refuses an empty producer. On `graph.query.*` the shape
  depends on the verb: `entity`, `batch`, `prefix` and `hierarchyStats` are bare, and `byName`, `predicate`,
  `entityByAlias`, `relationships`, `pathSearch`, `summary`, `localSearch`, `globalSearch`, `searchGraph`, `semantic`,
  `similar`, `spatial` and `temporal` are enveloped. A caller
  decodes by the verb's table row (`QueryVerb.Enveloped`, and `QueryVerb.Reply` for the type of the body or of `data`),
  never by the reply's content.
- **Reading at a revision.** A request on an enveloped verb may carry `min_revision` (`MinRevisionField`), the
  `kv_revision` of a write the answer must include. An index that has not reached it refuses with transient
  `revision_not_reached` (`ErrorCodeRevisionNotReached`), the revision it reached in the detail, and does not wait. The
  bare verbs ignore the field.
- **The verb table.** `QueryVerbs` declares the request/reply verbs, each with its subject and responder:
  graph-ingest's `entity`, `batch` and `prefix` on `graph.ingest.query.*`, and graph-index's eight verbs (`outgoing`,
  `incoming`, `alias`, `predicate`, `predicateList`, `predicateStats`, `predicateCompound`, `byName`) on
  `graph.index.query.*`, the four verbs of the embedding, spatial and temporal responders (`search` and `similar` on
  `graph.embedding.query.*`, `bounds` on `graph.spatial.query.*`, `range` on `graph.temporal.query.*`), and graph-query's
  `entity`, `batch`, `prefix`, `byName`, `predicate`, `entityByAlias`, `relationships`, `pathSearch`, `hierarchyStats`,
  `summary`, `localSearch`, `globalSearch`, `searchGraph`, `semantic`, `similar`, `spatial` and `temporal` on
  `graph.query.*`. A name is unique only within its responder: `entity` is declared for graph-ingest and for
  graph-query. Each row also marks `Enveloped` and gives the `Reply` type.
  The embedding responder is in the tree (`processor/graph-embedding`). The spatial and temporal responders are not,
  so those two rows name a responder that does not exist, and graph-query's `spatial` and `temporal` forwards and its
  `globalSearch` time-range and area strategies are refused transient `internal`. A verb a consumer serves under
  `graph.query.*` is not in the table. `QueryVerbs`' doc comment names each verb's request and reply types.
- **Local search.** `LocalSearchRequest` names an entity (`entity_id`), a `level` from 0 to 2 and a `query` of 1 to
  `MaxSearchTerms` (16) distinct words, and the search runs in that entity's community: the group of entities that share
  the first 4, 3 or 2 parts of its ID. `LocalSearchData` is the reply: the community's record (`community_id`,
  `level`, `member_count`, `community_summary`, `keywords`), the matching `entities` best first, `entities_scanned`,
  `entities_truncated` and an `answer` sentence. The matches come from one page of at most 1,000 entities in ID order,
  so on a larger group they are the best of the lexically smallest IDs. `EntitiesTruncated` is true when the page had
  more to read or matches were left out to fit `max_payload`.
- **Global search.** `GlobalSearchRequest` is a `query` (required, not blank), a `level` (0 to 2, omitted means 0, read
  only by the text strategy) and `min_revision`; any other key is refused. The verbs `globalSearch` and `searchGraph`
  take it and reply with the same `GlobalSearchData`: `strategy` (`entity`, `path`, `temporal`, `spatial` or `text`)
  and exactly one field holding that strategy's result as read: `entity` (the entity verb's reply bytes), `path` (a
  `PathSearchData`), `temporal` (a `TemporalData`), `spatial` (a `SpatialData`) or `text` (a `TextSearchData`).
  `TextSearchData` holds the `groups` read (`TextSearchGroup`: `community_id`, `member_count`, `community_summary`,
  `keywords`), the `matches` (`TextSearchMatch`: `id`, `label`, `score` and the `entity` as the page sent it),
  `entities_scanned`, `entities_truncated` (true when the matches are a sample), `unreadable_groups`,
  `unhonoured_hints` (the hints the text search could not apply: `path`, `similarity`, `ranking`) and an `answer`
  sentence. `TemporalRangeRequest` (`start_time` and `end_time`, RFC 3339, `limit`, `min_revision`) and
  `SpatialBoundsRequest` (`north`, `south`, `east`, `west`, `limit`, `min_revision`) are what graph-query sends to the
  time-range and bounds verbs, and so what change 7's responders decode; the pin spelled the times `startTime` and
  `endTime`. Which questions each strategy answers, and what is refused, is in `processor/graph-query/README.md`.
- **Counts.** `PrefixQueryResponse.Total` is the number of distinct keys in the listing before the cursor and any limit,
  as listed and not a snapshot (#512). `HierarchyStatsData` and `SummaryData` build on one prefix page: `TotalEntities`
  is the whole group, `EntitiesScanned`, `Truncated` and `NextCursor` say how much of it this page covers, and the type
  counts or children cover that page and no more. `Truncated` false says only that it is the last page. When `summary`
  leaves predicates out (`ExcludePredicates`), its `indexed_revision` is 0, which claims nothing: do not compare it with
  a `kv_revision`.

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

- `graph/kvcatalog`: the bucket catalog, its acquisition, the owned-bucket retention check and the write-guard pair
  (`IsFrameworkOwnedBucket`, `FrameworkOwnedWriteRefusal`) the rule engine reads once it is ported (change 6).
- `natsclient`: `IsKVTombstone`, which reports whether a KV watch entry is a delete or a purge.
- `graph/readiness`: the readiness computation and the GRAPH_STATUS publisher.
- `graph/inference`: hierarchy inference (its hierarchy slice).

## Import

```go
import "github.com/c360studio/semengine/graph"
```
