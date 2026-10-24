# Inventory: fix-384-discovery-default

Base: main c770283 (claim commit 9896fd1). Issue #384, claim PR #394. Not a porting change: `config` is already
ported (ledger row adapt), so no lifecycle-suite or test-repeat run on a pin copy applies here. Every pin fact below is
read with `gh api ...?ref=8b99efe9`.

## 1. The claimed gap

`config/config.go:491` — `"discovery": types.ServiceConfig{` (`Enabled: false`, `Config: {}`, `:491-494`), beside
`heartbeat` at `:495-498`. `getDefaults` is read by `LoadFile` (`:387`) and `LoadFromBytes` (`:404`), so every loaded
`Config.Services` holds the key `discovery`. `service.RegisterAll` (`service/register.go:20-27`) registers metrics,
component-manager, message-logger, storage-observability, heartbeat and metrics-forwarder, and no discovery.

Readers of the name `discovery`, and whether each depends on the default:

| Site | Depends on the default? |
| --- | --- |
| `config/config_test.go:57,80-82` (`TestLoader_LoadJSON`) | No. The document it writes sets `"discovery": {"enabled": true}`; the name is arbitrary. |
| `config/config_test.go:114-116` (`TestLoader_Defaults`) — `assert.False(t, discoveryEnabled) // dormant by default` | Reads the default, but cannot tell it apart from no entry: `getServiceEnabled` (`:17-19`) on a missing key returns false. The assertion passes on the base and after the fix; it cannot detect the phantom. |
| `config/config_test.go:250-252` (`TestLoader_ExampleConfig`) | No. It reads `example_config.json`'s own entry. |
| `config/example_config.json:13` — `"discovery": {"enabled": true, "config": {}}` | No. But it enables a service no constructor builds; composing it would fail at `service/service_manager.go:219` (`no constructor registered for service %s`). No test composes the example's services: `composition/example_config_test.go:20-36` validates components only. |
| `config/manager_test.go:46` (subject-match table), `:208,266-277` (`TestConfigManager_PushToKVWritesEverySection`) | No. It builds its own `Config` literal. |
| `config/manager_integration_test.go:145,168` (`TestChannelSubscriptions`), `:205` (`TestConcurrentKVUpdates`) | No. The suite's base config has `Services: make(types.ServiceConfigs)` (`:42`); the name is a KV key it writes itself. |
| `config/README.md:48` — "two services, `discovery` (disabled) and `heartbeat` ..." | States the default; becomes false. |
| `types/service.go:47` — `// The map key is the service name (e.g., "metrics", "discovery").` | Doc example only. Its `:48-50` "registered a constructor via init()" is also false (`service/register.go:19`, the `RegisterAll` rule). |

Production readers and writers of `Services` that see the entry:

- `config/manager.go:1027` — `for name, svcConfig := range cfg.Services {` writes `services.discovery` to the
  configuration bucket on every push.
- `config/manager.go:742-761` (`applyConfigKey`, `:733`) — `currentConfig.Services[serviceName] = svcConfig` writes a
  bucket entry `services.<name>` into memory, for watcher events (`updateConfig`, `:714`) and for `syncFromKV`.
- `config/manager.go:1501-1502` (`syncFromKV`, `:1470`) — `current.Services = make(types.ServiceConfigs)`. When the
  bucket wins the version check, its `services.*` keys replace the file's map. So after the fix, a bucket that already
  holds `services.discovery` from an earlier boot puts the entry back in memory. The owner's no-migration ruling
  (2026-10-11: deployments wipe and start fresh; no legacy-state readers or migration) covers this, so the change
  writes no cleanup for old buckets.
- `config/manager.go:989` (`nameByKey`): re-keys only; the name has no space, so no effect.
- `service/service_manager.go:167-249` `ConfigureFromServices`: `:214` skips the disabled entry, and `:249` keeps it in
  `bootServiceConfigs`.
- `service/service_manager_http.go:540-575` `handleServiceList`: lists composed services only (`:542-563`). The boot and
  desired copies both carry `discovery`, disabled, so `pendingServiceChanges` (`service/service_configs.go:183`) reports
  nothing for it.
- `composition/`: `git grep -n -w Services -- 'composition/*.go'`, no production hit.

Searches: `git grep -n -i discovery -- config service composition`. Then, outside those three directories and the
archives, `git grep -n -E '"discovery"|services\.discovery|`discovery`' -- ':!config' ':!service' ':!composition'
':!openspec/changes/archive' ':!openspec/changes/fix-384-discovery-default'` finds only `types/service.go:47`. The
service hits `service/message_logger*_test.go:138,382` are subject auto-discovery, not the service name.

## 2. Other spellings and the pin

- Pin probe: `gh api 'repos/C360Studio/semstreams/contents/config/config.go?ref=8b99efe9'` gives `:519-522` the same
  `"discovery"` default (`Enabled: false // Dormant by default`) beside `heartbeat` `:523-526`. The default is carried
  from the pin.
- Pin `service/register.go:10-18` registers metrics, message-logger, log-forwarder, metrics-forwarder,
  component-manager, heartbeat and storage-observability, with no discovery. A pin tree search
  (`git/trees/8b99efe9?recursive=1`, paths matching `discover`) finds `component/discovery.go` (component capability
  discovery, a library and not a service) plus ADR-077 and docs. There is no discovery service.
- Archived inventory: `openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md:334-336` § 5
  (c). It records "The Discovery service ... (no such service)" (`configurable.go:13`) and that the pin's README
  documents a `discovery` service (`README.md:328-345`) that the code does not implement.
- Pin ADRs: all 85 under `docs/adr` fetched at the pin and searched for `getDefaults`, `discovery service`,
  `services.discovery` and `"discovery"`: 0 hits. No ADR governs the default service set.

## 3. Adjacent claims

- Open PRs (`gh pr list --state open`):
  - #394, this claim: three `openspec/` files.
  - Draft #393, the log forwarder: six files under `openspec/changes/setup-04a-03-boot-service-log-forwarder/`. No
    shared file. But its `design.md:25-26` reads "`config`'s defaults do not enable it (`config/config.go:491-498`:
    `discovery` and `heartbeat` only)". If #394 merges first, that citation is stale and #393 updates it when it merges
    `origin/main`. The reverse order needs nothing from #394.
  - #395, docs for `metric/README.md` and the repository map (#313, #86): files `metric/README.md` and
    `docs/repository-map.md`. It shares `docs/repository-map.md`, which this change edits at archive (task 4.2).
    Neither depends on the other; whichever merges second merges `origin/main` and resolves the map rows.
- #340 (config.Manager delayed delete) is about `DeleteComponentFromKV` in `config/manager.go` (the issue cites `:887`
  at `88b6244`). It does not touch `config.go`'s defaults or any line above. No overlap.
- `gh issue list --state open --search config`: 16 hits. Kept as naming the package: #384 (retired by this change) and
  #340 (stays; unrelated lines). #7 and #196 name `config/manager.go` in comments, not the defaults. #9 names `config`
  in the change-3 plan. #195 names `config` in a fixture-adoption count. The others name the word, not the package:
  #198 #362 #27 #280 #134 #84 #281 #25 #350 #109. `--search discovery`: #384, #110 and #9; the last two are
  word-only.
- Specs: `openspec/specs/config-loading/spec.md:12-24` requires the loader to "start from the loader's defaults" and
  pins only the default NATS URL (scenario `:21-24`). No requirement lists the default services. config-manager:
  `git grep -n -i default -- openspec/specs/config-manager` gives 0 hits. `openspec/specs/service-lifecycle/spec.md:838`
  and scenario `:856-861` depend on the `heartbeat` default (tests `TestHeartbeatComposedFromTheConfigDefaults`,
  `TestHeartbeatRefusesAMiscasedKeyBesideTheDefault`), not on `discovery`.
- Ledger: `docs/admission-ledger.yaml:2708` `- source_path: config`, disposition adapt (`:2755`).
  `known_risks` starts at `:2729`. `:2738-2739` "Retained: unknown keys elsewhere accepted, gulf_mexico default region,
  framework streams for writers not yet ported" is the sentence beside which a dropped-default line belongs; today no
  line records the `discovery` default. `types` row `:1926`, disposition adapt, so `types/service.go` may be edited.
- Proposal corrections: (a) Why: the operator listing does not show the entry (§ 1). What it does show is a
  `services.discovery` key in the configuration bucket and the boot service copy. (b) What Changes / Impact: no test
  breaks; `TestLoader_Defaults:114-116` is vacuous and needs a real oracle; add `types/service.go:47-50`; treat
  `example_config.json:13` as an example that enables an unbuildable service, not as "treating it as a default".

## 4. Who it is for

Nothing new: the change removes a default entry and adds no exported symbol, subject, bucket or config field.

## 5. Problem shape

A default that names a phantom (a name no constructor answers to): the shape of `class:phantom-config`. The nearest
existing check is `service/service_manager.go:217-219`, which refuses an enabled name with no constructor. It does
not reach a disabled one. In-tree precedent for asserting a default exactly: `config-loading` scenario `:21-24`
(default URL) and `config/config_test.go:107-111`, which reads the expected value from `getDefaults`.

## 6. Pin decision records and adopters

- ADRs: none govern the default service set (§ 2).
- Consumers. Question: does a starter consumer read `services.discovery` or rely on the default? Scope:
  `docs/inventory-scope.md:15-18`, read-only. Local checkouts grepped with no `git` command:
  `grep -rn -E '"discovery"|services\.discovery'` over `*.go/json/yaml` in semsource, semconnect, semteams and
  semboids gives 0 hits. Remote `gh api search/code` (`discovery`, `language:json`) per repo: semboids 0; the
  semsource, semconnect and semteams hits are evidence files and the subject `discovery.>`
  (`semteams/configs/flow-bootstrap.json:187`, `e2e-flow-bootstrap.json:152`), not a service key. Products that walk
  `Services`: `semboids/cmd/semboids/main.go:432-461`, `semconnect/cmd/cs-graph-backend/main.go:248` and
  `semteams/cmd/semteams/main.go:728` hand the map to `ConfigureFromServices`, which skips the disabled entry;
  `semteams/cmd/semteams/logging.go:80` reads `log-forwarder` only. No consumer reads `discovery`. Remote HEADs at
  search time: semsource e4febc0d2e23, semconnect d0d06e00bf05, semteams c09194dfcc31, semboids 8c03cc53836c.
- Adopter seam. (1) What must a config author know? Today, that `discovery` is in every loaded config but cannot be
  turned on: enabling it fails composition (`:219`). (2) If they do nothing, the entry stays disabled and is skipped;
  the bucket carries `services.discovery`. (3) Where do they find out? A boot error, and only if they enable it.
  (4) What should they have to know? Nothing. An existing bucket keeps its old `services.discovery` key; under the
  owner's no-migration rule, deployments wipe and start fresh.

## 7. Proving test and sensitivity

- On the base, no test fails if `discovery` is removed from or added to the defaults (§ 1: every reader writes its
  own document or reads a missing key as false).
- The independent oracle is the literal set `{heartbeat}`, not `getDefaults` read back, since reading the defaults
  back would also accept the phantom. `LoadFromBytes([]byte(`{"platform":{"org":"acme","id":"p1"}}`))` returns
  `Services` keys `[discovery heartbeat]` on the base (§ 1, `:490-499`).
- Mutation run: `task mutate:check -- -pkg ./config -test <name> -file config/config.go -mutant <file> -expect ...`
  (`Taskfile.yml:73-76`, run by hand), with the mutant putting the `discovery` entry back (equivalently, any extra
  default service).
