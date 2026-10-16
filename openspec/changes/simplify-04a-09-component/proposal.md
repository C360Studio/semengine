# Proposal: simplify-04a-09-component

## Why

Epic #223, PR 7, claimed as draft PR #273 on the owner's word ("PR 7 — component", 2026-10-16). `component` registers
a factory through two methods and two structs that copy each other. Its type is free text that is checked only at
boot. Each component reports its schema twice (#197, ruled). The config check reads only the first JSON value, and
`SafeUnmarshal(raw, nil)` panics. Schemas use two words for each basic type (#189). The admission snapshot is a wrapper
with nothing left to hide, test code ships in production files, and the lifecycle doc cannot be read (#153). The
`drop`-marked exports that the epic line lists are already gone from `main` (inventory § 1).

## What Changes

- `ValidateFactoryConfig`, and so `CreateComponent`, `Declare` and `SafeUnmarshal`, refuses data after the first JSON
  value.
- `SafeUnmarshal` refuses a nil target with an invalid error. Its contract is written in `component/README.md`.
  Owner question 4: whether it also refuses unknown keys.
- Generated schemas use `int`, `float` and `bool` for array items too (owner question 3).
- **BREAKING (Go API):** `Registry.Register(Registration)` replaces `RegisterFactory`, `RegisterWithConfig` and
  `RegistrationConfig`. `Registration.Type` is `types.ComponentType`, checked at registration. The registry stores a
  copy (owner question 1).
- **BREAKING (Go API):** `Snapshots()` returns `[]Declaration`. `declarationSnapshot` and the test-only
  `Registry.declaration` go.
- **BREAKING (Go API):** `Discoverable` loses `ConfigSchema()` (#197, ruled). A component that defines the method
  still compiles.
- `SimpleMockComponent` moves into a test file. `createSimpleMockComponent` and `attack_test.go` are deleted.
- The `LifecycleComponent` doc lists the observable rules in plain words, and the `PlatformMeta` comment drops its false
  reason.
- Owner question 5: `ValidateConfig(map, ConfigSchema)` and `ValidationError` are deleted, because nothing reads them.
- Kept: `Metadata` and `Discoverable.Meta()`, `ListFactories`, the `PlatformMeta` alias.

## Capabilities

- Modified: `component-registration` (four added requirements).

## Impact

- `component/registry.go`, `discovery.go`, `lifecycle.go`, `dependencies.go`, `validation.go`, `schema.go`,
  `schema_tags.go`, `test_helpers.go` (moves into a test file), `attack_test.go` (deleted), their tests,
  `component/README.md` and `doc.go`.
- `processor/graph-ingest/component.go` (`Register`, `ConfigSchema`), `component_test.go`.
- `docs/admission-ledger.yaml` (`component` row: `contract`, `proving_tests`).
- Consumers at cutover: semsource renames 26 registration calls, and semboids changes 2. No consumer calls
  `Snapshots`, `SafeUnmarshal`, `ValidateConfig`, or `ConfigSchema()` through the interface (inventory § 4).
- Closes #153 (owner question 2), #189 and #197. Addresses #223.
