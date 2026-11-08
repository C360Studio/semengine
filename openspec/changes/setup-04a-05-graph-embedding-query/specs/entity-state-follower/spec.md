# entity-state-follower

## ADDED Requirements

### Requirement: A plug-in may narrow which failures withhold queries

`follower.Config` SHALL have an optional `Withhold func() int64`. When it is set, `QueryReady` SHALL refuse transient
`index_not_ready` for failures only while `Withhold` returns above 0; when it is nil, `QueryReady` SHALL use the count
`Failed` returns, as before. The envelope and `Stuck` SHALL keep reading `Failed`.

#### Scenario: A failure the plug-in does not withhold on

- **WHEN** `Failed` returns count 1 and `Withhold` returns 0 after the first build
- **THEN** `QueryReady` returns nil, while the envelope reads `failed_count` 1 and `Stuck` names it
  (`TestWithholdNarrowsQueryReady`)

#### Scenario: No Withhold

- **WHEN** `Withhold` is nil and `Failed` returns count 1
- **THEN** `QueryReady` refuses transient `index_not_ready`, as for graph-index (`TestGraphIndexWithholdUnchanged`)
