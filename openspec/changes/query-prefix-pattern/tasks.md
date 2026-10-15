# Tasks: query-prefix-pattern

## 1. Wire Type, Handler, and Proving Tests

- [ ] 1.1 Add failing unit tests in `graph/query_prefix_types_test.go`
  (`TestPrefixQueryRequestPatternJSONRoundTrip`, `TestPrefixQueryRequestMarshalOmitsPatternWhenEmpty`) and
  `processor/graph-ingest/query_prefix_unit_test.go` (`TestQueryPrefix_PatternMatchesAcrossSystems`,
  `TestQueryPrefix_PatternAndPrefixMutuallyExclusive`, `TestQueryPrefix_PatternInvalid`,
  `TestQueryPrefix_PatternCursorPaging`, `TestQueryPrefix_PatternNoMatches`) and integration test
  `TestIntegration_PrefixQuery_PatternAcrossSystems` in `processor/graph-ingest/query_prefix_integration_test.go`, then
  implement `PrefixQueryRequest.Pattern` in `graph/query_prefix_types.go`, wildcard filter matching in
  `processor/graph-ingest/component_test.go`, and `req.Pattern` validation/filtering in
  `processor/graph-ingest/query.go`.

## 2. Documentation, Ledger, and Verification

- [ ] 2.1 Update `processor/graph-ingest/README.md` and `docs/admission-ledger.yaml` (`graph` and
  `processor/graph-ingest` rows), record developer notes in `openspec/changes/query-prefix-pattern/notes/1.1.md`, and run
  `task mutate:check` and full `task verify` on a clean worktree.
