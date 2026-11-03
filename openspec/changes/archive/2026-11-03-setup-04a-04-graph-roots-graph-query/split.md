# Split: `processor/graph-query`

The port, re-cut after the rulings on PR #504. Line figures are hand estimates of what each step holds, production
plus tests, not measurements; the owner's "line count is a guide" applies (#505). Each step compiles, is tested and
passes `task verify`. A later verb is not in the table until its step, so on a default server a request for it reports
no responders, and never gets a wrong answer.

| Step | PR | Purpose | Delivers | Depends on | Retires | Lines (prod + test) |
| --- | --- | --- | --- | --- | --- | --- |
| 1 Direct lookups | #504 | First answers on `graph.query.*`; settles the table, the envelope and `min_revision` | Shell; table marks; `entity`, `batch`, `prefix`, `byName` forwards; `entityByAlias`; `relationships`; `min_revision` in graph-index; second-responder finding; ADR-060 | none | none | 700 + 1,200 |
| 2 Traversal and counts | new draft | Everything else that answers without a community or the classifier | `pathSearch` with hard limits and validators; `hierarchyStats` and `summary` with truncation and predicate state declared; `semantic`, `similar`, `spatial`, `temporal` forwards; ADR-071 | 1 | none | 750 + 1,300 |
| 3 Community reads | new draft | One honest read of `COMMUNITY_INDEX`, and local search over it | `graphview.View[clustering.Community]` per ruling 3; poison-counting list in `pkg/graphview`; exported key helper in `graph/clustering`; one page per group per ruling 2; `localSearch`, `entities_truncated`, truncation metric; entity loading, labels, digests, scoring; plain-function answer; ADR-087 note | 1 (and 2 for the embedding subject's table row, if local search keeps its semantic call, `pin:graphrag.go:308`) | #406, #31 | 900 + 1,100 |
| 4 Coordinator and structured strategies | new draft | Questions that need no text tier | Classifier call with its refusal; `globalSearch` coordinator with typed strategies (Q5(b)); entity, path, temporal, spatial, semantic strategies; resolver | 2, 3 | #405, #479 | 700 + 700 |
| 5 Text tier and `searchGraph` | new draft | Finishes the package and SemSource's one search call | Text tier, one page per group, five groups at most; digests, enrichment, sources; `searchGraph`; last table rows | 3, 4 | #106, #103 (see below) | 900 + 900 |

Not carried: language-model paths and `summary_view.go` (#485-#487, #32), owner-proof tests (`inventory.md` § 9).
Added to that by the rulings: `community_cache.go`, generation, lease and fence (431 + 538 tests), `router.go` and its
tests (432), `AnswerSynthesizer`, the relay validators (`pin:query.go:299-345`). The table sums to about 8,500 lines,
against the inventory's 14,328 carried; both count differently, so treat that as a direction only.

## What changed from the inventory's table, and why

| Inventory | Now | Why |
| --- | --- | --- |
| 9.1, 3,825 lines | Step 1, about 1,900 | Thin forward removes validators and router (owner, Q3); the shell replaces hand lifecycle; no metrics have readers. The 9.1a/9.1b split is unneeded |
| 9.3, split a/b over the cap | Step 3 whole | The cache and its machinery go (ruling 3), so no separate stepping stone is left; `pkg/graphview` and `clustering` changes sit with their reader |
| 9.3's 10,000-entity member load | One page per group | Ruling 2 |
| 9.4 coordinator then 9.5 strategies, with a declared fallback | Step 4 coordinator and strategies, step 5 text tier | See "Stepping stones". The old order made a question with a time range lose its filter until 9.5 |
| `searchGraph` last, alone | Still last, with the text tier it needs | It delegates to global search; moving it earlier gains nothing, since it refuses until changes 5 and 7 |
| `AnswerSynthesizer`, `LLMAnswerSynthesizer` split | Plain function in step 3 | Owner, Q6 |
| `min_revision` "if ruled" | Step 1, in graph-index too | Ruling 1 |
| Cap dependency, second #500 PR gates | None | The cap is now a guide (owner); #501 and #505 merged (`f7528d9`, `e61ce55`) |

## Stepping stones and what each step answers

| After step | Answers on merge | Refuses or absent until |
| --- | --- | --- |
| 1 | Six lookups, against in-tree responders | Consumer changes: `processor/graph-query/MIGRATION.md` |
| 2 | Path search, counts | Four forwards: refused transient, when the request reports no responders, until changes 5 and 7 bring them |
| 3 | Nothing until `COMMUNITY_INDEX` has records | `localSearch` refuses `index_not_ready` until change 7 wires the writer (ruling 3) |
| 4 **(stepping stone)** | Entity and path questions | Questions needing the text tier refuse with a distinct temporary code, whether or not records exist, so `index_not_ready` stays true (ADR-109 D4, ruling 3) and no filter is lost; step 5 removes it. Step 4's design names its class, since transient invites retries until step 5. Absent: `searchGraph` |
| 5 | The package is whole (contract, "One package per pull request") | Semantic fallback until change 5 |

Steps 2 and 3 can run in parallel; both edit `graph/query_verbs.go` and `component.go`, so the later one merges
`origin/main`.

## Issues, ADRs and carried questions

- `#103` retires at step 5, its marker left with `#15` (ruling 1). `#106`: boot refusal in step 1; retires when the
  last rows land; the declared-ports limit is accepted (`processor/graph-query/MIGRATION.md`). #111: item 2 step 2,
  item 3 steps 4 and 5, item 4 stays for change 7. #105: `IncludePredicates` in step 2; stays folded into #32. #68,
  #477: sites in step 3; stay for other packages. #506 keyword index: not this port.
- ADR-060 step 1. ADR-071 step 2. ADR-087 Decision 5 step 3. ADR-099's sentence stands (ruling 2); ADR-081, ADR-085
  apply in step 3.
- Applied from section 13 by each step's design: Q5(b) in steps 3 to 5; Q7(a) in step 2, with Q7(b) a follow-up after
  change 7; Q8(a) in step 2. Hard limits' numbers are step 2's design (owner, D); SemConnect's config already names
  `max_traversal_depth` and `max_traversal_nodes`, which the pin ignored; a reason to look, not a shape to keep.
- Step 2's design: ruling 1 does not name `semantic`, `similar`, `spatial`, `temporal` (not graph-index, not
  authority), and marking them bare sits against its headline. A design choice on its merits, including whether each
  forward lands with its responder's port.
- Step 3's design: the one new community record field (owner left it there); the community verbs' `indexed_revision`
  depends on it.
- Step 5's silent consumer change, SemSource's `searchGraph` decode: `processor/graph-query/MIGRATION.md`, which
  step 1 moved out of this change so it survives the archive; each later step appends what it changes.
- Steps 2 to 5 carry the owner's ruling (PR #504 comment 6588079803): each design decides a shape on its merits, with a
  consumer break a migration note. Pin keys such as `startTime` and `totalEntities` are the next candidates.

## Step 1 dispositions

- **Shape sweep** (`inventory.md` § 8). Required, done here (`AGENTS.md` rows, ADR-108, ADR-114, surface audit,
  build): S4, S5, S6, S9 for reserved subjects, S10, S17, S24, S25. Optional, gone with D2 and D7 at no cost: S19,
  S21, S23, S26. Code not carried here, step in `split.md`: S1, S2, S7, S8, S11-S16, S18, S20, S22. Clean: S3, S27,
  S28. Dead surface dropped: `router.go`. Pin tests rewritten on a broker (`split.md`).
- **Issues:** retires none. Stay, required later: `#103` (step 5; marker `#15`), `#106` (see above), `#405` (4;
  ADR-114 half here), `#406`, `#31`, `#477` (3), `#479` (4), `#105` (2), `#111` (2, 4, 5; item 4 change 7). Stay,
  optional or not this package's: `#68` (no collector yet), `#195`, `#196`, `#16` (the table widens it), `#15`,
  `#108`, `#155`, `#32`, `#485`-`#487`, `#506`, `#484`, `#9`, `#500`. Names the word only: `#7`, `#24`, `#109`,
  `#300`, `#350`.
- **ADRs:** ADR-060 ported, with a note that ADR-091 amends it and is not here; ADR-109, -113, -114 applied; D18
  amended by ruling 1; ADR-071 step 2, ADR-087 D5 step 3.
- **Overlap:** none (draft #507 holds only its own OpenSpec change so far). This step updates the reserved-subject
  row. The `query-pattern` skill grows.

## Tests, by step (`inventory.md` § 9)

Carried and adapted: `pathrag_test`, `summary_test`, `TestComponent_PathSearch_*` (2); `graphrag_test` pure functions,
`entity_digest_labels_test` (3); `searchgraph_test`, `classifier_defense_test`, `slice_e_contract_test` renamed (4,
5). Rewritten from behaviour on a real broker, since the requester interface and hooks are gone:
`query_error_class_test`, `operation_inventory_test`, `batch_passthrough_integration_test`, config and lifecycle parts
of `component_test`, and the behaviours worth keeping from `attack_test` (1); `component_integration_test` by step;
`community_cache_test`'s cross-level cases, `community_query_behavior_test`, `graphrag_enrich_test` (3, 5). Dropped:
`router_test`, `community_generation_test`. New per Fable rulings: counting subscriber on the prefix verb,
blocked-write tick, poison, watch loss (3).

## Merge order

1, then 2 and 3 (either order), 4, 5, all before change 5 and change 7, which step 4's stepping stone relies on.
Change 5's fusion port reads `relationships` as enveloped, with `predicate`.
