# Inventory: port-composition

base: `8bfab9771e48363004b8c8adc4cefe267c55bd93` (origin/main `3a8611f` plus the claim, draft PR #342). Pin paths at
SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read from the pin tarball
(`gh api repos/C360Studio/semstreams/tarball/8b99efe9`).

## Admission

- `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:216`: "`composition` (914) +
  `component/flowgraph` (1,324) | `service/component_manager.go:402` boot path | carry | **Admit (carry)**: ADR-100
  boot validation; the home for #16's refusal (D11)". D11 `:481`: the #16 row lists `composition`
  ("`composition.Analyze` refuses any stream whose subjects cover a reserved request subject; error names stream,
  filter, subject").
- `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:165` (change 3 list); `:176`: `composition/cli`
  is change 7; `:199`: "`composition/assert.go` in change 3" is a T-B1 collision; `:421`: "#16 `composition.Analyze`
  refusal | 3"; `:478`: change 3 carries `graph-transport-boundary` (MODIFIED: `composition.Analyze` refusal
  scenarios); ruling (g) `:724`: "Q12 is read per package for multi-package repair rows".
- `docs/tier1-cross-check.md:68`: `composition`, `composition/cli` in the ruled 65 (tier 0 only). No ledger row (`git
  grep -n -w composition docs/admission-ledger.yaml`: only the flowgraph and config rows' prose, `:2619-2625`,
  `:2659`, `:2680`). Order: #9 comment 6270875286 (sixth of seven); session note #9 comment 6294662570.
- Pin lines, non-test / test: `analyze.go` 291, `assert.go` 31, `catalog.go` 93, `doc.go` 19, `findings.go` 141,
  `graph.go` 123, `mermaid.go` 60, `validate.go` 156 = **914**; `assert_test.go` 60, `catalog_test.go` 52,
  `fmt_test.go` 5, `helpers_test.go` 120, `mermaid_test.go` 52, `shipped_configs_test.go` 113, `validate_test.go` 438
  = **840**. Not carried: `composition/cli/main.go` 151 + 179 test (change 7). Under the #159 bound.
- Dependencies, all in tree: `component`, `component/flowgraph` → `internal/flowgraph` (PR #320), `config`, `types`;
  stdlib `encoding/json`, `errors`, `fmt`, `sort`, `strings`, `testing` (`assert.go` only). No goroutine, no I/O, no
  `context` use (`grep -n 'context\.' composition/*.go`: none).

## Pin probe

go1.26.4 darwin/arm64, package only. The lifecycle suite does not apply because the package has no lifecycle owner.

- `go test -race -count=1 -p 1 -cpu 1 ./composition/...`: ok (both packages). `-count=5 -p 1 -cpu 1 -shuffle=on
  ./composition`: ok (a separate `-v` shuffle run: seed 1792510980315911000). Coverage 89.6%; under 100%:
  `Mermaid` 75.8%, `interfaceType` 80.0%, `AssertValid` 83.3%, `add` 83.3%, `declareInstance` 84.2%, `graphOf`,
  `orphanedPortFinding`, `interfaceFindings` 85.7%, `subscriberStreamNames` 87.5%, `sortFindings` 88.9%, `Validate`
  91.4%, `explicitStreamCovers` 92.9%, `Catalog` 95.0%.
- Test-text rules over `composition/` and `composition/cli/`: no `time.Sleep`, `Skip*`, build tag, `t.Parallel`,
  `NewTestClient`, or `TestNoFixedAddressesInTests` literal (`nats://localhost:`, `nats://127.0.0.1:`, `:4222"`,
  `:8222"`; `internal/harness/contract/addresses_test.go:17-22`). Its one `NewTestClient` hit is a comment,
  `assert.go:13`. `validate_test.go:46,197` declare `NetworkPort{... Port: 14550}`, a declared port with no bind.
  gofmt: clean. The tree's revive (`go tool revive -config revive.toml <pin path>/...`) reported nothing, but it was
  not sensitivity-checked on an out-of-module path, so this is unconfirmed.
- Behaviour probe (a throwaway test in the pin copy, then removed) for two components on one port, counting
  `exclusive_resource_conflict` errors: `http:0.0.0.0:8080`/`websocket:0.0.0.0:8080` 0; `http:0.0.0.0`/`http:::` 0;
  `0.0.0.0`/`127.0.0.1` 0; `UDP`/`udp` 0; same text 1; `udp`/`http` on one port 0, which the registry also admits.
  No other error finding is emitted in the five zero cases. One component claiming the same text twice: no finding
  (read from the pin code, `validate.go:58-64`; the tree's `Declare` refuses it). The tree's registry refuses the
  first four (`openspec/specs/component-registration/spec.md:149-200`).

## Shape sweep (pin `composition`)

1. Second spelling:
   a. Exclusive-claim rule: `validate.go:62` — `for _, resource := range declaration.ExclusiveResources {` and
      `validate.go:140-142` — `holder, taken := claimed[resource]` compare resource text. The tree's one rule is
      `component/registry.go:659` — `func claimsClash(a, b PortFacts) bool {`, with `component/port_facts.go:165` —
      `func (f NetworkFacts) clashes(other NetworkFacts) bool {`. **Required**: #338 (owner "Wanted"), probe above.
   b. Admission pre-checks: `validate.go:94-121` repeats name, entry, factory-name, unknown-factory and type checks
      that `component/registry.go:256-282` (`Declare`) also runs, before calling `Declare` at `:124`. It exists so
      each refusal gets its own finding type without reading error text. Optional.
   c. Edge order: `graph.go:103-121` and `mermaid.go:24-42` — the same six-key comparator. Optional.
   d. Interface type of a fact: `analyze.go:283-291` (`interfaceType`) and `graph.go:63-65`. Optional.
2. Copied shell: none (no Start/Stop/watch).
3. Layered interface: none (`grep -n 'interface {'`: none).
4. Exported surface and text:
   - `assert.go:14` — `func AssertValid(t testing.TB, catalog *component.Registry, cfg *config.Config) {` imports
     `testing` from a production file. The tree's `TestImportGraph` (`internal/harness/contract/imports_test.go:17`;
     `openspec/specs/harness-boundaries/spec.md:14`) refuses it, so **required** (`verify`). Pin readers: tests only
     (`assert_test.go:46,56`). Consumer reader: semteams `test/contract/composition_contract_test.go:30`. Pin contract
     line: `composition-validation/spec.md:219-229`. Its cited precedent, `natsclient.NewTestClient`, was rehomed to
     `internal/harness/natsfixture` (`docs/admission-ledger.yaml:32-48`).
   - `catalog.go:38-46` `displayNames` names 7 factories. `robotics`, `graph-processor` and `context-processor` have
     no factory at the pin (`grep -rn 'Name: *"<n>"'`: 0 each). `udp` is `input/udp`, which is not admitted. Optional.
   - Text describing behaviour no code implements once ported over `internal/flowgraph`: `analyze.go:49` —
     `"Give each network listener its own address",` and `findings.go:24` — `(network binding or KV writer
     conflicts, ...)`. Flowgraph's network-bind refusal was removed (ledger `:2650`, item 9). **Required** under
     surface audit (c). `analyze.go:195` — `case flowgraph.IssueOptionalInterfaceUnused:` does not compile on the
     tree (ledger item 6), so **required**. `findings.go:66-68` and `analyze.go:212` cite deleted SemStreams paths.
     Optional.
   - Type change: `validate.go:111` — `if registration.Type != string(entry.Type) {`, `catalog.go:67` —
     `Type: registration.Type,` and `catalog.go:72` — `Category: registration.Type`. In the tree,
     `Registration.Type` is `types.ComponentType` (`component/registry.go:47`), a compile adapt. A
     `go build -overlay` of the pin files onto the tree (import paths rewritten, nothing written) gives exactly four
     errors: `analyze.go:195`, `catalog.go:67`, `catalog.go:72`, `validate.go:111`.
   - Parallel port and fact lists: `analyze.go:145` — `if index >= len(declaration.InputFacts) {`, `:284`,
     `graph.go:61`. Guards for the shape #280 item 2 describes; that decision is left to the `service` port.
5. Classification by error text: none (`err.Error()` only builds messages, at `validate.go:34,86`, `analyze.go:47`
   and `catalog.go:83`).

## Who reads it (#190)

Pin production readers outside the package (`grep -rn 'composition\.<Sym>\b'` minus `_test.go`):

- `Analyze`: `service/component_manager.go:389,402`. `Result`: `:90,:437`; `component_manager_http.go:280`; cli
  `:62,:117`. `Graph`: `component_manager_http.go:281`. `Node`: `component_manager.go:1433`.
- `Catalog`: `component_manager_http.go:371,374,429`; cli `:76`; `cmd/openapi-generator/main.go:75` (not admitted).
  `CatalogEntry`: `component_manager_http.go:388`; openapi-generator `:74,:184`. `Ports`: openapi-generator `:158`.
- `Validate`: cli `:127` (admitted, change 7); `processor/agentic-tools/executors/composition_tools.go:17,88`
  (excluded). `Mermaid`: `component_manager_http.go:576`; cli `:104`; agentic-tools `:98`.
- `Finding`, `PortView`, `Edge`: reached through `Result`/`Graph` fields; direct readers in tests only.
- `Type*`, `Severity*`, `Status*` constants: read inside the package. Outside it, only tests and
  `test/e2e/client/observability.go:298` (`TypeDisconnectedNode`, not admitted) read them. `StatusValid` has no
  reader outside the package. Pin contract: `composition-validation/spec.md:74-100` ("the exported constants").
- `AssertValid`: as in sweep item 4.
- SemEngine: nothing imports `composition` (`git grep -n -w composition -- '*.go'`: comments only).
- Consumers (`gh search code`): semteams uses `AssertValid` (above) and reflects `composition.Result{}` and `Graph{}`
  in `cmd/openapi-generator/openapi_generator_test.go:457`. semsource only lists the package in
  `docs/testing/*/dependency-results.json` (it is in its dependency closure, not read by its code). semconnect and
  semboids: 0. `composition/cli`: 0 in all four. `ExclusiveResources`: 0 in all four.
- `Declaration.ExclusiveResources` in tree: written at `component/registry.go:315,619`, cloned at `:708`, read only
  by `component/registry_boot_admission_test.go:62,87-88,499` and `registry_declare_test.go:86-87`. At the pin, its
  readers are `component/registry.go:465,728` and `composition/validate.go:62,140`.

## Pin ADRs and specs (#157)

All 85 records searched for `composition/`, `composition.`, `AssertValid`, `ExclusiveResources`, `exclusive
resource` and `Mermaid`. ADR-100 governs the package; ADR-094 and ADR-096 were read because ADR-100 cites them;
ADR-075 was read by title.

- ADR-100 (`100-...md:31-52`) **binds**. Decision 1: the unit is the boot config plus the catalog, and a diagram is
  a projection. Decision 2: port declarations are static facts. Decision 3: one validator, one vocabulary, offline
  and at boot; boot refuses an error finding. Decision 4: `catalog`/`validate`/`graph` through one CLI entry point
  (`composition/cli`, change 7). It is not in SemEngine `docs/adr` (`ls docs/adr | grep ^100`: none).
- ADR-094 (`:35-48`): boot seals composition. It governs `service`/the registry seal; `:47-48` is superseded by
  ADR-100.
- ADR-075 (explicit capability composition): about registration roots. It names no composition symbol.
- ADR-096: its paragraph 4 stands as substrate (ADR-100 `:78-82`); it governs `service`.
- `exclusive` hits in ADR-063 and ADR-087 concern store ports and KV ownership, not network claims.
- Pin spec `openspec/specs/composition-validation/spec.md` (403 lines; requirements at `:17`, `:74`, `:182`, `:219`,
  `:232`, `:266`, `:325`, `:347`, `:359`). `:74`, `:219` and `:347` govern this package; `:182` covers the CLI;
  `:232`, `:266` and `:359` cover `service`; `:325` covers agentic tools (excluded). `:347` requires a
  shipped-configs test: the pin's `shipped_configs_test.go:10-18` imports `componentregistry` (not ported,
  `docs/tier1-cross-check.md:95`), `cmd/e2e-semstreams/mission`, `examples/*` and `frameworkadapters/otel`.
  SemEngine ships no `configs/` directory; `config/example_config.json` exists. No pin skill or contract section
  names the package (`.agents/skills/semstreams-dev/SKILL.md:33` names only the framework-composition spec).

## Open issues (#160)

`gh issue list --state open --search composition` returns #338, #16, #24, #9, #96 and #7. Searches for
`AssertValid`, `Mermaid` and `composition/cli` return nothing. `ExclusiveResources` and `claimsClash` return #338.
`Declaration` returns #280, #338, #16, #106 and #9.

- #338: names `composition/validate.go:58-65,:139-155`. Retired by this PR.
- #16: names `composition.Analyze` through D11 and 04A `:421`. The composition share (a refusal naming stream,
  filter and subject) is a repair-before-port gate under ruling (g), so it is **required here**. The issue stays
  open: the real-NATS PubAck regression and refusal before readiness need `service`, and graph-query's site is
  change 4. Reserved subjects in the tree: `graph/query_verbs.go:25` (`QueryVerbs()`) and
  `internal/graphmutation/protocol.go:18` (`SubjectFamily`). No code checks streams against them today
  (`git grep -n 'QueryVerbs\|graphmutation.SubjectFamily'`: no config or stream reader).
- #9: the epic. Stays open (`Addresses #9`).
- #280 item 2: names `component.Declaration`'s parallel lists. Composition reads them; the decision is left to the
  `service` port, so it stays.
- #24, #96, #7: use the word, not the package. #106 (graph.query verb catalog) sits next to #16's reserved set in
  change 4.

## Overlapping PRs

`gh pr list --state open`:

- #343 (draft, `service` inventory): `openspec/changes/setup-04a-03-boot-service/proposal.md` only.
- #342: this claim.
- #341 (draft, #325): four files under `openspec/changes/config-key-letter-case/`. Its tasks edit `config/config.go`
  (`loadRawJSONFromBytes`, a new `rejectCaseVariantKeys`), `config/load_test.go`, `config/README.md` and
  `config/doc.go`. Composition reads `*config.Config` after loading (`Clone`, `Validate`, `Streams`) and never
  loads, so no file or behaviour overlaps.

Merge order: this PR before the component manager's `service` PR, which imports `composition`. #341 and #343 may
land in any order relative to this one.

## Problem shape

Admit-or-refuse at a seam, as an offline prediction of boot admission. The closest instance in the tree is
`component/registry.go:579` — `func (r *Registry) checkResourceConflictsLocked(candidate Declaration) error {`,
which uses `claimsClash` `:659`, and `exclusiveResources` `:626` (refuses within one component, so `Declare`
already refuses a component that clashes with itself). At the pin a self-clash yields no finding (the pin's
`component/registry.go:763-772` only sorts and compacts the claim text, and `composition/validate.go:58-64` checks
`claimed` before adding the component's own claims); on the tree it is `port_declaration_error`, an outcome that
changes from the pin. No other reader would adopt an exported
clash rule: flowgraph dropped its bind refusal (#324), and the pin's `service` reads no `ExclusiveResources`. No
collision table is needed for the clash rule.

For #16's refusal (a stream filter covering a reserved request subject) the tree already has: the matcher
`internal/flowgraph/flowgraph.go:275` `SubjectCovers`, which pin composition already calls at `analyze.go:125`; the
reserved subjects `graph.QueryVerbs()` (`graph/query_verbs.go:25`) and `graphmutation.SubjectFamily`
(`internal/graphmutation/protocol.go:18`), which composition must read, never spell (literal-spelling guard
`internal/harness/contract/reservedsubjects_test.go:43`, current spec
`openspec/specs/graph-transport-boundary/spec.md:11`, where 04A `:478` puts the refusal scenarios); and the stream
type `config/streams.go:57` `StreamConfigs`. No further collision table is needed (no durable, communication or
coordination primitive). No intent check is needed (no boundary moves).

## Adopter seam

The package is public at its pin path, and products reach it through the CLI (change 7), the component manager's
HTTP projection and boot refusal (`service`), and `AssertValid` in product CI.

1. What must they know? Today, nothing beyond `Validate(catalog, cfg)`. If `AssertValid` is dropped, they need two
   facts: "check `len(result.Errors)`" and "warnings pass".
2. If they do nothing: an offline `Validate` that admits a clash the registry refuses (O1 below) turns a CI pass
   into a boot error.
3. Where they find out: a boot error (registry) today; a typed finding in CI once the rule is shared.
4. What they should know: nothing. The offline and boot judgments agree (ADR-100 decision 3).

## Options for the design (outside the inventory under review; no recommendation)

How the clash rule reaches `composition`, and what each option changes in `component`'s exported surface:

- O1, do nothing: port the text compare. No change to `component`, but it is a second spelling of the rule, which
  #338's "Wanted" rules against. `ExclusiveResources` keeps its one reader.
- O2, pairwise rule exported: `claimsClash` becomes, for example, `func (f PortFacts) Clashes(other PortFacts)
  bool` (+1 method), and the registry calls it. Composition selects claims itself with `IsExclusive()`, which is
  already exported, so claim selection gets a small second spelling.
- O3, declaration-level check exported: for example `func (d Declaration) Clash(held Declaration) (claim,
  heldClaim string, ok bool)` or a function returning the registry's own invalid-class error (+1 symbol).
  `checkResourceConflictsLocked` delegates to it, so the rule, the claim selection and the message have one home.
- O4, offline admission on `Registry` (for example a method judging a set of declarations): the largest addition;
  it overlaps `CreateComponent`, which the `service` manager reaches through `service.ComponentManager` (#91).
- O5, drop composition's own check and leave clashes to boot admission: no `component` change. The vocabulary
  shrinks from 13 to 12 types, and the offline prediction diverges from boot, which changes ADR-100 decision 3 and
  so needs an owner ruling.
- O2, O3 and O5 leave `Declaration.ExclusiveResources` with no production reader in the tree or in any consumer.
  The design must drop it or state why it stays.

## Open questions for the owner

1. The pin's one-line test helper for product CI (`AssertValid`) cannot live in a normal package here, because no
   production code may import Go's `testing` package. semteams calls it in one test. Drop it (products call
   `Validate` and check the error list), keep it only for SemEngine's own tests, or add a named exception to the
   rule?
2. The approved plan puts a second check into `composition` in this change: refuse a JetStream stream whose subjects
   would capture the graph's request subjects (#16). The approved plan already says so (03B D11 `:481`, 04A `:421`,
   ruling (g) `:724`), which makes this PR bigger. The question is only whether to waive that for this PR and give
   the check its own PR.
3. Once composition uses the registry's clash rule, nothing reads the resource-text list on a component
   declaration. Remove it in this PR (a small change to `component`) or file it?
4. The pin's test that every shipped config validates cannot be ported because its imports are not admitted.
   SemEngine has one example config. Should a test validate it, or does that requirement wait for a consumer?
