# Design: rule-core-04-graph-actions

Pin: SemStreams `8b99efe9`, `processor/rule/`. Inventory `inventory.md` (base a362eabb), probe
`notes/inventory-probe.md`. Givens: target-shape § 2 and its approved proposal; PR 2 D2, D3, D6; PR 3 D7 (owner ruling
PR #759 comment 7170655515: `Lifecycle` rides on the context); the graph-event drop (#9 comment 7160955176); PR #752
ruling items 1, 6, 9; ADR-110. Spec delta: `rule-actions` (new); `rule-definitions` (two requirements, D6, D9).

PR 4 makes an action run: the executor, `Run` on the handler, the built-ins `publish`, `add_triple`, `remove_triple`,
`update_triple`, and `DenyVerdict`; no lifecycle owner. Size (inventory § 1): 716 production pin lines carried, 280
not; tests 1,271 carried, 397 moved, 1,109 dropped. Measured at the pin: nothing to write through succeeds silently
(P1); a remove clears every writer's statements (P8), which main refuses (a replace names one source,
`graph/mutation_requests.go:22-36`); an update writes twice, ignoring a failed remove (P10); `ttl`, update's
`properties`, remove's `object` do nothing (P3, P4); an unfilled name goes out as text (P5, P6).

## Decisions

**D1. Exported surface.** Framework API (contract line: the `rule-actions` spec): `Run` on `ActionHandler`, `Outputs`
(D2), `HandlerConfig` and `NewHandlers(product, cfg)` (D9), `DenyVerdict`, `ErrDenyVerdict` (D10). Unexported: the
executor (D3), the writer, the framework's outputs, the envelope. `internal/graphmutation` gains `AppendResultError`
from `pkg/projection` (D4). Not carried: F1, F2, F16, F17 (constructors, switch, `ExecuteAddTriple`,
`ExecuteRemoveTriple`, `ParseTTL`, `TripleMutator`, `Publisher`, `MutationTimeout`), `platform`, logger, setters (#27),
the predicate fill (F11). Moved: publisher (PR 8), `publishRuleEvent` (PR 9), own-write skip (PR 10).

**D2. What a handler is handed (owner decision; Fable).** Facts (inventory § 12): the approved page hands every handler
a publisher refusing the reserved space; semteams' `publish_agent` uses only the executor's publish and triple add (pin
`actions.go:737-749`, `:2024-2036`, `:2062-2073`); `NewHandlers` builds the built-ins with no dependencies
(`action.go:203-205`), so what reaches a handler at run is also the built-ins' route to the writer, and PR 5's to its
reconciler and KV writer; `Lifecycle` stays on the context under every option. (a, recommended) a fourth argument,
`Run(ctx context.Context, a Action, ec *ExecutionContext, out Outputs) error`; `Outputs` holds the two calls a product
makes, `Publish(ctx context.Context, subject string, data []byte) error` and `AddTriple(ctx context.Context, subject,
predicate string, object any) error`. `run` builds the framework's value per call, bound to the rule ID, so PR 10's skip
and `Fire`'s `now` (if PR 7 wants it) reach the writer with no change to `Outputs` and no context value. Remove and
replace, with no product reader, are unexported methods of that value; the built-ins (and PR 5's reconcile and KV
write) reach them by asserting its type, refusing any other `Outputs`, classified invalid. `Outputs` widens only for a
named product need, and every stand-in then adds the method. The page's three-argument sketch changes (archived page
not edited, as PR 3 D4). (b) fields on `ExecutionContext` that `run` fills per call: one route, but `run` writes the
caller's context, a field the caller set and one `run` sets conflict, and the rule ID is one more field. (c)
nothing: no guard, routing or skip; products write through the public `pkg/projection.MutationClient` under any source.
The writer stamps `rule_engine`, time and confidence 1 (the lane accepts any source, ADR-110 Decision 5).

**D3. The executor, once, unexported (owner decision).** `newActionExecutor(h Handlers, deps) (*actionExecutor,
error)`, no setters (item 6); `deps`: PR 8's publisher, the writer (absent with writes off). It refuses, classified
invalid, a set `NewHandlers` did not build, no publisher, a set and writer that disagree. `run(ctx, ruleID, a, ec)`
refuses nil arguments and a type the set lacks, naming type and rule; calls `Run` with the outputs; wraps its error with
`%w` naming rule and type (`errors.As` finds a `*DenyVerdict`; a class stays). The rule ID is the caller's
(`ec.RuleID()` is empty on cron). Runs may be concurrent. (a, recommended) unexported: semteams' four test files that
run actions in-process (inventory § 5; two via `NewActionExecutorFull`, two running `lifecycle_transition`,
`add_triple`, `remove_triple` over their own fakes) move to PR 8's processor on NATS; a product handler's test calls
`Run` with a stand-in. This replaces the approved consequence "contract tests that build executors with no writer need
a writer or must use no triple action" (target-shape proposal `:44`). Why: exporting it exports the publisher and
writer interfaces too, two more surfaces PRs 7-10 keep stable, with PR 10's skip behind a replaceable writer. (b)
exported, one constructor taking set, publisher, writer: semteams renames. Proof: `TestRunDispatch`,
`TestNewActionExecutorRefuses`, `TestRunConcurrent` (`-race`).

**D4. Graph writes and the write budget.** Through `internal/graphmutation.Client` and `graph.ExactEntityReader`
(ADR-110 Decision 1), built as `pkg/projection/mutation_client.go:56-63`, both errors returned (F4),
`natsclient.DefaultRequestTimeout`. First `ValidateEntityID` and `RequireDeclaredPredicate`, else invalid, nothing sent.

- Add: one append; applied or unchanged succeeds; failures via `graphmutation.AppendResultError`, shared with
  `pkg/projection` (`:219-228`, `:518-527`): missing entity is invalid `entity_not_found` (P9); a failed subject keeps
  its reply's class and code (F5).
- Remove (owner decision): exact read, conditional replace under `rule_engine` desiring nothing. (a, ruled) clears
  rules' statements only; only another source's statements fail, invalid, naming predicate and source, sending
  nothing (owner, PR #782). (b) one replace per source: not atomic; could erase a lifecycle phase.
- Update (owner decision): exact read, conditional replace leaving the one new rule statement. (a, recommended) one
  write; another source's statement stays, and a single-value read (`GetTriple`, `graph/types.go:44-55`) picks the
  latest timestamp (the pin cleared every source). (b) the pin's remove then append (two writes, a swallowed failure,
  P10). (c) drop it: 0 loaded uses, but semteams' parked `replace_owned` (9 actions, no handler) is this replace and
  maps onto (a), where a product handler would need the private replace.
- An absent entity fails remove and update with the read's `entity_not_found` (`processor/graph-ingest/query.go:87-96`);
  a reply of unchanged (same value) succeeds.
- Retry: a mismatch commits nothing, so remove and update re-read and retry, at most five attempts, per ADR-110
  Decision 2 (`docs/adr/110-write-seam.md:84-85`) and as `pkg/lifecycle/manager.go:505-509`; the last mismatch returns
  (`errs.ErrRevisionMismatch`). Commit-unknown returns at once (ADR-111 Decision 3). Trigger: semteams' `agent-run/11`
  clears `clarification-resumed` on a run entity others write; failing it, "rule 09 stays permanently blocked", and a
  failed action does not rerun (item 7).
- Budget: a firing writes nothing of its own; each triple action commits at most one write (remove, update: at most
  five read-and-replace pairs, one can commit); publish writes no graph; requests bounded by timeout and caller's ctx.

Proof: `TestTripleWriter` (integration, real graph-ingest, as `pkg/lifecycle/harness_gate_integration_test.go:74-104`),
`TestTripleWriterRefusesBeforeSending`, `TestTripleWriterRetries` (stand-in requester).

**D5. Triple fields.** No or empty `subject` is the firing entity, else filled (D7); neither is refused. `predicate` is
the literal `Load` checked (F11). A whole-token `object` keeps its type (PR 3). Proof: `TestTripleActionFields`.

**D6. Settings nothing acts on are refused at load (owner decision).** `ttl` on add and update (nothing expires a
triple, `message/triple_identity.go:31-33`), update's `properties`, remove's `object` (F8-F10; 0 uses) leave
`builtin.go:24-34`. (a, recommended) refuse; (b) keep `ttl`, documented as never enforced. PR 5 likewise.

**D7. An unfilled name stops the action (owner decision).** A built-in fills each template it reads with the one scan
(`execution_context.go:300-319`); an unfilled token fails, classified invalid, naming field and tokens, nothing sent or
logged. (a, recommended) everywhere; (b) only a triple subject (pin). semteams 5 objects, semboids 12 properties.

**D8. Publish.** `properties`: shallow copy, top-level strings filled as text (semboids coerces,
`internal/sim/steering.go:49-50`). Envelope struct: `entity_id`, `subject`, `timestamp` (RFC 3339 nanoseconds, UTC; pin
local offset, P7), `source` `rule_engine`, `properties` (`null` when absent); no `related_id` (F14). `out.Publish`
refuses `user.response.>` (`builtin.go:120-123`) for every handler. Proof: `TestPublishAction`, `TestOutputsPublish`.

**D9. Settings.** `NewHandlers(product, cfg HandlerConfig)`; `cfg.TripleWrites` false makes the triple types' `Check`
refuse ("the rule processor writes no triples"); zero is off (pin `config.go:249`). A product handler's `out.AddTriple`
with writes off fails at fire, classified invalid, same words; the handler decides (pin skipped silently,
`actions.go:2061`). Proof: `TestBuiltinCheck`, `TestOutputsAddTripleWritesOff`.

**D10. `DenyVerdict`, import guard.** `deny.go` as is (F18); `run` passes it, PR 7 stops on it (#28). #27: the helper
at `internal/harness/contract/servicedeps_test.go:77-103` takes a package and forbidden elements;
`TestRuleReachesNoAgenticOrGovernancePackage` covers `./processor/rule/...`, with a sensitivity test. No generated
checks (`notes/2.1.md`). Docs: `actions.md` corrected (F15), `syntax.md`, `doc.go`, the repository map.

## ADRs

Kept: ADR-110 Decisions 1, 2, 4, 5; pin ADR-091 §§ 1-2; ADR-055 (must-exist append, subject override); ADR-074;
ADR-093's fire half; ADR-102/104; ADR-111 Decision 3; ADR-060. Not binding: 040, 049, 054, 056, 059 (inventory § 8).
Changed, for the owner: a remove clears one source; an update is one write; ADR-091's no-retry yields to ADR-110
Decision 2 on a mismatch; ADR-093's optional `related_id` leaves. Dropped, ruled: 046, 047, 053, 039, 055 § 3a; 076.

## Issues, merge order, risks

`Closes #27`. Named, no closing keyword: #25, #28, #350 (proposal decision), #702, #294, #704, #24. Stay, no PR 4
work: #195, #196, #703, #714, #9. No open PR overlaps; PR 5 waits; merge `origin/main` first if it moved. Risks:
`NewHandlers` gains an argument (12 callers here); five failed attempts on a busy entity fail the action.
