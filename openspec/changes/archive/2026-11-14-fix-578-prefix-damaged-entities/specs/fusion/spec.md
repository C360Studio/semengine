# fusion Delta

## MODIFIED Requirements

### Requirement: A fatal seed read fails the call

When `Resolve` or `Entities` returns an error `errs.IsFatal` reports, such as `graph_state_reset_required` from a
reset, `Fuse` SHALL return that error and no response. `Fuse` SHALL refuse a nil context. A poisoned seed does not
reach `Fuse` as an error from `fusionnats`, which lists it as unhydrated (requirement "A poisoned seed is
unhydrated, not a failed read"). Nor does a damaged entity under a resolved prefix (requirement "A prefix resolve
lists IDs, then reads them").

#### Scenario: A reset while hydrating seeds

- **WHEN** `Resolve` answers and `Entities` returns `graph_state_reset_required`
- **THEN** `Fuse` returns that error (`TestFatalReplyReturnsError`)

## ADDED Requirements

### Requirement: A prefix resolve lists IDs, then reads them

`fusionnats`' prefix `Resolve` SHALL send the query as `prefix`, with the resolve limit, to graph-query's
`prefixIDs` row. It SHALL refuse the resolve when `graph.ValidateDecodedEntityIDs` refuses a listed ID, and SHALL
return one seed per listed ID, in listed order, with no score. A damaged entity under the prefix therefore reaches
hydration as a seed.

#### Scenario: A damaged entity under the prefix

- **WHEN** a real graph-ingest holds two entities and one damaged value under a prefix, and `Fuse` runs a prefix lens
  over it
- **THEN** the response holds the two entities and lists the damaged one in `unhydrated` with reason `error`
  (`TestDamagedEntityUnderPrefixOverRealResponders`)

#### Scenario: The listing's subject

- **WHEN** `Resolve` runs in prefix mode on a recording requester
- **THEN** it sends one request, to the subject of graph-query's `prefixIDs` row, and returns the listed IDs as seeds
  in order
