# keyed-dispatch

## ADDED Requirements

### Requirement: Wait joins the lanes once Shutdown has begun

`KeyedPool.Wait()` SHALL return once every lane has exited and SHALL take no context. Lanes exit only after
`Shutdown` has begun and every accepted item has been processed, so a `Wait` called before `Shutdown` SHALL block
until then, and a `Wait` called after the lanes exited SHALL return at once.

#### Scenario: Shutdown under an ended context, then Wait

- **WHEN** `Wait` is waiting, then `Shutdown` is called with an ended context while an accepted item is still queued
- **THEN** `Shutdown` returns the context's error, and `Wait` returns only after that item was processed and every
  lane exited
- **AND** the test that verifies this is `TestKeyedPoolWaitJoinsAfterShutdown`
