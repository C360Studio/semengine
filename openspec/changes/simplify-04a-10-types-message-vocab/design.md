# Design: simplify-04a-10-types-message-vocab

Status: accepted. Review passed at `9771bb4` (three rounds); the owner accepted questions 1–5 as recommended on #219
(comment 6215330911). Inventory: `inventory.md` (base `e8c90fb`).

## Context

Epic #223, PR 3: issues #219 (items 1–5), #78 and #90. The owner's direction on #223 (comment 6214503370) asks for
every export this change removes, unexports or keeps, with its #190 kind; that is the table under "Exports". Two
of the claim's premises are false (inventory § 1): `message/types.go` has readers outside `message` (`message.Type`,
`message.ParseEntityID`), and `DcIdentifier` is still a Go constant, so `vocabulary/README.md:300` is accurate. No ADR
changes: ADR-074, ADR-103 and ADR-054 (pin) are kept as they are (inventory § 6).

`*payloadregistry.Registry` reaches every product component through `component.Dependencies.PayloadRegistry`
(`component/dependencies.go:53`), so every exported `Registry` method is framework API, as `pkg/security` is through
`Dependencies.Security` (owner's amended ruling on #90, comment 6214503074).

## One pull request or two (#159)

Recommended: **one pull request.** About 270 non-test lines change: `message` about 90, `vocabulary` about 110
(mostly deletions), `payloadregistry` about 35, `pkg/types` about 5, and graph-ingest 3 call sites (owner question 4).
About 35 README lines change as well. #159 bounds porting pull requests at about 3,000 pin lines each; this one ports
nothing and is a tenth of that. PR 5 carried three packages in one pull request on the same reasoning. The first
three packages also depend on each other: D2 needs D1, and D6 uses `types.Type` as the key.

If the owner wants the split anyway: **3a** `pkg/types` + `message` + `payloadregistry` (D1–D6), then **3b**
`vocabulary` (D7–D8). 3b touches nothing 3a touches, so either can land first; 3a first keeps #78 and #219's
larger part together.

## Options for `Hash` (#78)

1. **Remove `Hash` from `Message` and `BaseMessage` (recommended).** Nothing calls it here, at the pin or in a
   consumer (inventory § 4). No contract line names it; the ledger's `message` contract ("BaseMessage … as at the
   pin") covers it only if read broadly. Cost: a product that wants a content digest computes one over the encoded
   message. Owner question 1.
2. **`Hash() (string, error)`**, returning the payload's marshal error. It fails closed. Cost: the `Message`
   interface changes, and an API with no reader keeps its maintenance.
3. **Do nothing.** A refused payload keeps hashing as its type alone, and #78 stays open.

## Options for generic JSON validation (#219 item 2)

Today `Validate` checks only `Data != nil`; the shape check (kinds, UTF-8, cycles) runs in `MarshalJSON`, which then
calls `json.Marshal`. `json.Marshal` refuses three more things the shape check accepts: a NaN or infinite `float32` or
`float64`, and a `json.Number` whose text is not a JSON number (inventory § 1).

1. **`Validate` also runs the shape check; `MarshalJSON` keeps it; NaN, infinity and bad number text stay refused
   only by encoding (recommended).** No behaviour changes except that `Validate` refuses earlier what the shape check
   refuses. Decoded input never holds these three (JSON has no NaN, and decoded numbers are valid literals), so
   only in-process code can build them, and generic JSON is for outside input. Cost: `Validate` passing does not
   promise encoding passes for those three values; the doc says so.
2. **Also refuse the three in the shape check.** `Validate` passing then promises encoding passes. Cost: new code
   (a finite check and a number-literal check), and their refusal changes from `encoding/json`'s unclassified error
   to an invalid-data error naming the path.
3. **Move the check out of `MarshalJSON`, as the epic line says.** Rejected: the spec requires `MarshalJSON` itself to
   refuse (`message-codec` `:41`, `:63`). A caller that encodes without validating would write U+FFFD with no error,
   which breaks the content-preservation gate.

The key sort (`generic_json.go:184`) makes a refusal name the same key every time. A walk in map order that re-runs
sorted on its first failure gives the same stable error with no sort on success. Recommended: keep the sort. It is
one code path, and it costs only on the fallback payload's encoding. The re-run is a second path whose saving was not
measured. Owner question 5 covers both choices.

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
(`base_message.go:212-220`) goes, since `MarshalJSON` already calls `Validate` first (`:193`). A message whose type
holds `.` is no longer encodable. **This changes an existing proving test's oracle**: `FuzzDecoderStrings`
(`message/decoder_fuzz_test.go:345-348`, `:375-381`, `:410-418`, seed `{"a.b",…}`) expects such a message to marshal
and be read back unchanged. Its oracle becomes "marshal is refused, invalid data, exactly when `Validate` refuses",
and the `default:` read-back branch goes. Proof: new `TestBaseMessageValidateRefusesTypeGrammar` (a dotted category
is refused; it fails on the base commit), and the changed `FuzzDecoderStrings`, which fails on the base commit on its
`a.b` seed. The existing non-UTF-8 type test still passes. One `task mutate:check` run that deletes the new
`Validate` call makes it fail, which shows `Validate` is now the only check.

### D3 Generic JSON `Validate` runs the shape check (#219 item 2; question 5)

Option 1 above. `Validate` runs the shape check after the nil check; `MarshalJSON` and the sort are unchanged. The
`Validate` doc names the three values only encoding refuses. Proof: new
`TestGenericJSONValidateRefusesWhatTheShapeCheckRefuses`, built from the shape-test fixtures (a value that is not
JSON-shaped, a cycle, invalid UTF-8 at depth). It fails on the base commit. `FuzzGenericJSONShapes` gains this law:
for non-nil `Data`, `Validate` returns an invalid-data error exactly when the generator marked the value `foreign` or
`badUTF8`, and returns nil when it marked it `nonJSON` (the NaN seed, `generic_json_fuzz_test.go:215`) or valid.
Under option 2, `nonJSON` moves to the refused side, and `MarshalJSON`'s error for it becomes invalid data.

### D4 `Hash` (#78; owner question 1)

Option 1: `Hash` leaves `Message` (`message.go:37-39`) and `BaseMessage` (`base_message.go:125-147`) with its test
(`base_message_test.go:168-174`). The `Message` doc's "Content-addressable" line (`message.go:11`) goes. The
ledger's `known_risks` sentence goes. Proof: `task build`, `task lint`, `task cover:check` (the `message` 80% floor).
Under option 2 instead: `TestHashRefusesAPayloadThatCannotEncode`. Two messages of one type whose payloads refuse to
encode each return an error, and neither returns a digest.

### D5 Unread exports go; `message.Type` stays (#219 item 3; owner question 2)

Removed: `message.Keyable`, `message.EntityType`, `message.EntityID`, `Triple.IsExpired`, `EntityType.IsValid`.
None has a reader outside its own tests, here, at the pin or in a consumer (inventory § 4). `message.ParseEntityID`
now returns `types.EntityID`, the same type. `triple_identity.go:31-35` keeps its hazard note but no longer cites
`IsExpired`. `message.Type` stays: 55 consumer production lines and `graph.EntityState.MessageType` name it. The
`message` README gains the line that offers it (#190). Proof: `task build`, `task lint`. Docs follow:
`pkg/types/README.md:71-98`, `pkg/types/doc.go:6`, `message/doc.go:78`, `:155-157`, `message/message.go:11`,
`graph/README.md:39` (it names `message.EntityID`), `vocabulary/README.md:480`.

### D6 The payload registry is keyed by `types.Type` (#219 item 4; question 4)

The map key is `types.Type`. `Create` and `Build` build the key from their arguments, so their signatures stay.
`GetRegistration` and `IndexingProfileFor` take a `types.Type`; this is a signature change on framework API. No
consumer calls either (inventory § 4), and graph-ingest's three callers drop `.Key()`. `Contracts` keeps its
promised order, by message-type key and then by contract name: it sorts the typed keys by `Key()`. `List` keeps
string keys, built with `Key()`. `ListByDomain` only iterates. Behaviour does not change. A component holding `.`
cannot be registered, so string keys were already one-to-one. Proof: `TestRegistryLooksUpByType` (a registered type
is found by its value; a dotted type whose key text equals no registered key reports not registered). The existing
`TestContractsReturnsIndependentSortedCopies` (`attributes_test.go:175-186`) fixes the `Contracts` order, and
`TestRegistry_ListPayloads`, `TestRegistry_ListByDomain`, `TestRegistry_BuildPayload_Success` and the graph-ingest
tests also pass unchanged. If the owner declines the signature change, the map cannot be keyed by type without
parsing the string argument back into a type, a second parser. The fallback is then D6′: keep `map[string]` and
spell every key with `Type.Key()`.

### D7 `ParsePredicate` parses with no memo and no allocation (#219 item 5)

`ParsePredicate` splits with two `strings.Cut` calls and one `strings.IndexByte`. It keeps every verdict, every
`Reason` and every `Segment` exactly. The memo, its counter, its cap and `memoizeParsedPredicate` go, and with them
the process-wide state (up to about 20 MB by its own comment, `predicate_contract.go:89-92`). Proof:
`TestParsePredicateDoesNotAllocate` gives every run a valid predicate not parsed before, `testing.AllocsPerRun`'s
warm-up included. The predicates come from a list built outside the measured function, and the test requires zero
allocations. It fails on the base commit, where each such parse is a cold one that allocates in `strings.Split` and in
the memo's store. `FuzzParsePredicateAgreesWithGrammar` replaces `predicate_memo_test.go` and extends
`FuzzParsePredicateNeverPanics` (see generated checks).

The probe is **UNVERIFIED** until task 4.3 posts it on the PR. It is a local copy of the parse, not repository code:
`/tmp/simplify-04a-10-evidence/probe/p_test.go`, Go 1.26.9 darwin/arm64, local only. It measured a `strings.Cut`
split of `agent.run.entity-id` at 0 allocations per run with and without `-race`, and a `strings.Split` split at 1
(48 B). The design makes no speed claim. The developer records `go test -bench` before and after in `notes/` as
information.

`ParsePredicateNamespace` (`namespace_authority.go:43-60`) and `ParseEntityTypeKey` (`entity_type.go:80-98`) parse
other grammars, a namespace and an entity-type key. They share `validatePredicateSegment` with `ParsePredicate` and
stay as they are.

### D8 Dead code and #90

`parseDomainCategory` is deleted; nothing calls it. #90 needs no edit. Its ruling (comment 6214442133) said PR 3
closes it, and its last item, the README line, is accurate (inventory § 1). The PR body keeps `Closes #90`, and
task 4.2 posts the measurement on #90.

Not in this change: F-1 (the registry's three empty checks before `Validate`; optional, the error identity differs),
the `strings.Split` in `ParseEntityTypeKey` and `ParsePredicateNamespace`, and the two entity-type grammars (F-3).

## Exports (#190)

Kinds: **API** is framework API, with its contract line and the test that drives it as a product would. **Plumbing**
is exported for another package here, with its production caller. **Unread** has no reader outside its own package
or tests and no contract line that names it, so the owner decides. The table lists every export the change touches or
an issue names; the four packages' other exports are untouched and not audited here.

| Symbol | Action | Kind | Contract line or caller; driving test |
| --- | --- | --- | --- |
| `types.Type.IsValid` | keep, D1 | API | spec delta (new); semconnect `graph_mutations.go:140`; `TestTypeIsValidMatchesValidate` |
| `types.Type.Validate` | keep | API | `message-codec` `:40`; `TestTypeValidateOwnsComponentGrammar` |
| `types.EntityType.IsValid` | remove | Unread | ledger `pkg/types` "type … validation as at the pin" only if read broadly; owner question 2 |
| `types.EntityType.Validate` | not added | (new) | no reader would exist; owner question 3 |
| `message.Type` | keep | API | `message` README line (new, D5); 55 consumer lines; `graph/types.go:40` |
| `message.Keyable`, `.EntityType`, `.EntityID` | remove | Unread | none; owner question 2 |
| `message.ParseEntityID` | keep | API | ledger `message` contract "entity-ID parsing"; `entity_id_contract_test.go:32`; `vocabulary/export/export.go:135` |
| `message.IndexingProfiler` | keep | API | ledger `message` contract; `message/README.md:66`; `component.go:1913`; `TestIndexingProfile_Graphable_IndexingProfilerWins` |
| `Triple.IsExpired` | remove | Unread | none; owner question 2 |
| `Message.Hash`, `BaseMessage.Hash` | remove (option 1) | Unread | ledger "BaseMessage … as at the pin" only if read broadly; owner question 1 |
| `BaseMessage.Validate` | keep, D2 | API | `message-codec` (encoding validates); spec delta; `TestBaseMessageValidateRefusesTypeGrammar`, `FuzzDecoderStrings` |
| `GenericJSONPayload.Validate` | keep, D3 | API | spec delta; `TestGenericJSONValidateRefusesWhatTheShapeCheckRefuses` |
| `GenericJSONPayload.MarshalJSON` | keep | API | `message-codec` `:63`; `generic_json_shape_test.go` |
| `Registry.GetRegistration` | keep, typed (D6) | API | `Dependencies.PayloadRegistry`; ledger "lookups"; `TestRegistry_GetRegistration`; graph-ingest `canonical_mutations.go:218`, `component.go:678` |
| `Registry.IndexingProfileFor` | keep, typed (D6) | API | `Dependencies.PayloadRegistry`; ledger "lookups"; `TestIndexingProfileFor`; `component.go:2063` |
| `Registry.Create` | keep | Plumbing | production caller `message/base_message.go:315` (decoding); also reachable through `Dependencies.PayloadRegistry` and called by semteams `scripts/…:572`; `TestRegistry_CreatePayload_Success` |
| `Registry.Build` | keep | Unread | no contract line (it builds a payload; the ledger's "lookups" does not name it); no caller here, at the pin or in a consumer; the only reader of `Registration.Builder`, which semboids fills (`internal/boidgraph/payload.go:138`, `internal/zone/payload.go:113`); `TestRegistry_BuildPayload_Success`; owner question 2 |
| `Registry.List` | keep | API | ledger "lookups"; `TestRegistry_ListPayloads`; semteams `scripts/measure-agent-contract-family.go:571` |
| `Registry.ListByDomain` | keep | API | ledger "lookups"; `TestRegistry_ListByDomain` |
| `Registry.Contracts` | keep, re-sorted (D6) | API | doc "ordered by message-type key"; `TestContractsReturnsIndependentSortedCopies`; semconnect `cmd/cs-graph-backend/main.go:234` |
| `Registration.MessageType` | keep | Unread | in-package only (`registry.go:153`); pin reader `examples/processors/document/payload.go:291`; owner question 2 |
| `vocabulary.ParsePredicate` | keep, D7 | API | ledger `vocabulary` contract; spec delta (new); `TestParsePredicate`; `graph/entity_predicate_contract.go:98` |
| `vocabulary.DcIdentifier` | keep | API | `vocabulary/README.md:300`; semconnect `parser/sensorml/predicates.go:129` |

Counts (25 existing exported symbols): API 15, Plumbing 1 (`Create`), Unread 9 (7 removed if the owner agrees;
`Registration.MessageType` and `Registry.Build` kept), back door 0; and 1 proposed symbol not added
(`EntityType.Validate`). Unexported code removed: `parseDomainCategory`, `validParsedPredicates`,
`validParsedPredicateCount`, `maxMemoizedPredicates`, `memoizeParsedPredicate`.

## Generated checks (`docs/testing.md`)

- **`ParsePredicate`: used.** It is an input format with many interacting cases and size limits.
  `FuzzParsePredicateAgreesWithGrammar` checks two rules on every input. (1) It accepts exactly the ADR-074 grammar,
  with the same parts, by an oracle the test owns: a per-segment regular expression
  `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`, three segments, at most 64 bytes each and 194 in total. (2) Its error equals, in
  `Reason` and `Segment`, the pre-change parse, which is copied into the test file as `parsePredicateBySplit`. Oracle
  (2) is the base implementation on purpose: the promise is "no verdict or reason changes", and the order of reasons
  for an input with several faults is defined by the base code (the spec delta now states it). The seeds hold one
  input per reason, a multi-fault input such as `a..b.c`, and 64/65-byte segments and 194/195-byte totals. On the
  base commit oracle (2) is the code itself, so the target's ability to fail is shown on the new code: one
  `task mutate:check` mutant of the `strings.Cut` implementation that checks segments before arity is detected. The
  PR reports the seed replay (`task test:unit`) and one exploratory `-fuzz` run separately, with its `-fuzztime`.
- **`GenericJSONPayload.Validate`: used**, as the law added to `FuzzGenericJSONShapes` (D3).
- **`BaseMessage.Validate`: used**, through the changed `FuzzDecoderStrings` oracle (D2).
- **`Type.IsValid`: examples suffice.** Three components times four classes is the whole space for a boolean that
  hides which component failed. The table lists all twelve rows.
- **Registry keys, removals, `Hash`: not needed.** No new grammar or history. Struct equality is the key, existing
  tests fix the orders, and the compiler finds a removed symbol's readers.

## Issues

| Issue | Disposition |
| --- | --- |
| #219 | Retired (`Closes #219`): tasks 2.1–2.6, 3.1–3.8. Item 2 follows owner question 5; item 1's `EntityType.Validate` follows owner question 3 |
| #78 | Retired (`Closes #78`): task 3.4 |
| #90 | Retired (`Closes #90`) on its ruling, with no edit: task 4.2 records the measurement |
| #33 | Stays: the `message` half is done; the rest is in graph-ingest, `internal/harness/payloadfixture` and unported `storage/objectstore` |
| #134, #139 | Stay: their work is in `graph/inference` and graph-ingest |

## Ledger

All four rows stay `adapt`; none is `carry`, so `task ledger:check` does not compare them with the pin.

- `pkg/types`: `contract` adds "`Type.IsValid` is `Validate() == nil`; `EntityType.IsValid` removed";
  `proving_tests` adds `TestTypeIsValidMatchesValidate`.
- `message`: `contract` adds that `BaseMessage.Validate` checks the type grammar, that `GenericJSONPayload.Validate`
  runs the shape check, and that `message.Type` and `ParseEntityID` are the aliases kept. `known_risks` drops
  the #78 sentence and adds to the dead-surface list `Hash`, `Keyable`, `EntityType`, `EntityID` and `IsExpired`.
- `payloadregistry`: `contract` says the registry is keyed by `types.Type` and `GetRegistration` and
  `IndexingProfileFor` take one (D6).
- `vocabulary`: `known_risks` adds "`ParsePredicate` keeps no memo; `parseDomainCategory` removed";
  `proving_tests` replaces `predicate_memo_test.go` with the fuzz target.

## Merge order

No open PR shares a file in the four packages. #273 (PR 7) edits `processor/graph-ingest/component.go` at other
lines; D6 edits `:678` and `:2063`. #272 and #273 both edit other rows of `docs/admission-ledger.yaml`. The rule
PRs #265 and #266 used applies: whichever lands second merges `origin/main` first. The Dependabot PRs touch `go.mod` and
workflows only. PR 10 (graph-ingest, unclaimed) should start after this PR if question 4 is accepted.

## Owner questions

1. **Remove the message hash?** Recommended: yes. Nothing calls it here, in SemStreams or in any consumer, and it
   hides a payload that fails to encode. The ledger's broad "as at the pin" line for the message envelope could be
   read to cover it, so removing it needs your word. The other answer changes it to return an error, which changes
   the message interface and keeps a method nobody uses.
2. **Remove five unused exports and keep three others?** Recommended: yes. Nothing outside its own tests reads
   `message.Keyable`, `message.EntityType`, `message.EntityID`, `Triple.IsExpired` or `EntityType.IsValid`. The last
   one sits under the ledger's broad "type validation as at the pin" line. Kept: `message.Type`, which the four
   consumers write 55 times in production (the epic line said to remove it). `Registration.MessageType` is also kept:
   products fill in `Registration`, and this method is the one spelling of the key it registers under, which the
   registry's `List` uses after this change; unexporting it would make a product rebuild that key by hand.
   `Registry.Build` is kept: no contract line names it and nothing calls it, but it is the only code that reads the
   `Builder` field semboids fills in production, so removing it would leave that field doing nothing.
3. **Skip adding an entity-type `Validate`?** Recommended: skip it. No code would call it, and the two rules for an
   entity type already disagree (`Robotics_1` passes the entity-ID rule and fails the vocabulary rule). Adding one
   means choosing between them first.
4. **Have the payload registry's lookups take a type, not a string?** Recommended: yes. This changes two methods
   that every component can reach through its dependencies. No consumer calls them, and every caller here already
   holds the type and turns it into text only to look it up. The other answer keeps string keys and spells them in
   one place.
5. **Generic JSON: have `Validate` run the same shape check as encoding, keep that check in encoding too, and keep
   its key sort?** Recommended: yes to all three. `Validate` then catches wrong kinds, bad UTF-8 and loops early.
   Encoding still refuses them for callers that skip `Validate`, which the spec requires. One gap stays: a NaN, an
   infinity or bad number text still fails only at encoding. Only in-process code can build those. The other answer
   adds checks for them, and their error class changes. The sort keeps the error naming the same key every time.
   Dropping it saves a sort only on this fallback path and needs a second code path.

For information: #90's ruling said the README's `DcIdentifier` line was stale. It is not. The constant still exists
and semconnect uses it, so this PR closes #90 without an edit and posts the measurement there.
