# Design: rule-core-04-graph-actions

Pin: SemStreams `8b99efe9`, `processor/rule/`. Inventory: `inventory.md` (base a362eabb; awaiting INVENTORY PASS),
probe `notes/inventory-probe.md`. Givens, not reopened: the target-shape page § 2 and its approved proposal; PR 2's
handler set (D2, D3, D6); PR 3's handler contract (D7, owner ruling PR #759 comment 7170655515); the graph-event drop
(#9 comment 7160955176); PR #752 ruling items 1, 6, 9. Spec delta: `rule-actions` (new); `rule-definitions` (two
modified requirements, D6, D9).

## Context

PR 4 makes an action run: the executor, `Run` on the handler, the four built-ins `publish`, `add_triple`,
`remove_triple`, `update_triple`, and `DenyVerdict`. No lifecycle owner. Size: 716 production pin lines carried and
rewritten, 280 not carried here; 2,777 test lines, 1,271 carried, 397 moved to PRs 7 and 10, 1,109 dropped (inventory
§ 1). Measured premises: at the pin a triple or publish action with nothing to write through succeeds silently (P1);
`remove_triple` clears a predicate from every writer (P8), which main's write rules no longer allow (a conditional
replace names one source, `graph/mutation_requests.go:22-36`); `update_triple` writes twice and ignores a failed remove
(P10, F7); `ttl`, update's `properties` and remove's `object` do nothing (P3, P4, F8-F10), and no product rule sets
them (inventory § 5); an unfilled name in an object, property or publish subject goes out as text (P5, P6).

## Decisions

**D1. What PR 4 carries and exports.** Framework API (contract line: the `rule-actions` spec): `Run` on
`ActionHandler`; `Outputs` (D2); `HandlerConfig` and `NewHandlers(product, cfg)` (D9); `DenyVerdict` and
`ErrDenyVerdict` (D10). Unexported: the executor, its constructor and `run` (D3), the triple writer, the framework's
`Outputs`, the publish envelope. In `internal/graphmutation`: `AppendResultError`, moved from `pkg/projection` (D4).
Not carried: four constructors and the closed switch (F1, F2), `ExecuteAddTriple`, `ExecuteRemoveTriple`, `ParseTTL`,
`TripleMutator`, `Publisher`, `MutationTimeout` (F16, F17), the executor's `platform`, logger and setters (with
`publish_agent`, #27), the predicate fill (F11), the Debug lines. Moved: the port-aware publisher to PR 8,
`publishRuleEvent` to PR 9, the own-write skip to PR 10. Proof: `go doc ./processor/rule` lists no name without a
reader or a spec line.

**D2. What a handler is handed to change the world (owner decision; Fable).** The approved page hands every handler a
publisher that refuses the reserved space (target-shape proposal `:45-52`), and a product's `publish_agent` publishes
and writes triples (pin `actions.go:2024-2036`, `:737-749`, `:2062-2073`). Options: (a, recommended) a fourth argument,
`Run(ctx, a Action, ec *ExecutionContext, out Outputs) error`, where `Outputs` is one interface: `Publish(ctx, subject,
data)`, `AddTriple`, `RemoveTriple`, `ReplaceTriple(ctx, subject, predicate[, object])`. The executor passes the one
framework implementation to every handler, built-in or product; a product test passes a stand-in. It changes the page's
three-argument sketch; the archived page is not edited, this design and the proposal are the record (as PR 3 D4). (b)
Two fields on `ExecutionContext` beside `Lifecycle` (`execution_context.go:48`): every path that builds a context (pin
`stateful_evaluator.go:210`, `:289`, `:550`) must fill both, and a forgotten one is a nil at fire. (c) Nothing handed:
no guard, no port routing or own-write skip for products, which cannot reach `internal/graphmutation`. Triple methods
take subject, predicate and object, never a whole statement: the writer stamps source, time and confidence, so a handler
cannot write under a reserved source, which the mutation lane accepts (ADR-110 Decision 5).

**D3. The executor, once, unexported (owner decision).** `newActionExecutor(h Handlers, deps) (*actionExecutor,
error)`: one constructor, dependencies at construction, no setters (ruling item 6). `deps` holds the publisher (PR 8's)
and the triple writer, absent when graph writes are off. It refuses, classified invalid, a set `NewHandlers` did not
build, no publisher, and a set and writer that disagree (writes on and no writer, or the reverse). `run(ctx, ruleID, a,
ec) error` refuses a nil context or execution context, looks up `a.Type` in the set (a miss, which only a set mismatch
reaches since `Load` refuses it first, is classified invalid naming type and rule), calls `Run(ctx, a, ec, out)` and
wraps its error with `%w` naming rule and type, so `errors.As` still finds a `*DenyVerdict` and an
`errs.ClassifiedError` keeps its class. The rule ID is the caller's (PR 7's `Fire` holds the rule), not `ec.RuleID()`,
which is empty on cron (PR 3 inventory § 13). It holds nothing a run changes; runs may be concurrent. Options:
(a, recommended) unexported, as the page allows; semteams' 5 test sites that build one (inventory § 5) call their
handler's `Run` with a stand-in, or drive the processor from PR 8. (b) exported: a second way to run an action beside
the processor, and an API PR 7 must keep stable. Proof: `TestRunDispatch`, `TestNewActionExecutorRefuses`,
`TestRunConcurrent` (`-race`).

**D4. Graph writes and the write budget.** Every triple write goes through graph-ingest's typed operations
(`internal/graphmutation.Client`, `graph.ExactEntityReader`; ADR-110 Decision 1), built as `pkg/projection` builds them,
both constructor errors returned (F4; `pkg/projection/mutation_client.go:56-63`), with
`natsclient.DefaultRequestTimeout` per request as `pkg/projection` uses when given none. Before any request the subject
passes `ValidateEntityID` and the predicate `RequireDeclaredPredicate`, else classified invalid. Each statement: source
`rule_engine`, time from the clock (an originator may assert its own, ADR-110 Decision 5), confidence 1. Add: one
append; applied or unchanged is success; a missing entity is classified `entity_not_found` (must-exist, ADR-110 Decision
4; P9); a failed subject keeps its reply's class and code (F5) through one interpreter,
`graphmutation.AppendResultError`, which `pkg/projection` also calls (`:219-228`, `:518-527`; adoption sweep, two
sites). Remove: one exact read, one conditional replace under `rule_engine` with nothing desired (owner decision): (a,
recommended) only what rules wrote, since a replace names one source; semteams' two removes clear predicates only its
rules write (inventory § 5); (b) every source: one write per source found, not atomic, and a rule could erase a
lifecycle phase or another component's fact. Update: one exact read, one conditional replace with the one statement
(owner decision): (a, recommended) one write; (b) the pin's remove then append, two writes and a swallowed failure
(P10); (c) drop the type (0 uses, ruling item 9), though it is the replace the reviewer contract asks of a single-valued
fact. A revision mismatch or commit unknown is returned with its class, never retried (pin ADR-091 `:96-97`; ADR-111
Decision 3). **Budget:** a firing writes nothing of its own (graph event dropped); each triple action makes at most one
write (add one request; remove and update one read and one request); a publish writes nothing to the graph. A firing
writes at most once per triple action it runs; per rule and entity, at most the firing cap times that (PR 7). Each
request is bounded by the request timeout and the caller's context. PR 10 adds the own-write skip inside the writer,
keyed by the rule ID `run` passes, without changing `Outputs`. Proof: `TestTripleWriter` (integration, real graph-ingest
on the package's shared NATS server, as `pkg/lifecycle/harness_gate_integration_test.go:74-104`);
`TestTripleWriterRefusesBeforeSending`.

**D5. Triple action fields.** `subject` absent or empty is the firing entity, else filled (D7); with neither, the
action is refused, classified invalid. `predicate` is the literal `Load` checked, never filled (F11). `object`: one
whole token keeps its type (`SubstituteVariablesTyped`, PR 3), else filled as text. Proof: `TestTripleActionFields`
with PR 3's five forwarded typed tests.

**D6. Settings nothing acts on are refused at load (owner decision).** `ttl` on add and update (F8: nothing expires a
triple, `message/triple_identity.go:31-33`), `properties` on update (F9), `object` on remove (F10); 0 product uses
each. They leave the built-in key sets (`builtin.go:24-34`), so `Check` refuses each naming key, rule and file.
Options: (a, recommended) refuse; (b) keep `ttl` as a stored expiry documented as never enforced; an author who sets
it believes the value goes away, and re-asserting never extends it (append identity ignores expiry). PR 5 answers
`reconcile_predicates`' `ttl` the same way.

**D7. An unfilled name stops the action (owner decision).** A built-in fills every template it reads (triple subject
and object, publish subject, each top-level string property) with the one scan, as condition values are
(`execution_context.go:300-319`); a token left unfilled is an error, classified invalid, naming the field and the
tokens; nothing is written or sent and the action logs nothing (its caller logs the error once, PR 7). Options:
(a, recommended) everywhere; (b) the pin's: refuse only a triple subject (pin `actions.go:988`), send the rest as text
with a warning (P5, P6). Counts: semteams 5 object templates no condition guarantees; semboids 12 property tokens.
Proof: `TestUnfilledNameStopsAction`.

**D8. Publish.** The subject is filled (D7); `properties` is a shallow copy with top-level strings filled as text, as at
the pin (semboids coerces, `internal/sim/steering.go:49-50`). The envelope is a struct marshalled to `entity_id`,
`subject`, `timestamp` (RFC 3339 with nanoseconds, UTC; the pin used local offset, P7), `source` `rule_engine` and
`properties` (`null` when absent); no `related_id` (F14). It goes out through `out.Publish`, which refuses a subject in
`user.response.>` with the one predicate (`builtin.go:120-123`) before the publisher is called, for every handler:
ADR-093's fire half has one home. No idempotency key: a redelivered firing publishes again; PRs 8 and 9 declare the
lane. Proof: `TestPublishAction` (envelope, fill, refusals), `TestOutputsPublish` (a product handler's reserved
subject refused, nothing published).

**D9. The set's settings.** `NewHandlers(product, cfg HandlerConfig)`; with `cfg.TripleWrites` false, the three triple
types' `Check` refuses, saying the rule processor writes no triples (target-shape § 2; approved proposal `:42-44`).
The zero value is off, as the pin's `enable_graph_integration` default (pin `config.go:249`); a product checking its
packs sets what its processor sets. PR 5 adds its settings as fields; PR 8 fills them from configuration. Proof: two
cases in `TestBuiltinCheck`.

**D10. `DenyVerdict` and the import guard.** `deny.go` is carried as is (`RuleID`, `Reason`, `ErrDenyVerdict` and its
`Is`; F18 kept, both have pin readers). Nothing in the core returns one; a product's handler does, `run` passes it
through (D3), and PR 7 stops the list on it (#28). #27: the service guard's helper (`internal/harness/contract/
servicedeps_test.go:77-103`) becomes one helper taking a package and its forbidden path elements;
`TestRuleReachesNoAgenticOrGovernancePackage` runs it over `./processor/rule/...` with `agentic`, `agentic-*` and
`governance`, with a sensitivity test like the service one. #27's family half: a handler built into the set by
`NewHandlers` checks at load and runs at fire through `run` (`TestRunDispatch`).

Tests and docs: carried tests become decoded JSON under the harness rules; no generated checks, as dispatch and the
four writes are a handful of cases with no law across inputs (recorded in `notes/2.1.md`). `docs/actions.md` comes over
corrected (F15), with `syntax.md`'s key table, `doc.go` and the repository map; F19's history lines keep their reason.

## ADRs

Kept: ADR-110 Decisions 1, 4, 5; pin ADR-091 §§ 1-2 and its no-retry consequence; ADR-055's must-exist append and
subject override; ADR-074 (load, PR 2); ADR-093's fire half (a spec requirement; not ported to `docs/adr`, PR 2's
reason); ADR-102/104 (graph-ingest's authority gate); ADR-111 Decision 3; ADR-060. Changed, for the owner: what a
remove clears (ADR-091's reconcile of every source becomes ADR-110 Decision 4's one source, D4); update's two writes
become one (D4). Dropped, ruled: the executor's platform and setters with `publish_agent` and deny/approve (046, 047,
053, 039, 055 § 3a; Q15, #27, #28); ADR-076 decisions 1-3 and 5.

## Issues, merge order, risks

`Closes #27`. Named without a closing keyword: #25 (dispatch at fire proved here; semboids' pack fires at PR 7), #28
(`DenyVerdict` here; the stop at PR 7), #350 (owner decision: the validator is not built; #350's change adds it at this
writer), #702, #294, #704 (cause and hop count on writes stays parked), #24. Stay, no PR 4 work: #195 (the integration
test opens its client with `natsfixture.Open`, adopted), #196 (no lifecycle owner), #703, #714, #9. No open PR overlaps
(inventory § 5); PR 5 waits for this; merge `origin/main` first if it moved. Risks: `NewHandlers` gains an argument (12
call sites, all in this package's tests and `doc.go`); `ActionHandler` gains a method (no implementer outside this
repository yet); `pkg/projection` changes one call, held by its tests; a busy entity can make a remove or update fail
on revision mismatch, returned and not retried.
