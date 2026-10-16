# Tasks: simplify-04a-10-types-message-vocab

Each task names the outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec deltas pass pre-owner review and are
  accepted by the owner on #219 (owner questions 1–4). Gate: `task spec:check` and `task docs:check`.
  Hold: owner answers to questions 1–4 on #219.

## 2. Failing tests

- [ ] 2.1 `TestTypeIsValidMatchesValidate` (D1): twelve rows, three components by four classes. Gate: the dotted and
  non-UTF-8 rows fail on the base commit.
- [ ] 2.2 `TestBaseMessageValidateRefusesTypeGrammar` (D2). Gate: fails on the base commit.
- [ ] 2.3 `TestGenericJSONValidateRefusesWhatMarshalJSONRefuses` and the `Validate`/`MarshalJSON` law in
  `FuzzGenericJSONShapes` (D3). Gate: both fail on the base commit.
- [ ] 2.4 `TestRegistryLooksUpByType` (D6). Gate: fails on the base commit (no typed signature).
- [ ] 2.5 `TestParsePredicateDoesNotAllocate`, with a fresh predicate on every run including the warm-up (D7).
  Gate: fails on the base commit.
- [ ] 2.6 `FuzzParsePredicateAgreesWithGrammar` (D7, generated checks): a grammar oracle and a copy of the base parse;
  seeds hold one input per reason, a multi-fault input such as `a..b.c`, and the 64/65 and 194/195-byte limits.
  Gate: passes on the base commit, and fails under one `task mutate:check` mutant that swaps the arity and
  segment-empty checks.

## 3. Implementation

- [ ] 3.1 `Type.IsValid` is `Validate() == nil`. Gate: 2.1 passes.
- [ ] 3.2 `BaseMessage.Validate` calls `Type.Validate`; the UTF-8 loop in `MarshalJSON` goes. Gate: 2.2 and the
  existing non-UTF-8 type test pass; one `task mutate:check` run that deletes the new call fails that test.
- [ ] 3.3 `GenericJSONPayload.Validate` runs the shape check; `MarshalJSON` unchanged. Gate: 2.3 passes.
- [ ] 3.4 `Hash` leaves `Message` and `BaseMessage` with its test and the "Content-addressable" doc line (owner
  question 1; under option 2, `Hash() (string, error)` and `TestHashRefusesAPayloadThatCannotEncode`). Gate:
  `task build`, `task lint`.
- [ ] 3.5 Remove `message.Keyable`, `message.EntityType`, `message.EntityID`, `Triple.IsExpired` and
  `EntityType.IsValid` (owner question 2); `triple_identity.go`'s hazard note stops citing `IsExpired`. Gate:
  `task build`, `task lint`.
- [ ] 3.6 Registry keyed by `types.Type`; `GetRegistration` and `IndexingProfileFor` typed; graph-ingest's three calls
  drop `.Key()`; `Registration.MessageType` spelled with `Type.Key()` (owner question 4; otherwise D6′). Gate: 2.4
  and the existing registry and graph-ingest unit tests pass.
- [ ] 3.7 `ParsePredicate` with `strings.Cut`; the memo and `predicate_memo_test.go` go. Gate: 2.5 and 2.6 pass;
  before/after `go test -bench` recorded in `notes/3.7.md` as information.
- [ ] 3.8 Delete `parseDomainCategory`. Gate: `task build`.

## 4. Documentation

- [ ] 4.1 READMEs and doc comments (D5): `pkg/types/README.md:71-98`, `pkg/types/doc.go:6`, `message/doc.go:78`,
  `:155-157`, `message/message.go:11`, `vocabulary/README.md:480`, and the `message` README line that offers
  `message.Type`. Gate: `task docs:check`.
- [ ] 4.2 Comment on #90 with the measurement: `DcIdentifier` is `vocabulary/standards.go:47`, semconnect reads it
  (`parser/sensorml/predicates.go:129`), and `vocabulary/README.md:300` is accurate. Gate: the comment is read back.
- [ ] 4.3 Post the `strings.Cut` allocation probe on the PR with its source, command, Go version and output (D7).
  Gate: the comment is read back.
- [ ] 4.4 `docs/admission-ledger.yaml`: the four rows as the design's Ledger section says. Gate: `task ledger:check`,
  `task docs:check`.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-10-types-message-vocab`) as the last content commit.
  Gate: `task spec:check`.
