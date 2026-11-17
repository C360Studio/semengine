# Inventory: setup-04c-02-hybrid-search

base: 8c48da4710df7fe1cf5a48d24e71e50296b80f84 (claude/hybrid-search-668; main 2f66d480f4f89739206f33114364bcd357d1f02b)

No package is carried from the SemStreams pin; categories 6, the shape sweep and the pin probe do not apply.

## 1. The claimed gap: "nothing merges word and vector lists"

- `processor/graph-query/textsearch.go:72` — `if subjects.semantic != "" && len(matches) == 0 {`: the embedding
  search is asked only when words match nothing.
- `processor/graph-query/textsearch.go:44` — `if subjects.semantic != "" && isIndexNotReady(err) {`: or when the word
  index is not ready.
- Searches that came up empty, on tracked Go and Markdown outside archived changes and this change: `git grep -n -i
  -w -e reciprocal -e rrf -e "rank fusion" -- '*.go' '*.md'` hits only mean-reciprocal-rank arithmetic and prose:
  `graph/lexical/scorecard_test.go:95` — `reciprocal             float64`, `:100`, `:112`;
  `processor/graph-query/scorecard_integration_test.go:187`, `:195`, `:204`; `processor/graph-query/README.md:280`;
  `processor/graph-query/MIGRATION.md:322`. None merges lists. `pkg/fusion` is the lens-driven answer engine
  (`pkg/fusion/doc.go:1`), not a rank merge.
- The only merge that exists is the throwaway probe's `rrf` (archived
  `2026-11-16-setup-04c-01-http-embedder/notes/probes/scorecard-semembed-7ceb5281.md`, "Source"), deleted after
  the run.

## 2. Every current spelling: which list found a match, and in what order

- `graph/query_global_search_types.go:29-34` — `Text *TextSearchData` / `Semantic json.RawMessage` / `Fallback
  *GlobalSearchHint`: the word list, the embedding reply as its bytes came, and the step that asked for it.
- `graph/query_global_search_types.go:91-95` — `TextSearchMatch{ID, Label, Score, Entity}`; `Score` is "the word
  index's score" (`:87-90`).
- `graph/query_search_types.go:14-16` — `SimilarityHit{EntityID, Similarity}`, the embedding list.
- `graph/query_lexical_types.go:19-22` — `LexicalHit{EntityID, Score}`, the word list.
- `processor/graph-query/globalsearch.go:28` — `hintSemantic       = "semantic"`; `:33-34` — `fallbackNoMatches` /
  `fallbackIndexNotReady`.
- `processor/graph-query/globalsearch.go:157` — `func (c *Component) semanticFallback(...)`: the one place graph-query
  asks the embedding search for searchGraph.
- Release status: `git merge-base --is-ancestor 2f66d480 v0.1.0-alpha.5` → false. `Semantic` and `Fallback` are
  unreleased.

## 3. Adjacent claims

- Specs: `openspec/specs/graph-query/spec.md:528-606` (searchGraph, its fallback, the `embedding_enabled` switch),
  `:683-716` (matches and what a reply left out), `:778-800` (answer template, word scorecard), `:802-812` (losing
  the provider).
- Rulings: #563 (tier 1 labelled fallback; closed by hand 2026-11-16 after PR #681); #668 comment 6920821032 and #666
  comment 6920823901 (owner 2026-11-16: merge first, floor on the merged row, about 60%); #666 owner comment (any
  `embedding.Embedder`; the result names its model); archived setup-04c-01 design Open Question 2 (scorecard as a
  Task target before each tier 1 release, output in the release evidence; no real provider in verify).
- Open issues naming graph-query (`gh issue list --state open --search "graph-query"`): #668, #666 (this change);
  #682 (reranking over the merged list, follows this); #683, #667 (embedded text; the scorecard measures them later);
  #487 (tier 2 synthesis reads searchGraph's list); #602 (generation beside `indexed_revision`); #670 (graph-query
  README Health text); #678 (a graph-query test bug). Names the word, not the package: #486, #657, #656, #535, #484,
  #477, #111, #630, #536, #621, #68, #564, #659, #10, #109, #300, #9, #195, #108, #155, #196, #11, #350.
- Open PRs (`gh pr list --state open`): #684 (draft, this claim; its files are this change's OpenSpec files);
  #597, #598 (Dependabot, `go.mod`, `go.sum` only). No overlap with OpenSpec capabilities. PR B opens its own draft
  claim when its work starts.
- alpha.5 behaviour: `git show v0.1.0-alpha.5:processor/graph-query/query.go` lines 67-70 — `// searchGraph answers
  as globalSearch, ... with no semantic fallback.`: the released searchGraph answers as globalSearch at every tier.
- `openspec/specs/nats-fixture/spec.md:14-26` (New "SHALL give one test its own NATS server"; cleanup "reported as a
  test error"); `docs/test-infrastructure.md:216-221` (when harness code is written); `Taskfile.yml:87-95`
  (`pindiff`, `mutcheck`: harness programs run by Task, not by tests).
- Docs that describe searchGraph as globalSearch under another name: `.agents/skills/query-pattern/SKILL.md:84`,
  `docs/repository-map.md:91`.

## 4. Who the new surface is for

- `TextSearchMatch.WordRank`, `.VectorRank`, `GlobalSearchData.Merge` (renamed from `Fallback`): framework API, read
  by consumers of `graph.query.searchGraph` (semsource reads searchGraph, `disclosure.go:31` per the archived
  setup-04c-01 design, Migration Plan; #487 tier 2 synthesis). Contract line: the graph-query spec delta and README.
- The scorecard program and Task target: a harness tool with a repo caller (`Taskfile.yml`), like
  `Taskfile.yml:92-95` — `mutate:check` → `go run ./internal/harness/mutcheck`.
- `natsfixture.New` parameter narrowed from `testing.TB` (`internal/harness/natsfixture/fixture.go:107` — `func New(t
  testing.TB, opts ...Option) *Fixture {`): its new caller is the scorecard program; it uses only `Helper`, `Errorf`,
  `Logf`, `Cleanup` (`:108-120`).

## 5. The problem shape

- Merge two ranked lists from two responders into one labelled answer that degrades when one responder fails. Closest
  instance: `processor/graph-query/textsearch.go:71-76` (hints verified beside the word answer; a failed fallback
  named, not a refusal) and `globalsearch.go:145-150` `notHonouredBy`. Adopted.
- Measure ranking quality on a known-answer set against a stated bar:
  `processor/graph-query/scorecard_integration_test.go:281-311` (`TestSearchScorecard`, `requireBar`). Adopted for
  the arithmetic and the bar's form; cannot be adopted as a test (needs a real provider, see the design).
- Boot the real stack outside a unit test: `internal/smoke/doc.go:1-5` (integration-tagged composition root). Not
  adopted as a place: its tests run in verify.

## Same-class collision table

Not triggered: the change adds no durable, communication or coordination primitive (no bucket, subject, stream or
lease). The merge is a pure ranking step inside one existing handler.

## Adopter seam inventory (searchGraph callers)

1. What must they know? (a) at tier 1, `text.matches` is the merged list and `score` is the merged score; (b) `merge`
   says whether the embedding list was read. Two items.
2. If they do nothing: a caller reading `text.matches` in order gets the better list; reading `score` as a BM25 value
   is wrong only if they compare it across replies, which the existing doc already forbids
   (`graph/query_lexical_types.go:17-18`). A caller reading `fallback` finds it gone: it was never released.
3. Where they find out: `merge.honoured` false with `code` in every reply (typed runtime field); ranks per match.
4. What they should have to know: nothing to get the best list; the labels are for disclosure only. No gap.
