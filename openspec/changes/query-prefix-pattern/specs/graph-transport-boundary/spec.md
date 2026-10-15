# graph-transport-boundary Specification

## ADDED Requirements

### Requirement: Prefix queries support literal prefixes and wildcard patterns

`graph.PrefixQueryRequest` on `graph.ingest.query.prefix` SHALL accept either a 1–6 position literal `Prefix`
(`prefix`), an exact 6-position wildcard `Pattern` (`pattern`, where each position is a canonical literal segment or
`*`), or neither (matching all entities). When `Pattern` is empty, `json.Marshal` SHALL omit `"pattern"` from the JSON
wire representation. Setting both `Prefix` and `Pattern` on the same request SHALL be rejected before any KV operation
with a classified invalid error (`errs.ErrorInvalid`, code `invalid_request`). A non-empty `Pattern` SHALL be validated
before any KV operation via `pkg/types.ValidateEntityIDPattern` (failing with `errs.ErrorInvalid`, code
`entity_id_pattern_invalid` when malformed), SHALL list matching keys via `KVStore.KeysByFilter`, SHALL NOT fall back to
an exact-key `Get` when zero keys match, and SHALL apply the same lexicographic key sorting, cursor continuation, limit
clamping, and `max_payload` byte-fitting as literal prefix queries.

#### Scenario: A six-position wildcard pattern matches entities across systems

- **WHEN** `ENTITY_STATES` holds entities under multiple `system` segments and a caller requests
  `graph.PrefixQueryRequest{Pattern: "o.p.*.intel.platform.*"}` on `graph.ingest.query.prefix`
- **THEN** graph-ingest returns only the entities matching `o.p.*.intel.platform.*` in lexicographic ID order

#### Scenario: Prefix and Pattern are mutually exclusive

- **WHEN** a caller sends a `graph.PrefixQueryRequest` with both `Prefix` and `Pattern` non-empty
- **THEN** graph-ingest rejects the request before any KV read with a classified invalid error (`code:
  "invalid_request"`)

#### Scenario: An invalid pattern is rejected before any KV read

- **WHEN** a caller sends a `graph.PrefixQueryRequest` whose `Pattern` has fewer or more than six positions, contains
  `>`, or contains a malformed segment
- **THEN** graph-ingest rejects the request before any KV read with a classified invalid error (`code:
  "entity_id_pattern_invalid"`)

#### Scenario: A pattern query pages across sorted matches

- **WHEN** the keys matching `Pattern` exceed `Limit` or the `max_payload` byte budget and the caller pages using each
  response's `NextCursor` until `NextCursor` is empty
- **THEN** the concatenated pages contain the exact sorted set of matching entities with no gaps or duplicates

#### Scenario: A pattern with no matches skips exact-key fallback

- **WHEN** a caller sends a valid `Pattern` that matches zero keys in `ENTITY_STATES`
- **THEN** graph-ingest returns an empty `Entities` slice (`{"entities":[]}`) with no `NextCursor` and performs no
  exact-key `Get` call on the bucket
