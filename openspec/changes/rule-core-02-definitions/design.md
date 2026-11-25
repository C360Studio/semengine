# Design: rule-core-02-definitions

Pin: SemStreams `8b99efe9`. Givens, not reopened: the target-shape page
(`archive/2026-11-25-design-rule-core-target-shape/`, §§ 1-2, approved proposal); shape rulings on PR #752 (7162146299
items 1, 2, 6, 9; addendum 7162180652); rulings on epic 9 (7160955176, 7165028479) and on PR #758 (7166884728), Fable's
read 7166853645 finding 4 (D3, D4, D9 answer 1-3). Spec delta: `specs/rule-definitions/` (new), `harness-boundaries`
(two modified).

## Context

PR 2 lands the rule definition and the one load step every later PR calls. Measured premises (inventory sections): no
pin load path refuses an unknown action type (`actions.go:943-944`, § 3); six decode paths disagree on absent keys (§
4b); four load sites log and skip (§ 3), which the epic-9 ruling forbids. Pin probe: PR 2's 34 tests pass (§ 2);
`json.Marshal` compacts and escapes raw JSON (§ 7). Size: 906 production and 388 test pin lines carried and rewritten
(`tasks.md` 2.9), plus new tests; until PR 8 only its own tests call `processor/rule`.

## Decisions

**D1. What PR 2 carries, and which names are exported.** Framework API (target-shape page § 1-2; contract line: the
`rule-definitions` spec): `Definition` and `EntityConfig`, `Action`, `ActionHandler`, `Handlers` and `NewHandlers` (D3),
`DecodeDefinition` and `Load` (a product checks its packs with its own handlers), `Rule` (what `Load` returns; fields
unexported), the nine built-in type names. Unexported: the cap key, `LoadedAction`, the default cap. Not carried, by
ruling: `Factory`, `Dependencies`, `Schema`, the registry functions, `interfaces.go`, `docs/custom-rules.md`, the map
round trip, the agentic fields and checks, deny/approve; a rule's `entity.watch_buckets` with
`ErrorCodeEntityWatchBucketUnsupported` and `unsupportedEntityWatchBucket` (PR #758); nor `test_rule_factory.go`
(nothing to register into). Moved to PR 8, as they read `Config` or `Processor`: the file loader `rule_loader.go:55-99`
(a departure from the split plan, put to the owner), `schema.go` (`:12` reflects `Config{}`), `status.go`. `Load` takes
the pattern check of `validateDefinitionEntityPattern` (`entity_pattern_contract.go:49-65` less the bucket lines
`:50-54`, `:56-58`); the processor-setting helpers (`:19-47`, `:79-121`) move with their readers in PRs 8, 10 and 12.
Two callers of the dropped helper remain: `entity_pattern_contract.go:41` (the processor's `entity_watch_buckets`
setting, PR 8) and `entity_watcher.go:101` (PR 10); each still refuses a foreign bucket, classified invalid, without the
code (no consumer reads it).

**D2. The handler type arrives in two steps.** PR 2's `ActionHandler` has `Check(a Action) error`; PR 4 adds `Run` once
PR 3 has brought `ExecutionContext`. Options: an empty exported `ExecutionContext` now (a type that promises what it
does not hold); no handler type until PR 4 (the type set spelled twice). Adding a method breaks no one: every
implementer is in this repository until PR 8 registers products.

**D3. Load refuses, never skips (ruled), with the complete handler set.** `Load(def, source, h Handlers)` compiles the
conditions and each `when` (PR 1's `Compile`) and checks every action with its type's handler in `h`, returning an error
classified invalid naming rule ID, source, position (`on_enter[1].when[0]`) and cause. Disabled rules are checked too,
as `prepareInitialRules` did (`rule_loader.go:116-119`); checking only enabled ones lets a broken rule wait. Sources: a
rules file's path, `inline_rules[<index>]`, `rules.<id>@<revision>`. PR 8 fails construction on a refusal; PR 12 rejects
the candidate set (ADR-094). `NewHandlers(product map[string]ActionHandler) (Handlers, error)` is the one builder of the
set (target-shape § 2): the nine built-ins plus the product's, refusing a reused built-in name, an empty name or a nil
handler; `Load` refuses a set it did not build. Its arguments grow in PRs 4 and 5 by what built-in checks read from
configuration (D6), on D2's reasoning; the executor builds from it too. Other option: built-ins inside `Load`, so PRs
4-5 change `Load` or add a second path. Proof: `TestLoadRefuses` (one case per spec refusal); `TestNewHandlers`.

**D4. One decode, strict; the body is the action.** `DecodeDefinition(data []byte, source string) (Definition, error)`
decodes one rule object with `DisallowUnknownFields`, refusing a key `Definition` or a condition does not declare,
naming key, rule and source. `Action`'s unmarshaler keeps the object as `Body` and decodes the four core keys strictly,
so a misspelt `when` key is refused naming `when[<i>]`; body keys are the handler's `Check` (D6; PR 1, D2 option C).
`Body` is the action: the four fields are read-only copies the decode fills, `MarshalJSON` writes `Body`, and `Load`
refuses an action with no `Body` or with fields that differ from it, saying to build it by decoding. `Load` leaves its
input unchanged: it compiles the `Rule`'s own copies, and the fields-to-`Body` check ignores compiled state, so loading
one definition twice succeeds. semteams' three `[]rule.Action{` test literals (§ 6) become decoded JSON. Other option:
both spellings, with a rule for which copy wins. Absent keys: `enabled` true, `required` false (files' meaning;
semboids' 14 conditions rely on it), `logic` `and`, `name` empty. Callers: PR 8 (each rules-file element, source the
file; each `inline_rules` element) and PR 12 (each stored value). "Equal" after a marshal and decode: every field equal,
each `Body` equal in canonical form (D8's, so the key is unchanged), not byte for byte (§ 7). Proof in PR 2:
`TestDecodeDefinition` (that equality; undeclared keys refused, a `when[0]` one and `entity.watch_buckets` included; the
absent-key meanings) and `TestActionBody`. Other option: refuse undeclared keys in `inline_rules` only; typos elsewhere
stay silent.

**D5. Fields with no engine behaviour.** `metadata` stays as author notes the engine never reads (85 semteams rules);
the claim that it is substituted goes. Rule-level `max_iterations` stays as the value `$state.max_iterations` reads,
documented as not a limit (7 rules). `related_patterns` is dropped (0 rules, no reader), so D4 refuses it.
`on_recovery`, `rerun_on_recovery`, `while_true` stay (ADR-111 Decision 5 relies on the first two); a
`rerun_on_recovery` with nothing to re-run is refused (only dead code checks it, `rule_factory.go:266`). Other options:
enforce `max_iterations` (changes 7 rules); drop `metadata` (refuses 85).

**D6. Built-in checks.** Each of the nine built-ins refuses body keys neither the core nor its type reads (the pin
handlers' key sets, `tasks.md` 2.4) and keeps the pin's load checks: `publish` refuses a declared subject in
`user.response.>` (ADR-093's load half; the after-substitution half is PR 4's); the triple types need a literal,
declared predicate (ADR-074); `max_iterations` is 0 or more (`config_validation.go:340-393`). Checks that need
configuration come in PRs 4 (triple writer) and 5 (reconcile targets, `update_kv` buckets #294, lifecycle manager).
Proof: `TestBuiltinCheck`; a one-off run over both products' packs (`notes/2.4.md`) refuses no built-in for a key.

**D7. Cron before PR 11: refuse only enabled cron rules.** semteams lists its disabled cron rule in both flows
(`configs/flow-bootstrap.json:366`, `configs/e2e-flow-bootstrap.json:331` at 4df44ae8); the pin skips it before
`NewCronRule` (`rule_loader.go:185-188`). Options: (a) refuse every cron rule: with D3, semteams' processor fails to
start from PR 8 to PR 11 unless it edits both flows; (b, recommended) refuse an enabled cron rule, and give a disabled
one the common checks only (its schedule is checked from PR 11; until then its own fields go unchecked); (c) accept cron
and run nothing: silent. Proof: in `TestLoadRefuses`.

**D8. The firing-cap key (ruled; consequences).** `id`, else a SHA-256 over rule ID, type and the canonical body
(`UseNumber`, sorted keys, no spacing, no HTML escaping), replacing `action_id.go:37-106`. `max_iterations` and `when`
are in the body, so raising a cap from 3 to 5 gives 5 fresh firings; the pin's key excluded them and kept the count (2
more). Pin-keyed stored counts are not migrated. The reconcile request ID (PR 5, `actions.go:1394-1400`) is built from
the key: an edit changes it, but within one firing it is stable, so a redelivered write keeps its key (ADR-111 Decision
5). Other options: drop `max_iterations` from the hashed body (keeps the pin's raised-cap count; departs from the ruled
"body as written"); a separate request key (a second spelling). Proof: `TestCapKey`.

**D9. #26, byte preservation (accepted proof).** A decode keeps `Body` as written: `TestBodyKeepsBytes` reads semteams'
fixtures at `a331910e` (MIT), copied to `processor/rule/testdata/rule-family-consumer/` with a README naming repository,
commit and paths; each `on_enter[0].Body` equals its bytes in the file, and its `tool_choice` and `response_format`
decode equal to `expected.json`. The proof covers a decode. A stored rule (PR 12) is re-encoded: it keeps its values and
its cap key, not its bytes (D4).

**D10. The guard exception goes.** `processor/rule.Dependencies.Platform` leaves `authority_test.go:232`; the fixture's
`processor/rule/rule.go:6` moves from the silent list (`:194`) to the failing list. The guard never reports an unused
exception (§ 4i), so a later struct of that name would pass unchecked.

## ADRs

Kept: ADR-036 (rule-opaque refusal), ADR-074 (declared predicate), ADR-093 (reserved space, load half here), ADR-031
(cron fields decoded; running waits, D7), ADR-111 Decision 5 (recovery keys). Later: ADR-094 (PR 12), ADR-102/104 (PR
8). Dropped, each ruled: ADR-039; the fields of ADR-023, 034, 046, 053, 067 (Q15); ADR-076 decisions 1-3 and 5. ADR-036
and ADR-093 are not ported to `docs/adr`: each binds one rule-side clause, carried as a spec requirement.

## Issues and merge order

`Closes #26`. Named without a closing keyword: #25 (load half here; fire half PR 7); #27, #28 (agentic checks and
`VerdictAuditor` not carried; retire at PRs 4 and 7); #294 (PR 5); #702 (default cap carried; PR 7); #703 (its keys join
`Definition` in PR 14). Stay, no PR 2 work: issues 195 (no NATS client here), 196 (no lifecycle owner here; PRs 8, 11,
12), 350 (PR 5's reconcile path), 714 (PR 13), 704 (decision epic). PR #753 merges first (`expression.Compile`; shared
ledger, map, coverage list), then this branch merges `origin/main`. Risks: `ActionHandler` gains `Run` and `NewHandlers`
arguments in PRs 4-5 (D2, D3); every caller of `Load` lands in PR 8 or later; `assert.Equal` on conditions sees the
compiled pattern (PR 1).
