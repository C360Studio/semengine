# Tasks: rule-core-02-definitions

Gates per code task: `task verify` on a clean tree. Pin paths are SemStreams `8b99efe9`, `processor/rule/`.

## 1. Design

- [ ] 1.1 The inventory review passes, the design review passes, Fable reads the design; the owner fills `Approved-by:`.
  OPEN-Q: the owner's approval of the proposal.
  Hold: Fable reads the design before any code (new exported framework surface, `design.md` D1-D4).
- [ ] 1.2 Start implementation once PR #753 has merged; merge `origin/main` into this branch first.
  Hold: PR #753 (`rule-core-01-expression`) not yet merged.

## 2. Port

- [ ] 2.1 Write the tests before the code (failing-first). Then run `task mutate:check` on each refusal and record
  the result as detected or survived in `notes/2.1.md`.
  - `TestDecodeDefinition` (D4): the same rule from file bytes, inline config and a stored KV value decodes equal; an
    undeclared key and `related_patterns` are refused, naming key, rule and source; absent `enabled`, `required` and
    `logic` mean true, false and `and`.
  - `TestLoadRefuses` (D3, D5, D7): an unknown action type; a bad or templated regex; a rule-opaque or undeclared
    condition field; a dotted ID; negative `fire_every_n_events` or `max_iterations`; `rerun_on_recovery` with nothing
    to re-run; `type: "cron"`; a disabled rule with an unknown type. Each case checks `errs.IsInvalid`, the rule ID,
    the source, the position and the cause.
  - `TestBuiltinCheck` (D6).
  - `TestCapKey` (D8).
  - `TestBodyKeepsBytes` over the semteams fixtures (D9).
  - The semboids pack `configs/rules/zone-steering/*.json` (ab4e0fbc), copied to testdata, loads with no product
    handler.
- [ ] 2.2 `Definition` and `EntityConfig` from `rule_factory.go:15-76`, less `related_patterns` (D5). `DecodeDefinition`
  with undeclared keys refused (D4). Doc comments state that `metadata` is author notes and that rule-level
  `max_iterations` is not a limit (D5).
- [ ] 2.3 `Action` (`Type`, `When`, `ID`, `MaxIterations`, `Body`) replacing `actions.go:89-444`. The nine built-in
  type names from `actions.go:31-76`, without `publish_agent`, `deny` and `approve`. `ActionHandler` with `Check`
  only (D2). `LoadedAction`. The cap key replacing `action_id.go:37-106` (D8); `DefaultActionMaxIterations` kept
  (`:17`).
- [ ] 2.4 Built-in `Check`s (D6) with these body key sets (pin readers in brackets):
  - `publish`: `subject`, `properties` [`actions.go:1150-1221`];
  - `add_triple`: `subject`, `predicate`, `object`, `ttl` [`:973-1080`];
  - `remove_triple`: `subject`, `predicate`, `object` [`:1081-1133`];
  - `update_triple`: `subject`, `predicate`, `object`, `ttl`, `properties` [`:1222-1327`];
  - `reconcile_predicates`: `subject`, `predicate`, `object`, `ttl`, `projection_contract`, `projection_group`
    [`:1328-1444`];
  - `update_kv`: `bucket`, `key`, `payload`, `merge` [`:2253-2310`];
  - `lifecycle_transition`: `workflow`, `phase`, `reason`, `set`; `lifecycle_complete`: `workflow`;
    `lifecycle_fail`: `workflow`, `reason` [`actions_lifecycle.go:35-165`].

  Load checks: the reserved subject (`config_validation.go:340-345`, `user_response_subject_reservation.go`) and the
  literal, declared predicate (`:370-381`). Check: no checked-in semteams or semboids built-in action is refused (a
  test over the copied packs).
- [ ] 2.5 `Rule` and `Load` (D3), carrying `ValidateDefinition`'s checks (`config_validation.go:273-327`), the live
  factory checks (`expression_factory.go:395-430`), the `rerun_on_recovery` check (`rule_factory.go:266`), the cron
  refusal (D7), and `expression.Compile` on conditions and on each `when`. Also the entity-pattern checks
  (`entity_pattern_contract.go:13-77`; `:79-121` moves with its PR 8/12 readers if unused here).
- [ ] 2.6 Carry these tests, rewritten to the harness rules and to `Load`: `config_validation_test.go:113-250` and
  `:461-493`, `entity_pattern_contract_test.go:15-62`, `rule_id_token_test.go`, `action_id_test.go` (rewritten for
  D8), `user_response_subject_reservation_test.go:10-54` and `predicate_authoring_test.go`. Imports go through
  `internal/harness/semantictest`.
- [ ] 2.7 D10: remove `processor/rule.Dependencies.Platform` from `internal/harness/contract/authority_test.go:232` and
  its fixture (`:151-194`), per the spec delta. Check: `go test ./internal/harness/contract/`.
- [ ] 2.8 Ledger row `processor/rule` (`adapt`, partial: names what PRs 3-12 add). Add `processor/rule unit` to
  `scripts/cover-check.sh`. Add a `docs/repository-map.md` row. Check: `task ledger:check`, `task cover:check` at 80%
  or more.

## 3. Docs

- [ ] 3.1 Rewrite `doc.go` to describe what this PR carries: the definition, the decode, `Load`, action handlers and
  the cap key. No factory, `init` or graph-event text.
- [ ] 3.2 `processor/rule/testdata/rule-family-consumer/README.md` names semteams, commit `a331910e` and each source
  path. Check: `task docs:check`.

## 4. Land

- [ ] 4.1 Sync `openspec/specs/rule-definitions/` and `harness-boundaries/` and archive the change as the PR's last
  content commit. Check: `task spec:check`.
