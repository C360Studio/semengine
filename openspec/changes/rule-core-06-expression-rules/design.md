# Design: rule-core-06-expression-rules

Pin: SemStreams `8b99efe9`, `processor/rule/`. Inventory: `inventory.md` (base a2174c4a; INVENTORY PASS, 3 LOW
folded in), probe `notes/inventory-probe.md`. Givens: the target-shape page § 1 and § 3; the graph-event drop (#9
comment 7160955176); #702 (cooldown to PR 7); PR 1's seam (`archive/…-01-expression/design.md:76-90`); PR 2's `Load`;
PR 3's forward list. Spec delta: `rule-matching` (new); `rule-definitions` (one added requirement, D6);
`rule-conditions` (one added requirement, D4).

## Context

PR 6 brings the answer to "do this rule's conditions hold?" for an entity and for a message, the stateless `Matches`
pair, and the payload projection that turns a message into fields a condition reads. No lifecycle owner. Size: 747
production pin lines, 502 carried and rewritten, 245 not carried; 2,291 test lines, 1,382 carried, 567 moved to PRs 7,
8 and 9, 342 dropped (inventory § 1). One PR. Measured premises: `Load` already refuses bad operators, logic and
cooldowns (§ 3); 84 of 91 product rules carry a `required` condition, 0 of 91 use `or`, 0 of 91 name a value their
path never has (§ 5); the pin's firing path compares an unresolved template as text and logs evaluation errors at
Debug (probes P2, P5).

## Decisions

**D1. What PR 6 carries and exports.** Framework API (contract line: the `rule-matching` spec): `(*Rule).Matches(ctx,
state *graph.EntityState) (bool, error)`, `(*Rule).MatchesWithLifecycle(ctx, state, m *lifecycle.Manager) (bool,
error)` (D2) and `expression.ErrRequiredFieldMissing` (D4). Unexported, read by `Matches` here and by PRs 7 and 9 (pin
callers `message_handler.go:118`, `:308`): the firing entity match and message match on `*Rule`, `ruleFields`,
`payloadTypeLabel`. Not carried: `ExpressionRule`, `NewExpressionRule` and its pack and platform checks (read only by
the graph event), `SetLifecycleManager` (F7), `Name`, `Subscribe` (F2), `ExecuteEvents` (ruled), the cooldown, trigger
latch and last-fired stamp (F6), the factory, `Validate` (in `Load`), `Schema`, `init`. Proof: `go doc ./processor/rule`
lists no name without a reader or a spec line.

**D2. `Matches` is a method of a loaded rule, refusing what production never asks (owner decision 1).** Options: (a)
the pin's package functions over a `Definition`: a second, partial load path that compiles regexes itself and skips
`Load`'s declared and rule-opaque field checks (ADR-036); (b, recommended) methods on the `*Rule` `Load` returns. Its
cost: the asker must pass `Load`, so it needs a handler for every action type in the rule and every bare condition
field declared (inventory § 11, item 1); semteams' research rules carry `publish_agent` and predicates semteams
declares. A product running the rule processor has both. (c) drop: no caller in scope, and semteams' contract test
loses its way to ask. The pair stays split: the lookup decides which conditions can be answered, so it is in the name.
`Matches` gives no verdict production never would: a rule with no `entity.pattern` runs only on messages, so it is
refused, classified invalid; an entity outside the rule's pattern (`semtypes.MatchEntityIDPattern`,
`pkg/types/entity_id.go:169`) is `false, nil`, since production never evaluates that rule for it and nothing is owed
(refusing would make a pack-wide scan fail on every rule an entity is not for); a malformed entity ID is an error. A
nil rule, context, state or manager is refused. Rules 1 to 3 of the pin's adopter note
(`docs/operations/adopter-match-and-inflight.md`: ask the framework, not the evaluator; obligation, not instant; three
outcomes; pass a deadline, as the lookup does I/O) become the methods' doc comment; the note itself is not ported, as
its readers are post-MVP. Proof: `TestMatches` (spec scenarios, including the two refusals and the out-of-pattern
entity).

**D3. One match for firing and asking.** The firing entity match: a disabled rule or one with no conditions does not
match; lifecycle fields are resolved (a failed lookup leaves them absent, as at the pin; PR 7 decides otherwise, PR 3's
forward); condition values are filled under the caller's context (F5); then the evaluator answers. The rule processor
picks entity rules by pattern before calling (pin `message_handler.go:265-276`), so the pattern check is `Matches`'
own. `Matches` adds the D2 refusals, the stateless ones (`EnsureStatelessResolvable`; a failed lookup when a condition
reads `$entity.lifecycle.*`) and returns what the firing match turns into no match (D4). Cooldown applies to neither.
Proof: the oracle is the carried `matches_test.go` corpus with hand-written verdicts, plus the carried entity tests. A
generated check (`rapid`, seed and completed checks recorded) guards drift, not truth: whenever both answer they agree,
and whenever the firing match errors, so does `Matches`; a mutant where `Matches` skips the disabled check is shown to
fail it.

**D4. A required field that is absent: no match when firing, an error when asking (owner decision 2).** Products use
`required: true` to mean "match only when present". This exempts an absent required field from PR 1's every-error-at-
Warn commitment: otherwise 84 rules would log on every evaluation they do not match. The verdict is the pin's (false);
only the report changes. The evaluator's errors for an absent required field (`evaluator.go:167`, `:183`, `:229-240`)
wrap `ErrRequiredFieldMissing`, so callers use `errors.Is`, never the text. A required `transition` with no entity
(`:283-287`) does not wrap it: the entity is absent, not a field; `Matches` refuses `transition` before evaluating, and
the firing entity match never runs without an entity. Under `or`, an absent required field vetoes the whole rule, as at
the pin, since the evaluator returns at the first error (`:108-119`); 0 product rules use `or`. Other evaluation errors
stay errors on both paths. Other option: return every error and let PRs 7 and 9 log each once per rule and field (a
real fault is then silenced after its first report). Proof: `TestRequiredFieldMissing`.

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
changes its verdict for an unfilled template, D4 keeps the verdict and changes only the report (for the owner). 031:
PR 11.

## Issues, merge order, risks

Retires none. #702 and #25 retire at PR 7, named without a closing keyword. #756 and #757 stay, optional inherited
maintenance: the message match is the first path where Go-built values reach the evaluator, but no `RuleFields` at the
pin or in a consumer returns a float or a `[]string` (inventory § 9). Independent of PR 4. Risks: semteams' contract test
moves to `Load` and `Matches`, needs its handlers and predicates, and gets an error where it asserted false for a
missing required field; `ErrRequiredFieldMissing` is new surface in PR 1's package.
