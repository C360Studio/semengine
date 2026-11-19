# fusion Delta

## ADDED Requirements

### Requirement: Every graph read passes on the damaged count

`fusionnats` SHALL copy a reply's `damaged_entities` into what each `RetrievalClient` read returns: `Resolution`'s
`DamagedEntities`, and the `DamagedEntities` of the `fusion.Neighbors` and `fusion.Names` values that `Neighbors` and
`Names` return in place of `[]Edge` and `[]string`.

#### Scenario: A relationships reply with a count

- **WHEN** the relationships reply carries `damaged_entities` 2 with one edge
- **THEN** `Neighbors` returns that edge and `DamagedEntities` 2 (`TestReadsPassOnDamagedCount`)

### Requirement: A response counts the damaged entities its reads left out

Every response SHALL carry `damaged_entities`, the largest count any `RetrievalClient` read its answer used reported,
omitted at 0: on nodes, a miss, or an `all_seeds_unhydrated` deferral, whose unhydrated seeds the resolve found. An
`index_not_ready` deferral answers nothing and SHALL carry none. It SHALL NOT set `truncated`, which says the budget
cut nodes off.

#### Scenario: A walk read reports damage

- **WHEN** `Resolve` reports 0 and a relations `Neighbors` read reports 1
- **THEN** the response carries `damaged_entities` 1 and `truncated` is unchanged (`TestResponseCarriesDamagedCount`)

## MODIFIED Requirements

### Requirement: A fatal seed read fails the call

When `Resolve` or `Entities` returns an error `errs.IsFatal` reports, such as `graph_state_reset_required` from a read
of a damaged entity, `Fuse` SHALL return that error and no response. `Fuse` SHALL refuse a nil context. A damaged seed
does not reach `Fuse` as an error from `fusionnats`, which lists it as unhydrated (requirement "A damaged seed is
unhydrated, not a failed read"). Nor does a damaged entity under a resolved prefix (requirement "A prefix resolve lists
IDs, then reads them").

#### Scenario: A reset while hydrating seeds

- **WHEN** `Resolve` answers and `Entities` returns `graph_state_reset_required`
- **THEN** `Fuse` returns that error (`TestFatalReplyReturnsError`)
