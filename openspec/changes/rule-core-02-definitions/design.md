# Design: rule-core-02-definitions

Base: `738e9bca` (origin/main `da704d07`). Pin: SemStreams `8b99efe9`. Inventory: `inventory.md` (INVENTORY PASS,
round 2). Givens, not reopened: the target-shape page (`archive/2026-11-25-design-rule-core-target-shape/`, §§ 1-2
and its approved proposal), the shape rulings on PR #752 (comment 7162146299, items 1, 2, 6, 9; addendum 7162180652)
and the rulings on epic 9 (comments 7160955176, 7165028479). Spec delta: `specs/rule-definitions/` (new) and
`specs/harness-boundaries/` (two requirements modified).

## Context

PR 2 lands the rule definition and the one load step every later PR calls. Measured premises (inventory sections):

- No pin load path refuses an unknown action type; the switch refuses it at fire (`actions.go:943-944`, § 3).
- Six decode paths with different defaults for an absent `enabled`, `required`, `name` and `logic` (§ 4b); four
  load sites log and skip a bad rule with no metric (§ 3), which the ruling on epic 9 forbids.
- Pin probe: the 34 tests of PR 2's files pass once under `-race` and five times shuffled (seed
  `1795617175502578000`, § 2). Decoding into raw JSON keeps the bytes; `json.Marshal` compacts and HTML-escapes them
  (go1.26.11 probe, § 7).
- `ActionHandler.Run` takes `*ExecutionContext`, which PR 3 ports (§ 6). semteams: 85 rules, 124 actions; semboids:
  7 rules (§ 12).

Size: about 1,018 production and 467 test pin lines carried (ranges in `tasks.md`), rewritten, plus new tests; under
the 3,000-line guide. `processor/rule` is whole when PR 12 lands; until PR 8 its callers are its own tests (the
carried-in-tree precedent, split plan option A).

## Decisions

**D1. What PR 2 carries.** `Definition`, `EntityConfig`, `Action` (`Type`, `When`, `ID`, `MaxIterations`, `Body`),
`Rule`, `LoadedAction`, `ActionHandler`, `DecodeDefinition`, `Load`, the nine built-in type names, the cap key, the
entity-pattern checks, the reserved-subject check and a rewritten `doc.go`. Not carried here: what the rulings drop
(`Factory`, `Dependencies`, `Schema`, the registry functions, `interfaces.go`, `docs/custom-rules.md`, the map round
trip, agentic fields and checks, deny/approve); `test_rule_factory.go` (nothing left to register into). Moved to PR
8, because they read `Config` or `Processor`: `schema.go` (`:12` reflects `Config{}`), `status.go`, and the file read
in `rule_loader.go:55-99`, which calls `DecodeDefinition`. Other option: carry `schema.go` and `status.go` now;
`schema.go` does not compile without `Config`.

**D2. The handler type arrives in two steps.** PR 2's `ActionHandler` has `Check(a Action) error`; PR 4 adds `Run`
once PR 3 has brought `ExecutionContext`. Options: (b) an empty exported `ExecutionContext` now that PR 3 fills, a
type that promises what it does not hold; (c) no handler type until PR 4, which spells the type set twice in PR 2.
Every implementer is in this repository until PR 8 registers products, so adding a method breaks no one.

**D3. Load refuses, never skips.** `Load(def, source, handlers)` compiles conditions and each `when` (PR 1's
`Compile`) and checks every action through its handler. It returns an error classified invalid naming rule ID,
source, position (`on_enter[1]`) and cause. Disabled rules are checked too, as `prepareInitialRules` does
(`rule_loader.go:116-119`). The source is a rules file's configured path, `inline_rules[<index>]` for an inline rule,
or `rules.<id>@<revision>` for a KV rule. How later PRs use it: at boot a refusal fails the processor's construction
(PR 8); on hot reload it rejects the whole candidate set and the previous generation keeps running (ADR-094; PR 12).
Other option: skip with a log and a metric, which the ruling on epic 9 forbids. Proof: `TestLoadRefuses`, one case
per refusal in the spec, asserting the class and the named rule, source and position.

**D4. One decode, strict.** `DecodeDefinition` refuses a key `Definition` does not declare, naming key, rule and
source; no checked-in rule uses one (§ 12). An absent key means: `enabled` true (no rule omits it), `required` false
(files' meaning today; semboids' 14 conditions and semteams' 7 `when` conditions omit it), `logic` `and` (PR 1),
`name` empty. Other option: refuse undeclared keys only in `inline_rules`, as the unknown-config-key rule requires; a
typo in a file or a KV rule then stays silent. Proof: `TestDecodeDefinition` decodes one rule from file bytes, an
inline config and a stored KV value, and checks that the three are equal.

**D5. Fields with no engine behaviour.** `metadata` stays as author notes the engine never reads (85 semteams rules),
and the doc's claim that it is substituted into actions goes. Rule-level `max_iterations` stays as the value
`$state.max_iterations` reads, documented as not a limit (7 rules). `related_patterns` is dropped (0 rules, no
reader), so D4 refuses it. `on_recovery`, `rerun_on_recovery` and `while_true` stay (0 rules; ADR-111 Decision 5
relies on the first two). A `rerun_on_recovery` with no `on_enter` or `on_recovery` is refused at load; today only
dead code checks it (`rule_factory.go:266`). Other options: enforce `max_iterations` (changes 7 rules); drop
`metadata` (refuses 85 rules).

**D6. Built-in checks.** Each of the nine built-ins refuses body keys that neither the core nor its type reads (the
pin handlers' key sets, `tasks.md` 2.4) and keeps the pin's load checks: `publish` refuses a declared subject in
`user.response.>` (ADR-093); the triple types need a literal, declared predicate (ADR-074); `max_iterations` must be
0 or more (`config_validation.go:340-393`). Checks that need configuration come later: a missing triple writer (PR
4); reconcile targets, `update_kv` buckets (#294) and the lifecycle manager (PR 5). No checked-in built-in action is
refused. Proof: `TestBuiltinCheck`, one accepted and one refused body per type.

**D7. Cron waits for PR 11.** `Load` refuses `type: "cron"` until PR 11, naming rule and source; semteams' one cron
rule is disabled but checked (D3). Other option: accept cron definitions and run nothing, which is silent.

**D8. The firing-cap key (ruled; consequences).** The key is `id`, or else a SHA-256 over rule ID, type and the body
in canonical form (`UseNumber`, sorted keys, no spacing, no HTML escaping); it replaces `action_id.go:37-106`. Three
readers: the cap gate (PR 7); stored `action_iterations` (PRs 3, 7), where pin-keyed counts are not migrated because
deployments start fresh; and the reconcile request ID (PR 5, `actions.go:1394-1400`), which an edit changes but which
stays stable within one firing, so a redelivered write keeps its key (ADR-111 Decision 5). Other option: a separate
request key, which is a second spelling. Proof: `TestCapKey`: the key is equal across file bytes and their stored,
compacted and escaped form, and across a formatting-only edit; it differs on any value edit; `id` wins.

**D9. #26, byte preservation (accepted proof).** `Body` is the action object as written. The test reads semteams'
fixtures at `a331910e`, copied to `processor/rule/testdata/rule-family-consumer/` with a README naming repository,
commit and path. It checks that each `on_enter[0].Body` equals its bytes in the file, and that the `tool_choice` and
`response_format` values inside decode equal to `expected.json`.

**D10. The guard exception goes.** With `Dependencies` gone, `processor/rule.Dependencies.Platform` leaves
`authority_test.go:232`, its fixture (`:151-194`) and the spec; the guard never reports an unused exception (§ 4i).
Other option: keep it until PR 8 places the platform, and a later struct of that name passes unchecked.

## ADRs

Kept: ADR-036 (rule-opaque refusal), ADR-074 (declared predicate), ADR-093 (reserved space), ADR-031 (cron fields
decoded; running them waits, D7) and ADR-111 Decision 5 (recovery keys). Applied later: ADR-094 (PR 12) and ADR-102
and 104, with `deps.Platform` reaching the processor (PR 8). Dropped, each already ruled: ADR-039; the fields of
ADR-023, 034, 046, 053 and 067 (Q15); ADR-076 decisions 1-3 and 5. ADR-036 and ADR-093 are not ported to `docs/adr`:
the binding part of each is one rule-side clause, carried as a spec requirement, and the rest of each is agentic.

## Issues and merge order

`Closes #26`. Named without a closing keyword:

- #25: its load half (an unknown type is refused at load; semboids' pack loads with no product handler); the fire
  half is PR 7's.
- #27 and #28: the agentic checks and `VerdictAuditor` are not carried; they retire at PRs 4 and 7.
- #294 (PR 5); #702 (the default cap is carried; PR 7); #703 (its keys join `Definition` in PR 14, as D4 requires).

PR #753 merges first: PR 2 calls `expression.Compile` and edits the same ledger, repository map and coverage list.
This branch then merges `origin/main`.

## Risks

- The handler interface gains `Run` in PR 4 (D2), and every caller of `Load` lands in PR 8 or later.
- Tests comparing conditions with `assert.Equal` see the compiled pattern (PR 1).
