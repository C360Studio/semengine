# Proposal: simplify-04a-10-types-message-vocab

## Why

Epic #223, PR 3: issues #219, #78 and #90, in `pkg/types`, `message`, `payloadregistry` and `vocabulary`, claimed as
draft PR #275 on the owner's word (2026-10-16). A message type has two validity checks that disagree, so
`BaseMessage.Validate` passes types the registry refuses. `GenericJSONPayload.Validate` passes data that `MarshalJSON`
then refuses. `BaseMessage.Hash` hides a payload that fails to encode (#78) and has no caller. `message` keeps three
aliases nothing reads, plus a wall-clock `Triple.IsExpired` read only by its test. The payload registry spells its
key with `fmt.Sprintf` in three places. `vocabulary.ParsePredicate` keeps a process-wide memo of up to 65,536 entries
to avoid one allocation that a plain split does not make. Each export kept or removed is listed with its kind (#190;
owner direction on #223, comment 6214503370).

## What Changes

- `types.Type.IsValid()` reports `Validate() == nil`, and `BaseMessage.Validate` calls `Type.Validate`, which makes the
  type check in `BaseMessage.MarshalJSON` redundant, so it goes.
- `GenericJSONPayload.Validate` runs the shape check that `MarshalJSON` runs; `MarshalJSON` keeps it (spec) and its
  key sort; a NaN or an infinity is still refused only by encoding, and number-literal syntax is left to `encoding/json`
  (owner question 5).
- `Message.Hash` and `BaseMessage.Hash` are removed (owner question 1).
- Removed with no reader: `message.Keyable`, `message.EntityType`, `message.EntityID`, `Triple.IsExpired`,
  `EntityType.IsValid` (owner question 2). Kept: `message.Type` and `message.ParseEntityID`, which consumers and this
  repository read.
- Not added: `EntityType.Validate` (owner question 3).
- The payload registry is keyed by `types.Type`. `GetRegistration` and `IndexingProfileFor`, framework API through
  `component.Dependencies.PayloadRegistry`, take a `types.Type` (owner question 4).
- `ParsePredicate` splits with `strings.Cut`. Its memo and the dead `parseDomainCategory` go. Every verdict and reason
  stays the same.
- #90: no edit; `vocabulary/README.md:300` is accurate.

## Capabilities

- Modified: `message-codec`, `vocabulary-packs`.

## Impact

- `pkg/types/type.go`, `entity_type.go`, `README.md`, `doc.go`.
- `message/base_message.go`, `message.go`, `types.go`, `triple.go`, `triple_identity.go`, `generic_json.go`,
  `doc.go`, `README.md`, and their tests.
- `payloadregistry/registry.go` and its tests; `processor/graph-ingest/canonical_mutations.go:218`,
  `component.go:678`, `:2063` (owner question 4).
- `vocabulary/predicate_contract.go`, `registry.go`, `README.md:480`; `predicate_memo_test.go` is replaced.
- `docs/admission-ledger.yaml`: rows `pkg/types`, `message`, `payloadregistry`, `vocabulary`.
- Exported surface: no symbol added; up to seven removed, all with no reader here, at the pin or in a consumer; two
  signatures change (owner question 4).
- Closes #219, #78, #90. Addresses #223.
