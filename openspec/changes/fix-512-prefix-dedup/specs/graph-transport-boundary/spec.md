# Delta: graph-transport-boundary

## MODIFIED Requirements

### Requirement: A prefix page names its total

A `graph.ingest.query.prefix` reply SHALL carry `total`, the number of distinct keys in the request's key listing,
before the cursor and before any limit, on every path. It SHALL be counted in the byte fit. It is true as listed. A
key the listing repeats SHALL appear once in `total` and at most once across pages. Changed: a repeated key was counted
and served twice (#512).

#### Scenario: A later page

- **WHEN** a group of N entities is read on its second page, or on a page the byte fit trimmed
- **THEN** `total` is N (`TestPrefixPageNamesItsTotal`)

#### Scenario: The listing repeats a key

- **WHEN** the key listing holds one key twice
- **THEN** the page holds that entity once, and `total` counts it once
