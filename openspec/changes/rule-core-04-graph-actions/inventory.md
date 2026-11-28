# Inventory: rule-core-04-graph-actions

base: a362eabb5a059cd1dd3b778d70862474ef5b5df1 (claim commit on `claude/rule-core-04-graph-actions`, parent origin/main
efee4f2965b6b6df792ee3f3d5189c3335d4c59f; adds only `proposal.md` and `tasks.md`; draft PR #782). Probe evidence:
`notes/inventory-probe.md`. Pin copy: `gh api repos/C360Studio/semstreams/tarball/8b99efe9`, unpacked to a scratch
directory (local only). Consumers read by `gh api …/tarball/<sha>` at the commits PR 6's inventory used: semteams
4df44ae877d0c711f769ec6b48b88100dbb267d1, semboids ab4e0fbc1b382c4fda10bae988662ae0e0bc1e5c, semsource 34dc2680.

Question: what PR 4 of the rule-core split carries ("`ActionExecutor` and `Execute` dispatching core types and
registered families; `publish`, `add_triple`, `remove_triple`, `update_triple`; `DenyVerdict`",
`archive/2026-11-25-design-rule-core-split-plan/split.md`, row 4), against the target-shape page
(`archive/2026-11-25-design-rule-core-target-shape/design.md` § 2 and its approved proposal), PR 2's handler set
(`archive/2026-11-25-rule-core-02-definitions/design.md` D2, D3, D6), PR 3's handler contract
(`archive/2026-11-26-rule-core-03-context/design.md` D7 and its forward list), the split plan's PR 4 corrections
(`split.md`, "Required corrections, by PR"), and the owner's rulings on PR #752 (comment 7162146299, items 1, 6, 9).

Track: `design` (`proposal.md:5`). Lifecycle owner: none. Nothing carried has `Start` or `Stop`, so the lifecycle suite
does not apply.

## 1. Size (measured, `sed -n a,bp | wc -l` at the pin)

| Production file | Lines | Carried (rewritten) | Not carried here |
| --- | --- | --- | --- |
| `actions.go` (PR 4's part) | 577 | `:446-465` `ParseTTL` (20), `:467-477` `TripleMutator` (11), `:479-486` `Publisher` (8), `:534-561` `ActionExecutor` (28), `:801-911` four constructors (111), `:913-1311` `Execute`, subject resolution, add, remove, properties, publish, update (399) | the rest of the file belongs to PRs 2 and 5 or leaves with `publish_agent` and deny/approve (split plan) |
| `deny.go` | 48 | 48 | 0 |
| `triple_mutator.go` | 91 | 91 | 0 |
| `publisher.go` | 231 | 0 | `:26-77` the processor's port-aware publisher (52) to PR 8; `:175-231` `publishRuleEvent` (57) to PR 9; `:1-25`, `:78-174` graph events (122), dropped by ruling |
| `graph_event_identity.go` | 49 | 0 | 49, dropped by ruling (#9 comment 7160955176) |
| **total** | **996** | **716** | **280** |

| Test file | Lines | Carried | Moved | Dropped |
| --- | --- | --- | --- | --- |
| `actions_test.go` (PR 4's part) | 1,179 | `:1-39`, `:40-185`, `:403-423`, `:440-897`, `:2651-2874`, `:3131-3188` = 946 | 0 | `:186-402`, `:424-439` = 233 |
| `actions_subject_override_test.go` | 325 | 325 | 0 | 0 |
| `action_failure_metrics_test.go` | 163 | 0 | 163 to PR 7 (it drives `runActions`) | 0 |
| `triple_mutator_revision_integration_test.go` | 234 | 0 | 234 to PR 10 (it drives the processor's own-revision skip) | 0 |
| `executor_platform_wiring_integration_test.go` | 90 | 0 | 0 | 90 |
| `graph_event_identity_test.go`, `graph_event_identity_semantics_test.go`, `publisher_graph_event_contract_test.go` | 786 | 0 | 0 | 786 |
| **total** | **2,777** | **1,271** | **397** | **1,109** |

Sent here by PR 3 and counted in PR 3's size: `typed_substitution_test.go:60-272` (five typed-object tests, each
building `NewActionExecutorWithMutator`). Carried pin lines, production plus test: 716 + 1,271 = 1,987, under the
3,000-line guide. One PR.

Why each dropped range goes: `actions_test.go:186-256` (`TestAction`) builds `Action` Go literals with `$related.id` (PR
2 refuses literals; PR 3 dropped `$related.*`); `:257-267` asserts four constants equal their own text; `:268-334` tests
`ParseTTL`, which goes if `ttl` goes (F8); `:335-377` substitutes `$related.id`; `:378-402` asserts a struct literal is
not nil; `:424-439` is `mockVerdictAuditor` (deny/approve, #28). `executor_platform_wiring_integration_test.go` pins the
executor's `platform` field, read only by `publish_agent`'s foreign-firing guard (`actions.go:563-759`), which leaves
with `publish_agent` (#27). The graph-event files test the dropped graph-event path.

## 2. Pin probe (commands in `notes/inventory-probe.md`)

- PR 4's 39 unit tests (the carried ranges of `actions_test.go` and all of `actions_subject_override_test.go`) on the
  pin copy: `-race` ok, 39 top-level `--- PASS`; `-count=5 -cpu=1 -shuffle=on` ok, `-test.shuffle
  1795863972197797000`, 0 `--- FAIL`.
- The two integration tests of `triple_mutator_revision_integration_test.go` (`-tags integration`, `-race`), Docker
  29.9.3 reachable: both `--- PASS`.
- Behaviour probe, P1 to P10 (same notes file), each observed as stated:
  - P1: with no mutator and no publisher, `add_triple`, `remove_triple`, `update_triple` and `publish` each return
    `err=<nil>` and write or send nothing.
  - P3: `remove_triple` with `object: "only-this"`: the mutator receives subject and predicate only.
  - P4: `update_triple` with `ttl: "1m"` and `properties: {"k": "v"}`: one remove and one add; the added triple carries
    `expires_at` and no trace of `properties`.
  - P5: an `add_triple` object `$entity.triple.x.y.z` on an entity without it is written as that text; the same token
    as the subject is refused (`has unresolved template variables`).
  - P6: `publish` with subject `s.$message.missing` and properties `{"n": "$message.a", "m": "$message.missing", "k":
    3}`: published to `s.$message.missing`; `m` arrives as the token text, `n` as the string `"7"` (the message holds
    the number 7), `k` as `3`; two warnings logged.
  - P7: the publish payload is `{"entity_id", "properties", "source": "rule_engine", "subject", "timestamp"}`, with
    `properties: null` when the action has none and a timestamp in local time with its offset.
  - P8 (integration, real graph-ingest): a predicate holding one statement from `some-other-component` and one from
    `rule_engine`; `remove_triple` leaves 0 statements of it: the pin removes every source's.
  - P9 (integration): `add_triple` onto an entity that does not exist returns `entity not found: <id>` (plain text).
  - P10 (integration): `update_triple` over a held value moves the revision by 2 (5 to 7): two writes; over no value, by
    1.
- Test-text rules over the PR 4 test files (`grep -n -E 'time\.Sleep|\.Skip(f|Now)?\(|//go:build'`): 0 sleeps, 0
  skips, two `integration` build tags (`executor_platform_wiring_integration_test.go:1`,
  `triple_mutator_revision_integration_test.go:1`). `t.Parallel` appears in the unit files only.
- Context in the carried production code: no struct field of type `context.Context`; `context.Background`, `TODO`,
  `WithoutCancel`: 0. Nil defaults: a nil logger becomes `slog.Default()` in each constructor (`actions.go:804-806`,
  `:825-827`, `:849-851`, `:901-903`).
- Test-side compile drift: `actions_test.go:13-25` imports `agentic`, `governance`, `processor/agentic-tools`,
  `payloadregistry` and `types` (none in this repository, or not with these names); the carried tests build `Action`
  literals with `Predicate`, `Object`, `Subject`, `Properties` and `TTL` fields (`:46-52` and on), which main's
  `Action` does not have (`processor/rule/action.go:64-92`); they call `NewActionExecutorWithMutator` and
  `NewActionExecutorFull`, and `testExecutorPlatform` (`actions_test.go:3656`, among the run-scope tests not
  carried). The fakes `mockPublisher` (`:403-423`) and `mockTripleMutator` (`:698-732`) carry over in shape.

## 3. Category 1: the claimed gap

- "PR 4 adds `Run` to `ActionHandler`" (PR 2 D2): **absent today**. `processor/rule/action.go:187-189` — `type
  ActionHandler interface {` / `Check(a Action) error` / `}`. Nothing runs an action:
  `git grep -n -E 'func .*(Execute|Run)\(' -- processor/rule ':!*_test.go'`: 0.
- "PR 4 adds a triple writer": absent in `processor/rule`. The wire it needs exists: `internal/graphmutation/
  client.go:74` — `func NewClient(requester requester, timeout time.Duration) (*Client, error) {`, and
  `graph/exact_entity.go:46` — `func NewExactEntityReader(requester classifiedRequester, timeout time.Duration)
  (*ExactEntityReader, error) {`.
- "Graph integration off refuses triple actions at load" (target-shape § 2): absent. `builtin.go:80-87` checks the
  triple predicate only; `NewHandlers` takes no setting (`action.go:202` — `func NewHandlers(product
  map[string]ActionHandler) (Handlers, error) {`).
- "The reserved-subject check after substitution": absent; the load half exists (`builtin.go:75-79`, and the one
  predicate `builtin.go:120` — `func inReservedUserResponseFamily(subject string) bool {`).
- `DenyVerdict`: absent (`git grep -n DenyVerdict -- '*.go'`: 0).
- An agentic import guard for `processor/rule`: absent. One exists for `service`
  (`internal/harness/contract/servicedeps_test.go:16` — `func TestServiceReachesNoAgenticPackage(t *testing.T) {`).
  Today `go list -deps ./processor/rule | grep -E 'semengine/(agentic|governance|vocabulary/agentic)'` prints nothing:
  the core already has no such import (no package of those names exists here).

## 4. Category 2: every current spelling

- **What an action may use at fire:** the context's fields and its fillers (PR 3 D7; `execution_context.go:27-53`),
  with one dependency already on it: `execution_context.go:48` — `Lifecycle LifecycleManager`. At the pin, the
  executor's fields (`actions.go:536-561`): `tripleMutator`, `publisher`, `kvWriter`, `lifecycle`, `toolRegistry`,
  `verdictAuditor`, `reconciler`, `targetIndex`, `revisionWriter`, `platform`, `metrics`.
- **The handler set:** one, `Handlers` (`action.go:194-196`), built by `NewHandlers`; read by `Load`
  (`load.go:67`, `:244`). The pin's second spelling, the closed switch (`actions.go:916-945`), is not on main.
- **The reserved-subject test:** one, `inReservedUserResponseFamily` (`builtin.go:120-123`). The pin spells it three
  times at fire (`actions.go:1159`, `:1768`, `:2170`, via `targetsReservedUserResponseSubject`).
- **Refusing an unfilled token:** on main, one scan returns the unfilled tokens (`execution_context.go:167` — `func
  (ec *ExecutionContext) substitute(ctx context.Context, template string, names map[string]string) (string,
  []string) {`); one caller refuses on them (`execution_context.go:300-319`, condition values). The pin's triple
  subject used a regex main does not have (`actions.go:988` — `unresolvedTemplateVarRe.FindAllString`).
- **Interpreting an append result:** `pkg/projection/mutation_client.go:219-228` (failed, then entity not found) with
  `:518-527` — `func classifiedAppendFailure(failure *graph.MutationFailure) error {`. The pin's rule writer spells it a
  second way, as text (`triple_mutator.go:48-64`).
- **Writers of `ENTITY_STATES` through the mutation lane:** `pkg/projection/mutation_client.go:56-67` (wire client and
  exact reader, both errors handled), `pkg/lifecycle/manager.go:104` — `mutationClient, err :=
  graphmutation.NewClient(client, 5*time.Second)`. A reconcile names one source on main
  (`graph/mutation_requests.go:22-36`, ``Source string `json:"source"` ``); the pin's request has no source
  (pin `graph/mutation_requests.go:27-34`).
- **Statement source names:** reserved, `graph/sources.go:21-27` (`SourceIndexingProfile`, `SourceHierarchy`,
  `SourceLifecycle`); the pin's rule writes use the literal `"rule_engine"` (`actions.go:1043`, `:1177`, `:1286`); main
  spells it nowhere (`git grep -n rule_engine -- '*.go'`: 0).
- **A triple's expiry:** `message/triple.go:94-98` (`ExpiresAt`, "the triple is considered expired after this
  timestamp"); `message/triple_identity.go:31-33` — "triple TTL is currently unenforced (no production code compares
  ExpiresAt with the clock)". Readers of `ExpiresAt` on main: `graph/helpers.go:113`, `pkg/projection/
  mutation_client.go:389`, `processor/graph-ingest/canonical_mutations.go:619-622`, each copying or comparing it;
  none expires anything.
- **Request timeout for a mutation:** `natsclient/request.go:17` — `const DefaultRequestTimeout = 5 * time.Second`,
  used by `pkg/projection/mutation_client.go:40-43` when none is given; `pkg/lifecycle/manager.go:104` writes 5
  seconds; the pin's `triple_mutator.go:15` — `const MutationTimeout = 5 * time.Second` (exported; no reader outside
  the package at the pin or in a consumer).

## 5. Category 3: adjacent claims

- Open PRs (`gh pr list --state open --json number,title,files`, all 6 read): #782 (this claim, 2 files). #760-#764
  Dependabot (`go.mod`/`go.sum`, `package*.json`, two workflows): no shared file; #761 bumps nats.go, which PR 4's
  integration test runs on. No open PR touches `processor/rule`, `internal/graphmutation`, `pkg/projection`,
  `internal/harness/contract`, `docs/admission-ledger.yaml` or `openspec/specs/rule-*`. PR 5
  (`rule-core-05-state-actions`) has no claim (`git ls-remote --heads origin | grep -i rule`: only this branch); it
  waits for this PR (split plan, merge order).
- Specs: `rule-definitions` (`openspec/specs/rule-definitions/spec.md:76-91`, the handler set; `:121-136`, built-in
  checks); `rule-substitution` (`:42-64`, an unfilled token stays and is reported); `graph-entity-writes` ("One rule
  per write mode", "An append adds only a new statement", "A conditional replace names one source", "Statement
  metadata is required"); `service-lifecycle` (`:1427-1439`, the agentic import guard's shape).
- Ledger: `docs/admission-ledger.yaml:5239` — `- source_path: processor/rule`; its destination names "the action
  executor and graph actions (4)" (`:5249`). Coverage: `scripts/cover-check.sh:78` — `"processor/rule merged"`.
- Accepted commitments that bind PR 4: target-shape § 2 (`design.md:26-67`): one handler per type in one map, built-ins
  included; `Run(ctx, a Action, ec *ExecutionContext) error`; "With graph integration off, triple `Check` refuses";
  "`publish`'s `Check` and `Run` refuse `user.response.>` as declared and after substitution, and so does the
  publisher handed to every handler"; "no setters". Its approved proposal (`proposal.md:42-52`): "The publisher the
  framework hands each handler refuses that space … semteams' handler gets the guard by using it". PR 2 D2/D3: `Run`
  and `NewHandlers`' settings arrive here. PR 3 D7 (owner ruling, PR #759 comment 7170655515): what a product handler
  gets from the context. PR #752 ruling item 6: one executor constructor, dependencies at construction, no setters;
  item 9: a usage count for each feature carried. Split plan PR 4 corrections (`split.md`): #27's import guard;
  `NewExactEntityReader`'s error handled; the substitution methods' status, settled by PR 3 D7.
- Owner ruling on #350 (comment 6710782733, Q3): "the projection client only, for now … When it [`processor/rule`] is
  [ported], its write path must validate too". The validator does not exist: `git grep -n -i
  'ValidateCandidateState\|CandidateState' -- ':!openspec/changes/archive'`: 0.
- Consumer use (symbol level): semteams `test/contract/rule_family_consumer_test.go:155`, `:160` and
  `research_loop_budget_test.go:65`, `:70`, `:230-231`, `:473`, `:502` call `rule.NewActionExecutorFull(…, nil,
  publisher, platform)` and `executor.Execute(ctx, action, ec)`; `:474`, `:503` pass the executor to
  `NewStatefulEvaluator`. semboids: no executor symbol. semsource: no `processor/rule` import. No consumer names
  `ExecuteAddTriple`, `ExecuteRemoveTriple`, `ParseTTL`, `TripleMutator`, `Publisher`, `MutationTimeout`,
  `DenyVerdict` or `ErrDenyVerdict` (`grep -rn` over the three trees).
- semboids reads the publish payload: `internal/sim/steering.go:47-62` decodes `{properties: {boid_id, zone_id, kind,
  ttl_ticks}}`, coercing numbers given as strings ("`$message.*` substitution can render numbers as strings",
  `:49-50`). The pin's ADR-093 names the envelope as a product contract: `entity_id`, `subject`, an RFC 3339
  `timestamp`, `source` exactly `rule_engine`, optional `properties` and `related_id` (pin
  `docs/adr/093-typed-user-response-subject-ownership.md:28-32`).
- **Usage counts** (python over semteams and semboids `configs/**/*.json`, 92 rule objects; semsource has no rules):
  action types `add_triple` 57, `publish_agent` 42, `replace_owned` 9, `lifecycle_transition` 9,
  `reconcile_predicates` 6, `publish` 6, `remove_triple` 2, `update_triple` 0. Keys: `add_triple` uses `subject` 48,
  `predicate` 57, `object` 57, `ttl` 0; `remove_triple` `predicate` 2, `object` 0, `subject` 0; `publish` `subject` 6
  (all the literal `boids.steering`), `properties` 6. Templates in triple fields: 42 `$entity.triple.*` tokens, of
  which 9 in objects; 4 of the 9 have a required condition on the same predicate, 5 do not (all
  `coordinator.decision.reason`: `create-change/01-…`, `coordinator/03-ask-user.json`, `03b-respond-direct.json`,
  `dev-from-task/01a-…`, `01b-…`). semboids' 6 publish actions carry 12 `$message.*` property tokens (`boid_id`,
  `zone_id`), none checked by a condition on the same field. The two `remove_triple` actions
  (`agent-run/11-resume-awaiting-to-executing.json:34`, `:38`) clear `agent.run.clarification-pending` and
  `…-resumed`; their only writers are semteams rules (`agent-run/07`, `08`, `10`, `add_triple`;
  `grep -rn` over semteams' Go and JSON outside tests and docs finds only the vocabulary declarations
  `cmd/semteams/vocab/vocab.go:172`, `:177`).

## 6. Category 4: who each name is for

| Name | Pin production readers | Consumer readers | Disposition basis |
| --- | --- | --- | --- |
| `ActionExecutor`, `Execute` | `processor.go:665-711`, `stateful_evaluator.go:426`, `cron_scheduler.go:609` | semteams contract tests (§ 5) | target-shape § 2: "the executor … may be unexported"; the design decides |
| `NewActionExecutor`, `…WithMutator`, `…Full`, `…Complete` | `processor.go:670-678` (two) | semteams tests (`Full`) | ruling item 6: one constructor |
| `ExecuteAddTriple`, `ExecuteRemoveTriple` | `actions.go:919`, `:922` only | 0 | surface audit (a): no outside reader |
| `ParseTTL` | `actions.go:1026`, `:1271` (and `:1359`, PR 5's reconcile) | 0 | goes if `ttl` goes (F8) |
| `TripleMutator`, `Publisher` | the executor's fields; one production implementer each (`triple_mutator.go:17`, `publisher.go:29`) | 0 | the design decides what replaces them |
| `MutationTimeout` | `triple_mutator.go:28`, `:31` | 0 | surface audit (a) |
| `DenyVerdict`, `ErrDenyVerdict`, `(*DenyVerdict).Is` | producer `actions.go:2111` (deny, not carried); readers `stateful_evaluator.go:427`, `cron_scheduler.go:610` | 0 | ported; contract: #28's matrix row ("a `*DenyVerdict` stops the chain"); a product's deny handler is its producer |
| `newTripleMutator`, `tripleMutator` (unexported) | `processor.go:670` | — | plumbing; PR 8 builds it |
| New: `Run` on `ActionHandler` | — | semteams' `publish_agent`, moving to semteams (pin `actions.go:1683-2090`) | framework API; target-shape § 2 |
| New: what a handler is handed to publish and write triples | — | semteams' `publish_agent`: publishes (pin `actions.go:2024-2036`) and writes triples (`:737-749`, `:2062-2073`) | framework API; the design names its contract line |
| New: the handler set's settings (graph writes on or off) | — | a product checking its packs with `Load` (PR 2 D1) | framework API; plumbing for PR 8 |

## 7. Category 5: problem shapes and nearest instances

- **Dispatch by type to a registered handler:** `load.go:244` — `handler, ok := handlers.byType[decoded.Type]`. Adopt:
  the executor looks up the same map.
- **A writer over the mutation lane, both constructor errors handled:** `pkg/projection/mutation_client.go:56-63`
  (`graphmutation.NewClient`, then `graph.NewExactEntityReader`, each `if err != nil { return nil, err }`). Adopt.
- **Interpreting an append result:** `pkg/projection/mutation_client.go:219-228`, `:518-527`. A second interpreter
  would be the pin's text form; the design consolidates (adoption sweep below).
- **Refusing an unfilled token:** `execution_context.go:300-319`. Adopt for every templated action field.
- **A dependency handed to plug-in code at run time:** `execution_context.go:48` (`Lifecycle`, a field the caller
  fills); `component.Dependencies` for components at construction. Considered in the design (D2).
- **A guard applied at a shared output:** none for subjects at fire on main; the load half is
  `builtin.go:120-123`. The design puts the fire half at the one output every handler publishes through.
- **An import guard over `go list -deps`:** `internal/harness/contract/servicedeps_test.go:16-19`, `:77-103`. Adopt:
  one helper over packages, extended to `./processor/rule/...` and `governance`.
- **A typed error with a payload, read through wrapping:** `pkg/errs` `ClassifiedError` with `errors.As`;
  `processor/rule/state_tracker.go:63` — `ErrStateNotFound` with `errors.Is`. `DenyVerdict` is both forms.

Adoption sweep (the append-result interpreter, if moved to `internal/graphmutation`): `pkg/projection/
mutation_client.go:219-228` and the rule writer. The pin's other hand-written interpreters
(`processor/gated-dag/claim.go`, `processor/agentic-tools/decide.go`) are not ported. Two sites; no tracking issue
needed beyond this PR, which adopts both.

## 8. Category 6: the pin's decision records for the carried files

Searched: ADR references in the carried ranges (`grep -o -E 'ADR-[0-9]{3}'`: ADR-046 2, ADR-047 2, ADR-053 3, ADR-055
4, ADR-102 4), then every pin ADR for `add_triple|remove_triple|update_triple|TripleMutator|triple_mutator|
DenyVerdict|executePublish|ActionExecutor|rule_engine|publish action` (039, 040, 046, 049, 053, 054, 055, 056, 059,
060, 093), then records whose titles name graph writes.

- **ADR-091, graph mutation authority without semantic ownership** (pin; superseded for the seam by SemEngine's
  ADR-110): graph-ingest is the one physical writer; any component may request one of four typed operations; "Rule
  reconcile performs one exact read and one mutation request. It surfaces `revision_mismatch` and `commit_unknown`
  without automatic retry" (pin `091-…md:96-97`). Governs `triple_mutator.go`.
- **ADR-110, the write seam** (`docs/adr/110-write-seam.md`): Decision 1, everyone but graph-ingest changes entity
  state through the four typed operations; Decision 4, append is must-exist and keyed on (subject, predicate,
  datatype, source, context, object), a conditional replace names one source; Decision 5, every statement carries a
  `Source` and a `Timestamp`, an originator may assert its own time. Governs every triple write.
- **ADR-055, graph write intent taxonomy** (pin; mutation-lane taxonomy superseded by ADR-091): must-exist append;
  "the `add_triple` rule action supports `action.Subject` override … targeting an entity OTHER than the trigger"
  (`055-…md:225-226`).
- **ADR-056, authoritative semantic state** (pin; superseded in full by ADR-091): rule triple writes cannot declare
  ownership (`:54-56`). No decision left to port.
- **ADR-074, canonical predicate contract** (`docs/adr/074-…md`): a rule triple predicate is a declared literal; kept
  at load by PR 2 (`builtin.go:80-87`).
- **ADR-093, typed user-response subject ownership** (pin, not on main): publish may not target `user.response.>`,
  declared or after substitution; the publish envelope is a raw product contract (`093-…md:20-32`). PR 2 carried the
  load half as a spec requirement and did not port the record.
- **ADR-102 and ADR-104** (ported): the deployment mints under its own authority; graph-ingest refuses a foreign
  subject on the mutation lane (`processor/graph-ingest/canonical_mutations.go:326-331`). The executor's `platform`
  field (`actions.go:556`) serves only `publish_agent`'s foreign-firing guard.
- **ADR-111, durable execution primitive** (`docs/adr/111-…md`): Decision 3, a blocker is an `errs.ClassifiedError`
  and a `commit unknown` is never retried; Decision 4, effect then guard then acknowledgement.
- **ADR-060, unified RPC error contract** (ported): mutation refusals arrive classified.
- **ADR-046** (fan-out; cited for the subject override as its "Phase 1 join gap", `actions.go:949-952`), **ADR-047**,
  **ADR-053** (lifecycle and agent runs, read by `publish_agent`), **ADR-039** and **ADR-055 § 3a** (deny/approve,
  `VerdictAuditor`): not carried (Q15; #27, #28).
- **ADR-076**: the graph-event identity; decisions 1-3 and 5 not ported, ruled.

## 9. Open issues against `processor/rule`

Queries: `gh issue list --state open --search "<q>" --limit 200` for `rule`, `action`, `actions`, `triple`, `publish`,
`deny`, `DenyVerdict`, `executor`, `mutator`, `ActionExecutor`, `add_triple`, `remove_triple`, `update_triple`, `ttl`,
`ExpiresAt`, `graphmutation`, `ExactEntityReader`, `NewExactEntityReader`, `user.response`, `governance`, `agentic`.
Kept, as naming a carried package or file:

- #27 (`publish_agent` leaves the core; import guard; an `adapt` ledger row whose contract cites the matrix row).
- #25 (action-family registry; retires at PR 7).
- #28 (deny/approve and `VerdictAuditor` leave the core; "`deny.go` stays so a `*DenyVerdict` still stops the chain").
- #350 (value constraints; ruling Q3 names the rule write path).
- #702 (firing cap; PR 7), #294 (write-guard pair; PR 5), #703 (deadline timer; PR 14), #714 (skill; PR 13).
- #704 (closed-loop control): names "Mutations and publish actions" and parks "Cause and hop count on writes … an
  optional field on mutation requests" until a consumer needs provenance.
- #24 (durable execution epic: effect fences).
- #9 (epic), #195 (adoption: `natsfixture.Open`), #196 (adoption: `internal/lifecycleguard`).

Name the word, not the package: #756, #757, #778 (rule expression and `syntax.md`, no PR 4 file), #730, #141,
  #712, #110, #109, #635, #108, #780, #779, #155, #554, #614, #84, #284, #535, #734, #134, #665, #776, #60, #484,
  #300, #775, #683, #576, #765, #111, #625, #83, #750, #659, #641.

## 10. Shape sweep (pin copy)

Each finding ends with its disposition: required, with the binding basis, or optional, with why changing it is
optional ([Bounded port scope](../../../.agents/protocol.md#bounded-port-scope)).

- **F1. Four constructors.** `actions.go:803`, `:820`, `:843`, `:894`. Shape 1. Required (ruling item 6).
- **F2. A closed switch.** `actions.go:916-945` — `switch action.Type {` … `default: return fmt.Errorf("unknown action
  type: %s", action.Type)`. Shape 1 (beside the handler set). Required (ruling item 1; target-shape § 2).
- **F3. A triple or publish action with nothing to write through succeeds silently.** `actions.go:1060-1075` — `if
  e.tripleMutator != nil {` … `} else if e.logger != nil { e.logger.Debug("Triple not persisted (no mutator
  configured)"`; also `:1104-1119`, `:1185-1213`, `:1295-1309`; probe P1. Required (fail closed; target-shape § 2).
- **F4. Constructor errors dropped.** `triple_mutator.go:28` — `mutations, _ := graphmutation.NewClient(client,
  MutationTimeout)`; `:31` — `reader: graph.NewExactEntityReader(client, MutationTimeout),` (one result at the pin,
  two on main). Required (build for `:31`, split plan; fail closed for `:28`).
- **F5. A classified append failure becomes text.** `triple_mutator.go:50` — `return 0, fmt.Errorf("append triple for %s
  failed: %s/%s", triple.Subject, result.Error.Class, result.Error.Code)`; `:61` — `fmt.Errorf("entity not found: %s",
  triple.Subject)` (probe P9). Required (ADR-111 Decision 3; ADR-060).
- **F6. A reconcile without a source.** `triple_mutator.go:80-83` — `graph.ReconcilePredicatesRequest{ EntityID:
  entityID, ExpectedRevision: exact.KVRevision, Predicates: []string{predicate}, Desired: nil,`; main refuses it as
  `invalid_request` (`graph/mutation_requests.go:25-27`). Required (wire contract); what it removes changes (probe P8),
  put to the owner.
- **F7. `update_triple` swallows a failed remove and writes twice.** `actions.go:1256-1266` — `// Log but continue -
  triple may not exist, which is fine for update` … `e.logger.Debug("No existing triple to remove (or error)"`; probe
  P10. Required (fail closed: a degrade is declared); the shape is put to the owner.
- **F8. `ttl` stores an expiry nothing enforces.** `actions.go:1025-1036`, `:1270-1280`; main
  `message/triple_identity.go:31-33`; pin `docs/actions.md:90` — "Triple will be automatically removed after 5
  minutes." Surface audit (c). Required (drop or document); put to the owner.
- **F9. `update_triple`'s `properties` is only logged.** `actions.go:1247-1252` — `"properties", action.Properties)`;
  probe P4. Surface audit (b). Required.
- **F10. `remove_triple`'s `object` is filled and only logged.** `actions.go:1093` — `object :=
  ec.SubstituteVariables(ctx, action.Object)`, read only by the Debug line at `:1096-1100`; probe P3. Surface audit (b).
  Required.
- **F11. The predicate is filled at fire though load refuses `$` in it.** `actions.go:1017` — `predicate :=
  ec.SubstituteVariables(ctx, action.Predicate)`, `:1092`, `:1239`; main `builtin.go:81-84`. Shape 1 (dead path).
  Optional (no behaviour depends on it); dropping it removes the suggestion that a predicate can be templated.
- **F12. A regex main does not have.** `actions.go:988` — `unresolvedTemplateVarRe.FindAllString(resolved, -1)`.
  Required (build; PR 3 D2 removed the regexes).
- **F13. An unfilled object, property or publish subject goes out as text.** Probes P5, P6. Required (fail closed); the
  shape is put to the owner.
- **F14. The publish payload names a field that is gone.** `actions.go:1180-1182` — `if ec.RelatedID != "" {
  payload["related_id"] = ec.RelatedID }` (PR 3 D4 dropped `RelatedID`). Required (build). Building the envelope as a
  map rather than a struct: optional.
- **F15. Guide claims no code implements.** pin `docs/actions.md:125` ("an atomic remove+add operation"; probe P10),
  `:197-201` (payload "includes … Rule ID … Entity triples snapshot"; probe P7), `:208` (`${entity.type}`, a syntax
  that does not exist), `:77-92` (TTL removal). Surface audit (c). Required when the guide is ported.
- **F16. Exported with no reader outside the package:** `ExecuteAddTriple`, `ExecuteRemoveTriple`, `ParseTTL`,
  `MutationTimeout` (§ 6). Shape 4. Required (surface audit (a)).
- **F17. Interfaces with one production implementation:** `TripleMutator` (`actions.go:472`), `Publisher` (`:481`).
  Shape 3. Optional on their own (they are the executor's test seams); the design decides what replaces them.
- **F18. `DenyVerdict` has two ways to ask "is this a deny".** `deny.go:38-48` (`ErrDenyVerdict` and an `Is` that
  matches any `*DenyVerdict`) beside `errors.As`. Shape 1. Optional (both have pin readers; 48 lines).
- **F19. Port-history comments:** 14 lines in the carried ranges (`grep -c -E '#[0-9]+|gh#|ADR-|PR #|Review H'`).
  Required (AGENTS.md comments rule, #606).
- **F20. A nil logger defaults to the global one** in each constructor (§ 2). Optional (not a context); the design
  carries no logger in the executor.
- Classification by error text: none in the carried ranges (`grep -n -E 'strings\.Contains\(.*[Ee]rr|err\.Error\(\)'`:
  0). Copied shell: none. `NewGenericJSON` in carried code: none.

## 11. Same-class collision table (the triple writer)

| Dimension | Evidence |
| --- | --- |
| Semantic class | Changing an `ENTITY_STATES` entity, on behalf of code outside graph-ingest, through graph-ingest's typed mutation operations |
| Owners | `internal/graphmutation.Client` (the one wire adapter, `client.go:66-82`); `pkg/projection.MutationClient` (`mutation_client.go:28-68`, contract-bound, the caller names the source); `pkg/lifecycle` (`graph_emit.go:31`, `manager.go:104`, source `graph.SourceLifecycle`); the pin's rule `tripleMutator` (source `rule_engine`). Not ported: pin `processor/gated-dag/claim.go:14-31`, `processor/agentic-tools/decide.go:78-79` |
| Catalogs | `graph/kvcatalog` row for `ENTITY_STATES` (`WriteOwnerOnly`); mutation subjects only in `internal/graphmutation/protocol.go` (`graph-transport-boundary` spec) |
| Status | graph-ingest counts each mutation outcome (`canonical_mutations.go:381-387`); the rule processor's metrics are PR 8's |
| Lifecycle | none: each call is one request; the writer is built once by the processor (PR 8) |
| Ownership | one graph-ingest process (ADR-091 § 2); the revision is the only fence (ADR-110 Decision 3); a conditional replace names one source (Decision 4) |
| Readers | graph-ingest's mutation handlers; every `ENTITY_STATES` watcher, the rule's own from PR 10 |
| Writers | graph-ingest only, physically (`graph-entity-writes`, "One rule per write mode") |
| Recovery | none in the writer: `commit_unknown` and `revision_mismatch` are returned, never retried (ADR-091 consequences; ADR-111 Decision 3) |

## 12. Adopter seam inventory

Two developers: semteams' author moving `publish_agent` into a product handler, and a rule author.

1. **What they must know.** The handler author: implement `Check` and `Run`; publish and write triples through what
   the framework hands `Run`, not a connection of their own, to get the reserved-subject refusal, the processor's
   port routing (JetStream or core NATS, PR 8) and, from PR 10, the rule's own-write skip; treat the context's fields
   as read-only (PR 3 D7). Four facts: a design finding unless the default path makes them true without knowing.
   The rule author: a remove now removes only what rules wrote (if the owner rules so); `ttl`, update's `properties`
   and remove's `object` are refused (if ruled); an action with an unfillable name fails instead of writing the text
   (if ruled).
2. **If they do nothing.** semteams' five test sites stop compiling (`NewActionExecutorFull`, `Execute`): a compile
   error. A handler that publishes over its own connection compiles and runs, and silently has no reserved-subject
  refusal. A rule
   with a refused key fails `Load` naming key, rule and file (boot error, PR 8). A rule whose object token does not
   resolve fails that action with an error naming the field and the token.
3. **Where they find out.** Compile error (executor names gone; `Run` added to the interface); boot error (refused
   keys); typed runtime error (unfilled names, reserved subject, entity not found); doc only for "use what you are
   handed".
4. **What they should have to know.** Nothing beyond `Check` and `Run`. The gap: a product can always reach NATS some
   other way, so whatever the framework hands a handler has to be its easiest path, not a fact it must learn.

Prefer observation: the source name and the statement time are the writer's, never the caller's to supply; the
rule ID a write is tracked under (PR 10) comes from the rule the executor runs, not from the context's state, which a
cron context lacks (PR 3 inventory § 13).

## 13. Intent check

No boundary moves: the port set is ruled (#8 Q15, D12); `publish_agent` and deny/approve were already out of the
core. Dropping `ttl` removes a key no code honours, not a capability (§ 4, F8).

## 14. What PR 4 sends forward

- **PR 5:** the five built-in types whose `Run` arrives there (`reconcile_predicates`, `update_kv`, `lifecycle_*`);
  `reconcile_predicates`' own `ttl` key (`builtin.go:29`) gets the same answer as D6.
- **PR 7:** `Fire` calls the executor with the rule's ID; what a failed action means (ruling item 7); the stop on a
  `*DenyVerdict` (#28); `action_failure_metrics_test.go`; whether the triple time takes `Fire`'s `now`.
- **PR 8:** builds the publisher (`publisher.go:26-77`, port routing) and the triple writer from its NATS client and
  `enable_graph_integration`; the executor once in `Start`; `events_published` for publish actions.
- **PR 9:** `publishRuleEvent` (`publisher.go:175-231`); the message path's entity ID, which a triple action with no
  subject writes to.
- **PR 10:** the own-write skip: the writer reports each applied revision with the rule ID;
  `triple_mutator_revision_integration_test.go` (a suppressed add or a no-op remove claims no revision).
- **PR 12:** a live change to `enable_graph_integration` (pin `runtime_config.go:72-76`) cannot rebuild the
  executor; refuse it or rebuild.
