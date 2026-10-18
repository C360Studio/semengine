# Design: graph-ingest-metering

This design rests on `inventory.md` (INVENTORY PASS, round 2). Premises cite its sections. Lines are at the base
commit `bccd8b8`.

Binding rules for the developer:

- Plain Go.
- No parameter that switches a function's behaviour.
- No test-only hook, field or option on `Component`.
- State travels as an argument or a field, never as a context value.
- No `time.Sleep`.
- Every new test uses an independent oracle and is shown able to fail. Its failing run on the base, or the mutation
  check that stands in for one, is recorded in `notes/<task>.md`.

## D1. One wrapper meters a mutation request (#303, #300 B)

Premises (inventory § 1, #303 and section B):

- Requests pass through two stacked wrappers, from `mutation_runtime.go:26` through `canonical_mutations.go:32`.
- WARN 1 is logged at `mutation_runtime.go:158` for every refusal. WARN 2 is logged at `canonical_mutations.go:40`
  for a revision mismatch only.
- Success replies are re-decoded at `canonical_mutations.go:46-53` and `:67-100`.
- The label is already the code (PR #286, `2d9ce7c`).
- Only one test reads WARN 2's text (`canonical_mutations_test.go:810`). No test counts mutation WARN lines.

Options:

| Option | Cost |
| --- | --- |
| (a) Do nothing | #303 stays; ruling L's last clause stays unimplemented |
| (b) Smallest fix: delete WARN 2 (`:39-44`) | Fixes #303 in 6 lines. The CAS-lost reconcile (`:315`) logs no expected revision, because its error text omits it; the re-decode and second wrapper stay |
| (c) Chosen: one wrapper, outcomes recorded at the decision | About 80 lines leave `canonical_mutations.go`, and the 10 test call lines move to the new wrapper |

Behaviour after (c):

- **Wrapper.** `setupMutationHandlers` subscribes each route through one wrapper, `meteredCanonicalMutation(route)`
  (today's outer wrapper, same name and signature). It is the only metering layer; the inner
  `meteredMutation(subject, handler)` goes.
- **Refused request, not an authority refusal.** `mutation_rejections_total{subject, reason=code}` +1 and
  `graph_mutation_outcomes_total{operation, outcome=code}` +1, with `internal` where the error has no code. A refusal
  that wraps a `graph.EntityPredicateContractError` or a statement refusal also counts once per distinct reason on
  `predicate_contract_rejections_total{lane=subject, reason}`, as `recordPredicateContractRejections` does today
  (`mutation_runtime.go:66-83`; reached from create, `canonical_mutations.go:236`, and append, `:341-342`). One WARN
  line, "graph mutation rejected", with attributes `subject`, `reason`, `error`, and `detail`: the classified error's
  Detail map, when it has one. For a revision mismatch the detail holds `entity`, `expected_revision` and (on a read
  mismatch) `current_revision`; the rejection sites are at `:296`, `:315`, `:449`, `:455`.
- **Authority refusal.** Unchanged: one authority WARN with no identity (`authority_gate.go:79-99`), reason
  `authority_foreign` or `authority_claimed`, outcome `entity_id_authority_invalid`.
- **Successful reply.** The handler counts its outcome after its reply is built and before it returns:
  - create `applied`;
  - reconcile `applied` or `unchanged`;
  - delete `applied`;
  - append once per subject: `applied`, `unchanged`, `entity_not_found`, or the failure's code from
    `canonicalAppendFailure` (`:495-502`).
- **Reply that cannot be built.** It is returned as `internal` and counted only under `internal`. A request
  therefore counts exactly once, and an append once per subject. The spec says "each canonical mutation", and the
  delta states the per-subject rule (§ Spec deltas).
- **Removed.** `meteredMutation` (the inner wrapper), `recordCanonicalResponse`, `canonicalErrorOutcome` (the wrapper
  code once and uses it for both counters) and WARN 2.
- **Unchanged.** A request refused before `Start` completes (`component.go:1048-1050`) is still not metered.

Declared metric change: no label value changes. Only the `internal` count of a reply that fails to re-decode
disappears, and that path is unreachable because the handler decodes its own `json.Marshal` output.

Tests:

- `TestRefusedMutationMetersAndLogsOnce` (failing first). A table that calls `meteredCanonicalMutation(route)` with the
  routes `canonicalMutationRoutes()` returns, as `setupMutationHandlers` does; the name and signature are the same on
  the base and after the change, so the test compiles on both and fails by assertion. One row each for: revision
  mismatch (reconcile and delete), `entity_already_exists`, `invalid_request`, `message_type_unregistered`, a create
  whose triple has an invalid predicate, and an authority refusal. Each row asserts:
  - exactly one `level=WARN` line (precedent: `keyed_ingest_test.go:270`, `strings.Count(logLine, "level=WARN")`);
  - +1 on the rejection series with the spelled-out reason;
  - +1 on the outcome series with the spelled-out code;
  - for the revision-mismatch rows, that the line contains `expected_revision`.
  - for the predicate row, +1 on `predicate_contract_rejections_total` under each distinct reason (the case
    `mutation_rejection_metric_test.go:110` pins today).

  The revision rows fail on the base (two WARN lines).
- `TestMutationOutcomeCountedAtTheDecision`. Drives create, reconcile (unchanged, then applied), delete, and an
  append with three subjects (applied, not found, unchanged) through the same wrapper, and asserts each series delta
  with label strings spelled out, not taken from constants. It passes on the base, because the re-decode gives the
  same counts. After the change it is shown able to fail by `task mutate:check` runs recorded in `notes/2.2.md` (the
  counting sites exist only then): drop append's per-subject count, and count reconcile's unchanged branch as
  `applied`.
- Removed with the re-decode: `TestCanonicalAppendOutcomeMetricCountsEachSubjectResult` and
  `…RejectsFailureWithoutDetail` (`canonical_mutations_test.go:867`, `:890`). `:788`'s WARN-2 text assertion moves
  into the table.

## D2. `DataFlow` counts input messages (#302; owner question 1)

Premises (inventory § 1, #302):

- `messagesProcessed` is incremented at four write sites (`component.go:2160`, `:2276`, `:2314`;
  `canonical_mutations.go:674`). None of them counts an input message.
- Append counts nothing.
- The fact lane counts a merge that wrote nothing (`:2160`, `revision == 0`) and each container born (`:2276`).
- `ErrorRate = errors/messages` (`:902-903`).
- No reader of `DataFlow` exists in this tree or at the pin: `service` fills `ComponentStatus.DataFlow` and nothing
  reads it.
- No spec line says what it counts.

Options:

| Option | Cost |
| --- | --- |
| (a) Do nothing | The number stays unexplained, and the first reader (change 3, or the #109 shell) inherits it |
| (b) Smallest: keep counting writes, add append, and document `MessagesPerSecond` as entity writes | A name that says "messages" counting writes; the per-delivery view is lost |
| (c) Recommended: count input messages | Moves four increments to two sites. `TestComponent_DataFlow_ReturnsMetrics` (`component_test.go:614`) changes, because in-process `CreateEntity` stops counting |

Behaviour after (c):

- **One count per input message.**
  - Stream lane: once per delivery, at the start of the consume callback (`component.go:1518`), before the metadata
    check. Poison, refused, stale and redelivered messages all count; a redelivery counts again.
  - Mutation lane: once per request, at the start of the D1 wrapper.
  - Queries are reads on the verb table, not a declared input port (`DefaultConfig`, `component.go:341-361`), and do
    not count.
  - In-process `CreateEntity` and container births are not input messages and do not count.
- **Bytes and last activity.** `bytesProcessed` adds the input payload's length at the same two sites, and
  `lastActivity` moves there too. The write-site increments of both go.
- **`ErrorRate`.** Its numerator (`c.errors`, which is also `Health().ErrorCount`) is unchanged. It now reads "errors
  counted per input message", and a multi-subject append that fails can push it above 1 (`component.go:2515`,
  `:2521`). Changing what `c.errors` counts is #300 D or #109, not this change.
- **`entities_updated_total`.** Answering #302's comment ("decide what each counter counts on every lane"): it counts
  every write that changed an entry. Append's applied subjects join create, reconcile, merge and container birth.
  Delete stays out, because removing an entry is not an update; deletes show on
  `graph_mutation_outcomes_total{operation="entity.delete"}`. A consumer reads it: semboids `cmd/sweep/main.go:466-467`
  takes `semstreams_datamanager_entities_updated_total` (its SemStreams-era name) as its ingest rate
  `entities_updated_per_s` (`:407`). semboids writes on the stream lane and through `pkg/lifecycle`; a search for
  `triple.append` and `AppendTriples` in its non-test Go finds comments only, so its rate does not change. In-tree
  delta tests (`entity_writes_test.go:596`, `:633`, `:706`) are stream-lane and unaffected.

Tests:

- `TestIntegration_DataFlowCountsInputMessages` (failing first). With `enable_hierarchy`, publish one stream message
  that creates its containers and one message whose merge writes nothing. Send one append with three subjects and
  one refused create. Send one entity query. The oracle is the test's own tally of messages sent (4) and their summed
  payload lengths. The in-package counters must equal it, and `entities_updated_total` must rise by the append's
  applied subjects. On the base the count is wrong several ways (container births and no append).
- `TestComponent_DataFlow_ReturnsMetrics` is rewritten to move the rate with one mutation request through the D1
  wrapper instead of `CreateEntity`.

## D3. Query refusals carry a code (#300 B; owner questions 2 and 3)

Premises (inventory § 1, query table):

- Ten return sites carry no code: `query.go:114`, `:223`, `:240`, `:270`, `:282`, `:305`, plus the four invariant
  failures `:363`/`:389`/`:435`/`:438`, which are unclassified and so fatal on the wire
  (`natsclient/errors.go:348-358`).
- `rejectInvalid` and `rejectInternal` exist (`mutation_runtime.go:214-224`) but are unused here.
- `"response_too_large"` is a literal at `:420` beside natsclient's unexported constant (`natsclient/request.go:20`).
- The empty-IDs batch reply is hand-written at `:118`.
- The entity verb already codes every refusal; a malformed ID is `invalid_request` (`query.go:78-80`), and this design
  keeps that. The batch verb does not validate IDs: an empty ID is reported as missing (`query.go:481`, `:495`).
- semsparql reads these replies by verb table (`reader/nats.go:72`). It writes its fallback code when a reply has
  none (`server/server.go:765-771`): `upstream_unavailable` for transient errors (503), `query_execution_error` for
  invalid ones (400), `internal_error` for fatal ones (500).

Options:

| Option | Cost |
| --- | --- |
| (a) Leave uncoded | No consumer change; the spellings stay scattered (ADR-060 at the pin: "Code must be NON-EMPTY") |
| (b) Code only where a spec line already requires one | No consumer change. Only `:243` and the pattern code are spec'd (spec `:113-119`) and both are already coded, so this is (a) plus moving `:72`/`:79`/`:96` onto the helpers |
| (c) Recommended: every refusal carries a code and keeps its class | invalid → `invalid_request`; transient → `internal`; the invariant failures → `internal` with class fatal; `:420` uses the exported constant; `:118` marshals `graph.EntityBatchResponse{}`; the entity verb is unchanged. Consumer-visible: semsparql's JSON `code` becomes `invalid_request` (400 rows), `internal` (503 rows) or `internal` (500 rows) where it showed `query_execution_error`, `upstream_unavailable` or `internal_error`; its HTTP status does not change, because the class does not. The empty-IDs reply becomes `{"entities":null}` (`graph/query_batch_types.go:52` has no `omitempty`), matching the all-empty-IDs reply |

- Question 3: export `natsclient.ErrorCodeResponseTooLarge`, beside `ErrorCodeKV*`
  (`natsclient/kv_key_contract.go:30-38`). Production callers: `natsclient/request.go:383` and `query.go:420`, so
  under #190 it is plumbing. It is still a new exported name, which puts this PR in the critical-stage review class
  for new exported framework surface. semsource
  (`processor/supersession/prefixquery.go:40`) and semsparql (`reader/nats.go:176`) may adopt it; this change does
  not touch them.
- Tests: `TestQueryRefusalsCarryACode`, a unit table over each batch and prefix refusal row, asserting class and
  code. The uncoded rows fail on the base. `query_prefix_*` tests that spell `"response_too_large"` keep the literal
  as their oracle.

## Exports (#190)

No export is removed or unexported. Touched:

- `(*Component).DataFlow`: kept; its meaning is stated for the first time.
  - Kind: API (`component.Discoverable`, `component/discovery.go:28`; `component-registration` spec `:82`; the new
    `graph-ingest-recovery` requirement).
  - Product-style tests: D2's two tests.
- `graph.MutationOutcome` and its four values, and the `graph.ErrorCode*` constants: unchanged. API (spec
  `:154-159`; `graph/mutation_responses.go:8-16`).
- `natsclient.ErrorCodeResponseTooLarge`: added if question 3 is answered yes. Plumbing (two repo callers):
  `natsclient/request.go:383`, `query.go:420`.
- `entities_updated_total` (metric, not a symbol): rises on append. Reader: semboids `cmd/sweep/main.go:466-467`
  (ingest rate); unaffected, as D2 says.
- Unexported code that goes: `meteredMutation`, `recordCanonicalResponse`, `canonicalErrorOutcome`, and
  `recordCanonicalEntityCommit`, whose remaining work is one `entitiesUpdated.Inc()`.

## Open issues and merge order (#160)

| Issue | Disposition |
| --- | --- |
| #302, #303 | retired (`Closes`); D2, D1 |
| #300 | section B done here (D1, D3); C, D, F, G, H stay under the owner's split (comment 6250544834); stays open |
| #306 | stays: the cache handle at `Stop`; may be overtaken by #300 C |
| #301 | stays: attached to the #282 lifecycle PR 2 |
| #291, #216 | stay: out of scope (PR body) |
| #270 | stays: one unexplained failure; filed as a flake if it recurs on this branch |
| #20 | stays: owner question on `rejectInternal`'s class; D3 reuses `rejectInternal` unchanged |
| #109 | stays: the shell will own `DataFlow`; D2 fixes what it counts, not where it lives |
| #106, #111, #33, #134, #141, #195, #196, #103 | stay; untouched |

No open pull request overlaps (inventory § 3). If the #282 lifecycle PR 2 opens first, it rewrites `start`, which
holds the consume callback D2 edits. Whichever merges second merges `origin/main` in.

## Owner questions

1. **What should graph-ingest's "messages per second" count?** Today it counts entity writes, skips appends, and
   counts some messages that wrote nothing. Nothing reads the number yet. **Recommended:** count input messages,
   meaning every delivery on its stream input and every mutation request, but not queries or graph-ingest's own
   internal creates. Bytes follow the same rule, and the "entities updated" counter also counts appended entities
   (semboids reads that counter as its ingest rate; it does not append, so its numbers do not move).
   **Alternative:** keep counting writes and say so in the docs.
2. **Should every query refusal carry an error code?** Some do and some do not. **Recommended:** yes, with no change
   to whether an error counts as invalid, transient or fatal. semsparql's error responses would then show
   `invalid_request` or `internal` where they now show semsparql's own fallback words (`query_execution_error`,
   `upstream_unavailable`, `internal_error`). Its HTTP status codes stay the
   same. **Alternative:** leave them as they are; only the internal spelling gets tidied.
3. **May natsclient export its "response too large" code as a named constant?** The string is typed by hand in
   graph-ingest and in two consumers. **Recommended:** yes, next to the KV codes natsclient already exports. It is
   used in two places in this repository; as a new exported name it gets the critical-stage review.

## Spec deltas (draft text for `specs/<capability>/spec.md`)

`graph-transport-boundary`: MODIFIED "Mutation replies carry four outcomes" (whole text in
`specs/graph-transport-boundary/spec.md`). ADDED "Query refusals carry a code", included only if question 2 is
answered yes.

`graph-ingest-recovery`: ADDED "graph-ingest's data flow counts input messages" (`specs/graph-ingest-recovery/spec.md`).
