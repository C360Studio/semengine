# Inventory: rule-core-06-expression-rules

base: a2174c4a94c8719f0c711c27dbe494ca4778eae3 (claim commit on `claude/rule-core-06-expression-rules`, parent origin/main
3d823c82b14f2ee1a6e9a6b8e46b0b7e1fc35d8a; adds only `proposal.md` and `tasks.md`; draft PR #777). Probe evidence:
`notes/inventory-probe.md`. Pin copy: `gh api repos/C360Studio/semstreams/tarball/8b99efe9`, unpacked to a scratch
directory (local only). Consumers read by `gh api …/tarball/<sha>` at the commits PR 3's inventory used: semteams
4df44ae877d0c711f769ec6b48b88100dbb267d1, semboids ab4e0fbc1b382c4fda10bae988662ae0e0bc1e5c, semsource 34dc2680.

Question: what PR 6 of the rule-core split carries ("`ExpressionRule` and its factory, `Matches`, payload projection",
`archive/2026-11-25-design-rule-core-split-plan/split.md`, row 6), against the target-shape page (`archive/
2026-11-25-design-rule-core-target-shape/design.md` § 1), the owner's drop of the graph-event path (#9 comment
7160955176), #702 (cooldown leaves for PR 7), PR 1's seam (`archive/2026-11-25-rule-core-01-expression/design.md:76-90`),
PR 2's `Load`, and what PR 3 sent forward (`archive/2026-11-26-rule-core-03-context/design.md:113-118`).

Track: `design` (`proposal.md:5`). Lifecycle owner: none. Nothing carried has `Start` or `Stop`, so the lifecycle suite
does not apply.

## 1. Size (measured, `wc -l` and `sed -n a,bp | wc -l` at the pin)

| Production file | Lines | Carried (rewritten) | Not carried |
| --- | --- | --- | --- |
| `expression_factory.go` | 449 | `:57-75` (19), `:143-209` (67), `:210-236` (27), `:237-327` (91) = 204 | `:1-16` (16), `:17-56` (40), `:76-83` (8), `:84-132` (49), `:133-142` (10), `:328-367` (40), `:368-441` (74), `:442-449` (8) = 245 |
| `matches.go` | 228 | 228 | 0 |
| `payload_projection.go` | 70 | 70 | 0 |
| **total** | **747** | **502** | **245** |

| Test file | Lines | Carried | Moved | Dropped |
| --- | --- | --- | --- | --- |
| `expression_factory_test.go` | 792 | `:1-18`, `:229-363`, `:451-481`, `:542-735` = 378 | `:364-407` cooldown to PR 7 (44); `:482-541`, `:736-792` config to PR 8 (117) | `:19-43`, `:44-193`, `:194-228`, `:408-450` = 253 |
| `matches_test.go` | 688 | 660 | 0 | `:312-339` (28) |
| `matches_lifecycle_integration_test.go` | 104 | 104 | 0 | 0 |
| `payload_projection_test.go` | 428 | `:1-240` (240) | `:302-428` to PR 9 (127) | `:241-301` (61) |
| `payload_projection_integration_test.go` | 279 | 0 | 279 to PR 9 | 0 |
| **total** | **2,291** | **1,382** | **567** | **342** |

Both pin totals equal the split plan's (`archive/…-split-plan/inventory.md:20`). Sent here by PR 3, counted in PR 3's
size, not above: `typed_substitution_test.go:273-351`, `lifecycle_substitution_test.go:301-327`, `:329-355`,
`:460-483` (each builds `NewExpressionRule`).

Why each dropped test range goes: `:19-43` and `:408-450` test the factory registry (not carried, target-shape § 1);
`:44-193` tests `ExpressionRuleFactory.Validate`, whose every case is already in PR 2's `TestLoadRefuses`
(`processor/rule/load_test.go:161-193` — `cause: "at least one condition"` … `"cooldown": "soon"`); `:194-228` tests the
constructor; `matches_test.go:312-339` checks the trigger latch and last-fired stamp, which are not carried (§ 6, F6);
`payload_projection_test.go:241-301` reads `configs/rules/agentic-workflow/architect-editor.json` and `agentic` types,
neither in this repository (agentic tier, Q15).

## 2. Pin probe (commands in `notes/inventory-probe.md`)

- PR 6's 36 unit tests (`expression_factory_test.go`, `matches_test.go`, `payload_projection_test.go`) on the pin copy:
  `-race` ok, 36 `--- PASS`; `-count=5 -cpu=1 -shuffle=on` ok, `-test.shuffle 1795789945646102000`.
- PR 6's 4 integration tests (`-tags integration`, `-race`), Docker reachable: all 4 `--- PASS`.
- Behaviour probe, P1 to P7 (same notes file), each observed as stated:
  - P1: `$message.x ne "a"` on payloads `{}` and `{"y":1}`: both no match. No condition tried passes on an absent field.
  - P2: `$message.x gt "abc"` on `{"x":5}`: no match, and nothing logged at Info (the error goes to Debug).
  - P3: on the message path, `$message.who eq "$entity.id"` with `who` holding `"$entity.id"`: no match. `$entity.id`
    became `""` (no entity), silently.
  - P4: `Subscribe()` is `[>]` without an entity pattern, `[]` with one.
  - P5: a condition value `$entity.triple.x.y.missing` (absent): `EvaluateEntityState` true (compared as literal
    text against a triple holding that text); `Matches` refuses: `condition on "a.b.c" has an unresolved value
    template "$entity.triple.x.y.missing"`.
  - P6: a `transition` condition through `EvaluateEntityState`: always false.
  - P7: a disabled rule does not match; `NewExpressionRule` accepts a rule with no conditions (`err=<nil>`).
- Test-text rules over the five test files (`grep -n -E 'time\.Sleep|\.Skip|//go:build'`): one sleep,
  `expression_factory_test.go:400` — `time.Sleep(150 * time.Millisecond)` (cooldown test, moves to PR 7); 0 skips; two
  `integration` build tags. `t.Parallel` in `matches_test.go` (22), not integration-tagged.
- Context in the carried production code: no struct field of type `context.Context`; `context.Background`,
  `context.TODO`, `WithoutCancel`: 0. One nil context passed: `expression_factory.go:198` —
  `expr.Conditions = substituteConditionValues(nil, expr.Conditions, ec)`.
- Test-side compile drift: `agentic.LoopCompletedEvent` and `payloadbuiltins.NewTestDecoder`
  (`payload_projection_test.go:14-16`) are not in this repository; the file's own `declaredSubsetPayload`
  (`:44-70`) is a typed `RuleReadable` payload the carried tests can use instead. `NewExpressionRule`,
  `testPlatform` (`platform_test.go`, PR 8) and `GetRuleFactory` are not carried; tests build a rule with
  `DecodeDefinition` and `Load`. `matches_lifecycle_integration_test.go:29` builds a real `*lifecycle.Manager`;
  main has `lifecycle.NewManager(client *natsclient.Client, logger *slog.Logger)` (`pkg/lifecycle/manager.go:87`) and
  the package's shared NATS server (`processor/rule/state_tracker_integration_test.go:19`, natsfixture).

## 3. Category 1: the claimed gap

- "PR 6 adds `Load`'s expression arm and operator validation" (target-shape `inventory.md:14`): **already present**.
  `processor/rule/load.go:73-76` — `case ruleTypeExpression:` … `checkExpressionRule(def)`; `:143-183` refuses no
  conditions, no field, no operator, an unknown operator (`:147` — `operators := expression.RegisteredOperators()`),
  a logic other than `and`/`or`, a cooldown that is not a duration. PR 2 carried `ExpressionRuleFactory.Validate`
  (`expression_factory.go:395-429`) there. PR 6 owes no operator check.
- Matching, payload projection, the unreadable report: absent on main.
  `git grep -n -E 'rule\.Matches|MatchesWithLifecycle|ExpressionRule|ruleFields|payloadTypeLabel' -- ':!openspec'`:
  only `docs/adr/041-unified-condition-evaluator.md` (history text) and `load.go:74`, `:139`, `:143`
  (`checkExpressionRule`).
- `message.RuleReadable` exists: `message/rule_readable.go:56` — `type RuleReadable interface {`; its doc promises a
  report nothing yet makes (`:58-60` — "not implementing the interface at all, which means the payload is unreadable
  and the engine reports it"). One implementer: `message/generic_json.go:266` — `func (g *GenericJSONPayload)
  RuleFields() map[string]any {`.

## 4. Category 2: every current spelling

- **Condition-value substitution:** `processor/rule/execution_context.go:296` — `func substituteConditionValues(ctx
  context.Context, conds []expression.ConditionExpression, ec *ExecutionContext) …`; it calls `SubstituteVariables`,
  which logs unresolved tokens at Warn (`:101-110`). The one scan returning the unresolved tokens without logging:
  `:167` — `func (ec *ExecutionContext) substitute(ctx context.Context, template string, names map[string]string)
  (string, []string) {`.
- **Unresolved-token grammar:** the pin's `matches.go:220` reads `unresolvedTemplateVarRe`
  (`execution_context.go:35` at the pin), which main does not have (PR 3 D2 replaced the regexes with the scan). One
  spelling on main: the scan's unresolved list (`execution_context.go:195-196`).
- **Lifecycle fields for conditions:** `lifecycle_substitution.go:112` — `func populateLifecycleStateFields(ctx
  context.Context, manager LifecycleManager, entityID string, fields expression.StateFields) error {`.
- **Stateless answerability:** `expression/stateless.go:54` — `func EnsureStatelessResolvable(condition
  ConditionExpression, lifecycleResolved bool) error {`.
- **"A rule with no conditions does not match":** pin, four places: `expression_factory.go:399` (Validate),
  `:171` (Evaluate), `:268` (entity path), `matches.go:147`. Main: `load.go:144-146` refuses an expression rule with
  none; the evaluator passes an empty list (`expression/evaluator.go:108-111`, `rule-conditions` spec "Conditions
  combine"). A loaded cron rule has no conditions.
- **Which path a rule runs on:** pin, two spellings: `Subscribe()` returns `>` or nothing by entity pattern
  (`expression_factory.go:112-115`, read at `message_handler.go:390`, `processor.go:1451`) and
  `ruleDef.Entity.Pattern != ""` (`message_handler.go:102`, `:265`). `matchesRuleSubject`'s prefix branch
  (`message_handler.go:395-400`) can never run: `>` is the only subject. Main: `definition.go:76` —
  `Pattern string \`json:"pattern"\``, the one field.
- **A required field that is absent:** `expression/evaluator.go:167`, `:183`, `:229-240` return an `*EvaluationError`
  ("required state field not found", "required message field not found"), with no sentinel; `EvaluationError` has
  `Err` and `Unwrap` (`expression/types.go:50-66`).
- **Regex identity after substitution:** `expression/regex.go:83-86` — the kept pattern counts only while
  `c.pattern.String() == pattern`; `substituteConditionValues` copies the condition and rewrites `Value`.
- **The message projection:** pin `payload_projection.go:40` (`ruleFields`), read by `expression_factory.go:175`,
  `message_handler.go:460` (`extractMessageData`, PR 9) and `test_rule_factory.go:72` (not carried).

## 5. Category 3: adjacent claims

- Open PRs (`gh pr list --state open --json number,title,files`, all 7 read): #777 (this claim, 2 files). #774
  (draft, 22 files: `graph/constants.go`, `graph/kvcatalog/…`, `processor/graph-index/…`, its own change): no
  shared file or capability. #760-#764 Dependabot (`go.mod`/`go.sum`, `package*.json`, a workflow): no shared file;
  #761 bumps nats.go, which PR 6's integration test runs on. No open PR touches `processor/rule`,
  `docs/admission-ledger.yaml` or `openspec/specs/rule-*`. PR 4 (`rule-core-04-graph-actions`) has no claim
  (`git ls-remote --heads origin | grep rule-core` gives only this branch).
- Specs: `rule-conditions` (`openspec/specs/rule-conditions/spec.md:69-77` empty list passes; `:91-99`
  `EnsureStatelessResolvable`); `rule-definitions` (`:42-74` Load); `rule-substitution` (`:42-64` unresolved token stays
  and is reported). The pin's contract for this code: `openspec/specs/rule-engine/spec.md` at the pin, requirements at
  `:64`, `:107`, `:191`, `:235`, `:268` (stateless match; refuses what it cannot answer; answers production's question;
  reads any rule-readable payload; an unreadable payload is observable).
- Ledger: `docs/admission-ledger.yaml:5196` — `- source_path: processor/rule`; its destination already names "the
  expression rule and its matching (6)" (`:5206`). Coverage: `scripts/cover-check.sh:78` — `"processor/rule merged"`.
- Accepted commitments that bind PR 6: PR 1 seam, "PRs 6 and 7 report an evaluation error at Warn or above with the
  rule ID, never Debug" (`archive/…-01-expression/design.md:86`); PR 3: "PR 6 the typed test at `:273`, the nil
  context at `expression_factory.go:198`, `syntax.md` (`${entity.field}`, pair rules), its lifecycle tests"
  (`archive/…-03-context/design.md:114-115`); target-shape § 3: cooldown and every gate move to `Fire` (PR 7).
- Consumer use (symbol level): semteams `test/contract/research_loop_budget_test.go:185`, `:189`, `:455` —
  `rule.NewExpressionRule(platform, "research", rule02Def)`; `:227`, `:249`, `:257`, `:310`, `:327`, `:379`, `:477`,
  `:507` call `EvaluateEntityState`. `Matches`, `MatchesWithLifecycle`, a `RuleFields` implementer: 0 in semteams,
  semboids and semsource (`grep -rn -E 'MatchesWithLifecycle|rule\.Matches|RuleFields\(\) map'`). Pin-wide,
  `Matches` has no production caller; its named adopters are SemMachina and semdragon (pin
  `docs/operations/adopter-match-and-inflight.md:3`), both "post-MVP or not returning" (`docs/inventory-scope.md:27`).
- Product rules (semteams and semboids `configs/**/*.json`, `type: expression`): 91; 43 without an entity pattern
  (message path), 48 with one. With a `required: true` condition: 36 of 43 message rules, 48 of 48 entity rules.
  Templated condition values: 5 (one regex holding `$`, `research/02-plan-to-gather.json`; `.length` in
  `research/03b-…json`; `$entity.org.$entity.platform…$entity.instance` in `autoresearch/14-`, `16-`, `18-…json`).
  A message rule with `$entity.` in a field or value: 0. An entity rule with `$message.` in a field or value: 0. Empty
  condition lists: 0.

## 6. Category 4: who each carried name is for

| Pin name | Pin production readers | Consumer readers | Disposition basis |
| --- | --- | --- | --- |
| `Matches`, `MatchesWithLifecycle` | none | none in scope; contract: pin `rule-engine` spec `:64-233`, pin adopter note | framework API by contract line; the shape is the design's |
| `ExpressionRule`, `NewExpressionRule` | via the `Rule` interface: `message_handler.go:118`, `:308` (PRs 9, 10) | semteams contract test (§ 5) | `Rule` interface and factory not carried (target-shape § 1) |
| `SetLifecycleManager` (rule) | `runtime_config.go:241`, `processor.go:702` | 0 | a setter; the lookup can be an argument |
| `Name`, `Subscribe` | `processor.go:1451`, `message_handler.go:390` | 0 | go with the `Rule` interface |
| `ExecuteEvents` | `message_handler.go:207` | 0 | ruled dropped (#9 comment 7160955176) |
| `ExpressionRuleFactory`, `NewExpressionRuleFactory`, `Create`, `Validate`, `Schema`, `init` | `expression_factory.go:444` only | 0 | not carried (target-shape § 1) |
| `ruleFields`, `payloadTypeLabel` (unexported) | `expression_factory.go:65`, `:175`; `message_handler.go:460` | — | plumbing |

## 7. Category 5: problem shapes and nearest instances

- **One function behind two entry points that must not drift:** the pin's own `matchesWithLookup`
  (`matches.go:128`) behind `Matches`/`MatchesWithLifecycle`, sharing `evaluateConditionsAgainstEntity`
  (`expression_factory.go:260`) with the firing path. On main, nearest: `SubstituteVariables` and
  `SubstituteVariablesWith` over one scan (`execution_context.go:95`, `:127`, `:167`).
- **Classified refusal plus a bounded observed signal (once per pairing):** pin `expression_factory.go:64-74`
  (`sync.Map` per rule). On main (`git grep -n LoadOrStore -- '*.go' ':!*_test.go'`): `pkg/lifecycle/manager.go:281` —
  `if _, alreadyLogged := m.driftSeen.LoadOrStore(driftKey, struct{}{}); !alreadyLogged {`, a Warn logged once per
  key; `processor/graph-index/index.go:308` counts once per entity.
- **A distinct error a caller tells apart with `errors.Is`:** `processor/rule/state_tracker.go:63` —
  `var ErrStateNotFound = errors.New("rule state not found")`; `pkg/lifecycle/errors.go:37` — `ErrEntityNotFound`.

## 8. Category 6: the pin's decision records for the carried files

Searched every pin ADR (`grep -l -E 'expression_factory|ExpressionRule|rule\.Matches|MatchesWithLifecycle|RuleReadable|
ruleFields|RuleFields|payload_projection|EvaluateEntityState|gh#731|cooldown'`), then read those whose titles name rule
evaluation:

- **ADR-041, unified condition evaluator** (ported, `docs/adr/041-…md`): one evaluator for rule conditions and action
  guards; `Evaluate` routes through it (`:146`). Governs `expression_factory.go:143-327`.
- **ADR-047, lifecycle harness** (PR 5 ports it): `$entity.lifecycle.*` conditions resolve before evaluation; read by
  `expression_factory.go:271-277`, `matches.go:151-185`.
- **ADR-036, agent-private observable state**: no rule condition matches a rule-opaque predicate; kept as a Load check
  (`load.go:175-186`, `checkConditionField`).
- **ADR-076, framework-derived entity identity**: decisions 1-3 and 5 (trigger identity) are read only by
  `ExecuteEvents` (`expression_factory.go:349`); not ported, ruled (#9 comment 7160955176).
- **ADR-039, tool-call governance**: cites `expression_factory.go` for message-path field conditions (`:80-81`); the
  governance half leaves with deny/approve (Q15); `$message.*` conditions stay.
- **ADR-031, time trigger**: cooldown and every-N on cron (`:308`); PR 11.
- ADR-028 and ADR-055 name cooldown or `expression_factory.go` in passing (`028:73`, `055:289`); no decision on PR 6
  code. No ADR records `Matches` or the payload projection; their contract is the pin's `rule-engine` spec (§ 5).

## 9. Open issues against `processor/rule` and `processor/rule/expression`

Queries: `gh issue list --state open --search "<q>" --limit 200` for `rule`, `expression`, `Matches`, `projection`,
`ExpressionRule`. Kept, as naming a carried package:

- #756 (`rule/expression`: a Go `float64` NaN compares equal to every number) and #757 (list operators misread a Go
  `[]string`): PR 6's payload projection is the first path that hands Go-built values to the evaluator. Measured: no
  `RuleFields` implementer at the pin or in the consumers returns a float or a `[]string` (`agentic/rule_fields.go` at
  the pin: strings, ints, RFC 3339 strings; `putTime` `:89`); `GenericJSON` from JSON carries neither.
- #702 (cooldown per rule and entity; firing cap): names `expression_factory.go:31`, `:161`, `:219`, `:363`.
- #25 (action-family registry): names the rule core; retires at PR 7.
- #28, #27, #294, #714, #704, #703, #24, #9: name `processor/rule`; no work in PR 6's files.
- #195 (adoption: `natsfixture.Open` per package's test client): PR 6's integration test is a site.
- #196 (adoption: `internal/lifecycleguard`): no lifecycle owner here.

Name the word, not the package: #730, #141, #712, #110, #109, #635, #108, #155, #554, #614, #84, #284, #535, #734, #134,
  #665, #776, #765, #484, #60, #300, #775, #683, #576, #111, #771, #487, #350. The `projection` query also returns #477
  (adoption: `pkg/graphview` for shared KV views) and #270 (a graph-ingest integration test that failed once); neither
  names a carried package.

## 10. Shape sweep (pin copy)

Each finding ends with its disposition: required, with the binding basis, or optional, with why changing it is
optional ([Bounded port scope](../../../.agents/protocol.md#bounded-port-scope)).

- **F1. One rule's conditions are evaluated twice on the entity path.** `stateful_evaluator.go:261-263` — "EvaluateEntityState
  runs without $prev.* state fields, so transition conditions always return false there. This method re-evaluates the
  full condition set"; probe P6. Shape 1. Optional here (the second evaluation is PR 7's file; PR 6 adds no input
  without a caller); PR 7 decides.
- **F2. Two spellings of which path a rule runs on** (§ 4); `matchesRuleSubject`'s prefix branch is dead. Shape 1.
  Required (target-shape § 1 ruling: the `Rule` interface, which `Subscribe` belongs to, is not carried).
- **F3. The no-conditions check in four places** (§ 4). Shape 1. Optional (inherited; one check remains in the one
  match, which a loaded cron rule reaches with no conditions).
- **F4. Evaluation errors go to Debug.** `expression_factory.go:201-205` — `slog.Debug("ExpressionRule: message-path
  evaluation error",`; `:226-230`; probe P2. Required (accepted design commitment: PR 1 seam,
  `archive/…-01-expression/design.md:86`).
- **F5. A nil context.** `expression_factory.go:198` (§ 2). Required (architect contract: callers never pass nil; PR 3
  forward).
- **F6. Mutable per-rule state written by concurrent evaluations.** `expression_factory.go:206` —
  `r.shouldTrigger = result`, `:233`, `:363` — `r.lastTriggered = time.Now()`; the file says evaluation "runs
  concurrently across NATS deliveries" (`:47-48`). Required (owner rulings: the latch goes with `ExecuteEvents`, #9
  comment 7160955176; the stamp with the cooldown, #702 to PR 7).
- **F7. A setter for a dependency.** `expression_factory.go:80` — `func (r *ExpressionRule) SetLifecycleManager(m
  LifecycleManager) {`; the lookup is read only by `EvaluateEntityState` (`:224`). Required (build: the type it sets on
  is not carried, target-shape § 1; the lookup becomes an argument).
- **F8. A second token grammar.** `matches.go:220` — `if token := unresolvedTemplateVarRe.FindString(s); token != ""
  {`; main has no such regex (§ 4). Required (build).
- **F9. An `$entity.` value on the message path resolves to `""` without a report** (probe P3; main resolves
  `$entity.id` to `ec.EntityID`, `entity_substitution.go:43-44`). Required (AGENTS.md: failure paths fail closed; a
  degrade is declared); the design puts the shape to the owner.
- **F10. The firing path compares an unresolved value template as text** (probe P5); `Matches` refuses it
  (`matches.go:194-200`). Required (fail closed, as F9); the design puts the change to the owner.
- **F11. A required field that is absent is an evaluation error** (§ 4). With 84 of 91 product rules carrying one
  (§ 5), on the firing path it is the ordinary "this entity or message is not for this rule". Combined with F4's
  correction, every such evaluation would log at Warn. Required (F4's commitment makes it a log flood); the shape is
  the owner's.
- **F12. The empty-payload early return** (`expression_factory.go:181-183`). For P1's condition (`ne`, a literal value)
  evaluation gives the same answer. It does not hold for a `$message.` value template: with the early return the pin
  answers false; without it, under the design's unresolved-template rule, the match returns an error. Optional (the
  return is a shortcut); the design says which it keeps.
- **F13. Exported with no reader:** the factory, `Schema`, `init` (§ 6). Shape 4. Required (surface audit (a); and
  AGENTS.md: imports register nothing).
- **F14. A layered interface with one implementation:** `Rule`/`EntityStateEvaluator`, `ExpressionRule` the only
  production implementer (target-shape `inventory.md:21`). Shape 3. Required (target-shape § 1 ruling).
- **F15. Port-history comments:** 10 lines in the carried files name issues, reviewer findings or ADRs as history
  (`grep -c -E '#[0-9]+|gh#|ADR-|owner ruling|Measured'`: `expression_factory.go` 6, `matches.go` 4). Required
  (AGENTS.md comments rule, #606).
- **F16. An evaluator built per rule and per `Matches` call** (`expression_factory.go:129`, `matches.go:205`); it holds
  only the operator table (`expression/types.go:42-44`). Optional (no behaviour depends on it; a cost of a map
  build per call).
- Classification by error text: none (`matches.go:178` tests the field's prefix, not an error's text). Copied
  shell: none. `NewGenericJSON` in carried code: none.

## 11. Adopter seam inventory

The developer is semteams' author of `research_loop_budget_test.go`, or a product writing a recovery pass.

1. **What they must know.** Today: build an `ExpressionRule` with a platform and pack ID neither matching reads; call
   `EvaluateEntityState`, which answers false on an error. With `Matches` on a loaded rule: `Load` must accept the
   rule, so the caller needs a handler for every action type the rule uses (`Load` refuses a type not in the set,
   `load.go:118-130`, `:216-219`; the set is built by `NewHandlers`, `action.go:202`) and every bare condition field
   declared in the vocabulary (`load.go:175-186`). semteams' `research/02-plan-to-gather.json` and `03b-…json` each
   carry a `publish_agent` action, and their condition predicates (`agent.loop.role`,
   `coordinator.decision.subtopics`, …) are declared by semteams, not here. A product running the rule processor has
   both already; a recovery pass that only asks `Matches` must supply them too. A rule from a raw decode, not `Load`,
   has uncompiled regex conditions, which evaluation refuses (PR 1 risk, `archive/…-01-expression/design.md:111-112`).
2. **If they do nothing.** `rule.NewExpressionRule` does not compile: a compile error, the best rank. A pack with an
   action type the handler set lacks fails `Load` naming type, rule and position: a typed error. A semteams contract
   test that keeps asserting false for a rule missing a required field gets an error from `Matches` instead.
3. **Where they find out.** Compile error (types gone); typed runtime errors (`Load`'s refusal; `Matches`' three
   answers).
4. **What they should have to know.** That a loaded rule answers "does it match", and an error means "cannot tell".
   The gap: the handler set and declared predicates, which a product needs for `Load` anyway.

## 12. What PR 6 sends forward

- **PR 7:** cooldown, every-N and the cap (target-shape § 3); feeding `$prev.*`/`$state.*` to the one match so a rule is
  evaluated once (F1); the firing path's lifecycle lookup failure (PR 3 forward); the cooldown test (`:364-407`).
- **PR 8:** the config tests (`:482-541`, `:736-792`); status no longer lists subjects (F2).
- **PR 9:** `extractMessageData` reads the same projection; `payload_projection_test.go:302-428` and the integration
  test; whether message rules see every subject (`>`) as today.
- **PRs 7 and 9:** how often a returned evaluation error is logged (F4, F11).
