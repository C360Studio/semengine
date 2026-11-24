# transport-client Delta

## ADDED Requirements

### Requirement: A Client opens a sibling with its settings

`Client.NewSibling() (*Client, error)` SHALL return a Client that is not connected and carries the parent's server
URLs, credentials, timeouts and reconnect settings, and the parent's name, which every sibling shares. Once the parent's
`Close` has begun, `NewSibling` SHALL return an error matching `nats.ErrConnectionClosed`. The sibling SHALL promise
nothing about timing beyond what any Client promises.

#### Scenario: No sibling once Close has begun

- **WHEN** `NewSibling` is called after the parent's `Close` has begun
- **THEN** it returns an error matching `nats.ErrConnectionClosed`
- **AND** the test that verifies this is `TestSiblingRefusedAfterClose`

### Requirement: A sibling has no metrics and no exit policy

The sibling SHALL have no metrics: no collector of its own, no metrics poller, and no write to the parent's series;
a permission refusal on the sibling SHALL be logged and not counted. The sibling SHALL have no connection-lost
callback and no connection-loss timeout. The parent's `Close` SHALL NOT close a sibling; each sibling is closed by
its own `Close`.

#### Scenario: A sibling reaches the parent's server with the parent's credentials

- **WHEN** a sibling of a Client built with credentials for a password-protected server calls `Connect`
- **THEN** it connects, and the server's connection listing shows it under the parent's name
- **AND** the test that verifies this is `TestSiblingCarriesTheClientSettings`

#### Scenario: A sibling has no metrics and no exit policy

- **WHEN** a sibling of a Client built with a metrics registry, a connection-lost callback and a connection-loss
  timeout has connected
- **THEN** no metrics poller runs for it, it holds no callback and no timeout, and the registry's series are only the
  parent's
- **AND** the test that verifies this is `TestSiblingHasNoMetricsAndNoLossCallback`

#### Scenario: The parent's Close leaves a sibling open

- **WHEN** the parent's `Close` returns while a sibling is connected
- **THEN** the sibling is still connected until its own `Close`
- **AND** the test that verifies this is `TestParentCloseLeavesSiblingOpen`
