# Inventory delta: setup-04a-04-graph-roots-graph-query-search

base: `f5ffe3b` (the claim commit on `43688fd`); `origin/main` is `25750d5`, which adds only PR #529's edit to
`openspec/specs/graph-query/spec.md` ("Requests are counted for operators"; this change does not modify that
requirement). Adds to the archived `inventory.md` (PR #504) and the step 2 and 3 `inventory-delta.md`; repeats nothing
they settle. Pins are at base unless marked `pin:` (`8b99efe9`; the local pin copy's blobs for `graphrag.go`,
`entity_resolver.go`, `graph/query/classifier.go` and `processor/graph-index/component.go` match `gh api
contents?ref=8b99efe9` SHAs, checked this run). **This file is the inventory-only deliverable: it needs `INVENTORY
PASS` before `design.md` is reviewed.**

## 1. The claimed gap: no coordinator, no resolver, no classifier reader

- `git grep -n -i 'globalSearch\|resolvePartial\|text_search' -- '*.go'`: comments only (`model/registry.go:27`,
  `natsclient/client.go:105`, `service/service_manager_config.go:23`, a test comment). No handler, type or row.
- `git grep -n 'graph/query"' -- '*.go' ':!graph/query/*'`: one test guard
  (`internal/harness/contract/embeddingprovider_test.go:38`). `graph/query` has no production importer; step 4 is its
  first.
- `graph/query/classifier_chain.go:90` — `func (c *ClassifierChain) ClassifyQuery(ctx context.Context, query string)
  (*ClassificationResult, error) {`: refuses an impossible number with `errs.WrapInvalid` (`:96-98`, PR #480).
- `graph/query/classifier.go:47` — ``zonePattern   = regexp.MustCompile(`(?i)\b(zone|area)[-\s]([a-zA-Z0-9-]+)\b`)``,
  `:174` — `opts.PathStartNode = matches[1] + "-" + matches[2]`: "the area of the plant" gives start `area-of`.
- `processor/graph-query/query.go:161` — `func (c *Component) entityByAlias(aliasSubject, entitySubject string)
  queryHandler {`: alias first, holder wins, a valid ID read as itself, anything else `entity_not_found` with no second
  request (ADR-114 Consequences as amended). The one existing spelling of "reference to entity".
- `processor/graph-query/pathsearch.go:29` — `func (c *Component) pathSearch(`, `:78` `readPathStart`, `:143`
  `walkPaths`: step 2's walk; its reply `graph.PathSearchData` is already an explicit chain (`graph/query_path_types.go`,
  "Edges[i] first reached Entities[i+1]").
- `graph/query_search_types.go:37`, `:49` — `SpatialData`, `TemporalData`: reply types exist; **no request type** for
  `bounds`, `range` or `search` (`git grep -n 'type .*Request struct' graph/`: entity, batch, prefix, path, hierarchy,
  summary, local search, mutation types only). Step 2 forwards their bytes unread.
- `processor/graph-index/index.go:429` — `plan, err := c.buildEntityIndexPlan(state, entityID)` / `:430-432` `if err !=
  nil { return err }`; `:207-213` `reconcileEntity` logs Warn and `return !errors.Is(err,
  errAuthoritativeIdentityMismatch)`: a refused plan completes the revision unmarked (#428). Pin:
  `pin:processor/graph-index/component.go:1546-1549`, same shape.

## 2. Every spelling of the facts modeled

| Fact | Spellings today | Pin |
| --- | --- | --- |
| A question's intent | `query.ClassificationResult.Options` keys (`classifier_chain.go:182-222`) | plus `resolveStrategy` by bare strings (`pin:graphrag.go:633-679`) |
| Entity reference to ID | `entityByAlias` (`query.go:161-197`) | plus `resolvePartialEntityID` (`pin:entity_resolver.go:16-46`), whose suffix verb change 2 removed |
| "Not answerable yet" | `index_not_ready`, reserved by ADR-109 D1/D4 for replay and unresolved writes (`graph/mutation_responses.go:66-86`) | `communityCacheNotReadyReason` degrade (`pin:graphrag.go:27`) |
| Entity cannot be indexed | `markEntityFailed` (`index.go:243-250`) for write failures; reset latch for unreadable state (`:413-427`) | the same, plan refusal unmarked |

## 3. Pin facts (probe and reads)

Probe: `notes/probes/step4-pin-probe.go.txt`, added by `go test -overlay` to the pin copy
(`/tmp/setup-04a-04-graph-query-stage/pin`, local only), Go 1.26.4, one run, output `step4-pin-probe.log`. Tree probe
the same way, `step4-tree-probe.*`. Neither copy was changed.

- P1. At tier 0 the pin reaches four strategies: `pathrag` (path words with a start), `temporal` (time only),
  `spatial` (area only), `graphrag` (everything else). `entity_lookup` and `semantic` are reached only through an
  explicit `strategy` option (`pin:graphrag.go:640-656`), which only the LLM tier or a tier-1 example's options set
  (`graph/query/classifier_embedding.go:209`; tier 1 is not composed by graph-query): "show me sensor-001", a full ID
  alone, "forklift", "similar to", "how many", "top 5" all went `graphrag`.
- P2. Time and area together went `graphrag` (`:669-670`), which drops both filters (`:818`). Path plus time went
  `pathrag` with the time range unused (`:402-490` reads no `time_range`).
- P3. "the area of the plant" gives `path_start_node` `area-of` at pin and tree (both logs).
- P4. The tree's chain refuses "last 0 hours" with an invalid error naming the phrase (tree log).
- P5 (archived P5b, re-cited). The resolver turns an alias refusal into `""`, nil.
- Reads: the temporal and spatial strategies fall to `graphrag` with `nil` type filters when the hint is missing
  (`:1199-1209`, `:1267-1277`); the type-filter fallback widens an emptied set (`:841-850`, `:1117-1128`, #111 item 3);
  `extractSearchRefinements` and `filterClassifierTemplates` (`:523-571`) guard LLM output only, since the keyword tier
  sets neither `query` nor `types` (`classifier_chain.go:182-222`, tree).
- Unit runs at the pin for this package (race once, five shuffled, integration once) and the test-text findings:
  archived `inventory.md` §§ 1, 9; not re-run (same pin, same package).

## 4. Open issues

Query: `gh issue list --state open --search <term>` for graph-query, classifier, resolver, graph-index,
globalSearch; bodies and comments read.

| Issue | Names step 4 code | Proposed |
| --- | --- | --- |
| #405 | resolver, suffix verb | Retire |
| #428 | `processor/graph-index/index.go:429-432` | Retire (owner assigned to step 4, comment 6688101995) |
| #479 | item 3, zone/area start (items 1-2 merged by PR #480, `f3b48f6`) | Retire |
| #111 | item 3, `pin:graphrag.go:841-850`, `:1117-1128` | Stays: step 4 removes the semantic-strategy site; the `graphrag` site is step 5's; item 4 change 7 |
| #487 | comment 6688945537 (ImplicitHop notes) | Stays, tier 2; note 1 applied |
| #486 | the LLM classifier tier, which alone reaches `entity_lookup` and `semantic` at the pin | Stays, tier 2 |
| #106 | the table grows a row | Stays to step 5 (last rows) |
| #103, #31 | envelope marker; labels | Stay, step 5 |
| #105, #32 | relay/NL split; provider seam | Stay (#105 folded into #32) |
| #16 | the new row widens reserved subjects | Stays, no code here |

Checked, naming no step 4 code: #7, #20, #408. Names the word, not step 4 code: #9, #24, #68, #108, #109, #110,
and #141, #155, #195, #196, #300, #350, #477, #482, #484, #485, #506, #512, #521, #525.

## 5. Overlapping open PRs (`gh pr list --state open --json number,title,changedFiles,files`)

Open: #530 (this claim, no files); #516, #517 (Dependabot, `go.mod`, `go.sum` only). #529 merged as `25750d5` after base.
No overlap; the branch merges `origin/main` before its spec sync.

## 6. Governing ADRs (pin `docs/adr`, 85 files)

`grep -l -i -e globalSearch -e resolveStrategy -e ClassifierChain -e path_start_node -e pathrag -e resolvePartial`:
043, 059, 062, 085, 099. 043 (injection corpus), 059 (trust tier), 062 (fusion, change 5) mention only. 085 and 099
govern step 3's reads and step 5's text tier. Governing step 4 by what its code does: ADR-060 (classified errors,
ported), ADR-114 (alias, Consequences bind the resolver), ADR-109 D1/D4 (`index_not_ready`'s meaning), ADR-113 (no new
run context needed), ADR-112 (no-responders class not promised), ADR-066 (graph-index: an index that drops writes and
reports caught up; #428), ADR-071 (type-filter axis; its site is not carried here).

## 7. Collision table: the temporary refusal (a coordination signal)

| Dimension | Evidence |
| --- | --- |
| Semantic class | "this build has no tier that answers the question" |
| Owners | none today; nearest `index_not_ready` (ADR-109 D1, D4: replay and unresolved writes only), `resource_exhausted` (narrow the query) |
| Status | none; `Health` unaffected by caller refusals (spec "Requests are counted") |
| Lifecycle | lives from step 4's merge to step 5's |
| Readers | graph-gateway at the pin (consumer adapter, not in tree); no SemEngine reader; semsource reads `searchGraph` only (archived § 3) |
| Writers, ownership, catalogs, recovery | graph-query alone; no durable state; no catalog: `git grep -n 'ErrorCode' graph/ pkg/errs/` |

## 8. Adopter seam: a product calling `graph.query.globalSearch`

1. Must know: which question shapes answer (one entity reference; a path from a named entity; a time range; an area);
   that the rest refuse until step 5; strategy names and reply fields; partial IDs resolve only as aliases.
2. Doing nothing: a pin-shaped request (`level`, `max_communities`) is refused naming the key; a free-text question is
   a classified refusal. Nothing answers empty or drops a filter.
3. Found out by: typed runtime error. 4. Should know: nothing but the reply shape; the gap is the temporary refusal.

## 9. Size

Pin, carried or replaced: 611 lines of `graphrag.go` (`:156-220`, `:402-507`, `:633-795`, `:1001-1066`, `:1196-1358`,
`:2562-2609`) and 116 of `entity_resolver.go`; tests about 120 of `query_error_class_test.go` (`:152-220`, `:570-621`)
and 274 of `slice_e_contract_test.go`. Not carried: `:523-571`, `:1068-1191` (LLM-only), `:818-990` (step 5).
