# graph-query Delta

## MODIFIED Requirements

### Requirement: A search reply declares what it left out

A local or global search reply SHALL carry `matched`, the word index's count before the limit; `excluded`, its
excluded words; `bodies_not_indexed`, its count; `damaged`, the hits the batch named damaged, omitted when empty; and
`truncated`, true exactly when `matched` is above the hits returned, a hit is missing or damaged, matches were left
out to fit `max_payload`, or a reply it was built from carried `damaged_entities` above 0.

#### Scenario: A limited answer

- **WHEN** 40 entities match, the limit is 10 and the query is `pump -test`
- **THEN** `matched` is 40, `truncated` is true and `excluded` is `["test"]` (`TestSearchDeclaresWhatItLeftOut`)

#### Scenario: The word index left a damaged entity out

- **WHEN** graph-lexical answers with every hit read back and `damaged_entities` 1
- **THEN** the reply carries `damaged_entities` 1 and `truncated` true (`TestSearchTruncatedWhileDamaged`)

### Requirement: A merged search reply declares what it left out

In every searchGraph reply with `embedding_enabled` true, merged or not, `truncated` SHALL be true exactly when the
lists read (both, or the word list alone when the embedding search failed) held more entities than the limit,
`matched` is above the word hits read, a hit is missing or damaged, matches were left out to fit, or a reply it was
built from carried `damaged_entities` above 0. A damaged hit SHALL be named in `damaged` in merged order.

#### Scenario: A limited merged answer

- **WHEN** at tier 1 the word search returns 3 hits, the embedding list holds 12 entities, 2 of them among those 3,
  and the limit is 10
- **THEN** `matched` is 3 and `truncated` is true, since 13 entities were found and 10 sent
  (`TestSearchGraphMergedTruncation`)

#### Scenario: A damaged hit in a merged answer

- **WHEN** at tier 1 a hit found only by the embedding search is named `damaged` by the batch
- **THEN** it is left out, named in `damaged` in merged order, and `truncated` is true
  (`TestSearchGraphMergedNamesDamagedHit`)

#### Scenario: The embedding index left a damaged entity out

- **WHEN** at tier 1 both searches read back every hit and the embedding reply carries `damaged_entities` 2
- **THEN** the reply carries `damaged_entities` 2 and `truncated` true (`TestSearchTruncatedWhileDamaged`)
