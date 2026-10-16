# Design: simplify-04a-13-graph

This design depends on `INVENTORY PASS` for `inventory.md`. Premises cite its sections; line numbers are at the base
commit. Change 2 task 3.3 is archived. It left #132 items 1–6 open (`tasks.md:129-130`); this change owns them.

## D1. One stored-value rule (#276; #132 item 2)

`graph.UnmarshalEntityState(key string, data []byte, entity *EntityState) error` is the one decoder for a stored
entity. It checks, in this order:

1. the JSON decodes (`unreadable_entity_state`);
2. the canonical contract (`noncanonical_predicate`, or `noncanonical_entity_id`);
3. `entity.ID == key` (`noncanonical_entity_id`);
4. every statement has a `Source` and a `Timestamp` (`missing_statement_metadata`).

This is graph-ingest's order today (`mutation_runtime.go:219-239`). Each refusal is exactly one `*errs.ClassifiedError`:
class fatal, code `graph_state_reset_required`, wrapping a `*StateContractError` with that `Reason` and
`EntityID == key`. It is built by one unexported constructor and carries no attribution prefix.

`ValidateStatementMetadata(triples) error` is step 4 on its own. graph-ingest's write lanes call it and classify the
result as `invalid_request` (`entity_writes.go:68`).

Who changes:

- graph-ingest: `decodeStoredEntity` becomes the call. `checkStoredEntityKey`, `missingStatementMetadata` and the
  error-field mutation at `:224` go. The sweep's fallback (`component.go:1296-1300`) cannot fire any more and goes.
- `decodeStoredForWrite` keeps ADR-110 Decision 6's trusted decode plus key comparison. On a mismatch, its refusal is
  the one `UnmarshalEntityState(key, current, …)` returns, on the failing path only.
- `pkg/lifecycle`: `watchEvent` passes `entry.Key()`. The test copy `decodeStoredValue` is deleted, and the doubles
  call the rule.
- `ExactEntityReader`: calls the rule with the requested ID. Its own ID check goes.
- `ClassifyStateContractError` goes. Its remaining callers (`query.go:98`, `:458`) only pass through errors that are
  already classified, so they return `err`.
- graph-ingest's `rejectFromError` (`mutation_runtime.go:267-270`) returns a state error unchanged instead of adding a
  second fatal layer. Its callers (reconcile, delete, create, and `canonicalAppendFailure`, inventory § 1 item 2) then
  reply with the decoder's one classification.
- The aggregate validator family `ValidateDecodedEntityState{,s,Pointers,IDs}` (inventory § 2) runs steps 2 and 4.
  It has no key, so step 3 cannot apply. Step 4 is added because the entities in batch and prefix replies come from
  `readEntity`, which has already enforced it: a real reply is never refused for it, and a reader of a reply then
  checks the same rule as a reader of the bucket. The steps are unexported helpers shared with the decoder, which no
  longer calls the family. The family then has no reader in the tree, so it joins owner question 1; the step-4
  addition applies only if it is kept. The rejected alternative, leaving step 4 out, would keep a partial
  spelling for change 4's batch readers to adopt.

Observable:

- A watch sees a value with a statement lacking `Source`, or a value stored under another key, and returns the
  classified error. Today it delivers the entity.
- For bytes with both a key mismatch and a noncanonical predicate, the write seam now records `noncanonical_predicate`,
  the same reason a read of those bytes gives.
- A reconcile, delete or append refused for its stored value replies with one `ClassifiedError` layer (fatal,
  `graph_state_reset_required`), where today it carries two.
- The write seam itself (`decodeStoredForWrite`) still does not run steps 2 and 4 on a value it accepts (ADR-110
  Decision 6). `encodeMerge`'s metadata check and `MarshalEntityState` still gate the commit.
- `MarshalEntityState` is unchanged. graph-ingest checks statement metadata before each write so that the refusal is
  `invalid_request` naming the statement index (spec "Statement metadata is required").

Options:

- (a) Chosen: the key becomes an argument of the existing decoder. This keeps ADR-074's one codec and its name.
- (b) A new `DecodeStoredEntity` beside `UnmarshalEntityState`. That leaves two decoders, and `UnmarshalEntityState`
  would have only test callers outside the root.
- (c) The lifecycle package keeps its own copy of the rule. That keeps one of the six spellings (inventory § 2).
- (d) Do nothing. #276 stays open as MEDIUM.

Tests:

- `TestUnmarshalEntityStateRefusesWhatGraphIngestRefuses`: a table, one row per reason, plus two double-fault rows
  (key mismatch with missing `Source` gives `noncanonical_entity_id`; key mismatch with a noncanonical predicate gives
  `noncanonical_predicate`). Each row asserts exactly one `ClassifiedError` layer.
- `TestWatchRefusesAStoredValueGraphIngestRefuses` (`pkg/lifecycle`, `synctest`, `Watch` and `WatchEvents`).
- `TestExactEntityReaderRefusesAReplyWithoutStatementMetadata`.
- `TestReconcileReplyForPoisonedStateHasOneClassification` (graph-ingest): a reconcile of an entity whose stored value
  lacks `Source` returns an error with exactly one `ClassifiedError` layer. It fails on the base commit (two layers).
- Sensitivity: `task mutate:check` with step 4 removed must be detected by the watch test.

Generated checks are not used (`docs/testing.md`). There are four checks with a fixed precedence, so the per-reason
rows and the adjacent-precedence rows are the whole input space. The spec delta states the order.

## D2. The exact reader is a struct (#132 items 3, d)

- `type ExactEntityReader struct`, with an unexported `requester` interface declared once.
- `NewExactEntityReader(requester, timeout) (*ExactEntityReader, error)` refuses a nil interface and a negative
  timeout. A typed-nil pointer is not detected, the same as `graphmutation.NewClient`.
- The zero-value `ExactEntityReader{}` still returns an error, because an exported struct can be declared without the
  constructor.
- `ReadExactEntity` returns `&entity`, not a copy, as does graph-ingest's handler (`query.go:104`).
- `pkg/lifecycle` declares an unexported one-method interface for its field and fakes. That is #132 item 3's shape; it
  is not a back door, since nothing exported sets it. `pkg/projection` holds `*graph.ExactEntityReader`.
- Test: `TestNewExactEntityReaderRefusesNilRequester` fails on the base commit, which returns a reader.

## D3. One failure vocabulary (#132 items 1, c; ruling L)

- `MutationOutcome` keeps applied, unchanged, entity_not_found and failed.
- `MutationEntityAlreadyExists`, `MutationRevisionMismatch`, `MutationInvalid`, `isServerMutationOutcome`,
  `TestMutationOutcomeVocabularyIsClosed` and `TestErrRevisionMismatchCodeMatchesConstant` go.
- `canonicalErrorOutcome` returns the outer classified code, or `internal` when the error carries none. This is
  ruling L as written: "the outcome label becomes the classified code". Measured per operation in inventory § 1
  item 1, the labels change as follows.
  - `invalid` becomes `invalid_request`. It is a rename, not a split: `structural_invalid` never reaches the
    canonical lanes (its one producer is the stream lane's gate, `component.go:1748`), and `rejectFromError` puts
    `invalid_request` over an `entity_id_invalid` from `validateEntityID`.
  - Four codes stop being counted as `internal`: `message_type_unregistered` (create); `entity_id_invalid` (create,
    `authorizeSubject` on an unvalidated ID, returned bare); `entity_id_authority_invalid` (all four operations,
    bare); `graph_state_reset_required`.
  - `entity_already_exists`, `entity_not_found` and `revision_mismatch` are unchanged.
- The failure series then carries the same codes as `mutation_rejections_total{reason}` (`mutation_runtime.go:55-61`).
  Ruling L asks for this label. Whether one of the two series should go is #139's "one outcome record" (PR 10).
- `pkg/errs` gains `CodeRevisionMismatch` and `CodeIndexNotReady`. `ErrRevisionMismatch` uses the first.
  `graph.ErrorCodeRevisionMismatch` and `graph.ErrorCodeIndexNotReady` are defined as those constants.
  `natsclient.ErrorCodeBucketNotReady` goes (`kvspec.go:350` uses the errs constant), and so does the equality
  assertion at `kvcatalog_test.go:165`.
- The "set is closed" comment is rewritten to say who emits what.
- Test: `TestCanonicalMutationOutcomeLabelIsTheClassifiedCode`, driven through the real create handler:
  - a create with an unregistered message type counts `outcome="message_type_unregistered"` (base: `internal`);
  - a create with a nil entity counts `outcome="invalid_request"` (base: `invalid`).

  Sensitivity: a mutant that maps `message_type_unregistered` back to `internal` must be detected.

## D4. One wire spelling, one verb declaration (#132 items 4, 5)

- `ExactEntity` is tagged `kv_revision`, and the tag is written once. Today the reader decodes the reply into its own
  struct with its own tag (`exact_entity.go:79`). It will decode into a struct that embeds `ExactEntity` and shadows
  only `Entity` with a `json.RawMessage`, which it validates with D1's decoder. `encoding/json` takes the shallower
  `entity` field and the embedded `kv_revision`. The five JSON literals in `graph/exact_entity_test.go`
  (`:49`, `:70`, `:88`, `:89`, `:119`) change with it.
- `QueryVerb` becomes `{Name, Subject, Responder}`. The type strings go: their only reader here is a test, and the
  same at the pin (inventory § 1).
- `graph.ResponderGraphIngest` replaces graph-ingest's `queryResponder`.
- `EntityRequest{ID}` and `EntityBatchRequest{IDs}` (paired with `EntityBatchResponse`) are declared in `graph`. The
  exact reader and graph-ingest's two handlers use them.
- Item 4's time half is not done here. `published_at` leaves with #183 (ADR-109 Q1). `FirstFailureAt` has no producer
  here; its producer, graph-index, is ported in change 4. Recorded on #110.
- Tests:
  - `TestExactEntityReplyCarriesKVRevision` (graph-ingest) fails on the base commit. It sends graph-ingest's real reply
    from `handleQueryEntityNATS` through `NewExactEntityReader` over a requester that calls the handler, and asserts
    the reply's JSON key is `kv_revision` and the reader returns the stored revision. A rename on one side only fails
    with "zero KV revision".
  - `query_verbs_test.go` holds the three-field table.

## D5. Exports used by one package, and twins (#132 items e, f, h, Tests)

- `EncodeCursor`/`DecodeCursor` and `UnmarshalEntityStateTrusted` move, unexported, into graph-ingest. Their tests move
  with them. ADR-081's ban on the trusted decoder outside the owner becomes structural.
- `MaxPrefixQueryLimit` stays; `DefaultPrefixQueryLimit` goes. The doc
  says a limit of zero or less, or over 1000, means 1000.
- `CreateEntityResponse` and `ReconcilePredicatesResponse` become `EntityMutationResponse`. The JSON is unchanged.
  `recordCanonicalResponse` decodes once.
- `ValidateEntityPredicates` is unexported. The benchmark calls `ValidateEntityStateContract`.
- `query_prefix_types_test.go` becomes one round-trip table.
- Item f is optional (#132 "Carried, low"). It is included because the compiler catches every site. Dropping it would
  leave it for a follow-up issue.

## D6. Typed index state (#132 item b)

- `type IndexState string`. `IndexStatusResponse.State` and the four constants take it.
- `AllIndexStates` becomes `IndexStates() []IndexState`, returning a new slice. Callers: `readiness/gauges.go:144`,
  `readiness_gate.go:168`. The wire is unchanged.
- `readiness.AllDeferReasons` stays with 8e (#135).
- Test: `TestIndexStatesReturnsACopy`.

## D7. Text follows the code (#132 item 6)

- One `graph/doc.go`, stating change 2's D16 split. The seven other package comments go.
- `gh#NNN`, and ADR numbers not in `docs/adr/`, are prefixed "SemStreams" (`docs/provenance.md` keeps the pin text).
- `Clone`'s doc says which fields are shared (inventory § 7).
- Also updated: `graph/README.md`, both `query-pattern` skill files, and the ledger `graph` row (contract;
  known_risks drops #132).

## Exports (#190)

Kinds (#190): **API** has a contract line (spec, README or ledger contract) plus a driving test. **Plumbing** has a
production caller, here or, for the group under owner question 1, at the pin in an admitted package not ported yet.

| Symbols | Action | Kind | Contract line or caller; test |
| --- | --- | --- | --- |
| `EntityState` (+`GetTriple`, `GetPropertyValue`, `Clone`), `Graphable` | keep | API | README "data model", "Graphable"; `graph-entity-writes` "A single-value read…"; `pkg/lifecycle/manager.go`, `message/storable.go`; `TestSingleValueReadPicksLatestAcrossSources` |
| `ReplaceBySource`, `ReplaceResult`, `StaleSet` | keep | Plumbing | `processor/graph-ingest/entity_writes.go`, `component.go` |
| `Source*` (3), `IsReservedSource` | keep | API | `graph-entity-writes` "Statement metadata is required"; `pkg/lifecycle/graph_emit.go`, `graph/inference/hierarchy.go`, `statement_metadata.go` |
| `MarshalEntityState`, `ValidateEntityStateContract`, `EntityStateContractError`, `EntityStateContractField` (+3), `EntityPredicateContractError`, `InvalidEntityPredicate` | keep | Plumbing | graph-ingest `canonical_mutations.go:264`, `mutation_runtime.go` |
| `UnmarshalEntityState` | keep, D1 signature | API | ADR-074; spec delta (added); 4 production callers |
| `ValidateStatementMetadata` | new, D1 | Plumbing | `entity_writes.go:68`; root decoder |
| `StateContractError`, `StateResetReason` (+4), `ErrorCodeGraphStateResetRequired`, `IsStateContractError` | keep | Plumbing | graph-ingest (7 files), `pkg/projection/mutation_client.go` |
| `ClassifyStateContractError` | remove, D1 | — | no caller remains |
| `ExactEntity`, `ExactEntityReader`, `NewExactEntityReader` | keep, D2/D4 | API | ledger contract (6); `graph-transport-boundary` "Authority reads…"; `pkg/lifecycle`, `pkg/projection` |
| `EntityRequest`, `EntityBatchRequest` | new, D4 | API | spec delta "Reserved request subjects…"; `query.go:70`, `:116`; `exact_entity.go:67` |
| `ResponderGraphIngest` | new, D4 | Plumbing | `processor/graph-ingest/query.go:45` |
| `QueryVerb`, `QueryVerbs` | keep, D4 fields | API | `graph-transport-boundary` "Reserved request subjects…"; `query.go:44` |
| Mutation requests (4); `AppendSubjectResult`, `AppendTriplesResponse`, `DeleteEntityResponse`, `MutationFailure` | keep | Plumbing | `internal/graphmutation/client.go`, `pkg/lifecycle`, `pkg/projection`, graph-ingest |
| `CreateEntityResponse`, `ReconcilePredicatesResponse` | merged into `EntityMutationResponse`, D5 | Plumbing | same callers |
| `MutationOutcome` + 4 values | keep | API | spec delta (added); `graphmutation/client.go:207-221` |
| `MutationEntityAlreadyExists`, `MutationRevisionMismatch`, `MutationInvalid` | remove, D3 | — | ruling L |
| `ErrorCode*` (9 with callers) | keep (2 now defined from `errs`, D3) | Plumbing | graph-ingest, `pkg/projection`, `pkg/lifecycle`, `natsclient` |
| `PrefixQueryRequest`/`Response`, `EntityBatchResponse`, `MissingEntity`, `MissingReason` (+2), `MaxPrefixQueryLimit` | keep | API | `graph-transport-boundary` "Prefix queries…"; `query.go` |
| `DefaultPrefixQueryLimit`, `EncodeCursor`, `DecodeCursor`, `UnmarshalEntityStateTrusted` | moved/removed, D5 | — | graph-ingest only |
| `ValidateEntityPredicates` | unexport, D5 | — | root only |
| `IndexStatusResponse`, `IndexState*` (4), `IndexState`, `IndexStates` | keep / new / replaces `AllIndexStates`, D6 | Plumbing | `graph/readiness` (5 files), `processor/graph-ingest/readiness.go` |
| `Bucket*` (18) | keep | Plumbing | `graph/kvcatalog/kvcatalog.go` |
| The 28: the 27 in inventory § 4 (index, name, predicate and summary types; predicate token codec; `MatchesAnyIDPrefix`; `ValidateDecodedEntity{States,StatePointers,IDs}`; `ErrorCodeEmbeddingUnavailable`, `ErrorCodeResourceExhausted`; `Provider`) plus `ValidateDecodedEntityState` (its one in-tree reader goes with D1) | keep (recommended) | Plumbing, readers at the pin only | admitted-package readers (`notes/3.3.md:94-235`); owner question 1 |
| `errs.CodeRevisionMismatch`, `errs.CodeIndexNotReady` | new, D3 | Plumbing | `pkg/errs/errs.go:344`, `natsclient/kvspec.go:350`, `graph` |
| `natsclient.ErrorCodeBucketNotReady` | remove, D3 | — | read only inside `natsclient` (`kvspec.go:350`, `kvspec_integration_test.go:210`) and by `graph/kvcatalog/kvcatalog_test.go:165`; the value moves to `errs.CodeIndexNotReady` |

## Open issues and merge order

- Retired: #132 (tasks 2.1–4.2) and #276 (tasks 2.1–2.3). Three #132 leftovers are recorded where their work lies:
  - item 4's `FirstFailureAt` on #110;
  - item g (`IncludePredicates`) on #105, if owner question 1 keeps the summary types;
  - item i (readiness tests) on #135.
- Stay: #103, #106, #110, #111, #133, #135, #139, #161, #183, #280 (inventory § 3).
- Merge order: #279 and #283 share only `docs/admission-ledger.yaml` with this change, each in another row. Either
  order works; whichever lands second merges `origin/main`. 8d, 8e and PR 10 start after this change merges (owner's
  order). They inherit
  `kvcatalog_test.go:165`, `IndexStates()` and the D3 label change.

## Owner questions

1. Twenty-eight exports have no reader in this repo. Code in packages we have admitted but not yet ported reads them at
   the pin; those packages come in change 4, 5 or 7. The 28 include `ValidateDecodedEntityState`, whose one reader
   here goes with D1. The contracts already say keep: the reviewer contract (§ Port-time and pattern review, `:441-442`)
   says "a symbol whose only caller is in an admitted package not ported yet is not dead surface". So the only
   question is whether your 2026-10-16 direction ("with no caller it is unexported or dropped") overrides "port whole"
   for these. Ruling P is a precedent for removal: the reply envelope left and returns with change 4.
   **Recommended: keep**, so that each package ported later finds its types already here. Removing them means each
   one comes back with its reader. Item g, and step 4 for the aggregate validators (D1), depend on this answer.

FYI, no ruling needed:

- (a) The metric label change is ruling L as written: `invalid` becomes `invalid_request`, and four codes leave
  `internal` (D3).
- (b) The verb table drops its type strings. #106's change-4 table can add typed fields if graph-query ever reads
  them.
