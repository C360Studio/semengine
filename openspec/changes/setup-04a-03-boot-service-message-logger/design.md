# Design: setup-04a-03-boot-service-message-logger

base: `265235183bf517121883c4b957fb37356c901835` (draft PR #367). Inputs: `sweep.md` (this change; M-ids), the
whole-package inventory (`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, S-ids), and
PR 3's archived design (`openspec/changes/archive/2026-10-22-setup-04a-03-boot-service-component-manager/design.md`),
whose shape this copies. Pin lines are SemStreams `8b99efe9`, `service/`; `:n` is `message_logger.go`. One lifecycle
owner, 2,245 non-test pin lines (under the ~3,000 bound, #159).

Owner intent applied, not re-asked: ADR-113 (context ownership and lifecycle control); no context-value bags; no back
doors on production types; natsclient passes nats.go through; no SemStreams state migration; simple idiomatic Go;
port whole, drop only dead surface; every real defect found and not fixed is filed.

## Inventory addendum

- **Callers outside `service`.** Pin, typed uses of every exported logger name: 0 (the inventory's tool,
  `users-prod.tsv`: `NewMessageLogger`, `NewMessageLoggerService`, `MessageLogger.*`, `MessageLoggerConfig`,
  `MessageLogEntry`, `KVWatchEvent`, `KVWatchConnectedEvent`, `DefaultMessageLoggerConfig`). Consumers: no Go use. The
  surface reached from outside is the configuration and the routes:
  - semteams enables it with explicit subjects (`configs/e2e-flow-bootstrap.json`: `{"max_entries":10000,
    "monitor_subjects":["agent.>","tool.>","user.>","graph.mutation.>","discovery.>"]}`)
  - semteams reads `/message-logger/entries` from its UI (`ui/src/lib/services/messageLoggerApi.ts:42`,
    `components/board/TaskTrace.svelte:66`)
  - semteams reads `stats`, `trace/{id}`, `kv/{bucket}` and `kv/{bucket}/watch` from `taskfiles/dev.yml:140-181`.

  semsource, semconnect, semboids: none.
- **Who mounts it.** `serviceNameToPrefix` keeps `message-logger` (`service/service_manager_http.go:380-381`); the
  service is not materialized by default (`service/service_configs.go:85-89`).
- **Open pull requests and issues:** `sweep.md` § 4 (#367 this claim; #358 no overlap, either order).

## D1. Files and surface (surface audit, #190)

- **Carried, adapted:** the three files, module path rewritten, § 2.4 renames (sweep § 1).
- **Dropped, no reader:**
  - `GetMessages` (M9)
  - `ConfigSchema` (M8), and with it `ConfigSchema`, `PropertySchema`, `NewConfigSchema`: `service/configurable.go`
    and `service/configurable_test.go` are deleted, closing PR 1's D1 plan
  - the reconciliation of a changing subject set (M1)
  - the test-swap seams `subscribe`, `retryAfter`, `handleKVQueryWith`, `kvBucketProvider`, the two S10 interfaces
    (M2)
  - `DefaultMessageLoggerConfig`, and the exported `NewMessageLogger` and `SetDecoder` (M5).
- **Construction** (`NewMessageLoggerService`, the one constructor, registered by `RegisterAll`) decodes strictly
  (unknown key, and a key in another letter case, refused: the existing requirement) and refuses each of these,
  naming it:
  - a nil NATS client (as `:49-51`)
  - a nil payload registry (M6)
  - `"*"` without a component registry (moved from `Start`, `:710-713`)
  - `max_entries` < 0 or > 100000 (`:117-127`)
  - `sample_rate` < 0 (M7).

  Defaults have one home: 0 means 10000 entries, 1 means every message; an empty `monitor_subjects` means `["*"]`.
- **Kept with contract lines** (README, ledger `contract`, D9):
  - framework API, read by the composition root through config and HTTP: `NewMessageLoggerService` (through
    `RegisterAll`), `MessageLoggerConfig`, `MessageLogger`, `MessageLogEntry` (OpenAPI schema), `KVWatchEvent`,
    `KVWatchConnectedEvent` (SSE wire shapes)
  - read by the routes and kept as at the pin: `GetLogEntries`, `GetEntriesByTrace`, `GetStatistics`
  - `RegisterHTTPHandlers`, `OpenAPISpec` (`HTTPHandler`).
- **Options:**
  - (a) chosen
  - (b) keep the seams and the reconciliation: about 270 lines and 7 tests that guard paths production never takes
  - (c) keep `ConfigSchema`: an unread method plus a second, contradicting spelling of the `max_entries` bound.

## D2. Lifecycle: Start bounds Start; Stop is the only stop

Governed by ADR-113, #196 and ADR-108 Decision 2.

The logger composes `internal/lifecycleguard` for `Start` and `Stop` (S3), as `BaseService` and the component
manager do (`service/base.go:191-193`, `service/component_manager_lifecycle.go:32-34`). Observable behaviour, each
with its proving test (D7 units in brackets):

- **Refusals.** `Start` refuses a nil or ended context, a second `Start`, and a `Start` after any `Stop`
  (`errs.ErrAlreadyStarted`); a refused `Start` holds nothing. `Stop` refuses nil; given an ended context it returns
  that context's error and stops nothing. Proof: `TestMessageLoggerPassesTheLifecycleSuite` [U-a]; must-fail owner =
  a logger stopped before `Start` (`service/base_lifecycle_test.go:249-259` precedent).
- **Startup work.**
  - `Start` starts the base service, then subscribes to each subject (D3) in sorted order.
  - The subscriptions are made under the one run context `Start` creates, kept only as a cancel (graph-ingest
    precedent, `processor/graph-ingest/component.go:982`, `:1024-1027`). natsclient makes that context the parent of
    every message's context (`natsclient/client.go:1036-1047`), so the Start context must not be it (M4).
  - `Start` checks its own context before each subscribe. If it has ended, `Start` returns that error and holds
    nothing: the guard rolls back through the cleanup. The subscribe already in progress may finish first; natsclient
    bounds its wait for the server by `DefaultRequestTimeout` (`natsclient/client.go:1003-1016`), not by the Start
    context.
  - Proof: suite `FailedStartHoldsNothing`. The mid-start cancel is review only: no test can end the context between
    two subscribes without a hook; `notes/2.3.md` records it.
- **Ending the Start context after `Start` returned stops nothing.** A message published afterwards is stored and
  counted. Proof: suite `StartContextEndStopsNothing`; `TestMessageLoggerKeepsCapturingAfterStartContextEnds` [U-d]
  (the probe's second run, inverted).
- **Retry.** A subscribe that fails with anything but a malformed subject (D3) leaves the logger running and
  degraded, as at the pin: `subject_reconciliation_degraded` and `_error` in `/stats`, Error line. The failed subjects
  are retried every 100 ms under the run context. The retry ends in `Stop`, which waits for it. Proof:
  `TestMessageLoggerRetriesSubscribeAfterReconnect` [U-d, own server: `// Own server: stops and restarts the server`
  with `natsfixture.Fixture.Stop`/`Start`].
- **Stop.**
  1. `Stop` ends the retry and joins it.
  2. It drains every subscription (`natsclient.Subscription.Drain`, which returns once the in-flight callback has
     returned, `natsclient/client.go:957-964`), so a message being handled when `Stop` begins is stored.
  3. Then it ends the run context, ends every open watch stream (D4) and waits for each stream's KV watcher to stop,
     and stops the base service.
  4. It returns nil only once all are released.

  Proofs:
  - `TestMessageLoggerStopDrainsInFlightMessage` [U-d]: the composition's `slog` handler blocks on the "Message
    sample" record until the test releases it; `Stop` runs on a goroutine. A production dependency, not a seam.
  - `TestMessageLoggerStopEndsOpenWatchStreams` [U-g]: an open watch's body reaches EOF and `Stop` returns nil.
  - `TestMessageLoggerIgnoresPublishesAfterStop` [U-d].
- **A failed `Stop` is retried (M3).** If the context ends first, or a drain fails, `Stop` returns that error. Every
  subscription not yet drained, the retry and open streams are left for the next `Stop`, which runs the same
  cleanup. A `Stop` after one that returned nil returns nil and calls nothing; a `Stop` while another runs is refused
  as transient. Proof: `TestMessageLoggerFailedStopIsRetried` [U-d]. The same blocking handler holds a drain; the
  first `Stop`'s context is cancelled; after release, the second `Stop` returns nil and a later publish is not
  captured. If the developer cannot observe that the first `Stop` reached the drain without a sleep, the mutant row
  is recorded as uncovered.
- **Reads during and after Stop.** `/entries`, `/stats`, `/trace` answer from memory at any time, holding no lock a
  `Stop` needs. `/subjects` answers `[]` after `Stop` (`:928-931`). A watch requested after `Stop` began answers 503
  and opens nothing. Proof: `TestMessageLoggerRoutesAfterStop` [U-e].
- **Context rules:** the logger keeps no `context.Context`; it holds one private cancel. The watch stream learns of
  `Stop` through a value the logger closes or a run-context child created per request and joined, not through a
  stored context. Locks, channels and join order are the developer's (failing-first under `-race`).

## D3. Subjects: one fixed set, covered subjects once, a malformed subject refused

- **The set** is computed once in `Start`, as at the pin (`:341-353`):
  - the explicit subjects, plus, with `"*"`, every NATS, NATS-request and JetStream port subject of every admitted
    declaration, skipping `*`, `>` and `_INBOX` (`:355-388`)
  - read through `component.Registry.Snapshots()`, the pairing in D5.

  The `Snapshots` doc comment (`component/registry.go:683-687`, "It exists for the component manager") gains the
  logger as its second framework reader (one sentence).
- **Covered subjects (M10).**
  - Replaces the three agentic pairs (`:391-395`, `:652-656`): a desired subject covered by another desired subject
    is not subscribed, and each omission is reported in `/stats` `subject_overlaps` as `{broader, covered,
    "covered subscription omitted"}`.
  - Law: every desired subject is covered by exactly one subscribed subject, and no subscribed subject covers
    another.
  - One unexported function decides "pattern covers subject" by NATS token rules (`*` one token, `>` one or more,
    last). The entries filter uses it too (Q1(c)).
  - Neighbour, not consolidated: `internal/flowgraph/flowgraph.go:366` `matchNATSPattern` answers a different
    question (do two patterns overlap, symmetric) and is unexported in another package.
  - Proof: `TestPropCoveredSubjectsSubscribedOnce` (generated, `pgregory.net/rapid`, U-b) and
    `TestMessageLoggerCapturesCoveredSubjectOnce` [U-d].
  - Options: (a) generic, chosen; (b) carry the pairs (a consumer's subjects in the framework, other overlaps
    captured twice); (c) no overlap handling (every overlap captured twice).
- **Malformed subject (M11).**
  - A subscribe refused with `nats.ErrBadSubject` (probe, `sweep.md` § 3) fails `Start`, naming the subject and
    `monitor_subjects`; the guard rolls back.
  - Proof: `TestMessageLoggerRefusesMalformedSubject` [U-d].
  - This applies by analogy the owner's 2026-10-21 Q-B ruling on PR 3, which said fail boot and name it. That ruling
    was about a component that cannot be built, not this logger. With no subject validator in the tree, the
    library's refusal is the check.

## D4. HTTP surface (`operator-surface`; 03B D14/D15 row "Keep", `docs/contract.md:139`)

- **Routes.** Served after boot under `/message-logger/`:
  - `entries` (`limit` 1-10000, default 100; `subject` filter)
  - `stats`
  - `subjects`
  - `GET trace/{traceID}` (32 hex, else 400)
  - `kv/{bucket}` (`pattern` key glob, `limit` 1-1000, `status`)
  - `GET kv/{bucket}/watch` (SSE: `connected`, then `kv_change` events, the initial values ending with an
    `initial_sync_complete` operation, `retry: 5000`).

  Every route answers `GET` only; any other method gets 405.
- **OpenAPI without an import-time registration (M15).** `RegisterAll` registers the logger's spec, as PR 3 D5 option
  (a). Proof: `TestOpenAPIDocumentResolvesEverySchemaRef`, extended so the composition enables `message-logger` with
  `"*"` and an empty registry (no NATS needed).
- **Error kinds (S7).**
  - `errors.Is(err, jetstream.ErrBucketNotFound)` → 404 and `errors.Is(err, jetstream.ErrNoKeysFound)` → empty
    result. Probe at nats.go v1.54.0, `sweep.md` § 3.
  - Proof: `TestKVQueryMissingBucketIs404AndCreatesNothing`, `TestKVQueryEmptyBucketReturnsNoEntries` [U-f].
- **One spelling of the bucket and its name check (M16).** Both KV routes take `{bucket}` from the route
  (`r.PathValue`), not the hard-coded prefix. One validator (empty, `.`, `..`, `/`, `\`, `*`, `>` → 400) serves both.
  Proof: `TestKVRoutesRejectMalformedBucket` [U-e].
- **The watch filter (M17).** Empty or `*` watches every key, as at the pin
  (`message_logger_kv_watch.go:216-220`). Any other `pattern` passes `natsclient.ValidateKVWildcardFilter`, else 400
  naming it. This accepts a final `>`, which the pin refused, and refuses a space, which it accepted. Proof:
  `TestKVWatchRejectsInvalidFilter` [U-g].
- **The trace query (M13).** The trace index is dropped. `GetEntriesByTrace` scans the buffer (at most 100,000
  entries) and returns the trace's entries still held, oldest first. Memory stays within `max_entries`. Proof: the
  pin's `:268-347` and `:379-415`, adapted [U-b].
- **Text (M20).** The dev/test-mode claims, the 403, `Last-Event-ID` and "reconnection" are rewritten to what the code
  does. The README states the gate: the service is off unless configured. The watch keeps one KV watcher per stream
  (pin ADR-081 not done; M22).
- **Retained and declared:** the query's `pattern` is a key glob while the watch's is a NATS filter (one parameter
  name, two grammars; changing the query's is the owner's, not asked); `/stats` keys as at the pin, except Q1(d).

## D5. #280 item 2: `component.Declaration`'s parallel port and facts lists

`Declaration` holds `InputPorts`/`OutputPorts []Port` beside `InputFacts`/`OutputFacts []PortFacts`
(`component/registry.go:62-70`); `InputFacts[i]` describes `InputPorts[i]`, which its doc comment does not say. Every
reader (gopls `references` on `component/registry.go:66-69`, production):

- **Writers:** `component/registry.go:328-331` (`Declare`), `:622-625` (admission), both from one projection
  (`cloneAndProjectPorts`, `:660-681`). Clone: `:706-709`.
- **Facts only:** `exclusiveClaims` `:79-80`, `:647-655`.
- **Pairs read by index:**
  - `comparePortLane` `:353-378` (no bounds check on facts)
  - `composition/analyze.go:162-166`, `:217-221` (with `index >= len(facts)` guards), `:233` (ports only),
    `:292-300` with `interfaceType` `:349-357` (guarded)
  - `composition/catalog.go:82-83` and `composition/graph.go:52-86` (`portViews`, guarded `:63`)
  - `internal/flowgraph/flowgraph.go:88-89`.
- **Tests:** `component` (`registry_declare_test.go`, `discovery_test.go`, `register_test.go`,
  `registry_boot_admission_test.go`) and `internal/flowgraph/declarations_test.go`; none in `composition`.
  Consumers: no use (consumer tarballs, `grep -rn 'InputDeclarationFacts|component.Declaration'`: 0).

Options:

- **(A, recommended) Keep the two lists.**
  - The logger reads them in pairs, `ports[i]` with `facts[i]`, as `portViews` does (the pin's `:363-367` already
    does).
  - One sentence on `Declaration` states the alignment, and that both lists come from one projection.
  - Changes nothing else. A hand-built misaligned `Declaration` has no path today: every one comes from `Declare` or
    admission.
  - Item 2 is then decided ("kept"); #280 stays open for items 1 and 3.
- **(B) One list of pairs** (`Inputs, Outputs []DeclaredPort{Port, Facts}`).
  - Misalignment becomes unrepresentable, and the four dead bounds checks go.
  - It changes `component`'s exported type and every reader above, in three packages plus the tests: a `component`
    pull request of its own (#159), not this `service` one. Filed as a `class:port-refactor` issue if chosen.
- **(C) Facts cached on `Port`.** Rejected. `Port` is a plain value callers build and edit; `Port.Facts()`
  (`component/port_resolver.go:168`) revalidates and can fail. A cached copy can go stale and changes what admission
  promises (#280's own text).
- **(D) Leave item 2 open:** costs a decision later for the same answer.

## D6. Adopter seam (a composition root enabling `message-logger`)

1. **They must know:**
   - (i) enable it in `services` with `monitor_subjects`
   - (ii) `"*"` needs `Dependencies.ComponentRegistry`
   - (iii) `Dependencies.PayloadRegistry` is required.
2. **If they do nothing:** the logger is off (not materialized). If they get (ii) or (iii) wrong, construction
   refuses, naming the field. A typo'd subject fails boot, naming it (D3).
3. **Where they find out:** a boot error, for all three. Today (iii) is nowhere: every message is silently "raw".
4. **What they should have to know:** (i) only. (ii) and (iii) are the composition the root already builds (semteams
   `cmd/semteams/main.go:208`, `:789`).

Intent check: no boundary moves.

## D7. Tests

| Pin test file (lines) | Here |
| --- | --- |
| `message_logger_test.go` (415) | U-b; `:12-55`, `:67-75` dropped (D1); `:76-96` replaced by the helper (no fixed address); trace tests use `storeEntry` only |
| `message_logger_registry_test.go` (276) | behaviours moved to U-d on the shared server; fake subscriber `:193-276` dropped (M2); `:80-133` flipped (M3) |
| `message_logger_lifecycle_test.go` (378) | `:70-100` → the suite; `:102-145` → `TestMessageLoggerStopDrainsInFlightMessage`; `:147-178` → `TestMessageLoggerFailedStopIsRetried` and the retry test; `:180-378` dropped with M1 (they test the obsolete-drain and reconcile gate) |
| `message_logger_subscription_integration_test.go` (92) | U-d; `:62-63` flipped (a failed Stop is retried) |
| `message_logger_http_kv_query_test.go` (223) | `:126-147` carried (U-e); `:98-125`, `:149-205` moved to real NATS (U-f); `:206-223` dropped (asserts the injected provider's context: mechanism, M2) |
| `message_logger_kv_acquisition_integration_test.go` (145) | U-f, through `fixtureClient`; `graph.EnsureCatalogBucket` from `graph/kvcatalog` |
| `message_logger_kv_watch_test.go` (92) | U-g; `TestIsValidWatchPattern` replaced by the shared validator's cases; `TestDetectKVOperation` per Q1(b) |
| `message_logger_user_response_test.go` (49) | U-h: a test payload registered in a local `payloadregistry` instead of `agentic.UserResponse` (ADR-093 kept) |
| `message_logger_census_test.go` (556) | not carried (imports outside the port set) |

**Units** (one or two files each; parallel developers; brief cites only the D-sections named):

- **U-a** `service/message_logger_helpers_test.go`, `service/message_logger_suite_integration_test.go` (new). D1, D2.
  - Calls: `NewMessageLoggerService(json.RawMessage, *Dependencies) (Service, error)`; `*MessageLogger` `Start`,
    `Stop`, `Status`; `fixtureClient(t)` (`service/fixture_client_integration_test.go:48`); `newUnconnectedClient`
    (`service/service_manager_test.go:817`); `component.NewRegistry`; `payloadregistry.New`; `lifecycletest.Run`,
    `Owner`, `Observation`, `Promise`.
  - `Observe` reports base status, held subscriptions, a running retry and open streams (unexported reads in the
    package; no guard-state field).
- **U-b** `service/message_logger_test.go`. D1, D3, D4 (trace).
  - Pin `:98-415` adapted: sampling `:98-148`, `containsWildcard` `:150-189`, `uniqueStrings` `:191-196`, overlap
    `:198-220` rewritten for the generic rule, config `:222-266`, trace and ring `:268-415`.
  - New: construction refusals (D1) and `TestPropCoveredSubjectsSubscribedOnce`.
  - Calls: `newMessageLogger`-free: entries are stored through `handleMessage(ctx, subject, data)` and `storeEntry`;
    `GetLogEntries`, `GetEntriesByTrace`, `GetStatistics`.
- **U-d** `service/message_logger_subscription_integration_test.go`. D2, D3.
  - Pin `subscription_integration_test.go:17-92` and `registry_test.go:19-191` behaviours on the shared server.
  - New tests: Start-context end, in-flight drain, failed Stop retried, publishes after Stop, covered subject once,
    malformed subject, retry after reconnect (own server).
  - Calls: `natsclient.Client.Publish`, `Subscription` via the logger only, `natsfixture.New`, `Fixture.Stop`,
    `Fixture.Start`.
- **U-e** `service/message_logger_http_test.go` (new). D4, Q1.
  - Routes on a logger built over `newUnconnectedClient` with `"*"` and an empty registry: entries (limit, filter),
    stats, subjects, trace (400 on bad id), 405, bucket validation, routes after Stop.
  - Plus `recordMatchesStatus` (pin `http_kv_query_test.go:126-147`) and `TestEntriesServeNonJSONPayload` (Q1(a)).
  - Calls: `RegisterHTTPHandlers("/message-logger", mux)`, `httptest`.
- **U-f** `service/message_logger_kv_integration_test.go`. D4.
  - Pin `kv_acquisition_integration_test.go:18-145` and `http_kv_query_test.go:98-125`, `:149-205` on the shared
    server: missing bucket 404 and nothing created, empty bucket, failed-record round trip, lookup failure 500
    (unconnected client).
- **U-g** `service/message_logger_kv_watch_test.go`, `service/message_logger_kv_watch_integration_test.go`. D2
  (streams), D4, Q1(b), Q1(e).
  - Validators; event sequence `connected` → initial values → `initial_sync_complete` → `kv_change`; filter refusal;
    `Stop` ends streams; 503 after `Stop`; missing bucket 404 (Q1(e)).
- **U-h** `service/message_logger_decode_test.go`. D1 (payload registry), ADR-093.
- **Integration with PR 3's test:** `TestOpenAPIDocumentResolvesEverySchemaRef` gains the logger (D4); one line in its
  composition.

Harness rules as PR 3's D7: no `time.Sleep`, `Skip`, fixed port, or `t.Parallel` in an integration file; cleanup
stops under `context.WithTimeout(context.WithoutCancel(t.Context()), ...)`; every NATS connection through
`fixtureClient` (#195) except the own-server retry test.

**Generated checks.** The covered-subject rule is a stated law (D3), so it gets a rapid property:

- generator: random subject sets from a small token alphabet with `*` and `>`
- checks: every input subject is covered by exactly one output subject; no output covers another; the output is
  sorted and unique
- the run records its seed and check count.

The ring buffer's law ("never let an older sequence replace a newer", `:1062-1075`) keeps the pin's example tests
(`:349-377`). Start/Stop histories are the guard's (`internal/lifecycleguard/guard_test.go`) plus the suite; examples
suffice.

**Sensitivity** (task 2.3; unit mutants through `task mutate:check`, integration ones by hand):

| Mutant | Caught by |
| --- | --- |
| run context derived from `Start`'s | suite `StartContextEndStopsNothing`; `TestMessageLoggerKeepsCapturingAfterStartContextEnds` |
| subscriptions made under the Start context | `TestMessageLoggerKeepsCapturingAfterStartContextEnds` |
| a failed `Stop` forgets its subscriptions (the pin's `:922-928`) | `TestMessageLoggerFailedStopIsRetried` (or recorded uncovered, D2) |
| run context cancelled before the drain | `TestMessageLoggerStopDrainsInFlightMessage` |
| `Stop` not ending watch streams | `TestMessageLoggerStopEndsOpenWatchStreams` |
| retry not joined by `Stop` | `TestMessageLoggerRetriesSubscribeAfterReconnect` (`-race`, joined goroutine) |
| `RegisterAll` not registering the logger's spec | `TestOpenAPIDocumentResolvesEverySchemaRef` |
| a covered subject also subscribed | `TestPropCoveredSubjectsSubscribedOnce`; `TestMessageLoggerCapturesCoveredSubjectOnce` |
| a malformed subject degraded instead of refused | `TestMessageLoggerRefusesMalformedSubject` |
| a nil payload registry accepted | `TestMessageLoggerConstructionRefusals` |
| the watch filter checked by the pin's grammar | `TestKVWatchRejectsInvalidFilter` (a space and `a.>` cases) |
| the 404 branch removed | `TestKVQueryMissingBucketIs404AndCreatesNothing` |
| `errors.Is` replaced by the pin's text test | equivalent on every real error measured; recorded as a survivor |
| non-JSON payload stored raw (Q1(a)) | `TestEntriesServeNonJSONPayload` |
| trace query returning an overwritten entry | the pin's `:379-415`, adapted |

## D8. Ledger, text, ADRs, coverage, issues

- **Ledger:** the `service` row edited in place.
  - `consumer_purpose`: PR 4, with the semteams readers in the addendum.
  - `contract`: the D9 requirements; pin ADR-093 (decoding is observability evidence, the logger an observer), ADR-068
    D1 (readers never create a bucket), ADR-081 (not done); `RegisterAll` registers the logger and its spec.
  - kept names: D1. `configurable.go` and its three names leave the row.
  - `known_risks`: one watcher per watch stream; the query's glob vs the watch's filter.
- **Text:** `service/README.md` and `service/doc.go` gain the logger: enabling it, its keys and refusals, routes,
  the off-by-default gate, what each `/stats` key means, and the SSE events.
- **ADRs:** ADR-113 applied (D2), changing pin ADR-095's final failed Stop as PR 3 did. ADR-108 Decision 2 (guard).
  ADR-110: the logger writes no entity. No ADR is drafted.
- **Guidance:** none at the pin (`sweep.md` § 5).
- **Coverage:** `service merged` floor (`scripts/cover-check.sh:46`) holds.
- **Issues:** retired none. #196, #195 and #280 stay (`sweep.md` § 4). To file: each Q1 item the owner chooses to
  carry; no other defect is left unfixed.

## D9. Spec deltas (written after acceptance; `skip_specs` removed then)

- **`service-lifecycle`, ADDED:**
  - "The message logger's Start bounds Start and Stop is the only stop" (D2's bullets as scenarios, each naming its
    test)
  - "The message logger subscribes to a fixed subject set, covered subjects once" (D3)
  - "A malformed monitored subject fails the logger's Start" (D3)
  - "The message logger's construction refuses an incomplete composition" (D1).
- **`operator-surface`, ADDED:**
  - "The message logger's routes are served under /message-logger" (D4: routes, 400/404/405/503, `RegisterAll`
    registers its spec, importing registers nothing)
  - "The KV watch streams changes until the client leaves or the logger stops" (event sequence, operation values per
    Q1(b), filter validation, drops logged).

  No MODIFIED requirement: "/openapi.json documents the mounted routes" already covers the paths.

## Owner questions

- **Q1. The logger's routes give five wrong answers at the pin. Fix them in this port, or carry them as they are and
  file an issue for each?** Recommendation: fix all five. Each is small and makes a route say what its documentation
  already promises. Fix (a) even if you choose to carry the rest, because today it breaks the whole page.
  - (a) A single message that is not JSON makes the entries and trace pages fail with an error for every entry until
    it ages out (`sweep.md` M14). Fix: store such a message as a JSON string, as the watch already does.
  - (b) The watch calls a key's first write "update" unless it is the bucket's very first write, because NATS numbers
    writes across the whole bucket (M18, probed). It shows a purge as "unknown". Fix: report what NATS records,
    `put`, `delete` or `purge`. semteams' UI does not read this field; its dev script prints it.
  - (c) The `subject` filter on the entries page is documented as a NATS pattern but works as a text glob, so
    `agent.>` matches nothing (M20). Fix: NATS matching (D3's function).
  - (d) `invalid_messages` in the stats is always 0, and `valid_messages` counts everything (M12). Fix:
    `valid_messages` counts messages decoded as typed envelopes, `invalid_messages` the rest.
  - (e) Watching a bucket that does not exist answers "OK" and then an error event, where the documentation says 404
    (M20, M21). Fix: look the bucket up before starting the stream: 404 if missing, 500 for other failures.
- **Q2. (#280 item 2) Keep `component.Declaration`'s two parallel lists and state their alignment (recommended), or
  file a separate `component` change that joins them into one list of pairs?** D5 lists every reader each choice
  touches.

ADR-113 is applied, not asked. D1's drops follow the surface audit and PR 1's D1 plan. D3's malformed-subject refusal
applies the owner's PR 3 Q-B ruling by analogy (that ruling was about components); please confirm the analogy holds
for this logger.
