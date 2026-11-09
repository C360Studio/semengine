# Proposal: setup-04a-04-graph-roots-graph-query-text

Approved-by: Coby Leuschke (owner), 2026-11-07, PR #544 comment 6711659719:
D1 option (a), D7 accepted, all new exports accepted.

Track: `design` (adds spec requirements and touches three capabilities).

## Why

Step 5 of five of `processor/graph-query` (change 4 item 9, epic #9; `split.md` of the archived step 1 change): after
it "the package is whole". Step 4 answers the questions with an exact answer and refuses the rest with a temporary
code, `text_search_unsupported`. SemSource's one search call, `graph.query.searchGraph`, has no responder. The pin's
text tier reads five groups even when none matches, loads up to 10,000 members, falls back from a failed semantic call
without saying so, and names entities through a hard-coded label list that imports agentic predicates (#31). The
owner's ruling on the port's shape: "our job when we port is to create a pragamtic, idiomatic solution while noting
what consumer use cses we need to support" (PR #504 comment 6588079803).

## What Changes

- **Text tier** (`strategy` `text`): a question with no hint, or a one-word reference naming no entity (and, under
  the recommended option, a path-word, similarity or ranking question, the dropped hint named), reads one
  enumeration of `COMMUNITY_INDEX` at `level` (0 to 2), up to five groups whose record matches, one prefix page each,
  and answers the matching entities, best first, each as stored with its label, beside the groups' records and a plain
  answer. Every cut is declared (`entities_truncated`, `unreadable_groups`) and counted.
- **Refusals that replace the temporary code:** a count or other aggregation is `invalid_request` naming it (under
  option (b), path-word, similarity and ranking questions too); a named start naming no entity is `entity_not_found`;
  an unusable view or an empty level is `index_not_ready` (so every text question until change 7 writes the index).
- **`searchGraph`:** its own verb row, answering as `globalSearch`. The pin's semantic fallback is not carried.
- **`graph/clustering`:** `LevelPrefix`, the one home of `{level}.` and the range 0 to 2.
- **`pkg/graphview`:** `View.ListCounted`, which lists and counts poisoned keys under one lock (Fable ruling 3).
- Labels read the vocabulary's label predicates (`vocabulary.DiscoverLabelPredicates`), as graph-index's name index
  does (#31).
- Consumer breaks in `processor/graph-query/MIGRATION.md`, including SemSource's `searchGraph` decode and its
  `summarize_threshold` key, now refused by name.

Not here: a semantic tier or fallback (change 5), LLM answers and summaries (#485-#487), a keyword index (#506).

## Capabilities

- Added requirements: `graph-query` (the text tier's routing, groups, matches, reply, revision, refusals;
  `searchGraph`); `graph-view-subscription` (a counting enumeration).
- Modified: `graph-query` ("Refusals that are not errors", "Global search answers by its hints", "A one-word question
  is an entity reference", "Global search refuses what no strategy can honour", "One resolver reads an entity
  reference", "A strategy answers with its own result", "A strategy's revision is its read's", "Truncated searches are
  counted"); removed:
  "Questions needing the text search are refused until it exists"; `graph-transport-boundary` (the table gains
  `searchGraph`).

## Impact

- About 420 production and 1,050 test lines (hand estimate); pin read about 715 and 890 (`inventory.md` § 9). One PR,
  one developer unit per task.
- **New exported surface, for the owner to accept:** in `graph`, `TextSearchData`, `TextSearchGroup`,
  `TextSearchMatch`, `GlobalSearchData.Text`, `GlobalSearchRequest.Level`; in `graph/clustering`, `LevelPrefix`; in
  `pkg/graphview`, `View.ListCounted`. New
  subject `graph.query.searchGraph`. No new bucket, metric or config key.
- **For the owner:** (1) design D1: how free-text questions holding the classifier's trigger words ("connection",
  "path", "like", "top 5", "area 51") are treated, option (a) recommended; the spec deltas follow (a) and change with
  the ruling. (2) ADR-114's step 4 sentence narrows (design D7): a named start naming no entity is
  `entity_not_found` rather than a text search.
- Retires #103 (its generation marker stays with #15), #106, #31. No code before the owner approves.
