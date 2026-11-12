# Design: setup-04b-01-lexical-index

## Context

Facts are in `inventory.md` (passed at `a1783aa1`), cited by section. Tier 0 search today has three tokenizers, four
scorers and no word index (§1, §2). The embedder's statistics are learned per process and never subtracted (§2), the
cause of the order and restart drift SemSource's R1 names (§5). An entity's text is picked by predicate suffix and
its offloaded body is fetched by the caller (§3). Entities with no text are reached only by structure (§4). The
package is new, has no ledger row, and is public, so it is framework API (§6, §7). Memory was measured (§8).
Three new probes back the choices below, to be committed under `notes/probes/`: `sequence-probe`,
`tokenizer-probe-2` and `word-shape-probe` (source and log each).

## Goals / Non-Goals

Goals: one pure package, `graph/lexical`, that ranks by BM25 over the entities it holds, answers words, `"phrases"`
and `-excluded` words, ranks name words above body words, and proves it with laws and a scorecard. Each of the four
"Room to extend" points in the proposal, plus `similar`, can be added later with no change for existing callers.

Non-goals: callers (changes 2 to 4), a stored index, a per-entity token record, typo tolerance, word splitting for
scripts written without spaces (Chinese, Japanese), and any help for entities without text (README says so, D9).

## Decisions

**D1. Surface.** `NewIndex() *Index`; `Put(entityID string, doc Document) error`, which replaces what the entity
held; `Delete(entityID string)`; `Len() int`; `ParseQuery(text string) (Query, error)`;
`Search(q Query, opts Options) (Result, error)`. `Document{Name, Body string}`. `Options{Scope []string; Limit int}`.
`Result{Hits []Hit; Matched int}`, `Hit{EntityID string; Score float64}`. `Tokens(text string) []string` is the shared
tokenizer. Put refuses an invalid entity ID (`semtypes.ValidateEntityID`), so a held entity always matches a scope
correctly (`graph.MatchesAnyIDPrefix` fails closed on invalid IDs, inventory §2). A document with no words removes
the entity, since it could never match. Search refuses a malformed scope prefix or a limit below 1. ParseQuery
refuses a query left with no word to match (only stopwords, or only exclusions) and one over `MaxQueryWords` (32),
with sentinel errors (spec). Hits are ordered by score, then ID. `Matched` counts every match in scope before the
limit, for truncation reporting and rank fusion.
Options considered: (a) Put takes text in fields (chosen): one call, and the index does the tokenizing.
(b) Put takes already-split words: two calls every caller must pair, and they can pair them wrongly. Option (a)
leaves room for (b) later as a second write method that sits beside Put (D7). `Query` keeps its fields unexported,
so adding a query form adds no field any caller must set.

**D2. A search during a write.** (a) `Index` is safe for concurrent use. Each Put or Delete is atomic: a search
answers as if it ran wholly before or wholly after each write (chosen; proved by `TestSearchDuringWrites` under
`-race` against two answers worked out beforehand). (b) The caller locks: change 2 would then have to know to lock
(adopter seam, inventory §7). (c) Copy-on-write snapshots: memory doubles while a write runs. The lock is the
developer's choice.

**D3. Tokenizer.** The smallest rule set that meets the laws; the exact rules and their witnesses are in the spec.

- Lower-case with Unicode rules. A word is a run of Unicode letters and digits; anything else splits, so
  `graph.query.status`, `pump-7` and `_test.go` become separate words. One-letter words stay (`-v`, `7`).
- Identifiers split at a lower-to-upper step and before the last capital of a capital run (`withRetry` is held as
  `retry`, since `with` is a stopword). The joined form is not held, so the query `withretry` finds nothing. The
  spec and README declare this, and a behaviour question pins it. Holding the joined form as well would add 8.3%
  more words and 14,899 distinct forms beside 10,504 on our Go files, and it would have to stay out of the word
  sequence to keep phrases adjacent. It can be added later as an internal change, since the index is rebuilt.
- Length: a word over 32 bytes is dropped after splitting. Three options were measured on our Go and Markdown
  (`word-shape-probe.log`). Capping at 32 before splitting loses 2,263 of 41,864 runs: every long test or type name.
  Capping at 32 after splitting loses 345, all of them lower-case hex (commit SHAs, digests). Capping at 64 before
  and 32 after loses 402. Random base64 still becomes junk words under every option (about 300 to 1,100 per
  4,000 characters); each junk word is held by one entity and costs memory only. Chosen: cap after splitting.
- Stopwords: the 30 words of `graph/clustering/terms.go:16`, not the embedder's 84. The 84-word list drops `not`,
  `no`, `get`, `find`, `show` and `all`, which carry meaning in phrases and in code. Measured
  (`tokenizer-probe-2.log`): R@10 36.6% against 37.0%, with 3,709 matches per query against 3,341. The original
  `tokenizer-probe.log` measured only the 84-word list. Change 4 points clustering at this list, leaving one.
- Plurals: the plural-only stemmer (`tokenizer-probe.go.txt:11`) plus two rules that go before it. `es` is stripped
  after `ss`, `x`, `ch` or `sh`, and a final `e` after them, so class, match, box, hash and cache fold with their
  plurals. Corpus pairs that fail to fold drop from 117 to 54, and R@10 is unchanged (`tokenizer-probe-2.log`).
  Words in `-us` and `-s` (bus, status) still do not fold. Rejected: the embedder's suffix stemmer (it splits 545
  pairs) and a dictionary stemmer (a new dependency for about one point at best).

**D4. Phrases.** (a) Each entity keeps its word sequence (word numbers, repeats included) in memory. A phrase is
first narrowed to the entities holding all its words, then checked for adjacency in their sequences (chosen).
Measured at one million entities: 472 B per entity against 471 B with no sequence (20 words), and 1,590 B against
1,495 B (120 words) (`sequence-probe.log`). The index needs these lists anyway, to delete an entity's words.
(b) Positions in every posting: 1,186 B (20 words) and about 5.2 KB (120 words) (`memory-probe.log`).
(c) Re-read the text of the best candidates: the index would have to read storage, including offloaded bodies, which
the proposal rules out. Option (a) still allows (b) later as an internal change if long-text phrase checks prove slow
(D7). Phrases and the words around them are compared after stopwords are removed: `"out of memory"` also matches
"out the memory". This is declared in the spec and the README.

**D5. Exclusion.** `-x` removes every entity that holds `x`'s words, adjacent, in any field (Name or Body). `-test`
drops an entity whose Body holds `foo_test.go`, because `_test.go` splits into `test` and `go`. What goes into the
Body (a file path, as R3 needs) is change 2's mapping of predicates to fields. (a) Fields only (chosen).
(b) Also index the entity ID's segments: rejected, because instance segments are often hashes and an ID's meaning is
the product's (`AGENTS.md`, What this is for).

**D6. Scoring and the name weight.** Field-weighted BM25 (BM25F): term frequency is `w·tf(Name) + tf(Body)`, length
is `w·len(Name) + len(Body)`, with Lucene's IDF `ln(1 + (N − df + 0.5)/(df + 0.5))` (never negative), `k1` 1.2 and
`b` 0.75, the probe's reference (`retrieval-probe.go.txt:85`, `:112`). N, df and average length are read from the
index at search time, so equal held sets give equal scores. Scope filters before the limit and never changes a
score. Options: (a) BM25F (chosen); (b) two BM25 scores summed: saturates twice, and a name match can lose to a
long body; (c) a flat bonus for a name match: a second unit of score, with no length normalisation. Value of `w`:
task 2.4 sweeps 1, 2, 3 and 5 on the scorecard and records the results in `notes/2.4.md`. It keeps the smallest `w`
that passes `TestNameOutranksBody` and is within one point of the best R@10. Not measured yet.

**D7. Room to extend.** Each later addition is a new method or an internal change; no caller and no scorecard
question changes.

- **Token record:** add `Words(id)` to read an entity's sequences and `PutWords(id, ...)` to restore them. A stored
  record would then also hold a tokenizer version.
- **Positions (D4b):** internal; the phrase law test does not see the layout.
- **Typo tolerance and query forms:** a new form adds an unexported field to `Query`; the word table is internal.
- **Tier 1 vectors by rank:** `Result` gives a total order and `Matched`, which is what rank fusion reads.
- **Entity-to-entity `similar` (change 4):** `SimilarTo(id, Options)` can build a query from the entity's own held
  words.

**D8. Scorecard.** (a) A frozen known-item set: 2,000 sections of this repository's Markdown at `2853486d`, each
file's first heading as Name and the section as Body, with 500 section headings as questions. It is kept in
`graph/lexical/testdata/scorecard/` and gzipped (size reported in task 2.4). The floor is the measured R@10 minus one
point. The floor is shown able to fail with `task mutate:check` and an IDF-is-one mutant (notes/2.4.md).
(b) About 30 behaviour questions in SemSource's `questions.json` shape: phrases, flags, dotted names, `-test`,
identifiers. Every one must pass. SemSource's 21.6k corpus and 33 questions stay in their repository, for their
runner against the composed stack once change 2 serves the verb. Rejected: a generated corpus (it proves the
arithmetic, which D10's reference model already does, not quality) and copying SemSource's corpus (too large, and
their data).

**D9. README.** It says plainly that word search helps only entities with text, and that telemetry and other numeric
data are reached through structure: prefix, predicate and value, relationships, name and alias, and groups by ID.
The time-range and bounds verbs have no responder yet (inventory §4).

**D10. Rules (inventory §6).**

- `TestLexicalImportsNoNATS` lives in `internal/harness/contract`, beside `graphtransport_test.go`, with a
  sensitivity case.
- Fuzz targets `FuzzTokens` and `FuzzParseQuery`: they never panic, every word is lower case, 32 bytes or less and
  free of separators, and tokenizing twice gives the same words.
- `"graph/lexical unit"` is added to `scripts/cover-check.sh`.
- Rapid properties over a closed vocabulary (lower-case, no `s` or `x`, never ending in `e`, no stopword, so
  tokenizing changes nothing):
  history equality, index against a reference model (a naive BM25F over a `map[id]Document`), absent words, and
  plural folding. Named witnesses run before each `rapid.Check`.
- Framework-API contract line: the spec plus the README. Product test: `ExampleIndex` in package `lexical_test`.

## Risks / Trade-offs

- Memory is about 1.6 KB per entity at 120 words; text up to the 4,000-character cap could be several times that
  (owner question 2). Query time is not measured; change 2 measures it.
- The name weight and the floor are tuned on our own Markdown, so they may overfit it; SemSource's runner is the
  outside check.
- The tokenizer becomes part of a stored format once a token record exists (D7 records a version then).
- Stopword removal makes phrases match across removed words (D4).

## Migration Plan

None: no caller until change 2. Nothing in the tree changes behaviour.

## Open Questions

1. Owner: commit about 2,000 sections of our own Markdown, gzipped, as test data (D8a)? Recommended: yes.
2. Owner: should change 2 cap the words indexed per entity? Recommended: yes, a number measured in change 2.
3. Change 2: which predicates feed Name and Body, including file paths for R3.
