# nats-fixture

## MODIFIED Requirements

### Requirement: Broker max payload is settable

A test SHALL be able to start the fixture with a broker `max_payload` it chooses; with none set, the broker default
SHALL apply.

#### Scenario: Publish above the set limit

- **GIVEN** a fixture started with max payload N
- **WHEN** a client publishes more than N bytes
- **THEN** the publish is refused with `nats.ErrMaxPayload` (nats.go checks the limit the broker announced before it
  sends), and a message of at most N bytes is accepted
