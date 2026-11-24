# entity-state-follower Delta

## ADDED Requirements

### Requirement: The follower's watch runs on a connection of its own

`Run` SHALL open a sibling of `Config.Client` for its watch, connecting it within the same startup wait `Open` uses
for the bucket, and SHALL close it before it returns. A sibling that cannot be connected within that wait SHALL
declare the feed lost, as a watch that cannot start does. The follower's status reads SHALL stay on `Config.Client`.

#### Scenario: Run closes its connection when it returns

- **WHEN** `Run` returns because its context ended
- **THEN** the connection it opened is closed and the server lists no connection of the follower's
- **AND** the test that verifies this is `TestFollowerWatchHasItsOwnConnection`

#### Scenario: A connection that cannot be opened is a lost feed

- **WHEN** the sibling cannot connect within the startup wait
- **THEN** the follower declares the feed lost and `QueryReady` refuses
- **AND** the test that verifies this is `TestFollowerDeclaresLostFeedWhenItsConnectionCannotOpen`
