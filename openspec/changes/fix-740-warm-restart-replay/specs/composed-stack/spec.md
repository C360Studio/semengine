# composed-stack Delta

## ADDED Requirements

### Requirement: A warm restart over a large store starts on default broker limits

A composed stack restarted over a retained store SHALL start, and every graph reader SHALL report its snapshot
applied, on a NATS server with its default `max_pending` of 64 MiB. No connection of the stack SHALL be closed by
the server as a slow consumer during the boot.

#### Scenario: Restart over 66 MB of entity state

- **WHEN** the stack is seeded with at least 66 MB in `ENTITY_STATES`, stopped, and started again on the same server
  with default settings
- **THEN** `Start` returns nil, every follower and the websocket view report the snapshot applied, and the account's
  slow-consumer count on the server is the same after the restart as before it
- **AND** the test that verifies this is `TestSmoke_WarmRestartOverALargeStore`

### Requirement: Each boot-time whole-bucket reader has a connection of its own

Each of these readers SHALL hold its whole-bucket watch on a NATS connection that carries no other watch: the
`ENTITY_STATES` follower of graph-index, graph-lexical, graph-clustering and graph-embedding; the websocket output's
`ENTITY_STATES` view; graph-query's `COMMUNITY_INDEX` view; graph-ingest's boot sweep; and graph-embedding's
failed-record scan, Worker and vector cache over `EMBEDDING_INDEX`. Watches outside this list (request-driven,
small boot-time, or a product's own) are not covered.

#### Scenario: A follower's watch adds one connection

- **WHEN** a follower's `Run` is running
- **THEN** the server lists one connection more than before `Open`
- **AND** the test that verifies this is `TestFollowerWatchHasItsOwnConnection`

### Requirement: A reader's connection lives as long as its watch

Each reader named above SHALL close its connection before the code that started its watch returns: the run that
holds the watch, or `Start` for the boot sweep and the failed-record scan. The followers and views connect theirs in
their run; graph-embedding's Worker and vector cache connect theirs in `Start`, where they are built.

#### Scenario: A view's connection lives as long as its run

- **WHEN** the websocket output's view, or graph-query's community view, runs and then its component stops
- **THEN** the view's connection is listed by the server while it runs and closed after `Stop`, and no natsclient
  goroutine is left running
- **AND** the tests that verify this are `TestWebsocketViewClosesItsConnection` and
  `TestCommunityViewClosesItsConnection`

### Requirement: Closing a reader's connection leaves nothing behind

A clean stop of each reader named above SHALL return what it returns today, log nothing at Error from the close, and
leave no natsclient goroutine running; a failed `Start` SHALL return its cause and leave none either.

#### Scenario: graph-ingest's boot sweep closes its connection inside Start

- **WHEN** graph-ingest's `Start` returns, with or without an error
- **THEN** the connection its boot sweep opened is closed, nothing from the close is logged at Error, and no
  natsclient goroutine is left running
- **AND** the test that verifies this is `TestEntityStateGuardClosesItsConnection`

#### Scenario: graph-embedding's watches each close their connection

- **WHEN** graph-embedding starts, runs, and stops
- **THEN** its failed-record scan's connection is closed before `Start` returns, and the Worker's and the vector
  cache's are closed before `Stop` returns, with no natsclient goroutine left running
- **AND** the test that verifies this is `TestGraphEmbeddingWatchesCloseTheirConnections`
