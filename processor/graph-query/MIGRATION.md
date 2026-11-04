# Consumer migration: `processor/graph-query`

For developers of projects moving onto SemEngine's graph-query. It says what keeps working and what you change at your
cutover. The port breaks old shapes where that gives a better result (owner, PR #504 comment 6588079803). Cites to
sister repositories are at SemSource `e4febc0d` and SemConnect `d0d06e00`. Nothing here was run against your code: it
comes from reading it.

`processor/graph-query` is ported in five steps, and this file grows with them. Only step 1 is in the tree today; a
change marked with a later step describes what that step will bring and does not apply yet. Each later step appends
what it changes here.

| Step | What it adds to graph-query |
| --- | --- |
| 1 | The six lookups: `entity`, `batch`, `prefix`, `byName`, `entityByAlias`, `relationships`; `min_revision`; the check for a second responder on a declared subject |
| 2 | Path search and counts (`pathSearch`, `hierarchyStats`, `summary`), and the `semantic`, `similar`, `spatial` and `temporal` forwards |
| 3 | Community reads and local search (`localSearch`) |
| 4 | The `globalSearch` coordinator and its structured strategies |
| 5 | The text tier and `searchGraph`; the package is whole |

## The changes that fail silently

Read these first. Nothing errors.

- **SemSource `searchGraph` (not yet in the tree; arrives with step 5, the text tier).** Search replies gain a wrapper:
  `data`, `indexed_revision`, `producer`, `timestamp`. `graphSearch` (`processor/mcp-gateway/query_tools.go:113`)
  decodes into `graphSearchBody` (`graph_matches.go:107`), which has no `data` field. It ignores the unexpected wrapper
  instead of failing, so you get no error, no matches and no answer.
- **SemSource `deriveDisclosure` (with `searchGraph`, step 5).** It reads the same bytes for top-level fields
  (`disclosure.go:86`), so it reports no answer rather than a failure.
- **Fix for both:** unwrap once before reading, and assert it on a real reply. No framework mechanism does this for
  you.
- **A copy of the GraphQL gateway (already at step 1, more at each later step).** It nests every wrapped answer one
  level too deep; see its section below.
- **A `relationships` decoder that keeps `edge_type` (the `relationships` verb, step 1).** After unwrapping, the field
  reads empty. With a predicate filter, the old fusion client drops every edge (SemStreams `client.go:526`). Change 5's
  port of fusion must not carry this.
- **A `pathSearch` decoder that reads `paths` (the `pathSearch` verb, step 2).** After unwrapping, `paths` is gone, so
  the field reads empty and every entity looks unreachable. Read `edges`, where `edges[i]` first reached
  `entities[i+1]`, and follow them back for a path.
- **A bare `entityByAlias` decoder (the `entityByAlias` verb, step 1).** It reads no entity from the wrapper and gets nil.
- **A `kvRevision` decoder with no zero check (the `entity` verb, step 1).** It gets revision 0. SemConnect's has the
  check, so its failure is loud; see its section.

## SemSource

**Use cases kept:** free-text search (`searchGraph`, which is not in the tree until step 5 lands and changes 5 and 7
bring what it needs); prefix paging (`processor/supersession/prefixquery.go:19`, bare reply, unchanged); your own verbs
on the same family (`sources`, `sourceSummary`, `status`, `predicates`, `versionDiff`), which the verb table does not
reserve.

**What you change:**

- The search decode above.
- Your collision guard. graph-query's second-responder check sees declared ports only. Your graph requests subscribe
  with no port (`processor/source-manifest/component.go:176`, `InputPorts` empty at `:613`), so your old
  `graph.query.summary` collision (`:34-41`) would still pass it. Change
  `TestNoSemSourceSubjectCollidesWithTheSubstrate` (`cmd/semsource/subject_ownership_test.go`) to compare
  `RequestSubjects()` (`source-manifest/component.go:932`, and supersession's) with `graph.QueryVerbs()`, in place of
  its hand list. Declaring your verbs as ports also works. The framework adds no check at start: it would miss
  anything that starts later.
- Nothing in your graph-query config: `ports` alone is valid.
- A slow `searchGraph`, once step 5 lands: your 10 s default (`mcp-gateway/component.go:194-199`) can time out first, loudly.

## SemConnect

**Use cases kept:** exact entity reads and batch hydration (`graph.query.entity`, `graph.query.batch`,
`gateway/cs-api/systems.go:80-81`), both still bare; predicate reads, already enveloped (`:993`); `Register`, for your
backend (`cmd/cs-graph-backend/main.go:127`, on the branch of PR 74).

**What you change:**

- `gateway/cs-api/systems.go:808` decodes `kvRevision`; the key is `kv_revision`. Change the struct tag, or decode
  `graph.ExactEntity` as your ADR-004 § 3 already says. Until then entity reads fail loudly: `:814` turns revision 0
  into fatal `internal`.
- Delete `request_timeout`, `max_traversal_depth`, `max_traversal_nodes` from `deploy/semstreams.json` and
  `conformance/compose.semstreams.config.json`. The old version ignored them; unknown keys now refuse boot, naming the
  key. `query_timeout_ms` is the deadline on each downstream reply (default 15000).
- Keep the `graph-query` port block in `deploy/semstreams.json`: by reading, not run, it is accepted as written
  (`outputs: []` counts as none). Only the three keys above must go.
- Your 5 s default (`gateway/cs-api/config.go:157`) is under ingest's 10 s budget, so a 5 to 10 s `batch` still times
  out on your side, loudly, while graph-query keeps forwarding. Raise it if that matters.

## Whoever keeps the GraphQL gateway

`gateway/graph-gateway` is not ported: the ledger has it as consumer-owned, composed by SemSource but not exercised,
and a copy needs the agentic and `graph/inference` packages. SemSource composes it (`cmd/semsource/run.go:867`) and
documents `/graphql` (`docs/integration/m5-consumer-integration.md:178`); no owner is named after cutover.

**Use case to keep:** GraphQL over the `graph.query.*` verbs, if anyone still wants it.

**What a copy must change:**

- It unwraps by detection (`component.go:2059`), which is not ported, and the old unwrap accepts only `data` and
  `timestamp` (SemStreams `graph/query_contracts.go:89`). The new envelope has four keys, so a copy silently nests
  every wrapped answer one level too deep: `relationships` and `entityByAlias` from step 1, `summary` and the other verbs
  as their steps land. Decode by `graph.QueryVerbs()` instead.
- Its schema advertises `kvRevision` (`:1869`); the key is `kv_revision`.
- Its `relationships` type must read `predicate`, not `edge_type`, and must send `direction`.
- At the old version its alias lookup looks broken: it sends `alias` and the handler read `aliasOrID`, refusing as
  empty (read, not run). The request key is now `alias`, so that mismatch goes.

## Any caller

| Verb | Change |
| --- | --- |
| `entityByAlias` | Request key `alias`, was `aliasOrID`. Reply enveloped, with the entity in `data`. Refusals reach you: `alias_ambiguous`, `index_not_ready`, `internal`, `revision_not_reached`. An unknown alias not shaped like an ID answers `entity_not_found`, not `invalid_request`; this narrows one sentence of ADR-114, which ADR-114 now records |
| `relationships` | Reply enveloped, was a bare list. `direction` is required, `outgoing` or `incoming`. Rows name `predicate`, was `edge_type`. Fusion reads this; change 5 ports fusion here |
| `pathSearch` (step 2) | Request keys: `start_entity`, `direction`, `max_depth`, `max_nodes`, `predicates`, `min_revision`. `direction` is required, `outgoing`, `incoming` or `both`: nothing, or a typo, read as outgoing before. `max_depth` is required, 1 to 10; there is no default. `max_nodes` is 1 to 500, 0 means 100, and it now counts the start: the old limit did not, so the same number gives one entity fewer. `timeout`, `include_siblings` and `max_paths` are gone, and any unknown key is refused. Over a limit is `invalid_request` stating the bound, never a clamp. Reply enveloped: `data` holds `entities`, `edges` and `truncated`. Rows name `entity_id` and `depth`, were `id`, `type` and `score`; `paths` is gone, see "The changes that fail silently". An incoming edge now reads as the graph holds it, `from_entity_id` and `to_entity_id`, not backwards. `truncated` is true only when an entity was left out, not on reaching the limit. Any hop's refusal ends the walk, its message naming that hop's entity; a hop over `max_payload` is `response_too_large` |
| `entity` | Bytes forwarded unchanged, with no re-check by graph-query. The revision key is `kv_revision`, was `kvRevision` |
| `batch`, `prefix`, `byName` | Bytes forwarded unchanged, with no re-check by graph-query |
| `semantic` (step 2) | Forwards to `graph.embedding.query.search`, bytes unchanged, so `scope` reaches the responder (ADR-071). Reply enveloped by the responder, `data` holding `results` and `embedder_type`; `query` and `duration` are gone. Refused transient `internal`, naming that subject, until change 5 brings the responder. Fusion reads this; change 5 ports fusion here |
| `similar` (step 2) | Forwards to `graph.embedding.query.similar`, bytes unchanged. Reply enveloped by the responder. `data.similar` is now `data.results`; the echoed `entity_id` and `duration` are gone. Refused transient `internal` until change 5 |
| `spatial` (step 2) | Forwards to `graph.spatial.query.bounds`, bytes unchanged. Reply enveloped by the responder, was a bare list: `data` holds `results` and `truncated`. Rows name `entity_id`, was `id`, with `lat`, `lon`, `alt`; `type` is gone. Refused transient `internal` until change 7 |
| `temporal` (step 2) | Forwards to `graph.temporal.query.range`, bytes unchanged. Reply enveloped by the responder, was a bare list: `data` holds `results` and `truncated`. Rows name `entity_id`, was `id`; `type` is gone. Request keys become `start_time` and `end_time`, were `startTime` and `endTime`, at change 7, with the responder; until then it is refused transient `internal` |
| Enveloped verbs | May send `min_revision`; an index behind it refuses transient `revision_not_reached`, with the reached revision in detail. Bare verbs ignore it |

A stream capturing a declared `graph.query.*` subject refuses boot (#16), and so does a declared NATS-request input
port overlapping one, unless it belongs to graph-query. An undeclared subscriber is not seen. Decode by the verb table:
`QueryVerb.Enveloped` and `QueryVerb.Reply`.

**New names step 1 exports** (later steps add theirs after these). In `graph`: `ResponderGraphQuery`,
`QueryVerb.Enveloped`, `QueryVerb.Reply`, `MinRevisionField`, `ErrorCodeRevisionNotReached`, `RelationshipEdge`. In
`composition`: `TypeRequestSubjectConflict`. In the new `graphquery` package, the old version's names: `Register`,
`DeclarePorts`, `CreateGraphQuery`, `DefaultConfig`, `Config`, `Component`. Step 2's path search adds, in `graph`:
`PathSearchRequest`, `PathSearchData`, `PathEntity`, `MaxPathSearchDepth`, `MaxPathSearchNodes`, `DefaultPathSearchNodes`.
