# Inventory: simplify-04a-13-graph

base: `6562ada9ba8169bfac92798a4e08b7fc25964a0a` (origin/main `838c1fad0eea3be5353b1637485b9c93c24e9b09` plus the empty
claim commit; draft PR #286)

Question: where the `graph` root spells one fact twice, classifies one error more than once, or exports surface with no
reader, and where a reader of a stored entity skips the stored-value rule. Scope: #132 (body items 1–7, comment items
a–i, its Tests paragraph) and #276 (epic #223, PR 8c; owner 2026-10-16, #223 comment 6216623583: "**8c — `graph`**: #132,
plus #276 (export graph-ingest's stored-value check; lifecycle's Watch adopts it)."). Not in scope:
`graph/kvcatalog` (8d, #133), `graph/readiness` (8e, #135), the component Start context (#282).

This change brings in no package from the pin. `graph` was ported by change 2 (`docs/admission-ledger.yaml:1923-1962`),
so the pin probe runs are not owed. #132's line numbers are at PR #93 head `7bf1eb8`; every pin below is at the base
commit. The root holds 1,810 non-test lines (`cat graph/*.go` without `_test.go`, `wc -l`).

Repositories read (`docs/inventory-scope.md`): this tree; the SemStreams pin `8b99efe9` through `gh api …?ref=`
(ADRs, `processor/graph-query/{query,component,operation_inventory_test}.go`, `graph/exact_entity.go`); semsparql (the
one named consumer that imports SemEngine's `graph`), at its working tree, without git.

## 1. The claimed gaps (re-measured)

### #276: the lifecycle watch skips the stored-value rule

- `pkg/lifecycle/manager_query.go:267` — `if err := graph.UnmarshalEntityState(entry.Value(), &state); err != nil {`.
  `watchEvent` (`:258`) decodes under the canonical contract only. It checks neither that the entity is the one the
  key names nor that each statement has a `Source` and a `Timestamp`.
- graph-ingest's rule: `processor/graph-ingest/mutation_runtime.go:219` — `func decodeStoredEntity(key string, value
  []byte) (graph.EntityState, error) {`. It does the decode, then the key check (`:244` — `func
  checkStoredEntityKey(key, storedID string) error {`), then the metadata rule (`entity_writes.go:79` — `func
  missingStatementMetadata(triples []message.Triple) error {`).
- A third copy of the rule: `pkg/lifecycle/manager_test_helper_test.go:74` — `func decodeStoredValue(key string, value
  []byte) (graph.EntityState, error) {` ("copied … This copy can fall behind the rule", `:68-73`). It exists because of
  #199, fixed in PR #272 with a test-package copy.
- Severity: MEDIUM, as #276 says (#185). It needs a writer other than graph-ingest, and only graph-ingest writes
  `ENTITY_STATES`. Readers of the bucket in production: `pkg/lifecycle/manager.go:202` and graph-ingest only
  (`git grep -n BucketEntityStates -- '*.go' ':!*_test.go'`).

### #132 item 1: two vocabularies for one mutation failure (ruling L, binding)

- `graph/mutation_responses.go:111` `MutationEntityAlreadyExists`, `:113` — `MutationRevisionMismatch MutationOutcome =
  "revision_mismatch"`, `:115` `MutationInvalid`.
- Their only production reader is `processor/graph-ingest/canonical_mutations.go:58` — `func canonicalErrorOutcome(err
  error) string {`, which maps codes to metric labels. `:70` — `case graph.ErrorCodeInvalidRequest,
  graph.ErrorCodeStructuralInvalid:` maps both codes to `invalid`. Every other code (`message_type_unregistered`,
  `graph_state_reset_required`, the entity-ID codes) becomes `internal` (`:72-73`).
- `:121` — `func isServerMutationOutcome(outcome MutationOutcome) bool {`: no production caller. Its only reader is
  `graph/mutation_protocol_test.go:115` (`TestMutationOutcomeVocabularyIsClosed`).
- `revision_mismatch` is spelled three times: `mutation_responses.go:16`, `:113`, and `pkg/errs/errs.go:346` — `Code:
  "revision_mismatch",`. `graph/errcode_sentinel_test.go:15` exists only to keep two of them equal.
- `:8-9` — "The set is closed — adding a value requires updating both this declaration and the graph-ingest handler that
  emits it." This is false: `index_not_ready` is emitted by `natsclient/kvspec.go:350` and
  `pkg/lifecycle/manager_query.go:204`, `:233`, and nothing in the tree emits `embedding_unavailable` or
  `resource_exhausted`.
- Test-only readers of the three outcomes that go: `internal/graphmutation/client_test.go:406`, `:458`, and
  `processor/graph-ingest/canonical_mutations_test.go:795`.

### #132 item 2: one state-contract error classified two or three times

- `graph/entity_predicate_contract.go:247` and `:257` both read `return ClassifyStateContractError(errs.WrapFatal(&StateContractError{`.
  `WrapFatal` (`pkg/errs/errs.go:369-375`) builds a fatal `ClassifiedError` with no code. `ClassifyStateContractError`
  (`graph/state_contract.go:33-44`) finds no code and wraps again (`:43`). Result: two `ClassifiedError` layers and a
  `graph.UnmarshalEntityState: decode authoritative entity state failed:` prefix.
- `graph/exact_entity.go:92` — `return nil, ClassifyStateContractError(err)` reclassifies an error that is already
  classified.
- graph-ingest wraps a third time at `mutation_runtime.go:232` and `:248`, and passes through at `query.go:97-98` and
  `:455-458`.
- `mutation_runtime.go:224` — `contractErr.EntityID = key` sets a field on an error after it was built.
- The key check is written twice: `exact_entity.go:94` — `if entity.ID != entityID {`, and `mutation_runtime.go:245`.
- No test matches the prefix text (`git grep -n -e "authoritative entity state" -e "UnmarshalEntityState:" --
  '*_test.go'`: only `t.Fatalf` labels).

### #132 item 3, comment item d: one-implementation interface

- `graph/exact_entity.go:23` — `type ExactEntityReader interface {`. Its one implementer is `natsExactEntityReader`
  (`:27`).
- Fakes in `pkg/lifecycle` tests: `manager_test_helper_test.go:45` (`bucketExactEntityReader`), `children_test.go:26`,
  `manager_test.go:1725`.
- The requester interface literal appears twice (`:28-30`, `:38-40`). Equal literals also exist at
  `internal/graphmutation/client.go:15-17` and `pkg/projection/mutation_client.go:18-20`.
- `:57` — `if r == nil || r.requester == nil {`: a nil requester is accepted by the constructor and refused on every
  call. `graphmutation.NewClient` refuses it at construction.
- `:101` — `return &ExactEntity{Entity: entity.Clone(), …}` deep-copies a local value that was just decoded. The same
  happens at `processor/graph-ingest/query.go:104` — `Entity:     entity.Clone(),`.
- Production callers: `pkg/lifecycle/manager.go:45` (field), `:90`; `pkg/projection/mutation_client.go:31`, `:60`.

### #132 item 4: one field, two wire spellings

- `graph/exact_entity.go:19` — `` KVRevision uint64       `json:"kvRevision"` `` and `:79`. Every other key in the
  package is snake case, for example `mutation_responses.go:136` — `` `json:"kv_revision"` ``. The pin spells it
  `kvRevision` too (pin `graph/exact_entity.go:22`, `:71`).
- The time half is already settled elsewhere. `IndexStatusResponse.PublishedAt` (`graph/index_status.go:135`) leaves
  under ADR-109 Q1 (`docs/adr/109-ready-after-boot.md:218`, ruled on #9 comment 6111608867) through #183 (wave PR 10).
  `FirstFailureAt` (`:80`) has no producer in this tree (`git grep -n -w FirstFailureAt`: readiness tests only).

### #132 item 5: verb-table fields read only by tests

- `graph/query_verbs.go:11-15` declare `RequestType` and `ReplyType`. Their only reader is `graph/query_verbs_test.go:19-28`.
- At the pin, graph-query's table carries the same strings (`processor/graph-query/query.go:36`, `:50-58`). They are
  read only by `operation_inventory_test.go:91-93`. Searched at the pin: `query.go`, `component.go`,
  `operation_inventory_test.go`.
- The responder name is spelled twice: `graph/query_verbs.go:18` — `const responderGraphIngest = "graph-ingest"`, and
  `processor/graph-ingest/query.go:25` — `const queryResponder = "graph-ingest"`.
- The `{id}` request is an anonymous struct twice: `exact_entity.go:67-69` and `query.go:70-72`. The `{ids}` request is
  anonymous at `query.go:116-118`.
- The spec says the table names "request type and reply type" (`openspec/specs/graph-transport-boundary/spec.md:15-16`).
  So does the skill (`.agents/skills/query-pattern/SKILL.md:20-21`).

### #132 item 6: seven package comments and citations that resolve nowhere

- `// Package graph` appears at line 1 of `mutation_requests.go`, `mutation_responses.go`, `query_index_types.go`,
  `query_predicate_types.go`, `query_prefix_types.go` ("the SemStreams knowledge graph engine"),
  `query_summary_types.go` and `types.go`.
- `go doc ./graph` opens with "Package graph provides the canonical graph mutation request/reply contract" and then
  "See mutation_requests.go for the full package documentation".
- Citations in non-test root files: 40 lines match `gh#NNN` or `ADR-NNN`
  (`git grep -c -E "gh#[0-9]+|ADR-0[0-9]{2}|ADR-1[0-9]{2}"`: `index_status.go` 13, `mutation_responses.go` 7,
  `constants.go` 5). `docs/adr/` holds 066, 083, 084, 085, 088, 102, 104 and 107–112. Any other ADR number, and every
  `gh#`, resolves only in SemStreams.

### #132 item 7

- `QueryDirection` and the function `GetPropertyValue` were removed by change 2 task 3.3 (`notes/3.3.md:13-17`).
- `graph/types.go:94` — `type Provider interface {` has no reader in this tree. Its readers at the pin are in admitted
  packages: `graph/clustering/types.go:82` and `graph/structural/kcore.go:15` (change 7), per `notes/3.3.md:36-38`.

### #132 comment items (2026-10-09)

- **b.** `graph/index_status.go:32` — `` State string `json:"state"` ``; `:139-144` untyped constants; `:152` — `var
  AllIndexStates = []string{`, a mutable allow-list read by `graph/readiness/gauges.go:144` and
  `readiness_gate.go:168`. `readiness.AllDeferReasons` is in 8e's package.
- **c.** `natsclient/kvspec.go:142` — `const ErrorCodeBucketNotReady = "index_not_ready"` beside
  `graph/mutation_responses.go:79`. Held equal by `graph/kvcatalog/kvcatalog_test.go:165`. Its readers:
  `kvspec.go:350` and `kvspec_integration_test.go:210`.
- **d.** Covered under item 3 above.
- **e.** `graph/query_prefix_types.go:84` `EncodeCursor` and `:90` `DecodeCursor`. One production reader:
  `processor/graph-ingest/query.go:314`, `:381`, `:384`. The type's own doc says "Consumers MUST NOT parse, construct,
  or modify cursor values" (`:24-25`).
  `entity_predicate_contract.go:287` `UnmarshalEntityStateTrusted`: one caller, `processor/graph-ingest/entity_writes.go:327`.
  `query_prefix_types.go:17` — `const MaxPrefixQueryLimit = DefaultPrefixQueryLimit`: both read only at
  `processor/graph-ingest/query.go:268-271`.
- **f.** `mutation_responses.go:133` and `:142`: two field-identical response types. They are decoded separately at
  `processor/graph-ingest/canonical_mutations.go:80`, `:86`. Other readers: `internal/graphmutation/client.go:86-106`,
  `pkg/lifecycle/graph_emit.go:15-16`, `:30`, `:41`, `pkg/lifecycle/manager.go`; 21 test files.
- **g.** `graph/query_summary_types.go:28` `IncludePredicates bool` "Defaults to true". The type has no reader in this
  tree; its pin reader is `processor/graph-query/summary.go:125`.
- **h.** `entity_predicate_contract.go:91` `ValidateEntityPredicates`. Its root reader is `:183`. Its one outside
  reader is a benchmark, `processor/graph-ingest/merge_entity_bench_test.go:246`.
- **i.** The `IndexStatusResponse` round-trip tests live in `graph/readiness` (8e's package), plus
  `graph/write_rules_test.go:241`.
- **Tests paragraph.** `graph/mutation_protocol_test.go:115`, `graph/errcode_sentinel_test.go:15`, and
  `graph/query_prefix_types_test.go` (11 tests: cursor, JSON omitempty, and `:117` `TestPrefixQueryLimitConstants`).

## 2. Every spelling of the facts being modeled

- **The stored-value rule** (decode, canonical contract, key, statement metadata): `mutation_runtime.go:219-239`;
  `entity_writes.go:79-89` (the metadata rule, also used by the write lanes through `requireStatementMetadata`, `:68`);
  `exact_entity.go:90-100` (decode and key, no metadata); `manager_test_helper_test.go:74-91` (a full copy);
  `manager_query.go:267` (decode only). Five spellings.
  The write seam runs the trusted decode plus the key check (`entity_writes.go:325-338`), as ADR-110 Decision 6 states
  (`docs/adr/110-write-seam.md:129-131`).
- **The state-reset classification:** `state_contract.go:33-44`; `entity_predicate_contract.go:247-250`, `:257-260`;
  `mutation_runtime.go:232`, `:248`; `component.go:1296-1300` (the sweep builds an unclassified record when a refusal is
  not a `*StateContractError`).
- **Mutation failure:** the classified codes (`mutation_responses.go:10-96`), the three outcomes (`:111-115`), and the
  metric labels (`canonical_mutations.go:58-74`).
- **Cross-package error codes:** `revision_mismatch` at `mutation_responses.go:16` and `pkg/errs/errs.go:346`;
  `index_not_ready` at `mutation_responses.go:79` and `natsclient/kvspec.go:142`.
- **The requester interface:** `exact_entity.go:28-30`, `:38-40`; `internal/graphmutation/client.go:15-17`;
  `pkg/projection/mutation_client.go:18-20`.
- **The name "graph-ingest" as an identifier:** `graph/query_verbs.go:18`; `processor/graph-ingest/query.go:25`;
  `graph/kvcatalog/kvcatalog.go:61`, `:148`; `graph/readiness/watcher.go:63` (`KeyGraphIngest`);
  `processor/graph-ingest/component.go:690`, `:713`, `:815`.

## 3. Adjacent claims

- Open PRs (`gh pr list --state open --json number,title,isDraft,changedFiles,files`; none over 100 files):
  - #286: this claim, 0 files.
  - #283 (draft, PR 6a, natsclient storage report, 0 files so far). The files it plans to move
    (`natsclient/storage_report*.go`) read `graph.BucketStorageReport`, which this change does not touch. This change
    edits `natsclient/kvspec.go:136-142`, `:350` and `kvspec_integration_test.go:210`. Different files.
  - #279 (draft, `pkg/security` docs, 16 files). It edits `docs/admission-ledger.yaml` (the security/tlsutil rows).
    This change edits the `graph` row (`:1923-1962`), a different hunk. No other shared file.
  - #242–#248: Dependabot bumps (`go.mod`, `go.sum`, `package*.json`, `ci.yml`). No overlap.
- Specs:
  - `graph-transport-boundary` "Reserved request subjects have one declaration" (`spec.md:13-31`) names request and
    reply types. "Authority reads are not enveloped" (`:83-92`) names no key.
  - `graph-entity-writes` "Statement metadata is required" (`:80-100`) and "The write path refuses a stored value it
    cannot change" (`:181-212`) bind the rule's content. Neither says who else reads a stored entity.
- Ledger `graph` row (`docs/admission-ledger.yaml:1923-1962`): `adapt`. Its contract names `ExactEntityReader` and
  `IndexStatusResponse`; known_risks names #132.
- Coverage floor: `graph merged` (`scripts/cover-check.sh:32`).
- Docs naming the touched surface: `graph/README.md:25`, `:30`, `:34`; `.agents/skills/query-pattern/SKILL.md:16`,
  `:20-21`, `:25-28` and its adapter `.claude/skills/query-pattern/SKILL.md`; `processor/graph-ingest/README.md:10`,
  `:103` (stay true).
- Consumers: semsparql (named as importing SemEngine `graph`) holds no Go code (its tree is `LICENSE` only).
  No starter consumer imports `github.com/c360studio/semengine/graph` (`grep -rln` over semsource, semconnect,
  semteams, semboids, semvocab: empty).

### Open issues naming the package

`gh issue list --state open --search "graph" --limit 200` gave 47 hits. Each body and comment was searched for a root
file `path:line`, `` `graph` `` or a root export:

| Issue | Names it in | Disposition |
| --- | --- | --- |
| #132 | title, body, comments | retired (tasks 2.1–4.2; leftovers routed below) |
| #276 | body | retired (tasks 2.1–2.3) |
| #103 | body/comments (`exact_entity.go:17-19`, `mutation_responses.go:79`, `:136`) | stays: the reply envelope arrives with change 4 (ruling P) |
| #106 | body (`query_verbs.go:29`, `exact_entity.go:73`) | stays: change 4 extends the verb table to `graph.query.*`; this change narrows its fields |
| #110 | body (`index_status.go:26-30`, `:134-138`) | stays: fusion's copy goes in change 5; gains `FirstFailureAt` (item 4) |
| #111 | body (`query_summary_types.go:97-101`) | stays: graph-query's silent degrade, change 4 |
| #133 | body (errors naming `graph`) | stays: 8d |
| #135 | body (`graph.IndexStatusResponse`) | stays: 8e; gains #132 item i |
| #139 | body (`sources.go:11`, `helpers.go:108-131`, `exact_entity.go:85-91`) | stays: PR 10 (graph-ingest) |
| #183 | ADR-109 Q1 (`published_at`) | stays: PR 10 |
| #161 | body (names #132 as a case) | stays: harness check, unbuilt |
| #280 | body (`graph.EntityState` in lifecycle structs) | stays: lifecycle |
| #223 | epic lines | `Addresses #223` |
| #9, #15, #16, #20, #24, #31–#35, #68, #84, #94–#96, #105, #107–#109, #134, #141, #174, #195, #196, #198, #216, #222, #270, #282 | name the word, not the root package | — |

## 4. Readers of the surface touched

Measured with a script over the base tree (`/tmp/arch8c/counts.sh`, local only): for each exported identifier,
`git grep -l -w "graph\.<Name>"` outside the root, split into non-test and test, plus in-root counts. Known positive:
`EntityState` has 10 production files. Production readers of what changes:

- `UnmarshalEntityState`: `pkg/lifecycle/manager_query.go:267`, `processor/graph-ingest/mutation_runtime.go:221`.
  20 test lines in 9 outside test files.
- `ClassifyStateContractError`: `mutation_runtime.go:232`, `:248`; `query.go:98`, `:458`.
- `ExactEntityReader` / `NewExactEntityReader`: `pkg/lifecycle/manager.go:45`, `:90`;
  `pkg/projection/mutation_client.go:31`, `:60`.
- `MutationOutcome` values that go: `canonical_mutations.go:65-71` only.
- `CreateEntityResponse` / `ReconcilePredicatesResponse`: `internal/graphmutation/client.go`,
  `pkg/lifecycle/graph_emit.go`, `canonical_mutations.go` (also `ReconcilePredicatesRequest` callers in `pkg/lifecycle`,
  `pkg/projection`).
- `QueryVerbs`: `processor/graph-ingest/query.go:44`.
- `AllIndexStates`: `graph/readiness/gauges.go:144`, `readiness_gate.go:168`.
- Cursor codec, trusted decoder and limits: graph-ingest only (§ 1, item e).
- Proposed homes and their callers (category 4): the metadata rule's write-lane caller is `entity_writes.go:68`. The
  entity/batch requests: `exact_entity.go:67`, `query.go:70`, `:116`. The responder name: `query.go:45`.
  `errs.CodeIndexNotReady`: `natsclient/kvspec.go:350`.

Exports with no reader in this tree and a non-test reader in an admitted package at the pin (27, from
`openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/notes/3.3.md:94-235`): `AliasData`,
`CompoundPredicateData`, `CompoundPredicateQuery`, `IncomingEntry`, `IncomingRelationshipsData`, `OutgoingEntry`,
`OutgoingRelationshipsData`, `PredicateData`, `PredicateListData`, `PredicateListQuery`, `PredicateStatsData`,
`PredicateSummary`, `NameIndexItem`, `EncodePredicateToken`, `ErrorCodeResourceExhausted` (graph-index);
`EntityTypeSummary`, `SummaryData`, `SummaryRequest`, `ValidateDecodedEntityStatePointers` (graph-query);
`ErrorCodeEmbeddingUnavailable`, `MatchesAnyIDPrefix` (graph-embedding); `DecodePredicateToken` (graph-clustering);
`NameData`, `NameMatch`, `ValidateDecodedEntityIDs`, `ValidateDecodedEntityStates` (fusionnats); `Provider`
(clustering, structural). `ValidateDecodedEntityState` is also read by the root (`entity_predicate_contract.go:211`).

## 5. Problem shape

One validator at a seam, consumed by every reader. The closest existing instance is the write side:
`MarshalEntityState` (`graph/entity_predicate_contract.go:188`) is the one write gate that every `ENTITY_STATES` writer
calls (ADR-074 "One authoritative state codec", pin `docs/adr/074…:56-61`). The read side has the same intent, but
graph-ingest grew the key and metadata checks beside it (§ 2).

This is a reusable primitive, so an adoption sweep is owed. In this tree every adopter is in this change: the
lifecycle watch, the lifecycle test doubles, graph-ingest, and the exact reader. Outside the tree, the `ENTITY_STATES`
followers that change 4 ports (graph-index, graph-embedding; the follower in #109, ADR-081 "decodes and
contract-validates each write exactly once", pin `081…:78-85`) should adopt it.

The collision table and the intent check are not triggered: no new durable, communication or coordination primitive,
and no boundary moves.

## 6. Pin decision records

All 85 pin ADRs fetched at `8b99efe9`. Searched for `StateContractError`, `graph_state_reset_required`,
`ExactEntityReader`, `QueryVerbs`, `MutationOutcome`, `revision_mismatch`, `kvRevision`, `UnmarshalEntityState`,
`MarshalEntityState`, `IndexStatusResponse`, `AllIndexStates`, `ErrorCode`, `index_not_ready`, and the root file paths.
Titles also read.

- **ADR-074** (Canonical Predicate Contract). "All authoritative readers use `graph.UnmarshalEntityState`, which rejects
  unreadable or noncanonical state" (`:59-61`); `graph_state_reset_required` (`:70`, `:80`).
- **ADR-079** (Per-entity poison response). The reset error is scoped per entity on the authoritative surface (`:24`).
  Ported by reference through ADR-110 Decision 6.
- **ADR-081** (Graph view subscription). Graph-view consumers decode and validate each write once. The trusted decoder
  "is forbidden here" (`:78-85`).
- **ADR-060** (Unified RPC error contract). Failures are classified codes. `revision_mismatch` is the one sentinel,
  `errs.ErrRevisionMismatch` (`:207-216`, `:249-250`, `:294-295`).
- **ADR-091** (Mutation authority without semantic ownership). Typed outcomes for four operations (`:95`).
- **ADR-084** (local `docs/adr/084…`). Reserves `index_not_ready` for soundness. Read-your-writes compares the
  mutation's `kv_revision` (`:122`).
- **ADR-066 / ADR-083 / ADR-085 / ADR-088** (local). The `IndexStatusResponse` envelope and its states. Item b changes
  the Go type only.
- **ADR-110** (SemEngine, local). Decision 5: every stored statement has `Source` and `Timestamp` (`:114-125`).
  Decision 6: the write seam reads with the trusted decoder plus the key check (`:127-131`).
- ADR-102, ADR-103 and ADR-104 govern `EntityState.ID` and the type codes. Untouched.

## 7. Shape sweep (current tree)

1. **Second spelling:** the stored-value rule (5 spellings, § 2); the classification (§ 1 item 2); the failure
   vocabulary (§ 1 item 1); the two error codes (§ 2); `kvRevision` (§ 1 item 4); the responder name and request
   structs (§ 1 item 5); two field-identical responses (item f); `MaxPrefixQueryLimit = DefaultPrefixQueryLimit`
   (item e). Required where an issue item or #276 names it.
2. **Copied shell:** none in the root.
3. **Layered interface with one implementation:** `ExactEntityReader` (§ 1 item 3).
4. **Exported surface with no reader:** `isServerMutationOutcome` (unexported; test-only); the three outcomes;
   `RequestType` / `ReplyType`; `ValidateEntityPredicates` outside the root; the 27 identifiers with pin-only readers
   (§ 4). No back door: no production hook swaps a dependency.
5. **Classification by error text:** none. `IsStateContractError` uses `errors.As` and the code
   (`state_contract.go:18-28`).
6. Not in either issue:
   - `types.go:107-110` "Clone returns a deep copy … to avoid race conditions". It copies the `Triples` slice; each
     triple's `Object` and `ExpiresAt` stay shared. MEDIUM at most (#185: needs two goroutines mutating one object).
     Folded into the design's doc pass rather than filed.
   - Outside the package: `processor/graph-ingest/component.go:86` — `Help: "Graph-mutation requests rejected
     (success=false), by subject and reason (MutationResponse ErrorCode; 'unclassified' when absent)"`. The labels
     are the arrival lane and an authority reason (`authority_gate.go:79-81`), and `MutationResponse` no longer exists.
     Not fixed here: filed for PR 10.

## Adopter seams

Who reaches this surface from outside: a product author importing `graph` (none today, § 3), and SemEngine package
authors (graph-ingest, `pkg/lifecycle`, `pkg/projection`).

1. **What they must know today.**
   - To read a stored entity correctly, call `UnmarshalEntityState` and then also compare the ID with the key and check
     every statement's `Source` and `Timestamp`. graph-ingest's rule is unexported.
   - That `MutationOutcome` lists three values no reply carries.
   - That the reader's revision key is `kvRevision` while every other is snake case.
2. **If they do nothing.** A watcher silently delivers an entity that the authority would refuse (#276). A
   switch on `MutationOutcome` handles cases that never occur and has no case for the classified codes that do occur.
3. **Where they find out.** Nowhere, for the rule. Documentation, for the rest.
4. **What they should have to know.** Nothing: the one decoder applies the whole rule, and the key is a required
   argument, so the compiler asks for it.
