# Tasks: simplify-04a-13-graph

Each task names its outcome and the gate that proves it. Tests come first. Record each failing on the base commit in
`notes/<task>.md`. Tests in 2.1 and 3.1 are written against the base API and fail by assertion. Only 2.2 and 4.1 count
a compile failure as failing. Review stops (Codex) after 2.3, 3.3 and 4.2.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec deltas pass pre-owner review and are accepted
  by the owner, with owner question 1 answered. Gate: `task spec:check` and `task docs:check`.

## 2. One stored-value rule and the exact reader (batch 1)

- [ ] 2.1 Failing tests against the base API, D1 and D2:
  - `TestWatchRefusesAStoredValueGraphIngestRefuses` (`pkg/lifecycle`, `synctest`; `Watch` and `WatchEvents`; a
    statement without `Source`, and a value stored under another key);
  - `TestExactEntityReaderRefusesAReplyWithoutStatementMetadata`;
  - `TestNewExactEntityReaderRefusesNilRequester`;
  - a one-layer classification check on today's `UnmarshalEntityState` (base: two layers).

  Gate: each fails by assertion on the base commit.
- [ ] 2.2 `TestUnmarshalEntityStateRefusesWhatGraphIngestRefuses` (one row per reason plus the two double-fault rows).
  Every caller moves to `UnmarshalEntityState(key, data, &entity)`: the 20 test lines in 9 outside test files, the
  root tests, and `vocabulary/export/datatype_roundtrip_test.go` with stamped statements (D1). Gate: fails to compile on
  the base commit.
- [ ] 2.3 Implement D1 and D2:
  - the root decoder, `ValidateStatementMetadata`, and the one constructor; `ClassifyStateContractError` removed;
  - graph-ingest's `decodeStoredEntity`, `decodeStoredForWrite` and the sweep;
  - `pkg/lifecycle` `watchEvent`, with `decodeStoredValue` deleted;
  - the struct reader, with lifecycle's unexported interface and `pkg/projection`'s field.

  Gate: 2.1 and 2.2 pass; `go test -race` of `graph`, `pkg/lifecycle`, `pkg/projection`,
  `processor/graph-ingest/...`; `task mutate:check` with D1 step 4 removed is detected by the watch test.

## 3. Vocabulary and wire (batch 2)

- [ ] 3.1 Failing tests against the base API:
  - `TestCanonicalMutationOutcomeLabelIsTheClassifiedCode` (structurally invalid create gives `structural_invalid`;
    `message_type_unregistered` is not counted as `internal`) (D3);
  - `TestExactEntityReplyCarriesKVRevision` (D4).

  Gate: both fail by assertion on the base commit.
- [ ] 3.2 Implement D3:
  - the four outcomes; the three outcomes, `isServerMutationOutcome` and the two tests removed;
  - the label as the classified code;
  - `errs.CodeRevisionMismatch` and `errs.CodeIndexNotReady`; `natsclient.ErrorCodeBucketNotReady` and
    `kvcatalog_test.go:165` removed;
  - the error-code comment rewritten.

  Gate: 3.1's label test; `task mutate:check` on a mutant mapping `structural_invalid` to `invalid` is detected.
- [ ] 3.3 Implement D4 and D5:
  - `kv_revision`; `QueryVerb{Name, Subject, Responder}`; `ResponderGraphIngest`; `EntityRequest`, `BatchRequest`;
  - the cursor codec and trusted decoder moved into graph-ingest; one prefix limit; `EntityMutationResponse`;
  - `ValidateEntityPredicates` unexported; the prefix-type tests as one table.

  Gate: 3.1's wire test; `task test:unit`; `task cover:check` keeps the `graph merged` floor.

## 4. Index state and text (batch 3)

- [ ] 4.1 `TestIndexStatesReturnsACopy`, then D6: `IndexState`, typed constants, `IndexStates()`, and the readiness call
  sites. Gate: the test fails to compile on the base commit, then passes; `go test ./graph/readiness/...`.
- [ ] 4.2 D7:
  - `graph/doc.go`, with the seven package comments removed; the SemStreams prefixes; the `Clone` doc;
  - `graph/README.md`; `.agents/skills/query-pattern/SKILL.md` and `.claude/skills/query-pattern/SKILL.md`;
  - the `graph` ledger row.

  Gate: `go doc ./graph` prints one package comment; `task ledger:check`, `task docs:check`, and the adapter check in
  `task verify`.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Comments posted: #110 (`FirstFailureAt`), #135 (item i), #105 (item g, if kept); the follow-up issues
  filed; then the change is archived (`openspec archive simplify-04a-13-graph`) as the last content commit. Gate:
  `task spec:check`.
