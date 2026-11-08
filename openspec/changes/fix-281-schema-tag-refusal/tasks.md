# Tasks: fix-281-schema-tag-refusal

Quick track: proposal and tasks, with no design document and no spec delta, because no requirement changes. Codex
reviews the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [x] 1.1 Proposal and tasks committed (`e688450`); owner approval (2026-11-08, #281 comment 6722207550). Gate:
  `task spec:check` and `task docs:check`.

## 2. Code

- [x] 2.1 `TestGenerateConfigSchemaPanicsOnRefusedTag` in `component/schema_tags_refusal_test.go` and
  `TestStoragePressureThresholdsSchemaTypesHeadroomFloat` in `natsclient/storagereport/storage_pressure_schema_test.go`
  (`2fae3e0`). On the base commit `GenerateConfigSchema` returns without panicking at the top level, in a nested object
  and in an array's item struct, and the schema of `StoragePressureThresholds` has no `warning_headroom`,
  `high_headroom` or `critical_headroom`. Gate: each fails on the base commit.
- [x] 2.2 The three skip sites panic through `mustParseSchemaTag`, naming the struct type, the field, the tag and the
  parse error; the doc comments say so; the three `storagereport` tags use `type:float`; the nested-schema test's
  `type:nonsense` field, which relied on the skip, is removed (`964e6b7`). `task mutate:check` detects putting back the
  skip in `GenerateConfigSchema` (subtest `top_level`) and in `generateNestedSchema` (subtests `nested_object` and
  `array_item_struct`). Putting it back in `GenerateCacheFieldSchema` survives: that function reads only
  `cache.Config`, whose tags are all accepted, so no test input reaches the refusal without a back door; a refused tag
  added to `cache.Config` would still fail `TestGenerateConfigSchemaExpandsNestedStructsAndArrays`, which expects
  every cache field. Gate: the tests in 2.1 pass; `task verify` passed on a clean tree at `964e6b7`.

## 3. Code review

- [ ] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open. The PR body carries
  `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive --skip-specs`). Gate: `task spec:check` and
  `task docs:check`.
