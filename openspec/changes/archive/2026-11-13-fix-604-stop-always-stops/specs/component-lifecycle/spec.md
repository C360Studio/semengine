# component-lifecycle Delta

## MODIFIED Requirements

### Requirement: The shell is running from start's return until cleanup begins

A shell SHALL be running from the moment the `start` function given to `Start` returns nil until the `cleanup`
function begins, whether for a `Stop` (under a live or already-ended non-nil context) or a failed start's rollback. A
`start` that fails or panics SHALL never make the shell running.

#### Scenario: Reads during Stop

- **WHEN** `Health` is called while the cleanup of a `Stop` is held open
- **THEN** it reports `Healthy` false and `Status` `stopped`
- **AND** the test that verifies this is the `stop cleanup held open` row of `TestLifecycleReadsInEveryPhase`

#### Scenario: Stop with an ended context begins cleanup

- **WHEN** `Stop` is called on a running shell with an already-cancelled context
- **THEN** `cleanup` runs, `Health` reports `Healthy` false and `Status` `stopped`, `Stop` returns `context.Canceled`,
  and a subsequent `Stop` with a live context returns nil once `cleanup` succeeds
- **AND** the test that verifies this is `TestLifecycleStopWithEndedContextBeginsCleanup`

#### Scenario: A failed start

- **WHEN** the `start` given to `Start` returns an error
- **THEN** no read taken during or after that `Start` reports running
- **AND** the test that verifies this is `TestLifecycleFailedStartNeverRuns`
