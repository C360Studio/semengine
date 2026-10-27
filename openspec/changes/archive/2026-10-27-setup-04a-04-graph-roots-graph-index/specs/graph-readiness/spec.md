# graph-readiness

## ADDED Requirements

### Requirement: ComputeIndexStatus projects a revision watermark

`readiness.ComputeIndexStatus` SHALL report `ready` when the target is above zero and the indexed floor reaches it,
`lag` as the target less the floor (never below zero), state `degraded` when the failed count is above zero, else
`ready` when ready, else `degraded` when stuck, else `building`; and `staleness_ms` 0 when ready or the commit time is
unknown, else the age at least 1.

#### Scenario: Caught up over failures

- **WHEN** the floor equals a nonzero target and the failed count is 1
- **THEN** `ready` is true and the state is `degraded`

### Requirement: A revision-lag producer exports its revisions

The gauge set built for a revision-lag producer SHALL also export `indexed_revision` and `target_revision`; the set
built for a backlog producer SHALL not.

#### Scenario: graph-index's gauges

- **WHEN** graph-index's follower registers its gauges and publishes an envelope with floor F and target T
- **THEN** `semengine_graph_index_indexed_revision` reads F and `semengine_graph_index_target_revision` reads T
