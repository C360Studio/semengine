# fusion Delta

## MODIFIED Requirements

### Requirement: A fatal seed read fails the call

When `Resolve` or `Entities` returns an error `errs.IsFatal` reports, such as `graph_state_reset_required` from a
reset, `Fuse` SHALL return that error and no response. `Fuse` SHALL refuse a nil context. A poisoned seed
does not reach `Fuse` as an error from `fusionnats`, which lists it as unhydrated (requirement "A poisoned seed is
unhydrated, not a failed read").

#### Scenario: A reset while hydrating seeds

- **WHEN** `Resolve` answers and `Entities` returns `graph_state_reset_required`
- **THEN** `Fuse` returns that error (`TestFatalReplyReturnsError`)

## ADDED Requirements

### Requirement: A damaged entity on a prefix page is a seed

`fusionnats`' prefix resolve SHALL seed the page's entities and each ID its `missing` list names, in ID order, so a
damaged entity reaches hydration and is listed unhydrated `error`.

#### Scenario: A prefix page with one damaged entity

- **WHEN** the prefix reply holds A and C and lists B missing with reason `error`
- **THEN** `Fuse` answers with A and C and lists B unhydrated `error`
