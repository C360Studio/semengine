# Tasks: rule-core-03-context

Gates per code task: `task verify` on a clean tree. Pin paths are SemStreams `8b99efe9`, `processor/rule/`.

## 1. Design

- [x] 1.1 The inventory review passes, the design review passes; the owner fills `Approved-by:`.
- [x] 1.2 Fable reads this design before any code, or the owner declines that read (proposal, first decision:
  `design.md` D1, D7, D8, D9).
- [x] 1.3 Start implementation once PR #758 (`rule-core-02-definitions`) has merged; merge `origin/main` into this
  branch first.

## 2. Port

- [x] 2.1 Tests come before the code, each seen to fail first; after 2.4, run `task mutate:check` on each refusal and
  record detected or survived in `notes/2.1.md`.
  - `TestSubstituteOnce` (D2): the probe's F19 case gives `note: see $message.secret` and reports nothing.
  - `TestTokenEnds` (D2): the probe's F20 case stays unchanged and is reported; `$entity.org.$entity.platform.chain`
    and a token before a sentence's full stop resolve; `$nowhere`, `$entity.idx`, `$entity.lifecycle.x`,
    `$state.last_transition` and `$schedule.last_fired_at` stay and are reported; `$caller.id` and `$related.id` stay
    and are not reported; `.value` after a missing 3-part predicate gives `""` unreported, and
    `$entity.triple.autoresearch.best.value` reads the predicate `autoresearch.best.value`.
  - `TestSubstituteWithNames` (D7): a loop item `subtopic` holding `$entity.id` comes out as written; `$subtopic.`
    and `$subtopic-x` resolve, `$subtopics` does not; a name `now`, `nowx`, `entity` or `message` is refused,
    classified invalid, before any scan.
  - `TestTripleLength` (D3): three scalar triples give `3`, a two-element list `2`, absent `0`.
  - Generated checks with `pgregory.net/rapid` (D2, D3), recording seed and completed checks: text outside tokens and
    every inserted value come out unchanged; a template with no `$` is returned as is; `.length` equals the length
    of `.triples` decoded.
  - `TestStateTracker` (D9), on the package's shared NATS server (natsfixture, integration tag): round trip;
    `ErrStateNotFound`; a wildcard entity key and a dotted rule ID refused, classified invalid, no record written;
    a nil context refused; `DeleteAllForEntity` keeps other entities' records, returns nil on an empty bucket, and
    refuses `>` and an ID holding `*`, classified invalid, with every record kept.
- [x] 2.2 `ExecutionContext` and the substituter from `execution_context.go`, `typed_substitution.go`,
  `entity_substitution.go`, `message_substitution.go`, `lifecycle_substitution.go` (D1, D2, D3, D5, D6), reading
  `expression.SubstitutionPrefixes()`. Not carried: `Schedule` and the `$schedule.*` pass (D1),
  `caller_substitution.go`, `RelatedID`, `Related` and every `$related` branch (D4), `:256-268`, `:377-379`,
  `:718-767` (D7); `SubstituteVariablesWith` rebuilt on the one scan (D7). The doc comment says `Entity`, `State`
  and `MessageData` are read-only to a handler, that an `ExecutionContext` is not safe for concurrent use, and prefers
  `$entity.id` to `$entity.instance` for an identity (ADR-056). Record in `notes/2.2.md` a one-off run of every
  template string in semteams' rules (4df44ae8) and semboids' (ab4e0fbc) through the pin copy and the port, each
  referenced predicate present: list every difference, each one an F19 or F20 case, `.length` (D3), or a triple path
  outside D2's `[a-z0-9_-]`, which no stored predicate can hold.
- [x] 2.3 `LifecycleManager` with `LookupByEntityID` and `GetWorkflowDefinition` only (D5, `actions.go:513-514`).
- [x] 2.4 `MatchState`, the transition type and `detectTransition` (unexported), `StateTracker` from `state_tracker.go`
  (D8, D9): no
  `ActionIterations`, no pair key, no `Delete`, no nil-bucket branches; `NewStateTracker(kv *natsclient.KVStore)`;
  doc comments give D8's field meanings. Unexport `substituteConditionValues` (one function, no twin) and
  `populateLifecycleStateFields` (D1). Check: `go doc ./processor/rule` lists no name without a reader outside the
  package or a spec line.
- [x] 2.5 Remove `$related.` and `$caller.` from `expression.SubstitutionPrefixes()` and its doc comment (D4); add a
  `Compile` case accepting `$caller.id` and `$related.id`; update the token lists in
  `expression/compile_fuzz_test.go:33`, `:45` and `compile_test.go:132`. Check: `go test ./processor/rule/expression/`.
- [x] 2.6 Carry the tests of PR 3's code, rewritten to the harness rules (`internal/harness/semantictest`,
  `t.Context()`): the ten test files of inventory § 1 less the 11 tests that drive PRs 4, 6 and 7 (left for those PRs:
  `typed_substitution_test.go:60`, `:109`, `:154`, `:191`, `:221` to PR 4, `:273` to PR 6, five lifecycle tests),
  the 7 `SubstituteVariablesWithIterVar`/`ResolveListValue` tests and `caller_substitution_test.go`; tests asserting
  `[ERROR_LENGTH_NOT_LIST` change to D3.
- [x] 2.7 Comments: the 35 port-history lines of inventory F28 keep their reason and lose the history; the segment
  pass's order claim (`entity_substitution.go:31-34`) goes (F26).
- [x] 2.8 Extend PR 2's ledger row `processor/rule` with what this PR adds; keep its `scripts/cover-check.sh` floor
  (80%). Check: `task ledger:check`, `task cover:check`.
- [ ] 2.9 The PR body states the carried pin lines, production and test, as `sed -n a,bp | wc -l` ranges, and its
  adopter section says `Entity`, `State` and `MessageData` are read-only to a handler and that an `ExecutionContext`
  is not safe for concurrent use (D7).

## 3. Docs

- [x] 3.1 Port ADR-048 to `docs/adr/048-bounded-dispatcher-and-triples-substrate.md` with a port note (D3: Decision
  2 binds; the dispatcher half is not carried with it; `.length` now counts every value) and its ledger row.
  Check: `task docs:check`, `task ledger:check`.
- [x] 3.2 Add the `$entity.triple.*` clause (pin `.agents/contracts/semstreams-reviewer.md:241-242`) to
  `.agents/contracts/semengine-reviewer.md` § Graph and state ownership, replacing the deferral at `:325-326`.

## 4. Land

- [ ] 4.1 Sync `openspec/specs/rule-substitution/`, `rule-state/` and `rule-conditions/` and archive the change as the
  PR's last content commit. Check: `task spec:check`.
