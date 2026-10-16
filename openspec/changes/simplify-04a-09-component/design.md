# Design: simplify-04a-09-component

Status: draft, for inventory review, then pre-owner design review, then the owner's answers on #153.
Inventory: `inventory.md` (base `0fb79ad`).

## Context

Epic #223, PR 7, `component` only (#159). Issues #153, #189 (owner ruling: keep `SafeUnmarshal` and fix it) and #197
(owner ruling 2026-10-12: drop `ConfigSchema()` from `Discoverable`). The epic line's first #153 item, "remove the
remaining `drop`-marked exports", is already done on `main` (inventory § 1); this change does not repeat it.
No ADR changes. ADR-100 Decision 2 keeps the registration as the home of a factory's schema and ports; ADR-108
Decision 3 (e) and ADR-095 set what the `LifecycleComponent` doc says.

## Options for the registration types (#153 item 2)

1. **One `Registry.Register(Registration) error` (recommended).** It keys the factory by `Registration.Name`, checks a
   typed `Type`, and stores a copy. `RegisterFactory`, `RegisterWithConfig` and `RegistrationConfig` go. Cost: at
   cutover semsource changes 26 calls (`RegisterWithConfig(component.RegistrationConfig{…})` to
   `Register(component.Registration{…})`, a rename) and semboids 2. The compiler finds each one.
2. **Keep `RegisterFactory(name, *Registration)` only, refusing a `name` that differs from `Registration.Name`.**
   The fix is smaller, but the name is still spelled twice, and semsource changes its 26 calls anyway.
3. **Do nothing.** Two methods and two structs stay for one job, and a misspelled type is still found only at boot.

`Metadata` (from `Discoverable.Meta()`) stays. It is the running instance's own report, graph-ingest reads it in
production (`component.go:1547`), and removing it would change `Discoverable` for every consumer component (owner
question 1). `Info` is already gone.

## Decisions

### D1 The config check refuses data after the first JSON value (#189 item 1)

`ValidateFactoryConfig` returns an invalid-class error when anything other than whitespace follows the first value.
So do `CreateComponent`, `Declare` and `SafeUnmarshal`, which call it. The check is the same as
`component/port_codec.go:221`. Proof: `TestValidateFactoryConfigRefusesDataAfterTheFirstValue` (`{} {}`, `{} x`, and
`{}` followed by a newline, which passes), plus a `CreateComponent` row in which the factory is never called. Both fail
on the base commit.

### D2 `SafeUnmarshal` refuses a nil target (#189 item 2)

`SafeUnmarshal(raw, nil)` returns an invalid-class error and does not panic. A typed nil pointer is refused as well.
`component/README.md` gains the contract line: what `SafeUnmarshal` checks, in what order, and that it is offered to
component factories (#190). Proof: `TestSafeUnmarshalRefusesANilTarget`, which panics on the base commit. Owner
question 4 asks whether it should also refuse unknown keys (inventory F-2).

### D3 One word for each basic type in a schema (#189 item 3)

Schemas use `int`, `float` and `bool` everywhere: `generateItemSchema` stops emitting `integer`, `number` and
`boolean`. These are the words the field's doc (`discovery.go:53`), the tag parser, every property, `ValidateConfig`
and semteams' schemas already use. Only array items use the other words. Proof:
`TestGeneratedSchemaUsesOneWordPerType` walks a generated schema (properties, nested objects, items of items) and
requires every `Type` to be a word `isValidType` accepts. It fails on the base commit. Owner question 3.

### D4 One registration call (#153 item 2; option 1)

`Registry.Register(Registration) error` refuses each of these with an invalid-class error naming the factory:

- a `Name` that `ValidateComponentName` refuses (such a name can never be created today);
- a nil `Factory` or `Ports`;
- a `Type` outside the five component types, using the list `types` already holds (`types/component.go:122`), not a
  copy;
- a `Name` already registered.

`Registration.Type` becomes `types.ComponentType`, so literals such as `Type: "processor"` still compile. The registry
keeps its own copy. A later change to the caller's value, its schema or its dependency list changes nothing. Proof:
`TestRegisterRefusesATypeOutsideTheFive` (`"procesor"`), `TestRegisterKeepsItsOwnCopy` and
`TestRegisterRefusesADuplicateName`. All fail on the base commit, because the method does not exist. graph-ingest's
`Register`, `registry_integration_test.go` and the README move to the new call.

### D5 `Snapshots` returns `[]Declaration` (#153 item 3)

The result is sorted by instance name, and each value is a deep copy (`cloneComponentDeclaration`).
`declarationSnapshot`, its revive directive, and the test-only unexported `Registry.declaration` (inventory F-3) go.
Tests read `Snapshots`. Proof: the boot-admission tests read the fields, and a test that changes a returned
declaration's ports and reads the snapshot again sees no change. Change 3's two services read the fields
(inventory § 1).

### D6 `Discoverable` without `ConfigSchema` (#197, as ruled)

The method leaves the interface, graph-ingest, `SimpleMockComponent` and the test components. A component that still
defines it compiles unchanged. Proof: `TestDiscoverableNeedsNoConfigSchema` assigns a type that has no
`ConfigSchema` to a `Discoverable`, so it does not compile on the base commit. `component_test.go:603` reads
`ListFactories()["graph-ingest"].Schema`. The README says the schema is reported through the registration.

### D7 Test code leaves production files (#153 item 5, F-3)

`SimpleMockComponent` moves into a `_test.go` file, and the unread `createSimpleMockComponent` is deleted.
`attack_test.go` is deleted: its seven tests drive only test-local values. Proof: `task build`, `task lint`, and
`task cover:check`, which holds the 80% floor. The ledger's `proving_tests` drops `attack_test.go`.

### D8 Plain prose (#153 item 4; item 7, the alias)

The `LifecycleComponent` doc lists the rules a caller can observe, as the `lifecycle-suite` floor (`:15-24`) states
them: a nil context is refused; a second `Start` is refused; `Stop` before `Start` returns nil; `Stop` is bounded by
its own context; a repeated `Stop` does nothing; a failed `Start` releases what it took, or `Stop` releases it later.
The `PlatformMeta` alias stays, because semsource names it 9 times; its comment drops the false "import cycles"
reason. Proof: review only.

### D9 Dead schema validator (inventory F-1; owner question 5)

If the owner agrees, `ValidateConfig(map[string]any, ConfigSchema)`, `ValidationError` and their helpers in
`schema.go` are deleted, with their tests. Proof: `task build`, and `task cover:check`.

Generated checks: not used. Each decision has a small, finite set of inputs, and the tests list every one
(`docs/testing.md`).

## Issues

| Issue | Disposition |
| --- | --- |
| #189 | Retired (`Closes #189`): tasks 2.1–2.3, 3.1–3.3 |
| #197 | Retired (`Closes #197`): tasks 5.1–5.2 |
| #153 | Retired (`Closes #153`) for items 1–5 and the alias, provided the owner agrees that the rest moves to a new issue (owner question 2): tasks 4.1–4.3, 5.3–5.4, 6.2 |
| #29 | Stays open, although the work is done (inventory F-4). Closing it needs the owner's word |
| #109, #139, #34 | Stay: their work is the processor shell (change 4), graph-ingest (PR 10) and `output/websocket` |

## Merge order

No open PR shares a code file. #272 (9b) edits other rows of `docs/admission-ledger.yaml`, so whichever PR lands
second merges `origin/main` first. PR 10 (`processor/graph-ingest`, unclaimed) should start after this PR, because this
PR edits graph-ingest's `Register` and `ConfigSchema`.

## Owner questions

1. **Registration: one call, and `Metadata` stays.** Recommended: yes to both. Today a component can register through
   two methods and two structs that hold the same fields. One `Register` call replaces them. At cutover, semsource
   renames 26 calls and semboids changes 2. The compiler points to each one. `Metadata`, the description a running
   component gives of itself, stays as it is. Removing it would mean changing every consumer's components.
2. **Move the rest of #153 to a new issue, so that this PR can close #153.** Recommended: yes. Four items in #153
   are not in `component`: the lifecycle guard's two error spellings (item 6), two one-field structs in
   `pkg/lifecycle` (item 7; PR 9b's package, which its inventory did not list), an outdated storage doc comment, and
   two parallel lists in `Declaration` that change 3's services read. The new issue would hold all four.
3. **Schema type words: `int`/`float`/`bool`.** Recommended: these, not JSON Schema's `integer`/`number`/`boolean`.
   Every reader and writer in this repository already uses the short words. Only the code that describes the items of
   a list uses the long ones.
4. **Should `SafeUnmarshal` also refuse configuration keys it does not know?** Recommended: yes, in this PR. The
   project's rule is that a ported component refuses an unknown key and names it. Today the one helper offered to
   factories quietly ignores unknown keys, so a component that uses it breaks the rule. No consumer calls it yet. The
   cost is two lines and one test.
5. **Delete the unused schema validator?** Recommended: yes. `ValidateConfig` and `ValidationError` (about 200 lines)
   have no caller here, in any consumer, or in SemStreams. Their one caller was a file the port left out.

For information: #29 asked to remove the tool registry from component dependencies. That work was done when the ingest
kernel merged, but the issue is still open. It can be closed on your word.
