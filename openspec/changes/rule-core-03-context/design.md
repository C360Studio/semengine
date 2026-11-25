# Design: rule-core-03-context

Pin: SemStreams `8b99efe9`, `processor/rule/`. Inventory: `inventory.md` (PASS, PR #759 comment 7169129514), probe
`notes/inventory-probe.md`. Givens, not reopened: the target-shape page
(`archive/2026-11-25-design-rule-core-target-shape/` §§ 2, 4), shape rulings PR #752 comment 7162146299 (items 6, 9), #9
rulings 7160955176 and 7165028479, PR 1's design § Seam, PR 2's approved design (PR #758, 11bcabaa). Spec delta:
`rule-substitution`, `rule-state` (new); `rule-conditions` (one requirement, D4).

## Context

PR 3 brings `ExecutionContext`, its substitution, the record `MatchState` and its store `StateTracker`; no lifecycle
owner (`state_tracker.go:65-71`). Probe: the 92 tests pass under `-race` and shuffled; F3, F19, F20 observed (§ 2).
Size: 1,850 production and 2,890 test pin lines (§ 1), one PR; at least 136 production lines drop (D4, D7), and 11 tests
move to PRs 4, 6 and 7, whose code they drive (§ 1). Nothing outside the package's tests calls this code before PR 4.

Measured here, semteams 4df44ae8 `configs/rules/`: `autoresearch.experiment.completed` is one scalar triple per
iteration (`autoresearch/04a-execute-stamp-completion.json:33-36`, `04b-…:27-30`) and `05-iteration-dispatch.json:79`,
`:123` read its `.length`, so at the pin both prompts carry `[ERROR_LENGTH_NOT_LIST:…]` (F3's probe case; this corrects
F3's "do not show which"). Triple tokens name 2-part (`lineage.run-loop-entity-id`, 42 uses) and 4-part predicates. A
dot follows a fixed name (`create-change/01-…-spawn.json:55`, `$entity.org.$entity.platform.chain`); a sentence ends on
a token (`create-change/02-author-to-reviewer.json:46`).

## Decisions

**D1. What PR 3 carries and exports.** Framework API, since PR 4's `Run(ctx, Action, *ExecutionContext)` names it
(contract line: the `rule-substitution` spec): `ExecutionContext` with `EntityID`, `Entity`, `State`, `MessageData`,
`Lifecycle`; `RuleID`, `SubstituteVariables`, `SubstituteVariablesTyped`; `LifecycleManager` (D5). Exported as at the
pin, readers in PRs 6, 7, 10 (F12 optional): `SubstituteConditionValues` and its twin, `PopulateLifecycleStateFields`,
`MatchState`, `Transition`, `DetectTransition`, `StateTracker`, `NewStateTracker`, `ErrStateNotFound`. `Schedule` and
the `$schedule.*` resolver are `cron_substitution.go`'s (F27): PR 11 adds both; until then a `$schedule.` token is
unresolved (D2), as at the pin for every non-cron rule, and enabled cron rules are refused at load (PR 2 D7).

**D2. A template is read once, token by token (F1, F19, F20; F2 and F24 go with it).** The substituter scans left to
right. A token starts with `$now` or one of `expression.SubstitutionPrefixes()`, the one list (PR 1 D3). A fixed name
(`$now`, `$entity.id`, the six `$entity.<segment>`, `$state.iteration`, `$state.max_iterations`, the four
`$entity.lifecycle.*`) ends where the name ends, if the next character is not a letter, digit or `_`:
`$entity.org.$entity.platform.chain` resolves both, `$entity.idx` nothing. `$entity.triple.<path>` and `$message.<path>`
take the longest run of dot-joined segments, a final dot excluded (triple segments may hold `-`, message segments not,
as at `message_substitution.go:54`); the path is a suffix form (`.length`, `.triples`, or `.value` after a 3-part
predicate, `:655`) or the whole predicate. A resolved token is replaced and the inserted text is never read again, so
`see $message.secret` from a triple stays (F19). An unresolved token stays as written and is reported in the one Warn
(`:836-845`), so `…researcher-plan-entity` beside only `…researcher-plan` stays and warns (F20). The sort (`:338-341`),
the nine regexes and the lifecycle placeholders go; the typed path reads the same token and is typed only for a
whole-template bare triple, `$message` or `$state` token. Other options: a boundary per pass (nine spellings stay);
escaping inserted values (values change). Proof: the carried tests (oracle for unchanged behaviour);
`TestSubstituteOnce`, `TestTokenEnds`; `rapid` checks that text outside tokens and each inserted value come out
unchanged; a one-off run (`notes/2.2.md`) of every semteams and semboids template through pin and port, differing only
where F19, F20 or D3 applies.

**D3. `.length` counts every value, as ADR-048 says (F3).** `.length` is the number of values `.triples` lists: a scalar
triple one, a list object its elements, absent `0`; the error sentinel goes. Kept: `.triples` (every value, a JSON
array), bare `$entity.triple.P` (first value); the condition list operators already read every triple (`rule-conditions`
spec, `:79-83`). ADR-048 is ported to `docs/adr/` with a port note: Decision 2 binds, the dispatcher half's code is not
carried with it, `.length` is the change. semteams' two prompts then read the count. Other option: the pin's
first-triple count (the prompts keep the sentinel). Proof: `TestTripleLength`; a generated check that `.length` equals
the length of `.triples` decoded.

**D4. Namespaces nothing fills are dropped, for the owner (F9, F10, F17; ruling 9).** `$caller.*`: no writer pin-wide,
ADR-032's writer never built, 0 rules. `$related.*`, `RelatedID`, `Related`, pair keys: no writer, 0 rules,
`related_patterns` dropped (PR 2 D5). Both leave `SubstitutionPrefixes()` (the `rule-conditions` delta), so a regex or
template holding them is literal text; F17 is moot. This amends the target-shape page's "one entity or pair"
(`design.md:98`) to one entity; the archived page is not edited, this design and the proposal are the record. Other
option: carry them (`caller_substitution.go` with 7 tests, a `$related` branch in every pass, read by nothing; F17 then
needs a pair cleanup path).

**D5. `$entity.lifecycle.*` is carried (F4, F8; 0 rules).** PR 1 carried the condition half (PR 1 D6); dropping the
template half splits one namespace, and lifecycle is durable execution. `LifecycleManager` holds the two methods
substitution calls (`actions.go:513-514`; main `pkg/lifecycle/manager_query.go:460`, `:553`); PR 5 adds the four its
actions call, on PR 2 D2's reasoning (implementers are in this repository until PR 8); `Get` and `Create` (agent runs,
ADR-053) are not carried. One lookup per context, a failure kept (`:221-236`).

**D6. `$now` stays, against ADR-047's "No `$now` substitution" (`047-…md:373`; F21).** semteams uses it 3 times. Carried
as at the pin, the wall clock in RFC 3339 UTC; taking `Fire`'s `now` is PR 7's call; ADR-047's port (PR 5) names the
change. Other option: drop it.

**D7. What a product handler gets (F11, F25).** The context's fields, `SubstituteVariables`, `SubstituteVariablesTyped`,
`RuleID`. `SubstituteVariablesWithIterVar`, the overlay and `ResolveListValue` are dropped: only `publish_agent` reads
them (#27), and `for_each` leaves the core with it; semteams' 2 `for_each` actions take their list from `.triples` and
bind each item after substituting. The doc comment says an `ExecutionContext` is not safe for concurrent use (it stores
the lifecycle lookup). Other option: keep both as framework API, with a contract line and a product test (66 lines).

**D8. The record (F5, F18; target-shape § 4).** `MatchState` is the pin's less `ActionIterations` (PR 7's `FireState`)
and less the pair meaning of `EntityKey`. `MaxIterations` stays as the rule's value at that evaluation, what
`$state.max_iterations` reads, not a limit (PR 2 D5), never read back; its doc says so. Other option: drop it (the
context then needs a new field). `last_transition` and `transition_at` mean the last transition and when, kept through
evaluations without one; `last_checked` is every evaluation. PR 7's writer keeps them (today it overwrites,
`stateful_evaluator.go:200-203`). Other option: drop the three (read only by operators, `docs/operations.md:128-137`).

**D9. `StateTracker` over `natsclient.KVStore` (F13-F16; nearest shape `graph/embedding/storage.go:173-198`).**
`NewStateTracker(kv *natsclient.KVStore)`; `Get`, `Set`, `DeleteAllForEntity`. Each key passes
`natsclient.ValidateKVLiteralKey`, the rule ID as one token, before any bucket call, else classified invalid (F15); the
entity key need not be a 6-part ID, since the message path keys by message ID (`message_handler.go:433-446`, PR 9).
Absent is `ErrStateNotFound`; a nil context is refused (the store's checks, `kv.go:77-79`, `:207-209`, `:617-619`).
`DeleteAllForEntity` lists `*.<entity ID>` (`KeysByFilter`), deletes each and returns the failures joined; no match is
nil (`kv.go:626-660`), so F16's Warn goes. `Delete` (F13, no caller) and the nil-bucket branches (F14) go. `Set` is a
`Put`: ADR-111 Decision 2 allows it for one writer; ADR-110 Decision 7 binds only a `WriteOwnerOnly` catalog row, PR 8's
to create. Tests use the package's shared NATS server, not the pin's mock, whose `Keys` differs on an empty bucket (§
2). Other option: keep `jetstream.KeyValue` (semteams' 2 tests unchanged; no key check).

**D10. Comments and guidance.** F28's 35 lines keep the reason, lose the history; F26's order claim goes with D2. The
pin reviewer's `$entity.triple.*` clause (`semstreams-reviewer.md:241-242`) replaces `semengine-reviewer.md:325-326`'s
deferral.

## ADRs

Ported: 048 (D3). Kept: 041 (PR 1), 102/104 (segments by named field), SemEngine 111 (D9). Changed: 047's "no `$now`"
(D6; ported in PR 5). Dropped: 032's `$caller.*` (D4); 046's overlay (D7) and 039's reason for `$message.*`, both ruled
with `publish_agent` (Q15); `$message.*` stays (semboids, 26 uses). Later: 031 (PR 11), 028 and 054 (PR 13), 110
Decision 7 (PR 8).

## Issues, merge order, and what goes forward

Retires none; the issues in inventory § 9 stay at the PRs it names. #758 merges first; this branch then merges
`origin/main` (task 1.2). Forward: **PR 4** the 6 typed tests, `RuleID()` on a cron context; **PR 6** the nil context at
`expression_factory.go:198`, `syntax.md` (`${entity.field}`, pair rules), its lifecycle tests; **PR 7** the record's
writer (D8), `FireState`, `$now`'s clock, the `when` context with no `State`, `PopulateLifecycleStateFields`' dropped
error, its lifecycle tests; **PR 8** the `RULE_STATE` bucket, catalog row, bucket ADR and cleanup (ADR-111 Q2, #24), and
whether ADR-110 Decision 7 binds; **PR 9** the message path's entity key; **PR 11** `Schedule` and `$schedule.*`. Risks:
D2 changes output wherever a value held token text or a name extended another (the one-off run lists each);
`LifecycleManager` grows in PR 5.
