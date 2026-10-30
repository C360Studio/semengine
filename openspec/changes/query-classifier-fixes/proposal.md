# Proposal: query-classifier-fixes

## Why

`graph/query` merged on main as a whole port (PR #475, `d80acfd`). The owner placed its classifier repairs in one
follow-up "so the diff from the pin is the fix alone and the cross-agent review reads one thing. One change
for #94, #95 and (pending review) #96, since all three touch the classifier chain; each keeps its own failing test" (#94
comment 6036107878), then added #478 (#9 comment 6522523604, answer 3) and #479 items 1 and 2 (#479 comment
6525285377). Each is a tier-0 bug in the tier-0 stage, and graph is "our secret sauce. it needs to be solid". Every
named case reproduces on main (inventory Q1-Q3).

Approved-by:

## What Changes

- #94: the aggregation field skips an optional `of`/`the` (R17, R21); `the number of` selects count (R14); a
  numeric extremum (`lowest pressure reading`) selects min/max with its field (R23-R27), while entity ranking
  (`which sensor has the highest temperature`) does not. `total number of` stays count. No domain words.
- #95: an operation whose trigger is negated, quoted or one of two undecided alternatives gives no hint; when
  that leaves no hint the chain answers the default result and asks no later tier, so none can restore it in
  any words (R28-R31). A clear affirmative clause still counts, and R32 keeps its path intent.
- #96: the example classifier embeds a query with the read-only `GenerateQuery`, so traffic no longer moves scores.
- #478: tier-1 hints with a known name carry the same Go types as tiers 0 and 3; a value that does not decode,
  an unknown strategy, an inverted time range or an impossible box refuses `NewEmbeddingClassifier`; unknown
  names pass through.
- #479 items 1-2: every range in a question comes from one instant, testable with fixed instants and zones; a
  `last N` that overflows and an out-of-range latitude or longitude give no hint
  (declared: the search widens); boxes are clamped at the poles.
- README and ledger row say what ships; the README's tier-1 type column and "Limits" lines go.

## Capabilities

- Modified `query-classification` (one requirement modified, nine added).

## Impact

- `graph/query` only: `classifier.go`, `classifier_chain.go`, `classifier_embedding.go`, `README.md`, `doc.go`, tests;
  `docs/admission-ledger.yaml` (`graph/query` row). No new exported symbol, import, subject, bucket or goroutine.
- Changed behaviour of framework API: `NewEmbeddingClassifier` refuses a wrong-typed known hint and copies example
  options; tier-1 values change type; tier 0 drops excluded operations. No in-tree importer; PR 9 reads tier 1 with
  the pin's own assertions.
- Size: about 300 production lines and 900 test lines; one pull request.
- Issues: closes #94, #95, #96, #478; #479 stays open for item 3 (change 4 PR 9).
- Owner question: design "Open Questions" 1 (host zone or UTC).
