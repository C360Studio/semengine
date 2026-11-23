# composed-stack Delta

## MODIFIED Requirements

### Requirement: A running stack recovers from a broker restart

With file storage, a stack that keeps running while the NATS server restarts on the same URL SHALL reach ready again
without a process restart, and SHALL apply what is published after the restart. Because of the restart, no
`GRAPH_STATUS` envelope SHALL report a lost feed, graph-lexical's included, and
`/components/health` SHALL list all seven components by name, each healthy.

#### Scenario: Restart the broker under the running stack

- **WHEN** the NATS server restarts while the stack runs, and a second batch, an edit and a delete are published after
- **THEN** the graph-index, graph-clustering and graph-lexical keys reach their revision and graph-ingest's reads not
  degraded, with answers, graph-lexical's `search`, `localSearch` and global search included, that include them, no
  envelope reports a lost feed, and `/components/health` lists graph-ingest, graph-index,
  graph-clustering, graph-lexical, graph-query, graph-gateway and websocket, a total of 7, each healthy
  (`TestSmoke_BrokerRestartUnderRunningStack`)
