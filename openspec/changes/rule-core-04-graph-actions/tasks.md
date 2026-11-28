# Tasks: rule-core-04-graph-actions

Gates per code task: `task verify` on a clean tree. Pin paths are SemStreams `8b99efe9`, `processor/rule/`.

## 1. Design

- [ ] 1.1 OPEN-Q: the inventory review passes, the design review passes; the owner fills `Approved-by:`.
  Hold: OPEN-Q proposal awaits owner approval.
- [ ] 1.2 Fable answers, before any code, "Is a fourth `Run` argument handing every handler one set of outputs
  (publish, add, remove, replace; the framework stamping source and time and refusing the reserved space), with the
  executor unexported and a remove clearing only what rules wrote, the right exported dispatch surface and graph-write
  rule?" (`design.md` D2, D3, D4), or the owner declines that read.
  Hold: Fable's read of the design on PR #782.
- [ ] 1.3 Start implementation once 1.1 and 1.2 are done; merge `origin/main` into this branch first if it moved.

## 2. Port

- [ ] 2.1 Tests come before the code, each seen to fail first; after 2.7, run `task mutate:check` on each refusal and
  record detected or survived in `notes/2.1.md`, with why generated checks are not used (`design.md`, "Tests and docs").
  - `TestRuleReachesNoAgenticOrGovernancePackage` and its sensitivity test (D10): the fixture module reaches
    `agentic`, `vocabulary/agentic`, `processor/agentic-tools` and `governance` each through a middle package, and a
    clean neighbour named `notagentic`.
  - `TestRunDispatch` (D3, D10): a product handler for `test_family` in a set built by `NewHandlers` checks at load and
    runs at fire, receiving the body byte for byte, the execution context and the framework's outputs; a type missing
    from the set, a nil context and a nil execution context are refused, classified invalid, no handler called; a
    returned `*DenyVerdict` and a transient classified error pass through `errors.As` and keep their class.
  - `TestNewActionExecutorRefuses` (D3): a zero `Handlers`, no publisher, triple writes on with no writer, a writer
    with triple writes off.
  - `TestRunConcurrent` (D3), under `-race`: many goroutines run actions through one executor.
  - `TestOutputsPublish` (D2, D8): a product handler publishing to `user.response.x` is refused, classified invalid,
    and the publisher is not called.
  - `TestPublishAction` (D8): the envelope's exact keys and values (timestamp RFC 3339 with nanoseconds in UTC,
    `properties` `null` when absent, no `related_id`); semboids' `food-attract` action from
    `testdata/semboids-zone-steering/` gives `{"boid_id": "7", …}` for a message holding 7; a subject filled to
    `user.response.x` is refused with nothing published.
  - `TestUnfilledNameStopsAction` (D7): an unfilled token in a triple subject, a triple object, a publish subject and
    a publish property each fails naming field and token; nothing is written or published; nothing is logged.
  - `TestTripleActionFields` (D5): no subject writes to the firing entity; a filled subject writes to the named entity;
    neither is refused; the predicate is used as written; PR 3's five forwarded typed-object tests
    (`typed_substitution_test.go:60-272`) hold through the new writer.
  - `TestTripleWriterRefusesBeforeSending` (D4), over a stand-in requester: a subject that is not a six-part ID and an
    undeclared predicate are refused, classified invalid, with zero requests; a revision mismatch from the stand-in
    reconcile is returned with `errors.Is(err, errs.ErrRevisionMismatch)` after exactly one request; a commit-unknown
    transport failure is returned and not retried.
  - `TestTripleWriter` (D4), integration, real graph-ingest on the package's shared NATS server
    (`natsfixture.Open`, as `pkg/lifecycle/harness_gate_integration_test.go:74-104`): add appends under `rule_engine`
    and a repeat succeeds unchanged; an absent entity gives `entity_not_found` and creates nothing; remove leaves
    another source's statement of the predicate and clears the rule's; update over a held value raises the revision by
    exactly one and leaves another source's statement.
  - `TestBuiltinCheck` gains: `ttl` on add and update, `properties` on update, `object` on remove refused naming the
    key (D6); each triple type refused with triple writes off and accepted with them on (D9). The cases at
    `builtin_test.go:58`, `:65`, `:71` change to match.
- [ ] 2.2 Move `classifiedAppendFailure` and the entity-not-found case (`pkg/projection/mutation_client.go:219-228`,
  `:518-527`) into `internal/graphmutation` as `AppendResultError(entityID string, result graph.AppendSubjectResult)
  error` (nil for applied and unchanged); `pkg/projection` calls it (D4). Check: `go test ./pkg/projection/...
  ./internal/graphmutation/...`.
- [ ] 2.3 The triple writer from `triple_mutator.go` (D4): both constructor errors returned; the request timeout is
  `natsclient.DefaultRequestTimeout`; entry validation; source `rule_engine`, clock time, confidence 1; add one
  append, remove and replace one exact read and one conditional replace under `rule_engine`; no retry; no revision
  tracking. Not carried: `MutationTimeout`, `revisionTracker`.
- [ ] 2.4 `Outputs` and the framework's implementation (D2, D8): the reserved-subject refusal through
  `inReservedUserResponseFamily` before the publisher; triple methods delegate to the writer, and refuse with no
  writer.
- [ ] 2.5 `Run` on `ActionHandler`; the executor, its one constructor and `run` (D3); no logger, no setters.
- [ ] 2.6 `builtinHandler.Run` for `publish`, `add_triple`, `remove_triple`, `update_triple` from `actions.go:913-1311`
  (D5, D7, D8), filling with the one scan (`ec.substitute`) and refusing unfilled tokens; the five types PR 5 brings
  return an error naming their type. Not carried: the closed switch, `ExecuteAddTriple`, `ExecuteRemoveTriple`,
  `ParseTTL`, the predicate fill, the Debug lines, `related_id`.
- [ ] 2.7 `NewHandlers(product, cfg HandlerConfig)` with `TripleWrites` (D9); the key sets lose `ttl` (add, update),
  `properties` (update) and `object` (remove) (D6); every caller (`git grep -n 'NewHandlers('`: 12) updated.
- [ ] 2.8 `deny.go` carried with its three tests (`actions_test.go:3131-3188`) as `TestDenyVerdict` (D10).
- [ ] 2.9 Carry the tests of inventory § 1's Carried column, rewritten to the harness rules (`t.Context()`,
  `internal/harness/semantictest`), actions built by decoding JSON, the executor through its one constructor; the
  substitution cases at `actions_test.go:2651-2874` join `execution_context_test.go` where PR 3's tests lack them,
  `$related.*` and `$schedule.*` cases dropped. Check: `go doc ./processor/rule` lists no name without a reader outside
  the package or a spec line.
- [ ] 2.10 Comments: inventory F19's 14 history lines keep their reason and lose the history.
- [ ] 2.11 Extend the ledger row `processor/rule` with what this PR adds and does not carry (inventory § 1), with #27's
  `adapt` item: contract "the core compiles with no agentic or governance import; a product's action type is a handler
  in the set", proving tests `TestRuleReachesNoAgenticOrGovernancePackage` and `TestRunDispatch`, `known_risks` naming
  `class:port-refactor`; keep the `scripts/cover-check.sh` floor. Check: `task ledger:check`, `task cover:check`.
- [ ] 2.12 The PR body states the carried pin lines, production and test, as `sed -n a,bp | wc -l` ranges, and its
  adopter section says: a product handler implements `Run` with the outputs argument and publishes and writes triples
  through it, not its own connection; `NewActionExecutorFull` and `Execute` are gone, so semteams' contract tests call
  their handler's `Run` with a stand-in; a remove clears only what rules wrote; `ttl`, update's `properties` and
  remove's `object` are refused at load; an unfilled name fails its action.

## 3. Docs

- [ ] 3.1 Port `processor/rule/docs/actions.md`'s `add_triple`, `remove_triple`, `update_triple` and `publish` sections
  corrected to the code (inventory F15: no TTL, no "atomic remove+add" claim, the real envelope, no `${entity.type}`),
  plus a section on a product's action type and its outputs; update `syntax.md`'s key table (`:210-212`) and
  `doc.go`'s Actions section. Check: `task docs:check`.
- [ ] 3.2 Update the `processor/rule/` row of `docs/repository-map.md`: actions run; the processor and its inputs are
  not present. Check: `task docs:check`.

## 4. Land

- [ ] 4.1 Sync `openspec/specs/rule-actions/` and `rule-definitions/` and archive the change as the PR's last content
  commit. Check: `task spec:check`.
