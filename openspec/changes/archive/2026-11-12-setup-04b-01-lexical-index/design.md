# Design: setup-04b-01-lexical-index

## Context

Facts are in `inventory.md` (passed at `a1783aa1`), cited by section. Tier 0 search today has three tokenizers, four
scorers and no word index (§1, §2). The embedder's statistics are learned per process and never subtracted (§2), the
cause of the order and restart drift SemSource's R1 names (§5). An entity's text is picked by predicate suffix and
its offloaded body is fetched by the caller (§3). Entities with no text are reached only by structure (§4). The
package is new, has no ledger row, and is public, so it is framework API (§6, §7). Memory was measured (§8).
Probes under `notes/probes/` back the choices below: `sequence-probe`, `tokenizer-probe-2` and `word-shape-probe`
(committed), and `tokenizer-law-probe`, `corpus-size-probe` and `and-query-probe` (added with this revision), each
with its source and log.

## Goals / Non-Goals

Goals: one pure package, `graph/lexical`, that ranks by BM25 over the entities it holds, answers words, `"phrases"`
and `-excluded` words, ranks name words above body words, and proves it with laws and a scorecard. Each of the four
"Room to extend" points in the proposal, plus `similar`, can be added later with no change for existing callers.

Non-goals: callers (changes 2 to 4), a stored index, a per-entity token record, typo tolerance, word splitting for
scripts written without spaces (Chinese, Japanese), Unicode normalisation (a decomposed `é` splits its word), and
any help for entities without text (README says so, D9).

## Decisions

**D1. Surface.** `NewIndex() *Index`; `Put(entityID string, doc Document) error`, which replaces what the entity
held; `Delete(entityID string)`; `Len() int`; `ParseQuery(text string) (Query, error)`;
`Search(q Query, opts Options) (Result, error)`. `Document{Name, Body string}`. `Options{Scope []string; Limit int}`.
`Result{Hits []Hit; Matched int}`, `Hit{EntityID string; Score float64}`. `Tokens(text string) []string` is the shared
tokenizer. Put refuses an invalid entity ID (`ErrInvalidEntityID`), so a held entity always matches a scope correctly
(`graph.MatchesAnyIDPrefix` fails closed on invalid IDs, inventory §2). A document with no words removes the entity.
Search refuses a malformed scope prefix (`ErrInvalidScope`) or a limit below 1 (`ErrInvalidLimit`). ParseQuery
refuses a query with nothing to search for (`ErrEmptyQuery`), reserved syntax (`ErrReservedSyntax`, D7) and more
than `graph.MaxSearchTerms` (16) distinct words, counting plain, phrase and excluded words after tokenizing
(`ErrQueryTooLong`). That is the bound local search already enforces, though it counts white-space terms there
(`graph/query_local_search_types.go:12`). Change 3 moves local search onto this index and updates the constant's
doc comment, so one spelling serves both. Hits are ordered by score,
then ID. `Matched` counts every match in scope before the limit, for truncation reporting and rank fusion.
Options considered: (a) Put takes text in fields (chosen): one call, and the index does the tokenizing.
(b) Put takes already-split words: two calls every caller must pair, and they can pair them wrongly. Option (a)
leaves room for (b) later as a second write method that sits beside Put (D7). `Query` keeps its fields unexported,
so adding a query form adds no field any caller must set.

**D2. A search during a write.** (a) `Index` is safe for concurrent use. Each Put or Delete is atomic: a search
answers as if it ran wholly before or wholly after each write (chosen; proved by `TestSearchDuringWrites` under
`-race` against two answers worked out beforehand). (b) The caller locks: change 2 would then have to know to lock
(adopter seam, inventory §7). (c) Copy-on-write snapshots: memory doubles while a write runs. The lock is the
developer's choice.

**D3. Tokenizer.** The smallest rule set that meets the laws. The exact rules, their order and their witnesses are
in the spec. The order was run, not worked by hand: `tokenizer-law-probe` passes the witnesses and 32.4 million fuzz
inputs. The order is: split into runs, split identifiers, lower-case, fold, then drop.

- A word is a run of Unicode letters and digits, so `graph.query.status`, `pump-7` and `_test.go` split into
  separate words. One-letter words stay (`-v`, `7`). A capital is a letter that lower-casing changes. This
  definition came from the fuzz run: a capital with no lower-case form split again on a second pass.
- Identifiers split at a lower-to-upper step and before the last capital of a capital run (`withRetry` is held as
  `retry`). The joined form is not held, so `withretry` finds nothing; the spec, the README and a behaviour question
  pin this. Holding it would add 8.3% more words and 14,899 distinct forms beside 10,504 on our Go files, and it
  would have to stay out of the word sequence (D4). It can be added later as an internal change.
- Length: a folded word over 32 bytes is dropped. Capping at 32 before splitting loses 2,263 of 41,864 runs (every
  long test or type name); capping after loses 345, all lower-case hex; 64 before and 32 after loses 402
  (`word-shape-probe.log`). Random base64 becomes single-entity junk words under every option (memory only).
- Stopwords: the 30 words of `graph/clustering/terms.go:16`. A word is dropped when it or its fold is one, so
  `wills` and `does` are dropped and tokenizing stays idempotent. `tokenizer-probe.log` measured the embedder's
  84-word list, which drops `not`, `no`, `get`, `find`, `show` and `all`. The 30-word list was measured in
  `tokenizer-probe-2.log`: R@10 36.6% against 37.0%, 3,709 matches per query against 3,341. Change 4 points
  clustering at this list.
- Plurals: the plural-only stemmer (`tokenizer-probe.go.txt:11`) with two rules ahead of it (strip `es`, or a final
  `e`, after `ss`, `x`, `ch` or `sh`). Unfolded corpus pairs drop from 117 to 54 (`word-shape-probe.log`).
  `tokenizer-probe-2.log` measured an earlier variant (strip `es` after `ss`, `x`, `ch`, `sh` or `z`, no final-`e`
  rule), not this pair: R@10 was unchanged for it (36.6% against 36.6%). The shipped pair is measured by the
  scorecard (D8): R@10 56.0%, against 55.8% without the two rules (`notes/2.4.md`). `-us` and `-s` words (bus,
  status) still do not fold. Rejected: the embedder's stemmer (it splits 545 pairs) and a dictionary stemmer (a new
  dependency for about one point).

**D4. Phrases.** (a) Each entity keeps its word sequence (word numbers, repeats included) in memory. A phrase is
first narrowed to the entities holding all its words, then checked for adjacency in their sequences (chosen).
Measured at one million entities: 472 B per entity against 471 B with no sequence (20 words), and 1,590 B against
1,495 B (120 words) (`sequence-probe.log`). The index needs these lists anyway, to delete an entity's words.
(b) Positions in every posting: 1,186 B (20 words) and about 5.2 KB (120 words) (`memory-probe.log`).
(c) Re-read the text of the best candidates: the index would have to read storage, including offloaded bodies, which
the proposal rules out. Option (a) still allows (b) later as an internal change if long-text phrase checks prove slow
(D7). Phrases are compared after tokenizing, within one field: `"out of memory"` also matches "out the memory".
The spec and the README declare this.

**D4b. Combining forms.** Take `retry "docker compose" -test`. Three options:

- (a) Every plain word, phrase and exclusion is required. This is precise, but the answer to a natural question must
  then hold every word: 6.5% of the probes' known-item questions (`and-query-probe.log`).
- (b) Phrases and exclusions filter, and plain words rank (chosen).
- (c) A phrase only adds a bonus. This needs a second tuned weight, and it still returns passages that merely share
  the phrase's words, which R2 asks to push down.

The observable rule, its witnesses and the generated check over combined forms are in the spec ("Phrases and
exclusions filter, words rank") and task 2.3.

**D5. Exclusion.** `-x` removes every entity that holds `x`'s words, adjacent, in any field (Name or Body). `-test`
drops an entity whose Body holds `foo_test.go`, because `_test.go` splits into `test` and `go`. What goes into the
Body (a file path, as R3 needs) is change 2's mapping of predicates to fields. (a) Fields only (chosen).
(b) Also index the entity ID's segments: rejected, because instance segments are often hashes and an ID's meaning is
the product's (`AGENTS.md`, What this is for).

**D6. Scoring and the name weight.** Field-weighted BM25 (BM25F): term frequency is `w·tf(Name) + tf(Body)`, length
is `w·len(Name) + len(Body)` counted in held words after tokenizing, and the average length is taken over that same
weighted length. It uses Lucene's IDF `ln(1 + (N − df + 0.5)/(df + 0.5))` (never negative), `k1` 1.2 and
`b` 0.75, the probe's reference (`retrieval-probe.go.txt:85`, `:112`). N, df and average length are read from the
index at search time, so equal held sets give equal scores. Scope filters before the limit and never changes a
score. Options: (a) BM25F (chosen); (b) two BM25 scores summed: saturates twice, and a name match can lose to a
long body; (c) a flat bonus for a name match: a second unit of score, with no length normalisation. Value of `w`:
the exported constant `NameWeight` is its one home. Task 2.4 sweeps 2, 3 and 5 on the scorecard, reports 1 for
comparison (it ties a name match with a body match, so it cannot pass `TestNameOutranksBody`), and keeps the smallest
value within one point of the best R@10. The results go in `notes/2.4.md`, and the value is written here:
`NameWeight` is 2. On the scorecard, R@10 is 56.0% at 1 and 2 and 55.6% at 3 and 5, and R@1 falls as the weight
rises (25.4%, 24.6%, 23.0%, 22.0%). There, Name is the file's first heading, shared by every section of the file,
so the sweep shows a name weight costs little on this corpus, not that it helps; SemSource's runner is the check
with real entity names.

**D7. Room to extend.** Each later addition is a new method or an internal change; no caller and no scorecard
question changes.

- **Token record:** add `Words(id)` to read an entity's sequences and `PutWords(id, ...)` to restore them. A stored
  record would then also hold a tokenizer version.
- **Positions (D4, option b):** internal; the phrase law test does not see the layout.
- **Typo tolerance and query forms:** a new form adds an unexported field to `Query`, and the word table is
  internal. So that a later form changes no answer given today, its syntax (`name:`, `body:`, a letter or digit
  followed by `*` or `~`) is refused now. A single leading `-` excludes. Bare `AND`, `OR` and `NOT` are words, which
  forecloses word-shaped operators. The exact rules and witnesses are in the spec ("Syntax kept for later query
  forms is refused", "A query holds words, quoted phrases and excluded words").
- **Tier 1 vectors by rank:** `Result` gives a total order and `Matched`, which is what rank fusion reads.
- **Entity-to-entity `similar` (change 4):** `SimilarTo(id, Options)` can build a query from the entity's own held
  words.

**D8. Scorecard.** (a) A frozen known-item set: 2,000 sections of this repository's Markdown at `2853486d`, each
file's first heading as Name and the section as Body, with 500 section headings as questions. It is kept in
`graph/lexical/testdata/scorecard/` and gzipped (0.97 MB measured; open question 1). The floor is the measured R@10
minus one point. It lives as a constant in `TestScorecardHoldsItsFloor`, and task 2.4 also records its value here:
R@10 measured 56.0% (280 of 500) at `NameWeight` 2, so the floor is 55.0%. The probe's plain BM25 (the embedder's
tokens, Body only) scores 52.6% on the same corpus and questions (`notes/2.4.md`).
It is shown able to fail with `task mutate:check` and an IDF-is-one mutant (notes/2.4.md).
(b) About 30 behaviour questions in SemSource's `questions.json` shape: phrases, flags, dotted names, `-test`,
identifiers. Every one must pass. SemSource's 21.6k corpus and 33 questions stay in their repository, for their
runner against the composed stack once change 2 serves the verb. Rejected: a generated corpus (it proves the
arithmetic, which D10's reference model already does, not quality) and copying SemSource's corpus (too large, and
their data).

**D9. README.** It states what word search does not help with (telemetry and other entities without text) and where
those entities are reached instead; the content is task 3.1's acceptance check.

**D10. Rules (inventory §6).** The no-NATS guard sits in `internal/harness/contract`; there are two fuzz targets, a
`scripts/cover-check.sh` line, and Rapid properties against a naive reference model with named witnesses first. The
framework-API contract line is the spec plus the README, and the product test is `ExampleIndex`. Each test is named
in the spec and tasks 2.1 to 2.5.

## Risks / Trade-offs

- Memory is about 1.6 KB per entity at 120 words; text up to the 4,000-character cap could be several times that.
  Query time is not measured. Change 2 measures both and chooses any per-entity word cap.
- The name weight and the floor are tuned on our own Markdown, so they may overfit it; SemSource's runner is the
  outside check.
- The tokenizer becomes part of a stored format once a token record exists (D7 records a version then).
- Stopword removal makes phrases match across removed words (D4).

## Migration Plan

None: no caller until change 2. Nothing in the tree changes behaviour.

## Open Questions

1. Owner: may we commit 2,000 sections of our own Markdown as frozen test data (D8a)? It is 2.6 MB as JSON lines,
   0.97 MB gzipped, and 668 of the sections cite issue or pull request numbers (`corpus-size-probe.log`). That is
   the port history #606 is removing from the live docs; the frozen copy keeps it as it was at `2853486d`.
   Recommended: yes. A generated corpus cannot measure quality.
2. Change 2: which predicates feed Name and Body, including file paths for R3; and a per-entity word cap, chosen by
   measurement on SemSource data. Change 3: the doc comment of `graph.MaxSearchTerms` (D1).
