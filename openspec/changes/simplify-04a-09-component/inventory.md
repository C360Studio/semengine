# Inventory: simplify-04a-09-component

base: `0fb79ad27d8fb6b534180a52753d98ce27f974a1` (origin/main `2422d8f` plus the empty claim commit)

Question: what in `component` do #153, #189 and #197 still ask for on `main`, who reads each piece, and what else
claims the territory. Structural readers come from `gopls references` with `GOFLAGS=-tags=integration` (so
integration-tagged files count). Consumers are read for symbol use in their local working trees with `git grep`
(`docs/inventory-scope.md`): semsource `4093d3c`, semconnect `dff1265`; semteams `0f1a20b0` and semboids `37dbdb0` are
counted too, but for them `component` is outside the question the scope table names, so no decision rests on them
alone. Every consumer still imports `github.com/c360studio/semstreams/component`. Pin facts are `git grep` and
`git show` on the commit object `8b99efe9`, not a checkout.

This change carries no package from the pin: `component` is already ported (ledger row
`docs/admission-ledger.yaml:2139-2166`, disposition `adapt`), so no pin probe is owed. `component` has 4,187 non-test
lines and an 80% merged coverage floor (`scripts/cover-check.sh:38` — `"component merged"`).

## 1. The claimed gaps, re-measured

### #153 item 1 and the epic's `drop` list: premise false, already done

The epic line names `ListComponentTypes`, `GetFactory`, `ListAvailable`, `KVReadBucket`, `ConsumerName`,
`ValidateNetworkConfig`, `GetComponentSchema`, `ValidateConfigKey`. None exists on `main`. This search returns
nothing:

```bash
git grep -n -E 'ListComponentTypes|GetFactory|ListAvailable|type Info|KVReadBucket|ValidateNetworkConfig' -- '*.go' '*.md'
git grep -n -E 'GetComponentSchema|ValidateConfigKey|ZeroIsOmitted' -- '*.go' '*.md'
```

(both with `':!openspec/changes/archive'`). `ConsumerName` matches only the field at `component/port_jetstream.go:20`
(`ConsumerName string` with tag `consumer_name,omitempty`), not the dropped method `StreamFacts.ConsumerName`. Every
other `component` row marked `drop` in the archived appendix
(`openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/inventory.md:945-978`) is absent too. One
`git grep` per name, 0 hits each: `ValidateComponentConfig`, `IsLifecycleComponent`, `LogLevel*`, `LogEntry`,
`NewProcessorMetrics`, `MergePortConfig`, `Registerable`, `ValidateJSONSize`, `GetString`, `GetInt`, `GetBool`,
`GetFloat64`, `GetPropertyValue`, `GetProperties`, `IsComplexType`, `SortedPropertyNames`,
`declarationSnapshot.Factory`. They were removed on PR #93's branch before its squash merge (`d77c597`); `git log -S`
on `main` finds no commit that adds them. The ledger says so: `docs/admission-ledger.yaml:2157-2160` — "Dead surface
removed (design D6, #153) …".
adds them. The ledger says so: `docs/admission-ledger.yaml:2157-2160` — "Dead surface removed (design D6, #153) …".

### #153 item 2, four models of one component kind: **three remain**

- `component/registry.go:43` — `type Registration struct {`; its `:45` — `Type string` (untyped; `:251` and `:316` —
  `if registration.Type != string(config.Type) {` is the only check against `types.ComponentType`).
- `component/registry.go:59` — `type RegistrationConfig struct {`, a field-for-field copy; `:551` —
  `func (r *Registry) RegisterWithConfig(config RegistrationConfig) error {` builds a `Registration` and calls
  `:565` — `return r.RegisterFactory(config.Name, registration)`.
- `component/registry.go:145` — `func (r *Registry) RegisterFactory(name string, registration *Registration) error {`
  never compares `name` with `registration.Name`, checks `Type` only for empty (`:155-157`), and stores the caller's
  pointer: `:172` — `r.factories[name] = registration`. CONFIRMED.
- `Info`: gone (above).
- `component/discovery.go:38` — `type Metadata struct {`, returned by `Discoverable.Meta()` (`:19`). It is the running
  instance's self-report, not the registration: graph-ingest reads it in production for its consumer name,
  `processor/graph-ingest/component.go:1547` — `natsclient.PortConsumerContext{Component: c.Meta().Name, …}`, and its
  values already differ from the registration's (`component.go:819` — `Description: "Entity and triple ingestion
  processor",` against `:836` — `Description: "Entity and triple ingestion processor for graph system",`).

### #153 item 3, the snapshot wrapper: CONFIRMED

`component/registry.go:98` — `type declarationSnapshot struct {` with accessors `:102-123` that clone `Declaration`'s
fields; `:698` — `//revive:disable:unexported-return …`; `:699` —
`func (r *Registry) Snapshots() []declarationSnapshot {`.
Readers: tests only (`registry_boot_admission_test.go:35`, `:42`, `:147`, `:162`). At the pin its readers are the
change 3 services: `service/component_manager.go:397-401` (reads `snapshot.Declaration()`) and
`service/message_logger.go:347-349` (reads `Name()`, `Inputs()`, `Outputs()`, `InputDeclarationFacts()`,
`OutputDeclarationFacts()`); each maps to a `Declaration` field.

### #153 item 4, `LifecycleComponent` prose: CONFIRMED

`component/lifecycle.go:43-62` ("terminal admission fence", "admitted callback authority", "invent replacement
authority"). ADR-108 Decision 3 (e) (`docs/adr/108-processor-shell.md:83`) binds the rewrite to the plain rules.

### #153 item 5, tests that cannot fail and a mock shipped as API: CONFIRMED

- `component/attack_test.go:1` — `// Code generated by Reviewer Agent. DO NOT EDIT.`; seven tests (`:17`, `:29`, `:54`,
  `:66`, `:81`, `:94`, `:108`) drive test-local values only. The ledger names the file as a proving test
  (`docs/admission-ledger.yaml:2164`).
- `component/test_helpers.go:25` — `type SimpleMockComponent struct {` in a non-test file; readers are
  `registry_test.go:57`, `:131`, `:166`, `:231` only. `component/test_helpers.go:8` —
  `func createSimpleMockComponent(…)` has **no reader at all** (`gopls references`: empty).

### #153 items 6 and 7: mostly outside `component`

- Item 6 is `internal/lifecycleguard` (`guard.go:43` vs `:87` nil-ctx classes; `:113-114` untested refusal), another
  package. Still present.
- Item 7: `component/dependencies.go:16-18` — `// Type alias to avoid import cycles …` / `type PlatformMeta =
  types.PlatformMeta` (in `component`; the stated reason is false: `types` imports nothing from `component`);
  `Declaration`'s parallel `InputPorts`/`InputFacts` slices (`registry.go:80-83`; `Port.Facts()` exists at
  `port_resolver.go:168`); `pkg/lifecycle/manager.go:310` — `type createOutcome struct {` and `:533` —
  `type transitionOutcome struct {` (PR 9b's package); `storage/doc.go:82`, `:114-115` (`objectstore`, not in the
  tree; the `storage` package).

### #197: CONFIRMED, ruled

`component/discovery.go:28` — `ConfigSchema() ConfigSchema`. Its one caller through any receiver here is a test,
`processor/graph-ingest/component_test.go:603` — `schema := comp.ConfigSchema()`. Definers: graph-ingest
`component.go:860`, `test_helpers.go:57`, `registry_integration_test.go:120`, `registry_test_helpers_test.go:19`. Pin:
no non-test call (`git grep -n 'ConfigSchema()' 8b99efe9 -- '*.go' ':!*_test.go'`: declarations only).

### #189: all three CONFIRMED

1. `component/validation.go:46-51` decode one value and stop; nothing reads past it. `{} {"s":"\u0007"}` passes.
2. `component/validation.go:179-180` — `targetType := reflect.TypeOf(target)` / `if targetType.Kind() != reflect.Ptr {`:
   `Kind()` on the nil `reflect.Type` that `TypeOf(nil)` returns panics. Owner ruling (#189 comment, 2026-10-12): keep
   `SafeUnmarshal` and fix it; nine pin production callers (one, `output/websocket`, admitted at tier 0). No consumer
   calls it (0 in all four).
3. `component/schema_tags.go:590` — `return &PropertySchema{Type: "integer"}`, `:592` — `"number"`, `:594` —
   `"boolean"` (array items) against `:318` — `"string", "int", "bool", "float",` (`isValidType`) and `:625-629`
   (properties), and the field doc `component/discovery.go:53` — `// "string", "int", "bool", "float", …`.

## 2. Every spelling of the facts being modelled

| Fact | Spellings |
| --- | --- |
| A factory's identity, type and schema | `Registration` (`registry.go:43`), `RegistrationConfig` (`:59`), the `name` argument of `RegisterFactory` (`:145`); `Metadata` from `Meta()` (`discovery.go:38`) repeats name, type, description and version per instance |
| A component's configuration schema | `Registration.Schema` (`registry.go:50`); `Discoverable.ConfigSchema()` (`discovery.go:28`) |
| Valid component types | `types/component.go:122` (`ComponentConfig.Validate`); `Registration.Type` is unchecked beyond non-empty |
| An admitted declaration | `Declaration` (`registry.go:76`); `declarationSnapshot` (`:98`); unexported `Registry.declaration` (`:675`, readers: tests only, six sites) |
| A schema's basic type words | tag words `int`/`float`/`bool` (`schema_tags.go:318`, `:625-629`, `schema.go:89`, `:135-155`); JSON Schema words `integer`/`number`/`boolean` (`schema_tags.go:590-594`) |
| "No data after the first JSON value" | already refused in `component/port_codec.go:221`, `processor/graph-ingest/component.go:612`, `types/component.go:90`, `types/service.go:31`, `vocabulary/pack.go:148`, `pkg/lifecycle/manager.go:1039`; not in `ValidateConfig` |

## 3. Adjacent claims

- Specs: `openspec/specs/component-registration/spec.md` (per-package `Register`, the import fence, unknown keys
  refused) names no registry method, `Snapshots`, `SafeUnmarshal` or schema words. `lifecycle-suite` and
  `harness-boundaries` name none of the changed symbols (`git grep` for each: only `PlatformMeta` in
  `harness-boundaries/spec.md:538-575`, which governs `types.PlatformMeta`'s fields and `component.Dependencies.Platform`,
  not the alias).
- Rules: `AGENTS.md:83` and `.agents/rules-index.md:29` — `CreateComponent`, `SealComposition`, `Snapshots` only via
  `service.ComponentManager`; unchanged by this change.
- ADRs here: ADR-108 Decision 3 (e) (`docs/adr/108-processor-shell.md:83`), the plain `LifecycleComponent` doc.
- Ledger: `docs/admission-ledger.yaml:2139-2166`; `contract` lists three adaptations; `proving_tests` names
  `attack_test.go`.
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`, 2026-10-16):
  - #273 (this claim, draft, no files yet).
  - #272 (draft, PR 9b `pkg/lifecycle`, change `simplify-04a-08-lifecycle`): five `openspec/` files now; its task 4.1
    edits `docs/admission-ledger.yaml` (`pkg/lifecycle` row), a shared file. No shared code file. Its inventory
    (`:154-155`) files #153 under "names the word, not the package", although #153 item 7 names
    `pkg/lifecycle/manager.go:310` and `:533`.
  - #242–#248 (Dependabot: `go.mod`/`go.sum`, `package*.json`, `.github/workflows/ci.yml`): no overlap.
- Unclaimed wave PRs on neighbours: PR 10 (`processor/graph-ingest`: #139, #174, #183), whose files this change edits
  (`component.go:814-824`, `:858-862`, `component_test.go:603`, `:757`); PR 3 (`types`, #219), not edited here.

### Open issues naming the package

Query: `gh issue list --state open --search "component" --limit 200` (40 hits), each read.

| Issue | Names `component` by | What it asks |
| --- | --- | --- |
| #153 | title, body | items 1–7 (§ 1) |
| #189 | title, body | three defects (§ 1) |
| #197 | title, body | drop `ConfigSchema()` (ruled 2026-10-12) |
| #29 | `component/dependencies.go:7,47-57,76` | drop `Dependencies.ToolRegistry`: **already done** (`git grep ToolRegistry -- component`: nothing; ledger `:2148-2149` "(1) … dropped (#29)") but still open |
| #109 | `component/lifecycle.go:55-62`, `component.ProcessorMetrics` | the processor shell, which ADR-108 lands in change 4 |
| #139 | `component.DebugStatusProvider` | graph-ingest's `DebugStatus` has no reader (PR 10) |
| #34 | `component.NATSPort` | `output/websocket`'s input port (its port) |

Names the word, not the package: #7, #9, #90 (`pkg/security` gains `component` as a caller), #17, #27, #30,
and #33, #35, #36, #38, #68, #95, #96, #103, #105, #106, #108, #110, #111, #114, #131, #133–#137, #141, #161, #183,
and #195, #196 (`internal/lifecycleguard`), #198 and #223.

## 4. Readers of the surface the change touches

| Symbol | Here (prod / test) | Consumers, production lines | Pin, production |
| --- | --- | --- | --- |
| `RegisterWithConfig` / `RegistrationConfig` | 0 / 10 | semsource 26 / 26 | 33 calls |
| `RegisterFactory` / `Registration` literal | graph-ingest `component.go:814` / 9 | semboids 2 (`internal/sim/factory.go:16-27`, `componentregistry/register.go:74`) | 13 calls |
| `ListFactories` (kept) | 0 / 8 | semsource 1, semteams 2, semboids 2 | — |
| `Discoverable.ConfigSchema` | definer 1, caller 0 | definers: semsource 13, semconnect 1, semboids 1; callers through the interface 0 (semboids `internal/sim/factory.go:23` calls it on `*Component`) | 0 calls |
| `Snapshots` | 0 / 4 | 0 | 2 (change 3's services) |
| `SafeUnmarshal` | 0 / 7 | 0 | 9 |
| `component.PlatformMeta` | `dependencies.go:50` / 18 | semsource 9 | — |
| `ValidateConfig(map, ConfigSchema)` / `ValidationError` (`schema.go:53`, `:17`) | 0 (self-recursion only) / 8 | 0 | 1, `component/config_validator.go:78`, which #93 did not port |

semboids also reads `Registry.GetFactory` in production (`componentregistry/register.go:49`), which #93 dropped; that
gap is the cutover's, not this change's.

## 5. Problem shapes

- Refuse data after the first JSON value: adopt `component/port_codec.go:221` — `if err :=
  decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {`.
- Register-by-value keyed from the struct, stored as a copy: `payloadregistry/registry.go:98` —
  `func (pr *Registry) Register(registration *Registration) error {` keys by the registration's own type and stores
  `:174` — `pr.registrations[msgType] = &stored`.
- Defensive clone on read: `cloneComponentDeclaration` (`registry.go:723`), already used by `declarationsLocked`.

## 6. Pin decision records

None is carried (no port). Search over the 85 records at `8b99efe9` for `ConfigSchema|Discoverable|RegisterWithConfig|
RegistrationConfig|RegisterFactory|SafeUnmarshal|ValidateFactoryConfig|Snapshots|component\.Registration|
component/registry|component/schema|component/validation`: ADR-042 (`:400`, the word "Discoverable", unrelated) and
ADR-100 (`:19`). ADR-100 Decision 2: "A registration declares its ports as a pure function of raw configuration and
instance name, the same way it declares `Schema`", so the registration is the schema's home, as #197 rules. ADR-095
(one-shot running lifecycle) governs the rules the `LifecycleComponent` doc states.

## 7. Findings outside the three issues

- **F-1, dead surface.** `ValidateConfig(map[string]any, ConfigSchema)` and `ValidationError` (`schema.go:17-53`,
  ~200 lines with helpers): no production reader here, in a consumer or at the pin outside the unported
  `config_validator.go`. The transitive drop was missed in #93's appendix. It is also the only in-tree reader of the
  tag words (§ 2).
- **F-2, contract conflict.** `component/validation.go:186` — `// Unmarshal config - ignores unknown fields`.
  `SafeUnmarshal` is offered to factories, and the `component-registration` requirement "Unknown configuration keys
  are refused" binds every ported component. `output/websocket`, admitted at tier 0, calls it at the pin.
- **F-3, test-only unexported code (#186).** `Registry.declaration` (`registry.go:675`), six test readers;
  `createSimpleMockComponent` (`test_helpers.go:8`), none.
- **F-4, open but done.** #29 (§ 3).

## Adopter seams (existing surface)

- **Registration.** A product author must know two methods and two structs exist for one job, that the method's
  `name` must match the struct's `Name`, and that `Type` is free text until the first `CreateComponent`. Doing
  nothing: a misspelled type registers and fails every boot later; a name mismatch registers under the argument.
  Finding out: boot error, far from the line at fault. Should know: one call, refused at registration.
- **`ConfigSchema()`.** Every component author writes the schema twice; nothing compares the copies. Should know:
  nothing; the registration carries it.
- **Config check.** A factory using `json.NewDecoder` without a trailing check runs on half a hand-edited file;
  nobody finds out. `SafeUnmarshal(raw, nil)` crashes the process; unknown keys pass silently (F-2).
- **Schema words.** A schema reader must handle two words per type; nothing tells it so.
