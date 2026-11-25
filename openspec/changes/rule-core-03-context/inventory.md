# Inventory: rule-core-03-context

base: 97e1f5d75196292e49fa12009ebaa13b79577d47 (claim commit on origin/main 27023729b82ff42c292bbef45f1f51cf2b26222f;
draft PR #759). Pin copy: `gh api repos/C360Studio/semstreams/tarball/8b99efe9`, unpacked to a scratch directory
(local only). PR 2 read at `origin/claude/rule-core-02-definitions` 11bcabaa (PR #758, draft). Consumers read by
`gh api …/tarball/<sha>` at the commits PR 2's inventory used: semteams 4df44ae877d0c711f769ec6b48b88100dbb267d1,
semboids ab4e0fbc1b382c4fda10bae988662ae0e0bc1e5c. semsource: no `processor/rule` import (PR 2 inventory § 6, at
34dc2680), not re-read.

Question: what PR 3 of the rule-core split carries ("`ExecutionContext`, the substitution files, `MatchState` and
`StateTracker`", `split.md`), against the target-shape page (`archive/2026-11-25-design-rule-core-target-shape/
design.md` §§ 2 and 4), the shape rulings on PR #752 (comment 7162146299, items 6 and 9), the rulings on #9 (comments
7160955176, 7165028479), what PR 1 owes PR 3 (`archive/2026-11-25-rule-core-01-expression/design.md:39-46`, `:85`),
and PR 2's design. Repositories read (`docs/inventory-scope.md`): semengine; semstreams at the pin; semteams and
semboids at symbol level and their checked-in rule JSON.

Track: `design` (`proposal.md:5`). Lifecycle owner: none. `StateTracker` has no `Start` or `Stop`
(`state_tracker.go:65-71`), so the lifecycle suite does not apply.

## 1. Size (measured, `wc -l` at the pin)

Every file is carried whole; none is split with another PR (split inventory `:17`).

| Production file | Lines | Test file | Lines | `func Test` |
| --- | --- | --- | --- | --- |
| `execution_context.go` | 846 | `for_each_substitution_test.go` | 169 | 8 |
| `lifecycle_substitution.go` | 307 | `lifecycle_substitution_test.go` | 508 | 22 |
| `state_tracker.go` | 263 | `state_tracker_test.go` | 406 | 7 |
| `typed_substitution.go` | 195 | `typed_substitution_test.go` | 430 | 7 |
| `entity_substitution.go` | 88 | `entity_substitution_test.go` | 243 | 10 |
| `message_substitution.go` | 81 | `message_substitution_test.go` | 275 | 11 |
| `caller_substitution.go` | 70 | `caller_substitution_test.go` | 168 | 7 |
| | | `triple_length_substitution_test.go` | 194 | 5 |
| | | `triple_triples_substitution_test.go` | 211 | 6 |
| | | `triple_value_substitution_test.go` | 286 | 9 |
| **total** | **1,850** | | **2,890** | **92** |

Both totals equal the split plan's (`split.md`, PR 3 row). Ranges inside these files whose only production readers
are code no later PR carries (§ 6): `caller_substitution.go:1-70` (70); `execution_context.go:256-268`
(`SubstituteVariablesWithIterVar`, 13), `:377-379` (the overlay loop), `:718-767` (`ResolveListValue`, 50).

Test functions in PR 3's files that drive code other PRs carry (counted in the test column above):

- `typed_substitution_test.go`: 5 of 7 drive `NewActionExecutorWithMutator` and `Execute` (PR 4; `:60`, `:109`,
  `:154`, `:191`, `:221`), one drives `NewExpressionRule` (PR 6; `:273`). Only `:358`
  `TestSubstituteVariablesTyped_ReturnContract` tests PR 3 code alone.
- `lifecycle_substitution_test.go`: `:301`, `:329`, `:460` drive `NewExpressionRule` (PR 6); `:372` and `:432` drive
  `StatefulEvaluator` (PR 7).
- `for_each_substitution_test.go`: 7 of 8 test `SubstituteVariablesWithIterVar` and `ResolveListValue` (§ 6, F11);
  `:161` tests `coerceTripleObjectToStrings`, which `.length` and `.triples` also use.

## 2. Pin probe

- **Not run in this session.** The run of PR 3's 92 tests on the pin copy (`go test -race -run '^(…)$'
  ./processor/rule/`, then `-count=5 -cpu=1 -shuffle=on`) was refused by the session's permission system. The
  evidence on record is the split plan's whole-package probe, which includes these files: `go test -count=1 -race
  ./processor/rule/...` ok, `-count=5 -cpu=1 -shuffle=on` ok, seeds 1795602496010215000 and 1795602495775027000
  (archived split `inventory.md:119-123`). A PR-3-only run is owed before the design cites a pin test result.
- Test-text rules over the 10 PR 3 test files (`grep -n -E 'time\.Sleep|\.Skip|//go:build'`): 0 sleeps, 0 skips, 0
  build tags. `t.Parallel` appears in 9 files; none is integration-tagged, so the rule against `t.Parallel` in
  integration files does not apply.
- Context in the carried production files: no struct field of type `context.Context` (`grep -n -E
  '^\s+[a-zA-Z]+\s+context\.Context'`: 0). `context.Background`, `context.TODO`, `WithoutCancel`: 0.
- Test-side compile drift (what PR 3's tests need that main and PR 2 do not provide):
  - `internal/semantictest` (5 files) is `internal/harness/semantictest` on main (`EntityID` at
    `internal/harness/semantictest/fixtures.go:20`).
  - `state_tracker_test.go:174` — `bucket := newMockKVBucket()` and `lifecycle_substitution_test.go:376` use
    `mock_kv_test.go` (216 lines), which the split assigns to PR 8. The mock's `Keys` returns an empty slice and no
    error on an empty bucket (`mock_kv_test.go:84-93`); nats.go's `kvs.Keys` returns `ErrNoKeysFound` there (source
    read, not a probe: `jetstream/kv.go:1389` at v1.52.0, the pin's `go.mod:12`; `:1419` at v1.55.0, main's
    `go.mod:18`).
  - `typed_substitution_test.go` needs `testExecutorPlatform` and `mockTripleMutator`-style helpers from
    `actions_test.go` (PR 4); `testPlatform` lives in `platform_test.go` (PR 8).
  - `lifecycle_substitution_test.go:442` — `ctx := context.WithValue(context.Background(), contextKey("evaluation"),
    "exact")` (test proves the evaluator passes its own context to the lookup; PR 7's test).
  - Four tests replace the process logger and cannot run in parallel: `message_substitution_test.go:200`,
    `triple_value_substitution_test.go:153`, `:182`, `:265` ("Not t.Parallel(): slog.SetDefault mutates
    package-global state").
  - `context.Background()` appears in 9 of the 10 test files (`:78` of `caller_substitution_test.go` and on);
    `t.Context()` is main's form.

## 3. Category 1: the claimed gap

PR 3 claims no gap; it carries pin code. The claims the upstream documents make about PR 3, checked:

- **"`ExecutionContext` is Framework API because `Run` names it"** (target-shape page, header and § 2). `Run` does not
  exist yet: PR 2's design D2 gives `ActionHandler` only `Check` and says "PR 4 adds `Run` once PR 3 has brought
  `ExecutionContext`" (`origin/claude/rule-core-02-definitions:…/design.md`, D2). Main has no `processor/rule`
  package (`ls processor/rule`: `docs`, `expression`).
- **"`ExecutionContext`'s substitution methods … become the family-facing contract, or are dropped; PR 2's design
  settles which"** (`split.md`, PR 4 corrections). PR 2's design does not settle it: D1 to D10 never name
  `SubstituteVariables`, `SubstituteVariablesWithIterVar` or `ResolveListValue` (`git show …:design.md | grep -c
  Substitute`: 0). The target-shape page leaves it to "PRs 2, 3, 4: what a product handler gets (substitution, the
  triple writer `triple_mutator.go:27`, a publisher)" (§ "Who carries it"). Open for this design.
- **"PR 3 decides whether the stored `MaxIterations` copy stays"** (target-shape page § 4). Facts in § 4e.
- **"`MatchState` … less `ActionIterations`"** (target-shape page § 4; `FireState` is PR 7's). The pin's field:
  `state_tracker.go:62` — ``ActionIterations map[string]int `json:"action_iterations,omitempty"` ``. Its only
  production writer and reader is the evaluator (`stateful_evaluator.go:192-195`, `:403-423`, PR 7).
- **"The substituter reads `expression.SubstitutionPrefixes()`"** (PR 1 design `:44-46`, `:85`; restated on #9 in
  comment 7165028479). On main: `processor/rule/expression/regex.go:21` — `func SubstitutionPrefixes() []string {`,
  returning seven prefixes (`:22`). The pin's substituter spells its own set (§ 4a).

## 4. Category 2: every current spelling

### a. The set of substitution namespaces

| Spelling | Location | Members |
| --- | --- | --- |
| Unresolved-token regex | `execution_context.go:35` — `` var unresolvedTemplateVarRe = regexp.MustCompile(`\$(?:entity\|related\|state\|schedule\|caller\|message)\.[a-z0-9][a-z0-9_.-]*`) `` | 6 (no `$now`) |
| Substitution passes | `execution_context.go:288-381` | `$now`, entity, related, state, lifecycle, schedule, caller, message, overlay |
| Regex refusal list (PR 1) | `expression/regex.go:22` (main) | `$now`, `$entity.`, `$related.`, `$state.`, `$schedule.`, `$caller.`, `$message.` |
| Condition pseudo-fields | `stateful_evaluator.go:220-226` (PR 7), `lifecycle_substitution.go:86-88` | `$state.iteration`, `$state.max_iterations`, `$state.last_transition`, `$prev.<field>`, three `$entity.lifecycle.*` |
| Function doc | `execution_context.go:207-251` | the list authors read |
| Package docs | `docs/syntax.md:213-260`, `docs/actions.md:62`, `:155`, `:252` | includes `${entity.field}` (§ 10, F26) |

### b. The token grammar (predicate and path character classes)

| Where | Text |
| --- | --- |
| `execution_context.go:35` | `[a-z0-9][a-z0-9_.-]*` (unresolved check) |
| `execution_context.go:55` | `` var tripleLengthRe = regexp.MustCompile(`\$(entity\|related)\.triple\.([a-z0-9.-]+?)\.length\b`) `` |
| `execution_context.go:88` | `` var tripleTriplesRe = regexp.MustCompile(`\$(entity\|related)\.triple\.([a-z0-9.-]+?)\.triples\b`) `` |
| `execution_context.go:114` | `` var tripleValueRe = regexp.MustCompile(`\$(entity\|related)\.triple\.([a-z0-9][a-z0-9.-]*)\.value\b`) `` |
| `execution_context.go:343` | `key := "$entity.triple." + triple.Predicate` (plain `strings.ReplaceAll`, no boundary) |
| `typed_substitution.go:79` | `` var typedEntityTripleRe = regexp.MustCompile(`^\$entity\.triple\.([\w.-]+)$`) `` |
| `typed_substitution.go:83` | `` var typedRelatedTripleRe = regexp.MustCompile(`^\$related\.triple\.([\w.-]+)$`) `` |
| `typed_substitution.go:89` | `` var typedMessageRe = regexp.MustCompile(`^\$message\.(\w+(?:\.\w+)*)$`) `` |
| `message_substitution.go:54` | `` var messageTokenRe = regexp.MustCompile(`\$message\.[\w]+(?:\.[\w]+)*`) `` |

- The canonical predicate grammar on main: `vocabulary/predicate_contract.go:70` — `func ParsePredicate(predicate
  string) (PredicateParts, error) {` (three segments, each starting lower-case, `:98`). The pin's `.value` pass
  calls it (`execution_context.go:655`); no other pass does.

### c. Which entity a token reads, string path against typed path

| Token | String path | Typed path |
| --- | --- | --- |
| `$entity.triple.<p>` | first triple's object, `%v` (`:336-346`) | first triple's object as is (`typed_substitution.go:141-143`) |
| `$related.triple.<p>` (bare) | **no pass** (the generic loop reads `ec.Entity` only, `:336`) | first triple of `ec.Related` (`:144-146`) |
| `$message.<path>` | `%v` of the value (`message_substitution.go:79`) | the value (`typed_substitution.go:147-152`) |
| `$state.iteration`, `$state.max_iterations` | `%d` from `ec.State` (`:301-304`) | `int` from `ec.State` (`:155-164`) |

### d. "The values of a predicate" (three spellings)

1. `.length` — `execution_context.go:479-503`: returns on the **first** triple with the predicate; a scalar object
   gives `[ERROR_LENGTH_NOT_LIST:<p>]` (`:500`), a list object its element count.
2. `.triples` — `collectPredicateValues`, `:693-716`: every triple, scalars and list elements together.
3. Condition list operators (PR 1, main): `openspec/specs/rule-conditions/spec.md:79-83` — "SHALL read every triple
   carrying the predicate".

So for a predicate stamped as N scalar triples (the pin's "Pattern A", e.g. `gather.child.completed`, `:518-520`),
`.triples` gives N values, a `length_eq` condition counts N, and `.length` gives the error sentinel (read, not run).
ADR-048 says `.length` "resolves to the COUNT of values" (`048-…md:264`).

### e. Rule-level `max_iterations` and the iteration count

- Stored copy: `state_tracker.go:43-45` — "MaxIterations is the configured limit from the rule Definition. 0 means no
  limit." Written from the rule every evaluation: `stateful_evaluator.go:205` — `MaxIterations:
  ev.Rule.MaxIterations,`.
- Readers of the stored copy: `execution_context.go:303` (string) and `typed_substitution.go:160-164` (typed).
- Conditions read the rule, not the copy: `stateful_evaluator.go:222` — `"$state.max_iterations":
  ev.Rule.MaxIterations,`.
- Nothing compares an iteration against it (PR 2 inventory § 4g; PR 2 design D5 keeps the field "as the value
  `$state.max_iterations` reads, documented as not a limit").
- `$state.iteration`: `state_tracker.go:41` (`Iteration`), incremented on entry at `stateful_evaluator.go:183-186`;
  conditions read the local `iteration` (`:221`), templates read `ec.State.Iteration` (`:302`), the same value.

### f. Resolving lifecycle state (two resolvers)

- Templates: `applyLifecycleSubstitutions`, `lifecycle_substitution.go:164-217`: four tokens including
  `workflow_def`, memoised on the `ExecutionContext` (`:221-236`, field `execution_context.go:183`), a failed lookup
  cached and the tokens left in place.
- Conditions: `populateLifecycleStateFields`, `lifecycle_substitution.go:78-90`: three keys into `StateFields`
  (`:86-88`), no memo. `PopulateLifecycleStateFields` (`:54-59`) discards the error (`_ =`); the stateless path keeps it
  (`matches.go:158`, PR 6).

### g. The `RULE_STATE` key and who reads it

- `state_tracker.go:226-228` — `return ruleID + "." + entityKey`; pair key `:258-263` — `entityID1 + "_" +
  entityID2`, sorted.
- `stateKeyBelongsToEntity`, `:251-253` — `return entityID != "" && strings.HasSuffix(key, "."+entityID)`.
- Rule IDs: PR 2 refuses a dotted rule ID (PR 2 `tasks.md:24`, "a dotted ID"), so a key's first token is the rule.
- Operators read keys by hand: `docs/operations.md:127`, `:143`, `:149` (`nats kv get RULE_STATE "<rule>.<entity>"`,
  `nats kv ls RULE_STATE | grep …`).

### h. Transition kind

`state_tracker.go:17-26` (`type Transition string`, three constants) and `:34` — ``LastTransition string
`json:"last_transition"` // ""|"entered"|"exited"`` (the same set as a plain string).

## 5. Category 3: adjacent claims

### Open PRs

`gh pr list --state open` (each file list read with `gh api --paginate …/files`):

- **#758** (draft, `claude/rule-core-02-definitions`, 6 files, all under `openspec/changes/rule-core-02-definitions/`,
  including a new capability `specs/rule-definitions/spec.md` and a `harness-boundaries` delta). No file overlaps
  PR 3's two. Overlap in substance: PR 3's code lands in the `processor/rule` package PR 2 creates, extends PR 2's
  ledger row (PR 2 task 2.8: "`adapt`, partial: names what PRs 3-12 add"), its `scripts/cover-check.sh` entry (80%)
  and its `docs/repository-map.md` row. `split.md` makes PR 3 wait for PR 2; `tasks.md` 1.2 holds on #758.
- **#759** (this claim): `openspec/changes/rule-core-03-context/{proposal,tasks}.md`.

### What PR 3 needs from PR 2 (at 11bcabaa)

- The `processor/rule` package and `Definition` with `MaxIterations` (D1, D5).
- `LifecycleManager`: `ExecutionContext.Lifecycle` has that type (`execution_context.go:175`). At the pin it is in
  PR 2's range of `actions.go` (`:504-532`; split inventory `:31`), but PR 2's design does not list it among what it
  carries (D1) and its tasks name it only as the `lifecycle_*` handlers' key sets (`tasks.md:54-55`). No PR's design
  places it yet. Its eight methods (`actions.go:509-532`) include two that leave with the agent-run path (`Get` `:522`,
  `Create` `:526`, "ADR-053 D4"); substitution calls two (`LookupByEntityID` `:513`, `GetWorkflowDefinition` `:514`). Main's
  `*lifecycle.Manager` has all eight (`pkg/lifecycle/manager.go:244`, `:320`, `:529`, `:774`, `:836`;
  `manager_query.go:460`, `:511`, `:553`).
- `ActionHandler` without `Run` (D2): PR 3 provides the type `Run` will name.

### What PR 1 gives PR 3 (main)

`expression.ConditionExpression` (`types.go:12`, with the unexported compiled `pattern`, `:31-33`),
`ExtractMessageValue` (`message_path.go:20`), `MessageFields` (`types.go:128`), `StateFields` (`types.go:112`),
`SubstitutionPrefixes` (`regex.go:21`). `Compile` refuses a regex holding a substitution prefix (`regex.go:64-67`), so
`substituteConditionValues`'s copy (`execution_context.go:422-428`) cannot rewrite a regex condition's value; a pattern
holding a bare `$` (e.g. `foo$`) is re-assigned an equal string, and `compiledFromValue` compares strings
(`regex.go:83-86`). Both read, not run.

### Graph, identity, KV and lifecycle on main

`graph/types.go:23` (`EntityState`), `pkg/types/entity_id.go:132` (`ParseEntityID`; named fields `:90-99`),
`vocabulary/predicate_contract.go:70`, `natsclient/kv_key_contract.go:118` (`ValidateKVLiteralKey`; bounds `:13-17`),
`natsclient/kv.go:48` (`KVStore`), `:575-589` (`Keys`: "An empty bucket gives nil and no error"), `:617-625`
(`KeysByFilter`, "fixed-position wildcards such as `*.org.platform.system.domain.type.instance`").
`RULE_STATE` is not in `graph/kvcatalog` (`git grep -n -i 'RULE_STATE\|RuleState' -- graph/kvcatalog`: 0).

### Specs, ADRs and the ledger

- `openspec/specs/rule-conditions/spec.md:57-67` (transition reads `$prev.`, i.e. `MatchState.FieldValues`),
  `:79-90` (§ 4d).
- `openspec/specs/error-classification/spec.md:11` ("An unclassified error is classified fatal"): every
  `StateTracker` error is a bare `fmt.Errorf` (`state_tracker.go:106-207`) except `ErrStateNotFound` (`:74`).
- SemEngine ADR-111 cites this code: `RULE_STATE` written with `Put` by its one writer
  (`docs/adr/111-durable-execution-primitive.md:76-77`); the `SourceRevision` guard as the applied-sequence shape
  (`:110-111`); "a TTL is already forbidden on a bucket that must not be evicted … reclaim is an explicit step"
  (`:184-186`).
- Ledger on main: no `processor/rule` row; PR 2 adds it.

### Owed to PR 3 by earlier designs

- PR 1 (`design.md:44-46`, `:85`; #9 comment 7165028479): read `SubstitutionPrefixes()`.
- Target-shape page § 4: the stored `MaxIterations` copy; `MatchState` less `ActionIterations`.
- Target-shape page "Who carries it": what a product handler gets (substitution).
- `split.md` "ADRs and guidance": "The reviewer's `$entity.triple.*` clause with PR 3" (pin
  `.agents/contracts/semstreams-reviewer.md:241-242`); SemEngine's reviewer contract defers it to the rule port
  (`.agents/contracts/semengine-reviewer.md:325-326`).
- Ruling 9 (PR #752 comment 7162146299): a usage count for each feature carried, zeros to the owner (§ 12).

### Issues

Listed in § 9.

## 6. Category 4: who each carried exported name is for

Pin-wide production readers outside PR 3's files (`grep -rn --include='*.go' -F` over the pin, `_test.go` excluded;
script `readers.sh`, local only), and consumers at symbol level (import alias, then `<alias>.<Name>` per file):

| Name | Pin production readers (PR) | semteams 4df44ae8 | semboids |
| --- | --- | --- | --- |
| `ExecutionContext` (fields `EntityID`, `Entity`, `State`, `MessageData`, `Lifecycle`) | built at `stateful_evaluator.go:210`, `:289`, `:550` (7); `expression_factory.go:196`, `:295` (6); `cron_scheduler.go:599` (11) | 5 test files; literals set `EntityID`, `Entity` (`test/contract/research_loop_budget_test.go:70`, `rule_family_consumer_test.go:160`); test fakes implement `Execute(ctx, Action, *ExecutionContext)` | 0 |
| `ExecutionContext.RelatedID`, `.Related` | **no production writer**: the two `Evaluation{` literals set neither (`message_handler.go:145-150`, `:329-336`); read at `actions.go:1180` | 0 | 0 |
| `ExecutionContext.Caller`, `CallerContext` | **no writer pin-wide** (`CallerContext` 0 hits outside its file) | 0 | 0 |
| `ExecutionContext.Schedule` | `cron_scheduler.go:600` (11); type in `cron_substitution.go:41` (11) | 0 | 0 |
| `RuleID()` | 15 sites in `actions.go` (4, 5), e.g. `:1060` — `e.tripleMutator.AddTriple(ctx, ec.RuleID(), triple)` | 0 | 0 |
| `SubstituteVariables` | `actions.go:977`, `:1017`, `:1022`, `:1092`, `:1093`, `:1141`, `:1158`, `:1239`, `:1244`, `:1356` (4, 5) | 1 test file, 2 calls (`autoresearch_reply_integration_test.go:145`, `:149`) | 0 |
| `SubstituteVariablesTyped` | `actions.go:1019`, `:1241`, `:1353` (4, 5) | 0 | 0 |
| `SubstituteVariablesWithIterVar` | `actions.go:1497-1786`, 9 sites, all `publish_agent` (not carried, E3) | 0 | 0 |
| `ResolveListValue` | `actions.go:1711` (`publish_agent` `for_each`, not carried) | 0 | 0 |
| `SubstituteConditionValues` | `stateful_evaluator.go:291`, `:557` (7); `expression_factory.go:301` (6) | 0 | 0 |
| `substituteConditionValues` (unexported twin) | `expression_factory.go:198` — `expr.Conditions = substituteConditionValues(nil, expr.Conditions, ec)` (6; **nil context**) | — | — |
| `PopulateLifecycleStateFields` | `stateful_evaluator.go:231`, `:283` (7) | 0 | 0 |
| `MatchState` | `stateful_evaluator.go:197-208` (7) | 0 | 0 |
| `StateTracker`, `NewStateTracker` | `processor.go:659` (8) | 2 test files (`research_loop_budget_test.go:471`, `:500`; `autoresearch_reply_integration_test.go:71`), each passing its own in-memory `jetstream.KeyValue` | 0 |
| `Get`, `Set` | `stateful_evaluator.go:136`, `:249` (7) | via the evaluator | 0 |
| `Delete` | **no production caller** (`grep -rn 'stateTracker.Delete('`: 0) | 0 | 0 |
| `DeleteAllForEntity` | `entity_watcher.go:857` (10) | 0 | 0 |
| `DetectTransition`, `Transition*` | `stateful_evaluator.go:170` (7) | 0 | 0 |
| `ErrStateNotFound` | `stateful_evaluator.go:140` (7) | 0 | 0 |
| `buildPairKey` | `stateful_evaluator.go:115`, only when `RelatedID != ""` (never, row 2) | — | — |
| `LifecycleManager` (PR 2's file) | field type here; executor (5) | 2 test files (`approvalpause/rule_order_test.go`, `autoresearch_reply_integration_test.go`) | 0 |

`MatchState` fields read by no production code (writers `stateful_evaluator.go:200-203`, every evaluation):
`LastTransition`, `TransitionAt`, `LastChecked`. `SourceRevision` is read at `stateful_evaluator.go:155`;
`FieldValues` at `:225`, `:277`; `IsMatching` at `:144`.

## 7. Category 5: problem shapes and their nearest instances

- **Text templates over a context value.** No template substituter on main (`git grep -n -E 'os\.Expand|text/template|
  ReplaceAll\([^,]+, "\$' -- '*.go'`: 0 production hits). PR 3 brings the first; it is used by one package, so it is
  not a cross-package primitive and no adoption sweep is owed.
- **A package's own KV record store.** `graph/embedding/storage.go:173-198`: a `Storage` over `*natsclient.KVStore`,
  validating keys with `natsclient.ValidateKVLiteralKey` (`:197-198`), every read-modify-write through
  `UpdateWithRetryRead` (`:163`). `graph/clustering/storage.go` uses the validators the same way. The pin's
  `StateTracker` holds a raw `jetstream.KeyValue` (`state_tracker.go:69`) and validates nothing but emptiness.
- **Delete every record of one entity.** `natsclient.KVStore.KeysByFilter` (`kv.go:617-625`) lists a fixed-position
  filter, so `*.<entity ID>` names exactly the single-entity keys when rule IDs have no dots (§ 4g). The pin lists every
  key and filters by suffix (`state_tracker.go:205-219`).
- **Nil context refused at an exported boundary.** `natsclient.KVStore` refuses nil (`kv.go:576-578`,
  `nilContextErrorOf`). `StateTracker.Get`, `Set`, `Delete`, `DeleteAllForEntity` do not check
  (`state_tracker.go:103-222`).

## 8. Category 6: the pin's decision records for the carried files

Search: all 85 records at the pin's `docs/adr`, `grep -l -E 'execution_context|ExecutionContext|SubstituteVariables|
substitution|\$entity\.|\$message\.|\$caller\.|\$state\.|MatchState|StateTracker|RULE_STATE|state_tracker'` (16 hits,
each read at the hit); `grep -o -E 'ADR-[0-9]{3}'` per carried file (`execution_context.go`: 039, 046 ×5, 047 ×2,
048 ×6; `lifecycle_substitution.go`: 047; `message_substitution.go`: 039; the other four: none); SemEngine's
`docs/adr` for the same terms.

| ADR | Title | What it decides for PR 3's code | Cited at | In SemEngine `docs/adr` |
| --- | --- | --- | --- | --- |
| 048 | BoundedDispatcher and `.triples` substrate | `.triples` renders a JSON array; bare `$entity.triple.P` is the first value; `.length` is "the COUNT of values" | `048-…md:253-268`; `execution_context.go:58-88` | no |
| 047 | Lifecycle harness substrate | the four `$entity.lifecycle.*` tokens (`:342-347`); "No `$now` substitution" (`:373`) | `lifecycle_substitution.go:1-22` | no (PR 5 per split) |
| 039 | Tool-call governance is rule-driven | adds `$message.*` (`:94`, `:221-226`, `:413`) for verdict templates; deny/approve not carried (E4) | `message_substitution.go:1-8`; `execution_context.go:161-164` | no |
| 046 | Parallel fan-out | the `for_each` overlay and `ResolveListValue` (`:107-111`, `:505-506`) | `execution_context.go:256-283`, `:718-742` | no; E3 removes `for_each` from the core |
| 032 | Policy DSL, tenancy, cluster | `$caller.{id,role,org}` shipped in Tag 1 (`:14`); its writer is Tag 2, "Pending" (`:15`) | `caller_substitution.go:1-24` (by subject) | no |
| 041 | Unified condition evaluator | `ExtractMessageValue` shared by conditions and `$message.*` (`:95-96`, `:121`, `:126`) | `message_substitution.go:75` | yes (PR 1) |
| 056 | Authoritative semantic state | a rule stamping an entity reference uses `$entity.id`, not `$entity.instance` (`:1761-1768`) | `entity_substitution.go` (by subject) | no |
| 054 | Indexing eligibility | a new reserved predicate namespace needs "a grammar-collision audit against every `$`-substitution regex" (`:319-321`) | (by subject) | no |
| 028 | Orchestration architecture | "Rules carry references, not content … should never substitute free-text content predicates" (`:119-122`) | (by subject) | no (PR 13 carries the clause) |
| 031 | Time-trigger primitive | `$schedule.*` substitution and `lastFiredNanos` (`:277`) | `cron_substitution.go` (PR 11) | no (PR 11) |
| 102, 104 | Entity-ID positions; unique platform authority | segment tokens resolve by named field (`entity_substitution.go:37-58`) | `pkg/types/entity_id.go:90-99` | yes |
| 111 (SemEngine) | Durable execution primitive | `RULE_STATE` single writer with `Put` (`:76-77`); `SourceRevision` guard shape (`:110-111`); no TTL, explicit reclaim (`:184-186`) | `state_tracker.go:136-163` | yes |
| 110 (SemEngine) | The write seam | no `EntityState.Version`; `SourceRevision` is the watch entry's revision, not a version field | (by subject) | yes |

Records searched with no PR 3 decision: 040 (retired boid subsystem, history), 043 (unrelated use of
"substitution"), 045 and 053 (agentic; `LifecycleManager.Get`/`Create` cite ADR-053 D4), 055 (`MessageData` feeds
`executeDeny`, not carried).

## 9. Open issues against `processor/rule`

`gh issue list --state open --search "rule" --limit 200` (37 hits, each read, comments included); then the same
query for `substitution`, `execution_context`, `ExecutionContext`, `state_tracker`, `StateTracker`, `MatchState`,
`RULE_STATE`, `caller_substitution`, `typed_substitution`, `lifecycle_substitution` (0 hits each); then a `grep`
of every hit's body and comments for the carried file names and types (one hit: #9).

| Issue | What it names | PR 3's disposition (from the split plan; design confirms) |
| --- | --- | --- |
| #9 | comment 7165028479: "PR 3: the substituter reads `expression.SubstitutionPrefixes()`" | stays (epic); the commitment is § 10 F1 |
| #25 | `actions.go:916-945` | stays; retires at PR 7 |
| #26 | `actions.go:191`, `:206` | stays; retires at PR 2 |
| #27 | `config_validation.go`, `actions.go` agentic paths | stays; PR 4. Bears on F11: `SubstituteVariablesWithIterVar` and `ResolveListValue` are read only by `publish_agent` |
| #28 | `actions.go:497-503` | stays; PR 7 |
| #294 | `config_validation.go:382-385` | stays; PR 5 |
| #702 | `action_id.go:17`; "cooldown is kept per rule and entity, in the rule's stored state" (body `:18`) | stays; PR 7 adds `FireState` to the record PR 3 carries |
| #703 | `cron_rule.go`; "the stored state records that the deadline has fired" (comment, `:79`) | stays; PR 14 adds the field |
| #714, #196, #195, #350, #704, #24 | skill; adoption lists; constraint model; decisions; durable-execution epic (ADR-111) | stay; no PR 3 work |
| #756, #757 | `processor/rule/expression` (PR 1's package, which PR 3 imports) | stay; #757 names list values Go code builds (`[]string`), the shape `coerceTripleObjectToStrings` also accepts (`execution_context.go:789-790`) |

Names the word, not the package: 60, 84, 109, 110, 111, 134, 141, 155, 284, 300, 484, 535, 554, 576, 614, 635, 665,
683, 712, 730, 734. PR 3 retires none (`split.md`, Issues).

## 10. Shape sweep (pin copy, with the disposition each finding's basis supports)

### Second spellings

- **F1.** The namespace set is spelled in the substituter (`execution_context.go:35`, the passes `:288-381`) and in
  PR 1's `SubstitutionPrefixes()` (§ 4a). *Required:* accepted design commitment (PR 1 design `:44-46`, `:85`; #9
  comment 7165028479). Consequence: dropping a namespace (F9, F10) changes `SubstitutionPrefixes()` in PR 1's package.
- **F2.** The token grammar is spelled nine times with five character classes (§ 4b). *Optional* (inherited
  duplication) except as F1 binds it.
- **F3.** Three meanings of "the values of a predicate" (§ 4d): `.length` stops at the first triple and errors on a
  scalar; `.triples` and the condition list operators read every triple; ADR-048 calls `.length` "the COUNT of
  values". Read, not run. *Required:* the design keeps or changes ADR-048's decision and says so (architect contract,
  "Cite the pin's decision records"); semteams' 4 `.length` uses (§ 12) read one list-valued triple
  (`coordinator.decision.subtopics`), so they do not show which.
- **F4.** Two lifecycle resolvers (§ 4f) with different key sets and caching. *Optional* (inherited); ruling 9:
  `$entity.lifecycle.*` is used by 0 rules (§ 12), so carry or drop goes to the owner.
- **F5.** `$state.max_iterations` comes from the stored copy in templates and from the rule in conditions (§ 4e).
  *Required to decide:* target-shape page § 4 ("PR 3 decides whether the stored `MaxIterations` copy stays").
- **F6.** The `$state` namespace differs between conditions (`last_transition`, `$prev.*`) and templates
  (`iteration`, `max_iterations` only). *Optional.*
- **F7.** `Transition` and `LastTransition string` spell one set twice (§ 4h). *Optional.*

### Copied shells

None: PR 3 carries no `Start`, `Stop`, `Health` or watch loop.

### Layered interfaces with one implementation

- **F8.** `LifecycleManager` (`actions.go:504-532`): one production implementer, `*lifecycle.Manager`; "Defined as an
  interface so the executor can be tested without a running NATS client" (`:507-508`); substitution needs 2 of 8
  methods. *Required to place:* build need, since `execution_context.go:175` names it and no PR's design carries it
  (§ 5). The interface's width is *optional* (inherited). `lifecycleParticipantSnapshot` and the anonymous interface
  of `participantSnapshot` mirror `lifecycle.Participant` (`lifecycle_substitution.go:252-282`): *optional*.

### Exported surface with no reader

- **F9.** `CallerContext`, `ExecutionContext.Caller`, `applyCallerSubstitutions` (`caller_substitution.go:1-70`,
  `execution_context.go:150`, `:363`): no writer pin-wide (§ 6); ADR-032's writer was never built; 0 rules use
  `$caller.*`. *Required:* surface audit (a)/(b), dead surface is not protected by Bounded port scope; ruling 9 puts
  carry or drop to the owner.
- **F10.** The related entity: `RelatedID`, `Related` (`execution_context.go:123-130`), `$related.*` in every pass
  (`:292`, `:298`, `:469-470`, `:561-562`, `:669-670`, `:753-755`; `typed_substitution.go:83`, `:144-146`),
  `buildPairKey` (`state_tracker.go:258-263`), and the pair-key reasoning in `stateKeyBelongsToEntity`
  (`:245-250`). No production code sets either field (§ 6); 0 rules use `$related.` (§ 12); PR 2 drops
  `related_patterns` (D5). *Required:* surface audit (a)/(b); ruling 9, owner.
- **F11.** `SubstituteVariablesWithIterVar` (`execution_context.go:263`), the overlay (`:377-379`), `ResolveListValue`
  (`:743`): read only by `publish_agent`, which leaves the core (#27, E3). semteams has 2 `for_each` actions, which
  its own `publish_agent` handler will run. *Required to decide* (surface audit (a)): kept as what a product handler
  gets (target-shape page, "PRs 2, 3, 4: what a product handler gets"), with a contract line and a test calling it as a
  product would, or dropped. `split.md` sent this to PR 2's design, which did not settle it (§ 3).
- **F12.** `SubstituteConditionValues` and its unexported twin (`execution_context.go:413-431`): two names for one
  function; the twin's one caller passes a nil context (`expression_factory.go:198`, PR 6). *Optional* for the twin
  (inherited); the nil context is *required* in PR 6 (architect contract: "Callers never pass nil"). Exported names
  whose readers are all in the package (`SubstituteConditionValues`, `SubstituteVariablesTyped`,
  `PopulateLifecycleStateFields`, `DetectTransition`, `ErrStateNotFound`): in-package callers count as readers, so
  *optional* to unexport.
- **F13.** `StateTracker.Delete` (`state_tracker.go:166-180`) has no production caller (§ 6); unlike `Get` and `Set`
  it accepts empty IDs. *Required:* surface audit (a) (drop, or keep with its reason).
- **F14.** Nil-bucket branches "for tests" (`state_tracker.go:112-115`, `:145-148`, `:167-170`, `:199-202`): production
  code whose only trigger is a test. *Optional* (unexported test-only pin code, Bounded port scope).

### Classification by error text

None: no `strings.Contains(err…` or `err.Error()` comparison in the carried files (the two `err.Error()` hits,
`execution_context.go:587`, `lifecycle_substitution.go:292`, `:303`, are log fields).

### Other

- **F15.** Keys are built without the shared validator: `state_tracker.go:117`, `:150`, `:172` call `buildStateKey`
  on unchecked parts. *Required:* AGENTS.md § 2, "NATS KV keys/filters pass shared validators at entry"
  (`natsclient.ValidateKVLiteralKey`, `kv_key_contract.go:118`).
- **F16.** `DeleteAllForEntity` lists every key (`:205`), logs a failed delete and returns nil (`:212-219`), and on an
  empty bucket returns nats.go's `ErrNoKeysFound` (source read, § 2), which its caller logs at Warn
  (`entity_watcher.go:857-860`). *Optional* at the pin's bar (the skip is declared by the log); nearest shape in § 7.
- **F17.** "Orphaned pair state is left to TTL-based invalidation" (`state_tracker.go:249-250`), but the bucket is
  created with `TTL: 0` (`processor.go:643`) and a non-evictable bucket may have no TTL (AGENTS.md § 2; ADR-111
  `:184-186`). *Required:* surface audit (c); moot if F10 drops pair state.
- **F18.** Record fields whose documented meaning the code does not keep: `MaxIterations` "0 means no limit"
  (`state_tracker.go:43-45`; nothing limits, § 4e); `TransitionAt` and `LastChecked` both set to `time.Now()` and
  `LastTransition` to `""` on every evaluation without a transition (`stateful_evaluator.go:200-203`), while
  `docs/operations.md:128-137` shows `transition_at` earlier than `last_checked` and `last_transition: "entered"` on a
  later check. No production reader (§ 6). *Required:* surface audit (b)/(c) (keep with a meaning the code implements,
  or drop); the writer is PR 7's.
- **F19.** A substituted value is substituted again. Bare triple values are written at `:344`, then the lifecycle,
  schedule, caller and message passes run over the result (`:353-368`), so a triple object holding `$message.x`
  becomes the message's field, and one holding any namespace text trips the unresolved warning (`:381`). Read, not
  run. *Required if a probe shows it:* metadata or content preservation (Bounded port scope). Real-world trigger:
  triple objects carrying agent-written text, which ADR-028 `:119-122` warns rules against substituting; unmeasured.
- **F20.** No token boundary. `strings.ReplaceAll` of `$entity.triple.<p>` (`:343-344`), `$entity.id` (`:291`),
  `$state.iteration` (`:302`) and `$caller.id` (`caller_substitution.go:66`) also matches a longer token, so a
  reference to an absent predicate that extends a present one becomes `<value><rest>` with no warning. The sort at
  `:338-341` covers only the case where both are present; the pin's comment names the real pair
  (`agent.lineage.researcher-plan` and `…-plan-entity`, `:330-334`). Read, not run. *Required:* integrity (Bounded port
  scope), named trigger.
- **F21.** `$now` reads the wall clock inside substitution (`:288`); ADR-047 says "No `$now` substitution" (`:373`);
  semteams uses `$now` 3 times (§ 12); the target-shape page's `Fire` takes `now` (§ 3). *Optional* here; the ADR
  conflict is named for the design, the clock for PR 7.
- **F22.** An unresolved token is logged and the literal goes on into the action (`:245-250`, `:836-845`). The comment
  promises "callers that feed the result into an identifier (KV key, NATS subject) will then get a loud failure"
  (`:248-250`); whether a NATS subject holding `$` is refused is not probed. *Optional* (carried text, Bounded port
  scope; the library-behaviour rule asks for a probe or "not promised" if the design restates it).
- **F23.** Substitution logs to the process logger (`slog.Default()` at `:473`, `:494`, `:565`, `:584`, `:841`;
  `lifecycle_substitution.go:290`, `:301`); 4 tests must replace it (§ 2). *Optional.*
- **F24.** `lifecycle_substitution.go:152` — `func init() {` sorts a package table. Not a registration, so the
  "imports register nothing" rule does not reach it. *Optional.*
- **F25.** The lifecycle memo is written during substitution (`execution_context.go:183`,
  `lifecycle_substitution.go:230`), so an `ExecutionContext` is not safe for concurrent use. At the pin every caller
  is sequential (`actions.go:1741` — `for _, item := range items {`). Unchosen condition, no named trigger:
  *optional*; the design states the contract, since `ExecutionContext` becomes what product handlers receive.
- **F26.** Docs describing behaviour no code has: `docs/syntax.md:214-228` offers `${entity.field}` (no code parses
  `${`; 0 rule uses) and "pair rules" (F10); `entity_substitution.go:31-34` says the segment pass "runs after
  `$entity.id` and `$entity.triple.*`", but it runs before the triple passes (`execution_context.go:297` against
  `:312-346`). *Required:* surface audit (c), in the PR that carries each doc (`syntax.md`: PRs 1 and 6, `split.md`).
- **F27.** Build dependencies outside PR 3's files: `ScheduleContext` and `applyScheduleSubstitutions` are in
  `cron_substitution.go` (PR 11), named at `execution_context.go:141` and `:358`; `LifecycleManager` (F8).
  *Required:* build. The target-shape page replaces the cron scheduler with a NATS scheduled message (§ 3), so what
  feeds `$schedule.last_fired_at` later is PR 11's.

## 11. Same-class collision table: the per-rule, per-entity record (`RULE_STATE`)

| Dimension | Evidence |
| --- | --- |
| Semantic class | execution state for one rule and one entity (or pair): match flag, entry count, previous field values, applied revision, firing counts (ADR-111 Decision 1 calls this execution state) |
| Owners | pin: `StateTracker` (`state_tracker.go:68`); `ScheduleTracker`/`RULE_SCHEDULES` for cron (PR 11; the target-shape page § 4 folds cron into the same record store). Main: none for rules; same shape `graph/embedding.Storage` (`storage.go:173`), graph-ingest's applied-sequence guard (ADR-111 `:77`) |
| Catalogs | not in `graph/kvcatalog` (§ 5); created ad hoc by the processor (`processor.go:637-655`, PR 8) |
| Status | none: no metric or status names the bucket; operators read keys by hand (`docs/operations.md:127-149`) |
| Lifecycle | created if absent at processor start (`processor.go:637-651`); written after actions (`stateful_evaluator.go:247-249`); single-entity records deleted on entity delete (`entity_watcher.go:856-860`); pair records never (F17) |
| Ownership | one writer, the rule processor (ADR-111 `:76-77`); `Put`, no compare-and-set (`state_tracker.go:157`) |
| Readers | `stateful_evaluator.go:136`; operators (docs); semteams tests through their own in-memory bucket (§ 6) |
| Writers | `stateful_evaluator.go:249`; `DeleteAllForEntity` (`entity_watcher.go:857`); `Delete` has none (F13) |
| Recovery | bootstrap re-reads saved state to decide `OnRecovery`/`RerunOnRecovery` (`stateful_evaluator.go:127-132`, `:172-181`); a record that fails to decode is an evaluation error (`state_tracker.go:128-130`, `stateful_evaluator.go:139-142`) |

## 12. Adopter seam inventory

### Usage counts (ruling 9; checked-in rule JSON only)

`grep -rhoE '<token>' configs/rules` over each repository (`\b` as ugrep reads it). semteams: 85 rule files;
semboids: 3 rule files (7 rules, PR 2 inventory § 12).

| Token | semteams uses (files) | semboids uses (files) |
| --- | --- | --- |
| `$entity.triple.` (every form) | 231 (67) | 0 |
| `… .length` / `.triples` / `.value` | 4 (3) / 5 (4) / 12 (4) | 0 |
| `$entity.id` | 54 (24) | 0 |
| `$entity.<segment>` | 167 (52): `instance` 151, `org` 8, `platform` 8 | 0 |
| `$state.iteration` | 40 (7): 7 as a condition `field`, the rest in templates | 0 |
| `$state.max_iterations` | 1 (1) | 0 |
| `$now` | 3 (3) | 0 |
| `$schedule.` | 3 (1): the disabled cron rule `research/10-scheduled-program-pulse.json` | 0 |
| `$message.` | 0 | 26 (3) |
| `$related.`, `$caller.`, `$entity.lifecycle.`, `$state.last_transition`, `${` as a token | 0 each | 0 each |
| `for_each` | 2 (2) | 0 |
| Substituted condition `value` | 11 | 0 |

### Person 1: a semteams developer (rules and tests)

- **What they must know:** which tokens resolve where (`$state.*` differs between conditions and templates, F6; a
  `when` condition's value is substituted with no `State`, `stateful_evaluator.go:550-555`, so `$state.*` there stays
  literal); that `.length` on a multi-valued scalar predicate is an error sentinel (F3); that a token naming an absent
  predicate which extends a present one is silently rewritten (F20). Their tests build `ExecutionContext` literals
  (`EntityID`, `Entity`) and hand `NewStateTracker` an in-memory `jetstream.KeyValue` (§ 6).
- **If they do nothing:** an unresolved token reaches the action as literal text with a Warn log (F22); a changed
  `NewStateTracker` signature or renamed fields fail their test build.
- **Where they find out:** compile error for test code; a log line for unresolved tokens; nowhere for F20.

### Person 2: a product-handler author (semteams' `publish_agent`, after PR 4)

- **What they must know:** what `Run` receives in `ExecutionContext` and which substitution calls it may make
  (F11, undecided); that it is not safe for concurrent use (F25).
- **If they do nothing:** their 2 `for_each` actions have no list resolution or per-item overlay unless PR 3 keeps
  one.
- **Where they find out:** compile error when a method is missing; nowhere for concurrency.

### Person 3: a semboids operator

26 `$message.*` uses in 3 files. `$message.` tokens resolve only when the inbound message has the field
(`message_substitution.go:66-81`), else the literal and a Warn. Nothing else in PR 3 reaches them.

### Facts the author is asked to predict

Whether a predicate will be present at fire time (unresolved literal, F22) and whether its name is a prefix of another
(F20): both are facts the engine holds when it substitutes.

## 13. What PR 3 sends forward

- **PR 4:** `Run(ctx, Action, *ExecutionContext)` (PR 2 D2); the typed path for triple objects (`actions.go:1019`,
  `:1241`); `RuleID()` is `""` for a cron-built context (`cron_scheduler.go:599-605`, no `State`), and triple writes
  pass it (`actions.go:1060`).
- **PR 6:** `substituteConditionValues(nil, …)` (`expression_factory.go:198`, F12); `populateLifecycleStateFields`
  (`matches.go:158`); the contexts built at `expression_factory.go:196`, `:295`.
- **PR 7:** the `MatchState` writer and its fields (F18), `FireState` and `ActionIterations` (target-shape page § 4),
  the `when` context without `State` (§ 12), `$now`'s clock against `Fire`'s `now` (F21), `PopulateLifecycleStateFields`
  discarding its error (§ 4f).
- **PR 8:** the `RULE_STATE` bucket (created at `processor.go:637-655`, not in `graph/kvcatalog`), `NewStateTracker`'s
  caller, `mock_kv_test.go` (§ 2).
- **PR 10:** `DeleteAllForEntity`'s caller (`entity_watcher.go:857`, F16).
- **PR 11:** `ScheduleContext` and `$schedule.*` (F27).
- **PR 13:** the pin reviewer's "grammar-collision audit across existing `$` token regular expressions"
  (`semstreams-reviewer.md:259`) and developer `:259-260`.
- **PR 14:** the deadline field on the record (#703).

Intent check: no boundary moves; the port set is ruled (#8 Q15, D12). Dropping a namespace (F9, F10) removes surface
inside an admitted package, which ruling 9 sends to the owner.

Not checked: the PR-3-only pin probe (§ 2, refused); F3, F19 and F20 are read from code, not run; nats.go's
`ErrNoKeysFound` is a source read; semsource was not re-read; usage counts miss rules built in Go, in tests or loaded at
runtime.
