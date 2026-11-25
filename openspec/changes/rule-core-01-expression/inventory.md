# Inventory: rule-core-01-expression

base: 245c59c468a0ad9d00bc5c4bc05cdb4be8fdc951 (claim commit on main c729fecdb06596d83965c178210014a9430495a2).
Pin: SemStreams `8b99efe9`, read with `git -C /Users/coby/Code/c360/semstreams show|archive 8b99efe9` (never the
working tree); scratch pin copy `/tmp/pin-full.4uEJ`, scratch main copy with the pin package `/tmp/se-drift.SlUo`
(local only). Products, read-only: semteams `c8571b34`, semboids `37dbdb0`; semsource `58839c5` and semconnect
`dff1265` at symbol level only (`docs/inventory-scope.md`).
Question: what PR 1 carries when it ports `processor/rule/expression` whole, and what binds it.

## Size

`wc -l` at the pin: production 1,417 (`evaluator.go` 965, `types.go` 176, `regex_cache.go` 137, `stateless.go` 90,
`message_path.go` 49); tests 2,110 (`evaluator_test.go` 1,449, `regex_cache_test.go` 343, `array_operators_test.go`
318). Total 3,527, matching the plan (`split.md`, PR 1 row).

## Pin probe

- `go test -count=1 -race ./processor/rule/expression/`: ok. `go test -count=5 -cpu=1 -shuffle=on`: ok (seed
  1795610870471937000). Coverage 79.4%.
- Test text: `time.Sleep` 0, `Skip` 0, build tags 0. `t.Parallel` in 14 unit tests (allowed: the rule covers
  integration files only). One test sets its own deadline: `regex_cache_test.go:265` —
  `ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)`.
- Lifecycle suite: not applicable (no service, no `Start`). Integration tests: none in the package.
- Production `context`, goroutines, `sync`: none. Package state: `regex_cache.go:15` —
  `var globalRegexCache cache.Cache[*regexp.Regexp]`, filled in `init` at `:18`.
- Behaviour probe (`zz_probe_test.go` in the scratch main copy): required and missing -> `required field not found`
  error; `lt` "apple" < "banana" -> true, no error; `lt` 5 < "banana" -> true, no error; `contains` on 12345 -> true;
  `eq` true vs "true" -> true; "9.9" `lt` "10.0" -> true; "NaN" `eq` "NaN" -> true; empty logic with one true and one
  false condition -> true (OR); logic `xor` -> error; unknown operator -> error; regex with a non-string value ->
  error; `[unclosed` -> error at evaluation; `in` with a non-list -> error; required `length_eq` 0 on a missing
  predicate -> true; `transition` through `Evaluate` -> false, no error.
- Regex probe (stdlib, go1.26.4): `$` as an anchor before a letter (`a$b`, `(?m)a$b`) matches nothing; a literal `$`
  matches. All true: `[$a-z]+` on the text `$`; `^\$USD` on `$USD`; `\Q$now\E`, `[$]now` and `\x24now` on `$now`;
  `(?i)\$Price` on `$price`. `regexp.Compile` refuses `a{1001}` and compiles `a{1000}` and `(a+)+$`.
- semteams' one regex pattern passes `validateRegexComplexity` and `regexp.Compile` and holds no substitution token.
- Empty-logic probe: on the pin copy, making `""` mean AND (`evaluator.go:151`) and running
  `go test ./processor/rule/...` (unit): both packages ok.
- JSON probe (go1.26.4): an error returned by a custom `UnmarshalJSON` comes back from `json.Unmarshal` unchanged
  (`errors.Is` holds); a decoder's `DisallowUnknownFields` does not reach keys a custom `UnmarshalJSON` decodes.

## Compile drift against main (production and test)

Copied into the scratch main copy at `processor/rule/expression`, module path rewritten, `pkg/cache` ->
`internal/cache`:

- `regex_cache.go:21` — `cache.WithEvictionCallback(func(_ string, _ *regexp.Regexp) {` — undefined (removed, #9
  comment 5950725772).
- `evaluator_test.go:11` — `"github.com/c360studio/semstreams/internal/semantictest"` — now
  `internal/harness/semantictest` (ledger :317).
- `evaluator_test.go:608` and `:1167` — `Version:   1,` — `graph.EntityState` has no `Version` (ADR-110).

With these three fixed: `go vet` clean; `-race` ok; `-count=5 -cpu=1 -shuffle=on` ok; pinned revive clean;
`go test ./internal/harness/contract/` ok.

## Surface audit

Pin-wide reader search: `git grep 'expression\.<Name>'` at `8b99efe9` outside the package.

Readers outside the package (production): `ConditionExpression` (rule core, `internal/boot`, test/e2e),
`LogicalExpression`, `StateFields`, `MessageFields`, `Evaluator`, `NewExpressionEvaluator`, `Evaluate`
(`expression_factory.go:317`), `EvaluateWithStateAndMessage`, `LogicAnd`, `OpEqual`, `OpTransition`,
`ExtractMessageValue` (`message_substitution.go:75`, `typed_substitution.go:151`), `RegisteredOperators`
(`rule_factory.go:288` — `_, ok := expression.RegisteredOperators()[op]`), `EnsureStatelessResolvable`
(`matches.go:171`), `EvaluationError` (rule tests; semteams tests `cmd/semteams/approvalpause/rule_order_test.go`,
`cmd/semteams/rule_hot_reload_integration_test.go`, `test/contract/autoresearch_reply_integration_test.go`).

No reader anywhere (pin, semteams, semboids, semsource, semconnect):

- `evaluator.go:844` — `func (e *Evaluator) HasTriple(` ; `:861` — `func (e *Evaluator) GetOutgoing(` ; `:887` —
  `func (e *Evaluator) Distance(` and the helpers `extractLatLonFromTriples`, `haversineDistance`, `isValidEntityID`
  (`:906-965`). Only `evaluator_test.go:429-697` reads them.
- `types.go:51` — `type TypeDetector interface {` — one implementation (`evaluator.go:392`), held in an unexported
  field with no setter; the evaluator calls its two lookup methods (`evaluator.go:236`, `:258`, `:340`).
  `DetectFieldType` (`evaluator.go:428`) is never called; `FieldType`, its five constants and `String()`
  (`types.go:67-95`) are read only by it.
- `types.go:48` — `type OperatorFunc func(...)` — exported, read only inside the package.
- Unexported, test-only: `clearCache`, `cacheSize`, `cacheStats` (`regex_cache.go:125-137`).

Config fields: none (the package reads no configuration). `ConditionExpression` has no `UnmarshalJSON`.

Advertised-absent (pin `processor/rule/docs/conditions.md`, which rides with PR 1):

- `conditions.md:217-232` says a required field that is missing makes the condition false; the code returns an error
  (`evaluator.go:192-196`, `:278-288`).
- `conditions.md:29-38` says the evaluator detects a field's type and lists operators by type; nothing gates
  operators by type (`DetectFieldType` unread; probe: `contains` on a number is true).
- `conditions.md:249-259` and `:378-385` say a failed coercion is an error; `compareValuesWithError`
  (`evaluator.go:718-742`) never returns one.
- `length_gte`, `length_lte` (`evaluator.go:50`, `:52`) are undocumented; `from` is missing from the documented
  struct (`conditions.md:7-14`); links name `03-conditions.md`, `04-actions.md`; regex limits are undocumented.

## Shape sweep

1. Second spelling: the default for empty logic. `evaluator.go:151` — `case LogicOr, "": // Default to OR if not
   specified`; the rule layer spells AND at `config_validation.go:464` —
   `Logic:   getStringWithDefault(ruleMap, "logic", "and"),`, `expression_factory.go:104-106`, `matches.go:187-189`,
   `stateful_evaluator.go:294-296`, `syntax.md:109`, `conditions.md:189`. Every production caller sets `Logic`
   (`expression_factory.go:302`, `:324`, `stateful_evaluator.go:292`, `:558`). Also `$entity.lifecycle.` at
   `stateless.go:20` and `lifecycle_substitution.go:92`; `$prev.` at `evaluator.go:359` and
   `stateful_evaluator.go:226`, `:278` (PRs 3 and 7).
2. Copied shell: none.
3. Layered interface with one implementation: `TypeDetector`.
4. Exported surface with no reader: `HasTriple`, `GetOutgoing`, `Distance`, `TypeDetector`, `FieldType`,
   `DetectFieldType`, `OperatorFunc`.
5. Classification by error text: none in production; tests match error text.

Other: 27 comment lines carry port history (`#147`, `gh#207`, `gh#731`, "Phase 1", "semteams rule packs"), e.g.
`evaluator.go:44`, `:748-768`, `types.go:25`, `stateless.go:15`. `regex_cache.go:40` — `// Security check: validate
pattern complexity to prevent ReDoS`; Go's regexp is "guaranteed to run in time linear in the size of the input"
(`go doc regexp`) and promises nothing about cost as the pattern grows.

## Every current spelling of "a regex pattern" and "a template"

- Pin compile per evaluation through the global cache: `evaluator.go:703` — `re, err := compileRegex(pattern)`;
  `regex_cache.go:46` — `re, err := regexp.Compile(pattern)`.
- Substitution runs before evaluation on any string value holding `$`: `execution_context.go:424-428` —
  `if !ok || !strings.Contains(s, "$") {` ... `out[i].Value = ec.SubstituteVariables(ctx, s)` — and replaces known
  tokens anywhere in the string (`:288-322`). The tokens: `$now` (`:288`), `$entity.` and `$related.` (`:291-298`,
  triple forms `:55`, `:88`, `:114`), `$state.` (`:302-303`), `$schedule.` (`cron_substitution.go`), `$caller.`
  (`caller_substitution.go`), `$message.` (`message_substitution.go:54`); the unresolved check
  `execution_context.go:35` —
  ``regexp.MustCompile(`\$(?:entity|related|state|schedule|caller|message)\.[a-z0-9][a-z0-9_.-]*`)``. The
  iteration variable `$<name>` (`:263-268`) leaves with `publish_agent`.
- SemEngine main: no rule code; `git grep -n 'regexp\.Compile([a-z]'` over production: none. Other user-pattern
  compiles at the pin are in non-admitted agentic packages only.

## Where the pin keeps and fires definitions

- Decode: per file at `rule_loader.go:72` (the file is known there), then merged; inline rules
  (`config.go:24` — ``InlineRules []Definition `json:"inline_rules,omitempty"` ``); validation at `rule_loader.go:117`
  (`ValidateDefinition`, reports the rule ID, no file).
- JSON clones after validation: `rule_loader.go:111`, `:121`, `:170`, `processor.go:270`
  (`cloneRuleDefinitions`, `:89-95`). Map round trip: `definitionToRuleMap` (`rule_loader.go:39`, `:216`, `:244`)
  and back through `definitionFromMap` (`config_validation.go:129`, `:564`, `runtime_config.go:183`), which builds
  conditions as struct literals (`config_validation.go:607-611`) and makes `required` default to **true**
  (`getBoolWithDefault(condMap, "required", true)`), where JSON decode makes it false.
- Kept for firing: `rp.ruleDefinitions[def.ID] = def` (`rule_loader.go:215`, `:243`) and
  `CreateRuleFromDefinition` (`:226`).
- Evaluation errors on the fire paths are silent: `expression_factory.go:200-205` and `:224-230` log at Debug and
  return false; `stateful_evaluator.go:380-386` logs at Warn and skips the action.
- KV rules: `kv_config_integration.go:348`, `:373`; `ListRules` logs and skips an entry it cannot decode (`:368-375`).

## Problem shape and closest instance

Admit-or-refuse at a load seam, keeping the prepared form. Closest: the pin's load-time operator check
(`rule_factory.go:247-256`, through `RegisteredOperators`) and `ValidateDefinition` (`config_validation.go:273`,
`errs.WrapInvalid`); in SemEngine `pkg/rulepack.ValidateID`. None keeps a compiled form. Adoption sweep: no admitted
package compiles a user-supplied pattern.

## Open issues

`gh issue list --state open --search <q>` for `expression`, `regex`, `regex_cache`, `evaluator`, `rule/expression`,
`condition`, `operator` (2026-11-25). Naming the package: #9 (ruling comment 5950725772 names `regex_cache.go:21`
for a later port-refactor; no issue was filed). Naming the word only: #702 (`expression_factory.go:31`), #25, and
also #28, #703, #484, #641, #502, #155, #712, #647, #646, #350, #110.

## Open PRs

`gh pr list --state open`: #753 (this claim); #754 (draft, `openspec/changes/design-rule-core-target-shape/`); #755
(draft, `claude/probe-703-scheduled-messages`, one empty commit). No file or capability overlap.

## ADRs

Pin `docs/adr`, 85 records; grep for the package path and exported names, then by title.

| ADR | Decides, for this code | Status here |
| --- | --- | --- |
| 041 Unified condition evaluator | one evaluator for rule conditions and action `when`; the field-resolution order | binding; not in `docs/adr` |
| 046 Parallel fan-out | joins count triples with `length_eq`; list operators see every triple | its list-operator note is code here; the rest PR 3 |
| 047 Lifecycle harness | `$entity.lifecycle.*` condition fields | prefix recognized here (`stateless.go:20`); ADR with PR 5 |
| 048 BoundedDispatcher and `.triples` | `.triples` composes with list operators | substitution, PR 3 |
| 094 Boot-only composition, observable rule activation | a KV rule candidate is accepted or `rejected`, observably | in `docs/adr`; governs how PR 12 reports a refusal |
| 039 Tool-call governance | deny/approve | not carried (E4) |
| 040 Retire boids | mentions the evaluator in passing | not governing |

SemEngine: ADR-110 (no `EntityState.Version`) explains the test drift.

## Guidance at the pin

`git grep -i 'rule/expression|regex|ConditionExpression'` over pin `.agents`, `.claude`: none. Docs:
`processor/rule/docs/conditions.md` (393 lines), `syntax.md` (330 lines; rule definitions, actions, templates).

## Usage count

Checked-in rule JSON at each product's HEAD (`git show HEAD:<file>`). semteams: 84 files under `configs/rules/`;
semboids: 3 under `configs/rules/zone-steering/`. Conditions (files):

| Feature | semteams `configs/rules/` | semboids |
| --- | --- | --- |
| `eq` / `ne` | 138 (73) / 20 (18) | 14 (3) / 0 |
| `lt` / `lte` / `gt` / `gte` | 0 / 3 (3) / 6 (4) / 0 | 0 |
| `between`, `contains`, `starts_with`, `ends_with` | 0 | 0 |
| `regex` | 1 (`configs/rules/research/02-plan-to-gather.json:25`) | 0 |
| `in` / `not_in` | 26 (18) / 0 | 0 |
| `length_eq` / `length_gt` | 34 (27) / 9 (9) | 0 |
| `length_gte`, `length_lt`, `length_lte`, `array_contains` | 0 | 0 |
| `transition`, `from` | 0 | 0 |
| logic `and` / `or` / omitted | 84 lists / 0 / 0 | 7 / 0 / 0 |
| `required: true` / omitted | 230 / 7 | 0 / 14 |
| `$state.*` field | 7 (3), all in action `when` guards | 0 |
| `$prev.*`, `$entity.lifecycle.*` fields | 0 | 0 |
| `$message.<field>` / deeper `$message.a.b` | 0 / 0 | 14 (3) / 0 |
| `$`-templated value | 11 (7): `eq` 3, `gt` 4, `lte` 3, `length_eq` 1 | 0 |
| `$`-templated regex value | 0 | 0 |

semteams `test/fixtures/rule-family-consumer/` adds 3 files: 3 `eq` conditions, 3 `and` lists, 3 `required: true`.
Keys used in any condition: `field`, `operator`, `value`, `required` only. Not countable from files: bare names that
fall back to the message; rules built in Go, in tests, or at runtime.

## Adopter seam

Reached from outside: rule JSON authors (semteams, semboids) and Go code using the types (semteams tests:
`ConditionExpression`, `EvaluationError`; the rule core from PR 2 on).

1. What must they know? (a) A regex value cannot hold a substitution token. (b) Code that keeps a rule for firing
   must prepare its conditions first.
2. If they do nothing: (a) no checked-in rule is affected; (b) at the pin's fire paths an evaluation error is a
   Debug or Warn line and the rule or action silently does not run (above).
3. Where do they find out: (a) at load; (b) depends on the design and on PRs 6 and 7.
4. What should they have to know: nothing for (b).

Collision table: not triggered. Intent check: no boundary moves; the port set is ruled (#8 Q15, D12).
