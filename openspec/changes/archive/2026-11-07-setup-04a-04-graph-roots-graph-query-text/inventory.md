# Inventory: setup-04a-04-graph-roots-graph-query-text

base: `6808234` (claim commit on `origin/main` `335c3c6`, draft PR #544). Pin `8b99efe9`; `pin:` cites read through
`gh api …/contents/<path>?ref=8b99efe9`; the probe copy's `graphrag.go` and `searchgraph.go` blobs match the API SHAs
(`0ddc205c`, `1267cc34`, checked this run). Adds to the archived step 1 `inventory.md` and the step 2 to 4
`inventory-delta.md` files; repeats nothing they settle. **This file is the inventory-only deliverable: it needs
`INVENTORY PASS` before `design.md` is reviewed.** Author: `semengine-architect` (Claude Opus 5.5).

Question: what must step 5 (text tier, `searchGraph`) carry, and what already holds each fact it models? Repositories
read (`docs/inventory-scope.md`): this tree; SemStreams at the pin (code, `docs/adr`); SemSource at `e4febc0d` for its
one `searchGraph` call (the reading the step 1 rulings allowed and `MIGRATION.md` already cites).

## 0. Pin probes

Package-wide runs (race, five shuffled, integration once, test-text rules) are the archived step 1 `inventory.md` § 1
and § 9, same package, same pin; not re-run. Step 5 behaviour probe: `notes/probes/step5-pin-probe_test.go.txt`, run
in a tarball copy of the pin (`go test -count=1 -run TestStep5Probe -v ./processor/graph-query/`, go1.26.4), log
`notes/probes/step5-pin-probe.log`. No fakes: pure functions on literal inputs.

| # | Input | Result |
| --- | --- | --- |
| P1 | 7 records, none matching "pump", `scoreCommunitySummaries` three times | all 7 returned, input order each run; so `pin:graphrag.go:1408-1412` reads the first five, scored 0 |
| P2 | records a (summary and keyword "pump"), b (keyword "pu"), c (keyword "pumps"); "pump", "pump pump", "pumps" | `[a c b]`, `[a c b]`, `[c b a]`: a short keyword inside a term scores (b); "pumps" misses a's summary; ties keep input order. The log shows order only; that a repeated term weighs twice is read from R6, not shown |
| P3 | `synthesizeAnswer`, two summaries | `"Found 3 entities across 2 knowledge clusters.\n\n(12 entities, 100% match) … Representatives: Inlet temp [sensor] {tags: thermal}. Key themes: temp, flow.\n\n\n"`; the second record (no members, no summary) adds a blank paragraph |

Read, not probed (each cited, no experiment):

- R1 `pin:graphrag.go:1374-1499` `globalSearchTextBased`: lists every record at `req.Level` from the cache, scores,
  takes `MaxCommunities` (default 5, `:29`, caller-raisable, no upper bound), loads every member up to 10,000
  (`:1415-1428`), filters with the 0.3 text score, `Count` = matches. It never reads `summarize_threshold`: SemSource's
  `"summarize_threshold": 1` (`semsource:processor/mcp-gateway/query_tools.go:103`) is a no-op on the tier-0 path.
- R2 `pin:graphrag.go:1389-1402`: a level with no records answers success, empty; Fable ruling 3 makes it a refusal.
- R3 `pin:graphrag.go:818-969`: the default `graphrag` strategy tries `graph.embedding.query.search` first and, on any
  error, logs Debug and takes the text path (`:958-961`, `:971-990`): S14 of the step 1 sweep.
- R4 `pin:searchgraph.go:68-115`: `searchGraph` = `globalSearch`, then on an empty reply a semantic search of 8; a
  semantic error is `recordError` and the empty reply returned (`:102-108`). It also falls back when global search
  refuses `index_not_ready` (`:70-80`), returning that refusal only when the semantic search fails or finds nothing
  (`:102-108`). So at the pin a tier-1 deployment without community records answered SemSource from the embedding
  index; for change 5's design and SemSource's migration.
- R5 `pin:graphrag.go:1716-1721`, `:1917-1934`: label = `dc.terms.title`, then three agentic predicates (#31), then
  the first string object that is not an entity ID.
- R6 `pin:graphrag.go:2416-2470`: group score per term `+2.0` if in the summary, `+1.5` per keyword containing the
  term or contained in it; terms not de-duplicated; `sort.Slice` (not stable). `pin:graphrag.go:2473-2560`:
  `include_relationships` re-derives edges among matches from their triples; `include_sources` maps each match to
  a group and a position score.

## 1. The claimed gap

- `processor/graph-query/globalsearch.go:27` — `const codeTextSearchUnsupported = "text_search_unsupported"`; raised
  at `:97` (resolver miss), `:219` (`fallThroughNotFound`), `:344`, `:349`, `:355` (`chooseRoute`). No text tier.
- `git grep -n -i 'searchGraph' -- '*.go'`: no handler, no row (`graph/query_verbs.go:115` — `{Name: "globalSearch",
  …` is the last search row). `git grep -n -i 'labelPredicates\|resolveLabel\|EntityDigest' -- '*.go'`: none.
- `pkg/graphview/view.go:425` — `func (v *View[T]) List(prefix string, limit int) ([]KeyedEntry[T], error) {`:
  excludes poisoned keys and does not count them (doc `:420-424`). No call lists and counts poison: the gap Fable
  ruling 3 names. `git grep -n '\.List(' -- '*.go' ':!*_test.go'`: no production reader of `View.List`.

## 2. Every current spelling of the facts modeled

| Fact | Where it is held today |
| --- | --- |
| Query terms | `processor/graph-query/localsearch.go:50` — `func searchTerms(query string) []string {` (lowercase, split, dedupe, bound `graph.MaxSearchTerms`) |
| Entity text score, rank | `localsearch.go:21` — `const minLocalSearchScore = 0.3`; `:119` — `func rankEntities(page []json.RawMessage, terms []string) ([]json.RawMessage, int, error) {` |
| One page of a group | `counts.go:32` — `func (c *Component) readCountPage(ctx context.Context, subject, prefix, cursor string) (countPage, error) {` |
| Fit to `max_payload` | `localsearch.go:172` — `func fitLocalSearch(data graph.LocalSearchData, limit int, encode …` (local-search typed) |
| Group record | `graph/clustering/types.go:26-59`: `ID`, `Level`, `MemberCount`, `EntityRevision` (`:42`), `Keywords`, `StatisticalSummary`, `RepEntities`; no member list |
| Group key, level | `graph/clustering/storage.go:101` `GroupKey`; key `{level}.{id}` (`:137` `communityKey`) |
| Words of a record | `graph/clustering/terms.go:28` `extractWords` (3+ bytes, stop words dropped), unexported |
| Which predicates are labels | `vocabulary/registry.go:477` — `func DiscoverLabelPredicates() map[string]int {` (priority per label predicate; `dc.terms.title` registered at `vocabulary/labels.go:16-22`); read by `processor/graph-index/index.go:88` — `c.namePredicates = vocabulary.DiscoverLabelPredicates()`. The pin's `labelPredicates` list (R5) is a second spelling of this |
| Truncation counter | `processor/graph-query/metrics.go:15` — `searchTruncated *prometheus.CounterVec` (label `verb`) |
| Reply types | `graph/query_global_search_types.go:8`, `:19` (`GlobalSearchRequest`, `GlobalSearchData`); `graph/query_local_search_types.go` |
| Reference resolution | `processor/graph-query/query.go:195` `resolveReference`, shared by `entityByAlias` (`:165`) and `resolveGlobal` |

## 3. Adjacent claims

- **Specs:** `openspec/specs/graph-query/spec.md` "Refusals that are not errors" (`:190-199`), "Global search answers
  by its hints" (`:560`), "A one-word question is an entity reference" (`:586`), "Questions needing the text search
  are refused until it exists" (`:626`), "One resolver reads an entity reference" (`:645`), "A strategy answers with
  its own result" (`:664`), "Truncated searches are counted" (`:550`), local-search requirements `:397-505`;
  `graph-transport-boundary` "Reserved request subjects have one declaration" (`:13-41`);
  `graph-view-subscription` "Point reads are coherent with the stream" (`:120-131`).
- **ADR text:** `docs/adr/114-alias-index-keys-an-alias-by-its-owner.md:117-118` — "In global search a reference
  naming no entity falls through to the text tier".
- **Statements step 5 changes:** `openspec/specs/graph-query/spec.md:599-605` ("Global search refuses what no strategy
  can honour": decodes only `query` and `min_revision`); `:664-686` ("A strategy answers with its own result": exactly
  one of `entity`, `path`, `temporal`, `spatial`); `:687-698` ("A strategy's revision is its read's": no rule for
  text); `graph/query_global_search_types.go:5-7` — "The question is classified by its words alone; no other key
  steers which strategy answers." and `:13-18` (the `GlobalSearchData` comment, four strategies); `MIGRATION.md:153`
  (`level` "refused … until step 5"); `docs/repository-map.md:90` ("Steps 1 t…"); `.agents/skills/query-pattern/SKILL.md:125-126`
  (`searchGraph` "not in this tree yet").
- **Guidance and records naming the temporary code:** `.agents/skills/query-pattern/SKILL.md:89`,
  `docs/admission-ledger.yaml:4253`, `:4290`, `processor/graph-query/README.md:194-198`, `doc.go:49`,
  `MIGRATION.md:153`.
- **Rulings:** Fable rulings 2 and 3 (PR #504 comments 6582751783, 6582752155), accepted with the keyword index
  filed (6585009466); PR #504 comment 6588079803 (shape on merits); #487 comment 6688945537 note 2; #535 ruling
  6706930613; #9 comment 6386603659 (no LLM paths in change 4).
- **Open pull requests** (`gh pr list --state open --json number,title,changedFiles,files`, this run): #544 (this
  claim, no files); #545 (ateam build, `Taskfile.yml`, `scripts/doctor.sh`, docs; no overlap); **#543**
  (`claude/fix-512-prefix-dedup`, 13 files, ready): edits `processor/graph-query/MIGRATION.md`
  (`:47`), `README.md` (`:96`), `counts.go` (comment of `countPage` only) and the `graph-transport-boundary` spec.
  Overlap: comment and doc lines only; no capability delta under `graph-query`. #543 merges first.

## 4. Open issues naming the package or the carried code

Queries: `gh issue list --state open --search "<q>" --limit 200` for `graph-query`, `graphquery`, `searchGraph`,
`globalSearch`, `graphrag`, `text tier`; hits read.

| Issue | Names | Disposition |
| --- | --- | --- |
| #103 envelope, `min_revision` | `processor/graph-query/query.go` | Retire: the last verb gains the envelope and `min_revision`; generation marker stays with #15 (split.md) |
| #106 verb catalog | graph-query table | Retire: the last row (`searchGraph`) lands (split.md) |
| #31 agentic label predicates | `graphrag.go:1718-1720` | Retire: labels read `vocabulary.DiscoverLabelPredicates()`, no agentic import |
| #111 item 3 | `graphrag.go:841-850` | Stays (item 4, change 7); item 3's second site (`graphrag` tier 1) not carried here; comment |
| #15 generation marker | #103's addendum | Stays (`status:blocked`, graph-ingest); comment that #103 closed and the marker is its own |
| #535 classifier limits | `graph/query`, global search | Stays. Its ruling (6706930613) accepted only the loud "two places" refusals. How the text tier treats the classifier's other word triggers (path, similarity, ranking, aggregation, zone) on free text is the design's owner question (D1); under option (b) it adds refusals of answerable questions |
| #482 ADR port notes (083, 084, 085) | ADR-085, which this step applies | Stays: docs, not this package's work |
| #506 keyword index | text search over a large group | Stays (ruling 2 follow-up); the counter gains `globalSearch`, `searchGraph` |
| #32, #105 capability seam, tier split | graph-query | Stay: change 7's last task; #105 folded into #32 |
| #485, #486, #487 tier 2 | LLM summaries, classifier, synthesizer | Stay: tier 2 (#9 comment 6386603659) |
| #477 graphview adoption | `pkg/graphview` | Stays: other sites; graph-query already reads through a view |
| #512 listing repeats | prefix page | Closed by #543 on merge; `rankEntities` dedupe stays harmless |
| #68 metric `sync.Once` | pin `metrics.go` | Stays: not carried, other packages' sites |
| #525 ascending revisions | clustering maintainer | Stays (change 7); D5's revision rule leans on it |
| #9, #10, #16, #24, #108, #109, #155, #195, #196, #300, #350, #484, #536, #7 | | Names the word or another package, not this code |

## 4a. Shape sweep, the code step 5 carries (step 1 `inventory.md` § 8)

| # | Pin site | Disposition |
| --- | --- | --- |
| S1 | `graphrag.go:72-88` context-value lease | Not carried: no lease (ruling 3), no `context.WithValue` (AGENTS.md § 1) |
| S5 | `searchgraph.go:62-64` `searchGraphBeforeFinalize` hook | Not carried: test back door (#186) |
| S14 | `graphrag.go:958-961`; `searchgraph.go:102-108` silent lower tier | Not carried: no semantic tier here (D7) |
| S15 | `graphrag.go:2264-2281` storage lookup swallowing failures | Not carried: records come only from the view |
| S20 | `searchgraph.go:220` re-decoding a reply to test emptiness | Not carried: no fallback, typed result |
| S22 | `answer.go:69` `AnswerSynthesizer` | Not carried: owner ruling Q6 (6585009466 item 3); plain function |
| S26 | `searchgraph.go:81` `_ := json.Marshal` | Not carried with the fallback |

## 5. Problem shapes and nearest instances

- **Bounded fan-out read with declared truncation:** `localSearch` (`localsearch.go:218`), one record, one page,
  `entities_truncated`, fit, counter. Adopted shape; step 5 is five of it.
- **Enumeration of a view with poison declared:** none in the tree (`List` hides poison). The primitive Fable ruling 3
  asks for; one site (graph-query). Adoption sweep: `#477`'s sites use `Get` or subscriptions, none enumerates and
  serves; no other adopter found (`git grep -n 'graphview\.' -- '*.go' ':!pkg/graphview/*'`: graph-query only).
- **Level prefix and range:** `graph/clustering/maintainer.go:20` — `const levels = 3` (unexported), read by
  `storage.go:102`; `storage.go:137` — `func communityKey(level int, id string) string {` (unexported). No exported
  spelling of `{level}.` or of the range 0 to 2.
- **Pin's poison read:** `pin:pkg/graphview/view.go:399-409` `View.Poisoned`, a key list under a separate lock from
  `List`; not carried (`docs/admission-ledger.yaml:3944`, unread at the pin). Two calls cannot give one consistent
  pair, so it does not answer ruling 3.
- **Label of an entity:** graph-index's name index reads the label set (`index.go:88`); no reader picks one label.
- **Alias verb:** `searchGraph` answering as `globalSearch` — precedent: the four step 2 forwards answer as the verb
  they forward to (`graph/query_verbs.go:57-60` comment).

## 6. Governing pin ADRs

Searches: every `pin:docs/adr/*.md` for `searchGraph|globalSearch|GlobalSearch|graphrag|text-based|
semantic_fallback|EntityDigest|entity_digests`; titles read.

| ADR | Decides, for this code | Here |
| --- | --- | --- |
| 087 Summary store | Decision 5: readiness on `COMMUNITY_INDEX` only; tiered summary (LLM store, then statistical) | ported with note; store with #485 |
| 099 Prefix partition | members never stored, read by the prefix lane; "GraphRAG … shape-for-shape" | ported; the sentence yields to 6588079803 (its port note) |
| 081 Graph view | the community view site | ported |
| 085 Gate on health | level-keyed records (`:164`) | ported |
| 062 Deterministic fusion | `research-graph-execute`'s BM25 calls `searchGraph` (`:63`); its adapter reads `entity_digests` (`pin:processor/research-graph-execute/adapters.go:292-297`) | not ported; change 5, told digests are gone (design D9) |
| 071 Fusion scope | semantic scope; type filter post-retrieval | ported; no semantic path here |
| 060, 109 D4, 113, 114 | classified errors; `index_not_ready` reserved; run context; resolver | ported |
| 054, 059, 061, 086 | one-line mentions of GraphRAG quality or community edges | not governing |

## 7. Who it is for (new surface the design may add)

- `searchGraph` verb: SemSource `processor/mcp-gateway/query_tools.go:107` (its one search call); pin consumer adapters
  `research-graph-classify/adapters.go:40-61`, `research-graph-execute/adapters.go:268-300`, `graph-gateway` (`:1164`).
- Text result types in `graph`: the same readers (SemSource `graph_matches.go:107-114` reads `entity_digests`,
  `entities`, `entity_ids`, `community_summaries`, `answer`, `count`).
- A poison-counting enumeration in `pkg/graphview`: graph-query (ruling 3), one production reader.
- `level` on `GlobalSearchRequest`: `graph-gateway` sends it (`component.go:1415`); SemSource does not.

## 8. Adopter seam (SemSource's `searchGraph` call)

1. *Must know:* the reply is enveloped (`data`); the keys it reads change; at tier 0 nothing answers text until
   change 7 writes `COMMUNITY_INDEX`. Three facts: over the limit of two, so the design owes the loudest default.
2. *Does nothing:* `json.Unmarshal` into `graphSearchBody` ignores `data` → no matches, no answer, no error
   (`query_tools.go:113`, `MIGRATION.md:23-27`). Silent.
3. *Finds out:* doc only today. If its `summarize_threshold` key is refused by name, the first call is a typed runtime
   error naming the key: loud.
4. *Should know:* only "decode `data` by the verb table".

## 9. Size

Pin read for step 5: `searchgraph.go` 295, `graphrag.go` text tier and helpers about 420 (R1, R5, R6, `:2164-2210`);
tests `searchgraph_test` 146, `entity_digest_labels_test` 310, `graphrag_enrich_test` 434, `graphrag_test` parts.
