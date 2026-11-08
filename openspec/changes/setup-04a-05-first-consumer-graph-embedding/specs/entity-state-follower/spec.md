# entity-state-follower

## ADDED Requirements

### Requirement: The envelope carries the plug-in's failure reasons

The follower SHALL read the plug-in's failed count and per-reason counts in one `Config.Failed` call per envelope, and
SHALL set `failed_reasons` to those counts when the count is above zero and leave it absent otherwise. It SHALL NOT set
`first_failure_at`.

#### Scenario: Failures with reasons

- **WHEN** `Failed` returns count 3 and reasons `{"timeout": 2, "delete_failed": 1}`
- **THEN** the published envelope has `failed_count` 3, those `failed_reasons`, and no `first_failure_at`
  (`TestEnvelopeCarriesFailedReasons`)

#### Scenario: A plug-in with no reasons

- **WHEN** `Failed` returns a count and `nil` reasons, as graph-index does
- **THEN** the envelope has no `failed_reasons` (`TestGraphIndexEnvelopeUnchanged`)
