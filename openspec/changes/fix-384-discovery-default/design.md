# Design: fix-384-discovery-default

Base: main c770283. Rests on `inventory.md`. Issue #384, claim PR #394.

## Options

1. **Do nothing.** Every loaded configuration keeps a service name nothing builds, and the bucket gets
   `services.discovery` on every push. Rejected: it is phantom configuration (`class:phantom-config`), and the
   Surface audit drops config that no behaviour reads.
2. **Register a discovery service.** Rejected: none exists at the pin or here (inventory § 2), and nobody needs one
   (§ 6).
3. **Drop the entry from `getDefaults` (`config/config.go:491-494`).** Chosen. One deletion; `heartbeat` stays.

## Decisions

- **D1, spec delta (not `skip_specs`).** `config-loading` gains a requirement naming the default service set exactly:
  `heartbeat`, enabled. Reason: `service-lifecycle` (`:838`, `:856-861`) already relies on `config`'s defaults turning
  the heartbeat on, but no spec says what the defaults are, so a phantom could come back without breaking any spec.
  The wider wording "the defaults name only services the tree can build" was not chosen: proving it needs `config`'s
  tests to import `service` (`RegisterAll`), and the exact set says the same thing more plainly.
  `.openspec.yaml` drops `skip_specs: true`.
- **D2, `config/example_config.json:13`: fixed here** by deleting the line. The example is the file a developer copies
  first (`composition/example_config_test.go:13-14`). It enables the same phantom name, and composing it would fail at
  `service/service_manager.go:219`. Afterwards the example names `message-logger` (registered, `service/register.go:23`)
  and loads `heartbeat` from the defaults.
- **D3, `types/service.go:47-50`: fixed here.** It is a doc comment only. The example names become `"metrics",
  "heartbeat"`, and the false "registered a constructor via init()" becomes "a constructor is registered under that
  name (`service.RegisterAll` registers the framework's)". The `types` row is adapt, so `ledger:check` is unaffected.
- **D4, old buckets.** A bucket written before the fix keeps `services.discovery`, and `syncFromKV` loads it back when
  the bucket wins (inventory § 1). The owner's no-migration ruling (2026-10-11) covers this; no cleanup is written.

## Observable behaviour

- `LoadFile` and `LoadFromBytes` on a document that names no services return `Services` with exactly one key,
  `heartbeat`, enabled. A document's own service entries still merge over the defaults, as today.
- `config.Manager` no longer writes `services.discovery` on a fresh bucket.
- `config/example_config.json` loads with services `heartbeat` and `message-logger`.

## Proving tests

- **New: `TestLoad_DefaultServicesAreHeartbeatOnly`** (`config/load_test.go`). It runs over both `loadPaths` with
  `validDocument` and asserts `slices.Sorted(maps.Keys(cfg.Services))` equals the literal `[]string{"heartbeat"}` and
  that `cfg.Services["heartbeat"].Enabled` is true. The oracle is that hand-written literal, never `getDefaults` read
  back. On the base it fails with `[discovery heartbeat]`.
- **`TestLoader_Defaults`:** delete the vacuous lines `:114` and `:116`. The new test covers them.
- **`TestLoader_ExampleConfig`:** replace `:250` and `:252` (the discovery lines) with an assertion that the service
  keys are the literal `[]string{"heartbeat", "message-logger"}`. On the base it fails, because the file names
  `discovery`.
- **Mutation runs,** each `task mutate:check -- -pkg ./config -test TestLoad_DefaultServicesAreHeartbeatOnly
  -file config/config.go -mutant <copy> -expect load_test.go:<assertion line>`, and each must report detection:
  - M1: the base entry restored.
  - M2: the `heartbeat` default removed. This guards against removing too much.
  - M3: `heartbeat` `Enabled: false`.
- The example file is not a Go source file, so `mutate:check` does not take it. Its proof is the base failure of
  `TestLoader_ExampleConfig`, recorded in `notes/2.1.md`.
- Examples are enough: the default set is one fixed value with no interacting inputs.

## Merge order

No open PR shares a code file. #395 shares `docs/repository-map.md`, which this change edits only at archive
(task 4.2); whichever lands second merges `origin/main` and resolves the map rows. #393's `design.md:25-26` cites
`config/config.go:491-498` as "`discovery` and `heartbeat` only". If this PR lands first, #393 corrects that citation
when it merges `origin/main`. Neither PR waits on the other.
