# Inventory: simplify-04a-08-lifecycle

base: `127d4c09eec9541fa3a8365b6a6a2f5b0df3b756` (origin/main `e54b262` plus the empty claim commit)

Question: what in `pkg/lifecycle` do #154, #193, #199 and #257 ask to change, who reads each piece, and how the
emitter turns a graph mutation request's error into a commit state. Consumers read for symbol use
(`docs/inventory-scope.md`): semteams and semboids (symbol-level use of `pkg/lifecycle`); semsource and semconnect
import nothing from it (grep of their trees: no file imports `pkg/lifecycle`). Their local working trees were read
with grep. Pin facts come from a tarball of `8b99efe9` (`gh api repos/C360Studio/semstreams/tarball/8b99efe9…`,
local only).

This change carries no package from the pin. `pkg/lifecycle` is already ported (ledger row `pkg/lifecycle`,
`docs/admission-ledger.yaml:2112-2137`, `adapt`), so no pin probe is owed. Non-test lines: 3,775 (pin: 3,739).
The four issues touch about 10% of them.

## 1. The claimed gaps (re-measured; the issues cite PR #93 commits)

- **#257** — `pkg/lifecycle/graph_emit.go:98-99` — `default:` /
  `return fmt.Errorf("%w: %s: %w", ErrEmitFailed, operation, err)`.
  A classified reply with any code other than revision-mismatch, not-found or exists keeps its class, so
  graph-ingest's transient `internal` reply after an unconfirmed write reads `errs.IsTransient == true`. CONFIRMED.
  `projection.ClassifyMutationError` exists: `pkg/projection/mutation_client.go:423` —
  `func ClassifyMutationError(operation MutationOperation, err error) *MutationError {`.
- **#257 comment, item 1** — `graph_emit.go:102` — `if natsclient.IsNoResponders(err) {` is the only
  not-committed transport case. `graphmutation.TransportDeadline` (`internal/graphmutation/client.go:141-143`,
  `if err := ctx.Err(); err != nil {` … `Outcome: TransportDeadline`) falls through to `:111-114`, commit-unknown.
  CONFIRMED.
- **#257 comment, item 2** — `graph_emit.go:125-126` — `default:` / `return projection.MutationOperationAppend`.
  Callers pass only three constants (`:55`, `:69`, `:83`). CONFIRMED.
- **#154 item 1, nil client** — five refusals with four messages: `graph_emit.go:37` —
  `return &graphEmitterNATS{unavailable: errors.New("no NATS client")}`; `:41` —
  `return &graphEmitterNATS{unavailable: err}`;
  `:51`, `:65`, `:79` — `"%w: graph mutation client is unavailable: %w"`; `manager.go:191` — `if m.natsClient == nil {`;
  `manager.go:206` — `if m.exactReader == nil {`. Every emitter call follows a `getEntity` on the same path
  (`manager.go:396`→`:408`, `:396`→`:431`, `:561`→`:638`, `:743`→`:758`, `:882`→`:886`, `:923`→`:948`), so the
  emitter's refusal is reachable only by injecting a reader (`graph_emit_test.go:24` —
  `mgr.exactReader = bucketExactEntityReader{bucket: bucket}`). `graphmutation.NewClient` fails only for a nil
  requester or a non-positive timeout (`client.go:72-77`); the manager passes a non-nil client and `5*time.Second`,
  so `:41` and the `timeout <= 0` branch (`graph_emit.go:31-33`) are unreachable. CONFIRMED.
  **The premise "NewManager refuses a nil client" has readers against it** (§ 4).
- **#154 item 2, watch layers** — `manager_query.go:224-230` — `func (m *Manager) runWatchLoop(` …
  `onDelete func(entityID string) error,`;
  `:287` — `func (m *Manager) prepareWatchEntry(` returns `(lifecycleWatchDelivery, bool, error)`; `:315` —
  `func (m *Manager) deliverWatchEntry(`. Logged and returned: `:241` —
  `m.logger.Warn("lifecycle workflow watcher degraded",` and `:257` —
  `m.logger.Warn("lifecycle workflow watch encountered poisoned graph state; closing subscription",`. At the pin
  `Watch` returned a channel (`pin:pkg/lifecycle/manager_query.go:142` —
  `func (m *Manager) Watch(ctx context.Context, workflow string) (<-chan Participant, error) {`), so the log was the
  only signal; here the error is returned. A test pins the log:
  `watch_atomic_bootstrap_test.go:146` — `func TestLifecycleWatchPoisonWarningNamesSubscriptionAndAuthorityEntryOnce(`.
  CONFIRMED.
- **#154 item 3, two matchers** — `manager.go:368` —
  `matched, err := semtypes.MatchEntityIDPattern(reg.workflow.EntityIDPattern, entityID)`
  against `manager_query.go:505` — `func matchPattern(pattern, id string) bool {`, read at `manager.go:878`, `:919`,
  `manager_query.go:59`, `:292`, `:302`, `:495`. Registered patterns are already validated six-part patterns
  (`workflow.go:165` — `if err := semtypes.ValidateEntityIDPattern(w.EntityIDPattern); err != nil {`), so
  `matchPattern`'s `"*"` and other-arity branches are dead in production; only `projection_test.go:188`
  (`TestMatchPattern`) reaches them. CONFIRMED.
- **#154 item 4, change-log doc** — `manager.go:995-1008` —
  `// It does NOT check the decoded state's Workflow() against the argument. That`
  … `// constant (TestCreateFromOperator_UsesTheRouteSelectedRegistration).`; the same story at `errors.go:59-62` —
  `// Deliberately NOT a cause: a body whose Workflow() differs from the`. CONFIRMED.
- **#154 item 5** — `manager.go:136` — `_ = probe`. CONFIRMED. The mapping nit is #257.
- **#193 item 1** — `manager.go:842-857`: `Fail` checks only that `failed` is declared (`:850` —
  `const failedPhase = "failed"`), then calls `Transition`, which refuses an undeclared edge
  (`manager.go:574` — `if !reg.workflow.Transitions.IsValidTransition(currentPhase, newPhase) {`). Its doc says
  nothing about the edge. CONFIRMED.
- **#193 item 2** — `manager.go:807-811` — `func (m *Manager) selectReachableTerminal(…` /
  `terminals := reg.workflow.Transitions.TerminalPhases()` / `for _, terminal := range terminals {`;
  `transitions.go:130` — `sort.Strings(terminal)`. The order is by name. **The issue's premise "Transitions is a map,
  so declaration order is lost" is half true:** the phases are map keys, but each phase's out-edges are a declared
  `[]string` (`transitions.go:28` — `type Transitions map[string][]string`), so the current phase's edge order is
  available. Worked case at the pin: `agentic/agentrun/agentrun.go:60` —
  `"executing": {"awaiting_approval", "completed", "failed", "cancelled"},`: by name `Complete` picks `cancelled`;
  by edge order it picks `completed`. CONFIRMED.
- **#193 item 3** — `manager_query.go:414` — `page := refs[opts.Offset:end]`, then `:418` —
  `child, err := m.Get(ctx, r.workflow, r.entityID)`, and `:420` —
  `m.logger.Warn("lifecycle: Children — child load failed; skipping",`
  pages before loading, so a page can be short or empty while more children exist. CONFIRMED. A note on the same
  loop: it skips every error, not only absence (§ 7, D-1).
- **#199** — unit double `manager_test_helper_test.go:32-52` (`bucketExactEntityReader`) decodes with
  `graph.UnmarshalEntityState` only; integration doubles `manager_integration_test.go:87` (`startKVBackedResponders`,
  reconcile responder `:108-114`) and `:189` (`startExactEntityResponder`) likewise. graph-ingest's rule is
  `processor/graph-ingest/mutation_runtime.go:219` —
  `func decodeStoredEntity(key string, value []byte) (graph.EntityState, error) {`,
  which runs `checkStoredEntityKey` (`:244`) and `missingStatementMetadata`
  (`processor/graph-ingest/entity_writes.go:79`). The rule is held by
  `processor/graph-ingest/query_contract_guard_test.go:173` — `func TestBootSweepInventoriesStatementWithoutMetadata(`
  and the `storedPoison` cases at `poison_scoping_test.go:363-380`. CONFIRMED.

## 2. Every spelling of the facts being modelled

### Commit state of a mutation request's error

| Site | Spelling |
| --- | --- |
| `pkg/projection/mutation_client.go:423-431` | `ClassifyMutationError`: the one mapping (9a, D1) |
| `pkg/lifecycle/graph_emit.go:88-115` | `classifyEmitError`: three codes → sentinels, other replies → `ErrEmitFailed` (#257), no-responders → not committed, else unknown |
| `internal/graphmutation/client.go:55-61`, `:162` | transport outcomes; pre-send refusals labelled commit-unknown (#268, not this PR) |
| semteams `cmd/semteams/approvalpause/projection.go:160-172` | consumer hand copy (#258; at cutover) |

**"Does this ID belong to this workflow"**: `semtypes.MatchEntityIDPattern` (`pkg/types/entity_id.go:169`; readers
`pkg/lifecycle/manager.go:368`, `pkg/projection/mutation_client.go:286`, `:321`) and `matchPattern`
(`manager_query.go:505`, six readers, § 1).

**"No NATS client"**: five sites, four messages (§ 1, #154 item 1).

**Stored-value rule (a stored statement has Source and Timestamp, and the entity ID matches the key)**:
graph-ingest's `decodeStoredEntity` only. `graph.UnmarshalEntityState` (`graph/entity_predicate_contract.go:255`)
does not check either. Lifecycle readers without it: the three test doubles (#199) and production `Watch`
(`manager_query.go:299` — `if err := graph.UnmarshalEntityState(entry.Value(), &state); err != nil {`; § 7, D-2).

**Paging after filtering**: `List` filters, then applies Offset/Limit (`manager_query.go:58-91`), and skips only
`ErrEntityNotFound` and `ErrEntityNotLifecycleManaged` (`:63-68`); any other read error fails the call. `Children`
pages first and skips every error (§ 1).

## 3. Adjacent claims

- Specs: `openspec/specs/projection-mutation/spec.md:85-140` covers lifecycle's emitter in "A commit-unknown error is
  never retryable" and "A request no responder took keeps the transient class". "One mapping…" (`:119-140`) names
  only `MutationClient` writes. `component-registration` (`:38-56`) forbids `component` importing `pkg/lifecycle`;
  untouched. No capability spec states Manager's Fail/Complete/Children/Watch behaviour.
- Ledger: `docs/admission-ledger.yaml:2118-2126`, contract item (4): "nil NATS client returns ErrEmitFailed without
  panicking (#102)". `known_risks` `:2130-2131`: Watch kept whole until #24 (ADR-111 accepted; journal deferred).
- Accepted design: `openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/design.md:1147-1150` (D17): "With
  no client it holds no emitter, and every emit returns `ErrEmitFailed` naming the missing client."
- ADR-111 (`docs/adr/111-durable-execution-primitive.md`): Decision 3, a commit-unknown is never retried; `:194`,
  "`pkg/lifecycle` and `pkg/projection` stay what ADR-047, ADR-049 and ADR-091 made them".
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`, 2026-10-16): #272 (this claim, 0
  files); #271 (`.markdownlint-cli2.yaml`, not draft); #242–#248 (Dependabot: `go.mod`/`go.sum`, `package*.json`,
  `ci.yml`). None touches `pkg/lifecycle`, `pkg/projection`, `openspec/specs/projection-mutation/` or the ledger.
- Unclaimed neighbours: #268 (`internal/graphmutation`, PR 8's package); #132 item 3 (PR 8 may replace
  `graph.ExactEntityReader`, the type of `Manager.exactReader`).

### Open issues naming the package

Query: `gh issue list --state open --search "lifecycle" --limit 200` (24 hits), each read.

| Issue | Names it by | Disposition (the design records it) |
| --- | --- | --- |
| #154 | title, body | this PR |
| #193 | title, body | this PR |
| #199 | title, body | this PR |
| #257 | title, body, `graph_emit.go` | this PR |
| #268 | body ("and `pkg/lifecycle` after PR 9b") | stays: work is in `internal/graphmutation` |
| #132 | item 3, `pkg/lifecycle` fakes of `ExactEntityReader` | stays: PR 8 (`graph`) |
| #20 | `pkg/lifecycle/graph_emit.go:98` (pin) | stays: graph-ingest's server half |
| #24 | body, rulings | stays: epic; ADR-111 keeps the package as is |
| #33 | `pkg/lifecycle/harness_entity.go:14,63` | stays: lifecycle's half is already done (`git grep -n payloadbuiltins -- pkg/lifecycle`: nothing); other packages remain |
| #195 | `manager_integration_test.go:45` | stays: lists lifecycle as already adopted |
| #129 | `go test … ./pkg/lifecycle` as an example | stays: harness |
| #223, #9, #7 | epics and rulings | #223 PR 9 line ticks with this PR (`Addresses #223`) |

The other hits (#29, #38, #80, #81, #86, #109, #136, #139, #153, #189, #196) name the word, not the package. #196
matches `pkg/lifecyclecleanup`, and #153's lifecycle items are `internal/lifecycleguard`.

## 4. Readers of the surface the issues name

- `NewManager`: semteams `cmd/semteams/main.go:792`; semboids `cmd/semboids/main.go:187`; integration tests semboids
  `internal/boidgraph/migration_integration_test.go:63`, `internal/sim/cull_integration_test.go:129` (all non-nil).
  **With nil**, to check workflow registration only: semboids `internal/boidgraph/lifecycle_test.go:13` —
  `mgr := lifecycle.NewManager(nil, nil)`; semteams `test/contract/agentrun_registration_test.go:42`, `:72`. At the
  pin: `processor/gated-dag/executor_lifecycle_test.go:94`,
  `agentic/agentrun/milestone_settlement_internal_test.go:547`, `:668`. In this repo: `workflow_surface_test.go:106`,
  `:121`, `:144`, `:180`, `graph_emit_test.go:19`. No in-repo production caller (no file outside `pkg/lifecycle`
  imports it except `processor/graph-ingest/component_fixture_test.go:97`, for `RegisterPayloads`).
- `ErrEmitFailed`: no reader outside `pkg/lifecycle` in this repo, the pin, semteams or semboids (grep). Readers
  inside: `graph_emit_test.go:50`; writers `graph_emit.go:51`, `:65`, `:79`, `:99`, `manager.go:1054`, `:1059`.
- `ErrAlreadyExists`: semboids `internal/sim/lifecycle.go:292` —
  `if err == nil || errors.Is(err, lifecycle.ErrAlreadyExists) {` (inside `createWithRetry`, which retries any
  other error). `ErrWorkflowNotRegistered` and `ErrWorkflowAlreadyRegistered`: semteams contract tests.
- `Complete`, `Fail`: pin `processor/rule/actions_lifecycle.go:113`, `:148` (rule actions). No consumer calls them,
  and no consumer config names `lifecycle_complete`/`lifecycle_fail` (grep of semteams and semboids
  `*.json`/`*.yaml`/`*.go`).
- `Children`: pin `gateway/lifecycle-gateway/handlers.go:335` —
  `kids, err := c.manager.Children(r.Context(), entityID, lifecycle.ChildOptions{})`
  (a consumer adapter; no Limit). No consumer calls it.
- `Watch`/`WatchEvents`: no consumer call (grep). The signatures do not change.

## 5. Problem shape

(a) Classify a request error into a commit verdict plus class: closest instance `projection.ClassifyMutationError`
(`mutation_client.go:423`), adopted. (b) Filter, then page: `List` (`manager_query.go:58-91`), adopted for
`Children`. (c) One refusal for a missing dependency: `ensureBucket` (`manager.go:191-193`) is the shape, given one
message. No new primitive is established, so no adoption sweep is owed.

## 6. Pin decision records

Every pin ADR fetched; `grep -l -e 'pkg/lifecycle' -e 'lifecycle.Manager' -e 'LifecycleManager'` hits: 046, 047,
049, 050, 053, 054, 055, 056, 058, 060, 063, 073, 081.

- **ADR-047** (superseded by 049): Manager surface, including `Complete` and `Fail` (`:233-234`). It decides no
  selection rule.
- **ADR-049** (accepted): Manager API (`:300-327`); Children is depth-1 and paged (`:305`, `:389`); "operators
  paginate" over N+1 child reads (`:544-547`). It decides no terminal-selection or Fail-edge rule.
- **ADR-053** D3 (kept by ADR-111): the engine never infers a terminal phase from an outcome. `Complete` and `Fail`
  are explicit calls, so the rule is untouched.
- **ADR-060** (accepted): control flow is one sentinel (`errs.ErrRevisionMismatch`, `:248-254`) matched with
  `errors.Is`, and the rest is `ce.Code`; lifecycle's CAS loop is its named reader (`:225`).
- **ADR-091** `:69`, `:100`: a lost reply after possible delivery is commit-unknown and is not retried.
- 046, 050, 054, 055, 056, 058, 063, 073 and 081 name the package as context or precedent. None decides the
  behaviour these issues touch.

ADR-049 is not ported to `docs/adr`. This change keeps every ADR-049 decision, so no port is owed (Bounded port
scope).

## 7. Defects found outside the four issues (for filing)

- **D-1, HIGH.** `manager_query.go:418-427` — `Children` skips a child whose `Get` failed for any reason and returns
  success. Real trigger: the exact read times out under load (`nats.ErrTimeout`), or the client is reconnecting.
  The child then silently disappears from the page (and from the pin lifecycle-gateway's 200 response). `List`'s
  rule (`:63-68`) is the fix. It sits in the loop #193 item 3 rewrites (owner question 3).
- **D-2, MEDIUM.** `manager_query.go:299` — `Watch` decodes stored values with `graph.UnmarshalEntityState`, which
  checks neither statement metadata nor that the entity ID matches the key. It delivers a Participant that
  graph-ingest's exact read refuses (`missing_statement_metadata`, `noncanonical_entity_id`). Trigger: a writer
  other than graph-ingest (only graph-ingest writes `ENTITY_STATES`), so MEDIUM under #185. Home: `graph` (PR 8),
  by exporting graph-ingest's stored-value rule. That would give #199's second option a production reader.

## Adopter seams (existing surface)

- `NewManager(nil, …)`: three consumer test sites rely on it to check registration without NATS (§ 4). Refusing nil
  needs a signature change (`(*Manager, error)`) at eight consumer call sites, found by compile error, and leaves
  the Register-only check with no client-free way to run.
- Write errors: a consumer that matched `ErrEmitFailed` on a write would stop matching. Measured: none does.
  semboids' `createWithRetry` retries any error, including a future commit-unknown. The retried `Create` reads
  first, sees the phase triple and gets `ErrAlreadyExists`, which it treats as success, so a duplicate write cannot
  follow.
- `Complete`: a workflow author must know today that the pick is alphabetical, and finds out only from a Warn log
  (`manager.go:823-829`).
- `Children`: a caller cannot tell a short page from the last page. It finds out nowhere.
