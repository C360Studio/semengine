# What step 4 appends to `processor/graph-query/MIGRATION.md`

Draft text for tasks 3.1 to 3.3; the developer writes it as the code lands and corrects it to what the code does.

- Intro: "Steps 1 to 4 are in the tree today".
- "Any caller" table, new row:

| Verb | Change |
| --- | --- |
| `globalSearch` (step 4) | Request keys: `query` (required) and `min_revision`; `level`, `max_communities`, `summarize_threshold`, `include_summaries`, `include_relationships` and `include_sources` are refused by name until step 5, and so is any unknown key. Reply enveloped, was bare: `data` holds `strategy` and one of `entity`, `path`, `temporal`, `spatial`; `entities`, `count`, `duration_ms`, `answer`, `community_summaries`, `degraded` and `degraded_reason` are gone. Strategy names: `pathrag` is `path`, `entity_lookup` is `entity`; `graphrag` and `semantic` are not answered yet. A path answer is the path search's reply (both directions, depth 3, 100 entities), IDs and edges, no entity states: call `batch` for those. A partial ID such as `sensor-001` resolves only as an alias, the suffix lookup is gone. Alias refusals reach you (`alias_ambiguous`, `index_not_ready`, `internal`, `revision_not_reached`). A question with two of a path, a time range and an area is `invalid_request`, as is an impossible number ("the last 0 hours"). Any other question, and a name that resolves to nothing, is refused class invalid, code `text_search_unsupported`, until step 5; do not retry it unchanged. Time-range and area questions refuse transient `internal` until change 7 brings their responders |

- "The changes that fail silently", new bullet: a zone or area question whose start does not resolve used to answer
  an empty path result (#479); it is now refused `text_search_unsupported`, and from step 5 searched as text.
- "New names": "Step 4, in `graph`: `GlobalSearchRequest`, `GlobalSearchData`, `TemporalRangeRequest`,
  `SpatialBoundsRequest`. Package `graphquery` exports nothing new."
- SemSource and SemConnect sections: no change (neither calls `globalSearch`; SemSource's `searchGraph` is step 5's).
