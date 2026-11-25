# Design: rule-core-02-definitions

Base: `738e9bca` (origin/main `da704d07`). Pin: SemStreams `8b99efe9`. Inventory: `inventory.md` (INVENTORY PASS,
round 2). Givens, not reopened: the target-shape page (`archive/2026-11-25-design-rule-core-target-shape/`, §§ 1-2
and its approved proposal), the shape rulings on PR #752 (comment 7162146299, items 1, 2, 6, 9; addendum 7162180652),
the rulings on epic 9 (comments 7160955176, 7165028479). Spec delta: `specs/rule-definitions/` (new) and
`specs/harness-boundaries/` (two requirements modified).

## Context

PR 2 lands the rule definition and the one load step every later PR calls. Measured premises (inventory sections):
no pin load path refuses an unknown action type (`actions.go:943-944`, § 3); six decode paths disagree on absent keys
(§ 4b); four load sites log and skip (§ 3), which the epic-9 ruling forbids. Pin probe: the 34 tests of PR 2's files
pass once under `-race` and five times shuffled (seed `1795617175502578000`, § 2); decoding into raw JSON keeps the
bytes, `json.Marshal` compacts and HTML-escapes them (go1.26.11, § 7). semteams: 85 rules, 124 actions; semboids: 7
rules (§ 12). Size: 943 production and 388 test pin lines carried and rewritten (ranges in `tasks.md` 2.9), plus new
tests. `processor/rule` is whole when PR 12 lands; until PR 8 its callers are its own tests (split plan option A).

## Decisions

**D1. What PR 2 carries, and which names are exported.** Framework API (target-shape page § 1-2; contract line: the
`rule-definitions` spec): `Definition` and `EntityConfig` (semteams decodes them in 10 test files, § 6), `Action`,
`ActionHandler`, `DecodeDefinition` and `Load` (a product checks its packs with its own handlers), `Rule` (what `Load`
returns; fields unexported), the nine built-in type names (semteams tests name one, § 6), and
`ErrorCodeEntityWatchBucketUnsupported` (the code on that refusal, named by the spec; no consumer reads it today).
Unexported: the cap key, `LoadedAction`, the default cap. Not carried, by ruling: `Factory`, `Dependencies`, `Schema`,
the registry functions, `interfaces.go`, `docs/custom-rules.md`, the map round trip, the agentic fields and checks,
deny/approve; nor `test_rule_factory.go` (nothing to register into). Moved to PR 8 because they read `Config` or
`Processor`: the file loader `rule_loader.go:55-99` (a departure from the split plan's PR 2 list, put to the owner),
`schema.go` (`:12` reflects `Config{}`) and `status.go`. From `entity_pattern_contract.go`, `Load` takes
`validateDefinitionEntityPattern` (`:49-65`), `unsupportedEntityWatchBucket` (`:67-77`) and `:13-17`; the
watch-bucket helpers (`:19-47`, `:79-121`) move with their readers in PRs 8, 10 and 12.

**D2. The handler type arrives in two steps.** PR 2's `ActionHandler` has `Check(a Action) error`; PR 4 adds `Run`
once PR 3 has brought `ExecutionContext`. Options: an empty exported `ExecutionContext` now (a type that promises what
it does not hold); no handler type until PR 4 (the type set spelled twice). Adding a method breaks no one: every
implementer is in this repository until PR 8 registers products.

**D3. Load refuses, never skips (ruled); disabled rules are checked.** `Load(def, source, handlers)` compiles the
conditions and each `when` (PR 1's `Compile`) and checks every action through its handler. It returns an error
classified invalid naming rule ID, source, position (`on_enter[1].when[0]`) and cause. Disabled rules are checked
too, as `prepareInitialRules` already validated them (`rule_loader.go:116-119`); the other option, checking only
enabled rules, lets a broken rule wait. Sources: a rules file's configured path, `inline_rules[<index>]`,
`rules.<id>@<revision>`. PR 8 fails construction on a refusal; PR 12 rejects the candidate set (ADR-094). Proof:
`TestLoadRefuses`, one case per spec refusal, asserting class, rule, source and position.

**D4. One decode, strict.** `DecodeDefinition(data []byte, source string) (Definition, error)` decodes one rule object
with `DisallowUnknownFields`, so a key `Definition` or a condition does not declare is refused naming key, rule and
source. `Action` has its own unmarshaler, which keeps the object as `Body` and decodes the four core keys strictly
itself, so a misspelt key in a `when` condition is refused naming `when[<i>]`; the decoder's refusal stops at the body
(PR 1, D2 option C), so body keys are the handler's `Check` (D6). Absent keys: `enabled` true, `required` false
(files' meaning today; semboids' 14 conditions rely on it), `logic` `and`, `name` empty. Callers: PR 8 reads a rules
file (one object or an array, each element decoded with the file as source) and keeps `inline_rules` as raw elements,
each decoded with `inline_rules[<index>]`; PR 12 decodes each stored value with `rules.<id>@<revision>`. Proof in PR
2: `TestDecodeDefinition`. A file's bytes and the same rule marshalled and read back (compacted, escaped) decode
equal; undeclared keys refused, a `when[0]` one included; the absent-key meanings. The inline and bucket paths are
proved by PRs 8 and 12. Other option: refuse undeclared keys in `inline_rules` only; typos elsewhere stay silent.

**D5. Fields with no engine behaviour.** `metadata` stays as author notes the engine never reads (85 semteams rules);
the claim that it is substituted goes. Rule-level `max_iterations` stays as the value `$state.max_iterations` reads,
documented as not a limit (7 rules). `related_patterns` is dropped (0 rules, no reader), so D4 refuses it.
`on_recovery`, `rerun_on_recovery`, `while_true` stay (ADR-111 Decision 5 relies on the first two); a
`rerun_on_recovery` with no `on_enter` or `on_recovery` is refused at load (today only dead code checks it,
`rule_factory.go:266`). Other options: enforce `max_iterations` (changes 7 rules); drop `metadata` (refuses 85).

**D6. Built-in checks.** Each of the nine built-ins refuses body keys neither the core nor its type reads (the pin
handlers' key sets, `tasks.md` 2.4) and keeps the pin's load checks: `publish` refuses a declared subject in
`user.response.>` (ADR-093's load half; the after-substitution half becomes PR 4's spec requirement); the triple
types need a literal, declared predicate (ADR-074); `max_iterations` is 0 or more (`config_validation.go:340-393`).
Checks that need configuration come in PRs 4 (triple writer) and 5 (reconcile targets, `update_kv` buckets #294,
lifecycle manager). Proof: `TestBuiltinCheck`; a one-off run over semteams' and semboids' checked-in packs, recorded
in `notes/2.4.md`, shows no built-in action refused for a key.

**D7. Cron before PR 11: refuse only enabled cron rules.** semteams lists its disabled cron rule in both flows
(`configs/flow-bootstrap.json:366`, `configs/e2e-flow-bootstrap.json:331` at 4df44ae8); the pin skips it before
`NewCronRule` (`rule_loader.go:185-188`). Options: (a) refuse every cron rule: with D3, semteams' processor fails to
start from PR 8 to PR 11 unless it edits both flows; (b, recommended) refuse an enabled cron rule, and give a disabled
one the common checks only (its schedule is checked from PR 11); cost: a disabled cron rule's own fields go
unchecked until PR 11; (c) accept cron and run nothing: silent. Proof: in `TestLoadRefuses`.

**D8. The firing-cap key (ruled; consequences).** `id`, else a SHA-256 over rule ID, type and the canonical body
(`UseNumber`, sorted keys, no spacing, no HTML escaping), replacing `action_id.go:37-106`. `max_iterations` and
`when` are in the body, so raising a cap from 3 to 5 gives 5 fresh firings; the pin's key excluded them and kept the
count (2 more). Pin-keyed stored counts are not migrated (deployments start fresh). The reconcile request ID (PR 5,
`actions.go:1394-1400`) is built from the key: an edit changes it, but within one firing it is stable, so a
redelivered write keeps its key (ADR-111 Decision 5). Other options: drop `max_iterations` from the hashed body (keeps
the pin's behaviour for a raised cap; departs from the ruled "body as written"); a separate request key (a second
spelling). Proof: `TestCapKey`.

**D9. #26, byte preservation (accepted proof).** `Body` is the action object as written. `TestBodyKeepsBytes` reads
semteams' fixtures at `a331910e` (MIT), copied to `processor/rule/testdata/rule-family-consumer/` with a README naming
repository, commit and paths: each `on_enter[0].Body` equals its bytes in the file, and its `tool_choice` and
`response_format` decode equal to `expected.json`.

**D10. The guard exception goes.** `processor/rule.Dependencies.Platform` leaves `authority_test.go:232`; the
fixture's `processor/rule/rule.go:6` moves from the silent list (`:194`) to the failing list, the rest stays. The
guard never reports an unused exception (§ 4i); kept until PR 8, a later struct of that name would pass unchecked.

## ADRs

Kept: ADR-036 (rule-opaque refusal), ADR-074 (declared predicate), ADR-093 (reserved space, load half here), ADR-031
(cron fields decoded; running waits, D7), ADR-111 Decision 5 (recovery keys). Applied later: ADR-094 (PR 12),
ADR-102/104 (`deps.Platform` to the processor, PR 8). Dropped, each ruled: ADR-039; the fields of ADR-023, 034, 046,
053, 067 (Q15); ADR-076 decisions 1-3 and 5. ADR-036 and ADR-093 are not ported to `docs/adr`: the binding part of
each is one rule-side clause, carried as a spec requirement; the rest of each is agentic.

## Issues and merge order

`Closes #26`. Named without a closing keyword:

- #25: the load half (unknown type refused at load; semboids' pack loads with no product handler); fire half PR 7.
- #27, #28: agentic checks and `VerdictAuditor` not carried; they retire at PRs 4 and 7.
- #294 (PR 5); #702 (default cap carried; PR 7); #703 (its keys join `Definition` in PR 14, as D4 requires).

Stay, no PR 2 work: #195 (no NATS client here), #196 (no lifecycle owner here; PRs 8, 11, 12), #350 (PR 5's
reconcile path), #714 (PR 13), #704 (decision epic). PR #753 merges first: PR 2 calls `expression.Compile` and edits
the same ledger, repository map and coverage list; this branch then merges `origin/main`.

Risks: the handler interface gains `Run` in PR 4 (D2); every caller of `Load` lands in PR 8 or later; tests
comparing conditions with `assert.Equal` see the compiled pattern (PR 1).
