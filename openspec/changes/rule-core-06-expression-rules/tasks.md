# Tasks: rule-core-06-expression-rules

Gates per code task: `task verify` on a clean tree. Pin paths are SemStreams `8b99efe9`, `processor/rule/`.

## 1. Design

- [ ] 1.1 The inventory review passes, the design review passes; the owner fills `Approved-by:`.
  Hold: owner re-approval on the new digest
- [x] 1.2 Fable answers, before any code, "Is `Matches` as a method of a loaded rule (with the path and pattern
  refusals) plus the `ErrRequiredFieldMissing` sentinel the right exported shape?" (`design.md` D2, D4). Read: comment
  7223972395; owner ruled 7223995054, all three as recommended; no further read.

## 2. Port

- [ ] 2.1 Tests come before the code, each seen to fail first; after 2.5, run `task mutate:check` on each refusal and
  record detected or survived in `notes/2.1.md`.
  - `TestMatches` (D2, D3): the carried `matches_test.go` cases (`:86-311`, `:340-403`, `:444-464`) keep their
    hand-written verdicts, but for `:229-260`, whose absent required field now gives false and no error (D4) and whose
    error case becomes `gt "abc"` on a number; their inputs gain an `entity.pattern` and a six-part entity ID in place
    of `"test.entity.id"`, and the rules are built with `DecodeDefinition` and `Load`. In D2's order: a nil rule,
    context and state are refused; a malformed entity ID is an error, for a disabled rule too; a loaded disabled cron
    rule (no pattern, no conditions), an enabled rule with no `entity.pattern`, and an entity outside the pattern each
    give false and no error; a `$entity.lifecycle.` condition is refused, the error saying it cannot be answered without
    a lifecycle lookup; a rule with a cooldown is answered, not refused.
  - `TestRequiredFieldMissing` (D4): an entity lacking a required field gives no match and no error from the firing
    match and from `Matches`; a message lacking a required `$message.` field gives the same from the firing message
    match; under `or`, an absent required field gives no match though another condition holds; a type error
    (`gt "abc"` on a number) is an error on both and does not wrap the sentinel.
  - `TestUnresolvedValueTemplate` (D5): a value `$entity.triple.x.y.missing` against a triple holding that text is an
    error naming rule, field and token on both paths, and nothing is logged; semteams' `research/02-plan-to-gather.json`
    and `03b-synthesize-when-all-gathers-complete.json`, copied to `testdata/` with a README naming repository, commit
    and paths, load with `NewHandlers` given a fake `publish_agent` handler (its `Check` accepts any body) after the
    test registers their condition predicates with `vocabulary.Register` (`agent.loop.role`,
    `coordinator.decision.next-action`, `coordinator.decision.subtopics`, `research.gather.completed-subtopic`), and
    match an entity built to satisfy them through the firing match and `Matches`.
  - `TestLoadRefuses` gains two cases (D6): `$entity.id` as a condition value in a rule with no `entity.pattern`, and
    a `$message.x` field in a rule with one, each refused at `conditions[0]`, enabled or not.
  - `TestMessageMatch` (D7): the carried projection tests (typed payload declaring a subset; `GenericJSON`
    unchanged; an undeclared field not reachable; a legitimate false logs nothing), using the file's
    `declaredSubsetPayload` in place of `agentic.LoopCompletedEvent`; on an empty readable payload a literal condition
    gives the answer a payload without the field gives, and a `$message.` value template is an error (D5).
  - `TestUnreadableReportedOnce` (D7): many goroutines match one loaded rule against messages of one unreadable payload
    type, under `-race`; exactly one Warn names the rule ID and payload type; a second `Load` of the same definition
    reports once more.
  - A generated check with `pgregory.net/rapid` (D3), recording seed and completed checks, as a drift guard (the
    oracle is the hand-written corpus): over enabled rules with an `entity.pattern` and conditions `Matches` does not
    refuse, and entities inside the pattern, the firing match and `Matches` give the same answer (equal verdicts, or
    an error from both). The generator draws inputs that make the firing match error (an operator whose value cannot
    be compared, such as `gt "abc"` on a number, and an unfilled value template) and absent required fields. Record in
    `notes/2.1.md` a run against each of two mutants, failing: `Matches` turning an evaluation error into
    `false, nil`; `Matches` returning an absent required field as an error.
- [ ] 2.2 `ErrRequiredFieldMissing` in `processor/rule/expression`, wrapped as the `Err` of the evaluator's three
  absent-required errors (`evaluator.go:167`, `:183`, `:229-240`), not of the required `transition` with no entity
  (`:283-287`); the `rule-conditions` scenarios. Check:
  `go test ./processor/rule/expression/`.
- [ ] 2.3 The entity match and `Matches` from `expression_factory.go:210-327` and `matches.go` (D2, D3, D4, D5):
  methods on `*Rule`; the lifecycle lookup an argument of the unexported firing match; condition values filled under
  the caller's context with the one scan, `regex` conditions left as compiled; an unresolved token an error, not a log
  line; an absent required field no match and no error, read with `errors.Is` (on the message match too, 2.4);
  `Matches` answers false for a rule with no `entity.pattern` and for an entity outside it
  (`semtypes.MatchEntityIDPattern`), and refuses a `$entity.lifecycle.` condition, saying it cannot be answered
  without a lookup (the refusal message in `expression/stateless.go` drops "pass one to resolve it", and its doc
  comment follows). The method's doc comment carries rules 1 to 3 of the pin's
  `docs/operations/adopter-match-and-inflight.md` as D2 restates them. Not carried: `MatchesWithLifecycle` and the
  failed-lookup refusal (`matches.go:92-124`, `:175-183`), `NewExpressionRule`, `SetLifecycleManager`,
  `unresolvedTemplateVarRe`.
- [ ] 2.4 The message match and the projection from `expression_factory.go:57-75`, `:143-209` and
  `payload_projection.go` (D7): no cooldown, no latch, no empty-payload return; the unreadable report once per loaded
  rule and payload type.
- [ ] 2.5 The `Load` check of D6 in the expression arm (`load.go:143`).
- [ ] 2.6 Carry the tests of inventory § 1's Carried column and PR 3's four forwarded tests
  (`typed_substitution_test.go:273-351`, `lifecycle_substitution_test.go:301-327`, `:329-355`, `:460-483`), rewritten
  to the harness rules (`t.Context()`, `internal/harness/semantictest`), the `Matches` cases with a pattern and a
  six-part ID added and their verdicts kept but for D4's (2.1). The lifecycle tests that remain call the firing entity
  match with the lookup as an argument; `matches_lifecycle_integration_test.go:61-84` runs on the package's shared
  NATS server with a real `lifecycle.NewManager`. Those that ask only `MatchesWithLifecycle` wait with it (§ 1,
  Dropped). Check: `go doc ./processor/rule` lists no name without a reader outside the package or a spec line.
- [ ] 2.7 Comments: inventory F15's ten history lines keep their reason and lose the history.
- [ ] 2.8 Extend the ledger row `processor/rule` with what this PR adds and what it does not carry (inventory § 1);
  keep the `scripts/cover-check.sh` floor. Check: `task ledger:check`, `task cover:check`.
- [ ] 2.9 The PR body states the carried pin lines, production and test, as `sed -n a,bp | wc -l` ranges, and its
  adopter section says that `NewExpressionRule` and `EvaluateEntityState` are gone, that a product loads its pack and
  calls `Matches`, which answers false for a missing required field as `EvaluateEntityState` did, and that
  `MatchesWithLifecycle` waits until a product asks for it.

## 3. Docs

- [ ] 3.1 Port `processor/rule/docs/syntax.md` corrected to the code (D9): no `${entity.field}`, `$related.*` or pair
  rules; `cooldown` checked at load, nothing more; D6's refusals. Check: `task docs:check`.

## 4. Land

- [ ] 4.1 Sync `openspec/specs/rule-matching/`, `rule-definitions/` and `rule-conditions/` and archive the change as
  the PR's last content commit. Check: `task spec:check`.
