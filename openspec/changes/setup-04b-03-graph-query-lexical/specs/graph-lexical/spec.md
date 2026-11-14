# graph-lexical Delta

## MODIFIED Requirements

### Requirement: A search reply is scoped, limited, ranked and counted

A reply SHALL be a `graph.QueryResponse` from producer `graph-lexical` whose `indexed_revision` is a lower bound on
what it reflects, holding `graph.LexicalSearchData`: `results`, at most `limit` hits ordered by score then entity
ID, each `entity_id` and `score`; `matched`, every match under the scope before the limit; `excluded`, the query's
excluded words, each term's words joined by a space, omitted when there are none; and `bodies_not_indexed`, the number
of entities in the whole index, not the scope or the hits, whose offloaded body is not in the index when the reply is
built.

#### Scenario: The limit's range

- **WHEN** `limit` is 0, 1 to 100, or any other value
- **THEN** 0 means 10, 1 to 100 is used as given, and any other value is refused `invalid_request`
  (`TestSearchLimitRange`)

#### Scenario: A refused query

- **WHEN** `lexical.ParseQuery` refuses the query, here `pump*`
- **THEN** it is refused `invalid_request`, the message naming the rule, here reserved syntax
  (`TestSearchRefusesParseErrors`)

#### Scenario: Excluded words are declared

- **WHEN** the query is `go test -race`
- **THEN** the reply's `excluded` is `["race"]` (`TestSearchDeclaresExcluded`)
