# Design: rule-core-06-expression-rules

Pin: SemStreams `8b99efe9`, `processor/rule/`. Inventory: `inventory.md` (base a2174c4a; inventory review pending),
probe `notes/inventory-probe.md`. Givens: the target-shape page § 1 and § 3; the graph-event drop (#9 comment
7160955176); #702 (cooldown to PR 7); PR 1's seam (`archive/…-01-expression/design.md:76-90`); PR 2's `Load`; PR 3's
forward list. Spec delta: `rule-matching` (new); `rule-definitions` (one requirement, D6); `rule-conditions` (one
requirement, D4).

## Context

PR 6 brings the answer to "do this rule's conditions hold?" for an entity and for a message, the stateless `Matches`
pair, and the payload projection that turns a message into fields a condition reads. No lifecycle owner. Size: 747
production pin lines, 502 carried and rewritten, 245 not carried; 2,291 test lines, 1,382 carried, 567 moved to PRs 7,
8 and 9, 342 dropped (inventory § 1). One PR. Measured premises: `Load` already refuses bad operators, logic and
cooldowns (§ 3), so PR 6 owes no operator check; 84 of 91 product rules carry a `required` condition and 0 of 91
name a value their path never has (§ 5); the pin's firing path compares an unresolved template as text and logs
evaluation errors at Debug (probes P2, P5).

## Decisions

**D1. What PR 6 carries and exports.** Framework API (contract line: the `rule-matching` spec): `(*Rule).Matches(ctx,
state *graph.EntityState) (bool, error)`, `(*Rule).MatchesWithLifecycle(ctx, state, m *lifecycle.Manager) (bool,
error)` (D2) and `expression.ErrRequiredFieldMissing` (D4). Unexported, read by `Matches` here and by PRs 7 and 9 (pin
callers `message_handler.go:118`, `:308`): the entity match and the message match on `*Rule`, `ruleFields`,
`payloadTypeLabel`. Not carried: `ExpressionRule`, `NewExpressionRule` and its pack and platform checks (read only by
the graph event), `SetLifecycleManager` (the lookup is an argument, F7), `Name`, `Subscribe` (F2; the entity pattern is
the one spelling), `ExecuteEvents` (ruled), the cooldown, trigger latch and last-fired stamp (#702, F6), the factory,
`Validate` (in `Load`), `Schema`, `init`. Proof: `go doc ./processor/rule` lists no name without a reader or a spec
line.

**D2. `Matches` is a method of a loaded rule (owner decision 1).** Options: (a) the pin's package functions over a
`Definition`: they need their own compile of regex conditions, a second load path that skips `Load`'s declared and
rule-opaque field checks (ADR-036); (b, recommended) methods on the `*Rule` `Load` returns, so a product answers with
the rule it loaded; (c) drop: no caller in scope (its adopters are post-MVP), and semteams' contract test, which builds
`NewExpressionRule` to ask this question, loses its way to ask it. The pair stays split, as the pin's spec requires:
the lookup decides which conditions can be answered, so it is in the name. A nil rule, context, state or manager is
refused.

**D3. One match for firing and asking.** The entity match: a disabled rule or one with no conditions does not match;
lifecycle fields are resolved (a failed lookup leaves them absent, as at the pin; PR 7 decides otherwise, PR 3's
forward); condition values are filled under the caller's context (F5); then the evaluator answers. `Matches` adds
the stateless refusals (`EnsureStatelessResolvable` on each condition; a failed lookup when a condition reads
`$entity.lifecycle.*`) and returns what the firing match turns into no match (D4). Cooldown applies to neither:
`Matches` answers "is work still owed", and PR 7 gates firing. Proof: the carried `matches_test.go` corpus with its
hand-written verdicts; the carried entity tests; a generated check (`rapid`, seed and completed checks recorded) over
stateless-answerable conditions and entities: whenever both answer, they agree, and whenever the firing match returns an
error, so does `Matches`.

**D4. A required field that is absent: no match when firing, an error when asking (owner decision 2).** Products use
`required: true` to mean "match only when present" (84 of 91 rules). PR 1's seam has firing callers log evaluation
errors at Warn; without this, each of those rules would log on every entity or message it does not match. The
evaluator's existing error for an absent required field (`evaluator.go:167`, `:183`, `:229-240`) wraps
`ErrRequiredFieldMissing`, so callers tell it apart with `errors.Is`, never by its text. Other evaluation errors stay
errors on both paths. Other option: return every error and let PRs 7 and 9 log each once per rule and field (a real
fault in a rule is then also silenced after its first report). Proof: `TestRequiredFieldMissing` (both paths,
`errors.Is`).

**D5. An unresolved value template fails the match, on both paths (owner decision 3).** At the pin the firing path
compares `$entity.triple.x.y.missing` as text and can match (P5); `Matches` refuses it. Here both return an error naming
the rule, field and token, from the one scan's unresolved list (F8); the match logs nothing, so a token is not reported
twice. The 5 templated values in product rules resolve on any valid entity ID. A `regex` condition is not filled
(`Compile` refuses a pattern holding a token), so it keeps its compiled pattern. Proof: `TestUnresolvedValueTemplate`;
semteams' `research/02-plan-to-gather.json` (a regex holding `$`) and `03b-…json` (`.length`) match through the entity
match.

**D6. `Load` refuses a condition its path can never fill (owner decision 4).** A message rule (no `entity.pattern`)
has no entity, so `$entity.id` is silently `""` (P3); an entity rule has no message. `Load` refuses, at
`conditions[<i>]`, a field or value naming `$entity.` in a message rule and `$message.` in an entity rule. 0 of 91
product rules. Action `when` guards run with the action's context and are PR 7's. Proof: two cases in
`TestLoadRefuses`.

**D7. The message match and the payload projection.** A payload is readable when it implements
`message.RuleReadable`; nothing reads a payload's structure. Its fields feed both `$message.*` conditions and condition
values, and PR 9 fills the action context's message data from the same function. An unreadable payload is no match, no
error, and is logged at Warn once per loaded rule and payload type (rule ID, payload type, the remedy), so a rules
reload reports again; the nearest instance is `pkg/lifecycle/manager.go:281`. The early return on an empty payload goes
(F12; evaluation gives the same answer). A match is safe for concurrent use and writes nothing a later match reads
except that report. Proof: the carried projection tests on the file's own typed payload; `TestUnreadableReportedOnce`
with concurrent matches under `-race`.

**D8. Where PR 6 meets PR 4, and what goes forward.** PR 6 touches no action code (`action.go`, `builtin.go`,
`Handlers`, the executor) and builds no `ExecutionContext` for an action; PR 4's `Execute` and `Run` never see a match.
PR 7's `Fire` acts on the verdict. Either merges first; the second merges `origin/main` (shared: the ledger row,
`doc.go`). PR 7 waits for both and feeds `$prev.*`/`$state.*` into the one match instead of evaluating twice (F1). PRs 7
and 9 log returned errors at Warn (PR 1 seam). PR 8's status loses the subject list (F2).

**D9. Tests, docs, comments.** Carried tests build rules with `DecodeDefinition` and `Load`, use `t.Context()`; PR 3's
four forwarded tests come too. Dropped and moved ranges: inventory § 1. `processor/rule/docs/syntax.md` comes over
corrected to the code: no `${entity.field}`, `$related.*` or pair rules, and `cooldown` described as checked at load,
nothing more. F15's ten history lines keep their reason. Optional, left: one evaluator per call (F16).

## ADRs

Kept: 041 (one evaluator), 047's condition half (resolved before evaluation; ported in PR 5), 036 (`Load`). Dropped,
ruled: 076 decisions 1-3 and 5; 039's governance half (Q15). Changed: the pin's `rule-engine` spec says the firing
path's behaviour does not change; D4 and D5 change it (for the owner). 031 (cron cooldown): PR 11.

## Issues, merge order, risks

Retires none. #702 and #25 retire at PR 7, named without a closing keyword. #756 and #757 stay, optional inherited
maintenance: the message match is the first path where Go-built values reach the evaluator, but no `RuleFields` at the
pin or in a consumer returns a float or a `[]string` (inventory § 9). Waits for nothing open; independent of PR 4.
Risks: semteams' contract test moves to `Load` and `Matches`, and gets an error where it asserted false for a missing
required field (D4); `ErrRequiredFieldMissing` is new surface in PR 1's package.
