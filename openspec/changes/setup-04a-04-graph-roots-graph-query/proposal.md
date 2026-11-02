# Proposal: setup-04a-04-graph-roots-graph-query

Approved-by:

## Why

Change 4 of epic #9 ports the graph roots in the owner's order (#9 comment 6386856657); item 9, `processor/graph-query`,
is the last. The owner, 2026-11-02: "graph-query is essential to our success - many lessons learned in semstreams gets
us pretty close but does not mean we can not improve from ground up." The rulings on PR #504 (comments 6582751320,
6582751783, 6582752155, 6585009466) settled the reply contract and removed the pin's community cache, member walks and
answer-writer interface, so the port splits into five steps (`split.md`). This change is step 1: the component and the
six direct lookups, the first thing to answer on `graph.query.*`.

## What Changes

- New `processor/graph-query` on `component.Lifecycle` (ADR-108, ADR-113), with strict configuration. It answers
  `entity`, `batch`, `prefix`, `entityByAlias`, `relationships` and `byName`.
- `graph.QueryVerbs()` gains those six under responder `graph-query`, and every entry gains `Enveloped` and `Reply`
  (Fable ruling 1). `graph.ingest.*` and `graph.index.*` entries get the marks too.
- Authority replies and `byName` are forwarded byte for byte, with no re-check (owner, Q3). `entityByAlias` and
  `relationships` build one `graph.QueryResponse`.
- `min_revision` on graph-index's eight verbs: an index behind the revision refuses with transient
  `revision_not_reached`, naming the revision it has reached. No reply waits (ADR-109 Decision 4).
- `entityByAlias` refuses what the alias verb refuses, `alias_ambiguous` and `index_not_ready` among them (ADR-114).
- `composition.Analyze` reports a second responder on a declared verb, which refuses boot (#106, in part).
- Port ADR-060 with a note. Add ledger rows; grow the `query-pattern` skill.

Not here, by step: path search, counts and the other forwards (2); communities and `localSearch` (3); the classifier,
coordinator and structured strategies (4); the text tier and `searchGraph` (5). Language-model paths stay out
(`#485`-`#487`, `#32`). The generation marker stays with `#15`.

## Capabilities

- New: `graph-query`.
- Modified: `graph-transport-boundary` (the table and its marks), `graph-index` (`min_revision`),
  `composition-validation` (one finding type).

## Impact

- About 1,900 lines including tests (a hand estimate, not measured). `line count is a guide` (owner); draft PR #505
  words the rule so.
- **New exported surface, for the owner's acceptance.** Each has a caller named in `design.md`:
  `graph.ResponderGraphQuery`, `graph.QueryVerb.Enveloped`, `graph.QueryVerb.Reply`, `graph.MinRevisionField`,
  `graph.ErrorCodeRevisionNotReached`, `graph.RelationshipEdge`, `composition.TypeRequestSubjectConflict`. Package
  `graphquery` exports `Register`, `DeclarePorts`, `CreateGraphQuery`, `DefaultConfig`, `Config`, `Component`.
- New subjects: six on `graph.query.*`, now reserved (no stream may capture them; #16). New config key:
  `query_timeout_ms`. New buckets: none.
- Consumers, at their cutover to this module. SemConnect must read `kv_revision` (one struct tag) and delete three
  config keys its deployment sets and the pin ignored. `relationships` replies change shape; no consumer decodes them
  except through fusion, which change 5 ports here. SemSource's graph-query config passes as it stands.
- No code before the owner approves this proposal and its design. Retires no issue; `design.md` lists each one.
