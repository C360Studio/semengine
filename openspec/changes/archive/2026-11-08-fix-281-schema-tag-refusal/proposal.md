# Proposal: fix-281-schema-tag-refusal

Approved-by: owner, 2026-11-08 in Enjoy, chose the panic option; recorded on #281 (comment 6722207550)

## Why

Issue #281. `component.GenerateConfigSchema` skips a field whose `schema:` tag `ParseSchemaTag` refuses, and tells
no one: the field is missing from the schema. The same silent skip is in `generateNestedSchema` (nested objects and
array items) and `GenerateCacheFieldSchema` (`component/schema_tags.go`). This breaks the repo-wide rule "Failure
paths fail closed; any skip, drop, or degrade is declared" (`AGENTS.md` § 1).

Measured 2026-11-08 on `1eebea5`: no production schema hits it today. With the skip turned into a panic, the tests of
`component`, `processor/graph-index`, `processor/graph-ingest`, `processor/graph-query` and `storage/objectstore` (every
caller) pass. The only refused tags left in the tree are the three `type:number` tags on
`natsclient/storagereport.StoragePressureThresholds` (`storage_pressure.go:82`, `:86`, `:91`), which no schema is built
from yet. MEDIUM by #185: no current trigger.

Owner choice (2026-11-08, in Enjoy): panic, naming the field and the tag. Every caller builds its schema once, as a
package-level variable from constant struct tags, so a bad tag is a programming error, as a bad pattern is to
`regexp.MustCompile`; it now fails every test of the package that declares it. A consumer with a bad tag breaks at
upgrade: a migration note, not an owner question.

Overlap: no open PR touches `component/schema_tags*.go` or `natsclient/storagereport` (checked 2026-11-08).

Quick track: proposal and tasks. No spec delta: the code is brought in line with the existing `AGENTS.md` § 1 rule;
the component-registration requirement "A configuration schema uses one word for each basic type" is unchanged. Codex
reviews the code.

## What Changes

- `GenerateConfigSchema`, `generateNestedSchema` and `GenerateCacheFieldSchema` panic on a refused `schema:` tag, with
  the struct type, the field name, the tag and the parse error in the message. The doc comment's "Invalid schema tags
  result in skipped fields (graceful degradation)" says so instead.
- The three `storagereport` tags use `type:float`, the word the component-registration spec names for floating-point
  values.
- A unit test in `component` generates a schema for a struct with a refused tag at the top level, in a nested object,
  and in an array's item struct, and expects a panic whose message names the field and the tag. On the base commit it
  fails: no panic, the field is missing. A test in `natsclient/storagereport` generates the schema of
  `StoragePressureThresholds` and expects all three headroom fields, typed `float`; it fails on the base commit.
- Examples suffice, no generated checks (`docs/testing.md`): one law (a refused tag panics) over three named entry
  sites, each covered by its own example.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None.
