# Inventory: query-prefix-pattern

base: 7e2724a4f9e2424c74b34ae47c605df9c72be257

## Problem Statement

Issue `C360Studio/semengine#259` (`Addresses #9`, unblocking `C360Studio/semsparql#1` §2.2.3 and Task 4.1) identifies a
query-surface gap on `graph.ingest.query.prefix`:

1. Under ADR-102 (`org.platform.system.domain.type.instance`), `domain` and `type` sit at positions 4 and 5, after
   `system` at position 3.
2. `graph.PrefixQueryRequest` (`graph/query_prefix_types.go:45-58`) only carries `Prefix string`, which
   `handleQueryPrefixWithMaxPayload` (`processor/graph-ingest/query.go:250-254`) validates via
   `semtypes.ValidateEntityIDPrefix` (`pkg/types/entity_id.go:189-191`) with `allowWildcard=false` and lists via
   `c.entityBucket.KeysByPrefix(ctx, prefixDot)` (`processor/graph-ingest/query.go:272`).
3. Because any `*` segment is rejected on `Prefix`, a caller resolving virtual `rdf:type` / `rdfs:subClassOf*` patterns
   across systems (`<org>.<platform>.*.<domain>.<type>.*` or `*.*.*.<domain>.<type>.*` via
   `vocabulary.LookupEntityTypesByClassIRI`) cannot select entities by `<domain>.<type>` across all `system` values on
   the server and would otherwise have to page the entire deployment or bucket over NATS and filter client-side.

## Surface Inventory

### 1. The claimed gap

Commands run:

```bash
git grep -n "PrefixQueryRequest\|handleQueryPrefix\|KeysByFilter\|ValidateEntityIDPattern" graph/ processor/graph-ingest/ pkg/types/ natsclient/
gopls references graph/query_prefix_types.go:45:6
gopls references pkg/types/entity_id.go:160:6
gopls references natsclient/kv.go:618:22
```

- `graph/query_prefix_types.go:45` — `type PrefixQueryRequest struct {`
- `graph/query_prefix_types.go:49` — ``Prefix string `json:"prefix"` ``
- `graph/query_prefix_types.go:53` — ``Limit int `json:"limit,omitempty"` ``
- `graph/query_prefix_types.go:57` — ``Cursor string `json:"cursor,omitempty"` ``
- `processor/graph-ingest/query.go:246` — `var req graph.PrefixQueryRequest`
- `processor/graph-ingest/query.go:250` — `if req.Prefix != "" {`
- `processor/graph-ingest/query.go:251` — `if err := semtypes.ValidateEntityIDPrefix(req.Prefix); err != nil {`
- `processor/graph-ingest/query.go:266` — `prefixDot := req.Prefix`
- `processor/graph-ingest/query.go:267` — `if req.Prefix != "" {`
- `processor/graph-ingest/query.go:268` — `prefixDot = req.Prefix + "."`
- `processor/graph-ingest/query.go:272` — `keys, err := c.entityBucket.KeysByPrefix(ctx, prefixDot)`
- `processor/graph-ingest/query.go:279` — `if req.Prefix != "" && len(keys) == 0 {`
- `processor/graph-ingest/query.go:280` — `if _, getErr := c.entityBucket.Get(ctx, req.Prefix); getErr == nil {`

`PrefixQueryRequest` has no wildcard pattern field, and `handleQueryPrefixWithMaxPayload` has no branch calling
`semtypes.ValidateEntityIDPattern` or `c.entityBucket.KeysByFilter`.

### 2. Every current spelling of the facts being modeled

Commands run:

```bash
git grep -n "ValidateEntityID" pkg/types/ processor/graph-ingest/
git grep -n "KeysByPrefix\|KeysByFilter\|ValidateKVWildcardFilter" natsclient/ processor/graph-ingest/
git grep -n "ErrorCodeInvalidRequest\|Classified" processor/graph-ingest/query.go
```

- **Request / response envelope and verb declaration:**
  - `graph/query_verbs.go:37` — `Name: "prefix", Subject: "graph.ingest.query.prefix", Responder: responderGraphIngest,`
  - `graph/query_verbs.go:38` — `RequestType: "graph.PrefixQueryRequest", ReplyType: "graph.PrefixQueryResponse",`
  - `graph/query_prefix_types.go:45` — `type PrefixQueryRequest struct {`
  - `graph/query_prefix_types.go:69` — `type PrefixQueryResponse struct {`
  - `graph/query_prefix_types.go:78` — `func EncodeCursor(key string) string {`
  - `graph/query_prefix_types.go:84` — `func DecodeCursor(cursor string) (string, error) {`
- **Entity-ID prefix vs pattern validation:**
  - `pkg/types/entity_id.go:18` — `ErrorCodeEntityIDPatternInvalid = "entity_id_pattern_invalid"`
  - `pkg/types/entity_id.go:20` — `ErrorCodeEntityIDPrefixInvalid = "entity_id_prefix_invalid"`
  - `pkg/types/entity_id.go:160` — `func ValidateEntityIDPattern(value string) error {`
  - `pkg/types/entity_id.go:161` — `return validateEntityIDValue(`
  - `pkg/types/entity_id.go:162` —
    `value, ErrorCodeEntityIDPatternInvalid, canonicalEntityIDParts, canonicalEntityIDParts, true,`
  - `pkg/types/entity_id.go:169` — `func MatchEntityIDPattern(pattern, entityID string) (bool, error) {`
  - `pkg/types/entity_id.go:189` — `func ValidateEntityIDPrefix(value string) error {`
  - `pkg/types/entity_id.go:190` —
    `return validateEntityIDValue(value, ErrorCodeEntityIDPrefixInvalid, 1, canonicalEntityIDParts, false)`
- **KV key listing by prefix vs exact NATS filter:**
  - `natsclient/kv_key_contract.go:152` — `func ValidateKVWildcardFilter(filter string) error {`
  - `natsclient/kv.go:594` — `func (kv *KVStore) KeysByPrefix(ctx context.Context, prefix string) ([]string, error) {`
  - `natsclient/kv.go:600` — `return kv.KeysByFilter(ctx, prefix+">")`
  - `natsclient/kv.go:612` — `// KeysByFilter returns all keys matching an exact NATS subject filter. Unlike`
  - `natsclient/kv.go:613` — `// KeysByPrefix, this supports fixed-position wildcards such as`
  - `natsclient/kv.go:614` — `// "*.org.platform.system.domain.type.instance" (ADR-102 canonical order).`
  - `natsclient/kv.go:618` — `func (kv *KVStore) KeysByFilter(ctx context.Context, pattern string) ([]string, error) {`
  - `natsclient/kv.go:625` —
    `watcher, constructErr := kv.bucket.WatchFiltered(ctx, []string{pattern}, jetstream.IgnoreDeletes(), jetstream.MetaOnly())`
    (Note: `KeysByFilter` passes `pattern` directly to `WatchFiltered` without calling `ValidateKVWildcardFilter`; callers
    are responsible for entry validation.)
- **Handler sorting, pagination, hydration, and byte fitting:**
  - `processor/graph-ingest/query.go:247` — `if err := json.Unmarshal(data, &req); err != nil {`
  - `processor/graph-ingest/query.go:248` —
    `return nil, errs.Classified(errs.ErrorInvalid, fmt.Errorf("invalid request: %w", err))`
  - `processor/graph-ingest/query.go:290` — `sort.Strings(keys)`
  - `processor/graph-ingest/query.go:293` — `if req.Cursor != "" {`
  - `processor/graph-ingest/query.go:308` — `pageKeys := keys`
  - `processor/graph-ingest/query.go:318` —
    `entities, _, err := c.fetchEntitiesConcurrent(ctx, pageKeys, defaultMaxConcurrent)`
  - `processor/graph-ingest/query.go:328` —
    `sort.Slice(entities, func(i, j int) bool { return entities[i].ID < entities[j].ID })`
  - `processor/graph-ingest/query.go:338` — `return marshalFittingPrefixPage(entities, followingBoundary, maxPayload)`
- **Mock KV filter matching in unit tests:**
  - `processor/graph-ingest/component_test.go:206` —
    `func (m *mockKVBucket) WatchFiltered(...) (jetstream.KeyWatcher, error) {`
  - `processor/graph-ingest/component_test.go:209` — `lister, err := m.ListKeysFiltered(ctx, keys...)`
  - `processor/graph-ingest/component_test.go:259` —
    `func (m *mockKVBucket) ListKeysFiltered(ctx context.Context, filters ...string) (jetstream.KeyLister, error) {`
  - `processor/graph-ingest/component_test.go:292` — `func matchesNATSFilters(key string, filters []string) bool {`
  - `processor/graph-ingest/component_test.go:294` — `if filter == ">" {`
  - `processor/graph-ingest/component_test.go:297` — `if strings.HasSuffix(filter, ".>") {`
  - `processor/graph-ingest/component_test.go:302` — `} else if key == filter {`
    (Note: `matchesNATSFilters` in `component_test.go` currently supports only `">"`, `".>"` suffix matching, and exact
    `key == filter` equality; it does not match single-token `*` wildcards.)

### 3. Adjacent claims on the territory

Commands run:

```bash
task spec:queue
gh pr list -R C360Studio/semengine --state open --json number,title,headRefName,changedFiles,files
gh issue list -R C360Studio/semengine --state open --search "prefix" --limit 200 --json number,title
gh issue list -R C360Studio/semengine --state open --search "graph-ingest" --limit 200 --json number,title
```

- **Current specs:**
  - `openspec/specs/graph-transport-boundary/spec.md:13` — `### Requirement: Reserved request subjects have one declaration`
  - `openspec/specs/graph-transport-boundary/spec.md:15` —
    ``The request/reply verbs graph-ingest serves (`graph.ingest.query.entity`, `.batch`, `.prefix`) SHALL be declared once,``
  - `openspec/specs/graph-transport-boundary/spec.md:33` — `### Requirement: The responder serves exactly its declared verbs`
  - `openspec/specs/graph-transport-boundary/spec.md:71` — `### Requirement: The graph root imports no transport`
  - `openspec/specs/graph-transport-boundary/spec.md:83` — `### Requirement: Authority reads are not enveloped`
  - `docs/specs/entity-id-contract.md` — reference entity-ID contract for 6-part literal IDs, 6-part wildcard patterns,
    and 1–6 part literal prefixes.
- **Admission ledger rows:**
  - `docs/admission-ledger.yaml:487` — `- source_path: pkg/types` (`disposition: adapt`)
  - `docs/admission-ledger.yaml:1902` — `- source_path: graph` (`disposition: adapt`)
  - `docs/admission-ledger.yaml:2143` — `- source_path: processor/graph-ingest` (`disposition: repair-before-port`)
- **Active OpenSpec changes:**
  - None on `origin/main` (`task spec:queue` reports `0 active change(s), 0 task(s) on hold`).
- **Open PRs:**
  - `#261` (`gemini/vocab-stage1-1`) and `#260` (`gemini/vocab-export-stage-1-1`) — both touch `vocabulary/`,
    `vocabulary/export/`, `openspec/changes/`, and `docs/admission-ledger.yaml` (`vocabulary` and `vocabulary/export`
    rows); neither touches `graph/` or `processor/graph-ingest/`.
  - `#255` (`claude/simplify-errs-classify`) — touches `pkg/errs/`, `natsclient/errors*.go`, `pkg/lifecycle/`,
    `pkg/projection/`, and `processor/graph-ingest/commit_ambiguity_integration_test.go`; does not touch
    `graph/query_prefix_types.go`, `processor/graph-ingest/query.go`, or `processor/graph-ingest/query_prefix*_test.go`.
  - `#242`–`#248` — Dependabot dependency bumps (`go.mod`, `go.sum`, `package.json`, `.github/workflows/ci.yml`).

### 4. Who it is for

- **Sister repository consumer (`docs/inventory-scope.md`):**
  - `semsparql` (`C360Studio/semsparql#1`, §2.2.3 and Task 4.1): `NATSRPCReader` resolves virtual `rdf:type` /
    `rdfs:subClassOf*` triple patterns (`?s a <ClassIRI>`) across systems using `(domain, type)` pairs from
    `vocabulary.LookupEntityTypesByClassIRI`.
- **Internal callers in `semengine`:**
  - `processor/graph-ingest/query.go:246` (`handleQueryPrefixWithMaxPayload`) decodes `graph.PrefixQueryRequest` and
    answers `graph.ingest.query.prefix`.

### 5. The problem shape and closest existing instances in the tree

- **Closest validation instance in `pkg/types`:**
  - `pkg/types/entity_id_semantics_test.go:64` — `func TestTaxonomyAcrossSourcesIsPatternNotPrefix(t *testing.T) {`
  - `pkg/types/entity_id_semantics_test.go:67` — `require.NoError(t, ValidateEntityIDPattern("acme.dep1.*.git.*.*"))`
  - `pkg/types/entity_id_semantics_test.go:69` — `err := ValidateEntityIDPrefix("acme.dep1.*.git")`
  - `pkg/types/entity_id.go:160` — `func ValidateEntityIDPattern(value string) error {`
- **Closest callers of `ValidateEntityIDPattern` in the tree:**
  - `pkg/lifecycle/workflow.go:165` — `if err := semtypes.ValidateEntityIDPattern(w.EntityIDPattern); err != nil {`
  - `pkg/projection/contract/contract.go:75` —
    `if err := semtypes.ValidateEntityIDPattern(c.EntityPattern); err != nil {`
- **Closest KV wildcard / prefix filter instances in `natsclient`:**
  - `natsclient/kv.go:618` — `func (kv *KVStore) KeysByFilter(ctx context.Context, pattern string) ([]string, error) {`
  - `natsclient/kv_watcher_ownership_test.go:170` —
    `keys, err := (&Client{}).NewKVStore(bucket).KeysByFilter(ctx, "diag.>")`
  - `natsclient/kv_integration_test.go:467` — `func TestKVStore_KeysByPrefix(t *testing.T) {`
  - `natsclient/kv_integration_test.go:495` — `keys, err := kvStore.KeysByPrefix(ctx, "entity.sensor.")`
- **Closest prefix query handler validation in `processor/graph-ingest`:**
  - `processor/graph-ingest/query.go:250` — `if req.Prefix != "" {`
  - `processor/graph-ingest/query.go:251` — `if err := semtypes.ValidateEntityIDPrefix(req.Prefix); err != nil {`
  - `processor/graph-ingest/query.go:252` — `return nil, err`

### 6. Decision records

- `docs/adr/102-entity-id-segment-semantics.md` — fixes canonical 6-part entity-ID order as
  `org.platform.system.domain.type.instance` (`system` at position 3 before `domain.type` at positions 4–5) and defines
  6-position `*` wildcard patterns for cross-source taxonomy selection.
- `docs/adr/104-unique-platform-authority.md` — defines the deployment authority pair (`org.platform`) at positions 1–2.
- `docs/adr/112-natsclient-contract.md` — governs `natsclient.KVStore.KeysByFilter` filtered watcher lifecycle and
  snapshot completion guarantees.

### 7. Shape sweep and open issues naming `graph` or `processor/graph-ingest`

- **Shape sweep on `graph/query_prefix_types.go` and `processor/graph-ingest/query.go`:**
  - Neither file retains a `context.Context` in a struct, uses `context.WithValue`, registers global metrics, or builds
    `message.GenericJSON`.
  - `graph` root package imports only `encoding/base64` in `graph/query_prefix_types.go:5` (preserving
    `TestGraphImportsNoTransport`).
  - `processor/graph-ingest/query.go:236-339` (`handleQueryPrefixWithMaxPayload`) validates input before any KV I/O
    (`TestHandleQueryPrefix_InvalidPrefixHasNoBucketIO`, `processor/graph-ingest/query_prefix_unit_test.go:20`), sorts
    keys before cursor application (`query.go:290`), re-sorts fetched entities after cache+KV hydration
    (`query.go:328`), and fits pages against `maxPayload` (`query.go:338`).
- **Open issues naming `graph` or `processor/graph-ingest`:**
  - `#259` — `feat(graph-ingest): fixed-position wildcard pattern on graph.ingest.query.prefix (semsparql type scans)`
    (the issue claimed by this change).
  - `#103` — `port-refactor: graph query replies — one envelope carrying indexed_revision, min_revision on requests,
    UnwrapQueryResponse deleted` (Change 4 scope; unrelated to prefix pattern filtering).
  - `#106` — `port-refactor: graph.query.* — a responder-owned verb catalog; a second subscriber on a framework verb is
    refused at Start` (unrelated to `graph.PrefixQueryRequest`).
  - `#111` — `bug: silent degrades on tier-0 paths` (ingest/query paths outside prefix pattern).
  - `#132` — `port-refactor: graph — one vocabulary per failure, one classification per error, one wire spelling, one
    package comment; dead surface named for task 3.3` (unrelated to `graph.PrefixQueryRequest`).
  - `#139` — `port-refactor: graph-ingest — after the write seam: tests through the factory, one read, one outcome
    record, plain functions out of methods, the file's own seams` (unrelated refactor issue).
  - `#174` — `bug: graph-ingest — bookkeeping (cache invalidation, poison clear) is skipped after an unconfirmed append
    write` (append mutation path; unrelated).
  - `#183` — `port-refactor: graph-ingest after #93 — the backlog latch and published_at leave; Health() reads stuck
    states, not an error count (ADR-109)` (readiness/health path; unrelated).
  - `#199` — `test(lifecycle): the graph-ingest test doubles accept stored statements without Source or Timestamp`
    (`pkg/lifecycle` test doubles; unrelated).
  - `#216` — `graph-ingest stops receiving when its input stream is created again while it runs` (JetStream consumer
    lifecycle; unrelated).

## Adopter Seam Inventory

### Adopter Seam: `graph.ingest.query.prefix` (`graph.PrefixQueryRequest`)

1. **What must they know?**
   Today, a caller querying `graph.ingest.query.prefix` (`graph/query_verbs.go:37-39`) must construct a
   `graph.PrefixQueryRequest` (`graph/query_prefix_types.go:45-58`) with a literal 1–6 part `Prefix` (or empty string
   for all entities), optional `Limit`, and opaque `Cursor`. Because `handleQueryPrefixWithMaxPayload`
   (`processor/graph-ingest/query.go:250-254`) validates `Prefix` with `semtypes.ValidateEntityIDPrefix`
   (`pkg/types/entity_id.go:189-191`), any wildcard token (`*`) is rejected as `entity_id_prefix_invalid`. To select
   entities by `<domain>.<type>` (positions 4–5 under ADR-102) when `system` (position 3) is unknown, a caller must know
   that `Prefix` cannot express a cross-system type filter (`TestTaxonomyAcrossSourcesIsPatternNotPrefix`,
   `pkg/types/entity_id_semantics_test.go:64-74`).
2. **What happens if they do nothing?**
   If an adopter sends a wildcard string such as `"acme.ops.*.intel.platform.*"` in `PrefixQueryRequest.Prefix`, the
   responder refuses the request with `errs.ErrorInvalid` (`code: "entity_id_prefix_invalid"`). If the adopter instead
   queries only by the 2-part deployment prefix (`Prefix: "acme.ops"`) or empty prefix (`Prefix: ""`), the server
   hydrates and pages every entity in the deployment or bucket over NATS, forcing the client to discard non-matching
   entities via `types.MatchEntityIDPattern`.
3. **Where do they find out?**
   The request contract is documented on `graph.PrefixQueryRequest` (`graph/query_prefix_types.go:19-58`), in
   `processor/graph-ingest/README.md:101-111`, and in `docs/specs/entity-id-contract.md` (which distinguishes 1–6 part
   literal prefixes from 6-part wildcard patterns).
4. **What SHOULD they have to know, and what is the gap between 1 and 4?**
   An adopter that needs to page entities matching a 6-part entity-ID wildcard pattern across systems should only have
   to supply a canonical 6-part wildcard pattern (validated by `types.ValidateEntityIDPattern`) on the prefix query
   request and use the existing `Limit` / `Cursor` / `PrefixQueryResponse` pagination contract, without fetching
   unrelated entities across the wire. The gap between 1 and 4 is that `graph.PrefixQueryRequest` and
   `handleQueryPrefixWithMaxPayload` currently expose only literal prefix filtering (`KeysByPrefix`) and do not expose
   the 6-part wildcard pattern filtering (`ValidateEntityIDPattern` + `KVStore.KeysByFilter`) that `pkg/types` and
   `natsclient` already implement.

**Prefer observation to prediction:**

- Concrete existing callers in `semengine`: `processor/graph-ingest/query.go:246` (responder) and test callers in
  `processor/graph-ingest/query_prefix_unit_test.go`, `query_prefix_regression_test.go`,
  `query_prefix_integration_test.go`, and `poison_scoping_test.go`.
- Named prospective adopter (`docs/inventory-scope.md` and `C360Studio/semsparql#1` §2.2.3 / Task 4.1): `semsparql`'s
  `NATSRPCReader`, which maps `rdf:type` / `rdfs:subClassOf*` class IRIs via `vocabulary.LookupEntityTypesByClassIRI` to
  `(domain, type)` pairs and needs server-side pattern scans across systems.
