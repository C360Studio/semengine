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
- `SafeUnmarshal` refuses a nil target with an invalid error, even for empty input, and refuses a key its target does
  not define, naming the key (owner ruling 4). Its contract is written in `component/README.md`.
- Generated schemas use `int`, `float` and `bool` for array items too (owner ruling 3).
- **BREAKING (Go API):** `Registry.Register(Registration)` replaces `RegisterFactory`, `RegisterWithConfig` and
  `RegistrationConfig`. `Registration.Type` is `types.ComponentType`, checked at registration through a new
  `types.ComponentType.Validate()`, which `types.ComponentConfig.Validate` also uses. The registry stores a copy
  (owner ruling 1).
- **BREAKING (Go API):** `Snapshots()` returns `[]Declaration`. `declarationSnapshot` and the test-only
  `Registry.declaration` and `Registry.declarationsSnapshot` go.
- **BREAKING (Go API):** the `component.PlatformMeta` alias is removed; `Dependencies.Platform` is
  `types.PlatformMeta`.
- **BREAKING (Go API):** `Discoverable` loses `ConfigSchema()` (#197, ruled) and `Meta()`, and the `Metadata` type is
  deleted (owner ruling 1). A component's description and version live once, on its registration. A component that
  defines either method still compiles; one whose `Meta` returns `component.Metadata` does not.
- `SimpleMockComponent` moves into a test file. `createSimpleMockComponent` and `attack_test.go` are deleted.
- The `LifecycleComponent` doc lists ADR-108 Decision 3's rules in plain words.
- `ValidateConfig(map, ConfigSchema)` and `ValidationError` are deleted, because nothing reads them (owner ruling 5).
- Kept: `ListFactories`.

## Capabilities

- Modified: `component-registration` (four added requirements).

## Impact

- `component/registry.go`, `discovery.go`, `lifecycle.go`, `dependencies.go`, `validation.go`, `schema.go`,
  `schema_tags.go`, `test_helpers.go` (moves into a test file), `attack_test.go` (deleted), their tests,
  `component/README.md` and `doc.go`.
- `types/component.go`: one new exported method, `ComponentType.Validate` (the root `types` package, not `pkg/types`,
  which #275 owns; no shared file).
- `processor/graph-ingest/component.go` (`Register`, `ConfigSchema`), `component_test.go`.
- `docs/admission-ledger.yaml` (`component` row: `contract`, `proving_tests`).
- Consumers at cutover: semsource renames 26 registration calls and 9 `component.PlatformMeta` uses, and every
  consumer component loses its `Meta` method (semsource 26 lines, semconnect 2), all found by the compiler. No consumer
  calls
  `Snapshots`, `SafeUnmarshal`, `ValidateConfig`, or `ConfigSchema()` through the interface (inventory § 4).
- Closes #153 (owner ruling 2: item 6 goes to #196; the storage doc, `Declaration`'s parallel lists and item 7's
  `pkg/lifecycle` structs go to #280, because PR 9b merged without the structs), #189 and #197. Addresses #223.
