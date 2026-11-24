# transport-client Delta

## ADDED Requirements

### Requirement: A Client opens a sibling with its settings

`Client.NewSibling` SHALL return a Client that is not connected and carries the parent's server URLs, credentials,
name, timeouts and reconnect settings. The sibling SHALL register no metrics and SHALL carry no connection-lost
callback or connection-loss timeout. Once the parent's `Close` has begun, `NewSibling` SHALL return an error matching
`nats.ErrConnectionClosed`. The sibling SHALL promise nothing about timing beyond what any Client promises.

#### Scenario: A sibling reaches the parent's server with the parent's credentials

- **WHEN** a sibling of a Client built with credentials for a password-protected server calls `Connect`
- **THEN** it connects, and the server lists it under the parent's name
- **AND** the test that verifies this is `TestSiblingCarriesTheClientSettings`

#### Scenario: A sibling leaves the parent's metrics and exit policy alone

- **WHEN** a sibling of a Client built with a metrics registry and a connection-lost callback loses its connection
- **THEN** nothing is registered on the registry for the sibling, and the callback does not run
- **AND** the test that verifies this is `TestSiblingHasNoMetricsAndNoLossCallback`

#### Scenario: No sibling once Close has begun

- **WHEN** `NewSibling` is called after the parent's `Close` has begun
- **THEN** it returns an error matching `nats.ErrConnectionClosed`
- **AND** the test that verifies this is `TestSiblingRefusedAfterClose`
