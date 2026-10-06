# projection-mutation

## ADDED Requirements

### Requirement: Conditional reconcile at a caller-observed revision

`projection.ReconcileMutation` SHALL carry an expected revision, sent through the typed `MutationClient`. When it is
set, graph-ingest SHALL apply the mutation only if that revision is the entity's current revision, and otherwise
SHALL return a revision-conflict error naming the expected and the current revision and SHALL change nothing.

#### Scenario: Concurrent update between read and reconcile

- **WHEN** the caller read revision R, another writer committed R+1, and the caller reconciles at R
- **THEN** the result is a revision conflict naming R and R+1, and the entity is unchanged at R+1

#### Scenario: Unchanged revision

- **WHEN** the caller reconciles at the current revision R
- **THEN** the mutation is applied and the receipt reports it verified at the new revision

### Requirement: Commit ambiguity is preserved

graph-ingest SHALL classify a mutation failure as not committed only when the failure is proven to precede any
storage effect: an invalid request, a revision conflict, or an entity not found. Every other failure, including a
key-value write whose outcome the server did not confirm, SHALL reach the caller of the typed client as
commit-unknown.

#### Scenario: Backend write timeout

- **WHEN** the entity bucket's update reaches the server and its reply is lost or times out
- **THEN** the typed client's receipt reports commit-unknown, never not-committed

#### Scenario: Revision conflict

- **WHEN** the entity bucket refuses the update for a revision mismatch
- **THEN** the receipt reports not-committed with the conflict classification
