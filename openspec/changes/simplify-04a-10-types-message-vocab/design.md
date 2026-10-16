# Design: simplify-04a-10-types-message-vocab

Status: draft, for inventory review, then pre-owner design review, then the owner's answers on #219.
Inventory: `inventory.md` (base `e8c90fb`).

## Context

Epic #223, PR 3: issues #219 (items 1–5), #78 and #90. The owner's direction on #223 (comment 6214503370) asks for
every export this change removes, unexports or keeps, with its #190 kind; that is the table under "Exports". Two
of the claim's premises are false (inventory § 1): `message/types.go` has readers outside `message` (`message.Type`,
`message.ParseEntityID`), and `DcIdentifier` is still a Go constant, so `vocabulary/README.md:300` is accurate. No ADR
changes: ADR-074, ADR-103 and ADR-054 (pin) are kept as they are (inventory § 6).

## One pull request or two (#159)

Recommended: **one pull request.** About 260 non-test lines change: `message` about 90, `vocabulary` about 110
(mostly deletions), `payloadregistry` about 25, `pkg/types` about 5, and graph-ingest 3 call sites (owner question 4).
About 35 README lines change as well. #159 bounds porting pull requests at about 3,000 pin lines each; this one ports
nothing and is a tenth of that. PR 5 carried three packages in one pull request on the same reasoning. The first
three packages also depend on each other: D2 needs D1, and D6 uses `types.Type` as the key.

If the owner wants the split anyway: **3a** `pkg/types` + `message` + `payloadregistry` (D1–D6), then **3b**
`vocabulary` (D7–D8). 3b touches nothing 3a touches, so either can land first; 3a first keeps #78 and #219's
larger part together.

## Options for `Hash` (#78)

1. **Remove `Hash` from `Message` and `BaseMessage` (recommended).** Nothing calls it here, at the pin or in a
   consumer (inventory § 4), and no contract line names it. Cost: a product that wants a content digest computes one
   over the encoded message. Owner question 1.
2. **`Hash() (string, error)`**, returning the payload's marshal error. It fails closed. Cost: the `Message`
   interface changes, and an API with no reader keeps its maintenance.
3. **Do nothing.** A refused payload keeps hashing as its type alone, and #78 stays open.

## Decisions

### D1 `Type.IsValid` is `Validate() == nil` (#219 item 1)

`IsValid` reports false exactly when `Validate` returns an error: an empty, non-UTF-8 or dotted component. Today it
reports true for the last two. Proof: `TestTypeIsValidMatchesValidate`, a table of each component (domain, category,
version) crossed with each class (valid, empty, invalid UTF-8, contains `.`). The dotted and non-UTF-8 rows fail on
the base commit. The spec delta adds the contract line. Callers that tighten (inventory § 4) refuse with the class
they already used. graph-ingest's create path (`canonical_mutations.go:234`) now answers `invalid_request` instead of
`message_type_unregistered` for such a type. Such a type cannot be registered (`registry.go:134`), so it was refused
before too.

### D2 `BaseMessage.Validate` refuses what `Type.Validate` refuses (#219 item 1)

`Validate` calls `m.msgType.Validate()` and wraps its error as invalid data. The UTF-8 loop in `MarshalJSON`
(`base_message.go:212-220`) goes, since `MarshalJSON` already calls `Validate` first (`:193`). Proof: new
`TestBaseMessageValidateRefusesTypeGrammar` (a dotted category is refused; it fails on the base commit). The existing
non-UTF-8 type test still passes. One `task mutate:check` run that deletes the new `Validate` call makes it fail,
which shows `Validate` is now the only check.

### D3 Generic JSON `Validate` refuses what `MarshalJSON` refuses (#219 item 2)

`Validate` runs the shape check after the nil check. **`MarshalJSON` keeps it.** The epic line says "move", but the
spec requires `MarshalJSON` itself to refuse (`message-codec` `:41`, `:63`). A caller that encodes without validating
would otherwise write U+FFFD with no error, which breaks the content-preservation gate. Cost: on `BaseMessage`'s path
the fallback payload is walked twice. The key sort stays, because it makes the refusal name the same key every time
(`generic_json.go:183`). So the "double sort" item is declined. Proof: new
`TestGenericJSONValidateRefusesWhatMarshalJSONRefuses`, built from the shape-test fixtures, fails on the base commit.
`FuzzGenericJSONShapes` also gains the law `Validate() == nil` exactly when `MarshalJSON` succeeds, for non-nil
`Data`.

### D4 `Hash` (#78; owner question 1)

Option 1: `Hash` leaves `Message` (`message.go:37-39`) and `BaseMessage` (`base_message.go:125-147`) with its test
(`base_message_test.go:168-174`). The `Message` doc's "Content-addressable" line (`message.go:11`) goes. The
ledger's `known_risks` sentence goes. Proof: `task build`, `task lint`, `task cover:check` (the `message` 80% floor).
Under option 2 instead: `TestHashRefusesAPayloadThatCannotEncode`. Two messages of one type whose payloads refuse to
encode each return an error, and neither returns a digest.

### D5 Unread exports go; `message.Type` stays (#219 item 3; owner question 2)

Removed: `message.Keyable`, `message.EntityType`, `message.EntityID`, `Triple.IsExpired`, `EntityType.IsValid`. None has
a reader outside its own tests, here, at the pin or in a consumer (inventory § 4). `message.ParseEntityID` now returns
`types.EntityID`, the same type. `triple_identity.go:31-35` keeps its hazard note but no longer cites `IsExpired`.
`message.Type` stays: 55 consumer production lines and `graph.EntityState.MessageType` name it. The `message` README
gains the line that offers it (#190). Proof: `task build`, `task lint`. Docs follow: `pkg/types/README.md:71-98`,
`message/doc.go:78`, `:155-157`, `vocabulary/README.md:480`.

### D6 The payload registry is keyed by `types.Type` (#219 item 4; question 4)

The map key is `types.Type`. `Create` and `Build` build the key from their arguments, so their signatures stay.
`GetRegistration` and `IndexingProfileFor` take a `types.Type`, and their three graph-ingest callers drop `.Key()`.
`List` keeps string keys, built with `Registration.MessageType()`, which becomes `types.Type{…}.Key()`: one
spelling of the dotted key. Behaviour does not change. A component holding `.` cannot be registered, so string keys
were already one-to-one. Proof: `TestRegistryLooksUpByType` (a registered type is found by its value; a dotted type
whose key text equals no registered key reports not registered), plus the existing registry and graph-ingest tests.
If the owner declines the signature change, the map cannot be keyed by type without parsing the string argument
back into a type, a second parser. The fallback is then D6′: keep `map[string]` and spell every key with
`Type.Key()`.

### D7 `ParsePredicate` parses with no memo and no allocation (#219 item 5)

`ParsePredicate` splits with two `strings.Cut` calls and one `strings.IndexByte`. It keeps every verdict, every `Reason`
and every `Segment` exactly. The memo, its counter, its cap and `memoizeParsedPredicate` go, and with them the
process-wide state (up to about 20 MB by its own comment, `predicate_contract.go:89-92`). Proof:
`TestParsePredicateDoesNotAllocate` gives every run, `testing.AllocsPerRun`'s warm-up included, a valid predicate not
parsed before, taken from a list built outside the measured function, and requires zero allocations. It fails on the
base commit, where each such parse is a cold one that allocates in `strings.Split` and in the memo's store.
`FuzzParsePredicateAgreesWithGrammar` replaces `predicate_memo_test.go` and extends `FuzzParsePredicateNeverPanics` (see
generated checks). Probe (Go 1.26.9, darwin/arm64; posted on the PR with source, command and output): a `strings.Cut`
split of `agent.run.entity-id` allocates 0 objects per run with and without `-race`, and a `strings.Split` split
allocates 1 (48 B). The design makes no speed claim. The developer records `go test -bench` before and after in `notes/`
as information.

### D8 Dead code and #90

`parseDomainCategory` is deleted; nothing calls it. #90 needs no edit. Its ruling (comment 6214442133) said PR 3
closes it, and its last item, the README line, is accurate (inventory § 1). The PR body keeps `Closes #90`, and
task 4.2 posts the measurement on #90.

Not in this change: F-1 (the registry's three empty checks before `Validate`; optional, the error identity differs),
`ParseEntityTypeKey`'s `strings.Split`, and the two entity-type grammars (F-3).

## Exports (#190)

Kinds: **API** is framework API, with its contract line and the test that drives it as a product would. **Plumbing**
is exported for another package here, with its production caller. **Unread** has no reader and no contract line, so
the owner decides. The table lists every export the change touches or an issue names; the four packages' other
exports are untouched and not audited here.

| Symbol | Action | Kind | Contract line or caller; driving test |
| --- | --- | --- | --- |
| `types.Type.IsValid` | keep, D1 | API | spec delta (new); semconnect `graph_mutations.go:140`; `TestTypeIsValidMatchesValidate` |
| `types.Type.Validate` | keep | API | `message-codec` `:40`; `TestTypeValidateOwnsComponentGrammar` |
| `types.EntityType.IsValid` | remove | Unread | none; owner question 2 |
| `types.EntityType.Validate` | not added | (new) | no reader would exist; owner question 3 |
| `message.Type` | keep | API | `message` README line (new, D5); 55 consumer lines; `graph/types.go:40` |
| `message.Keyable`, `.EntityType`, `.EntityID` | remove | Unread | none; owner question 2 |
| `message.ParseEntityID` | keep | API | ledger `message` contract "entity-ID parsing"; `entity_id_contract_test.go:32`; `vocabulary/export/export.go:135` |
| `message.IndexingProfiler` | keep | API | ledger `message` contract; `message/README.md:66`; `component.go:1913`; `TestIndexingProfile_Graphable_IndexingProfilerWins` |
| `Triple.IsExpired` | remove | Unread | none; owner question 2 |
| `Message.Hash`, `BaseMessage.Hash` | remove (option 1) | Unread | none; owner question 1 |
| `BaseMessage.Validate` | keep, D2 | API | `message-codec` (encoding validates); spec delta; `TestBaseMessageValidateRefusesTypeGrammar` |
| `GenericJSONPayload.Validate` | keep, D3 | API | spec delta; `TestGenericJSONValidateRefusesWhatMarshalJSONRefuses` |
| `GenericJSONPayload.MarshalJSON` | keep | API | `message-codec` `:63`; `generic_json_shape_test.go` |
| `Registry.GetRegistration` | keep, typed (D6) | Plumbing | `canonical_mutations.go:218`, `component.go:678` |
| `Registry.IndexingProfileFor` | keep, typed (D6) | Plumbing | `component.go:2063` |
| `Registry.Create` | keep | Plumbing | `message/base_message.go:315` (also semteams `scripts/…:572`) |
| `Registry.List` | keep | API | ledger "lookups"; semteams `scripts/measure-agent-contract-family.go:571` |
| `Registry.Build` | keep, untouched in behaviour | Unread | none here, at the pin or in a consumer; owner question 2 (kept unless you say otherwise) |
| `Registration.MessageType` | keep | Plumbing | `payloadregistry/registry.go` `List` (today `:153`) |
| `vocabulary.ParsePredicate` | keep, D7 | API | ledger `vocabulary` contract; spec delta (new); `TestParsePredicate`; `graph/entity_predicate_contract.go:98` |
| `vocabulary.DcIdentifier` | keep | API | `vocabulary/README.md:300`; semconnect `parser/sensorml/predicates.go:129` |

Counts (23 existing exported symbols): API 11, Plumbing 4, Unread 8 (7 removed if the owner agrees, `Build` kept), back
door 0; and 1 proposed symbol not added (`EntityType.Validate`). Unexported code removed: `parseDomainCategory`,
`validParsedPredicates`, `validParsedPredicateCount`, `maxMemoizedPredicates`, `memoizeParsedPredicate`.

## Generated checks (`docs/testing.md`)

- **`ParsePredicate`: used.** It is an input format with many interacting cases and size limits.
  `FuzzParsePredicateAgreesWithGrammar` checks two rules on every input. (1) It accepts exactly the ADR-074 grammar,
  with the same parts, by an oracle the test owns: a per-segment regular expression
  `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`, three segments, at most 64 bytes each and 194 in total. (2) Its error equals the
  pre-change parse, copied into the test file as `parsePredicateBySplit`, in `Reason` and `Segment`. Oracle (2) is the
  base implementation on purpose: the promise is "no verdict or reason changes", and the order of reasons for an
  input with several faults is defined by the base code, not by the grammar. The seeds hold one input per reason,
  plus 64/65-byte segments and 194/195-byte totals. The PR reports the seed replay (`task test:unit`) and one
  exploratory `-fuzz` run separately, with its `-fuzztime`.
- **`GenericJSONPayload.Validate`: used**, as the law added to `FuzzGenericJSONShapes` (D3).
- **`Type.IsValid`: examples suffice.** Three components times four classes is the whole space for a boolean that
  hides which component failed. The table lists all twelve rows.
- **Registry keys, removals, `Hash`: not needed.** No new grammar or history. Struct equality is the key, and the
  compiler finds a removed symbol's readers.

## Issues

| Issue | Disposition |
| --- | --- |
| #219 | Retired (`Closes #219`): tasks 2.1–2.6, 3.1–3.8. Item 2's "double sort" is declined (D3) and item 1's `EntityType.Validate` depends on owner question 3 |
| #78 | Retired (`Closes #78`): task 3.4 |
| #90 | Retired (`Closes #90`) on its ruling, with no edit: task 4.2 records the measurement |
| #33 | Stays: the `message` half is done; the rest is in graph-ingest, `internal/harness/payloadfixture` and unported `storage/objectstore` |
| #134, #139 | Stay: their work is in `graph/inference` and graph-ingest |

## Ledger

All four rows stay `adapt`; none is `carry`, so `task ledger:check` does not compare them with the pin.

- `pkg/types`: `contract` adds "`Type.IsValid` is `Validate() == nil`; `EntityType.IsValid` removed";
  `proving_tests` adds `TestTypeIsValidMatchesValidate`.
- `message`: `contract` adds that `BaseMessage.Validate` checks the type grammar, that `GenericJSONPayload.Validate`
  refuses what `MarshalJSON` refuses, and that `message.Type` and `ParseEntityID` are the aliases kept. `known_risks`
  drops the #78 sentence and adds to the dead-surface list `Hash`, `Keyable`, `EntityType`, `EntityID` and
  `IsExpired`.
- `payloadregistry`: `contract` says lookups take `types.Type` (D6).
- `vocabulary`: `known_risks` adds "`ParsePredicate` keeps no memo; `parseDomainCategory` removed";
  `proving_tests` replaces `predicate_memo_test.go` with the fuzz target.

## Merge order

No open PR shares a file in the four packages. #273 (PR 7) edits `processor/graph-ingest/component.go` at other lines;
D6 edits `:678` and `:2063`. #272 and #273 both edit other rows of `docs/admission-ledger.yaml`. Rule, as PRs #265
and #266 used it: whichever lands second merges `origin/main` first. The Dependabot PRs touch `go.mod` and workflows only.
PR 10 (graph-ingest, unclaimed) should start after this PR if question 4 is accepted.

## Owner questions

1. **Remove the message hash?** Recommended: yes. `Hash` is meant to fingerprint a message, but nothing calls it:
   not here, not in SemStreams, not in any consumer. It also hides a failed payload encoding (#78). Removing it is
   the smallest fix. The other answer, `Hash() (string, error)`, keeps an unused method and changes the `Message`
   interface.
2. **Remove five unused exports, and keep two?** Recommended: yes. Nothing reads these five anywhere: `message.Keyable`,
   `message.EntityType`, `message.EntityID`, `Triple.IsExpired` and `EntityType.IsValid`. `message.Type` stays, unlike
   what the epic line says: the four consumers write it 55 times in production, and the graph's stored entity record
   (`graph.EntityState.MessageType`) uses it. `Registry.Build` also has no reader, but this PR does not touch what it
   does, so it stays unless you say otherwise.
3. **Skip adding `EntityType.Validate`?** Recommended: skip it. #219 asks for it, but no code would call it, and two
   rules for an entity type already disagree (`Robotics_1` passes the entity-ID rule and fails the vocabulary rule).
   Adding one means choosing between them, which is a separate question.
4. **Have the payload registry's lookups take a type, not a string?** Recommended: yes. Every caller already holds
   the type and turns it into text only to look it up. Two methods change their argument. No consumer calls them,
   and graph-ingest's three calls get shorter. The other answer keeps the string keys and only spells them once.

For information: #90's ruling said the README's `DcIdentifier` line was stale. It is not. The constant still exists
and semconnect uses it, so this PR closes #90 without an edit and posts the measurement there.
