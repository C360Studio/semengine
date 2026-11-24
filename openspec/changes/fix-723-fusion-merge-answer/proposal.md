# Proposal: fix-723-fusion-merge-answer

Approved-by:

Track: `quick`. Capability: `fusion` (modified). Packages: `pkg/fusion`, `pkg/fusion/fusionnats`.

## Why

At tier 1, a fusion `nl` answer marked `provenance: lexical` can mean either of two things, and the caller cannot
tell which. The merge was honoured, but no result from the embedding search made the list. Or the embedding search
failed or ran late, and the answer is words only. graph-query's `mergedSearch` reply already says which one it was
(`merge`: `honoured`, `detail`, `reason`, `code`; `graph/query_merged_search_types.go:25-34`). `fusionnats` reads
the reply but keeps only a provenance label (`pkg/fusion/fusionnats/client.go:184-195`), and `fusion.Response` has
no field for the merge answer (`pkg/fusion/contract.go:145-200`). So an outage of the embedding provider cannot be
seen on SemSource's fusion tools (#723).

## What changes

- `pkg/fusion` gains `Merge`, a struct of its own with `Honoured bool`, `Detail`, `Reason`, and `Code string`
  (JSON `honoured`, `detail`, `reason`, `code`; the last three are omitted when empty). It is fusion's copy of
  graph-query's merge answer, without graph's `hint` name, which is always `semantic` here.
- `fusion.Resolution` gains `Merge *Merge`. `fusionnats`'s NL resolve fills it from the reply's `merge`, and leaves
  it nil when the reply has none.
- `fusion.Response` gains `Merge *Merge` (`merge,omitempty`). Every response built after an NL resolve answered
  carries the resolution's `Merge`: nodes, a miss, or a later deferral. This is the rule `bodies_not_indexed` and
  `excluded` already follow (`pkg/fusion/engine_lens.go:144-149`). A symbol or prefix resolve, a failed resolve, and
  a reply with no `merge` leave it out.
- The `fusion` Requirement "An NL answer says where its seeds came from" gains the merge answer and its scenarios.

## Non-goals

- Changing `provenance`. It still says where the returned seeds came from.
- Setting `Deferred` when the merge was not honoured. The word list is still an answer.
- Changing graph-query's merge answer or its `searchGraph` reply.

## Impact

Two new exported fields and one new exported type, all optional on the wire. At tier 0 the reply has no `merge`, so
the response is unchanged. One PR, `Closes #723`.
