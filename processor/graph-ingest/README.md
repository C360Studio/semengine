# graph-ingest

Entity and triple ingestion component for the graph subsystem.

## Overview

The `graph-ingest` component is the entry point for entity data flowing into the graph. It consumes entity messages
from JetStream, serves the typed graph-mutation operations over NATS request/reply, and stores every entity in the
`ENTITY_STATES` KV bucket, which it owns. It also answers the authoritative entity reads declared for it in the verb
table in `graph` (`graph.QueryVerbs`).

## Architecture

```text
entity.>  (JetStream) ──┐
                        │  ┌──────────────┐
graph.mutation.> ───────┼─►│ graph-ingest ├──► ENTITY_STATES (KV)
  (request/reply)       │  └──────┬───────┘
                        │         └── answers graph.ingest.query.{entity,batch,prefix}
```

## Features

- **Entity writes:** create, merge, reconcile and delete entities.
- **Triple mutations:** append triples to an entity, and replace one source's triples of named predicates (reconcile).
- **Hierarchy inference:** optionally creates container entities from the 6-part entity ID structure.
- **At-least-once delivery:** JetStream consumption with explicit acknowledgment; a redelivery the applied-sequence
  guard has already seen is dropped and counted. The guard keeps the last applied sequence for each entity and stream in
  `GRAPH_INGEST_APPLIED_SEQ`, as eight bytes. A stored value of any other length cannot be decoded, so the input is
  neither applied nor acknowledged: it is delivered again, each refusal is counted, and the key is logged once.
  Deleting the key repairs it, and the next delivery is applied as first seen.

## Configuration

```json
{
  "type": "processor",
  "name": "graph-ingest",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {
          "name": "graph_mutations",
          "required": true,
          "config": {
            "kind": "nats-request",
            "subject": "graph.mutation.>",
            "interface": {"type": "semengine.graph.mutation", "version": "v1"}
          }
        },
        {
          "name": "entity_stream",
          "config": {"kind": "jetstream", "stream_name": "ENTITY", "subjects": ["entity.>"]}
        }
      ],
      "outputs": [
        {
          "name": "entity_states",
          "config": {"kind": "kv-write", "bucket": "ENTITY_STATES"}
        }
      ]
    },
    "enable_hierarchy": true
  }
}
```

### Configuration options

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | required | Input and output ports. Exactly one input must be the required `nats-request` port on `graph.mutation.>` with the `semengine.graph.mutation` v1 interface, and at least one output is required. |
| `enable_hierarchy` | bool | `false` | Create hierarchy container entities and edges (below). |
| `ingest_lanes` | int | `8` | Number of ingest lanes. Messages for one entity ID always use the same lane, so they apply in arrival order; different entities apply in parallel. `1` is fully serial. `0` counts as unset and takes the default of `8`; a negative value is clamped to `1`. |

A configuration key not in this table is refused when the component is built, with an error naming the key.

## Ports

### Inputs

| Name | Kind | Subject | Description |
| --- | --- | --- | --- |
| `graph_mutations` | nats-request | `graph.mutation.>` | The typed mutation operations (create, reconcile, append, delete) |
| `entity_stream` | jetstream | `entity.>` | Entity messages to merge into `ENTITY_STATES` |

### Outputs

| Name | Kind | Bucket | Description |
| --- | --- | --- | --- |
| `entity_states` | kv-write | `ENTITY_STATES` | Entity state storage |

## Request/reply verbs

graph-ingest subscribes to exactly the verbs `graph.QueryVerbs` declares with responder `graph-ingest`, and to no
other query subject. Replies are bare JSON, not wrapped in an envelope.

| Verb | Subject | Reply |
| --- | --- | --- |
| entity | `graph.ingest.query.entity` | `graph.ExactEntity` |
| batch | `graph.ingest.query.batch` | `graph.EntityBatchResponse` |
| prefix | `graph.ingest.query.prefix` | `graph.PrefixQueryResponse` |

## Hierarchy inference

When `enable_hierarchy` is on, the component creates container entities from the 6-part entity ID structure:

```text
org.platform.system.domain.type.instance
 │      │       │      │     │      │
 └──────┴───────┴──────┴─────┴──────┴─► Real entity
        │       │      │     │
        └───────┴──────┴─────┴─► Type container (5-part prefix + .group)
                │      │
                └──────┴─► Taxonomy container (4-part prefix, source × domain, + .group.container)
                       │
                       └─► Source container (3-part prefix + .group.container.level)
```

The padding tokens `group`, `container` and `level` are reserved instance values
(`pkg/types.IsReservedInstanceToken`). Containers are minted only for entities under the deployment's own
authority; an imported entity gets none. The payload registry must hold the hierarchy container type, or the
component is refused at construction.

## Dependencies

- **Upstream:** none; graph-ingest is the entry point.
- **Downstream:** components that watch `ENTITY_STATES`. None is ported to SemEngine yet.

## Metrics

Every collector is registered on the `MetricsRegistry` in the component's dependencies; with none, the collectors are
registered nowhere, and nothing is registered on Prometheus' global registry. Two components on one registry share
each collector.

| Metric | Type | Labels |
| --- | --- | --- |
| `semengine_datamanager_entities_updated_total` | counter | |
| `semengine_graph_ingest_indexing_profile_default_total` | counter | `message_type` |
| `semengine_graph_ingest_mutation_rejections_total` | counter | `subject`, `reason` |
| `semengine_graph_ingest_graph_mutation_outcomes_total` | counter | `operation`, `outcome` |
| `semengine_graph_ingest_batch_query_missing_total` | counter | `reason` |
| `semengine_graph_ingest_predicate_contract_rejections_total` | counter | `lane`, `reason` |
| `semengine_graph_ingest_entity_state_contract_rejections_total` | counter | `lane`, `field`, `reason` |
| `semengine_graph_ingest_processing_duration_seconds` | histogram | |
| `semengine_graph_ingest_ingest_lag_seconds` | histogram | |
| `semengine_graph_ingest_redeliveries_dropped_total` | counter | |
| `semengine_graph_ingest_guard_record_refusals_total` | counter | |
| `semengine_graph_ingest_cas_retries_total` | counter | |
| `semengine_graph_ingest_stale_sets_total` | counter | |
| `semengine_graph_ingest_duplicate_triples_suppressed_total` | counter | `lane` |
| `semengine_graph_ingest_poisoned_entities` | gauge | |

The readiness gauges of `graph/readiness` (`readiness`, `lag`, `bootstrap_complete`, `readiness_state` and
`status_publish_failures_total`, under the `graph_ingest` subsystem) and the ingest pool's `dispatch_*` collectors are
registered on the same registry.

## Health

The component reports healthy when it is running, no processing error has been counted, and no stored entity fails to
decode (the poison inventory is empty). Running implies the boot sweep of `ENTITY_STATES` completed: `Start` fails
when the sweep cannot reach the end of the snapshot.
