# operator-surface delta: refactor-362-remove-migration-overrides

## MODIFIED Requirements

### Requirement: The example storage alert rules

The example alert rules at `configs/prometheus/rules/storage-pressure.yml` SHALL read only published metric names, SHALL
NOT fire on a report row disappearing, SHALL name unbounded resources, and SHALL state that nothing is enforced.

#### Scenario: The example alert rules read only published names

- **WHEN** the example rules file is parsed
- **THEN** every `semengine_storage_*` name an alert reads is one the service publishes, no alert fires on a series
  disappearing, unbounded storage is named, every alert has a summary, a description and a `for` window, and at least
  one description states that the report is report-only
- **AND** the tests that verify this are `TestStorageAlertRules_ReferenceOnlyPublishedMetrics`,
  `TestStorageAlertRules_KeyNothingOnRowDisappearance`, `TestStorageAlertRules_MakeUnboundedResourcesVisible` and
  `TestStorageAlertRules_AreAnnotatedAndReportOnly`
