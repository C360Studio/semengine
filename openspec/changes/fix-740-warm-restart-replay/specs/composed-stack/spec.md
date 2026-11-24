# composed-stack Delta

## ADDED Requirements

### Requirement: A warm restart over a large store starts on default broker limits

A composed stack restarted over a retained store SHALL start, and every graph reader SHALL report its snapshot
applied, on a NATS server with its default `max_pending` of 64 MiB. No connection of the process SHALL be closed by
the server as a slow consumer during the boot.

#### Scenario: Restart over 66 MB of entity state

- **WHEN** the stack is seeded with at least 66 MB in `ENTITY_STATES`, stopped, and started again on the same server
  with default settings
- **THEN** `Start` returns nil, every follower and the websocket view report the snapshot applied, and no connection
  of the process is closed as a slow consumer
- **AND** the test that verifies this is `TestSmoke_WarmRestartOverALargeStore`

### Requirement: Each whole-bucket watch has a connection of its own

Every watch that replays a whole `ENTITY_STATES`, `EMBEDDING_INDEX` or `COMMUNITY_INDEX` bucket SHALL run on a NATS
connection that carries no other such watch. The connection SHALL be opened when the watch starts and closed before
the code that started it returns: the run that holds the watch (the follower's `Run`, a view's run, the embedding
Worker's `Run`), or `Start` for graph-ingest's boot sweep and graph-embedding's failed-record scan.

#### Scenario: A follower's connection lives as long as its watch

- **WHEN** a follower's `Run` is running, and then returns
- **THEN** the server lists one connection more than before `Open` while it runs, and none more after it returns
- **AND** the test that verifies this is `TestFollowerWatchHasItsOwnConnection`

#### Scenario: graph-ingest's boot sweep closes its connection inside Start

- **WHEN** graph-ingest's `Start` returns, with or without an error
- **THEN** the connection its boot sweep opened is closed
- **AND** the test that verifies this is `TestEntityStateGuardClosesItsConnection`
