# Delta: graph-readiness

## MODIFIED Requirements

### Requirement: Readiness wiring is refused, not ignored

`readiness.NewPublisher` SHALL return an error, and no publisher, for a nil bucket or an empty key. `Watcher.Run`
SHALL return an error before any I/O for a nil context, a nil bucket source, an empty key, a heartbeat of zero or less,
or a nil logger. `Publisher.Publish` SHALL return an error for a nil context without writing, and
`Watcher.WaitForFirst` SHALL return an error for a nil context whether or not an envelope has arrived. None SHALL
substitute a default for a missing value.

#### Scenario: A publisher with no key

- **WHEN** `NewPublisher` is called with a bucket and an empty key
- **THEN** it returns an error and a nil publisher

#### Scenario: A watcher with no heartbeat

- **WHEN** `Run` is called on a watcher built with a heartbeat of zero
- **THEN** it returns an error without opening the bucket

#### Scenario: A publish with a nil context

- **WHEN** `Publish` is called with a nil context
- **THEN** it returns an error and the bucket receives no write

#### Scenario: Waiting for the first envelope with a nil context

- **WHEN** `WaitForFirst` is called with a nil context after an envelope has arrived
- **THEN** it returns an error instead of reporting the envelope
