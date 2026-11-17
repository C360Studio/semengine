# Proposal: fix-686-embedder-input-type

Approved-by:

Track: `quick`. Capability modified: `embedding-index` (one existing requirement changed; none added).

## Why

An asymmetric embedding model needs its own instruction in front of a search query; without it the tier 1 vector
search loses about a third of its score (#686). Today `httpembedder.Config.QueryPrefix` makes each product supply that
instruction for its model, and nothing checks it. semembed (from `c4cfb309`) adds each model's own query or document
instruction itself when the request carries `input_type` (`query` or `document`, default `document`), and leaves text
that already starts with one of the model's instructions unchanged.

## What Changes

- `GenerateQuery` sends `"input_type": "query"` and `Generate` sends `"input_type": "document"` with every request, so
  a product on semembed no longer needs to know its model's instruction.
- New `Config.OmitInputType`: true sends no `input_type`, for a provider that refuses the field. Default false (the
  owner's ruling, 2026-11-17: send it, with a setting to turn it off).
- `Config.QueryPrefix` stays as an opt-in for a provider that does not apply instructions itself: the client still puts
  it before each query. On semembed it does no harm, since text already carrying the model's instruction is sent
  unchanged; on a provider that applies its own instruction for `input_type` and does not recognise the client's, the
  query would carry both, so a product sets one or the other.
- The client does not try to tell whether a provider honoured `input_type`: semembed's `prefix_applied` is one flag
  for the whole batch and is also false for a model with no instruction, so it cannot say. The tier 1 scorecard is the
  check.

## Non-goals

Reading `prefix_applied`; changing the scorecard's flags; per-call input types; other providers' field names.

## Impact

`graph/embedding/httpembedder` (`Config`, request body, tests); the `embedding-index` spec's query-prefix requirement;
`processor/graph-embedding/README.md` and `MIGRATION.md`, the ledger row and `docs/repository-map.md` where they
describe `QueryPrefix`. Products on semembed can drop `QueryPrefix`; nothing breaks for those that keep it.
