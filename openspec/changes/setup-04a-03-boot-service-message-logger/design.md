# Design: setup-04a-03-boot-service-message-logger

base: `265235183bf517121883c4b957fb37356c901835` (draft PR #367); revised after review round 1 at `8286432` and after
the Codex design review at `5e02cd4` (PR #367 comment 6343581103: D3 validation, D4 watch overflow). Inputs:
`sweep.md` (this change; M-ids), the whole-package inventory
(`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, S-ids), and PR 3's archived design
(`openspec/changes/archive/2026-10-22-setup-04a-03-boot-service-component-manager/design.md`), whose shape this copies.
Pin lines are SemStreams `8b99efe9`, `service/`; `:n` is `message_logger.go`. One lifecycle owner, 2,245 non-test pin
lines (under the ~3,000 bound, #159).

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
    `components/board/TaskTrace.svelte:66`) and its end-to-end specs (Q1(c))
  - semteams reads `stats`, `trace/{id}`, `kv/{bucket}` and `kv/{bucket}/watch` from `taskfiles/dev.yml:140-181`.

  semsource, semconnect, semboids: none.
- **Who mounts it.** `serviceNameToPrefix` keeps `message-logger` (`service/service_manager_http.go:380-381`); the
  service is not materialized by default (`service/service_configs.go:85-89`).
- **Open pull requests and issues:** `sweep.md` § 4 (#367 this claim; #358 no overlap, either order; #368 retired
  here, D3).
- **A second package.** D3 changes `internal/flowgraph` (one function added, one grammar line corrected, about ten
  production lines): the logger's boot refusal (Q3) reads that grammar and is wrong without the correction. A required
  correction the accepted Q3 commitment depends on, not a port; the one-package bound (#159) counts pin lines, and
  this adds none.

## D1. Files and surface (surface audit, #190)

- **Carried, adapted:** the three files, module path rewritten, § 2.4 renames (sweep § 1).
- **Dropped, no reader:**
  - `GetMessages` (M9)
  - `ConfigSchema` (M8), and with it `ConfigSchema`, `PropertySchema`, `NewConfigSchema`: `service/configurable.go`
    and `service/configurable_test.go` are deleted, closing PR 1's D1 plan
  - the reconciliation of a changing subject set (M1)
  - the test-swap seams `subscribe`, `retryAfter`, `handleKVQueryWith`, `kvBucketProvider`, the two S10 interfaces
    (M2; why `kvBucketProvider` changed disposition is in M2)
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
  - `Start` computes the subject set and validates every subject in it (D3) before it starts the base service or
    subscribes anything, so a refused subject needs no NATS and leaves nothing held. Then it starts the base service
    and subscribes to each subscribed subject in sorted order.
  - The subscriptions are made under the one run context `Start` creates, kept only as a cancel (graph-ingest
    precedent, `processor/graph-ingest/component.go:982`, `:1024-1027`). natsclient makes that context the parent of
    every message's context (`natsclient/client.go:1036-1047`), so the Start context must not be it (M4).
  - `Start` checks its own context before each subscribe. If it has ended, `Start` returns that error and holds
    nothing: the guard rolls back through the cleanup. A subscribe already in progress runs to its own end, which the
    Start context does not shorten: natsclient ends its wait for the server's answer at `DefaultRequestTimeout`, but
    a write held on the connection can keep the call "a minute or more" (`natsclient/client.go:1003-1016`). How long
    `Start` can run past its context's end is not promised.
  - Proof: suite `FailedStartHoldsNothing`. The mid-start cancel is review only: no test can end the context between
    two subscribes without a hook; `notes/2.3.md` records it.
- **Ending the Start context after `Start` returned stops nothing.** A message published afterwards is stored and
  counted. Proof: suite `StartContextEndStopsNothing`; `TestMessageLoggerKeepsCapturingAfterStartContextEnds` [U-c]
  (the probe's second run, inverted).
- **Retry, declared.** A subscribe that fails, with any error, leaves the logger running; a malformed subject never
  reaches a subscribe (D3).
  The failed subjects are retried every 100 ms under the run context. While the retry holds a subject that failed to
  subscribe (the retry's pending set, empty until a subscribe has failed, so the base service's first health check,
  which runs before the subscribes, `service/base.go:209`, `:290-295`, sees a healthy logger on a normal boot):
  - `/stats` reports `subject_reconciliation_degraded` true with `_error`, and an Error line is logged, as at the pin;
  - new here, the logger's health check (`SetHealthCheck`, `service/base.go:252`, as `service/metrics.go:105` and
    `service/component_manager.go:259` use it) returns an error naming the pending subjects, so `IsHealthy` turns
    false at the next check and `/readyz` answers not ready. Checks run every 30 s (`service/base.go:19`, `:290-336`),
    so turning unhealthy after a failed subscribe and healthy again after it succeeds each lag by up to 30 s
    (`service/service_manager_http.go:507`). At the pin the logger had no health check and no NATS check (its
    constructor passes no `WithNATS`, `:54-60`), so it stayed healthy while capturing nothing.

  Options: (a) health check, chosen; (b) a metric: new exported surface with no reader; (c) inherited under Bounded
  port scope: `/readyz` says ready while the logger is unsubscribed after a NATS restart during boot.

  Proof: `TestMessageLoggerRetriesSubscribeAfterReconnect` [U-c, own server: `// Own server: stops and restarts the
  server` with `natsfixture.Fixture.Stop`/`Start`] asserts, during the outage, `subject_reconciliation_degraded` true
  and, through the production path, `ml.performHealthCheck()` (`service/base.go:307`) then `IsHealthy()` false; after
  the restart, both cleared (awaited under a bounded context; `performHealthCheck` then `IsHealthy()` true) and a
  published message stored. A logger whose `SetHealthCheck` wiring is removed fails the outage assertion.
- **Stop.**
  1. `Stop` ends the retry and waits for it. A retry's `Subscribe` in progress runs under the run context, which
     only step 3 ends, so `Stop` waits for that call to return (bounded as in Startup work, not promised).
  2. It drains every subscription (`natsclient.Subscription.Drain`, which returns once the in-flight callback has
     returned, `natsclient/client.go:957-964`), so a message being handled when `Stop` begins is stored.
  3. Then it ends the run context, ends every open watch stream (D4) and waits for each stream's KV watcher to stop,
     and stops the base service.
  4. It returns nil only once all are released. If its context ends first at any step, it returns that error (M3).

  Proofs:
  - `TestMessageLoggerStopDrainsInFlightMessage` [U-c]. The composition's `slog` handler (U-a) holds the first
    "Message sample" record until released; a production dependency, not a seam. The pin logs that record after
    `storeEntry` (`:1001`, `:1027`), so message 1 is already stored. The test publishes message 2, flushes the
    publishing connection, runs `Stop` on a goroutine, releases, and asserts `Stop` returned nil and both messages
    are stored. Message 2 reaching the handler during the drain is measured, not promised: natsclient's `Drain`
    promises only the in-flight callback; the drain probe (`sweep.md` § 3) saw messages published and flushed before
    `Drain` handled in 200 of 200 runs at nats.go v1.54.0 / nats-server v2.15.1. A library change there fails this
    test loudly.
  - `TestMessageLoggerStopEndsOpenWatchStreams` [U-f]: with one watch open, `Stop` under a 10 s bound returns nil,
    the stream's body reaches EOF before the HTTP client's own 10 s deadline, and `Observe` reports no open stream.
  - `TestMessageLoggerIgnoresPublishesAfterStop` [U-c].
- **A failed `Stop` is retried (M3).** If the context ends first, or a drain fails, `Stop` returns that error. Every
  subscription not yet drained, the retry and open streams are left for the next `Stop`, which runs the same
  cleanup; natsclient rejoins a begun drain rather than starting another (`natsclient/client.go:957-964`). A `Stop`
  after one that returned nil returns nil and calls nothing; a `Stop` while another runs is refused as transient.
  Proof: `TestMessageLoggerFailedStopIsRetried` [U-c]:
  - with message 1 held, the first `Stop` runs under a 200 ms deadline and returns `context.DeadlineExceeded` (the
    held callback keeps the drain from finishing, so the deadline cannot fail a correct implementation);
  - the check: a second `Stop`, on a goroutine, has not returned within a 200 ms observation window while the
    callback is held. Under the M3 mutant it returns nil at once. The window is listed here as `docs/testing.md`
    § Porting a test from SemStreams requires; its expiry cannot fail a correct implementation (drain probe: a second
    `Drain` returned while the callback was held in 0 of 200 runs);
  - after release, the second `Stop` returns nil and a third returns nil.

  "A later publish is not captured" is not the check: a begun native drain finishes on its own, so it holds under
  the mutant too (probe: a publish after the drain began was handled in 0 of 200 runs). If the first `Stop` is still
  before its drain when its deadline ends, the mutant passes that run; a pass under the mutant is reported
  inconclusive, not as a survivor or a detection.
- **Reads during and after Stop.** `/entries`, `/stats`, `/trace` answer from memory at any time, holding no lock a
  `Stop` needs. `/subjects` answers `[]` after `Stop` (`:928-931`). A watch requested after `Stop` began answers 503
  and opens nothing. Proof: `TestMessageLoggerRoutesAfterStop` [U-d].
- **Context rules:** the logger keeps no `context.Context`; it holds one private cancel. The watch stream learns of
  `Stop` through a value the logger closes or a run-context child created per request and joined, not through a
  stored context. Locks, channels and join order are the developer's (failing-first under `-race`).

## D3. Subjects: one fixed set, validated first, covered subjects once

- **The set** is computed once in `Start`, as at the pin (`:341-353`):
  - the explicit subjects, plus, with `"*"`, every NATS, NATS-request and JetStream port subject of every admitted
    declaration, skipping `*`, `>` and `_INBOX` (`:355-388`)
  - read through `component.Registry.Snapshots()`, the pairing in D5.

  The `Snapshots` doc comment (`component/registry.go:683-687`, "It exists for the component manager") gains the
  logger as its second framework reader (one sentence).
- **Validation first (M11, Q3, #368).**
  - `Start` checks every subject of the set with `flowgraph.ValidSubject` before duplicates or covered subjects are
    removed, before the base service starts and before any subscribe. The first subject that fails refuses `Start`
    with an invalid-class error (`errs.WrapInvalid`, as the logger's other refusals) whose text names the subject and
    its source: `monitor_subjects`, or the component and port that declare it. Nothing is held and no NATS call is
    made, so the refusal is the same connected or not.
  - Why before reduction and subscribe (Codex, measured): with `["a.>", "a.foo bar"]` the cover check removes the
    second subject before anything could refuse it; while disconnected, natsclient returns `ErrNotConnected` before
    nats.go sees the subject (`natsclient/client.go:1074-1092`), which D2 retries; and nats.go refuses at subscribe
    only whitespace and empty tokens (`badSubject`, `nats.go:5074-5086` at v1.54.0). `a.>.b` passes nats.go: the
    server answers `Invalid Subject` and closes the connection (subject probe, `sweep.md` § 3; with natsclient's
    reconnects on, what follows is not probed).
  - **One grammar, one owner: `internal/flowgraph`.** `ValidSubject(subject string) bool` (new, exported inside an
    `internal` package) is the per-argument check `SubjectCovers` and `SubjectsOverlap` already make
    (`validSubjectTokens`, `internal/flowgraph/flowgraph.go:340-352`), so validation, the cover reduction and the
    entries filter (Q1(c)) read one grammar. Its reader: the logger's `Start` (production); the two helpers keep
    calling the unexported check it wraps. `service` already reaches `flowgraph` through `composition`. Grammar: one
    or more `.`-separated tokens, none empty; `*` and `>` only as whole tokens; `>` only last; no space, tab, line
    feed, carriage return or form feed in any token (nats-server's set, `isValidSubject`,
    `server/sublist.go:1214-1250` at v2.15.1).
  - **#368 fixed here.** `validSubjectTokens` gains that whitespace check, so `SubjectCovers("a.>", "a.foo bar")` and
    `SubjectsOverlap("a.>", "a.foo bar")` return false, as their doc comments already promise. Callers (gopls
    `references` on `flowgraph.go:279`, `:321`, `:340`): `composition/analyze.go:140` (`reservedCaptureFindings`), where
    a stream filter holding whitespace no longer reports a reserved-subject capture (it overlaps nothing), and
    `composition/analyze.go:198` (`explicitStreamCovers`), where such a subject is no longer covered, so its
    `stream_requirement` finding is no longer suppressed (fail closed); tests
    `internal/flowgraph/subject_cover_test.go` and `composition/reserved_test.go:161-165`, whose alphabets hold no
    whitespace, unchanged. `matchNATSPattern` (edge derivation) does not read the grammar and is untouched. The pull
    request body carries `Closes #368`.
  - **Stricter than NATS, declared; the owner kept it (#9 comment 6343904612).** A token mixing a wildcard with other
    text (`a*`, `b*.c`) is a literal to NATS (probe: subscribed, connection healthy) and malformed here, as composition
    already treats it; a form feed passes the wire (probe: subscribed, no error) and is refused here. A subject NATS
    accepts but this grammar refuses fails boot, named; that is the fail-closed side. semteams' configured subjects
    (addendum) all pass.
  - Since every subject reaching a subscribe has passed a grammar stricter than nats.go's, D2's retry treats every
    subscribe error alike; a later nats.go refusing more would show as a retried subject and an unhealthy logger
    (D2), not a silent one.
  - Options: (a) `flowgraph.ValidSubject`, chosen; (b) `SubjectCovers(s, s)` as the check, no new name, but it reads
    as a trick and makes reflexivity a promise of the cover helper; (c) a logger-owned validator, a second grammar
    beside flowgraph's (one home per interpreted fact); (d) the library's refusal as the check (the previous draft):
    misses the three paths above. `config/config.go:286` (`isValidNATSSubjectPart`) answers a different question,
    an identity part with no wildcards, and is not adopted.
  - Proofs: `TestMessageLoggerRefusesMalformedSubject` [U-b], over `newUnconnectedClient`, so it is the disconnected
    case: `a..b`, `a.foo bar`, `a.>.b`, `a*` and `""` each refuse `Start`, the error names the subject and
    `monitor_subjects`, and the logger holds nothing (base service not started, no subscription held);
    `TestMessageLoggerRefusesMalformedSubjectCoveredByBroader` [U-b]: `["a.>", "a.foo bar"]` refused, naming
    `a.foo bar`; `TestValidSubject` and `TestSubjectHelpersRefuseWhitespace` (#368's regression, all five whitespace
    bytes, both helpers) [U-h].
  - Adoption sweep (a validator other entry points should read; one tracking issue, not this PR's work): component
    port subjects are checked only for emptiness (`component/port_codec.go:322`, `:333`, `:356-359`); stream subjects
    (`config/streams.go:28`) by no subject grammar found in this search.
- **Covered subjects (M10).**
  - Replaces the three agentic pairs (`:391-395`, `:652-656`): after duplicates are removed, a desired subject that
    another desired subject covers is not subscribed, and each omission is reported in `/stats` `subject_overlaps` as
    `{broader, covered, "covered subscription omitted"}`.
  - Runs only on a validated set. "Covers" is `flowgraph.SubjectCovers(filter, pattern)`
    (`internal/flowgraph/flowgraph.go:271-313`): directional, false for malformed input, already called by
    `composition/analyze.go:198`. `service` imports `composition`,
    which imports `flowgraph`, so no cycle; no new matcher is written. The entries filter uses it too (Q1(c)).
  - Law: every desired subject is covered by at least one subscribed subject, and no subscribed subject covers
    another. Two subscribed subjects can still overlap without either covering the other (`a.b.*` and `*.b.c` share
    `a.b.c`); NATS delivers such a message to both subscriptions, so it is captured twice, as at the pin. The README
    says so.
  - Proof: `TestPropCoveredSubjectsSubscribedOnce` (generated, `pgregory.net/rapid`, U-b) and
    `TestMessageLoggerCapturesCoveredSubjectOnce` [U-c].
  - Options: (a) generic, chosen; (b) carry the pairs (a consumer's subjects in the framework, other covers captured
    twice); (c) no cover handling (every covered subject captured twice).

## D4. HTTP surface (`operator-surface`; 03B D14/D15 row "Keep", `docs/contract.md:139`)

- **Routes.** Served after boot under `/message-logger/`:
  - `entries` (`limit`: default 100; above 10000 clamped to 10000; zero, negative or not a number gives 100;
    `message_logger_http.go:251-258`. `subject` filter, Q1(c))
  - `stats`
  - `subjects`
  - `GET trace/{traceID}` (32 hex, else 400)
  - `kv/{bucket}` (`pattern` key glob; `limit` as `entries` with 1000 for 10000, `:422-428`; `status`)
  - `GET kv/{bucket}/watch` (SSE: `connected`, then `kv_change` events, the initial values ending with an
    `initial_sync_complete` operation, `retry: 5000`).

  Every route answers `GET` only; any other method gets 405.
- **OpenAPI without an import-time registration (M15).** `RegisterAll` registers the logger's spec, as PR 3 D5 option
  (a). Proof: `TestOpenAPIDocumentResolvesEverySchemaRef`, extended so the composition enables `message-logger` with
  `"*"` and an empty registry (no NATS needed).
- **Error kinds (S7).**
  - `errors.Is(err, jetstream.ErrBucketNotFound)` → 404 and `errors.Is(err, jetstream.ErrNoKeysFound)` → empty
    result. Probe at nats.go v1.54.0, `sweep.md` § 3.
  - Proof: `TestKVQueryMissingBucketIs404AndCreatesNothing`, `TestKVQueryEmptyBucketReturnsNoEntries` [U-e].
- **KV query reads (M23, M24).** Keys are sorted before `limit` applies (`AGENTS.md` § 2). nats.go v1.54.0 sorts
  them already (`jetstream/kv.go:1420`), but its doc promises only dedup (`kv.go:177`), so the order is not promised
  and the logger sorts. A key whose `Get` answers `jetstream.ErrKeyNotFound` was deleted or purged after the listing
  (probe: both give that error) and is left out, because it no longer exists. Any other `Get` error answers 500
  naming the key, where the pin logged a Warn and returned a short list (`message_logger_http.go:524-527`). Proof:
  `TestKVQuerySortsKeysBeforeLimit` [U-e]; the two `Get` branches need a change between two calls and are review
  only (`notes/2.3.md`).
- **One spelling of the bucket and its name check (M16).** Both KV routes take `{bucket}` from the route
  (`r.PathValue`), not the hard-coded prefix. One validator (empty, `.`, `..`, `/`, `\`, `*`, `>` → 400) serves both.
  Proof: `TestKVRoutesRejectMalformedBucket` [U-d].
- **The watch filter (M17).** Empty or `*` watches every key, as at the pin
  (`message_logger_kv_watch.go:216-220`). Any other `pattern` passes `natsclient.ValidateKVWildcardFilter`, else 400
  naming it. This accepts a final `>`, which the pin refused, and refuses a space, which it accepted. Proof:
  `TestKVWatchRejectsInvalidFilter` [U-f].
- **The trace query (M13).** The trace index is dropped. `GetEntriesByTrace` scans the buffer (at most 100,000
  entries) and returns the trace's entries still held, oldest first. Memory stays within `max_entries`. Proof: the
  pin's `message_logger_test.go:268-347` and `:379-415`, adapted [U-b].
- **Text (M20).** The dev/test-mode claims, the 403, `Last-Event-ID` and "reconnection" are rewritten to what the code
  does. The README states the gate: the service is off unless configured. The watch keeps one KV watcher per stream
  (pin ADR-081 not done; M22).
- **A watch that falls behind ends (M26).** Each stream buffers up to 100 events between its KV watcher and the
  client (`message_logger_kv_watch.go:226`). At the pin a full buffer dropped the event with a Warn (`:293-298`) and
  went on, so a slow client during the initial replay could get part of the bucket and then `initial_sync_complete`,
  which the code says means "all existing keys have been sent" (`:254-264`). Here the first event that does not fit
  ends the stream:
  - the watcher is stopped, the events already buffered are sent, then one `error` event,
    `{"error":"watch fell behind: events were dropped"}`, and the response ends;
  - no `kv_change` or `initial_sync_complete` is sent after a dropped event;
  - the Warn line stays (bucket, key, revision). The `error` event is the one the pin already sends when its watcher
    closes (`:337-340`); a client that reconnects after the stream's `retry: 5000` starts a new initial replay (how
    a client reconnects is the client's, not promised here).

  Options: (a) end the stream, chosen; (b) keep dropping, add a metric and mark the completion event incomplete: a
  metric with no reader and a wire field every client must learn, while a client that ignores it still reads a false
  "complete"; (c) block instead of dropping: moves the backlog into nats.go's watcher, whose behaviour behind a slow
  reader is not probed at v1.54.0 and not promised; (d) retain (the pin): a false completion claim, the silent-loss
  admission gate.

  Proof: `TestKVWatchFallingBehindEndsStream` [U-f]. 150 keys are put before the watch opens. The request goes
  through the mux (`RegisterHTTPHandlers`) to a test `http.ResponseWriter` and `http.Flusher` whose `Write` blocks
  until the test releases it: the client that does not read, outside the logger, so no production hook. The stream
  is held at its first write, so at most 101 of the 150 initial values fit (100 buffered, at most one in the
  stream's hands) and a later one is dropped, whatever the scheduling. The test waits (10 s bound) for the Warn
  record on U-a's `slog` handler, releases the writer, and waits (10 s bound, then cancels the request context) for
  `ServeHTTP` to return. It asserts: the last event is `error` with the text above; no `initial_sync_complete`
  anywhere; fewer than 150 `kv_change` events. Under the pin's drop-and-continue mutant the stream sends
  `initial_sync_complete`, which fails the assertion, not the bound.
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
  `registry_boot_admission_test.go`); `internal/flowgraph/declarations_test.go` (the `declaration` helper, `:23`) and
  `internal/flowgraph/flowgraph_validation_test.go:352`, which builds one through that helper. `flowgraph_test.go`
  builds `Discoverable` mocks (`:460-467`), not declarations. None in `composition`. Consumers: no use (consumer
  tarballs, `grep -rn 'InputDeclarationFacts|component.Declaration'`: 0).

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
   refuses, naming the field. A typo'd subject fails boot, naming it, whether NATS is up or not (D3, Q3). A subject
   NATS cannot serve yet makes the logger unhealthy until it is subscribed (D2). A watch client too slow for the
   bucket gets an `error` event and a closed stream, never a false "initial sync complete" (D4).
3. **Where they find out:** a boot error, for all three. Today (iii) is nowhere: every message is silently "raw".
4. **What they should have to know:** (i) only. (ii) and (iii) are the composition the root already builds (semteams
   `cmd/semteams/main.go:208`, `:789`).

Intent check: no boundary moves.

## D7. Tests

| Pin test file (lines) | Here |
| --- | --- |
| `message_logger_test.go` (415) | U-b; `:12-55`, `:67-75` dropped (D1); `:76-96` replaced by the helper (no fixed address); trace tests use `storeEntry` only |
| `message_logger_registry_test.go` (276) | behaviours moved to U-c on the shared server; fake subscriber `:193-276` dropped (M2); `:80-133` flipped (M3) |
| `message_logger_lifecycle_test.go` (378) | `:70-100` → the suite; `:102-145` → `TestMessageLoggerStopDrainsInFlightMessage`; `:147-178` → `TestMessageLoggerFailedStopIsRetried` and the retry test; `:180-378` dropped with M1 (they test the obsolete-drain and reconcile gate) |
| `message_logger_subscription_integration_test.go` (92) | U-c; `:62-63` flipped (a failed Stop is retried) |
| `message_logger_http_kv_query_test.go` (223) | `:126-147` carried (U-d); `:98-125`, `:149-205` moved to real NATS (U-e); `:206-223` dropped (asserts the injected provider's context: mechanism, M2) |
| `message_logger_kv_acquisition_integration_test.go` (145) | U-e, through `fixtureClient`; `kvcatalog.EnsureCatalogBucket` |
| `message_logger_kv_watch_test.go` (92) | U-f; `TestIsValidWatchPattern` replaced by the shared validator's cases; `TestDetectKVOperation` per Q1(b) |
| `message_logger_user_response_test.go` (49) | U-g: a test payload registered in a local `payloadregistry` instead of `agentic.UserResponse` (ADR-093 kept) |
| `message_logger_census_test.go` (556) | not carried (imports outside the port set) |

**Units** (one or two files each; brief cites only the D-sections named). Order: U-a first, because every other
unit builds its logger through U-a's helper; then U-b to U-g in any order, in parallel. Every unit builds the logger
the one production way, `NewMessageLoggerService`, through U-a's helper; none calls an unexported constructor.

- **U-a** `service/message_logger_helpers_test.go` (no build tag), `service/message_logger_suite_integration_test.go`
  (new). D1, D2.
  - Helper `newTestMessageLogger(t, client *natsclient.Client, config string, logger *slog.Logger) *MessageLogger`:
    `NewMessageLoggerService(json.RawMessage(config), &Dependencies{NATSClient, Logger, ComponentRegistry:
    component.NewRegistry(), PayloadRegistry: payloadregistry.New()})`, then the type assertion.
  - A `slog.Handler`, enabled at Debug (the record is logged at Debug, `:1027`), that holds the first "Message
    sample" record until the test releases it (D2's proofs), and signals when a record with a given message arrives
    (D4's watch overflow).
  - Suite adapter: `lifecycletest.Run`, `Owner`, `Observation`, `Promise`; the logger over `fixtureClient(t)`
    (`service/fixture_client_integration_test.go:48`). `Observe` reports base status, held subscriptions, whether a
    retry is running and the open streams (unexported reads in the package; no guard-state field).
- **U-b** `service/message_logger_test.go`. D1, D3, D4 (trace).
  - Logger: the helper over `newUnconnectedClient(t)` (`service/service_manager_test.go:817`), never started.
  - Pin `:98-415` adapted: sampling `:98-148`, `containsWildcard` `:150-189`, `uniqueStrings` `:191-196`, overlap
    `:198-220` rewritten for the cover rule, config `:222-266`, trace and ring `:268-415`.
  - Entries are stored through `handleMessage(t.Context(), subject, data)` (the subscription callback's body) and,
    for the ring cases, `storeEntry`; read through `GetLogEntries`, `GetEntriesByTrace`, `GetStatistics`.
  - New: construction refusals (D1, through `NewMessageLoggerService` directly);
    `TestMessageLoggerRefusesMalformedSubject` and `TestMessageLoggerRefusesMalformedSubjectCoveredByBroader` (D3,
    `Start` on this never-connected client); `TestPropCoveredSubjectsSubscribedOnce` (on the unexported function that
    turns a validated subject set into the subscribed set and the omissions).
- **U-c** `service/message_logger_subscription_integration_test.go`. D2, D3.
  - Logger: the helper over `fixtureClient(t)`, `Start` and `Stop` under bounded contexts.
  - Pin `subscription_integration_test.go:17-92` and `registry_test.go:19-191` behaviours on the shared server.
  - New tests: Start-context end, in-flight drain, failed Stop retried, publishes after Stop, covered subject once,
    retry after reconnect (own server, `natsfixture.New`, `Fixture.Stop`, `Fixture.Start`).
  - Calls: `natsclient.Client.Publish`, `GetConnection().Flush()` on the publishing client, `GetStatistics`,
    `GetLogEntries`, the unexported health check.
- **U-d** `service/message_logger_http_test.go` (new). D4, Q1.
  - Logger: the helper over `newUnconnectedClient(t)` with `"*"` and an empty registry; entries stored as in U-b.
  - Routes through `RegisterHTTPHandlers("/message-logger", mux)` and `httptest`: entries (limit clamp, filter),
    stats, subjects, trace (400 on bad id), 405, bucket validation, routes after Stop.
  - Plus `recordMatchesStatus` (pin `http_kv_query_test.go:126-147`) and `TestEntriesServeNonJSONPayload` (Q1(a)).
- **U-e** `service/message_logger_kv_integration_test.go`. D4.
  - Logger: the helper over `fixtureClient(t)`; buckets made with `Client.CreateKeyValueBucket` and
    `kvcatalog.EnsureCatalogBucket`; routes through `RegisterHTTPHandlers` and `httptest`.
  - Pin `kv_acquisition_integration_test.go:18-145` and `http_kv_query_test.go:98-125`, `:149-205`: missing bucket 404
    and nothing created (`Client.ListKeyValueBuckets`), empty bucket, failed-record round trip, lookup failure 500
    (a second logger over `newUnconnectedClient`, pin `:187-205`); new: `TestKVQuerySortsKeysBeforeLimit`.
- **U-f** `service/message_logger_kv_watch_test.go`, `service/message_logger_kv_watch_integration_test.go`. D2
  (streams), D4, Q1(b), Q1(e).
  - Unit file: the bucket validator (pin `kv_watch_test.go:9-44` cases plus `*`, `>`, `/`), the filter cases (pin
    `:45-77`, replaced by `natsclient.ValidateKVWildcardFilter`'s grammar), the operation mapping (pin `:78-92`, per
    Q1(b)).
  - Integration file: the helper over `fixtureClient(t)`, `Start`; `httptest.NewServer` over the mux; event sequence
    `connected` → initial values → `initial_sync_complete` → `kv_change`; filter refusal; `Stop` ends streams; 503
    after `Stop`; missing bucket 404 (Q1(e)); `TestKVWatchFallingBehindEndsStream` (D4, the blocking test writer
    beside the test).
- **U-g** `service/message_logger_decode_test.go`. D1 (payload registry), ADR-093.
  - Logger: the helper over `newUnconnectedClient(t)` with its own `payloadregistry.New()` holding one test payload;
    pin `user_response_test.go:16-49` adapted: `handleMessage` with a `message.BaseMessage` of that payload, then
    `GetLogEntries` shows its type.
- **U-h** `internal/flowgraph/flowgraph.go`, `internal/flowgraph/subject_cover_test.go`. D3 (grammar, #368). Runs
  first with U-a: U-b's malformed-subject tests need `ValidSubject`.
  - `TestValidSubject`: valid (`a`, `a.b`, `*`, `>`, `a.*.c`, `a.>`) and malformed (`""`, `a..b`, `.a`, `a.`,
    `a.>.b`, `a*`, `b*.c`, and each of space, tab, line feed, carriage return, form feed inside a token).
  - `TestSubjectHelpersRefuseWhitespace`: #368's reproduction, widened to form feed: both helpers return false for
    `("a.>", "a.foo<ws>bar")` with each whitespace byte. The existing cover cases stay.
- **Integration with PR 3's test:** `TestOpenAPIDocumentResolvesEverySchemaRef` gains the logger (D4); one line in its
  composition.

Harness rules as PR 3's D7: no `time.Sleep`, `Skip`, fixed port, or `t.Parallel` in an integration file; cleanup
stops under `context.WithTimeout(context.WithoutCancel(t.Context()), ...)`; every NATS connection through
`fixtureClient` (#195) except the own-server retry test. Real-clock windows, listed as `docs/testing.md` requires:
the two 200 ms windows of `TestMessageLoggerFailedStopIsRetried` (D2); every other wait is on a signal, bounded at
10 s or the test's context.

**Generated checks.** The covered-subject rule is a stated law (D3), so it gets a rapid property:

- generator: random valid subject sets from a small token alphabet with `*` and `>` (the reduction's input is a
  validated set, D3)
- checks: every input subject is covered by at least one output subject; no output covers another; the output is a
  sorted, duplicate-free subset of the input. Oracle: `flowgraph.SubjectCovers`, tested on its own
  (`internal/flowgraph/subject_cover_test.go`)
- the run records its seed and check count.

The ring buffer's law ("never let an older sequence replace a newer", `:1062-1075`) keeps the pin's example tests
(`message_logger_test.go:349-377`). Start/Stop histories are the guard's (`internal/lifecycleguard/guard_test.go`)
plus the suite; examples suffice.

**Sensitivity** (task 2.3; unit mutants through `task mutate:check`, integration ones by hand). A row counts as a
detection only when the test fails under the mutant; a hang is not a detection, so each Stop-path test runs under a
bounded context and asserts the result.

| Mutant | Caught by |
| --- | --- |
| run context derived from `Start`'s | suite `StartContextEndStopsNothing`; `TestMessageLoggerKeepsCapturingAfterStartContextEnds` |
| subscriptions made under the Start context | `TestMessageLoggerKeepsCapturingAfterStartContextEnds` |
| a failed `Stop` forgets its subscriptions (the pin's `:922-928`) | `TestMessageLoggerFailedStopIsRetried`: the second `Stop` returns inside the window while the callback is held (D2; a pass is inconclusive) |
| run context cancelled before the drain | `TestMessageLoggerStopDrainsInFlightMessage`: the message queued behind the held one is dropped |
| `Stop` not ending watch streams | `TestMessageLoggerStopEndsOpenWatchStreams`: `Stop` returns its bounded context's error, or the body read ends at the client's deadline |
| retry not joined by `Stop` | `TestMessageLoggerRetriesSubscribeAfterReconnect`, `Observe` right after `Stop` reports a running retry; an unjoined retry can exit before `Observe` runs, so a pass is reported inconclusive |
| no health check while a subject is unsubscribed | `TestMessageLoggerRetriesSubscribeAfterReconnect` |
| `RegisterAll` not registering the logger's spec | `TestOpenAPIDocumentResolvesEverySchemaRef` |
| a covered subject also subscribed | `TestPropCoveredSubjectsSubscribedOnce`; `TestMessageLoggerCapturesCoveredSubjectOnce` |
| validation dropped, the subscribe's `ErrBadSubject` left to refuse (the previous draft) | `TestMessageLoggerRefusesMalformedSubject`: on the never-connected client `Start` returns nil and retries |
| validation after the cover reduction | `TestMessageLoggerRefusesMalformedSubjectCoveredByBroader` |
| `validSubjectTokens` without the whitespace check (#368) | `TestSubjectHelpersRefuseWhitespace`; `TestValidSubject`; `TestMessageLoggerRefusesMalformedSubjectCoveredByBroader` |
| a full watch buffer drops and continues (the pin's `:293-298`) | `TestKVWatchFallingBehindEndsStream`: `initial_sync_complete` is sent |
| a nil payload registry accepted | `TestMessageLoggerConstructionRefusals` |
| the watch filter checked by the pin's grammar | `TestKVWatchRejectsInvalidFilter` (a space and `a.>` cases) |
| the 404 branch removed | `TestKVQueryMissingBucketIs404AndCreatesNothing` |
| the KV query's sort removed | survivor at nats.go v1.54.0, which sorts `Keys` itself; recorded as a survivor |
| `errors.Is` replaced by the pin's text test | equivalent on every real error measured; recorded as a survivor |
| non-JSON payload stored raw (Q1(a)) | `TestEntriesServeNonJSONPayload` |
| entries filter applied after the limit (Q1(c)) | `TestEntriesFilterBeforeLimit` [U-d] |
| trace query returning an overwritten entry | the pin's `message_logger_test.go:379-415`, adapted |

Uncovered, recorded in `notes/2.3.md`: the mid-start cancel (D2), the two KV `Get` branches (D4), the source naming
of a malformed subject declared by a component (D3: same loop as `monitor_subjects`, tested through the latter).

## D8. Ledger, text, ADRs, coverage, issues

- **Ledger:** the `service` row edited in place.
  - `consumer_purpose`: PR 4, with the semteams readers in the addendum.
  - `contract`: the D9 requirements; pin ADR-093 (decoding is observability evidence, the logger an observer), ADR-068
    D1 (readers never create a bucket), ADR-081 (not done); `RegisterAll` registers the logger and its spec.
  - kept names: D1. `configurable.go` and its three names leave the row.
  - `known_risks`: one watcher per watch stream; the query's glob vs the watch's filter; overlapping (not covering)
    subjects captured twice; a watch client slower than the bucket loses its stream (`error` event, then end); the
    subject grammar is stricter than NATS on a wildcard inside a token and on form feed.
  - the `internal/flowgraph` row: `contract` gains `ValidSubject` and the whitespace rule; the "a malformed subject
    overlaps nothing" line (`docs/admission-ledger.yaml:2673`) becomes true.
- **Text:** `service/README.md` and `service/doc.go` gain the logger: enabling it, its keys and refusals, routes,
  the off-by-default gate, what each `/stats` key means, the health check, and the SSE events, the overflow `error`
  among them. `internal/flowgraph/README.md` and `doc.go` gain `ValidSubject` and the grammar (one paragraph).
- **ADRs:** ADR-113 applied (D2), changing pin ADR-095's final failed Stop as PR 3 did. ADR-108 Decision 2 (guard).
  ADR-110: the logger writes no entity. No ADR is drafted.
- **Guidance:** none at the pin (`sweep.md` § 5).
- **Coverage:** `service merged` floor (`scripts/cover-check.sh:46`) holds.
- **Issues:** retires #368 (`Closes #368`; tasks 2.1, 2.2, U-h). #196, #195 and #280 stay (`sweep.md` § 4). One
  to file: the D3 adoption sweep's tracking issue (port and stream subjects read no grammar), an enumeration, not a
  defect this PR leaves. The owner ruled all five Q1 items fixed here; no other defect is left unfixed. #280 item 2
  is settled by this change (Q2), items 1 and 3 stay.

## D9. Spec deltas (written after acceptance; `skip_specs` removed then)

- **`service-lifecycle`, ADDED:**
  - "The message logger's Start bounds Start and Stop is the only stop" (D2's bullets as scenarios, each naming its
    test)
  - "The message logger is unhealthy while a monitored subject has failed to subscribe" (D2, Retry; scenarios: healthy at
    the first check of a normal boot; unhealthy within one 30 s check interval after a failed subscribe; healthy
    again within one interval after the retry succeeds)
  - "The message logger subscribes to a fixed subject set, covered subjects once" (D3, the law as stated)
  - "A malformed monitored subject fails the logger's Start" (D3, Q3; scenarios: refused while disconnected, holding
    nothing; refused when a broader subject covers it; the error names the subject and its source)
  - "The message logger's construction refuses an incomplete composition" (D1).
- **`operator-surface`, ADDED:**
  - "The message logger's routes are served under /message-logger" (D4: routes, clamped limits, 400/404/405/503,
    KV keys sorted before the limit, `RegisterAll` registers its spec, importing registers nothing)
  - "The KV watch streams changes until the client leaves, falls behind, or the logger stops" (event sequence,
    operation values per Q1(b), filter validation; scenario: a client that falls behind gets the buffered events,
    then one `error` event, then the end of the stream, and no `initial_sync_complete` after a dropped event).

  No MODIFIED requirement: "/openapi.json documents the mounted routes" already covers the paths.
- **`composition-validation`, ADDED** (the spec whose stream-cover scenarios already rest on `SubjectCovers`,
  `openspec/specs/composition-validation/spec.md:36`, `:82-86`; no spec names the grammar today, `grep -rln
  SubjectCovers openspec/specs`: none):
  - "One subject grammar decides validity, cover and overlap" (D3: the grammar as stated; scenarios: a subject with
    whitespace in a token is invalid, covers nothing and overlaps nothing, `TestSubjectHelpersRefuseWhitespace`;
    `ValidSubject` accepts exactly the subjects the grammar admits, `TestValidSubject`). The logger requirement
    above cites it.

## Owner questions

Ruled 2026-10-22 (#9 comments 6343081407 and 6343904612; #280 comment 6343082392): Q1 fix all five here; Q2
keep the two lists and document the pairing (D5 option A); Q3 fail boot, naming the subject. The questions are kept as asked.

- **Q1. Recommended: fix, in this port, the five wrong answers the logger's pages give at the pin.** Each is small and
  makes a page do what its documentation already says. Fix (a) even if you carry the rest, because one bad message
  breaks the whole page. Any you choose to carry is filed as an issue. Fix all five, or which to carry?
  - (a) A single message that is not JSON makes the entries and trace pages fail with an error for every entry until
    it ages out (`sweep.md` M14). Fix: store such a message as a JSON string, as the watch already does.
  - (b) The watch calls a key's first write "update" unless it is the bucket's very first write, because NATS numbers
    writes across the whole bucket (M18, probed). It shows a purge as "unknown". Fix: report what NATS records,
    `put`, `delete` or `purge`. semteams' UI does not read this field; its dev script prints it.
  - (c) The `subject` filter on the entries page is documented as a NATS pattern but works as a text pattern, and it
    is applied after the limit, so a filtered request returns fewer entries than exist (M20, M25). Fix: NATS matching
    with the cover check D3 uses, applied before the limit; a filter NATS cannot parse matches nothing. Cost:
    `subject=*`, which today returns everything, would return only subjects with no dot (write `>`), and `agent.*`,
    which today matches anything starting `agent.`, would match only two-part subjects (write `agent.>`). In
    semteams the UI documents the filter as a NATS pattern (`ui/src/lib/services/messageLoggerApi.ts:31`) and passes
    it through (`:42`); the trace panel sends none (`TaskTrace.svelte:66`); two end-to-end specs send
    `user.response.*` (`ui/e2e/agentic/clarification-autonomous.spec.ts:99`, `run-approval-boundary.spec.ts:85`) and
    would then see only three-part `user.response` subjects. Four other specs send `subject_prefix`, which neither
    the pin nor this port reads.
  - (d) `invalid_messages` in the stats is always 0, and `valid_messages` counts everything (M12). Fix:
    `valid_messages` counts messages decoded as typed envelopes, `invalid_messages` the rest.
  - (e) Watching a bucket that does not exist answers "OK" and then an error event, where the documentation says 404
    (M20, M21). Fix: look the bucket up before starting the stream: 404 if missing, 500 for other failures.
- **Q2. Recommended: keep a component declaration's two side-by-side lists, its ports and their facts, and write
  down that entry i of one describes entry i of the other.** The logger reads them in pairs, as the composition
  package already does. Joining them into one list would make a mismatch impossible, but it changes a public type
  read in three packages and belongs in its own pull request for the component package. This settles the second of
  the three items in the open issue about component declarations; the other two stay open. D5 lists every reader
  each choice touches. Keep them, or join them in a separate change?
- **Q3. Recommended: a subject in the logger's list that NATS cannot read (a typo such as `agent..x`) stops the
  process at boot, naming the subject.** This applies your 2026-10-21 ruling on the component manager (a component
  that cannot be built fails boot and is named) to the logger by analogy; that ruling was about components, not this
  logger. Cost: one typo keeps the whole process from starting. Today the process starts, the logger captures the
  other subjects, and it logs about ten errors a second for the bad one for as long as the process runs. The other
  choice: start, report the subject in `/stats` and as unhealthy, and stop retrying it. Fail boot, or start and
  report it?

ADR-113 is applied, not asked. D1's drops follow the surface audit and PR 1's D1 plan.
