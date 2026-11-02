# graph-query

## ADDED Requirements

### Requirement: Alias refusals reach the caller

The graph query component SHALL pass on `alias_ambiguous`, `index_not_ready` and `internal` from alias resolution as
refusals, and SHALL fall through only on "not found" (ADR-114). The owner's rulings on `inventory.md` add the rest of
this capability.

#### Scenario: A shared alias

- **WHEN** a query names an alias two entities share
- **THEN** the caller receives the `alias_ambiguous` refusal, not "not found"
