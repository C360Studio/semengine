# Tasks: rule-core-02-definitions

Gates per code task: `task verify` on a clean tree. Pin paths are SemStreams `8b99efe9`, `processor/rule/`.

## 1. Design

- [x] 1.1 The inventory review passes, the design review passes, Fable reads the design; the owner fills `Approved-by:`.
  Inventory PASS 738e9bca; design PASS add0458d; owner approval PR #758 comment 7168133081.
- [x] 1.2 Fable reads this design before any code (new exported framework surface, `design.md` D1-D4).
  Fable APPROVE at 11bcabaa, PR #758 comment 7168081578 (first read 7166853645, REQUEST CHANGES).
- [x] 1.3 Start implementation once PR #753 has merged; merge `origin/main` into this branch first.
  PR #753 merged as 27023729; `origin/main` merged into this branch.

## 2. Port

- [ ] 2.1 Tests come before the code, each seen to fail first. After 2.5, run `task mutate:check` on each refusal and
  record detected or survived in `notes/2.1.md`.
  - `TestDecodeDefinition` (D4): a rule decoded from a file's bytes, marshalled and decoded again has every field
    equal and each `Body` equal in canonical form, though not byte for byte (a body holding a space and `&`); an
    undeclared key, `related_patterns`, `entity.watch_buckets` and an undeclared condition key are refused, naming key,
    rule and source, and a misspelt key in a `when` condition is refused naming `when[0]`; absent `enabled`, `required`
    and `logic` mean true, false and `and`.
  - `TestLoadRefuses` (D3, D5, D7): an unknown action type; a bad or templated regex; a rule-opaque or undeclared
    condition field; a dotted ID; negative `fire_every_n_events` or `max_iterations`; `rerun_on_recovery` with nothing
    to re-run; an enabled `type: "cron"`; a disabled rule with an unknown type; a handler set `NewHandlers` did not
    build. Each checks `errs.IsInvalid`, rule ID, source, position and cause. A disabled cron rule loads.
  - `TestNewHandlers` (D3): with no product handlers the set holds the nine built-ins; a product handler named
    `publish`, an empty name and a nil handler are refused, classified invalid, naming the type.
  - `TestActionBody` (D4): an action written as a Go literal with `Type` set and no `Body` is refused by `Load`, and so
    is a decoded action whose `Type` was changed, each saying to build it by decoding; marshalling writes `Body`;
    `Load` twice on one decoded definition succeeds both times and leaves it deep-equal.
  - `TestBuiltinCheck` (D6), `TestCapKey` (D8), `TestBodyKeepsBytes` (D9).
  - semboids' `configs/rules/zone-steering/{food,predator,wind}.json` (ab4e0fbc, MIT) are copied to
    `processor/rule/testdata/semboids-zone-steering/` with a README naming repository, commit and paths. They load
    with the set `NewHandlers(nil)` builds. Their condition fields are all `$message.*`, so no predicate needs
    declaring.
- [ ] 2.2 `Definition` and `EntityConfig` from `rule_factory.go:15-76`, less `related_patterns` (D5) and
  `EntityConfig.WatchBuckets` (`:75`, ruled on PR #758).
  `DecodeDefinition(data, source)` with `DisallowUnknownFields` and the absent-key meanings (D4). Doc comments state
  that `metadata` is author notes and that rule-level `max_iterations` is not a limit (D5).
- [ ] 2.3 `Action` (`Type`, `When`, `ID`, `MaxIterations`, `Body`) replacing `actions.go:89-444`, with an unmarshaler
  that keeps the object as `Body` and a `MarshalJSON` that writes it; its doc comment says `Body` is the action and the
  four fields are read-only copies the decode fills (D4). The nine built-in type names from `actions.go:31-76`, without
  `publish_agent`, `deny` and `approve`. `ActionHandler` with `Check` only (D2); `Handlers` and `NewHandlers` (D3).
  The unexported cap key replacing `action_id.go:37-106` (D8), with the default cap of 3 (`:17`).
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
  `rerun_on_recovery` check (`rule_factory.go:266`), the enabled-cron refusal (D7), marked
  `// shortcut: enabled cron rules are refused, until the rule processor has a timer`, and the refusal of an action
  with no `Body` or with fields that differ from it (D4). It calls `expression.Compile` on the conditions and on each
  `when`. Also the entity-pattern check: `entity_pattern_contract.go:49-65` less the watch-bucket lines `:50-54` and
  `:56-58`. Not carried, ruled on PR #758: `:13-17` (`ErrorCodeEntityWatchBucketUnsupported` and its error) and
  `:67-77` (`unsupportedEntityWatchBucket`); the processor-setting helpers `:19-47` and `:79-121` stay out (PRs 8, 10,
  12).
- [ ] 2.6 Carry these tests, rewritten to the harness rules and to `Load`: `action_id_test.go:1-137` (rewritten for
  D8), `config_validation_test.go:113-245` and `:461-493`, `entity_pattern_contract_test.go:42-51`,
  `rule_id_token_test.go:13-29`, `user_response_subject_reservation_test.go:10-53` and `predicate_authoring_test.go`.
  Imports go through `internal/harness/semantictest`. Their `Action{` literals (19 in those ranges) become decoded
  JSON (D4).
- [ ] 2.7 D10: delete `authority_test.go:232`, and move `"processor/rule/rule.go:6:"` from the silent list (`:194`)
  to the failing list. Keep the rest of the fixture (`:151-167`). Check: `go test ./internal/harness/contract/`.
- [ ] 2.8 Ledger row `processor/rule` (`adapt`, partial: names what PRs 3-12 add). Add `processor/rule unit` to
  `scripts/cover-check.sh`. Add a `docs/repository-map.md` row. Check: `task ledger:check`, `task cover:check` at 80%
  or more.
- [ ] 2.9 The PR body states the carried pin lines, which reproduce with `sed -n a,bp | wc -l`:
  - production 879: `rule_factory.go:15-76`, `actions.go:31-76` and `:89-444`, `action_id.go:1-106`,
    `config_validation.go:256-443` less `:330-335`, `:346-367`, `:382-387` and `:394-412`,
    `entity_pattern_contract.go:49-65` less `:50-54` and `:56-58`, `user_response_subject_reservation.go:1-23`,
    `doc.go:1-142`;
  - tests 388: the 2.6 ranges.

## 3. Docs

- [ ] 3.1 Rewrite `doc.go` to describe what this PR carries: the definition, the decode, `Load`, action handlers, the
  handler set and the cap key. No factory, `init` or graph-event text.
- [ ] 3.2 READMEs for `processor/rule/testdata/rule-family-consumer/` (semteams, `a331910e`) and
  `testdata/semboids-zone-steering/` (semboids, `ab4e0fbc`), each naming repository, commit, paths and licence.
  Check: `task docs:check`.

## 4. Land

- [ ] 4.1 Sync `openspec/specs/rule-definitions/` and `harness-boundaries/` and archive the change as the PR's last
  content commit. Check: `task spec:check`.
