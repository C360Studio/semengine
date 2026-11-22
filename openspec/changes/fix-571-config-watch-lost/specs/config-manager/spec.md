# config-manager Delta

## ADDED Requirements

### Requirement: A watch that ends outside Stop is reported

When a configuration watch's updates end while the manager's run context is live, the manager SHALL log an error
naming the watch and SHALL report itself unhealthy from `Health()`, with a message naming each lost watch and saying
to restart the process, until the process restarts. A watch ended by `Stop` SHALL be neither logged nor reported.

#### Scenario: The config watch's updates close

- **WHEN** a started manager's config watch's updates channel closes and `Stop` has not been called
- **THEN** an error is logged naming the watch, and `Health()` is unhealthy with a message containing
  `restart the process`

#### Scenario: Stop ends the watches

- **WHEN** a started manager is stopped
- **THEN** no watch-lost error is logged, and `Health()` is healthy
