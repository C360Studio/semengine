# Design: rule-core-06-expression-rules

Pin: SemStreams `8b99efe9`, `processor/rule/`. Inventory: `inventory.md` (base a2174c4a; INVENTORY PASS, 3 LOW
folded in), probe `notes/inventory-probe.md`. Givens: the target-shape page § 1 and § 3; the graph-event drop (#9
comment 7160955176); #702 (cooldown to PR 7); PR 1's seam (`archive/…-01-expression/design.md:76-90`); PR 2's `Load`;
PR 3's forward list. Spec delta: `rule-matching` (new); `rule-definitions` (one added requirement, D6);
`rule-conditions` (one added requirement, D4).

## Context

PR 6 brings the answer to "do this rule's conditions hold?" for an entity and for a message, the stateless `Matches`,
and the payload projection that turns a message into fields a condition reads. No lifecycle owner. Size: 747
production pin lines, 460 carried and rewritten, 287 not carried; 2,291 test lines, 1,218 carried, 567 moved to PRs 7,
8 and 9, 506 dropped (inventory § 1). One PR. Measured premises: `Load` already refuses bad operators, logic and
cooldowns (§ 3); 84 of 91 product rules carry a `required` condition, 0 of 91 use `or`, 0 of 91 name a value their
path never has (§ 5); the pin's firing path compares an unresolved template as text and logs evaluation errors at
Debug (probes P2, P5).

## Decisions

**D1. What PR 6 carries and exports.** Framework API (contract line: the `rule-matching` spec): `(*Rule).Matches(ctx,
state *graph.EntityState) (bool, error)` (D2) and `expression.ErrRequiredFieldMissing` (D4). Unexported, read by
`Matches` here and by PRs 7 and 9 (pin callers `message_handler.go:118`, `:308`): the firing entity match on `*Rule`,
which takes the lifecycle lookup as an argument, the message match, `ruleFields`, `payloadTypeLabel`. Not carried:
`MatchesWithLifecycle` and the failed-lookup refusal only it reaches (`matches.go:92-124`, `:175-183`; D2),
`ExpressionRule`, `NewExpressionRule` and its pack and platform checks (read only by the graph event),
`SetLifecycleManager` (F7), `Name`, `Subscribe` (F2), `ExecuteEvents` (ruled), the cooldown, trigger latch and
last-fired stamp (F6), the factory, `Validate` (in `Load`), `Schema`, `init`. Proof: `go doc ./processor/rule` lists no
name without a reader or a spec line.

**D2. `Matches` is a method of a loaded rule, false for a rule production would not run (owner decision 1).** Options:
(a) the pin's package functions over a `Definition`: a second, partial load path that compiles regexes itself and skips
`Load`'s declared and rule-opaque field checks (ADR-036); (b, recommended) a method on the `*Rule` `Load` returns. Its
cost: the asker must pass `Load`, so it needs a handler for every action type in the rule and every bare condition field
declared (inventory § 11, item 1); semteams' research rules carry `publish_agent` and predicates semteams declares. A
product running the rule processor has both. (c) drop: no caller in scope, and semteams' contract test loses its way to
ask. Only `Matches` is exported: `MatchesWithLifecycle` has no caller in scope (§ 6) and would export a concrete
`*lifecycle.Manager` and NATS I/O; the lookup stays an argument of the unexported firing match, so the lifecycle tests
keep their entry, and adding the method later breaks nobody. Order, the first that applies deciding: (1) a nil rule,
context or state is refused (the pin's first check, `matches.go:132-149`); (2) a malformed entity ID is an error, even
for a disabled rule; (3) a disabled rule, one with no `entity.pattern` (it runs on messages or a schedule), an entity
outside the pattern (`semtypes.MatchEntityIDPattern`, `pkg/types/entity_id.go:169`), or no conditions is `false, nil`:
production never evaluates that rule for this entity, so nothing is owed and a pack-wide scan asks every loaded rule
unfiltered (no conditions is reached only once PR 11 loads cron rules with a pattern); (4) the stateless refusals (D3);
(5) evaluation. Rules 1 to 3 of the pin's adopter note (`docs/operations/adopter-match-and-inflight.md`) become the
method's doc comment, rule 3 listing an absent required field among the `false, nil` answers; the note's lookup half
waits with that method, and the note is not ported (its readers are post-MVP). Proof: `TestMatches`, in the spec's
order.

**D3. One match for firing and asking.** The firing entity match: a disabled rule or one with no conditions does not
match; lifecycle fields are resolved through the lookup argument (a failed lookup leaves them absent, as at the pin; PR
7 decides otherwise, PR 3's forward); condition values are filled under the caller's context (F5); then the evaluator
answers, an absent required field being no match (D4). The rule processor picks entity rules by pattern before calling
(pin `message_handler.go:265-276`), so the pattern check is `Matches`' own. `Matches` is that match with no lookup,
behind the D2 checks and the stateless refusals (`EnsureStatelessResolvable`: `transition`, `$state.*`, `$prev.*`, and
`$entity.lifecycle.*`, saying it cannot be answered without a lookup). Cooldown applies to neither. Two differences
remain, by design: outside the pattern the firing match answers on its conditions and `Matches` gives false (D2, step
three); a condition `Matches` refuses gets a verdict when firing. For an enabled rule with a pattern, an entity inside
it and conditions `Matches` can answer, the two give the same answer: equal verdicts, or both an error. The oracle is
the carried `matches_test.go` corpus with hand-written verdicts (each case given a pattern and a six-part entity ID),
plus the carried entity tests. A generated check (`rapid`, seed and completed checks recorded) guards drift, not truth:
over those rules and entities, drawing inputs that make the firing match error (an operator whose value cannot be
compared, an unfilled value template) and absent required fields, the two give the same answer. Two mutants are shown to
fail it: `Matches` turning an evaluation error into `false, nil`, and `Matches` returning an absent required field as an
error.

**D4. A required field that is absent: no match and no error, on every path (owner decision 2).** Products use
`required: true` to mean "match only when present" (inventory § 5), and semteams' contract test asserts false there (§
11, item 2). This exempts the case from PR 1's every-error-at-Warn commitment: otherwise 84 rules would log on every
evaluation they do not match. The verdict is the pin's firing verdict; the pin's `Matches` returned an error here
(`matches.go:80-82`). The evaluator's errors for an absent required field (`evaluator.go:167`, `:183`, `:229-240`) wrap
`ErrRequiredFieldMissing`; the rule package reads it with `errors.Is`, never the text, and gives no match on the firing
entity match, the firing message match and `Matches` alike. A required `transition` with no entity (`:283-287`) does not
wrap it: the entity is absent, not a field; `Matches` refuses `transition` before evaluating, and the firing entity
match never runs without an entity. On the firing message match a required `transition` is that unwrapped error, logged
at Warn by its caller, which is right: on a message it can never hold. Under `or`, an absent required field vetoes the
whole rule, as at the pin, since the evaluator returns at the first error (`:108-119`); 0 product rules use `or`. Other
evaluation errors and an unfilled template (D5) stay errors on both paths. Lost: an asker cannot tell an absent required
field from conditions that do not hold; no caller in scope needs to. Other options: an error from `Matches` only (every
asker special-cases the sentinel); every error returned, PRs 7 and 9 logging each once per rule and field (a real fault
is silenced after its first report). Proof: `TestRequiredFieldMissing`.

**D5. An unresolved value template fails the match, on both paths (owner decision 3).** At the pin the firing path
compares `$entity.triple.x.y.missing` as text and can match (P5). Here both paths return an error naming rule, field and
token, from the one scan's unresolved list (F8); the match logs nothing, so a token is not reported twice. The 5
templated values in product rules resolve on any valid entity ID. A `regex` condition is not filled (`Compile` refuses a
pattern holding a token), so it keeps its compiled pattern. Proof: `TestUnresolvedValueTemplate`; semteams' two research
rules, loaded with a fake `publish_agent` handler and their predicates registered, match through the firing match.

**D6. `Load` refuses a condition its path can never fill (owner decision 4).** A message rule has no entity, so
`$entity.id` is silently `""` (P3); an entity rule has no message. `Load` refuses, at `conditions[<i>]`, in a rule with
no `entity.pattern` a field that starts with `$entity.` or a string value containing it anywhere (`zone-$entity.id`
too), and in a rule with one the same for `$message.`. 0 of 91 product rules. Action `when` guards are PR 7's; PR 9,
which builds the message path, revisits the message-rule refusal if it gives a message rule an entity. Proof: two cases
in `TestLoadRefuses`.

**D7. The message match and the payload projection.** A payload is readable when it implements `message.RuleReadable`;
nothing reads its structure. Its fields feed `$message.*` conditions and condition values, and PR 9 fills the action
context's message data from the same function. An unreadable payload is no match, no error, and a Warn once per loaded
rule and payload type (rule ID, payload type, remedy); a reload reports again (nearest instance
`pkg/lifecycle/manager.go:281`). PR 8 adds a metric per rule and payload type with the processor's metrics. The early
return on an empty payload goes (F12): for a literal condition the answer is the same, and a `$message.` value template
on an empty payload becomes an error under D5 where the pin answered false. A match is safe for concurrent use and
writes nothing a later match reads except that report. Proof: the carried projection tests on the file's own typed
payload; `TestUnreadableReportedOnce` under `-race`.

**D8. Where PR 6 meets PR 4, and what goes forward.** PR 6 touches no action code (`action.go`, `builtin.go`,
`Handlers`, the executor) and builds no `ExecutionContext` for an action; PR 4's `Execute` and `Run` never see a match.
Files both may touch: `load.go` (PR 6's D6 check; PR 4's handler arguments), the ledger row, `doc.go`. Either merges
first; the second merges `origin/main`. PR 7 waits for both and feeds `$prev.*`/`$state.*` into the one match (F1). PRs
7 and 9 log returned errors at Warn. PR 8: the unreadable metric (D7); status loses the subject list (F2).

**D9. Tests, docs, comments.** Carried tests build rules with `DecodeDefinition` and `Load`, use `t.Context()`; PR 3's
four forwarded tests come too (inventory § 1). `processor/rule/docs/syntax.md` comes over corrected: no
`${entity.field}`, `$related.*` or pair rules; `cooldown` checked at load, nothing more. F15's ten history lines keep
their reason. Optional, left: one evaluator per call (F16), the four no-conditions checks' history (F3).

## ADRs

Kept: 041 (one evaluator), 047's condition half (ported in PR 5), 036 (`Load`). Dropped, ruled: 076 decisions 1-3 and 5;
039's governance half (Q15). Changed: the pin's `rule-engine` spec says the firing path's behaviour does not change; D5
changes its verdict for an unfilled template, D4 keeps the verdict and changes only the report. The pin's `Matches`
contract (that spec, `:64-233`) changes three ways: an absent required field is `false, nil` (D4); a rule with no
pattern or an entity outside it is `false, nil` and a malformed entity ID an error (D2); `MatchesWithLifecycle` is not
carried (D1). 031: PR 11.

## Issues, merge order, risks

Retires none. #702 and #25 retire at PR 7, named without a closing keyword. #756 and #757 stay, optional inherited
maintenance: the message match is the first path where Go-built values reach the evaluator, but no `RuleFields` at the
pin or in a consumer returns a float or a `[]string` (inventory § 9). Independent of PR 4. Risks: semteams' contract
test moves to `Load` and `Matches` and needs its handlers and predicates; its assertions of false hold, a missing
required field included; `ErrRequiredFieldMissing` is new surface in PR 1's package.
