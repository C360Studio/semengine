# What step 4 appends to `processor/graph-query/MIGRATION.md`

Draft text for tasks 3.1 to 3.3; the developer writes it as the code lands and corrects it to what the code does.

- Intro: "Steps 1 to 4 are in the tree today".
- "Any caller" table, new row:

| Verb | Change |
| --- | --- |
| `globalSearch` (step 4) | Request keys: `query` (required) and `min_revision`; `level`, `max_communities`, `summarize_threshold`, `include_summaries`, `include_relationships` and `include_sources` are refused by name until step 5, and so is any unknown key. Reply enveloped, was bare: `data` holds `strategy` and one of `entity`, `path`, `temporal`, `spatial`; `entities`, `count`, `duration_ms`, `answer`, `community_summaries`, `degraded` and `degraded_reason` are gone. Strategy names: `pathrag` is `path`, `entity_lookup` is `entity`; `graphrag` and `semantic` are not answered yet. A path answer is the path search's reply (both directions, depth 3, 100 entities), IDs and edges, no entity states: call `batch` for those. A partial ID such as `sensor-001` resolves only as an alias, the suffix lookup is gone. Alias refusals reach you (`alias_ambiguous`, `index_not_ready`, `internal`, `revision_not_reached`). A question with two of a path, a time range and an area is `invalid_request`, as is an impossible number ("the last 0 hours"). A question whose path, time range or area comes with a count ("how many"), a ranking ("top 5") or a similarity ("similar to") is not answered by that strategy any more (the old version answered the path or time part and dropped the rest). A six-part ID is now asked of the alias index first; the old version took it as itself. A one-word question is always an entity reference, read before any other meaning, so an ID or alias holding a word such as `today`, `zone-a`, `link`, `max` or `most` is looked up, not read as a time range, path, count or ranking. A lone `today` or `yesterday` that names no entity is refused `text_search_unsupported`, never as a time range. In a longer path question the start is whatever word with a hyphen the classifier finds, never a whole ID: "what is connected to acme.plant.robotics.gcs.drone-x.001" walks from the alias `drone-x`, and a full ID with no hyphenated segment gives no start and is refused `text_search_unsupported` below, or `invalid_request` naming both places beside a zone or area; name the start by alias. Only one start is read: "from A to B" walks from A, a limit of the keyword classifier tracked in #535; a question naming an entity and a zone or area ("connected to sensor-001 near zone-b", "show sensor-001 in zone-b"), or two different zones or areas however spelled ("zone a in zone b"), is refused `invalid_request`, naming both places. Any other question, and a name or ID that names no entity, is refused class invalid, code `text_search_unsupported`, until step 5; do not retry it unchanged. Time-range and area questions refuse transient `internal` until change 7 brings their responders |

- "Any caller", `globalSearch` row: a zone or area question whose start does not resolve is refused
  `text_search_unsupported` (#479), and from step 5 searched as text. A started pin component passed it on to its
  community search (pin `component.go:556` builds the cache before the responders, so `graphrag.go:475-489` falls
  through); the empty `pathrag` answer ran only without a cache, outside a successful `Start`. A refusal errors, so it
  is not a silent change.
- "New names": "Step 4, in `graph`: `GlobalSearchRequest`, `GlobalSearchData`, `TemporalRangeRequest`,
  `SpatialBoundsRequest`. Package `graphquery` exports nothing new."
- SemSource and SemConnect sections: no change (neither calls `globalSearch`; SemSource's `searchGraph` is step 5's).
