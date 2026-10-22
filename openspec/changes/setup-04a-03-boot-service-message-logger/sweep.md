# Shape sweep: setup-04a-03-boot-service-message-logger (service PR 4 of 7)

base: `265235183bf517121883c4b957fb37356c901835` (`main` `ad9fda0` plus the claim commit; draft PR #367)

The per-PR read the architect contract asks for (§ The shape sweep), of the three files this pull request carries, on
a scratch copy of the SemStreams pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (`gh api .../tarball/8b99efe9`, local
only). It re-uses the whole-package inventory, `INVENTORY PASS` round 2 at `ad946b7`
(`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, "the inventory" below), wherever
that already pins a line, and adds what it did not pin. Disposition is proposed here and settled in `design.md`. Pin
lines are `service/<file>:<line>`; `message_logger.go` is written `:n` where no file is named.

Files (non-test pin lines, 2,245 in all): `message_logger.go` 1,234, `message_logger_http.go` 614,
`message_logger_kv_watch.go` 397. Test files 2,613 lines (§ 6).

## 1. Findings the inventory already pins (cited, not repeated)

| Id | Shape | Pin lines | Disposition |
| --- | --- | --- | --- |
| S1 | run context from the Start context | `:714-715`; retry `:790`, `:801`; watch goroutine `message_logger_kv_watch.go:230` | required (ADR-113; probe § 3); design D2 |
| S3 | copied lifecycle state machine | `:233-246` (`lifecycleUsed`, `lifecycleTerminal`, `stopping`, `running`, the reconcile gate) | required (#196; ADR-108 Decision 2); D2 |
| S7 | classification by error text | `message_logger_http.go:443`, `:497` | optional; probe § 3 shows the sentinels hold; recommended in D4 |
| S10 | one-implementation interfaces | `:166-168` `messageLoggerSubscription`, `:175-177` `messageLoggerSubscribe` | optional; they go with M2 (D1) |
| § 2.4 | renamed names | `:17` and `:347` (`componentadmission.Access{}`, ruling C); `:347-349` (`Snapshots` returns `[]component.Declaration`: `Name()`, `Inputs()`, `InputDeclarationFacts()` become `InstanceName`, `InputPorts`, `InputFacts`); `:261` (`NewBaseServiceWithOptions` loses its config argument, PR 1 Q1) | required (build); #280 item 2 in D5 |
| § 7 | fixed address in a test | `message_logger_test.go:80` — `natsClient, err := natsclient.NewClient("nats://localhost:4222")` | required (harness rules); D7 |
| § 7 / #195 | test client | `message_logger_subscription_integration_test.go:18` — `natsclient.NewTestClient(...)`; `message_logger_kv_acquisition_integration_test.go:19` — `getSharedNATSClient(t)` | required (harness rules): `fixtureClient`; D7 |

## 2. Findings added by this sweep

Each is `path:line` — the line's text at the pin, then the shape and the proposed disposition.

- **M1, shape 4 (unexported code only tests reach): reconciliation of a changing subject set.** Production asks for
  one fixed set for the logger's whole life: Start computes it once (`:748` — `reconcileErr =
  ml.reconcileSubjectsAdmitted(subscriptionCtx, subjects, metadata, overlaps)`, `:753-755` for explicit subjects) and
  the retry passes the same captured `desired` (`:683` — `if ml.reconcileSubjects(subscriptionCtx, desired, metadata,
  overlaps) == nil {`). So the obsolete-drain path `:456-499` (`drainObsoleteSubscriptions`), the retained-overlap
  deferral `:651-669` (`retainedAcceptedOverlap`), the retained-handle branch `:518-520` and the "raced an existing
  subscription" branch `:559-561` never run in production; Start's reconcile returns before the retry goroutine starts
  (`:749-751`), so two reconciles never overlap either. Only tests reach them, by calling `reconcileSubjects` with new
  sets: `message_logger_lifecycle_test.go:125`, `:199-201`, `:225`, `:267`, `:312-315`. The reconcile gate
  (`:608-637`) fences Stop against Start and the retry, which the guard (S3) and a joined retry replace. Optional
  (inherited, unexported; Bounded port scope); recommended in D2 under the owner's simple-Go intent (PR 3 C1/C2
  precedent).
- **M2, shape 4 (test-swap seams).** `:196` — `subscribe         messageLoggerSubscribe` and `:278-283` (the
  production closure over `natsClient.Subscribe`); `:202` — `retryAfter        func(time.Duration) <-chan time.Time`
  (`:288` `time.After`); `message_logger_http.go:375-379` — `handleKVQueryWith`, "with the bucket source injected, so
  the endpoint's behavior can be driven without a live NATS server", with `kvBucketProvider` `:365-370`, whose only
  production value is `ml.natsClient` (`:375`); `:317-319` — exported `SetDecoder`, "tests can leave it nil", whose
  only production caller is the constructor (`:64`). Every pin test that swaps one is listed in § 6. Optional
  (unexported except `SetDecoder`; `AGENTS.md` § 4 back-door row); recommended dropped in D1, tests moved to the
  production path (D7).
- **M3, a failed Stop is final (required: ADR-113 Decision).** `:920-928` — `completeMessageLoggerStop` sets
  `ml.lifecycleTerminal = true` and `ml.subscriptions = make(...)` whatever `stopErr` was, so a subscription whose
  drain failed is forgotten and a later `Stop` returns nil (`:822-825`) and drains nothing. Probe § 3, third run.
  ADR-113 (`docs/adr/113-start-context-bounds-start.md:61-62`): "if `ctx` ends first it returns that error and a later
  `Stop` tries again". Same class as PR 3's C6. D2.
- **M4, S1's observable cost.** `:947-951` — `select { case <-ctx.Done(): return` runs before any counter, and the
  message context is a child of the subscribe context (`natsclient/client.go:1038` — `msgCtx, cancel :=
  context.WithTimeout(ctx, 30*time.Second)` in the tree; the pin's is the same shape). With the subscribe context
  derived from Start's (`:715`), every message after the Start context ends is dropped and not counted (probe § 3,
  second run: `entries=1 total_messages=1` after a second delivery). Required with S1. D2.
- **M5, a constructor that builds an unusable logger; three spellings of each default.** `:250-258` —
  `NewMessageLogger` with a nil config takes `DefaultMessageLoggerConfig()` (`:129-136`, `MonitorSubjects: []string{"*"}`),
  but only `NewMessageLoggerService` sets the registry (`:65` — `ml.componentRegistry = deps.ComponentRegistry`), so
  that logger's `Start` always refuses (`:710-713` — "message logger wildcard mode requires component registry").
  `:62-68` sets the decoder before checking the error. Defaults are written three times: `:33-41`, `:129-136`,
  `:263-268`. Pin-wide typed uses outside `service` of `NewMessageLogger`, `DefaultMessageLoggerConfig`, `SetDecoder`:
  0 each (the inventory's tool, `users-prod.tsv`); consumers: 0. Optional; recommended in D1: one unexported
  constructor, refusals at construction (PR 3 C2 precedent).
- **M6, a nil payload registry is accepted and every message becomes "raw".** `:64` —
  `ml.SetDecoder(message.NewDecoder(deps.PayloadRegistry))`; the decoder "fails fast on every Decode call"
  (`message/decoder.go:29-31`), and `:967-975` swallows that error into `msgType = "raw"`. Trigger: a composition that
  omits `PayloadRegistry` (#185: MEDIUM at most). semteams sets it (`cmd/semteams/main.go:208`), semsource builds it
  (`cmd/semsource/run.go:170`). Optional; recommended refused at construction (D1).
- **M7, a negative `sample_rate` is accepted.** `:117-127` — `Validate` checks only `max_entries`; `shouldSample`
  (`:333-339`) treats any value ≤ 1 as "all". Optional (fail-closed config); recommended refused (D1).
- **M8, surface audit (a): `ConfigSchema` has no reader; its bounds are a second spelling.** `:1196-1234` —
  `func (ml *MessageLogger) ConfigSchema() ConfigSchema {`, written for `Configurable`, which PR 1 dropped (no type
  assertion anywhere; PR 1 design `:76-80` left this method to PR 4, "if both go, the three names go with the last of
  them"; PR 2 dropped `Metrics.ConfigSchema`). Pin-wide and consumer readers of `service.ConfigSchema`,
  `service.PropertySchema`, `service.NewConfigSchema` outside `service`: 0 (`grep -rn -E
  'service\.(ConfigSchema|PropertySchema|NewConfigSchema)'` over the pin and the four consumer tarballs). Its schema
  says `max_entries` minimum 1000 (`:1211`) where `Validate` accepts 1-999. Required (surface audit): dropped with
  `service/configurable.go` and `service/configurable_test.go` (D1).
- **M9, surface audit (a): `GetMessages`.** `:1127-1129`; readers: tests only
  (`message_logger_subscription_integration_test.go:52`, `:61`). Required: dropped (D1).
- **M10, a consumer's subjects spelled in the framework.** `:391-395` and `:652-656` — `{"agent.toolcall.proposed.>",
  "agent.toolcall.proposed.*"}` and two more pairs: the logger resolves only these three overlaps, named after the
  agentic packages' subjects (`AGENTS.md`: SemEngine "never a consumer's domain semantics"); any other pair where one
  subject covers another is subscribed twice, and core NATS delivers a message to each matching subscription, so it is
  captured twice. No ported component declares these subjects today. Optional; recommended in D3: a generic covered-
  subject rule.
- **M11, a malformed explicit subject retries forever.** `:539` — `ml.logger.Error("Failed to subscribe to subject",
  ...)`, every `messageLoggerReconcileRetryDelay` (`:179`, 100 ms; `:682`), for the life of the process: about ten
  Error lines a second per bad subject, and `Start` returned nil (`:771-781`). Probe § 3: nats.go refuses `"foo bar"`,
  `"foo..bar"` and `""` with `nats.ErrBadSubject`. Trigger: a typo in `monitor_subjects`. Optional; recommended in D3:
  a subject nats.go refuses fails `Start`, naming it (by analogy with the owner's 2026-10-21 Q-B ruling on PR 3, which was
  about a component that cannot be built, not this logger: boot fails and names it; D3 asks the owner to confirm
  the analogy).
- **M12, § 5 (c) class: `invalid_messages` is never counted.** `:226` declares it, `:968-975` adds to
  `validMessages` on both branches, `:1168` reports it. Changing what `/message-logger/stats` answers is the owner's
  (PR #318 precedent): Q1(d).
- **M13, the trace index grows without bound.** `:1077-1083` — `ml.traceIndex[traceID] = append(...)`; nothing removes
  a sequence when its slot is overwritten (`:1062-1075`) or a trace ID when all its entries are gone. Trigger: a
  long-running process whose messages carry trace headers. A defect (#212): fixed here in D4 by dropping the index.
- **M14, one non-JSON payload breaks `/entries` and `/trace`.** `:997` — `RawData:     json.RawMessage(data),` with
  `message_logger_http.go:280-283` and `:309-312`: encoding the slice fails for every entry once one is not JSON, and
  the route answers 500 until that entry is overwritten (probe § 3). The watch already guards the same case
  (`message_logger_kv_watch.go:280-286`, `json.Valid`). Trigger: any non-JSON publish on a monitored subject. Q1(a),
  recommended required.
- **M15, an import registers something.** `message_logger_http.go:19-21` — `func init() {` /
  `RegisterOpenAPISpec("message-logger", messageLoggerOpenAPISpec())`; the tree's contract line forbids it
  (`service/openapi_registry.go:11-13`). The spec has a `SchemaRef` (`message_logger_http.go:91` —
  `#/components/schemas/MessageLogEntry`) that resolves only through the registry. Required; D4 (PR 3 D5 option a).
- **M16, the mount prefix spelled twice; two bucket validators.** `message_logger_http.go:392` — `path :=
  strings.TrimPrefix(r.URL.Path, "/message-logger/kv/")` ignores the `prefix` argument `RegisterHTTPHandlers`
  receives (`:27`); `message_logger_kv_watch.go:147-158` parses the path by hand although the route declares
  `{bucket}` (`message_logger_http.go:45`). Bucket names are checked twice, differently: `message_logger_http.go:408-413`
  (no wildcard check) and `message_logger_kv_watch.go:55-69`. Optional; recommended in D4.
- **M17, a KV filter outside the shared validator.** `message_logger_kv_watch.go:74-88` (`isValidWatchPattern`, its own
  grammar: refuses `>` anywhere, accepts a space) feeds `kv.Watch(ctx, pattern)` (`:219`). `AGENTS.md` § 2: NATS KV
  keys and filters pass shared validators at entry; the tree's is `natsclient.ValidateKVWildcardFilter`
  (`natsclient/kv_key_contract.go:157`). Required; D4.
- **M18, create and update guessed from the revision.** `message_logger_kv_watch.go:95` — `if entry.Revision() == 1 {`.
  Probe § 3: a KV revision is per bucket, so a second key's first put has revision 2 and is reported "update"; a purge
  falls to `"unknown"` (`:101-102`) and carries an empty-string value (`:278-286`). Q1(b).
- **M19, the watch is not joined and Stop does not end it.** `message_logger_kv_watch.go:230` — `go
  ml.consumeKVWatchUpdates(ctx, watcher, bucket, eventChan)`: the handler can return before that goroutine does;
  the stream ends only when the client goes or the watcher closes (`:306-337`). In the tree, `StopAll` stops services,
  then waits for in-flight requests (`service/service_manager_http.go:203-224`, "waits, within ctx, for the requests
  in flight. If ctx ends first it closes every connection"), so one open watch (semteams `taskfiles/dev.yml:172`, `curl
  -N .../kv/ENTITY_STATES/watch`) makes `StopAll` wait until its context ends and return that error. At the pin the
  server's base context ended with Start's (inventory S2), which PR 2 removed. Required (ADR-113: `Stop` is nil only
  after everything is released; background-work spec: spawned work joins). D2.
- **M20, documented, not implemented (§ 5 (c) class).** `message_logger_kv_watch.go:11` — "SSE reconnection support
  with event IDs and Last-Event-ID header" (nothing reads `Last-Event-ID`; ids restart at 1 per stream, `:313`) and
  `message_logger_http.go:200` (the same in OpenAPI); `message_logger_http.go:39` — "only in development/test mode" and
  `:389` "should be restricted to dev/test environments" (nothing restricts it; the gate is that the service is off by
  default: not materialized, `service/service_configs.go:85-89`, test `service/service_configs_test.go:50-52`; this
  answers 03B design M2, `setup-03b-contract-boundary/design.md:628-629`); `message_logger_http.go:189` — 403 "KV
  query disabled in production" (never answered); `message_logger_http.go:227` — the watch's 404 (it answers 200 and an
  SSE error event, `message_logger_kv_watch.go:117-122`); `message_logger_http.go:82` — `subject` "Filter by NATS
  subject pattern" (`matchesPattern`, `:579-609`, is a `*` glob, so `agent.>` matches nothing; semteams documents it
  as a NATS pattern, `ui/src/lib/services/messageLoggerApi.ts:31`). Required: text rewritten (PR 3 C12 precedent),
  except the 404 and the filter, which are Q1(e) and Q1(c).
- **M21, every watch lookup error is called "not found".** `message_logger_kv_watch.go:200` — `return nil,
  fmt.Errorf("bucket %s not found: %w", bucket, err)`, for `ErrNotConnected` too. Optional; goes with Q1(e).
- **M22, retained, recorded (optional, no named trigger beyond cosmetics).** `:1053-1059` raw summary keys in map order
  (the same payload summarises differently run to run); `:296` uptime measured from construction, not `Start`; `:539`
  ten Error lines a second per subject during a NATS outage (the retry is the right behaviour; the volume is
  inherited); one KV watcher per SSE client, which pin ADR-081 names as a trap to convert (`081:27-28`, `:201`) and the
  pin did not.

Not findings: `KVWatchEvent`, `KVWatchConnectedEvent` (exported wire shapes, read in their file); `GetLogEntries`,
`GetEntriesByTrace`, `GetStatistics` (read by the routes, `message_logger_http.go:270`, `:299`, `:333`); the
drop-when-full of a watch's 100-event buffer, declared by a Warn line (`message_logger_kv_watch.go:293-298`).

Searched and empty in these three files: new roots or context values (`grep -n -E 'context\.Background|context\.TODO|
WithoutCancel|WithValue|ctx\.Value|select \{\}'`: 0 hits; every context is a parameter); a `context.Context` field:
none (the cancels are `:237-239`); `GenericJSON`: 0 hits; shape 5 beyond S7 (`grep -n -E 'strings.Contains\(err|err\.
Error\(\)'`: `:443`, `:497`, and two that only print an error, `message_logger_kv_watch.go:114`, `:394`). The tree's
reserved-subject guard reads string literals only (`internal/harness/contract/reservedsubjects_test.go:101`); the
`graph.mutation.>` at `:964` is a comment.

## 3. Pin probes (inventory § 7 left `MessageLogger` "not run")

Three probes, 2026-10-22, about four minutes of wall clock, all outside the repository. Sources are local:
`/tmp/pr4probe-src/zz_ml_probe_test.go`, `/tmp/pr4tree/zzprobe/kv_probe_test.go`, `/tmp/pr4rawprobe/main.go`; the
orchestrating session posts them with this output on #367.

**Suite probe** (go1.26.4 darwin/arm64; the pin copy with this tree's `internal/harness/lifecycletest` copied in, as
PR 3's probe). The adapter builds the logger as the pin's tests do (`NewMessageLogger` with one explicit subject and a
`subscribe` stand-in that records the context and handler and delivers as `natsclient.Subscribe` does, with the message
context a child of the subscribe context); `Observe` reports the base status and whether the subscription is live.
Command: `go test -race -count=1 -run 'TestProbeMessageLogger' -v ./service/`. Output (INFO lines removed):

```text
--- FAIL: TestProbeMessageLoggerSuite (0.10s)
    --- PASS: TestProbeMessageLoggerSuite/NilContextsRefused (0.00s)
    --- PASS: TestProbeMessageLoggerSuite/PreCancelledStartRefused (0.00s)
    --- PASS: TestProbeMessageLoggerSuite/StopBeforeStartSafe (0.00s)
    --- FAIL: TestProbeMessageLoggerSuite/StartContextEndStopsNothing (0.10s)
    --- PASS: TestProbeMessageLoggerSuite/StopReturnsItsEndedContext (0.00s)
    --- PASS: TestProbeMessageLoggerSuite/RepeatedStopIsNoOp (0.00s)
    --- PASS: TestProbeMessageLoggerSuite/SecondStartRefusedOrRestartCycle (0.00s)
    --- PASS: TestProbeMessageLoggerSuite/FailedStartHoldsNothing (0.00s)
    zz_ml_probe_test.go:101: live Start ctx: entries=1
    zz_ml_probe_test.go:104: after Start ctx cancelled: entries=1 total_messages=1
--- PASS: TestProbeMessageLoggerRecordsAfterStartCtxEnds (0.00s)
    zz_ml_probe_test.go:121: first Stop err=drain probe.x: probe drain failure; second Stop err=<nil>; subscription drained by second=false
--- PASS: TestProbeMessageLoggerFailedStopIsFinal (0.00s)
```

The failing check's message: `after the Start context ended the owner stopped holding [base-status=running
subscription]`. Read: S1 holds for the logger; M4 (a message after the Start context ends is neither stored nor
counted); M3 (a failed Stop is final).

**nats.go probe** (go1.26.10; nats.go v1.54.0 and an embedded nats-server v2.15.1, the tree's `go.mod`; the test image
is `nats:2.15.0-alpine`), through this tree's `natsclient` at `ad9fda0`, in a `git archive` copy. Command: `go test
-count=1 -v ./zzprobe/`. Output:

```text
    kv_probe_test.go:38: missing bucket: err=nats: bucket not found errors.Is(ErrBucketNotFound)=true
    kv_probe_test.go:40: bad bucket name: err=nats: invalid bucket name errors.Is(ErrInvalidBucketName)=true errors.Is(ErrBucketNotFound)=false
    kv_probe_test.go:48: empty bucket Keys: err=nats: no keys found errors.Is(ErrNoKeysFound)=true
    kv_probe_test.go:85: live: key=x op=KeyValuePutOp revision=1 delta=0
    kv_probe_test.go:85: live: key=y op=KeyValuePutOp revision=2 delta=0
    kv_probe_test.go:85: live: key=x op=KeyValuePutOp revision=3 delta=0
    kv_probe_test.go:89: subscribe "foo bar": err=nats: invalid subject errors.Is(ErrBadSubject)=true
    kv_probe_test.go:89: subscribe "foo..bar": err=nats: invalid subject errors.Is(ErrBadSubject)=true
    kv_probe_test.go:89: subscribe "": err=nats: invalid subject errors.Is(ErrBadSubject)=true
```

(The same run's initial-values watch reported a purge as `KeyValuePurgeOp`.) Read: S7's two sites can use `errors.Is`
(`jetstream.ErrBucketNotFound`, `jetstream.ErrNoKeysFound`) at these versions; M18; M11.

**JSON probe** (go1.26.4): encoding `[]entry` where one `json.RawMessage` holds `plain text`:

```text
err=json: error calling MarshalJSON for type json.RawMessage: invalid character 'p' looking for beginning of value written=0
```

Read: M14. Not re-run: the unit runs (once under `-race`, five times shuffled), the integration run and the test-text
guards are the inventory's § 7, run on the whole package; their hits in this PR's tests are § 1's two rows.

## 4. Open issues and pull requests

Queries, 2026-10-22, `gh issue list --state open --search <q> --limit 200`: `message_logger` (no hits),
`MessageLogger` (#280, #9), `message-logger` and `message logger` (#280, #195, #196, #9), `kv_watch` (none), `kv
watch` (#340, #103, #9), `SSE` (none), `service` (#30, #363, #196, #344, #364, #362, #195, #361, #280, #10, #9, #11, #29,
and also #36, #109, #68, #7).

- **#196** (lifecycleguard adoption) names `service/message_logger.go:234`. Stays open: this PR does the logger's
  adoption (D2) and comments; the component rows remain (changes 4-7). Not `Closes`.
- **#195** (`natsfixture.Open` for each package's test client) names the message logger's test files (its comment of
  PR #343). Stays open: this PR adopts `fixtureClient` for the logger's tests (D7); storage-observability (PR 5) and
  other packages remain.
- **#280** item 2 names `service/message_logger.go:347-349`. Decided here (D5) and commented in task 3.1; items 1 and
  3 keep it open. Not `Closes`.
- Name the word, not the package: #9, #7, #10, #11 (epics); #340 and #103 ("watch" in `config` and graph query);
  #30, #363, #344 (heartbeat's decode, PR 6), #364, #362, #361, #29, #36, #109, #68 (other `service` owners or other
  packages; none names a message-logger file).

Open pull requests (`gh pr list --state open --json number,title,isDraft,changedFiles,files`, 2026-10-22): **#367**,
this claim (3 files); **#358**, dependabot `prometheus/client_golang` 1.24.1 → 1.25.0 (`go.mod`, `go.sum`). No overlap:
this PR adds no module. Either may merge first.

## 5. Pin decision records for the three files (inventory § 8)

Searched the 85 records at the pin for `message_logger`, `MessageLogger`, `message-logger`, `message logger` (`grep
-l -i`): 058, 081, 093. Governing or cited:

- **ADR-093** (Accepted 2026-08-13): "Message-logger decoding is observability evidence, not a claim of" delivery
  (`093:41-43`); "Message-logger remains an observer" (`093:58-59`). Kept (D1, D7 U-h).
- **ADR-068 D1** (not in the pin's `docs/adr`; inventory § 8): a reader never creates a bucket — the KV query's
  comment (`message_logger_http.go:467-482`, "a reader still must never be the emitter"). Kept (D4).
- **ADR-081** (Accepted 2026-07-19): names `message_logger_kv_watch.go:216` as one watcher per SSE client
  (`081:27-28`) and plans to convert it (`081:201`). Not done at the pin; not done here (M22).
- **ADR-058** names `MessageLogger` only as an exemplar (`058:53-56`); not governing.
- This repository: ADR-113, ADR-108 Decision 2, ADR-112 (the natsclient contract the logger calls), ADR-110 (the
  logger writes no `ENTITY_STATES` entry; it reads KV only).

Pin guidance: none. `grep -rn -i 'message.logger\|message_logger\|MessageLogger'` over the pin's `.agents`,
`.claude`, `AGENTS.md`, `CLAUDE.md` and `docs/*.md`: 0 hits. Operator pages that describe it
(`docs/operations/01-local-monitoring.md`, `03-distributed-tracing.md`, `debugging-data-flow.md`) are sources for the
README, not carried; the migration pages are not carried (owner, no SemStreams state migration).

## 6. Pin tests (2,613 lines)

| File (lines) | What it reaches |
| --- | --- |
| `message_logger_test.go` (415) | config, sampling, helpers, overlap pairs, ring buffer, trace index; `:80` fixed address; `:12-55`, `:67-75` `ConfigSchema` (M8) |
| `message_logger_registry_test.go` (276) | explicit and wildcard subscription, overlap, retry, Stop; all through `ml.subscribe` and `ml.retryAfter` (M2); fake subscriber `:193-276` |
| `message_logger_lifecycle_test.go` (378) | the reconcile gate and obsolete drains (M1), through `ml.subscribe`; `:70-100` nil and ended contexts |
| `message_logger_subscription_integration_test.go` (92) | handler context lives until Stop; asserts the final failed Stop (M3) at `:62-63` |
| `message_logger_http_kv_query_test.go` (223) | KV query through `handleKVQueryWith` and a fake provider (M2); `recordMatchesStatus` `:126-147` |
| `message_logger_kv_acquisition_integration_test.go` (145) | both KV routes read existing buckets and create none (ADR-068 D1) |
| `message_logger_kv_watch_test.go` (92) | `isValidBucketName`, `isValidWatchPattern`, `detectKVOperation` |
| `message_logger_user_response_test.go` (49) | typed decode of `agentic.UserResponse` (ADR-093); imports `agentic` |
| `message_logger_census_test.go` (556) | imports `cmd/e2e-semstreams/mission`, `componentregistry`, `examples/processors/*`, `frameworkcapabilities/graphresearch` (inventory § 2.2): cannot be carried |
