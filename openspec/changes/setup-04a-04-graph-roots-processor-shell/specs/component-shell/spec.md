# component-shell

## ADDED Requirements

### Requirement: The shell admits one Start after Initialize

`component.Shell`'s `Initialize` SHALL return nil every time and do no I/O. A `Start` before `Initialize` SHALL return
an error and leave the shell unused, so that a `Start` after `Initialize` is admitted. After `Initialize`, `Start` and
`Stop` SHALL follow the lifecycle suite's portable floor and the retained failed-start cleanup rule.

#### Scenario: Start before Initialize

- **WHEN** `Start` is called on a shell that was never initialized, then `Initialize` and `Start` are called
- **THEN** the first `Start` returns an error and runs nothing, and the second is admitted
- **AND** the test that verifies this is `TestShellStartBeforeInitializeIsRefusedAndLeavesItUnused`

#### Scenario: A shell-only owner passes the suite

- **WHEN** the lifecycle suite runs over an owner built only from a shell and one worker
- **THEN** every check passes, the failed-start check on a factory whose start fails
- **AND** the test that verifies this is `TestShellOwnerPassesTheLifecycleSuite`

### Requirement: The shell is running from start's return until cleanup begins

A shell SHALL be running from the moment the `start` function given to `Start` returns nil until the `cleanup`
function begins, whether for a `Stop` or a failed start's rollback. A `start` that fails or panics SHALL never make the
shell running.

#### Scenario: Reads during Stop

- **WHEN** `Health` is called while the cleanup of a `Stop` is held open
- **THEN** it reports `Healthy` false and `Status` `stopped`
- **AND** the test that verifies this is `TestShellReadsDuringStopReportStopped`

#### Scenario: A failed start

- **WHEN** the `start` given to `Start` returns an error
- **THEN** no read taken during or after that `Start` reports running
- **AND** the test that verifies this is `TestShellFailedStartNeverRuns`

### Requirement: Health reads running and the stuck states it is given

A shell's `Health(stuck...)` SHALL report: when not running, `Healthy` false, `Status` `stopped`, `Uptime` zero and no
`LastError`; when running with no stuck state, `Healthy` true and `Status` `running`; when running with one or more
non-empty stuck states (an empty string is ignored), `Healthy` false, `Status` `degraded` and `LastError` the states
joined by `"; "`. `ErrorCount` SHALL be the number of `CountError` calls and SHALL never lower `Healthy`.

#### Scenario: Every state

- **WHEN** `Health` is read in each state, with and without stuck states, with and without counted errors
- **THEN** each field matches the rule above
- **AND** the test that verifies this is `TestShellReadsInEveryPhase`

### Requirement: DataFlow reports counted inputs and their rates

A shell's `DataFlow` SHALL report as `Messages` and `Bytes` the `CountInput` totals since construction, running or
not; as `MessagesPerSecond` and `BytesPerSecond` those totals over uptime while running, else zero; as `ErrorRate`
counted errors per counted input while running with at least one input, else zero; and as `LastActivity` the time of
the last `CountInput`, or the zero time before the first.

#### Scenario: Totals and rates while running and after Stop

- **WHEN** inputs and errors are counted on a running shell and it is then stopped
- **THEN** the totals equal what was counted throughout, and the rates are above zero while running and zero after
- **AND** the test that verifies this is `TestShellReadsInEveryPhase`

#### Scenario: graph-ingest's exact counts

- **WHEN** graph-ingest receives a known number of stream messages and mutation requests
- **THEN** `DataFlow().Messages` and `DataFlow().Bytes` equal the test's own tally
- **AND** the test that verifies this is `TestIntegration_DataFlowCountsInputMessages`

### Requirement: The shell's reads never wait

A shell's `Health` and `DataFlow` SHALL return without waiting for a `Start` or `Stop` in progress, at any time.

#### Scenario: Reads while start and cleanup are held open

- **WHEN** both reads are called while `start` is held open, then while `cleanup` is held open
- **THEN** each read returns
- **AND** the test that verifies this is `TestShellReadsDoNotWaitForStartOrStop`

### Requirement: The shell holds no context

A shell SHALL hold no `context.Context` and no `context.CancelFunc` and SHALL create neither. The component's `start`
creates the run context for its continuing work and its `cleanup` ends it (ADR-113), so a component whose start runs
continuing work under the Start context fails the lifecycle suite.

#### Scenario: Work under the Start context is caught

- **WHEN** a shell owner's `start` runs its worker under the Start context
- **THEN** the suite's check that ending the Start context stops nothing fails for it
- **AND** the test that verifies this is `TestShellSuiteCatchesWorkUnderTheStartContext`
