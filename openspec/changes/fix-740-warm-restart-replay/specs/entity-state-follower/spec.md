# entity-state-follower Delta

## ADDED Requirements

### Requirement: The follower's watch runs on a connection of its own

`Run` SHALL open a sibling of `Config.Client` for its watch and SHALL close it before it returns. `Run` SHALL wait for
the sibling to connect for at most `Config.StartupAttempts` attempts `Config.StartupInterval` apart, a wait of its
own beside `Open`'s wait for the bucket. A sibling that does not connect within that wait SHALL declare the feed
lost, as a watch that cannot start does. The follower's status reads SHALL stay on `Config.Client`.

#### Scenario: A broker that is gone when Run starts is a lost feed

- **WHEN** the broker stops after `Open` returned and before `Run` starts
- **THEN** `Run`'s wait ends, the follower declares the feed lost, and `QueryReady` refuses with the lost-feed reason,
  not
  the initial-build one
- **AND** the test that verifies this is `TestFollowerDeclaresLostFeedWhenItsConnectionCannotOpen`

### Requirement: A clean stop of Run leaves nothing behind

When `Run` returns because its context ended, it SHALL return what it returns today, log nothing at Error from the
close, close its sibling, and leave no goroutine of the sibling's running.

#### Scenario: A clean stop closes the connection and leaves nothing

- **WHEN** `Run` returns because its context ended
- **THEN** it returns what it returns today, nothing from the close is logged at Error, the server lists no connection
  of the follower's beyond
  `Config.Client`'s, and no natsclient goroutine is left running
- **AND** the test that verifies this is `TestFollowerWatchHasItsOwnConnection`
