# Tasks: simplify-04a-10-types-message-vocab

Each task names the outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec deltas pass pre-owner review and are
  accepted by the owner on #219 (owner questions 1–5). Gate: `task spec:check` and `task docs:check`.
  Done: review PASS at `9771bb4`; owner accepted all five on #219 (comment 6215330911).

## 2. Failing tests

- [x] 2.1 `TestTypeIsValidMatchesValidate` (D1): twelve rows, three components by four classes. Gate: the dotted and
  non-UTF-8 rows fail on the base commit.
  Done: the six dotted and non-UTF-8 rows fail on `cfba65a`, the rest pass (`notes/2.1.md`).
- [x] 2.2 `TestBaseMessageValidateRefusesTypeGrammar` (D2), and `FuzzDecoderStrings`' oracle changed
  (`message/decoder_fuzz_test.go:345-348`, `:375-381`, `:410-418`): a type holding `.` is refused at marshal with
  invalid data, and the read-back branch goes. Gate: both fail on the base commit (the fuzz target on its `a.b` seed).
  Done: both fail on `cfba65a`, the fuzz target on seed #7 `a.b` (`notes/2.2.md`).
- [x] 2.3 `TestGenericJSONValidateRefusesWhatTheShapeCheckRefuses` and the law in `FuzzGenericJSONShapes` (D3):
  `Validate` refuses with invalid data exactly when the generator marked `foreign` or `badUTF8`, and returns nil for
  `nonJSON` (NaN, infinity) and valid values (owner question 5, option 1). Gate: both fail on the base commit.
  Done: both fail on `b840587`, the fuzz target on seeds #0, #2 and #19–#26 (`notes/2.3.md`).
- [x] 2.4 `TestRegistryLooksUpByType` (D6). Gate: fails on the base commit (no typed signature).
  Done: fails to compile on `ca8eb5f`, no typed signature (`notes/2.4.md`).
- [x] 2.5 `TestParsePredicateDoesNotAllocate`, with a fresh predicate on every run including the warm-up (D7).
  Gate: fails on the base commit.
  Done: fails at `ac02766` (test added, parse unchanged), 4 allocations per run (`notes/2.5.md`).
- [x] 2.6 `FuzzParsePredicateAgreesWithGrammar` (D7, generated checks): a grammar oracle and a copy of the base parse;
  seeds hold one input per reason, a multi-fault input such as `a..b.c`, and the 64/65 and 194/195-byte limits.
  Gate: passes on the base commit. Its ability to fail is shown in task 3.7, on the new code.
  Done: passes at `ac02766` (test added, parse unchanged), all 12 seeds (`notes/2.6.md`).

## 3. Implementation

- [x] 3.1 `Type.IsValid` is `Validate() == nil`. Gate: 2.1 passes.
  Done: `TestTypeIsValidMatchesValidate` passes at `a8ace02`.
- [x] 3.2 `BaseMessage.Validate` calls `Type.Validate`; the UTF-8 loop in `MarshalJSON` goes. Gate: 2.2 and the
  existing non-UTF-8 type test pass; one `task mutate:check` run that deletes the new call fails that test.
  Done: 2.2 and `TestBaseMessageRefusesTypeThatIsNotUTF8` pass at `a8ace02`; mutate:check verdict detection
  (`notes/3.2.md`).
- [x] 3.3 `GenericJSONPayload.Validate` runs the shape check; `MarshalJSON` and its key sort unchanged; the
  `Validate` doc names the values only encoding refuses (owner question 5). Gate: 2.3 passes.
  Done: 2.3 passes at `c2be504`; a 20 s `-fuzz` run found no failure (`notes/2.3.md`).
- [x] 3.4 `Hash` leaves `Message` and `BaseMessage` with its test and the "Content-addressable" doc line (owner
  question 1; under option 2, `Hash() (string, error)` and `TestHashRefusesAPayloadThatCannotEncode`). Gate:
  `task build`, `task lint`.
  Done: option 1; `task build` and `task lint` pass at `a7cd586`; `grep -rnw Hash --include='*.go'`: 0.
- [x] 3.5 Remove `message.Keyable`, `message.EntityType`, `message.EntityID`, `Triple.IsExpired` and
  `EntityType.IsValid` (owner question 2); `triple_identity.go`'s hazard note stops citing `IsExpired`. Gate:
  `task build`, `task lint`.
  Done: `task build` and `task lint` pass at `1ef1d60`; no Go reader of the five remains (`notes/3.5.md`).
- [x] 3.6 Registry keyed by `types.Type`; `GetRegistration` and `IndexingProfileFor` typed; graph-ingest's three calls
  drop `.Key()`; `Contracts` sorts typed keys by `Key()`; `List` and `Registration.MessageType` spell the key with
  `Type.Key()` (owner question 4; otherwise D6′). Gate: 2.4, `TestContractsReturnsIndependentSortedCopies`,
  `TestRegistry_ListPayloads`, `TestRegistry_ListByDomain`, `TestRegistry_BuildPayload_Success` and the graph-ingest
  unit tests pass.
  Done: all five and `./processor/graph-ingest/...` pass at `8cc4c63`; the strengthened `Contracts` order test detects a
  component-order sort mutant (`notes/2.4.md`).
- [x] 3.7 `ParsePredicate` with `strings.Cut`; the memo and `predicate_memo_test.go` go. Gate: 2.5 and 2.6 pass;
  one `task mutate:check` run against the new `strings.Cut` code, with a mutant that checks segments before arity,
  is detected by 2.6 (on the `a..b.c` seed); before/after `go test -bench` recorded in `notes/3.7.md` as
  information.
  Done: 2.5 and 2.6 pass at `8a724ce`; mutate:check verdict detection on seed #9 `a..b.c`; bench recorded
  (`notes/3.7.md`).
- [x] 3.8 Delete `parseDomainCategory`. Gate: `task build`.
  Done: `task build` passes at `3914d44`.

## 4. Documentation

- [x] 4.1 READMEs and doc comments (D5): `pkg/types/README.md:71-98`, `pkg/types/doc.go:6`, `message/doc.go:78`,
  `:155-157`, `message/message.go:11`, `graph/README.md:39`, `vocabulary/README.md:480`, and the `message` README
  line that offers `message.Type`. Gate: `task docs:check`.
  Done: `4b5c858`; `task docs:check` and `task build` exit 0. `pkg/types/doc.go:6` stays: `Keyable` is still in `pkg/types`.
- [x] 4.2 Comment on #90 with the measurement: `DcIdentifier` is `vocabulary/standards.go:47`, semconnect reads it
  (`parser/sensorml/predicates.go:129`), and `vocabulary/README.md:300` is accurate. Gate: the comment is read back.
  Done: posted and read back, #90 comment 6218976490.
- [x] 4.3 Post the `strings.Cut` allocation probe on the PR with its source, command, Go version and output (D7).
  Gate: the comment is read back.
  Done: posted and read back, #275 comment 6218742351 (Go 1.26.9: `strings.Cut` 0 allocations, `strings.Split` 1).
- [x] 4.4 `docs/admission-ledger.yaml`: the four rows as the design's Ledger section says. Gate: `task ledger:check`,
  `task docs:check`.
  Done: `task ledger:check` and `task docs:check` pass at `b7390ac` (`notes/4.4.md`).

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-10-types-message-vocab`) as the last content commit.
  Gate: `task spec:check`.
