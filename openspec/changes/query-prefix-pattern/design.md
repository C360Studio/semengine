# Design: query-prefix-pattern

## Context

`C360Studio/semengine#259` (`Addresses #9`) adds an optional fixed-position wildcard `Pattern` field to
`graph.PrefixQueryRequest` on the existing `graph.ingest.query.prefix` verb so `C360Studio/semsparql#1` (§2.2.3, Task
4.1) can scan entities by `<domain>.<type>` across all `system` values (`<org>.<platform>.*.<domain>.<type>.*` or
`*.*.*.<domain>.<type>.*`) without paging the entire deployment over NATS.

All underlying primitives already exist in the repository:

- `pkg/types.ValidateEntityIDPattern` (`pkg/types/entity_id.go:160-164`) validates an exact 6-position entity-ID
  pattern where each position is either a canonical literal segment or `*`, returning a classified `errs.ErrorInvalid`
  with code `types.ErrorCodeEntityIDPatternInvalid` (`"entity_id_pattern_invalid"`).
- `natsclient.KVStore.KeysByFilter` (`natsclient/kv.go:618-696`) lists all keys in a NATS KV bucket matching an exact
  6-position NATS subject filter with `*` token wildcards via `kv.bucket.WatchFiltered(ctx, []string{pattern},
  jetstream.IgnoreDeletes(), jetstream.MetaOnly())`.
- `handleQueryPrefixWithMaxPayload` (`processor/graph-ingest/query.go:288-339`) sorts listed keys lexicographically,
  applies cursor keyset skipping (`DecodeCursor`), clamps `Limit`, hydrates entities concurrently
  (`fetchEntitiesConcurrent`), re-sorts hydrated entities by `ID`, and fits the response page into `maxPayload`
  (`marshalFittingPrefixPage`).

## Governing ADRs

- **ADR-102 (`docs/adr/102-entity-id-segment-semantics.md`):** Fixes the 6-part entity-ID order as
  `org.platform.system.domain.type.instance` and defines 6-position `*` wildcard patterns as the canonical mechanism for
  selecting a taxonomy (`domain.type`) across `system` values (`TestTaxonomyAcrossSourcesIsPatternNotPrefix`,
  `pkg/types/entity_id_semantics_test.go:64-74`). Preserved unchanged.
- **ADR-104 (`docs/adr/104-unique-platform-authority.md`):** Defines the 2-part deployment authority prefix
  (`org.platform`). Preserved unchanged.
- **ADR-112 (`docs/adr/112-natsclient-contract.md`):** Governs `natsclient.KVStore.KeysByFilter` snapshot completion and
  bounded watcher drain. Preserved unchanged.

## Measured Premises & Invariants

### Measured Premises

- **P1 (`pkg/types/entity_id.go:160-164, 193-256`):** `ValidateEntityIDPattern` requires a non-empty string of `<= 256`
  bytes (`MaxEntityIDBytes`) with exactly 6 dot-separated segments where each segment is either `"*"` or matches
  `[A-Za-z0-9][A-Za-z0-9_-]*`. It rejects `">"` (alphabet violation), 5-part or 7-part strings (arity violation), empty
  segments, and inline wildcards such as `"u*"` (alphabet violation), returning a `*errs.ClassifiedError` with
  `Class: errs.ErrorInvalid` and `Code: types.ErrorCodeEntityIDPatternInvalid`.
- **P2 (`natsclient/kv.go:594-601, 618-696`):** `KVStore.KeysByPrefix(ctx, prefix)` is implemented as
  `kv.KeysByFilter(ctx, prefix+">")`. Calling `c.entityBucket.KeysByFilter(ctx, req.Pattern)` with a 6-part pattern
  validated by `ValidateEntityIDPattern` uses the identical `WatchFiltered` snapshot and drain path in `natsclient` with
  an exact 6-token NATS subject filter instead of a trailing `>` wildcard.
- **P3 (`processor/graph-ingest/query.go:279-286`):** The exact-entity fallback (`c.entityBucket.Get(ctx, req.Prefix)`)
  exists only because `KeysByPrefix` appends `"."` (`prefixDot = req.Prefix + "."`), which causes
  `KeysByPrefix("o.p.s.d.t.i.")` (`"o.p.s.d.t.i.>"`) to miss a 6-part key `"o.p.s.d.t.i"`. By contrast, a 6-part pattern
  passed directly to `KeysByFilter(ctx, req.Pattern)` matches 6-part keys directly on the NATS server, and wildcard
  tokens (`*`) are not valid literal KV keys for `Get`. Therefore, the `Get` fallback must not run when `req.Pattern !=
  ""`.
- **P4 (`processor/graph-ingest/component_test.go:292-307`):** The unit-test mock `matchesNATSFilters` currently handles
  only `filter == ">"`, `strings.HasSuffix(filter, ".>")`, and `key == filter`. Extending `matchesNATSFilters` in
  `component_test.go` to also match 6-position wildcard filters via `semtypes.MatchEntityIDPattern(filter, key)` lets
  unit tests exercise `handleQueryPrefixWithMaxPayload` through `createTestComponentWithMockKV` while integration tests
  in `query_prefix_integration_test.go` exercise real NATS JetStream KV `WatchFiltered`.

### Invariants

- **I1 (Wire compatibility when `Pattern` is empty):** For every `PrefixQueryRequest` with `Pattern == ""`,
  `json.Marshal` omits the `"pattern"` key (`specs/graph-transport-boundary/spec.md:9-10`), preserving existing wire
  encoding on `graph.ingest.query.prefix` (`openspec/specs/graph-transport-boundary/spec.md:13-21, 33-38`).
- **I2 (Mutual exclusion before KV I/O):** For every `PrefixQueryRequest` where `Prefix != "" && Pattern != ""`,
  `handleQueryPrefixWithMaxPayload` fails before any KV bucket call with `errs.ErrorInvalid` and code
  `graph.ErrorCodeInvalidRequest` (`specs/graph-transport-boundary/spec.md:10-11, 23-27`).
- **I3 (6-position pattern validation before KV I/O):** For every `PrefixQueryRequest` where `Pattern != ""` and
  `Prefix == ""`, `handleQueryPrefixWithMaxPayload` validates `Pattern` via `semtypes.ValidateEntityIDPattern` before
  any KV bucket call and rejects any non-6-part, `>`, over-256-byte, or malformed-segment pattern with
  `errs.ErrorInvalid` and code `types.ErrorCodeEntityIDPatternInvalid`
  (`specs/graph-transport-boundary/spec.md:11-13, 29-34`).
- **I4 (Cross-system matching and lexicographic cursor pagination):** For every valid `Pattern`, matching keys from
  `KeysByFilter(ctx, req.Pattern)` are sorted lexicographically before cursor advancement and after concurrent entity
  hydration, so paging to exhaustion via `NextCursor` yields the exact sorted set of matching entities with no gaps or
  duplicates (`specs/graph-transport-boundary/spec.md:13-16, 17-21, 36-40`).
- **I5 (Zero-match empty page without exact-key `Get` fallback):** For every valid `Pattern` that matches zero keys in
  `ENTITY_STATES`, `handleQueryPrefixWithMaxPayload` returns `{"entities":[]}` without calling `entityBucket.Get`
  (`specs/graph-transport-boundary/spec.md:13-14, 42-46`).

## Options Considered

### Option A: Optional `Pattern` field on `graph.PrefixQueryRequest` (Recommended)

- Add ``Pattern string `json:"pattern,omitempty"` `` to `graph.PrefixQueryRequest` (`graph/query_prefix_types.go`),
  mutually exclusive with `Prefix`.
- Keep the existing `graph.ingest.query.prefix` subject and `graph.QueryVerbs()` table unchanged.
- In `handleQueryPrefixWithMaxPayload` (`processor/graph-ingest/query.go`):
  1. If `req.Prefix != "" && req.Pattern != ""`, return `errs.ClassifiedCode(errs.ErrorInvalid,
     graph.ErrorCodeInvalidRequest, errors.New("invalid request: prefix and pattern are mutually exclusive"))` before
     any KV I/O.
  2. If `req.Pattern != ""`, validate via `semtypes.ValidateEntityIDPattern(req.Pattern)` before any KV I/O (returning
     the classified error directly on failure) and list keys via `c.entityBucket.KeysByFilter(ctx, req.Pattern)`.
  3. Otherwise (when `req.Pattern == ""`), run the existing `req.Prefix` validation, `KeysByPrefix(ctx, prefixDot)`, and
     6-part exact-key `Get` fallback unchanged.
- **Concrete cost:** One optional field on `graph.PrefixQueryRequest` and a 15-line branch in
  `handleQueryPrefixWithMaxPayload`.

### Option B: Separate `graph.ingest.query.pattern` request/reply verb

- Add a new verb `pattern` (`graph.ingest.query.pattern`) to `graph.QueryVerbs()` and register a second handler in
  `processor/graph-ingest/query.go` that shares pagination and byte-fitting logic with `handleQueryPrefixWithMaxPayload`.
- **Rejected because:** As noted in `#259`, both prefix and pattern queries perform key-filtered snapshot pagination
  over `ENTITY_STATES` with identical `Limit`, `Cursor`, and `PrefixQueryResponse` semantics; adding a new reserved
  subject would widen `graph/query_verbs.go` and `internal/harness/contract/reservedsubjects_test.go` with duplicate
  plumbing for no additional capability.

### Option C: Do nothing (client-side filtering over deployment prefix pages)

- Leave `graph.PrefixQueryRequest` and `processor/graph-ingest` unchanged; callers such as `semsparql`'s
  `NATSRPCReader` page `Prefix: "<org>.<platform>"` (or `Prefix: ""`) and filter hydrated `EntityState` payloads on the
  client with `types.MatchEntityIDPattern`.
- **Rejected because:** In any multi-system or multi-domain deployment, `graph-ingest` would have to hydrate every
  entity in the deployment from `ENTITY_STATES` and serialize full `EntityState` JSON payloads across NATS request/reply
  pages only for the caller to discard non-matching `<domain>.<type>` entities client-side.

## Target Decisions and Proving Tests

### D1: `PrefixQueryRequest.Pattern` wire field (`graph/query_prefix_types.go`)

- `graph.PrefixQueryRequest` gains:

  ```go
  // Pattern is an optional exact six-position entity-ID pattern in ADR-102
  // canonical order where any position may be "*". Mutually exclusive with
  // Prefix. Example: "acme.ops.*.intel.platform.*".
  Pattern string `json:"pattern,omitempty"`
  ```

- When `Pattern` is empty, `json.Marshal` omits `"pattern"` so existing prefix requests serialize byte-for-byte
  identically.
- **Proving tests:** `TestPrefixQueryRequestPatternJSONRoundTrip` and `TestPrefixQueryRequestMarshalOmitsPatternWhenEmpty`
  in `graph/query_prefix_types_test.go`.

### D2: Pre-I/O validation and `KeysByFilter` execution (`processor/graph-ingest/query.go`)

- In `handleQueryPrefixWithMaxPayload`:
  - **Mutual exclusion:** If `req.Prefix != "" && req.Pattern != ""`, return a classified invalid error
    (`errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, ...)`)
    before any KV call.
  - **Pattern validation:** If `req.Pattern != ""`, call `semtypes.ValidateEntityIDPattern(req.Pattern)`; if non-nil,
    return `err` directly (which is already classified as `errs.ErrorInvalid` with code
    `semtypes.ErrorCodeEntityIDPatternInvalid`) before any KV call.
  - **Key listing:**
    - When `req.Pattern != ""`, call `keys, err = c.entityBucket.KeysByFilter(ctx, req.Pattern)`. If `err != nil`,
      return `errs.Classified(errs.ErrorTransient, fmt.Errorf("failed to get keys: %w", err))`. Do not call
      `c.entityBucket.Get`.
    - When `req.Pattern == ""`, run the existing `req.Prefix` path (`KeysByPrefix(ctx, prefixDot)` + exact-match `Get`
      when `req.Prefix != "" && len(keys) == 0`).
  - **Downstream pipeline:** Sorting (`sort.Strings(keys)`), cursor decoding and advancing (`graph.DecodeCursor`), page
    slicing (`limit`), concurrent entity hydration (`c.fetchEntitiesConcurrent`), ID re-sorting, and byte-budget fitting
    (`marshalFittingPrefixPage`) execute identically for both `Prefix` and `Pattern`.
- **Proving tests:**
  - `TestQueryPrefix_PatternMatchesAcrossSystems` (`processor/graph-ingest/query_prefix_unit_test.go`)
  - `TestQueryPrefix_PatternAndPrefixMutuallyExclusive` (`processor/graph-ingest/query_prefix_unit_test.go`)
  - `TestQueryPrefix_PatternInvalid` (`processor/graph-ingest/query_prefix_unit_test.go`)
  - `TestQueryPrefix_PatternCursorPaging` (`processor/graph-ingest/query_prefix_unit_test.go`)
  - `TestQueryPrefix_PatternNoMatches` (`processor/graph-ingest/query_prefix_unit_test.go`)
  - `TestIntegration_PrefixQuery_PatternAcrossSystems` (`processor/graph-ingest/query_prefix_integration_test.go`)

## Merge Order

- No file overlap with open PR `#261` / `#260` (`vocabulary` and `vocabulary/export`; `docs/admission-ledger.yaml` rows
  are hundreds of lines apart: `vocabulary` at line 453 and `vocabulary/export` at line 2306 vs `graph` at line 1902 and
  `processor/graph-ingest` at line 2143) or PR `#255` (`pkg/errs`, `natsclient/errors*.go`, `pkg/lifecycle`,
  `pkg/projection`, `processor/graph-ingest/commit_ambiguity_integration_test.go`). Can merge in any order relative to
  `#255` and `#261` (merging `origin/main` into the second PR's branch before final merge).
