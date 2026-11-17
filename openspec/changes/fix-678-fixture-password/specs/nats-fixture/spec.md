# nats-fixture Delta

## ADDED Requirements

### Requirement: Broker refuses a client without its password

Start SHALL start each broker with a random password, and `URL()` SHALL carry it in the URL's user part, which
nats.go sends as the connection's token. A Restart SHALL keep the same password. A client that dials the broker's
host port without the password SHALL be refused, so a client from another test process that redials a port this
broker was later handed cannot join it.

#### Scenario: A client without the password is refused

- **WHEN** a client dials the host port in `URL()` without the URL's user part
- **THEN** the connection fails with `nats.ErrAuthorization`, and a client dialling `URL()` itself connects
