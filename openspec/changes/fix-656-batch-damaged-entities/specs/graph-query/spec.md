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

### Requirement: A search reply declares what it left out

A local or global search reply SHALL carry `matched`, the word index's count before the limit; `excluded`, its
excluded words; `bodies_not_indexed`, its count; `damaged`, the hits the batch named damaged, omitted when empty; and
`truncated`, true exactly when `matched` is above the hits returned, a hit is missing or damaged, or matches were left
out to fit `max_payload`.

#### Scenario: A limited answer

- **WHEN** 40 entities match, the limit is 10 and the query is `pump -test`
- **THEN** `matched` is 40, `truncated` is true and `excluded` is `["test"]` (`TestSearchDeclaresWhatItLeftOut`)

### Requirement: A merged search reply declares what it left out

In every searchGraph reply with `embedding_enabled` true, merged or not, `truncated` SHALL be true exactly when the
lists read (both, or the word list alone when the embedding search failed) held more entities than the limit,
`matched` is above the word hits read, a hit is missing or damaged, or matches were left out to fit. A damaged hit
SHALL be named in `damaged` in merged order.

#### Scenario: A limited merged answer

- **WHEN** at tier 1 the word search returns 3 hits, the embedding list holds 12 entities, 2 of them among those 3,
  and the limit is 10
- **THEN** `matched` is 3 and `truncated` is true, since 13 entities were found and 10 sent
  (`TestSearchGraphMergedTruncation`)

#### Scenario: A damaged hit in a merged answer

- **WHEN** at tier 1 a hit found only by the embedding search is named `damaged` by the batch
- **THEN** it is left out, named in `damaged` in merged order, and `truncated` is true
  (`TestSearchGraphMergedNamesDamagedHit`)
