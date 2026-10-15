# Proposal: query-prefix-pattern

## Problem Statement

Under ADR-102 (`org.platform.system.domain.type.instance`), an entity ID places `system` (position 3) ahead of
`domain.type` (positions 4–5). Today `graph.PrefixQueryRequest` (`graph/query_prefix_types.go:45-58`) accepts only a
literal 1–6 part `Prefix`, which `handleQueryPrefixWithMaxPayload` (`processor/graph-ingest/query.go:250-254`) validates
with `pkg/types.ValidateEntityIDPrefix` (`allowWildcard=false`) and evaluates via `entityBucket.KeysByPrefix`. Any `*`
wildcard segment is rejected with `entity_id_prefix_invalid`.

As a result, `C360Studio/semsparql#1` (§2.2.3, Task 4.1) cannot execute server-side `rdf:type` / `rdfs:subClassOf*`
type scans across systems (`<org>.<platform>.*.<domain>.<type>.*` or `*.*.*.<domain>.<type>.*`) via
`graph.ingest.query.prefix`, and would otherwise have to page the entire deployment or bucket over NATS and filter on
the client.

## What Changes

1. **`graph/query_prefix_types.go`:**
   - Add ``Pattern string `json:"pattern,omitempty"` `` to `graph.PrefixQueryRequest` as an optional exact 6-position
     entity-ID wildcard pattern in ADR-102 canonical order where any position may be `*`, mutually exclusive with
     `Prefix`.
2. **`processor/graph-ingest/query.go`:**
   - In `handleQueryPrefixWithMaxPayload`:
     - Reject requests where both `req.Prefix != ""` and `req.Pattern != ""` before any KV I/O with a classified
       `errs.ErrorInvalid` (`graph.ErrorCodeInvalidRequest`).
     - When `req.Pattern != ""`: validate `req.Pattern` via `semtypes.ValidateEntityIDPattern(req.Pattern)` before any
       KV I/O (returning its classified `errs.ErrorInvalid` / `types.ErrorCodeEntityIDPatternInvalid` on failure), list
       keys via `c.entityBucket.KeysByFilter(ctx, req.Pattern)`, and skip the exact-entity `c.entityBucket.Get`
       fallback (which applies only to literal `req.Prefix`).
     - Reuse the existing key sort, cursor skip, limit clamp, `fetchEntitiesConcurrent`, ID re-sort, and
       `marshalFittingPrefixPage` pipeline unchanged.
3. **Documentation & ledger:**
   - Document `PrefixQueryRequest.Pattern` in `processor/graph-ingest/README.md` and update the `graph` and
     `processor/graph-ingest` rows in `docs/admission-ledger.yaml`.

## Capabilities

- Modified capability: `graph-transport-boundary` (`openspec/specs/graph-transport-boundary/spec.md`)

## Scope and Non-Goals

- **In scope:** `graph/query_prefix_types.go`, `graph/query_prefix_types_test.go`, `processor/graph-ingest/query.go`,
  `processor/graph-ingest/component_test.go` (mock KV filter wildcard matching),
  `processor/graph-ingest/query_prefix_unit_test.go`, `processor/graph-ingest/query_prefix_integration_test.go`,
  `processor/graph-ingest/README.md`, and `docs/admission-ledger.yaml`.
- **Non-goals:**
  - Adding a new NATS request subject or modifying `graph.QueryVerbs()`.
  - Allowing wildcards in `PrefixQueryRequest.Prefix`.
  - Multi-pattern (OR) requests in a single call.
  - Server-side streaming key scans (key listing remains a snapshot followed by sorted keyset pagination, matching the
    existing prefix path).

## Issue References

- `Closes #259`
- `Addresses #9`
