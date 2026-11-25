# Inventory: rule-core-02-definitions

base: 6671304930fa5e79a3b47c29552209b024f3d624 (claim commit on origin/main da704d0733f5985970b0b516bf04f4c26dcd4774;
draft PR #758). Pin copy: `gh api repos/C360Studio/semstreams/tarball/8b99efe9`, unpacked to a scratch directory
(local only). PR 1 read at `claude/rule-core-01-expression` 71bef230b1c7c66083f7b4d8d75115ac6b52444f (PR #753, draft).
Consumers read by `gh api …/tarball/<sha>`: semteams 4df44ae877d0c711f769ec6b48b88100dbb267d1 (default branch HEAD,
2026-11-10) and a331910ee6ec297f86530c4a1d69ca84001a19f5 (the #26 fixtures); semboids
ab4e0fbc1b382c4fda10bae988662ae0e0bc1e5c (HEAD, 2026-11-11); semsource 34dc2680092d30f8344e3a9af5f81f2a10443f7f
(HEAD, 2026-11-25).

Question: what PR 2 of the rule-core split carries ("`Definition`, `Action` and the action types; the action-family
registry; load-time validation, refusing an unknown action type naming type and file; rule-type registry without
`init`; file loader", `split.md`), against the target-shape page (`archive/2026-11-25-design-rule-core-target-shape/
design.md` §§ 1-2), the owner's shape rulings (PR #752 comment 7162146299, items 1, 2, 6, 9; addendum 7162180652),
the rulings on #9 (comments 7160955176 and 7165028479), and PR 1's exported API. Repositories read
(`docs/inventory-scope.md`): semengine; semstreams at the pin; semteams and semboids at symbol level; semsource for
imports only.

Track: `design` (`proposal.md:5`). Lifecycle owner: none. PR 2 carries no `Start`/`Stop`, so the lifecycle suite does
not apply.

## 1. Size (measured, `wc -l` at the pin)

The split plan's PR 2 file list, whole files: production 4,082, test 1,035 (archived `inventory.md` § Size,
re-measured here and unchanged).

| File (pin) | Lines | Range PR 2 is assigned (split inventory) | Range size |
| --- | --- | --- | --- |
| `rule_factory.go` | 290 | whole | 290 |
| `actions.go` | 2,339 | `:1-535` (constants `:31-76`, `Action` `:89-444`, `ParseTTL` `:449-465`, interfaces `:472-532`) | 535 |
| `config_validation.go` | 658 | `:256-559`, `:586-641` (PR 12 takes `*Processor` methods `:18-255`, `:561-584`, `:642-658`; PR 5 takes `:382-387`) | 304 + 56 |
| `action_id.go` | 133 | whole | 133 |
| `interfaces.go` | 51 | whole | 51 |
| `doc.go` | 142 | whole | 142 |
| `entity_pattern_contract.go` | 121 | whole | 121 |
| `schema.go` | 85 | whole | 85 |
| `status.go` | 21 | whole | 21 |
| `test_rule_factory.go` | 219 | whole (moves to a test file) | 219 |
| `user_response_subject_reservation.go` | 23 | whole | 23 |
| `rule_loader.go` | 273 | **not assigned to PR 2 by the split inventory (listed under PR 8)**; the file read and decode is `:55-99` | 45 |

Tests: `action_id_test.go` 137, `config_test_helpers_test.go` 12, `config_validation_test.go` 493,
`entity_pattern_contract_test.go` 100, `predicate_authoring_test.go` 14, `rule_id_token_test.go` 29, `schema_test.go`
154, `user_response_subject_reservation_test.go` 96 (total 1,035). Not counted in either column:
`docs/custom-rules.md` (620). `split.md` assigns it to PR 2; the target-shape page § 1 says it is not carried.

Two discrepancies between the plan documents, for the design to settle:

- `split.md` puts the "file loader" in PR 2, but the split inventory lists `rule_loader.go` under PR 8.
- `split.md` gives PR 2 "~1,741 + 1,254" lines. The measured file list is 4,082 + 1,035, and the ranges above sum to
  2,025 production lines with `rule_loader.go:55-99` included (290 + 535 + 304 + 56 + 133 + 51 + 142 + 121 + 85 +
  21 + 219 + 23 + 45).

## 2. Pin probe (the PR 2 test files on the pin copy)

The 34 `Test*` functions in `action_id_test.go`, `config_validation_test.go`, `entity_pattern_contract_test.go`,
`rule_id_token_test.go`, `schema_test.go` and `user_response_subject_reservation_test.go` were run on the pin copy
(`go test -run '^(…)$' .` in `processor/rule`, go1.26.11 darwin/arm64):

- once with `-race`: `ok … 1.447s`;
- five times with `-cpu=1 -shuffle=on`: `ok … 0.303s`, seed `1795617175502578000`;
- 34 `--- PASS`.

Test-text rules across the 8 PR 2 test files: `time.Sleep` 0, `t.Skip` 0, `//go:build` 0.

The whole package was probed by the split plan (`ok` under both runs; 53 sleeps in 12 files, none in PR 2's files).
Context retention in the carried production files: 0. The one `context.Context` hit, `actions.go:732`, is a parameter.
`context.Background`, `context.TODO` and `WithoutCancel` in the carried files: 0.

Test-side compile drift (imports and helpers the PR 2 tests need that main or PR 2 does not provide):

- `config_validation_test.go` imports `internal/semantictest`. On main that package is `internal/harness/semantictest`
  (ledger `:317-323`).
- `config_validation_test.go:113` tests `(*Processor).validateExpressionRule`, which is PR 12. Its lines `:251-459`
  test `run_scope`, `filesystem_policy` and `related_loops`, which belong to `publish_agent` and are not carried.
  Its lines `:26-111` test `definitionFromMap`, the map round trip ruling 6 removes.
- `config_test_helpers_test.go:5` calls `NewConfig`, which is PR 8.
- `schema_test.go` imports `github.com/xeipuuv/gojsonschema` (not in `go.mod`: `grep -n xeipuuv go.mod` returns
  nothing). It tests `buildRuleProcessorSchema`, which is the processor `Config` schema, not a rule-type schema.
- `entity_pattern_contract_test.go:64` and `:83` build a `Processor` with `entityWatcherMap` (PR 10).
- `user_response_subject_reservation_test.go:55` drives the `ActionExecutor` (PR 4).
- `predicate_authoring_test.go:8` — `func init() {`: a test file registers vocabulary predicates in `init`.
- The rule type `"test_rule"` exists only through the production `init` at `test_rule_factory.go:212` (type name at
  `:163`, `:196`). Two counts:
  - Test files naming `TestRuleFactory` or `NewTestRule`: 3 — `entity_watcher_atomic_bootstrap_test.go`,
    `graph_event_identity_test.go`, `runtime_config_test.go`.
  - Test files using the string `test_rule` (`grep -l test_rule *_test.go`): 13 —
    `action_failure_metrics_test.go`, `actions_reconcile_test.go`, `config_projection_test.go`,
    `config_validation_test.go`, `actions_test.go`, `factory_integration_test.go`,
    `entity_watcher_atomic_bootstrap_test.go`, `graph_event_identity_test.go`, `fire_every_n_events_test.go`,
    `projection_derivation_test.go`, `runtime_config_test.go`, `runtime_config_lifecycle_test.go`,
    `rule_integration_test.go`.
  - Of the 13, only `config_validation_test.go` is a PR 2 test file. The rest belong to PRs 4, 5, 7, 8, 9, 10 and 12.

## 3. Category 1: the claimed gap

- **"Refuse an unknown action type at load."** At the pin no load path refuses one. The only refusal is at fire:
  - `actions.go:943` — `default:`
  - `actions.go:944` — `return fmt.Errorf("unknown action type: %s", action.Type)`

  Searched production files for `unknown action|unsupported action|DisallowUnknownFields`; the only hit is
  `actions.go:944`. `ValidateDefinition` (`config_validation.go:273-292`) never reads `Type`.

  Consumer instance: semteams' `replace_owned` type, 9 actions in 5 files under `configs/rules/dev-via-test/` (02c,
  02d, 07a, 07c, 07d). The pin has no Go symbol for it (`grep -rn replace_owned --include=*.go` over the pin: 0). The
  pin's ADRs record it as retired: ADR-046 `:15` ("its historical `replace_owned` claim/marker mechanism is retired"),
  `:19`; ADR-059 `:21`.

  None of the 5 files is in a checked-in flow's `rules_files`. The 49 listed paths cover agent-run 15, autoresearch
  20, coordinator 2, ops 1 and research 11. semteams tests read them through a local JSON type, not through the rule
  engine: `test/contract/dev_via_test_pack_test.go:31` — `const devViaTestPackDir = "../../configs/rules/dev-via-test"`;
  type at `:1610-1615`; assertions at `:531-539`.
- **"A refusal fails the load and is never logged and skipped"** (the ruling on epic 9, comment 7165028479). At the
  pin these load sites log and skip. None increments a metric: `grep -n -i metric rule_loader.go
  kv_config_integration.go` finds only the `activeRules` gauge set to the loaded count at `rule_loader.go:267-270`.
  - `rule_loader.go:207-213`: `NewCronRule` fails; `rp.logger.Error("Failed to create cron rule from definition", …)`,
    then `continue` (`:212`).
  - `rule_loader.go:226-232`: `CreateRuleFromDefinition` fails; `rp.logger.Error("Failed to create rule from
    definition", …)`, then `:232` — `continue // Skip invalid rules but continue loading others`. Refusals that reach
    this site:
    - `ValidateDefinition` (`rule_factory.go:196`) — but `prepareInitialRules` has already returned that error, which
      fails the load (`rule_loader.go:116-119`);
    - pack ID (`rule_factory.go:199` — `if err := validatePackID(deps.PackID); err != nil {`);
    - unknown rule type (`:202-205` — `return nil, fmt.Errorf("no factory registered for rule type: %s", def.Type)`);
    - `ExpressionRuleFactory.Validate` (`:208-210`; checks at `expression_factory.go:395-430`).
  - `kv_config_integration.go:362-377` (`ListRules`): a KV entry that cannot be read (`:366-370`) or decoded
    (`:372-376`) is logged at Warn ("…; skipping") and left out.
  - `kv_config_integration.go:239-251` (`reconcile`'s round trip): a definition that cannot be marshalled or
    unmarshalled is logged at Warn and left out.
  - Consequence for both KV sites: `reconcile` passes the result as the complete rule set
    (`kv_config_integration.go:228-229` — "applyRuleChanges removes any rule absent from the update map, so we always
    pass the complete ruleset"). A KV rule skipped there is removed from the running processor, not refused.
  - Disabled rules are skipped by design, not as a refusal: `rule_loader.go:185-188`, Debug log.
- **"Name the file."** No pin refusal of a single rule names its file:
  - `rule_loader.go:117` — `if err := ValidateDefinition(definition); err != nil {` (message names the rule ID only,
    `:118`)
  - `config.go:168` — `for index, definition := range c.InlineRules {` (names `inline_rules[%d]`, `:170`)
  - `rule_loader.go:65` and `:76` name the file only for read and parse failures.
- **"Rule-type registry without `init`."** Two production `init` registrations:
  - `expression_factory.go:443` — `func init() {` (calls `RegisterRuleFactory("expression", factory)` at `:445`)
  - `test_rule_factory.go:212` — `func init() {`

  Cron bypasses the registry: `rule_loader.go:206` — `if def.Type == CronRuleType {`; `config_validation.go:243-249`.
- **"Action-family registry."** There is none at the pin; the closed switch is `actions.go:916-945`. Main has no
  `processor/rule` package (`git ls-tree origin/main processor/rule`: absent; only `processor/rule/expression` exists
  on PR #753).

## 4. Category 2: every current spelling

### a. The action type set

| Spelling | Location | Members |
| --- | --- | --- |
| Constants | `actions.go:31-76` | 12 types |
| Dispatch switch | `actions.go:916-945` | 12 cases |
| `isArbitrarySubjectPublisher` | `user_response_subject_reservation.go:16-23` | `publish`, `publish_agent`, `approve` |
| `isTripleMutationAction` | `config_validation.go:436-443` | add, remove, update triple |
| `actionDistinguisher` | `action_id.go:89-106` | 5 case clauses covering 8 types |
| Reconcile selector | `projection_derivation.go:66`, `processor.go:1513` | `reconcile_predicates` |
| Lifecycle actions | `actions_lifecycle.go:36`, `:101`, `:129` | the three `lifecycle_*` |
| Metric label | `metrics.go:33-38` | the "closed set" `action_type` label |
| Docs | `docs/actions.md` | (doc) |

### b. Decoding a rule definition (six paths)

1. **Files:** `rule_loader.go:72` (array) and `:75` (single). Plain `json.Unmarshal`, unknown keys accepted.
2. **Inline rules:** through `Config.UnmarshalJSON`, `config.go:106-140`. Not strict.
3. **KV map:** `definitionFromMap`, `config_validation.go:449-559`, field by field with its own defaults.
4. **KV value:** `kv_config_integration.go:348` (`GetRule`) and `:373` (`ListRules`); `json.Unmarshal` into
   `Definition`.
5. **Copy by JSON round trip:** `cloneRuleDefinitions`, `rule_loader.go:89-99`.
6. **Back to a map:** `definitionToRuleMap`, `rule_loader.go:39-53`, and the reconcile path,
   `kv_config_integration.go:236-251` (marshal, then unmarshal into a map).

Spellings 1 and 3 differ in their defaults when a key is absent:

| Key | Files (path 1) | KV map (path 3) |
| --- | --- | --- |
| `enabled` | `false` (rule skipped at `rule_loader.go:185` — `if !def.Enabled {`) | `true` (`config_validation.go:463` — `Enabled: getBoolWithDefault(ruleMap, "enabled", true),`) |
| condition `required` | `false` | `true` (`config_validation.go:611` — `Required: getBoolWithDefault(condMap, "required", true),`) |
| `name` | `""` | the rule ID |
| `logic` | `""` | `"and"` |

Path 3 drops a malformed action list silently. `config_validation.go:513` — `parseActions := func(key string)
[]Action {` returns `nil` on a marshal error (`:519`) or an unmarshal error (`:524`), and the rule then loads with no
actions.

### c. Definition validation (five places)

- `ValidateDefinition`, `config_validation.go:273-292`. Called from `rule_factory.go:196`, `config.go:169`,
  `rule_loader.go:117`, `config_validation.go:554` and `kv_config_integration.go:316`.
- `BaseRuleFactory.ValidateExpression`, `rule_factory.go:240-271`. No production caller pin-wide: `grep -rn -E
  'ValidateExpression\(|NewBaseRuleFactory|BaseRuleFactory'` outside `rule_factory.go` finds only
  `processor/rule/docs/custom-rules.md` (8 lines). It holds the only `rerun_on_recovery` check for non-cron rules:
  `rule_factory.go:266` — `if def.RerunOnRecovery && len(def.OnEnter) == 0 && len(def.OnRecovery) == 0 {`. Cron
  rules refuse the key outright: `cron_rule.go:176-177` — `if def.RerunOnRecovery {` /
  `rejected = append(rejected, "rerun_on_recovery")`. The target-shape page § 1 cites `rule_factory.go:240-290` as
  "the factory's operator and condition checks".
- `ExpressionRuleFactory.Validate`, `expression_factory.go:395-430` (PR 6 file). The live operator, condition and logic
  check, plus cooldown parse (`:422` — `if def.Cooldown != "" {`).
- `validateExpressionRule`, `config_validation.go:161-238`. The hot-reload map copy of the operator, logic and
  rule-opaque checks.
- `validateCronDefinition` (PR 11, `cron_rule.go:61-62` and `:170-195`).

### d. Walks over a rule's action lists

- `config_validation.go:422` — `for label, actions := range map[string][]Action{`. Ranges a Go map, so with two bad
  lists the reported one varies between runs.
- `projection_derivation.go:56-62` (a slice).

### e. The firing-cap key

The key is computed at `action_id.go:37-68` (`effectiveID` and `fingerprint`, over rule ID, type, distinguisher and
role) and `:89-106`. The cap default is `action_id.go:17` — `const DefaultActionMaxIterations = 3`.

Three production readers at the pin:

1. The cap gate: `stateful_evaluator.go:406` — `actionID := action.effectiveID(ruleDef.ID)`; counts read at `:408`
   and written at `:423`.
2. Stored state: the count is kept in `RULE_STATE` as `MatchState.ActionIterations`. `state_tracker.go:53-62` —
   "Keys are Action.effectiveID(rule_id)"; field `:62` —
   ``ActionIterations map[string]int `json:"action_iterations,omitempty"` ``.
3. The reconcile request ID: `actions.go:1394-1400` builds `requestID := fmt.Sprintf("rule:%s:%s:%d:%s", ruleID,
   entityID, iteration, action.effectiveID(ruleID))`. `:1407` sends it as `MutationMetadata.RequestID`.

On main, that request ID becomes each written statement's `Context`: `pkg/projection/mutation_client.go:377-378` —
`if triple.Context == "" {` / `triple.Context = metadata.RequestID`. `:365-367` refuses a triple whose `Context`
differs from it. Main requires a request ID only for Create and Append (`:343-347`; `mutation_types.go:21-22`:
"Reconcile requires Source"). ADR-111 Decision 5 (`docs/adr/111-…md:114-122`) says a redelivered effect "must be
idempotent or carry a stable key". So the reconcile request ID is built from the key PR 2 redefines.

The pin keeps the key stable across edits that do not change an action's shape. `action_id_test.go:63` —
`func TestFingerprint_StableAcrossUnrelatedFieldChanges(t *testing.T) {` asserts that a `Prompt` edit keeps the key.
The target-shape page § 2 keys the cap on a canonical-form hash of the whole `Body`, so a `Prompt` edit changes it.
The new key changes all three readers: the cap count, the stored `action_iterations` keys and the reconcile request
ID. Pin ADR-045 relies on the per-action cap as its loop bound (`045-…md:26`, `:270-271`, `:355-356`).

### f. The ADR-093 reserved-subject check

`config_validation.go:340` (load) and `user_response_subject_reservation.go:5-23`. The fire-time check is in PR 4's
code (`user_response_subject_reservation_test.go:55`).

### g. Rule-level `max_iterations`

- `rule_factory.go:39-42` documents "limits how many times a rule can enter the matching state".
- The only production readers copy the value: `stateful_evaluator.go:205` (into `MatchState`) and `:222`
  (`$state.max_iterations`), plus `cron_rule.go:179` (refused on cron rules).
- No gate (`grep -n Iteration stateful_evaluator.go`: no comparison against `Rule.MaxIterations`).

### h. Rule `metadata`

- Only reader: `expression_factory.go:345` — `for k, v := range r.metadata {`, inside the dropped graph-event path.
- `cron_rule.go:122` — `func (r *CronRule) Metadata() map[string]any { return r.metadata }` has no production caller
  (`grep -rn '\.Metadata()' processor/rule`: definition only).
- `config_validation.go:539-541` claims "provenance carried through to action substitution". `execution_context.go`
  has no metadata substitution (`grep -n metadata execution_context.go`: 0).

### i. Who holds Platform and PackID for a rule

- `rule_factory.go:101` — `Platform types.PlatformMeta` (field of `Dependencies`, `:94-102`).
- Writers of `Dependencies`: `rule_loader.go:176-181` (`ruleDeps := Dependencies{`) and `config_validation.go:570`
  (`deps := Dependencies{`). Both fill `NATSClient`, `Logger`, `PackID` and `Platform`.
- Readers:
  - `rule_factory.go:199` reads `PackID` (`validatePackID(deps.PackID)`).
  - `ExpressionRuleFactory.Create` (`expression_factory.go:387`) passes `Platform` and `PackID` to
    `NewExpressionRule`. Inside, they are read by `validatePackID` (`:87`) and by `ruleTriggerEntityID` (`:349`),
    which the graph-event drop removes.
  - `TestRuleFactory.Create` (`test_rule_factory.go:184`) passes both to `NewTestRule`.
  - `NATSClient` and `Logger` are read by no `Create`.
- Pack ID is also checked independently of `Dependencies`: `config.go:162` — `if err := validatePackID(c.PackID); err
  != nil {` in `Config.Validate` (PR 8).
- Main's guard excepts the field by name: `internal/harness/contract/authority_test.go:232` —
  `"processor/rule.Dependencies.Platform": "the carrier deps.Platform",`; `openspec/specs/harness-boundaries/
  spec.md:818-820`. The guard only looks exceptions up (`authority_test.go:325`). Nothing checks that each exception
  still names a declared field (`grep -n -i 'unused\|stale' authority_test.go`: 0), so the exception stays and passes
  once the type is gone.

## 5. Category 3: adjacent claims

### Open PRs

`gh pr list --state open`, each file list read with `gh api --paginate …/files`:

- **#753** (draft, `claude/rule-core-01-expression` 71bef230, 28 files): `processor/rule/expression/*` (12),
  `processor/rule/docs/conditions.md`, `docs/admission-ledger.yaml` (adds rows for `processor/rule/expression`, ADR-041
  and `conditions.md` at the end), `docs/adr/041-unified-condition-evaluator.md`, `docs/repository-map.md`,
  `scripts/cover-check.sh`, `internal/harness/toolcheck/cover_test.go`, and
  `openspec/changes/rule-core-01-expression/**` including a new capability `specs/rule-conditions/spec.md`. Overlap:
  PR 2 builds on the `processor/rule` directory and edits the same ledger, repository map and coverage list. `split.md`
  makes PR 2 wait for PR 1.
- **#758** (this claim): `openspec/changes/rule-core-02-definitions/` only.

### PR 1's API that PR 2 depends on (at 71bef230)

- `processor/rule/expression/regex.go:38` — `func Compile(conds []ConditionExpression) error {`. Compiles in place onto
  the unexported `pattern` field (`types.go:31-33`). "A JSON copy of a condition does not carry the compiled pattern"
  (`regex.go:36-37`). Refusals are classified invalid, naming index and field (`:46-47`).
- `regex.go:21` — `func SubstitutionPrefixes() []string {` (PR 3 reads it).
- `evaluator.go:57` — `func RegisteredOperators() map[string]struct{} {` (the pin's `isValidOperator` reads it,
  `rule_factory.go:287-290`).
- `stateless.go:54` — `func EnsureStatelessResolvable(…)` (PR 6).
- `types.go:12` — `type ConditionExpression struct {` with `Required bool \`json:"required"\`` (no default-true).
- PR 1's design, § Seam (lines 76-88 of `design.md` at 71bef230): `Load(def, file, handlers)` is the one caller of
  `Compile`. It compiles conditions and each action's `when`, wraps refusals with rule ID, file and position (e.g.
  `on_enter[1].when[0]`), and "A refusal fails the load, never a log-and-skip". "PR 2 decides what `file` names for a
  rule from the KV bucket (key and revision)."
- PR 1's design, D2 option C: "a decoder's unknown-key refusal stops at a custom unmarshaler (probe)".
- PR 1's design, risks (line 113): "Later tests comparing conditions with `assert.Equal` see the compiled pattern
  (MEDIUM, PRs 2, 6)."

### Specs and the ledger

- `openspec/specs/component-registration/spec.md:18-21` (packages export `Register`; no aggregation) and
  `internal/harness/contract/boundaries_test.go:40` (`TestNoAggregatorPackage`). No structural guard catches a factory
  map filled in production `init`, so the AGENTS.md rule "imports register nothing" is review-only for that shape.
- `openspec/specs/harness-boundaries/spec.md:735` and `:818-825` (the `Dependencies.Platform` exception).
- `openspec/specs/rule-pack-identity/spec.md:12`, `:28`, `:44`: `pkg/rulepack.ValidateID`, which
  `validatePackID` wraps (`config.go:176-178`).
- Ledger: no `processor/rule` row on main (`git grep -n processor/rule docs/admission-ledger.yaml` gives cross-reference
  prose only: `:1863`, `:1894`, `:1939`, `:2955`, `:3028`, `:3033`, `:3054`). The pin's `pkg/rulepack` row
  (`:3028-3060`) names `processor/rule` as its reader.

### Shared helpers present on main

- `natsclient/kv_key_contract.go:94` (`ValidateKVLiteralToken`)
- `vocabulary/namespace_authority.go:30` (`RequireDeclaredPredicate`)
- `vocabulary/registry.go:522` (`IsRuleOpaque`)
- `pkg/errs/errs.go:384` (`WrapInvalid`) and `:310` (`ClassifiedCodeDetail`)
- `pkg/types/entity_id.go:160` (`ValidateEntityIDPattern`)
- `component/schema_tags.go:366` (`GenerateConfigSchema`)

Absent on main (agentic, not carried): `LineageTriplePredicate`, `IsKnownFilesystemPolicy`; 0 hits for
`git grep -n -E 'func (LineageTriplePredicate|IsKnownFilesystemPolicy)\('`.

### Issues

Listed in § 9.

## 6. Category 4: who each named surface is for

These are the target-shape page's names; the page says "PR 2's Fable read settles names". Pin consumers at symbol
level, measured by import alias then `grep -o '<alias>\.[A-Z]\w*'` per file:

| Symbol | Who uses it now |
| --- | --- |
| `Definition` | semteams: 10 test files, and production through `ConfigManager` (`cmd/semteams/main.go:142`, `rule_config_service.go:33-92`; PR 12); 6 `Definition{` literals in tests. Aliases: `rulepkg` in 9 files, `rule` in 6 (of the 15 importing files). semboids: none. |
| `Action` with typed fields | semteams: 6 test files that import `processor/rule` match a typed-field selector (upper bound; the grep counts any `.Subject`, `.Role`, … in those files): `autoresearch_reply_integration_test.go` 16, `research_loop_budget_test.go` 13, `loop_runtime_integration_test.go` 13, `program_pulse_contract_test.go` 8, `research_loop_budget_integration_test.go` 6, `rule_family_consumer_test.go` 5. Other semteams pack tests read rule JSON through local types (e.g. `dev_via_test_pack_test.go:1610-1615`). No semteams production file reads them. 3 `[]rule.Action{` literals in tests (`research_loop_budget_integration_test.go:205`, `:272`; `research_loop_budget_test.go:428`). |
| `ActionTypePublishAgent` | 9 semteams test references in 5 files |
| `ActionTypeReconcilePredicates` | 1 semteams test reference |
| `CronRuleType`, `NewCronRule` | semteams tests (PR 11) |
| `NewActionExecutor*`, `ExecutionContext`, `LifecycleManager` | semteams tests (PRs 3, 4) |
| `rule.Register` | semteams `internal/runtimecatalog/components.go:36`; semboids `componentregistry/register.go:23` (PR 8) |
| `GetRuleSchemas`, `GetRegisteredRuleTypes`, `RegisterRuleFactory`, `CreateRuleFromDefinition`, `ValidateDefinition`, `BaseRuleFactory`, `TestRuleFactory`, `DefaultActionMaxIterations`, `EntityStateEvaluator`, `ErrorCodeEntityWatchBucketUnsupported`, `entity_watch_bucket_unsupported`, `Status`, `rule.Rule`, `rule.Event` | 0 hits in semteams 4df44ae8, semboids ab4e0fbc or semsource 34dc2680 (the search the target-shape ruling 2 asks for) |
| `GetRuleSchemas` at the pin | no caller outside `rule_factory.go` |
| semsource | imports nothing from `processor/rule`; its 6 text hits are docs and manifests |

`Status` (`status.go:6`) is read only by `Processor.DebugStatus` (`processor.go:1475`, `:1496`), which is PR 8.
`schema.go:11` (`buildRuleProcessorSchema`) is read only by `processor.go:34` — `var schema =
buildRuleProcessorSchema()` (PR 8).

New in the target shape, with no pin equivalent (pin evidence for who would call each):

- `DecodeDefinition`: the six decode paths in § 4b.
- `Load`: callers listed in the target-shape page (`rule_loader.go:117`, `config.go:169`,
  `kv_config_integration.go:316`).
- `ActionHandler.Check`/`Run`: a product's `publish_agent`. semteams has no handler type yet; its tests drive
  `NewActionExecutorFull`.
- `LoadedAction`.
- `Action.Body`.

`ActionHandler.Run` names `*ExecutionContext` (target-shape page § 2). `execution_context.go` (846 lines) is PR 3's
file (`split.md`).

## 7. Category 5: problem shapes and their nearest instances

- **A registry a product extends from `main`:** `processor/graph-embedding/component.go:220-240` (`Register` /
  `RegisterWithEmbedder`, refusing an empty name or nil constructor with `errs.WrapInvalid`). Per-instance registries:
  `component/registry.go:128` (refuses a duplicate name, `:150`), `payloadregistry/registry.go:98`,
  `pkg/lifecycle/manager.go:128`. Process-global with registration in `init`: `vocabulary/registry.go:301`,
  `vocabulary/pack.go:237`, and the `init` registrations in `vocabulary/{hierarchy,identity,labels,lifecycle,
  relationships}.go`.
- **Admit or refuse at decode, naming the unknown key:** `processor/graph-embedding/component.go:132`,
  `processor/graph-clustering/config.go:182`, `gateway/graph-gateway/config.go:116-120`, `internal/cache/config.go:259`,
  `pkg/lifecycle/manager.go:1009-1015`, `component/port_codec.go:217` (all `DisallowUnknownFields`). The pin's rule
  decoders are all non-strict (§ 4b).
- **A content-addressed key over canonical JSON:** `message/triple_identity.go:117-135` (`canonicalObjectKey`: decode to
  `any` and re-marshal, so map keys sort, with default HTML escaping). `SetEscapeHTML(false)` on main only at
  `vocabulary/export/jsonld.go:26`. No canonical-JSON hash helper on main (`git grep -n -i 'canonical json\|
  canonicalJSON'`: test helpers only).
- **Byte preservation:** probe program `/tmp/arch-scripts/rawprobe/main.go` (local only), go1.26.11, over semteams
  a331910e `test/fixtures/rule-family-consumer/{forced-function,required-live,response-format}.json`:
  - Unmarshal of each `on_enter[0]` into `json.RawMessage` is a verbatim substring of the file (326 B, 1,484 B,
    1,288 B).
  - `json.Marshal` of that `RawMessage` equals `json.Compact` of it (246 B, 1,321 B, 635 B), not the source bytes.
  - ``json.Marshal(json.RawMessage(`{"a": "<x>&"}`))`` gives `{"a":"\u003cx\u003e\u0026"}`.
  - The fixtures contain no `<`, `>` or `&` (`grep -c '[<>&]'`: 0 each). `required-live.json` has one line with a
    non-ASCII byte.
  - semteams' own decode test (`test/contract/rule_family_consumer_test.go`, `TestRuleFamilyDecode`) compares decoded
    values against `expected.json`, not bytes. The fixtures differ between a331910e and 4df44ae8 only in
    `provenance.json` (`diff -r`).

## 8. Category 6: the pin's decision records for the carried files

Search: every record at `docs/adr` of the pin (85 files), `grep -l` for the carried file paths and exported names
(`RegisterRuleFactory`, `ValidateDefinition`, `effectiveID`, `DefaultActionMaxIterations`, `max_iterations`,
`user.response.>`, `RuleOpaque`, `rules_files`, `fire_every_n_events`), plus `grep -o 'ADR-[0-9]{3}'` per carried
file, plus records read by title.

| ADR | Title (pin) | What it decides for PR 2's code | Cited at |
| --- | --- | --- | --- |
| 031 | Time-Trigger Primitive for Reactive Rules | `type: "cron"`, `Schedule`, `Actions`, `fire_every_n_events` on cron | `rule_factory.go:56-69`; ADR `:308` |
| 036 | Agent-Private Observable State | conditions on rule-opaque predicates are refused | `config_validation.go:298-317`, `:210-217` |
| 041 | Unified Condition Evaluator | `Conditions` and `When` share one evaluator | `actions.go:295`; `test_rule_factory.go`; ported by #753 |
| 046 | Parallel Fan-Out (`for_each`) | `ForEach`/`ForEachVar` fields | `actions.go:411-443` (publish_agent only, not carried, E3) |
| 047 | Lifecycle Harness Substrate | `lifecycle_*` constants and the `LifecycleManager` interface | `actions.go:62-76`, `:505-532` (PR 5 uses them) |
| 039 | Tool-Call Governance is Rule-Driven | deny/approve constants, `VerdictAuditor` | `actions.go:51-61`, `:497-503`; `interfaces.go:28-49` (ADR `:566`); not carried (E4, #28) |
| 023, 034, 053, 067 | ToolChoice; `response_format`; agent-run; read-only policy | agentic `Action` fields and their checks | `actions.go:141-278`; `config_validation.go:330-335`, `:346-367`, `:394-412`; not carried (E2/E3) |
| 055 | Write-Intent Taxonomy | the retired deny/approve audit triples (`actions.go:78-84`) | comment only |
| 074 | Canonical Predicate Contract (in SemEngine `docs/adr`) | an action predicate must be a declared literal | `config_validation.go:370-381`; SemEngine ADR-074 `:72-73` |
| 093 | Typed User-Response Subject Ownership | arbitrary-subject actions may not target `user.response.>` | `user_response_subject_reservation.go`; `config_validation.go:340-345` |
| 094 | Boot-Only Composition and Observable Rule Activation (in SemEngine) | "A rejected candidate leaves the previous generation unchanged"; outcomes `applied`, `rejected`, `superseded`, `canceled_shutdown` | SemEngine `docs/adr/094-…md:94-106` |
| 102, 104 | Entity-ID positions; unique platform authority (in SemEngine) | `Dependencies.Platform` is the carrier | `rule_factory.go:97-101`; `authority_test.go:232` |
| 111 | Durable Execution Primitive (SemEngine) | Decision 5: recovery relies on `OnRecovery`/`RerunOnRecovery` | SemEngine `docs/adr/111-…md:114-122` |
| 076 | Framework-derived identity for rule events | decisions 1-3 and 5 not ported (owner, #9 comment 7160955176) | `Dependencies.PackID`/`Platform` feed it |
| 046, 059 | Parallel fan-out; Epistemic trust tier | record `replace_owned` as retired and superseded | ADR-046 `:15`, `:19`; ADR-059 `:21` |
| 045 | Graph Search Decomp+Fusion via Rule-Chain | its loops rely on the per-action `MaxIterations` cap | ADR-045 `:26`, `:270-271`, `:355-356` |

None found for `action_id.go`, `status.go`, `schema.go`, `entity_pattern_contract.go` or `rule_loader.go` beyond the
rows above. `rule_loader.go` cites ADR-047 once (`:235`, PR 8 code).

## 9. Open issues against `processor/rule`

`gh issue list --state open --search "rule" --limit 200`, every hit read. Hits that name the package:

| Issue | What it names | Note |
| --- | --- | --- |
| #25 | `actions.go:916-945` | ruled: one handler map, comment 7162378515; retires at PR 7 per comment 7160976281 |
| #26 | `actions.go:191`, `:206` | retires at PR 2 per comment 7160976896; fixtures at a331910e, comment 5971046097 |
| #27 | `config_validation.go:346,358,396,407`, `actions.go:542,769` | the publish_agent checks inside PR 2's `config_validation.go` range |
| #28 | `actions.go:497-503` | `VerdictAuditor`, inside PR 2's range |
| #294 | `config_validation.go:382-385` | moves to PR 5 |
| #702 | `action_id.go:17` | `DefaultActionMaxIterations` |
| #703 | `cron_rule.go` | wants `deadline` and `on_deadline` rule keys (PR 14) |
| #714 | (skill, PR 13) | |
| #196 | `processor.go:102`, `cron_scheduler.go:54` | |
| #195 | (adoption list) | |
| #350 | (constraint model, PR 5) | |
| #704, #24, #9 | (epics and decisions) | |

Issues #756 and #757 name `processor/rule/expression` (PR 1's package, `status:blocked` on #753), not a PR 2 file. #757
says
"Rules decoded from JSON are not affected."

Names the word, not the package (issue numbers): 730, 712, 141, 110, 109, 635, 155, 554, 614, 84, 284, 734, 535,
665, 134, 484, 60, 300, 683, 576, 111.

## 10. Shape sweep (pin copy, with each disposition basis)

### Second spellings

- **F1.** **Six decode paths with divergent defaults** (§ 4b): `rule_loader.go:72`, `config_validation.go:449`,
  `kv_config_integration.go:373`, `rule_loader.go:39`, `rule_loader.go:89`, `kv_config_integration.go:236-251`.
  *Required:* owner ruling 6 (one decode spelling) and the target-shape page § 1. `config_validation.go:513-528`
  (an action list dropped on error) is also silent loss, an admission gate.
- **F2.** **Five homes for definition checks** (§ 4c), one of them dead (`rule_factory.go:240-271`).
  *Required* where ruling 6 and the target-shape page § 1 put them in `Load`. Which copy is authoritative is a fact
  for the design: the live one is `expression_factory.go:395-430`. The non-cron `rerun_on_recovery` check is only in
  the dead one (`rule_factory.go:266`); cron refuses the key (`cron_rule.go:176-177`).
- **F3.** **Nine spellings of the action type set** (§ 4a). *Required* for the dispatch switch, the type constants'
  closedness, and `isArbitrarySubjectPublisher`'s agentic members (ruling 1; the target-shape page § 2).
  `actionDistinguisher` is replaced by the page's canonical-body key. The metric label is PR 4/8's.

### Copied shells

None in PR 2's files. There is no lifecycle code.

### Layered interfaces with one implementation

- **F4.** `Factory` (`rule_factory.go:79-91`) has one production implementer, `ExpressionRuleFactory`, plus the test
  factory. `Rule` (`interfaces.go:30-42`), `Event` (`:14-26`) and `EntityStateEvaluator` (`:47-51`) belong to the
  graph-event and factory model. *Required (not carried):* ruling 2, owner decision 1, the target-shape page § 1.

### Exported surface with no reader

- **F5.** `UnregisterRuleFactory` (`rule_factory.go:144`, "primarily for testing"), `GetRuleSchemas` (`:183`, 0 readers
  pin- and product-wide), `GetRegisteredRuleTypes` (`:171`), `Schema`/`Example` (`:105-121`), `BaseRuleFactory`
  (`:217-271`). *Required (not carried):* ruling 2.
- **F6.** `test_rule_factory.go` (219 lines), production code read only by tests and registered in `init` (`:212`).
  *Required:* no back doors; imports register nothing.
- **F7.** `Dependencies` (`rule_factory.go:94-102`): after decision 1, no reader of `Platform`/`PackID` (§ 4i);
  `NATSClient` and `Logger` are not read by `Create` (`expression_factory.go:386-392`). *Required (not carried with
  `Factory`, target-shape page § 1).* Consequence: the excepted field in `authority_test.go:232` and harness-boundaries
  `:818-825` names a type that will not exist.
- **F8.** `Definition.Metadata` (§ 4h): no engine reader after decision 1; 85 semteams rules set it with author notes
  (`pack` 83, `phase` 75, `adr` 73, …). Surface audit (b) and (c). Carry or drop is an owner question (ruling 9:
  "A zero is put to the owner"; this one is not a zero but has no reader).
- **F9.** Rule-level `MaxIterations` (§ 4g): documented as a limit, exposed only as `$state.max_iterations`; 7 semteams
  rules set it. Surface audit (c). Disposition is an owner question.
- **F10.** `RelatedPatterns` (`rule_factory.go:31`): no reader but the cron refusal (`cron_rule.go:188`); 0 rules
  (ruling 9). Owner question.
- **F11.** `doc.go` (142 lines) describes the factory pattern, `init` registration and graph events (`:1-142`), none of
  which is carried. Surface audit (c). *Required:* rewrite or drop with the code it describes.
- **F12.** `schema.go` and `status.go` are processor surfaces (readers in PR 8, § 6), not definition surfaces.
  *Optional:* placement between PR 2 and PR 8.
- **F13.** `Action.ParseTTL` (`actions.go:449`): readers are PR 4/5 handlers (`:1026`, `:1271`, `:1359`). With
  `Body`-as-raw, TTL becomes a handler key. *Required:* follows ruling 1.

### Classification by error text

None in PR 2's files: `grep -n 'strings.Contains(err' / 'err.Error()'` over the carried files found none. Refusals
use `errs.WrapInvalid` and `errs.ClassifiedCodeDetail` (`entity_pattern_contract.go:67-77`).

### Other

- **F14.** Unknown keys are accepted at definition and action level on every path (§ 7). The target-shape page § 2
  requires built-ins to refuse unread body keys. For definition-level keys in `inline_rules`, AGENTS.md "Ported
  components refuse unknown config keys at construction, naming the key" applies (`config.go:24` — `InlineRules
  []Definition`). Files and the KV bucket have no ruling.
- **F15.** Validation-order nondeterminism: `config_validation.go:422` ranges a Go map. *Optional.*

## 11. Same-class collision table: the action-handler map

| Dimension | Evidence |
| --- | --- |
| Semantic class | action type name → what checks it at load and runs it at fire |
| Owners | pin: `actions.go:916-945` (switch), `rule_factory.go:124-127` (rule-type map, a sibling class), the type sets in § 4a. Main: none for actions; registries of the same shape listed in § 7 |
| Catalogs | none durable. `component.Registration.Schema` carries config schemas (`processor/graph-embedding/component.go:251`). No pin catalog of action types beyond the constants and docs |
| Status | none at the pin (no status or metric names a registered type). ADR-094 activation outcomes (`rejected`) cover a hot-reload refusal |
| Lifecycle | pin: process-global, filled in `init`, removable by `UnregisterRuleFactory` (tests). Main's registries are per-instance values built in `main` |
| Ownership | pin: process-global map behind `sync.RWMutex` (`rule_factory.go:124-127`). Main: per `*component.Registry`, per `*payloadregistry.Registry`, per `*lifecycle.Manager` |
| Readers | pin: `Execute` (fire), `validateActionLists`, `actionDistinguisher`, the metric label |
| Writers | pin: `expression_factory.go:445`, `test_rule_factory.go:212-218`. Product writers: none (`RegisterRuleFactory` 0 hits in consumers) |
| Recovery | none: in memory, rebuilt each boot |

## 12. Adopter seam inventory

### Person 1: a semteams developer moving rule packs

- **What they must know** (counts at semteams 4df44ae8; 85 rule files, 85 rules, 124 actions):
  1. Which action types are built in. semteams uses `add_triple` 57, `publish_agent` 42, `replace_owned` 9,
     `lifecycle_transition` 8, `reconcile_predicates` 6 and `remove_triple` 2. `publish_agent` must come from the
     product, and `replace_owned` exists nowhere.
  2. That keys the core does not read are now the handler's, and a built-in refuses keys it does not read. semteams'
     built-in actions use only keys the pin's handlers read: `add_triple` (`subject`, `predicate`, `object`),
     `reconcile_predicates` (plus `id`, `max_iterations`, `when`, `projection_*`), `lifecycle_transition` (`workflow`,
     `phase`, `reason`).
  3. That the firing-cap key changes from shape-only to whole-body. 12 actions set `id`; 112 do not.
  4. That `metadata` (85 rules) and rule-level `max_iterations` (7 rules) do nothing in the engine (§ 4g, § 4h).
- **If they do nothing:** at the pin, `replace_owned` passes load and fails at every fire with `unknown action type`
  in a log line (`actions.go:944`). Under "refuse at load", the same files fail at load. The 6 semteams test files
  that read typed `Action` fields fail to compile. semteams' 230 rule conditions all set `required`, so a change of
  its default does not reach them. Its 7 `when` conditions all omit it, and their behaviour follows whichever default
  the one decode picks (§ 4b).
- **Where they find out:** compile error for the typed-field tests; boot or load error for an unknown type; nowhere
  for `metadata`, rule-level `max_iterations` and the default of `required` on `when` conditions.

### Person 2: a semboids operator

7 rules: `publish` 6, `lifecycle_transition` 1.

- **What they must know:** `rule.Register` (`componentregistry/register.go:23`). Their pack uses only core types and
  keys (`publish`: `subject`, `properties`). But all 14 of their conditions omit `required`. Decoded from a file
  today, that is `false`: a missing field means no match. Decoded through the KV map path, it is `true`: a missing
  field is an evaluation error (`config_validation.go:611`).
- **If they do nothing:** the pack loads. How its 14 conditions treat a missing field depends on the default the one
  decode picks. Under `true`, a message that lacks a field the rule names becomes an evaluation error where it is a
  silent no-match today.
- **Where they find out:** a log line of an evaluation error at fire, if the default changes; otherwise nowhere.

### Person 3: semsource

Imports no `processor/rule` symbol.

### Facts the caller is asked to predict

- Which body keys a built-in reads.
- Whether a body edit resets a cap.

Both are framework facts that the surface would make the author hold.

### Usage counts (ruling 9; checked-in rule JSON only)

| Feature | semteams (85 rules) | semboids (7 rules) |
| --- | --- | --- |
| cron rules | 1 | 0 |
| `entity` | 48 | 0 |
| `cooldown` | 2 | 0 |
| `metadata` | 85 | 0 |
| rule `max_iterations` | 7 | 0 |
| `on_exit` key present | 84 (all empty) | 0 |
| `logic` absent | 1 (the cron rule, `research/10-scheduled-program-pulse.json`) | 0 |
| `enabled: false` | 1 (the same cron rule) | 0 |
| `enabled` absent | 0 | 0 |
| conditions | 230 | 14 |
| conditions omitting `required` | 0 | 14 |
| action `when` conditions | 7 | 0 |
| `when` conditions omitting `required` | 7 | 0 |
| `while_true`, `on_recovery`, `rerun_on_recovery`, `related_patterns`, `fire_every_n_events` | 0 each | 0 each |
| action `id` | 12 | 0 |
| action `max_iterations` | 6 | 0 |
| action `when` lists | 7 | 0 |

Not counted: rules built in Go, rules in tests, rules loaded at runtime.

## 13. Intent check

No capability boundary moves. What PR 2 does not carry, and the ruling for each:

- the rule-type registry: ruling 2 on PR #752;
- the graph-event types: #9 comment 7160955176, item 1;
- the agentic `Action` fields, checks and types: #8 Q15 and D12 E2-E4, as recorded in the split plan.
