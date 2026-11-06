# Proposal: fix-521-empty-prefix-page-size

## Why

Issue #521. A `graph.ingest.query.prefix` reply is fitted to the server's maximum payload byte for byte before it is
sent (`marshalFittingPrefixPage`, `processor/graph-ingest/query.go`). The fit counts the page as `"entities":[`…`]`.
A group with no keys has a nil page (`fetchEntitiesConcurrent` returns nil for no IDs), and the reply encodes it as
`"entities":null`, two bytes more than counted. The live spec says an empty group replies
`{"entities":null,"total":0}` (`openspec/specs/graph-transport-boundary/spec.md`, "A pattern with no matches skips
exact-key fallback"), so the reply stays and the count changes.

Effect: when the maximum payload is one or two bytes under the empty reply, the fit accepts the page, the reply then
breaks the fitting invariant, and the caller gets class fatal, code `internal` instead of class invalid,
`response_too_large`. A page whose keys were all deleted mid-read is a non-nil empty slice, encodes `[]`, and is
already counted right.

No real-world trigger: no deployment runs a maximum payload of a few dozen bytes, so this is MEDIUM at most (#185).

No open PR touches `processor/graph-ingest` or `openspec/specs/graph-transport-boundary` (PR #530 checked on
2026-11-06).

Light process, as on #454 and #456: proposal, tasks and spec delta; no design document; Codex reviews the code.

## What Changes

- `marshalFittingPrefixPage` counts a nil page as `null`, two bytes more than `[]`.
- A unit test, `TestMarshalFittingPrefixPageMeasuresAnEmptyPageAsSent` in
  `processor/graph-ingest/query_prefix_unit_test.go`, fits an empty page, nil and non-nil, at exactly its reply's
  encoding and one byte under it. The oracle is `json.Marshal` of the reply. At one byte under, it expects class
  invalid, `response_too_large`, with `response_bytes` equal to the reply's length. It fails on the base commit for
  the nil page (fatal `internal`, 27 bytes sent, 26 counted).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `graph-transport-boundary`: adds "An empty prefix page is fitted at the bytes it sends".
