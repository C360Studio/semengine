# composed-stack Delta

## ADDED Requirements

### Requirement: A warm restart over a large store starts with the documented max_pending

A composed stack restarted over a retained store SHALL start, and every graph reader SHALL report its snapshot
applied, on a NATS server with `max_pending` set to the value `docs/boot.md` requires (536870912). No connection of
the stack SHALL be closed by the server as a slow consumer during the boot.

#### Scenario: Restart over 132 MB of entity state

- **WHEN** the stack is seeded with 132 MB in `ENTITY_STATES`, stopped, and started again on a server with
  `max_pending: 536870912`
- **THEN** `Start` returns nil, every follower and the websocket view report the snapshot applied, and the account's
  slow-consumer count on the server is the same after the restart as before it
- **AND** the test that verifies this is `TestSmoke_WarmRestartOverALargeStore`
