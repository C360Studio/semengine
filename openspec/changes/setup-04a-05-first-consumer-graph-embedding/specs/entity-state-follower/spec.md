# entity-state-follower

## MODIFIED Requirements

### Requirement: The envelope

The follower's envelope SHALL be `ComputeIndexStatus` over the floor, the commit time `IndexedAt` returns with it and
the bucket's last sequence, with these overrides: a target of 0 after the snapshot marker reads ready; a `Failed()`
count above zero reads not ready, `degraded`, and is carried in `failed_count`, with the per-reason counts the same
`Failed()` call returned in `failed_reasons`; a failed target read reads `degraded`; a latched reset reads
`reset_required` with its reason; while the floor is behind the target and no `Complete` call has arrived for 30 s,
the envelope reads `degraded`, and a held low revision with later completions within 30 s does not. `Failed()` SHALL
return a map the follower owns (the plug-in keeps no reference to
it). The envelope SHALL NOT carry `first_failure_at`.

#### Scenario: Unresolved writes

- **WHEN** `Failed()` returns 2 and the floor has reached the target
- **THEN** the envelope reads `ready` false, state `degraded`, `failed_count` 2

#### Scenario: Failures with reasons

- **WHEN** `Failed()` returns count 3 and reasons `{"timeout": 2, "delete_failed": 1}` while outcomes are reported
  concurrently
- **THEN** the published envelope has `failed_count` 3 and those `failed_reasons`, under `-race`
  (`TestEnvelopeCarriesFailedReasons`)

#### Scenario: A plug-in with no reasons

- **WHEN** `Failed()` returns a count and `nil` reasons, as graph-index does
- **THEN** the envelope has no `failed_reasons` (`TestGraphIndexEnvelopeUnchanged`)

#### Scenario: A held low revision while later revisions complete

- **WHEN** one revision is not completed for more than 30 s while later revisions keep completing
- **THEN** the envelope reads `ready` false and is not `degraded` (`TestHeldLowRevisionWithProgress`)

#### Scenario: Nothing completes

- **WHEN** the floor is behind and no revision completes for more than 30 s
- **THEN** the envelope reads `degraded` (`TestNoCompletionsReadsDegraded`)
