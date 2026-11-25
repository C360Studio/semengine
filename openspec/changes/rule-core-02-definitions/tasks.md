# Tasks: rule-core-02-definitions

Gates per code task: `task verify` on a clean tree. Pin paths are SemStreams `8b99efe9`, `processor/rule/`.

## 1. Design

- [ ] 1.1 The inventory review passes, the design review passes, Fable reads the design; the owner fills `Approved-by:`.
  Hold: OPEN-Q proposal awaits owner approval
- [ ] 1.2 Fable reads this design before any code (new exported framework surface, `design.md` D1-D4).
  Hold: the owner's Fable read of the design on PR #758.
- [ ] 1.3 Start implementation once PR #753 has merged; merge `origin/main` into this branch first.
  Hold: PR #753 (`rule-core-01-expression`) not yet merged.

## 2. Port

- [ ] 2.1 Tests come before the code, each seen to fail first. After 2.5, run `task mutate:check` on each refusal and
  record detected or survived in `notes/2.1.md`.
  - `TestDecodeDefinition` (D4): a file's bytes and the same rule marshalled and read back decode equal; an
    undeclared key, `related_patterns` and an undeclared condition key are refused, naming key, rule and source;
    absent `enabled`, `required` and `logic` mean true, false and `and`.
  - `TestLoadRefuses` (D3, D5, D7): an unknown action type; a bad or templated regex; a rule-opaque or undeclared
    condition field; a dotted ID; negative `fire_every_n_events` or `max_iterations`; `rerun_on_recovery` with nothing
    to re-run; an enabled `type: "cron"`; a disabled rule with an unknown type. Each checks `errs.IsInvalid`, rule ID,
    source, position and cause. A disabled cron rule loads.
  - `TestBuiltinCheck` (D6), `TestCapKey` (D8), `TestBodyKeepsBytes` (D9).
  - semboids' `configs/rules/zone-steering/{food,predator,wind}.json` (ab4e0fbc, MIT) are copied to
    `processor/rule/testdata/semboids-zone-steering/` with a README naming repository, commit and paths. They load
    with no product handler. Their condition fields are all `$message.*`, so no predicate needs declaring.
- [ ] 2.2 `Definition` and `EntityConfig` from `rule_factory.go:15-76`, less `related_patterns` (D5).
  `DecodeDefinition(data, source)` with `DisallowUnknownFields` and the absent-key meanings (D4). Doc comments state
  that `metadata` is author notes and that rule-level `max_iterations` is not a limit (D5).
- [ ] 2.3 `Action` (`Type`, `When`, `ID`, `MaxIterations`, `Body`) replacing `actions.go:89-444`, with an unmarshaler
  that keeps the object as `Body`. The nine built-in type names from `actions.go:31-76`, without `publish_agent`,
  `deny` and `approve`. `ActionHandler` with `Check` only (D2). The unexported cap key replacing
  `action_id.go:37-106` (D8), with the default cap of 3 (`:17`).
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
  literal, declared predicate (`:370-381`). Record in `notes/2.4.md` a one-off run of the key checks over semteams'
  85 checked-in rules (4df44ae8) and semboids' 7 (ab4e0fbc), with the predicate check left out (semteams' predicates
  are declared by its own vocabulary, not present here). Expected: no built-in action refused for a key.
- [ ] 2.5 `Rule` and `Load(def, source, handlers)` (D3). It carries `ValidateDefinition`'s checks
  (`config_validation.go:273-327`), the live factory checks (`expression_factory.go:395-430`), the
  `rerun_on_recovery` check (`rule_factory.go:266`) and the enabled-cron refusal (D7). It calls `expression.Compile`
  on the conditions and on each `when`. Also the definition's entity-pattern check: `entity_pattern_contract.go:13-17`,
  `:49-77`. The watch-bucket helpers `:19-47` and `:79-121` stay out (PRs 8, 10, 12).
- [ ] 2.6 Carry these tests, rewritten to the harness rules and to `Load`: `action_id_test.go:1-137` (rewritten for
  D8), `config_validation_test.go:113-245` and `:461-493`, `entity_pattern_contract_test.go:42-51`,
  `rule_id_token_test.go:13-29`, `user_response_subject_reservation_test.go:10-53` and `predicate_authoring_test.go`.
  Imports go through `internal/harness/semantictest`.
- [ ] 2.7 D10: delete `authority_test.go:232`, and move `"processor/rule/rule.go:6:"` from the silent list (`:194`)
  to the failing list. Keep the rest of the fixture (`:151-167`). Check: `go test ./internal/harness/contract/`.
- [ ] 2.8 Ledger row `processor/rule` (`adapt`, partial: names what PRs 3-12 add). Add `processor/rule unit` to
  `scripts/cover-check.sh`. Add a `docs/repository-map.md` row. Check: `task ledger:check`, `task cover:check` at 80%
  or more.
- [ ] 2.9 The PR body states the carried pin lines, which reproduce with `sed -n a,bp | wc -l`:
  - production 943: `rule_factory.go:15-76`, `actions.go:31-76` and `:89-444`, `action_id.go:1-133`,
    `config_validation.go:256-443` less `:330-335`, `:346-367`, `:382-387` and `:394-412`,
    `entity_pattern_contract.go:1-17` and `:49-77`, `user_response_subject_reservation.go:1-23`, `doc.go:1-142`;
  - tests 388: the 2.6 ranges.

## 3. Docs

- [ ] 3.1 Rewrite `doc.go` to describe what this PR carries: the definition, the decode, `Load`, action handlers and
  the cap key. No factory, `init` or graph-event text.
- [ ] 3.2 READMEs for `processor/rule/testdata/rule-family-consumer/` (semteams, `a331910e`) and
  `testdata/semboids-zone-steering/` (semboids, `ab4e0fbc`), each naming repository, commit, paths and licence.
  Check: `task docs:check`.

## 4. Land

- [ ] 4.1 Sync `openspec/specs/rule-definitions/` and `harness-boundaries/` and archive the change as the PR's last
  content commit. Check: `task spec:check`.
