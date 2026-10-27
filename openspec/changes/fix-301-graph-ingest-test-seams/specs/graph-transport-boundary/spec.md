# graph-transport-boundary

## MODIFIED Requirements

### Requirement: graph-ingest does no request work before Start completes

graph-ingest's query and mutation handlers SHALL do no work until `Start` has completed. Until then a request SHALL
get a transient error matching `errs.ErrNotStarted` from the handler, and SHALL read no entity, change no state and
record no mutation outcome or rejection; the requester receives the refusal as a transient error with no code. This
holds for a handler the transport runs during a registration that then fails, which a failed `Start`'s rollback has no
subscription to drain.

#### Scenario: A query during a failed Start

- **WHEN** a batch query for a stored entity reaches graph-ingest's handler after its query subscriptions are
  registered and before `Start` has completed, and `Start` then fails
- **THEN** the handler returns a transient error matching `errs.ErrNotStarted` without reading the entity, the
  rollback leaves nothing held, and a following `Stop` returns nil

#### Scenario: A mutation during a failed Start

- **WHEN** a create mutation reaches graph-ingest's handler after its mutation subscriptions are registered and
  before `Start` has completed, and `Start` then fails
- **THEN** the handler returns a transient error matching `errs.ErrNotStarted`, the entity is not stored, no mutation
  outcome or rejection is recorded, the rollback leaves nothing held, and a following `Stop` returns nil
