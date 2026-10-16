# Design: simplify-04a-09-component

Status: draft, revised after review round 1 (of `7f4a02d`). Next: pre-owner design review, then the owner's answers
on #153. Inventory: `inventory.md` (base `0fb79ad`).

## Context

Epic #223, PR 7, `component` only (#159). The issues are #153, #189 and #197. On #189 the owner ruled to keep
`SafeUnmarshal` and fix it. On #197 the owner ruled on 2026-10-12 to drop `ConfigSchema()` from `Discoverable`. The
epic line's first #153 item, "remove the remaining `drop`-marked exports", is already done on `main` (inventory § 1),
so this change does not repeat it. No ADR changes. ADR-100 Decision 2 keeps the registration as the home of a
factory's schema and ports. ADR-108 Decision 3 sets the rules the `LifecycleComponent` doc states.

## Options for the registration types (#153 item 2)

1. **One `Registry.Register(Registration) error` (recommended).** It keys the factory by `Registration.Name`, checks a
   typed `Type`, and stores a copy. `RegisterFactory`, `RegisterWithConfig` and `RegistrationConfig` go. Cost: at
   cutover semsource renames 26 calls (`RegisterWithConfig(component.RegistrationConfig{…})` to
   `Register(component.Registration{…})`), and the compiler finds each one.
2. **Keep `RegisterFactory(name, *Registration)` only, refusing a `name` that differs from `Registration.Name`.**
   The fix is smaller, but the name is still spelled twice, and semsource changes its 26 calls anyway.
3. **Do nothing.** Two methods and two structs stay for one job, and a misspelled type is still found only at boot.

`Metadata` (from `Discoverable.Meta()`) stays (owner question 1). It is the running instance's own report. Change 3's
component manager reads it through the interface (pin `service/component_manager.go:697`), and removing it would change
`Discoverable` for every consumer component. `Info` is already gone.

## Decisions

### D1 The config check refuses data after the first JSON value (#189 item 1)

`ValidateFactoryConfig` returns an invalid-class error when anything other than whitespace follows the first value.
So do `CreateComponent`, `Declare` and `SafeUnmarshal`, which call it. The check is the same as
`component/port_codec.go:221`. Proof: `TestValidateFactoryConfigRefusesDataAfterTheFirstValue` (`{} {}`, `{} x`, and
`{}` followed by a newline, which passes), plus a `CreateComponent` row in which the factory is never called. Both fail
on the base commit.

### D2 `SafeUnmarshal` refuses a nil target (#189 item 2)

The nil-target check runs first, before the config check and before the early return for empty input
(`validation.go:173-176`). So `SafeUnmarshal(raw, nil)` and `SafeUnmarshal(nil, nil)` both return an invalid-class
error and do not panic. A typed nil pointer is refused too. `component/README.md` gains the contract line: what
`SafeUnmarshal` checks, in what order, and that it is offered to component factories (#190). Proof:
`TestSafeUnmarshalRefusesANilTarget`, with a row for valid input (it panics on the base commit) and a row for empty
input (it returns nil on the base commit). Owner question 4 asks whether it should also refuse unknown keys
(inventory F-2).

### D3 One word for each basic type in a schema (#189 item 3)

Schemas use `int`, `float` and `bool` everywhere: `generateItemSchema` stops emitting `integer`, `number` and
`boolean`. These are the words the field's doc (`discovery.go:53`), the tag parser, every property and `ValidateConfig`
already use. Only array items use the other words. Proof: `TestGeneratedSchemaUsesOneWordPerType` generates a schema
for a struct with `int`, `float64`, `bool`, `[]int`, `[]float64`, `[]bool` and `[][]int` fields. It requires exactly
`int`, `float` and `bool` for those types in properties and in item schemas at every depth, and fails on the base
commit. Owner question 3.

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
nothing. Proof: `TestRegisterRefusesATypeOutsideTheFive` (`"procesor"`), `TestRegisterKeepsItsOwnCopy`,
`TestRegisterRefusesADuplicateName`, and `TestComponentTypeValidate` (in `types`, the five types pass and `""` and
`"procesor"` fail). All fail on the base commit, because the methods do not exist. graph-ingest's `Register`,
`registry_integration_test.go` and the README move to the new call.

### D5 `Snapshots` returns `[]Declaration` (#153 item 3)

The result is sorted by instance name, and each value is a deep copy (`cloneComponentDeclaration`). Removed:
`declarationSnapshot` and its revive directive, plus the unexported `Registry.declaration` and
`Registry.declarationsSnapshot` (inventory F-3), which `Snapshots` now duplicates. Tests read `Snapshots`. Proof: the
boot-admission tests read the fields, and a test that changes a returned declaration's ports and reads the snapshot
again sees no change. Change 3's two services read the fields (inventory § 1).

### D6 `Discoverable` without `ConfigSchema` (#197, as ruled)

The method leaves the interface, graph-ingest, `SimpleMockComponent` and the test components. A component that still
defines it compiles unchanged. Proof: `TestDiscoverableNeedsNoConfigSchema` assigns a type that has no
`ConfigSchema` to a `Discoverable`, so it does not compile on the base commit. `component_test.go:603` reads
`ListFactories()["graph-ingest"].Schema`. The README says the schema is reported through the registration.

### D7 Test code leaves production files (#153 item 5, F-3)

`SimpleMockComponent` moves into a `_test.go` file, and the unread `createSimpleMockComponent` is deleted.
`attack_test.go` is deleted, because its seven tests drive only test-local values. Proof: `task build`, `task lint`,
and `task cover:check`, which holds the 80% floor. The ledger's `proving_tests` drops `attack_test.go`.

### D8 Plain prose, and the alias goes (#153 items 4 and 7)

The `LifecycleComponent` doc lists ADR-108 Decision 3's rules as the ADR words them
(`docs/adr/108-processor-shell.md:74-83`):

- nil and ended contexts are refused;
- `Stop` before `Start` is safe;
- `Stop` is controlled while the `Start` context is still live, and ending that context first makes it an abort;
- a repeated `Stop` is a no-op;
- a second `Start` is refused, and there is no same-instance restart, ever (ADR-094, ADR-095);
- a failed `Start` is rolled back through `pkg/lifecyclecleanup`, and a cleanup that fails stays pending for a later
  `Stop`;
- configuration is validated once, at construction, and `Initialize` is idempotent and does no I/O.

Proof: review only (ADR-108 (e)).

The `PlatformMeta` alias (`dependencies.go:16-18`) is removed. `Dependencies.Platform` is declared as
`types.PlatformMeta`, its only production reader here. The tests that name the alias switch to `types.PlatformMeta`.
This follows the same rule as D4: a rename the compiler finds at cutover (semsource, 9 lines) does not keep a second
name. The `harness-boundaries` requirement names the field `component.Dependencies.Platform`, which stays. Proof:
`task build` and `task test:unit`.

### D9 Dead schema validator (inventory F-1; owner question 5)

If the owner agrees, `ValidateConfig(map[string]any, ConfigSchema)`, `ValidationError` and their helpers in
`schema.go` are deleted, with their tests. Proof: `task build` and `task cover:check`.

Generated checks: not used. Each decision has a small, finite set of inputs, and the tests list every one
(`docs/testing.md`).

## Issues

| Issue | Disposition |
| --- | --- |
| #189 | Retired (`Closes #189`): tasks 2.1–2.3, 3.1–3.3 |
| #197 | Retired (`Closes #197`): tasks 5.1–5.2 |
| #153 | Retired (`Closes #153`) for items 1–5 and 7's alias. The rest is routed as owner question 2 proposes: tasks 4.1–4.3, 5.3–5.4, 6.2 |
| #196 | Stays (change 3); gains #153 item 6 (owner question 2) |
| #29 | Stays open, although the work is done (inventory F-4). Closing it needs the owner's word |
| #109, #139, #34, #110, #274 | Stay: the processor shell (change 4), graph-ingest (PR 10), `output/websocket`, temporal's port (#132), and `pkg/security`'s contract line |

## Merge order

No open PR shares a code file. #272 (9b) edits other rows of `docs/admission-ledger.yaml`, so whichever PR lands
second merges `origin/main` first. #275 (PR 3) owns `pkg/types`; D4 edits the root `types` package, so the same rule
applies. PR 10 (`processor/graph-ingest`, unclaimed) should start after this PR, because this PR edits graph-ingest's
`Register` and `ConfigSchema`.

## Owner questions

1. **Registration: one call, and `Metadata` stays.** Recommended: yes to both. Today a component can register through
   two methods and two structs that hold the same fields. One `Register` call replaces them. At cutover, semsource
   renames 26 calls, and the compiler points to each one. (Side note: semboids changes 2 more.) `Metadata`, the
   description a running component gives of itself, stays as it is. Removing it would mean changing every consumer's
   components.
2. **Send the rest of #153 to the places that already own that work, so this PR can close #153.** Recommended: yes.
   - The lifecycle guard's two spellings of one error (item 6) go to #196, the open issue for that guard.
   - The two one-field structs in `pkg/lifecycle` (item 7) go to PR 9b (#272), which is changing that package now.
   - Two items need one new issue: the outdated storage doc comment, and the two parallel lists in `Declaration`. The
     lists wait because joining them is not a plain cleanup. The call that would replace one of them,
     `Port.Facts()`, can fail (`component/port_resolver.go:168`), so merging them changes what a declaration
     promises. The two services that change 3 ports read these lists.
3. **Schema type words: `int`/`float`/`bool`.** Recommended: these, not JSON Schema's `integer`/`number`/`boolean`.
   Every reader and writer in this repository already uses the short words. Only the code that describes the items of
   a list uses the long ones. The likely reader of these schemas is a configuration screen, and that is outside what
   we may inventory, so we cannot check which words it expects.
4. **Should `SafeUnmarshal` also refuse configuration keys it does not know?** Recommended: yes, in this PR. The
   project's rule is that a ported component refuses an unknown key and names it. Today the one helper offered to
   factories quietly ignores unknown keys, so a component that uses it breaks the rule. No consumer calls it. At the
   pin, nine components call it: eight consumer adapters and `output/websocket`. They would start refusing a config
   that carries a key their config struct lacks. The cost here is two lines and one test.
5. **Delete the unused schema validator?** Recommended: yes. `ValidateConfig` and `ValidationError` (about 200 lines)
   have no caller here, in any consumer, or in SemStreams. Their one caller was a file the port left out.

For information: #29 asked to remove the tool registry from component dependencies. That work was done when the ingest
kernel merged, but the issue is still open. It can be closed on your word.
