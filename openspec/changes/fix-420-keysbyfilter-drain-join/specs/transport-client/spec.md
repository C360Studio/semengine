# transport-client

## MODIFIED Requirements

### Requirement: A filtered listing stops its watcher before returning

Before returning, they SHALL stop the watcher and wait until its update channel closes, past the call's context, with
no fixed timeout in place of that wait. Changed: the wait gave up after 5 s and returned an error with nats.go's
delivery possibly still running (#420).

#### Scenario: The watcher is stopped

- **WHEN** either call returns
- **THEN** the watcher is stopped and its update channel has closed

#### Scenario: Delivery closes late

- **WHEN** the watcher's update channel stays open for longer than 5 s after `Stop` returns
- **THEN** the call has not returned before the channel closes, and returns once it has
