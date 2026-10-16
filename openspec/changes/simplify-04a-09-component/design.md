# Design: simplify-04a-09-component

Status: draft, revised after review round 1 (of `7f4a02d`) and the owner's rulings of 2026-10-16 on its five
questions (at `2d72c72`). Next: pre-owner design review of this revision. Inventory: `inventory.md` (base `0fb79ad`).

## Context

Epic #223, PR 7, `component` only (#159). The issues are #153, #189 and #197. On #189 the owner ruled to keep
`SafeUnmarshal` and fix it. On #197 the owner ruled on 2026-10-12 to drop `ConfigSchema()` from `Discoverable`. The
owner ruled on this design's five questions on 2026-10-16 (section "Owner rulings"). The epic line's first #153 item,
"remove the remaining `drop`-marked exports", is already done on `main` (inventory § 1), so this change does not repeat
it. No ADR changes. ADR-100 Decision 2 keeps the registration as the home of a factory's schema and ports. ADR-108
Decision 3 sets the rules the `LifecycleComponent` doc states.

## Options for the registration types (#153 item 2)

1. **One `Registry.Register(Registration) error` (ruled).** It keys the factory by `Registration.Name`, checks a typed
   `Type`, and stores a copy. `RegisterFactory`, `RegisterWithConfig` and `RegistrationConfig` go. Cost: at cutover
   semsource renames 26 calls (`RegisterWithConfig(component.RegistrationConfig{…})` to
   `Register(component.Registration{…})`), and the compiler finds each one.
2. **Keep `RegisterFactory(name, *Registration)` only, refusing a `name` that differs from `Registration.Name`.**
   The fix is smaller, but the name is still spelled twice, and semsource changes its 26 calls anyway.
3. **Do nothing.** Two methods and two structs stay for one job, and a misspelled type is still found only at boot.

`Metadata` and `Discoverable.Meta()` go too (owner ruling 1). The owner's condition was "if it's not used". It was
designed for people reading a flow, and at the pin that need is met by the registration, not by `Meta()`.
`GET /components/types` and the `list_components` tool serve `composition.Catalog(registry)`, whose entry is the
registration's name, type, description, version and schema (pin `composition/catalog.go:15-27`,
`service/component_manager_http.go:368-384`). `GET /components` lists instances from configuration and status
(`:330-358`). The only pin reads of `Meta()` are each component naming its own consumer
(`PortConsumerContext{Component: c.Meta().Name}`, about 20 components) and four debug logs of `.Type`
(`service/component_manager.go:697-720`). So the description and version people read live once, on the registration.
`Info` is already gone.

## Decisions

### D1 The config check refuses data after the first JSON value (#189 item 1)

`ValidateFactoryConfig` returns an invalid-class error when anything other than whitespace follows the first value.
So do `CreateComponent`, `Declare` and `SafeUnmarshal`, which call it. The check is the same as
`component/port_codec.go:221`. Proof: `TestValidateFactoryConfigRefusesDataAfterTheFirstValue` (`{} {}`, `{} x`, and
`{}` followed by a newline, which passes), plus a `CreateComponent` row in which the factory is never called. Both fail
on the base commit.

### D2 `SafeUnmarshal` refuses a nil target and unknown keys (#189 item 2)

The nil-target check runs first, before the config check and before the early return for empty input
(`validation.go:173-176`). So `SafeUnmarshal(raw, nil)` and `SafeUnmarshal(nil, nil)` both return an invalid-class
error and do not panic. A typed nil pointer is refused too. Decoding refuses a key the target struct does not define,
with an invalid-class error naming the key, and data after the first value. This is the shape of
`component/port_codec.go:215-228` (`decodeStrict`). `component/README.md` gains the contract line: what
`SafeUnmarshal` checks, in what
order, and that it is offered to component factories (#190). Proof: `TestSafeUnmarshalRefusesANilTarget`, with a row
for valid input (it panics on the base commit) and a row for empty input (it returns nil on the base commit), and
`TestSafeUnmarshalRefusesAnUnknownKey`, which fails on the base commit. At the pin, nine components call it. When
those are ported, a config carrying a key their struct lacks is refused.

### D3 One word for each basic type in a schema (#189 item 3; ruling 3)

Schemas use `int`, `float` and `bool` everywhere: `generateItemSchema` stops emitting `integer`, `number` and
`boolean`. These are the words the field's doc (`discovery.go:53`), the tag parser and every property already use.
Only array items use the other words. Proof: `TestGeneratedSchemaUsesOneWordPerType` generates a schema for a struct
with `int`, `float64`, `bool`, `[]int`, `[]float64`, `[]bool` and `[][]int` fields. It requires exactly `int`, `float`
and `bool` for those types in properties and in item schemas at every depth, and fails on the base commit.

### D4 One registration call (#153 item 2; option 1)

`Registry.Register(Registration) error` refuses each of these with an invalid-class error naming the factory:

- a `Name` that `ValidateComponentName` refuses (such a name can never be created today);
- a nil `Factory` or `Ports`;
- a `Type` outside the five component types;
- a `Name` already registered.

`Registration.Type` becomes `types.ComponentType`, so literals such as `Type: "processor"` still compile. The list of
the five types today is an inline `switch` inside `types.ComponentConfig.Validate` (`types/component.go:121-126`), and
nothing exported offers it. So this change adds one exported method in the root `types` package,
`func (t ComponentType) Validate() error`, which holds that switch. It has two callers: `ComponentConfig.Validate` and
`Registry.Register`. This is new surface in a second package, kept to one method so that the list has one home. The
alternative, `Register` building a throwaway `types.ComponentConfig{Name, Type}` to call `Validate`, adds no surface
but reads as a trick. The root `types` package is not `pkg/types`, which PR 3 (#275) owns, so no file is shared. Either
PR may land first. The second one merges `origin/main` first.

The registry keeps its own copy, made with the same clone `ListFactories` already does (`registry.go:470-482`), moved
into one helper that both use. A later change to the caller's value, its schema or its dependency list changes
nothing, with one exception: a schema default is copied when it is a JSON value (nil, a string, a bool, a number, a
`[]string`, or a `[]any` or `map[string]any` holding these at any depth); a default of any other type is stored as is,
so a pointer, slice or map it holds stays shared. The callers seen write only JSON values, so no reflection-based copy
is added. Proof: `TestRegisterRefusesATypeOutsideTheFive` (`"procesor"`), `TestRegisterKeepsItsOwnCopy`,
`TestRegisterRefusesADuplicateName`, and `TestComponentTypeValidate` (in `types`, the five types pass and `""` and
`"procesor"` fail). All fail on the base commit, because the methods do not exist. graph-ingest's `Register`,
`registry_integration_test.go` and the README move to the new call.

### D5 `Snapshots` returns `[]Declaration` (#153 item 3)

The result is sorted by instance name, and each value is a deep copy (`cloneComponentDeclaration`). Removed:
`declarationSnapshot` and its revive directive, plus the unexported `Registry.declaration` and
`Registry.declarationsSnapshot` (inventory F-3), which `Snapshots` now duplicates. Tests read `Snapshots`. Proof: the
boot-admission tests read the fields, and a test that changes a returned declaration's ports and reads the snapshot
again sees no change. Change 3's two services read the fields (inventory § 1).

### D6 `Discoverable` without `ConfigSchema` and `Meta` (#197; ruling 1)

`Discoverable` keeps `InputPorts`, `OutputPorts`, `Health` and `DataFlow`. `ConfigSchema()` and `Meta()` leave the
interface, graph-ingest, `SimpleMockComponent` and the test components, and the `Metadata` type is deleted.
A component that still defines either method compiles unchanged; only `component.Metadata` as a return type stops
compiling. At cutover that is every consumer component's `Meta` method (semsource 26 production lines name
`component.Metadata`, semconnect 2), and the compiler finds each one. graph-ingest names its consumer with the field
it already holds, `c.name` (`processor/graph-ingest/component.go:690`, `"graph-ingest"`, the value `Meta().Name`
returns today), so the consumer context it passes at `:1547` is unchanged.

Proof: `TestDiscoverableNeedsOnlyPortsHealthAndFlow` assigns a type that has only those four methods to a
`Discoverable`, so it does not compile on the base commit. `component_test.go:603` reads
`ListFactories()["graph-ingest"].Schema`. The README says a factory's schema, description and version are reported
through its registration.

### D7 Test code leaves production files (#153 item 5, F-3)

`SimpleMockComponent` moves into a `_test.go` file, and the unread `createSimpleMockComponent` is deleted.
`attack_test.go` is deleted, because its seven tests drive only test-local values. Proof: `task build`, `task lint`,
and `task cover:check`, which holds the 80% floor. The ledger's `proving_tests` drops `attack_test.go`.

### D8 Plain prose, and the alias goes (#153 items 4 and 7)

The `LifecycleComponent` doc lists ADR-108 Decision 3's rules as the ADR words them
(`docs/adr/108-processor-shell.md:74-83`), stated as outcomes:

- nil and ended contexts are refused;
- `Stop` before `Start` is safe;
- `Stop` is controlled while the `Start` context is still live, and ending that context first makes it an abort;
- a repeated `Stop` is a no-op;
- a second `Start` is refused, and there is no same-instance restart, ever (ADR-094, ADR-095);
- a failed `Start` holds nothing, or `Stop` releases what it left;
- configuration is validated once, at construction, and `Initialize` is idempotent and does no I/O.

The doc text names no package. The rollback behind the sixth rule is `pkg/lifecyclecleanup`'s. graph-ingest breaks
the last rule today: it validates in its factory (`processor/graph-ingest/component.go:622`) and again in `Initialize`
(`:955`). The second validation leaves with PR 10 or the processor shell (#109); this PR does not fix it. Proof:
review only (ADR-108 (e)).

The `PlatformMeta` alias (`dependencies.go:16-18`) is removed. `Dependencies.Platform` is declared as
`types.PlatformMeta`, its only production reader here. The tests that name the alias switch to `types.PlatformMeta`.
This follows the same rule as D4: a rename the compiler finds at cutover (semsource, 9 lines) does not keep a second
name. The `harness-boundaries` requirement names the field `component.Dependencies.Platform`, which stays. Proof:
`task build` and `task test:unit`.

### D9 The dead schema validator goes (inventory F-1; owner ruling 5)

`ValidateConfig(map[string]any, ConfigSchema)`, `ValidationError` and their helpers in `schema.go` are deleted, with
their tests. Proof: `task build` and `task cover:check`.

Generated checks: not used. Each decision has a small, finite set of inputs, and the tests list every one
(`docs/testing.md`).

## Issues

| Issue | Disposition |
| --- | --- |
| #189 | Retired (`Closes #189`): tasks 2.1–2.3, 3.1–3.3 |
| #197 | Retired (`Closes #197`): tasks 5.1–5.2 |
| #153 | Retired (`Closes #153`) for items 1–5 and 7's alias: tasks 4.1–4.3, 5.3–5.4. The rest is routed by task 6.2 (owner ruling 2) |
| #196 | Stays (change 3); gains #153 item 6 by comment (task 6.2) |
| #272 | PR 9b; gains #153 item 7's two `pkg/lifecycle` structs by comment (task 6.2) |
| new issue | The storage doc comment and `Declaration`'s parallel lists; text in `notes/issue-153-remainder.md` (task 6.2) |
| #29 | Stays open, although the work is done (inventory F-4). Not ruled; closing it needs the owner's word |
| #109, #139, #34, #110, #274 | Stay: the processor shell (change 4), graph-ingest (PR 10), `output/websocket`, temporal's port (#132), and `pkg/security`'s contract line |

## Merge order

No open PR shares a code file. #272 (9b) edits other rows of `docs/admission-ledger.yaml`, so whichever PR lands
second merges `origin/main` first. #275 (PR 3) owns `pkg/types`; D4 edits the root `types` package, so the same rule
applies. PR 10 (`processor/graph-ingest`, unclaimed) should start after this PR, because this PR edits graph-ingest's
`Register`, `ConfigSchema`, `Meta` and consumer name.

## Owner rulings (2026-10-16)

Recorded by the coordinating session from the owner's answers to this design's questions (at `2d72c72`), and posted on
the issues: rulings 1, 2 and 5 on #153 (comment 6214880970), rulings 3 and 4 on #189 (comment 6214881340).

1. **One register path; `Metadata` goes.** The owner's words: "agree that we should have a single register path. i
   am also okay getting rid of metadata if it's not used. but i am pretty sure metadat was deigned for ui/human
   interaction/understanding of each component in a flow?" Checked at the pin: what people see of a component type
   comes from the registration (`composition/catalog.go:15-27`), and `Meta()` is read only to name a component's own
   consumer and in four debug logs (section "Options"). So the condition holds: D4 and D6.
2. **Route the rest of #153 as proposed.** Item 6 goes to #196, item 7's `pkg/lifecycle` structs to #272, and one new
   issue takes the storage doc comment and `Declaration`'s parallel lists (task 6.2).
3. **`int`, `float`, `bool`** (D3).
4. **`SafeUnmarshal` refuses unknown keys, in this PR** (D2).
5. **Delete `ValidateConfig` and `ValidationError`** (D9).

Issue #29 was not answered and stays as it is.
