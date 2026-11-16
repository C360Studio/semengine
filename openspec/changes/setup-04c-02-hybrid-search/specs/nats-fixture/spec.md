# nats-fixture Delta

## MODIFIED Requirements

### Requirement: Own server per test

`natsfixture.New(t, opts...)` SHALL give its caller its own NATS server: Start starts it, Restart and Stop act on it,
and `URL()` and `JetStream()` reach it. `t` is a `natsfixture.TB` (`Helper`, `Errorf`, `Logf`, `Cleanup`): a test
passes its `*testing.T`, and a harness program run by Task passes its own. New SHALL make no Docker call. A second
Start SHALL return `ErrAlreadyUsed`. The fixture's methods are called from the caller's own goroutine. The fixture
SHALL retain no `context.Context`.

#### Scenario: A second Start is refused

- **WHEN** Start is called on a fixture whose Start already succeeded
- **THEN** it returns `ErrAlreadyUsed`

### Requirement: Own server cleanup

`natsfixture.New` SHALL register through `t.Cleanup` a Stop under a fresh 60-second context whose failure is
reported through `t.Errorf`. Stop SHALL close the fixture's connection and remove the container, and SHALL return nil
only when Docker reported the removal; a Stop after a successful Stop SHALL return nil and make no Docker call.

#### Scenario: Container gone after the test

- **WHEN** a test that started its own server ends
- **THEN** Docker lists no container with that server's container ID

#### Scenario: Repeated Stop

- **WHEN** Stop is called again after it succeeded
- **THEN** it returns nil and makes no Docker call
