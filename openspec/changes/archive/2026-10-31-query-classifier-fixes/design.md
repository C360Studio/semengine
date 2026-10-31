# Design: query-classifier-fixes

Base: `a5f1d0e` on `d80acfd`. Inventory: `inventory.md` (`INVENTORY PASS`, round 1; its MEDIUM folded in). Track:
`design` (adds requirements to `query-classification`). Draft PR #480. Records: ADR-045 keeps `Classifier`'s
signature; ADR-074 (no predicate added); ADR-113 (no lifecycle here). Probes Q1-Q3 (inventory).

Terms: a *hint* is one entry of `ClassificationResult.Options`. An *operation* is one of seven: count, avg, sum, min,
max, similarity, ranking; each is its own operation (count and avg are two). A *trigger* is a match of one of the
keyword tier's existing or D1 patterns, nothing else, so no question main sent to tier 1 loses its answer; a *clause* is
text between `;`, `,`, `.`, `:`, `?`, `!` or the word `but`. An operation is *excluded* when its trigger is negated,
quoted or undecided.

## Context

On main every named case reproduces (Q1), query traffic moves scores (Q2), and big numbers give inverted ranges and
impossible boxes (Q3). Owner scope (#94 c6036107878, #95 c6036108178, #96 c6036129531, #9 c6522523604 answer 3, #479
c6525285377): tier-0 bugs, one failing test each, named cases as table tests, semselect cited only; fields and domain
vocabulary caller-owned; `total number of` stays count; a numeric extremum is not entity ranking.

## Goals / Non-Goals

Goals: #94, #95, #96's `GenerateQuery`, #478, #479 items 1 and 2. Non-goals: #479 item 3 (zone/area start node,
PR 9); a public ambiguity API; general paraphrase; negation of filters (time, place, path); a time-zone setting;
the BM25 statistics design (SemStreams #619); any accuracy claim beyond the tests.

## Decisions

**D1. Field binding and ordinary phrases (#94).** The field is the first word after the trigger, skipping an optional
`of` and `the` (R17 `pressure`, R21 `humidity`; "average temperature of sensors" stays `temperature`). `the number
of` selects count (R14); `a number of` does not. A numeric extremum is `smallest|lowest` (min) or
`largest|highest|greatest` (max), then one field word, then a value noun (`value(s)|reading(s)|measurement(s)|
level(s)`) (R23-R27). Not an extremum when `which`, `who` or `with` comes before it in the clause: no aggregation
hint, as today. The word lists are grammar, not domain words. Precedence stays count > avg > min > max > sum.

**D2. Exclusion (#95).** Each stage has one of three outcomes: hint, no hint, excluded (owner, #95 comment
6036108178). The keyword tier excludes an operation when its trigger (a) follows `not|no|never|don't|do not|without`
in the same clause (R28, R29), (b) lies inside a balanced pair of `"` or `“ ”` (R30), or (c) is one of two different
operations joined by `or` in one clause (R31). Exclusion is per operation: "Do not count sensors; average pressure"
gives avg/pressure. An excluded operation gives no hint; other hints stand (R32 keeps `path_intent`).

| Keyword outcome | Chain result |
| --- | --- |
| any hint | tier 0, those hints (unchanged) |
| no hint, nothing excluded | tiers 1, 3, default, as today |
| no hint, some operation excluded | the default result; tiers 1 and 3 are not asked |

Options. (B, recommended) Stop at the default result after an exclusion. Tier 1 answers in the caller's own words:
the pin's example files say counting as intent `aggregation` with `operation: count` (inventory § 2), and at
threshold 0.7 "Do not count devices in error state" matches such an example at 0.992 and "Not devices similar to
TEMP-001" a `similarity` example at 0.898 (reviewer's measurement). No framework rule can read a caller's words, so
only not asking binds the exclusion "regardless of which later stages are composed". Cost: an excluded-only question
loses tier 1/3 fallback; R28-R31 expect no hint anyway. (A) Ask later tiers and drop an answer that requests the
operation by framework hint names: misses both measured matches. (A+) Also drop a tier-1 match whose own example
query the keyword tier reads as the operation: misses examples worded outside the keyword patterns ("tally…").
(C) Strip excluded hints: leaves `Intent` naming the operation. `KeywordClassifier.ClassifyQuery` (ADR-045) keeps its
signature and omits excluded operations; the exclusion reaches the chain through an unexported method.

**D3. Read-only query embedding (#96).** `FindBestMatch` embeds the query with `GenerateQuery`; construction keeps
`Generate`. PR 9 composes no example classifier (pin `component.go:443`), so its behaviour does not change; a
product that composes one, as the pin's graph-gateway does (`:333`), sees scores stop drifting with traffic.

**D4. Typed and checked tier-1 hints (#478).** Options: (A, recommended) `NewEmbeddingClassifier` decodes each
example's options by unmarshalling them into `SearchOptions`, then sets each known name present with a plain
`switch` of 15 cases, one per JSON name (`strategy` as `string(...)`); `TestEveryHintNameIsTyped` walks the JSON
tags by reflection, in
the test only, so a field added without a `case` fails (inventory T1). Each known name present becomes the type
tiers 0 and 3 give it (`*TimeRange`, `*SpatialBounds`, `[]string`, `int`, `bool`, `string`); unknown names pass
through unchanged (caller-owned). Construction refuses, naming domain, example and option, a value that does not
decode, a `strategy` outside the eight values (pin `graphrag.go:651` maps it to graphrag), a `time_range` with a zero
or inverted end, a `geo_bounds` with North or South outside [-90, 90] or South above North, and a negative `limit`:
the checks tier 3 and D6 apply (fail closed, as owner answer 2 and `LoadDomainExamples`' refusal of bad JSON). The
classifier keeps its own copies. PR 9's pin reader (comma-ok assertions) reads tier 1 as tier 0, with no edit.
(B) A typed `*SearchOptions` on the result: plainest to read, but every pin reader ranges over the map and PR 9
rewrites ten reads. (C) Decode in `LoadDomainExamples`: misses examples built in code. (D) Do nothing.

**D5. One instant per question (#479 item 1).** The keyword tier computes every range from one `now` passed as an
argument to an unexported function; `ClassifyQuery` passes `time.Now()` once. Tests call that function with fixed
instants and zones (exact values, week and month edges, a DST day). Zone: the zone of `now` (`time.Local`; the
`time` package doc: `TZ`, else `/etc/localtime`, else UTC; not probed), declared in the README (owner question 1).
Rejected: a `Now func()` field (`model/breaker.go:37` is a default with a test override; back-door rule) and a
context value (rule from #91).

**D6. Out-of-range numbers are refused (#479 item 2).** Bounds: `last N hours/days` needs N ≥ 1 and N × unit
within `time.Duration`; `within R of lat,lon` needs lat in [-90, 90], lon in [-180, 180], R finite. Geometry, not a
refusal: North/South clamp to ±90; a box reaching a pole spans West -180 to East 180; one crossing the antimeridian
keeps East > 180 or West < -180, declared. Past a bound: (A, recommended) `ClassifierChain.ClassifyQuery` returns
`(*ClassificationResult, error)`, here no result and an `errs.WrapInvalid` error naming the phrase and bound; PR 9
returns it as it returns one for an empty query (pin `graphrag.go:736`). Fail closed, plain Go, found at compile
time; other outcomes keep a nil error (an ended context still gives `nil`). ADR-045 binds only `Classifier`, so
`KeywordClassifier.ClassifyQuery` just omits the hint, documented. Cost: the chain's signature and its pin callers.
(B) A refusal field on the result: a caller that forgets it widens silently. (C) Keep the impossible value: garbage
nobody observes. (D) Widen, log and count: no logger or registry here, and the pin falls to graphrag
(`graphrag.go:658-675`), answering a question nobody asked. Owner question 2.

## Proving tests

| Test | Proves | Fails on main |
| --- | --- | --- |
| `TestAggregationFieldBinding` | R17, R21, optional `of`/`the`, caller fields: exact type and field through `ClassifierChain.ClassifyQuery` | R17, R21 |
| `TestOrdinaryAggregationPhrases` | R14, R23-R27 through the chain; descriptive mentions activate nothing ("Show sensors with a number of faults", "Which pump has the highest pressure reading", "Show the sensor with the lowest battery level") | R14, R23-R27 |
| `TestKeywordExclusions` | R28-R31 give no hint; negation then affirmative; quoted name in a valid request; R32 | R28-R31 |
| `TestChainKeepsExclusions` | examples "Count devices in error state" (intent `aggregation`, `operation: count`) and "Find devices similar to TEMP-001" (intent `similarity`), a counting fake LLM: (1) `FindBestMatch` on "Do not count devices in error state" and "Not devices similar to TEMP-001" returns the unwanted example at or above the threshold; (2) the chain gives both the default result, no LLM call; (3) controls "Count devices in error state" and "Devices similar to TEMP-001" give tier 0 count and `use_embeddings` | (2) |
| `TestQueryEmbeddingIsReadOnly` | equal scores with and without intervening traffic; construction still learns | Q2 |
| `TestTierOneHintsAreTyped`, `TestEveryHintNameIsTyped` | every `SearchOptions` JSON name decodes to its tier-0/3 type; unknown names pass; each refusal of D4 | yes |
| `TestTimeRangesFromOneInstant` | exact ranges for fixed instants and zones | n/a (new seam) |
| `TestOutOfRangeNumbersAreRefused` | Q3 cases: chain gives no result and an error `errs.IsInvalid` holds, naming the phrase; `KeywordClassifier` gives no such hint; pole clamp answers normally | Q3 |

Examples suffice except clause splitting: `FuzzKeywordClassify` checks the spec's two laws (no panic; triggers only
inside quotes give no operation hint). Seed and run count in notes.

## Risks / Trade-offs

- Negation and alternatives are word rules: "not only count but average" reads count as negated. Declared in README.
- D4 refuses example files with a wrong-typed known name; pin test `TestFindBestMatch_PreservesOptions`
  (`"time_range": "week"`) is rewritten (notes).

## Issues and merge order

Closes #94, #95, #96, #478. #479 stays open for item 3 (PR 9); a comment names this PR for items 1-2. Stay: #32
(adapter home), #105, #405, #111 (PR 9). No file shared with #464, #465 or #476 today; if #476 lands a ledger edit
first, this PR merges `origin/main` and re-runs `task verify`.

## Open Questions (owner)

1. "Today" keeps the host zone, declared (recommended, as the pin), or is always UTC?
   Answered 2026-10-30: host zone (#479 comment 6525906611).
2. A question with an impossible number is refused with an invalid-input error the caller sees (D6 A, recommended),
   or runs without that filter, logged and counted (D6 D)? A changes the chain's signature and what PR 9 returns.
   Answered 2026-10-30: refused, D6 A (#479 comment 6527967116).
