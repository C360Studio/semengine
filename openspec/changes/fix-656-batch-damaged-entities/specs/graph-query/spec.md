# graph-query Delta

## MODIFIED Requirements

### Requirement: Matches are the hits read back as stored

Both verbs SHALL read their hits with one `graph.ingest.query.batch` request and answer `matches` in the word index's
order, each `id`, `label`, `score` (the word index's) and `entity`, the bytes the batch sent for it, which graph-query
does not re-encode. In a searchGraph merged list the order is the merged order and `score` is the merged score. A hit
the batch names `damaged` SHALL be left out, named in the reply's `damaged` list in hit order, and set `truncated`;
it SHALL NOT be counted in `missing` or refuse the search. A hit the batch names missing with any other reason SHALL
be left out and counted in `missing`. A batch refusal SHALL reach the caller as it arrived.

#### Scenario: Order and missing hits

- **WHEN** the batch returns the entities in another order and names one hit missing `not_found`
- **THEN** the matches follow the hits' order without it, `missing` is 1 and `truncated` is true
  (`TestSearchMatchesKeepHitOrder`, `TestSearchDropsMissingHits`)

#### Scenario: A damaged hit

- **WHEN** the batch names one hit `damaged`
- **THEN** `localSearch` and `globalSearch` answer the other hits, `damaged` names it, `missing` is 0 and `truncated`
  is true (`TestSearchNamesDamagedHit`)

#### Scenario: Entities pass unchanged

- **WHEN** the batch sends a matching entity holding a value graph-query does not read, such as an integer above 2^53
- **THEN** that entity reaches the caller as the batch sent it (`TestLocalSearchEntitiesPassUnchanged`)
