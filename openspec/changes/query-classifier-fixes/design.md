# Design: query-classifier-fixes

Base: `a5f1d0e` on `d80acfd`. Inventory: `inventory.md` (round 1; this design is a draft until `INVENTORY PASS`).
Track: `design` (adds requirements to `query-classification`). Draft PR #480. Records: ADR-045 keeps `Classifier`'s
signature; ADR-074 (no predicate added); ADR-113 (no lifecycle here). Probes Q1-Q3 (inventory).

Terms: a *hint* is one entry of `ClassificationResult.Options`; an *operation* is an aggregation (count, avg, sum,
min, max), similarity, or ranking; a *trigger* is the word that requests one; a *clause* is text between `;`, `,`,
`.`, `:`, `?`, `!` or the word `but`. An operation is *excluded* when its trigger is negated, quoted or undecided.

## Context

On main every named case reproduces (Q1), query traffic moves later scores (Q2), and big or out-of-range numbers give
inverted ranges and impossible boxes (Q3). The owner set the scope (#94 comment 6036107878; #95 comment 6036108178; #96
comment 6036129531; #9 comment 6522523604 answer 3; #479 comment 6525285377): tier-0 bugs in the tier-0 stage,
each with its own failing test, named cases as plain table tests, semselect cited only. Fields and domain vocabulary
stay caller-owned; `total number of` stays count; a numeric extremum is not entity ranking.

## Goals / Non-Goals

Goals: #94, #95, #96's `GenerateQuery`, #478, #479 items 1 and 2. Non-goals: #479 item 3 (zone/area start node,
PR 9); a public ambiguity API; general paraphrase; negation of filters (time, place, path); a time-zone setting;
the BM25 statistics design (SemStreams #619); any accuracy claim beyond the tests.

## Decisions

**D1. Field binding and ordinary phrases (#94).** The field is the first word after the trigger, skipping an optional
`of` and `the` (R17 `pressure`, R21 `humidity`; "average temperature of sensors" stays `temperature`). `the number
of` selects count (R14); `a number of` does not. A numeric extremum is
`smallest|lowest` (min) or `largest|highest|greatest` (max), then one field word, then a value noun
(`value(s)|reading(s)|measurement(s)|level(s)`) (R23-R27). Not an extremum when `which`, `who` or `with` comes
before it in the clause ("which sensor has the highest temperature", "the sensor with the highest temperature
reading"): no aggregation hint, as today. The word lists are grammar, not domain words. Precedence stays count > avg >
min > max > sum. Option rejected: bare `smallest`/`highest` (the issue rules it out).

**D2. Exclusion (#95).** Each stage has one of three outcomes: hint, no hint, excluded (owner, #95 comment
6036108178). The keyword tier marks an operation excluded when its trigger (a) follows `not|no|never|don't|without|
do not` in the same clause (R28, R29), (b) lies inside a balanced pair of `"` or `“ ”` (R30), or (c) is one of two
different operations joined by `or` in one clause (R31). An excluded operation gives no hint; other hints stand
("Do not count sensors; average pressure" gives avg/pressure; R32 keeps `path_intent`). The chain then rules:

| Keyword outcome | Chain result |
| --- | --- |
| any hint | tier 0, those hints (unchanged) |
| no hint, nothing excluded | tiers 1, 3, default, as today |
| no hint, some operation excluded | tiers 1 and 3 run; an answer that requests an excluded operation is not used and the ladder goes on; else default |

"Requests" means: aggregation, an `aggregation_type` hint or `strategy` `aggregation`; similarity, `use_embeddings`
true or `strategy` `semantic`; ranking, `ranking_intent` true. Options: (A, recommended) drop a whole later answer
that requests an excluded operation: keeps useful fallback (#95 acceptance 5) and never returns half an answer with
a stale `Intent`; (B) skip tiers 1 and 3 after any exclusion: simpler, loses valid fallback; (C) strip the excluded
hints from a later answer: leaves `Intent` naming the excluded operation. `KeywordClassifier.ClassifyQuery`
(`Classifier`, ADR-045) keeps its signature and returns hints without the excluded operation; the exclusion reaches
the chain through an unexported method, so no new exported surface.

**D3. Read-only query embedding (#96).** `FindBestMatch` embeds the query with `GenerateQuery`; construction keeps
`Generate`. Composed behaviour changes nowhere: PR 9 composes no example classifier (pin `component.go:443`).

**D4. Typed tier-1 hints (#478).** Options: (A, recommended) `NewEmbeddingClassifier` decodes each example's known
hint names (the `SearchOptions` JSON names) into the Go type tiers 0 and 3 give them: `*TimeRange`, `*SpatialBounds`,
`[]string`, `int`, `bool`, `string` (`strategy` as `string`, one of the eight values). Unknown names pass through
unchanged (caller-owned: the pin's example files and governance adapter use their own keys, inventory § 2). A known
name whose value does not decode refuses construction with an error naming domain, example and hint. The classifier
keeps its own copies, so the caller's maps are no longer kept. PR 9's pin reader (comma-ok assertions, inventory § 2)
then reads tier 1 as it reads tier 0, with no edit. (B) `ClassificationResult` carries a typed `*SearchOptions`:
plainest to read, but changes the exported result every pin reader ranges over and makes PR 9 rewrite ten reads.
(C) decode at `LoadDomainExamples`: misses examples built in code. (D) do nothing: README keeps the split.

**D5. One instant per question (#479 item 1).** The keyword tier computes every range from one `now` passed as an
argument to an unexported function; `ClassifyQuery` passes `time.Now()` once. Tests call that function with fixed
instants and zones (exact values, week and month edges, a DST day). Zone: the zone of `now` (`time.Local`;
the `time` package doc: `TZ`, else `/etc/localtime`, else UTC; not probed), declared in the README (owner
question 1). Rejected: a `Now func()` field (the tree's one precedent, `model/breaker.go:37`, is a test
hook; back-door rule) and a context value (rule from #91).

**D6. Bounded numbers (#479 item 2).** A number that cannot give a true range gives no hint for its phrase, declared
in the doc (fail closed: no hint is the classifier's "refuse"; it has no error return). `last N hours/days`: N ≥ 1
and N × unit within `time.Duration`; else no time hint. `within R of lat,lon`: lat in [-90, 90], lon in [-180, 180]
and the radius finite, else no box; North and South are clamped to ±90; when the box reaches a pole or the longitude
half-width is 180° or more, West/East are -180/180. A box crossing the antimeridian keeps East > 180 or West < -180,
declared (owner question 2).

## Proving tests

| Test | Proves | Fails on main |
| --- | --- | --- |
| `TestAggregationFieldBinding` | R17, R21; optional `of`/`the`; caller fields | R17, R21 |
| `TestOrdinaryAggregationPhrases` | R14, R23, R24, R26, R27; `a number of`; `total number of`; entity ranking gives no extremum | R14, R23-R27 |
| `TestKeywordExclusions` | R28-R31 give no hint; controls (negation then affirmative, quoted name in a valid request, R32) | R28-R31 |
| `TestChainKeepsExclusions` | full chain with a counting fake LLM and an example classifier whose example would restore each excluded operation; a non-conflicting fallback still answers | yes |
| `TestQueryEmbeddingIsReadOnly` | equal scores and vectors before thresholding with and without intervening traffic; construction still learns (a one-example and a three-example classifier score differently) | Q2 |
| `TestTierOneHintsAreTyped`, `TestEveryHintNameIsTyped` | every `SearchOptions` JSON name, by reflection over the tags, decodes to its tier-0/3 type; unknown names pass; a bad value refuses construction | yes |
| `TestTimeRangesFromOneInstant` | exact ranges for fixed instants and zones | n/a (new seam) |
| `TestOutOfRangeNumbersGiveNoHint` | Q3 cases; pole clamp | Q3 |

Examples suffice except clause splitting, which a fuzz target `FuzzKeywordClassify` checks against two laws from the
spec: no panic, and a query with no trigger outside quotes yields no operation hint. Seed and run count in notes.

## Risks / Trade-offs

- Negation and alternatives are word rules: "not only count but average" reads count as negated (no count; avg
  stands). Declared in README; the tests pin the bound, not general language.
- D4 refuses example files that put a wrong type under a known name; the pin test `TestFindBestMatch_PreservesOptions`
  uses `"time_range": "week"` and is rewritten (notes).

## Issues and merge order

Closes #94, #95, #96, #478. #479 stays open for item 3 (PR 9); a comment records items 1-2 done. Stay: #32
(adapter home), #105, #405, #111 (PR 9). Merge order: no file shared with #464, #465 or #476 today; if #476 lands a
ledger edit first, this PR merges `origin/main` and re-runs `task verify`.

## Open Questions (owner)

1. "Today" keeps the host zone, declared (recommended), or is always UTC?
2. Out-of-range numbers give no hint (recommended), or are clamped to the nearest valid value?
3. A wrong-typed known hint in an example file refuses `NewEmbeddingClassifier` (recommended), or is dropped with a
   declared degrade?
