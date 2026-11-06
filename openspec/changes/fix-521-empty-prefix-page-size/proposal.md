# Proposal: fix-521-empty-prefix-page-size

Approved-by:

## Why

Issue #521. A `graph.ingest.query.prefix` reply is fitted to the server's maximum payload byte for byte before it is
sent (`marshalFittingPrefixPage`, `processor/graph-ingest/query.go`). The fit counts the page as `"entities":[`…`]`.
A page with no keys is nil (`fetchEntitiesConcurrent` returns nil for no IDs), and the reply encodes it as
`"entities":null`, two bytes more than counted. That happens for an empty group, and for a cursor at or past the
group's last key, which is routine when the keys after the previous page were deleted between requests (`total` is
then above 0). The live spec keeps `{"entities":null,"total":0}` (`openspec/specs/graph-transport-boundary/spec.md`,
"A pattern with no matches skips exact-key fallback"), and graph-query's count verbs read that literal
(`query_prefix_integration_test.go`), so the reply stays and the count changes.

Effect: when the maximum payload is one or two bytes under the empty reply, the fit accepts the page, the reply then
breaks the fitting invariant, and the caller gets class fatal, code `internal`. The "Query refusal codes" requirement
says `response_too_large` when no page fits. Under that, the refusal already has the right code but names
`response_bytes` two bytes short. A page whose keys were all deleted mid-read is a non-nil empty slice, encodes `[]`,
and is already counted right.

No real-world trigger: no deployment runs a maximum payload of a few dozen bytes, so this is MEDIUM at most (#185).

Overlap: PR #530 touches nothing under `processor/graph-ingest`; its spec delta modifies another
`graph-transport-boundary` requirement and this change has no spec delta, so merge order is free (checked
2026-11-06).

Quick track: proposal and tasks. No spec delta, because the code is brought in line with the existing
"Query refusal codes" requirement rather than changing one. Codex reviews the code.

## What Changes

- `marshalFittingPrefixPage` counts a nil page as `null`, two bytes more than `[]`.
- A unit test, `TestMarshalFittingPrefixPageMeasuresAnEmptyPageAsSent` in
  `processor/graph-ingest/query_prefix_unit_test.go`, fits an empty page, nil and non-nil, at exactly its reply's
  encoding and one byte under it. The oracle is `json.Marshal` of the reply. At one byte under, it expects class
  invalid, `response_too_large`, with `response_bytes` equal to the reply's length. It fails on the base commit for
  the nil page: the reply is 27 bytes, the fit counted 25, the maximum was 26, so the refusal was fatal `internal`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. The existing "Query refusal codes" requirement in `openspec/specs/graph-transport-boundary/spec.md` already
says `response_too_large` when no page fits; this change makes the code meet it.
