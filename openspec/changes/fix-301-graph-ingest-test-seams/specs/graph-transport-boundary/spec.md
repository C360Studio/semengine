# graph-transport-boundary

## ADDED Requirements

### Requirement: graph-ingest refuses request work until Start completes

graph-ingest's query and mutation handlers SHALL do no work until `Start` has completed. Until then a request SHALL
get a transient error matching `errs.ErrNotStarted` from the handler, and SHALL read no entity, change no state and
record no mutation outcome or rejection; the requester receives the refusal as a transient error with no code.

#### Scenario: A query before Start completes

- **WHEN** a batch query for a stored entity reaches graph-ingest's handler after its query subscriptions are
  registered and before `Start` has completed
- **THEN** the handler returns a transient error matching `errs.ErrNotStarted` without reading the entity
- **AND** once `Start` has completed, the same query is answered

#### Scenario: A mutation before Start completes

- **WHEN** a create mutation reaches graph-ingest's handler after its mutation subscriptions are registered and
  before `Start` has completed
- **THEN** the handler returns a transient error matching `errs.ErrNotStarted`, the entity is not stored, and no
  mutation outcome or rejection is recorded
- **AND** once `Start` has completed, the same mutation stores the entity

## REMOVED Requirements

### Requirement: graph-ingest does no request work before Start completes

**Reason**: Its two scenarios had a request arrive during a registration that then fails, a test-only fault the owner
retired with the test field that injected it (#301 comment 6453844541). OpenSpec's archive refuses a modified
requirement that drops a scenario, so the requirement is re-added above as "graph-ingest refuses request work until
Start completes", with the same text less the sentence on a registration that then fails (natsclient behaviour, proved
by `natsclient/subscribe_registration_test.go`), and scenarios in which the request arrives before `Start` completes and
`Start` then completes.
