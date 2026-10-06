# Design: authority-one-spelling

base: SemEngine `main` `2ec3bcf` (= the claim worktree `claude/authority-one-spelling`); SemStreams pin `8b99efe9`;
PR #48 head `c64ac338`. Revision 1, for independent pre-owner design review. Nothing here is approved.

## Context

Issue #72. The owner ruled (comment 5969505488) the rule's wording, **A**, and its enforcer, **C**: two checks in
`internal/harness/contract` under `task test:unit`, with the `AGENTS.md` row naming them. This design implements A
and C and nothing else; rulings B, D and E have other homes (the ruling's last paragraph). `inventory.md` is the
passed inventory; this design cites it by entry number and re-states none of it.

## What the inventory found, and what this design does

- One source, one establishing step, one carrier, one composer (inventory #1, #12, #19–#21, #45) — the rule names
  them; nothing here touches them.
- Four second spellings (#32, #34, #43, #44) — C-1 catches the first two by name and C-2 the third by field; the
  fourth (`summarizer.go:728`, a string join inside a function body) is outside both checks and stays review only
  (§"What the checks do not see").
- The carrier copies (#22–#25, #42): lowercase `org`/`platform` fields and `types.PlatformMeta`-typed fields. The
  owner did not ask for their consolidation; C-2's predicate is drawn so none of them fails (D3).
- `CallerContext.Org` (#47): the one named exclusion, by exact field name (D3).
- Open pull requests overlapping (inventory §4): #48 only. It carries C-1 itself (adopted here, D2) and the names C-1
  fails on; it merges first (D5).

## Goals and non-goals

Goals: the rule as a spec requirement with scenarios; two checks that fail with the file, the line and the rule;
a sensitivity test for each; the `AGENTS.md` row. Non-goals: consolidating any copy; a check on lowercase fields; a
check on values re-joined inside function bodies (a struct declared inside a function body is checked, D3, and gets no
exception: the exceptions name package-scope types); anything in changes 2–7 beyond recording what they will meet.

## Options

**O1 — the capability home.**

- (a) A delta to `harness-boundaries`. Evidence: `openspec/specs` holds five specs, all harness (`harness-boundaries`,
  `integration-test-runner`, `lifecycle-suite`, `merge-gate`, `nats-fixture`); `harness-boundaries` already holds
  two machine-checked rules over production code's shape — "No retained context" (`spec.md:20-29`, a struct-field
  rule with an exact-name exemption list) and, from PR #48, "Public signatures name no internal type"
  (#48's delta `:64-73`) — and its `AGENTS.md` rows point there. Cost: its Purpose (`spec.md:3-6`) names only the
  harness's own boundaries and needs one sentence in the archive commit, as `review-gate-check` did for
  `merge-gate` (its tasks 8.1).
- (b) A new capability, `deployment-authority`. Cost: a second spec of structural rules, so the next such rule must
  choose between two homes — the parallel-declaration shape the architect contract warns against; and strict
  validation accepts it no more readily than (a).
- (c) A delta to `entity-id-contract`, the rule's domain home at the pin (`spec.md:398-407`). Cost: that spec is not
  on `main`; it arrives with PR #48 as a ported document, and a delta archived before it would create a one-requirement
  spec that #48's port then collides with.

Chosen: (a). The rule's domain statement at the pin (`entity-id-contract/spec.md:400-402`, "`platform` is the minting
deployment authority: the composition root's `platform.id`, carried to components as `deps.Platform`") is cited by
the requirement; no edit to that spec is in scope.

**O2 — how C-1 relates to the public-signature contract.** (a) A second pass inside `TestPublicSignatures`; (b) a
sibling test over the same `go/packages` load. Chosen: (b), so the `AGENTS.md` row and the sensitivity pair have one
name each and a failure says which rule it is. PR #48 already carries that sibling as `TestNoDeploymentAuthorityNames`
(`signatures_test.go`, commit `bb004ef`); this change adopts it from PR #48 at its merged head rather than writing its
own (D2).

**O3 — C-2's predicate.** (a) Any field named `Org`/`Platform` outside the three packages, exact case; (b) also
lowercase `org`/`platform`; (c) also any field whose type is `types.PlatformMeta` or `platform.Config`. (b) and (c)
fail the carrier copies the owner left alone (#22–#25, #42). Chosen: (a).

**O4 — the carrier under (a).** Read literally, ruling C excludes `types.PlatformMeta`'s own two fields and nothing
else, so (a) fails the carrier that ruling A names ("carried to components once as `deps.Platform`"): at the pin,
`component/dependencies.go:73` (`Platform        PlatformMeta`, on `component.Dependencies`),
`service/dependencies.go:31` (`Platform          types.PlatformMeta`, on `service.Dependencies`) and
`processor/rule/rule_factory.go:101` (`Platform types.PlatformMeta`, on `rule.Dependencies`). Two ways to except
it, neither in ruling C's text, went to the owner as Q1: (i) by type — any field whose type unaliases to
`types.PlatformMeta` passes, under any name in any package; (ii) by exact field, in the requirement's own style, as
`CallerContext.Org` is named. **Ruled (ii)** (#72 comment 5969757891): the three fields are listed by package, type
and name, and both checks read non-test files only, as the harness norm. (i) is not in the predicate: a second
carrier of that type anywhere would have been invisible to the check.

## Decisions

### D1 The rule is one requirement with two enforcing paragraphs

`specs/harness-boundaries/spec.md` MODIFIES "No second spelling of deployment authority", the requirement PR #48
landed with C-1 (`openspec/specs/harness-boundaries/spec.md:459` on `main` `deaafd4`), rather than adding a second,
overlapping one: the owner's sentence verbatim as the SHALL, then **Names** (C-1, #48's text, with "case-sensitive"
and "comments and string literals" made explicit) and **Fields** (C-2), each stating what is matched, what is
excluded, what the failure names, and what is outside the check by construction. #48's closing sentence, that the
field check is not part of the requirement, is replaced by the Fields paragraph. Six scenarios, each observable by
running a test: #48's two name scenarios (the second gains the lower-case, comment and literal cases this change's
first delta named) and this change's four.

### D2 C-1: exported names containing `Federation`, `GlobalID` or `EntityIRI`

Scope: every package of the module (`./...` with the `integration` tag, as `loadModuleTypes` loads them for
`publicSignatureViolations`, `signatures_test.go:248-269` at `deaafd4`), including internal and `main` packages — the
ruling says "anywhere" — and in each, every exported object: package-scope names, exported methods of named types,
exported struct fields, interface methods, and alias members (#48's F11). Match:
the regular expression `Federation|GlobalID|EntityIRI` on the identifier, case-sensitive. Not checked: unexported
names, comments and literals; and test files, which every guard in `internal/harness/contract` leaves out
(`context_test.go:221-229` loads non-test packages; `testtext_test.go` is the one that reads test files, for text
rules) because a test file exports nothing a consumer imports — the harness norm, confirmed with Q1 (#72 comment
5969757891). Failure line, as #48 wrote it (`signatures_test.go:561-562, :600-601` at `30b9e5f`): `<path>:<line>:
<pkg>.<Qualified> spells the deployment authority outside the entity-ID family (harness-boundaries › No second
spelling of deployment authority)`.

Adopted from PR #48 (`bb004ef`, aliases and file/line from its F11 fix `a8ae8ad`): C-1 is
`TestNoDeploymentAuthorityNames` (lines at this change's `30b9e5f`: `signatures_test.go:410-414`; predicate `:408`,
walk `authorityNameViolations` `:573-654`) with its pair `TestNoDeploymentAuthorityNamesSensitivity` (`:511-559`,
fixture `authorityFixture` `:418-509`). What a caller observes: `TestNoDeploymentAuthorityNames` fails with one such
line per identifier, sorted, and with `checked no package of <module>` when it checked none; it logs the number of
packages it checked; passes on a tree with none.
Proof: the sensitivity test plants twenty names across a public package `pub`, an internal package
`internal/inner` and a `main` package `cmd/tool` — in `pub` a type, a function, a variable, a constant, a struct
field, a method, an exported method of an unexported type, an interface method, alias names and the members of
aliased struct, interface, pointer and outside-module types, and an interface method inherited from an embedded
interface (from outside the module, reported at the alias `Exposed` and the defined `Wraps`; from the module's own
`Inner`, reported once at `Inner`, not again at `Outer` or `OuterAlias`; Codex F3); one function in each of the
other two — beside non-matches (unexported `federationMeta` and `entityIRI`, a lowercase `globalID` field, an
unexported method, an exported `Confederation`, a comment and a string literal naming the words, and a `_test.go` `FederationTestHelper`),
and requires exactly those twenty lines, with the rule's name written from the spec heading rather than taken
from the check's constant, and three checked packages; a module with no package must give the one `checked no
package` line. Task 3.1 added the `Confederation`, comment and literal non-matches (`pub/words.go`), the count, the
empty-module case, the literal rule name and the inherited interface methods; #48's fixture let a case-insensitive
match and an inherited interface method pass.

### D3 C-2: exported `Org`/`Platform` fields outside the owners

Scope: every `*ast.StructType` in every non-test file of every module package, as `contextViolations` walks them
(`context_test.go:256-285`). Match: `field.Exported()` and `field.Name()` is `Org` or `Platform`. Excluded, by exact
spelling: the package paths `<module>/pkg/types`, `<module>/pkg/platform`, `<module>/config`; the two fields of the
type `PlatformMeta` in the top-level package `types` (import path `<module>/types`; not `pkg/types`, not Go's
`go/types`); the field `<module>/processor/rule.CallerContext.Org`; and the carrier, by exact field (Q1, ruled (ii), #72
comment 5969757891): `<module>/component.Dependencies.Platform`, `<module>/service.Dependencies.Platform` and
`<module>/processor/rule.Dependencies.Platform`. A field's type is not consulted: a fourth field of type
`types.PlatformMeta` under either name fails, and a legitimate new holder is a spec edit. The paths are the 04A design's
destinations (D2,
`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:155-178`); none but `pkg/types` and
`pkg/platform` exists at `c64ac338`, and a listed exception that does not exist excludes nothing. Failure line:
`<path>:<line>: field <Name> on <pkg>.<Type> spells the deployment authority outside its owners
(harness-boundaries › No second spelling of deployment authority)`; an anonymous struct is named `struct{…}`.

What a caller observes: `TestNoSecondAuthorityField` fails with one line per field, sorted; passes on `main` and at
`c64ac338`. Proof: `TestNoSecondAuthorityFieldSensitivity` plants, in a fixture module whose path is
`example.com/fixture`: `graph/inference.HierarchyConfig{Org, Platform string}` and `graph/llm.EntityParts{Org,
Platform string}` as at the pin, an anonymous struct with `Org string`, an internal package and a `main` package each
with `Platform string`; and, to pass: `pkg/types.EntityID{Org, Platform}`, `pkg/platform.Config{Org}`,
`config.Config{Platform}`, `types.PlatformMeta{Org, Platform}`, `component.Dependencies{Platform types.PlatformMeta}`
with `PlatformMeta = types.PlatformMeta` aliased, a second `Platform types.PlatformMeta` field on another type in
that package that must fail (the ruled form is by exact field, not by type), `processor/rule.CallerContext{Org
string}` beside a second type in the same package with `Org string` that must fail, `org`/`platform` lowercase fields,
and a `_test.go` with `Org`; and, to fail, `types.Other{Org string}` in the top-level `types` package and a
`Dependencies{Platform types.PlatformMeta}` and a `CallerContext{Org string}` outside the excepted packages (review F2
and F3 at `beb3c3e`, so neither a whole-package `types` exception nor an exception matched without its package
passes); and a function-local `Dependencies` with a `Platform` field in `component`, which must fail (Codex F1 at
`8f27b19`: an exception names a package-scope type). It
requires each failing field by file, line and type, requires silence for each passing one, and requires the exact
count.

### D4 The exceptions live in the spec, mirrored by exact string in the test

As `contextExemptTypes` (`context_test.go:215-217`) mirrors the "No retained context" exception: the test's map is
the spec's list and a new entry is a spec change first. The module path is read from `go.mod` (`modulePathOf`,
`context_test.go:291-304`), so the sensitivity fixture's `example.com/fixture/pkg/types` is excluded the same way
SemEngine's is.

### D5 Order against PR #48

PR #48 merges first; it merged as `deaafd4` on 2026-10-06. C-1 is #48's own test (`signatures_test.go`, adopted
from `bb004ef`), and at `c64ac338` C-1 named seven identifiers #48 was ruled to remove (premise P2); task 3.1 carried
`Hold: until PR #48 merges`. C-2 and the sensitivity tests did not wait. #48 landed C-1's half of the rule as the
requirement "No second spelling of deployment authority"; this change's delta MODIFIES that requirement (D1), so
the archive replaces it with the merged text and `harness-boundaries` holds one requirement for the rule.

### D6 Cost and budget

Each check is one `go/packages` load of the module with types (C-2 also syntax and type info). Budget: the
`contract` package's time in `test:unit`, `test:integration` and `test:repeat` together grows by at most 15 s over
`main`'s figure, measured in task 2.4 as `review-gate-check` task 2.5 measured its own; over budget, the two tests
share one load (the developer's mechanism) before anything is removed.

### D7 Documents changed with the code

`AGENTS.md` rules table, one row (`AGENTS.md:71`): rule "A deployment's authority is spelled once: established by
`config.Manager.Start`, carried as `deps.Platform`, composed only by `FrameworkIdentityFamily.EntityID`; no exported
name in a production package matches `Federation|GlobalID|EntityIRI`, and no second field or envelope copy (#72
ruling A as extended, comment 5969505488; ruling C)" | canonical home `harness-boundaries` spec, "No second spelling
of deployment authority" (PR #48, as this change modifies it); ADR-102, ADR-104 | enforced by
`TestNoDeploymentAuthorityNames` and `TestNoDeploymentAuthorityNamesSensitivity` (`task test:unit`, PR #48) for
exported names; `TestNoSecondAuthorityField` and its sensitivity test (`task test:unit`) for exported
`Org`/`Platform` struct fields outside the packages ruling C allows; lowercase and differently named copies, values
re-joined in a function body, the extract-after-`Start` ordering (ruling B), and the composer being the only
composer are review only. `docs/testing.md:173-181` (the sensitivity-pair list): the two sensitivity pairs.
`docs/repository-map.md:40`: "the deployment-authority spelling" in the contract package's clause.

## PR #48 and the other open pull requests

`gh pr list --state open` → #48 (draft) only. Overlap: `internal/harness/contract/signatures_test.go` (C-1's loader),
`openspec/specs/harness-boundaries` (#48 added the name requirement this change modifies), `AGENTS.md` (#48 edits
other rows). #48 first (D5).

## What the checks do not see

- A copy under another name (`OrgID`, `PlatformID`, `Organization`): `cmd/e2e-semstreams/mission/command.go:52` at
  the pin is the shape. Review only.
- Lowercase `org`/`platform` fields: the carrier copies, by the owner's scope. Review only.
- A pair re-joined inside a function body: `graph/clustering/summarizer.go:728` (#44). Review only; change 4's port
  of that file is where it is met.
- A fourth holder of the carrier is not invisible: by the ruled form (ii) a `Platform` or `Org` field of type
  `types.PlatformMeta` outside the three listed fields fails, and a legitimate new holder is a spec edit first.
- Whether the pair a component holds is the effective one (ruling B, change 3).
- Comments and documents that teach a second meaning (ruling D).

## Invariants and their spec homes

- I1: every exported identifier in a non-test file of a module package whose name contains one of the three words is
  reported exactly once, with its file and line — "Names" paragraph; scenarios 1 and 2.
- I2: every exported `Org`/`Platform` field in a non-test file outside the listed exceptions is reported exactly
  once, with its file, line and type — "Fields" paragraph; scenarios 3 and 4.
- I3: an exception is by exact string and exists only in the requirement — "Fields" paragraph, D4.
- I4: a check that matches nothing fails its sensitivity test — the package's rule (`doc.go:2-4`); every scenario.
- I5: a type-check error in the module fails the test rather than passing it — inherited from
  `publicSignatureViolations` (`loadModuleTypes`, `signatures_test.go:255-267` at `deaafd4`) and `contextViolations` (`context_test.go:233-242`).

## Adopter seam

The surface is two tests; the adopter is a developer porting a package in changes 2–7. (1) Must know: a struct that
holds the pair holds `types.PlatformMeta`, not two strings; nothing exported is named with the three words. (2) Does
nothing: the next `task test:unit` fails with the file, line and rule — not silent. (3) Finds out: typed test
failure, the second-best rank after a compile error. (4) Should have to know: nothing; the gap is what §"What the
checks do not see" lists, which stays with review.

## New surfaces and who uses them

None. Four test functions in a `_test.go` file of an internal harness package; no exported symbol, port, subject,
bucket or config field.

## The adoption sweep (category 5)

The shape is "a static walk over the module's packages that reports a forbidden declaration by file and line, with
exact-name exceptions held in the spec and a planted-violation sensitivity test". Its closest existing instances are
`TestNoRetainedContext` (`internal/harness/contract/context_test.go:22-25`, walk `:221-289`, exceptions `:215-217`,
sensitivity `:27-91`) and, in #48, `TestPublicSignatures` (`signatures_test.go:20-27` at `deaafd4`, walk
`:215-244`, loader `:248-269`, sensitivity `:153-195`), on the helpers `writeTree`, `requireViolation`, `requireNoViolations`
(`repo_test.go:65-81, 104-127`). This design adopts them: C-2 is the context walk with a different predicate, C-1 (as #48
wrote it) a second predicate over the signature loader. It establishes no new pattern, so no sweep is owed.

## Questions for the owner

**Q1 — how C-2 excepts the carrier: ruled.** Asked because ruling C's exclusion list, read literally, passes only
`types.PlatformMeta`'s own two fields and so fails the carrier ruling A names (O4). The options were (i) by type and
(ii) by exact field, recommended; the owner ruled (ii) (#72 comment 5969757891): `component.Dependencies.Platform`,
`service.Dependencies.Platform` and `processor/rule.Dependencies.Platform` (`rule_factory.go:101` at the pin:
`Platform types.PlatformMeta`) are listed by package, type and name, as `CallerContext.Org` is, and both checks read
non-test files only. D3, the delta's exclusion paragraph and its scenario 4 carry that form. No question is open.

One consequence to confirm by reading, not a ruling: C-2 as worded fails `graph/inference.HierarchyConfig.Org/Platform`
when change 2 ports it (premise P3). This design records it for the change-2 architect and does not add the type to
the exclusions; adding it would be a change to this requirement.

## Premises

- P1. On `main` `2ec3bcf` no non-test Go file declares a field named `Org` or `Platform`, and no exported identifier
  contains the three words — `git grep -n -E '^\s+(Org|Platform)\s' -- '*.go' | grep -v _test.go` → 0;
  `git grep -n -E 'Federation|GlobalID|EntityIRI' -- '*.go'` → 0. C-2 starts green; C-1 would too.
- P2. At PR #48's head `c64ac338`: fields named `Org`/`Platform` in non-test files → `pkg/types/entity_id.go:92-93`,
  `pkg/platform/platform.go:30` only (both excluded), so C-2 is green; exported names matching →
  `message/federation.go:22, :36, :50, :60`, `message/base_message.go:81, :91`, `vocabulary/iris.go:85` — seven,
  all ruled removed in #48 (#72 comment 5969505488, items "Ruling 1" and "Ruling 1 extended"). The worktree has
  already dropped `message/federation.go` unpushed. C-1 holds on #48.
- P3. At the pin, port-set fields C-2 fails: `graph/inference/hierarchy.go:100-101` (`Org`/`Platform string`, change
  2) and `graph/llm/prompt_types.go:14-15` (change 2, dormant). Excluded by package (`pkg/types`, `pkg/platform`,
  `config` only): `config/config.go:49`, `config/minimal_config.go:12`, `config/manager.go:96`,
  `pkg/types/entity_id.go:92-93`, `pkg/platform/platform.go:30`. Excluded by exact field (Q1, ruled (ii)):
  `types/component.go:135-136` (`types.PlatformMeta.Org`/`Platform`); the three carrier fields
  `component/dependencies.go:73` (`component.Dependencies.Platform`, of the alias `PlatformMeta` at `:21`),
  `service/dependencies.go:31` (`service.Dependencies.Platform`) and `processor/rule/rule_factory.go:101`
  (`rule.Dependencies.Platform`); and `processor/rule/caller_substitution.go:51` (`rule.CallerContext.Org`). Outside
  the port set: `agentic/*` ten fields, `cmd/e2e-semstreams/mission/command.go:53`, `test/e2e/*` four,
  `processor/agentic-tools/executors/register.go:48` (of the carrier's type, not a listed field, so it would fail).
  Search: `grep -rn --include='*.go' -E '^\s+(Org|Platform)\s+[A-Za-z\[\]\*\.]+' . | grep -v _test.go` over the pin
  snapshot.
- P4. `signatures_test.go` exists in #48 only: `ls internal/harness/contract` on `main` lists fourteen files, none
  of that name; the worktree has it (`internal/harness/contract/signatures_test.go:17`). On `main` since `deaafd4`.
- P5. The sensitivity pattern this design adopts: `context_test.go:27-91`, `signatures_test.go:153-195` (at `deaafd4`),
  `repo_test.go:65-81, 104-127`; `docs/testing.md:155-158` requires it.
- P6. `task test:unit` is `scripts/gopkgs.sh go test -race -count=1 -cpu 1 ./...` (`Taskfile.yml:81`), which runs
  the `contract` package; `task test:repeat` runs it five more times (`AGENTS.md` command table).
- P7. `task spec:check` is `openspec validate --all --strict` (`Taskfile.yml:108`, openspec 1.7.0); a delta with
  `## MODIFIED Requirements` naming the existing heading exactly and carrying the requirement's full text, with six
  scenarios in the `#### Scenario:` form, validates (run on this branch after the merge of `deaafd4`).
- P8. Open pull requests: `gh pr list --state open` → #48 only.

## Declared costs

- Two more module loads per unit run; budget D6.
- Measured (task 2.4, the field check only): the `contract` package's time in `test:unit` + `test:integration` +
  `test:repeat` was 24.614 + 19.482 + 47.322 = 91.4 s on `main` `2ec3bcf` (CI run 37121703748) and 19.556 + 14.228 +
  35.894 = 69.7 s on the branch at `d79c2bd` (CI run 37127911664): no measurable growth, within D6's 15 s. One run
  each; the 22 s difference is runner variance, not a saving. The name check (task 3.1) is not in either figure.
- Change 2 meets C-2 on two types it ports (P3); its design pays for the shape.
- C-1's value is nil until #48 merges; the hold is visible in `task spec:queue`.
- A name-based check: the next copy under a different name is review-only until someone adds a word to the
  requirement.
- The carrier is excepted by exact field (Q1): each later legitimate holder of `deps.Platform` costs one edit to the
  requirement's list before its port passes; the change-6 port of `rule_factory.go:101` is covered by the name
  listed now.
