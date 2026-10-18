# graph-transport-boundary

## RENAMED Requirements

- FROM: `### Requirement: The readiness envelope carries its publish time and no legacy fields`
- TO: `### Requirement: The readiness envelope carries no legacy fields`

## MODIFIED Requirements

### Requirement: The readiness envelope carries no legacy fields

`graph.IndexStatusResponse`, the value a producer writes to its `GRAPH_STATUS` key, SHALL carry no `published_at`,
`phase`, `revision` or `last_synced` field; `indexed_revision` is the one spelling of the revision. graph-ingest SHALL
publish `bootstrap_complete` true on every envelope, its boot work having finished before `Start` returned.

#### Scenario: A published envelope

- **WHEN** a producer publishes its status
- **THEN** the stored JSON has no `published_at`, `phase`, `revision` or `last_synced` key

#### Scenario: graph-ingest with a backlog standing

- **WHEN** graph-ingest has started and messages are outstanding on its bound consumer
- **THEN** its envelope has `bootstrap_complete` true and `ready` false
