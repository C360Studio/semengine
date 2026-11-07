# Design: `processor/graph-query` step 5, the text tier and `searchGraph`

Base `6808234`; pin `8b99efe9`. Evidence: `inventory.md` (P1-P3, R1-R6, §§ 1-9; INVENTORY PASS, round 1, with its
fold-ins), archived step 1 `inventory.md` and `split.md`. Binding: Fable rulings 2 and 3 as accepted (PR #504 comments
6582751783, 6582752155, 6585009466), PR #504 comment 6588079803, #487 note 2 (comment 6688945537), #535 ruling
6706930613, #9 comment 6386603659, ADR-087 D5, ADR-099, ADR-109 D4, ADR-113, ADR-114. **Goal.** A free-text question
gets the matching entities of at most five groups, each entity's raw facts with its name beside its ID, the groups'
summaries beside them, and every cut declared; `searchGraph` answers as `globalSearch`; the package is whole.
**Non-goals:** a semantic tier or fallback (change 5), LLM answers or summaries (#485-#487), a keyword index (#506).
Pin read about 715 + 890 lines (inventory § 9); this about 420 + 1,050 (estimate), one PR.

## D1. Which questions reach the text tier (owner question)

The keyword classifier finds hints by trigger words (`graph/query/classifier.go:48-49`, `:56`, `:64-73`, `:89-94`):
`connect*`, `relat*`, `path*`, `link*`, `between` (path); `similar*`, `like`, `resembl*`, `compar*`; `how many`,
`count`, `total`, `sum`, `average`, `mean`, `min`, `max` and the like; `top N`, `bottom N`, `most`, `least`; `zone X`,
`area X`. On SemSource's one search call these are ordinary words: "http connection pool" and "file path handling"
read as path questions with no start, "max retries" as an aggregation, "pumps in area 51" as a path from `area-51`.
The ruling on #535 accepted only the loud "two places" refusals, so how the text tier treats the rest is the owner's.
Fixed either way: several words and no hint, and a one-word reference naming no entity, go to the text tier; two
filters and the classifier's own refusals stay `invalid_request` (step 4).

| Option | Path words, no start; similarity; ranking (alone) | Any of those beside a filter (start, time range, area), or a start with path predicates | Count or other aggregation | Named start that names no entity | Cost |
| --- | --- | --- | --- | --- | --- |
| **(a) recommended** | text tier; the reply's `unhonoured_hints` names each (`path`, `similarity`, `ranking`) | `invalid_request` naming both, as step 4 refused two filters | `invalid_request` naming it | `entity_not_found` | "max retries" and "total memory usage" are still refused; "pumps in area 51" is not found (a zone named as a start). One new reply field |
| (b) refuse | `invalid_request` naming the hint | `invalid_request` naming both | `invalid_request` | `entity_not_found` | Common words refuse answerable free text on SemSource's call; `MIGRATION.md` lists every trigger word |

A filter is never dropped for a text answer: "top 5 sensors connected to pump-42" (ranking and a start), "find sensors
similar to pump-42 in the last 2 hours" (similarity and a time range) and "what is related to it in the last 2 hours"
(path words with no start, and a time range) are refused `invalid_request` naming both, under either option.

A count stays refused under both: a list of matches over sampled pages (ruling 2) invites the reader to count a
sample. The pin (everything to `graphrag`, silently, R3) and keeping the temporary code (the package never whole) are
not offered. A named start that names nothing (`what is connected to pump-99`) is `entity_not_found` in both, as
`entityByAlias` answers it: a text search would drop the walk. This narrows ADR-114's step 4 sentence (D7) and
replaces #479's scenario: "the area of the plant" (start `area-of`) is `entity_not_found`, never empty.
*Proof:* `TestGlobalSearchRoutesByHints` (each row, with "http connection pool", "file path handling", "max retries"),
`TestGlobalSearchTextQuestion`, `TestGlobalSearchUnknownWordIsSearched`, `TestGlobalSearchUnknownPathStart`,
`TestZoneQuestionNeverAnswersEmpty` (now `entity_not_found`), `TestGlobalSearchHintBesideAFilter`.

## D2. Which groups are read

**Request keys:** `query`, `min_revision`, and `level`, 0 to 2, omitted 0, read only by the text tier and checked for
every request before routing. `max_communities`, `summarize_threshold` and the three `include_*` keys stay refused by
name (D4). A caller's error (a bad key or level, more than `graph.MaxSearchTerms` distinct terms) is refused
before the view is read, so it wins over `index_not_ready`. One enumeration per request (ruling 3):
`View.ListCounted` (D8) over `clustering.LevelPrefix(level)`, a new
`graph/clustering` function returning `{level}.` and refusing a level outside 0 to 2: the one home of both, beside
`GroupKey`, so graph-query never spells `levels` or `communityKey` again (inventory § 5). Inside clustering the
prefix is spelled at `storage.go:138` (`communityKey`) and `maintainer.go:329` (`exists`), the range at
`storage.go:102` (`GroupKey`) and `:124` (`DecodeCommunity`); task 2.1 moves each onto `LevelPrefix` and one range
check. A view missing, catching up,
lost or stopped, and a level with no records, readable or not, refuse transient `index_not_ready` (ruling 3;
ADR-109 D4). **Score** (the pin's, R6): over the question's terms, lowercased and de-duplicated as `searchTerms` does,
a record gains 2.0 for each term contained in its statistical summary and 1.5 for each pair of keyword and term where
one contains the other, all lowercased. Up to five records scoring above 0 are read, best first, ties by key.

| Option | Cost |
| --- | --- |
| (a) the pin: the top five even at score 0 (P1) | Reads five groups chosen by input order for a question none matches |
| **(b) score above 0 only** | A match in a group whose ten keywords miss the term is not found; #506 is the fix |
| (c) score the question's words as the keywords were made (`clustering.extractWords`) | Better ranking, another export; optional, with #506 |

No record scoring: no groups, no matches, and the answer says no group at that level matched. *Proof:*
`TestTextTierReadsAtMostFiveGroups` (seven matching groups, five prefix requests, a counting subscriber: ruling 2's
test), `TestTextTierSkipsUnmatchedGroups`, `TestTextTierScore` (the rule's four cases, P2's records),
`TestTextTierRefusesAnEmptyLevel`, `TestTextTierReadsTheAskedLevel`, `TestLevelPrefix`.

## D3. Matches

Each selected group: one `graph.ingest.query.prefix` page at `MaxPrefixQueryLimit` (`readCountPage`), ranked by
`rankEntities` with the question's terms (the 0.3 score `localSearch` uses, one home). `rankEntities` returns each
match's ID and score as well as its bytes; `localSearch`'s behaviour and reply are unchanged. The groups at one level
are disjoint (a group is a function of the ID), so the merged list is the union: score, then ID. More than
`graph.MaxSearchTerms` distinct terms is `invalid_request`. *Proof:* `TestTextTierMatchesEqualAFilter`,
`TestTextTierMarksACutPage`, the `localSearch` tests unchanged.

## D4. Reply

**Reply fields:** `graph.GlobalSearchData` gains `text`, a `graph.TextSearchData`: `level`; `groups`, each record's
`community_id`, `member_count`, `community_summary` and `keywords` (what the answer reads); `matches`, best first,
each `id`, `label`, `score` and `entity`, the page's bytes unchanged; `entities_scanned`; `entities_truncated`;
`unreadable_groups`; under option (a) `unhonoured_hints`; `answer`. This is #487 note 2: one-hop facts as stored, the
entity's name beside its ID, summaries beside the facts and never instead. **Label:** graph-query reads
`vocabulary.DiscoverLabelPredicates()` once at `Start`, as graph-index does (`index.go:85-88`). An entity's label is
the first non-empty string value, in triple order, of its most salient label predicate: the lowest priority number,
ties by predicate name. None: omitted. The pin's list (R5) is a second spelling of that set, its agentic entries
are #31, and its last-resort "first string value" is a guess. **Fit:** over `max_payload`, the lowest-ranked matches are
left out as `localSearch` does, `entities_truncated` set, `search_truncated_total{verb}` raised for the verb asked.
**Answer**, a plain function: `Found {n} matching entities among {entities_scanned} read from {g} groups at level
{level}.`; when `entities_truncated`, `This is a sample: a group had more than one page, or matches were left out to
fit.`; per group `{community_id}: {community_summary}` (or `It has {member_count} members. Key themes: {first five
keywords}.`); when `unhonoured_hints`, `Not honoured: {hints}.`; when `u` > 0, `{u} group records at this level
could not be read.` SemSource reads the answer for its disclosure (`MIGRATION.md:28`). **Not carried:** auto-summary,
`include_summaries`, `include_relationships` (the edges are the matches' triples), `include_sources` (a match's group
is its ID's prefix, its score is on it), `max_communities` (five, ruling 2), digests and representative tags (#487),
`rep_entities` and a per-group score (no reader), `count`, `duration_ms`, `degraded*`, `answer_model`. *Proof:*
`TestTextTierReply` (literal JSON), `TestMatchLabel` (title; a more salient registered label; two values of one
predicate; an agentic predicate not read; none), `TestTextTierFitsMaxPayload`, `TestTruncatedSearchIsCounted`,
`TestTextTierAnswer` (each sentence).

## D5. Revision and unreadable records

`indexed_revision` is the highest `entity_revision` among the level's readable records. An applied write raises its
groups' records at every level (`graph/clustering/maintainer.go:173-184`), so a `min_revision` from it is reached
after the tick that publishes it, unlike `localSearch`'s per-group rule (`MIGRATION.md`). Exception, declared: a
delete that empties its group removes the record and raises none (`maintainer.go:263-275`, read). A `min_revision`
above the mark refuses transient `revision_not_reached`, detail `indexed_revision`, before any page read. An
unreadable record's revision is unknown, so it never counts toward the mark: if it held the highest, the mark is
lower and a request may be refused that could have been answered, never the reverse. Every record at the level
unreadable: transient `internal` naming the count, one error; no 0 stands for unknown. During a tick another group
may lag (ruling 3, declared); the matches come from graph-ingest, current. The rule leans on the maintainer taking
writes in revision order (#525). *Proof:* `TestTextTierRevision`, `TestTextTierRevisionIgnoresUnreadable`,
`TestTextTierAllUnreadable`, `globalSearch` and `searchGraph` in `TestMinRevisionRefusalThroughGraphQuery`.

## D6. Refusals and counting

| Case | Caller receives | Error in `Health` |
| --- | --- | --- |
| Bad request, a refused hint, over 16 terms | `invalid_request` | no |
| A named start that names no entity | `entity_not_found` | no |
| View not usable; level with no records | transient `index_not_ready` | no |
| `min_revision` above the mark | transient `revision_not_reached` | no |
| Some records unreadable | an answer, `unreadable_groups` set | yes, one |
| Every record unreadable; page broken; no responder; no fit | transient `internal` | yes, one |

Until change 7 writes `COMMUNITY_INDEX`, every text question is `index_not_ready` (`split.md`). *Proof:*
`TestComposedVerbsCountAsForwardsDo`, `TestTextTierPoisonIsDeclared` (an answer with `unreadable_groups` 1, one
error; healed after a valid write).

## D7. `searchGraph`, the semantic fallback, ADR-114

`searchGraph` is its own row and the same handler: its `data` equals `globalSearch`'s for the same request. The pin's
semantic fallback (R4) is not carried: it swallowed the semantic error (S14), and it also answered over a missing
community index (`pin:searchgraph.go:70-80`). Change 5 decides the semantic tier of the text strategy (R3), which is
where that case belongs; until then a tier-1 deployment's `searchGraph` refuses `index_not_ready` where the pin
answered from embeddings (`MIGRATION.md`). Options: (b) register `searchGraph` at change 5, leaving SemSource's call
with no responder a step longer; (c) port the fallback refusing until change 5, refusing honest empty answers.
ADR-114's sentence becomes: "In global search a one-word reference naming no entity is searched as text; a named start
naming no entity is `entity_not_found` (change `setup-04a-04-graph-roots-graph-query-text`, design D1)." *Proof:*
`TestSearchGraphAnswersAsGlobalSearch`, the verb table tests.

## D8. The counting enumeration in `pkg/graphview`

`View.ListCounted(prefix string) ([]KeyedEntry[T], int, error)`: what `List(prefix, 0)` returns, sorted by key, and
the number of poisoned keys under prefix, under one hold of the view's lock, with `List`'s readiness errors. Entries
are copies of the stored values, which an apply replaces whole and never edits (`pkg/graphview/view.go:611`); callers
read them only. The pin's `View.Poisoned` (inventory § 5) takes a separate lock and cannot pair with a listing; a
hook-fed count misses heals; `SnapshotAndSubscribe` copies the whole view (ruling 3). One reader. *Proof:*
`TestListCountedCountsPoison` (poison, heal, delete, readiness), `-race`.

## D9. ADRs, issues, order

ADR-087 D5 (readiness per request through the view), ADR-099 (prefix lane, one page), ADR-081, ADR-085, ADR-109 D4,
ADR-113 (no new goroutine), ADR-114 (D7) applied. ADR-062 and ADR-071 are change 5's; its fusion port is told the
`entity_digests` its BM25 adapter read are gone (`text.matches` carries ID, label, score). Retires #103 (generation
marker with #15), #106, #31. Stay: inventory § 4. Merge order: #543, then this PR (merging `origin/main`); both before
changes 5 and 7.
