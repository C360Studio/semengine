# Inventory: natsclient Subscribe handler root (#378)

Base: `46d3cd5`. The claim commit is empty (`git diff --stat 46d3cd5 HEAD` prints nothing). Draft PR #382.

## 1. Claimed gap

- `natsclient/client.go:1035` — `func (m *Client) Subscribe(ctx context.Context, subject string,`
  `handler func(context.Context, *nats.Msg)) (*Subscription, error) {`
- `natsclient/client.go:1041` —
  `return m.subscribeOwned(ctx, "Subscribe", subject, nil, func(*nats.Conn) nats.MsgHandler {`. It passes
  `cancelHandlers` as nil.
- `natsclient/client.go:1044` — `msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)`. The gap is confirmed. The
  issue cites
  `:1035-1038`, which were written at `05d6ec6` and are stale.
- The callback closure at `:1042-1053` captures the call's `ctx` for the whole life of the subscription.
- The doc comment promises the old shape: `client.go:997` — "The context is derived from the parent context with a
  30-second timeout
  for message processing."
- Traces from headers: `:1047-1049` (`ExtractTrace` / `ContextWithTrace`).
- A handler's context today ends when the call's context ends or after 30 s. `Close` does not end it, because it never
  cancels the
  caller's context. `Close` still joins the handler (`watchOwnedSubscription`, `:1181-1193`).

## 2. Every place that decides who parents handler contexts

- `natsclient/stream.go:461` — `func clientOwnedHandlerContext() (context.Context, context.CancelFunc) {`. It is the
  only root
  constructor. Its doc at `:455-460` names consumers and `SubscribeForRequests` only.
- `ConsumeStreamWithConfig`: `stream.go:445`; `:565` (the internal consume); owned by `owned_delivery.go:121-162`.
- `ConsumeStreamWithConfigContexts`: the caller passes `handlerCtx` (`stream.go:742-747`).
- `SubscribeForRequests`: `request.go:339-340`, doc at `:319-329`.
- The shared seam: `subscribeOwned`'s `cancelHandlers` (`client.go:1066-1076`) is cancelled on every early return
  (`:1081`, `:1088`,
  `:1094`, `:1105`, `:1120`). The watcher takes over from `:1132`, and `:1185-1189` cancels when `Close` begins and when
  delivery ends.
- Spec: `openspec/specs/transport-client/spec.md:172` — `### Requirement: A setup context does not parent its handlers`.
  - `:181-182` names the exception: "`Subscribe` does not yet follow this requirement (#378): its handlers' context
    descends from the
    call's own context."
  - `:180-181` is the `SubscribeForRequests` wording a `Subscribe` clause matches or departs from: "A
    SubscribeForRequests handler's
    context SHALL carry the trace from the request's headers when it has one and no value of the call's context."
  - `:280` restates the exception in "Subscribe returns after the server has read the SUB".
  - The scenarios at `:184-200` name consumers and `SubscribeForRequests` only.

Text that goes stale when `Subscribe` changes:

- `natsclient/client.go:997` — "The context is derived from the parent context with a 30-second timeout for message
  processing."
- `natsclient/client.go:1036` — "Refused here, not in the callback, which hands ctx to handler for every message."
- `natsclient/stream.go:455-460`, the doc of `clientOwnedHandlerContext`.
- `natsclient/README.md:58-60` — "`Subscribe` does not follow this rule yet: its handlers still descend from the context
  given to the
  call (#378)."
- `docs/admission-ledger.yaml:1013-1017`, natsclient item (12): "handlers run under a context the Client owns per
  consumer"; it names no
  subscription.
- `docs/admission-ledger.yaml:1128-1131`: `clientOwnedHandlerContext` is described as a root "per consumer … cancelled
  by the consumer's
  ownership goroutine (owned_delivery.go ownConsumer)". It names neither `SubscribeForRequests` nor
  `watchOwnedSubscription`.
- `docs/adr/113-start-context-bounds-start.md:47` cites `natsclient/stream.go:455-459`; the comment it quotes now spans
  `:455-460`.
- `service/message_logger.go:274-275` — "natsClient makes it the parent of every message's context."
- `service/message_logger.go:205` — "runCancel ends the run context Start creates, which every subscription runs under."
- Test comments citing `client.go` lines: `service/message_logger_http_test.go:209` (`client.go:1042-1043`),
  `service/message_logger_subscription_integration_test.go:162` (`client.go:1036-1047`) and `:324`
  (`client.go:1074-1078`).
- The archived logger design's premise
  (`openspec/changes/archive/2026-10-22-setup-04a-03-boot-service-message-logger/design.md:85-87`) and mutant rows
  `:481`, `:484`. The
  archive is history and is not edited.

## 3. Callers of `Client.Subscribe`

From gopls references at `client.go:1035:18`, with and without `-tags=integration`. `gopls implementation` finds no
interface.

**Production: one caller.** `service/message_logger.go:437` —
`subscription, err := ml.natsClient.Subscribe(runCtx, subject, func(msgCtx context.Context, msg *nats.Msg) {`

What the logger relies on from the handler context:

- `handleMessage` drops a message whose context has ended: `:651` — `if ctx.Err() != nil {`.
- It reads the trace from the context: `:676` —
  `if tc, ok := natsclient.TraceContextFromContext(ctx); ok && tc != nil {`.

Its two subscribe paths:

- **At Start:** `start` (`:262`) makes `runCtx` at `:276` —
  `runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))`. The
  comment at `:274-275` states the premise this change removes: "natsclient makes it the parent of every message's
  context". The loop
  checks the Start context before each subscribe (`:284-286`), then subscribes under `runCtx` (`:288`). After the last
  subscribe there
  is no check: `:292-305` start the retry and log, with no `ctx.Err()`. `Guard.Start` commits without reading ctx
  (`internal/lifecycleguard/guard.go:86-89`). So there is no final Start-context check to keep.
- **Retry:** `retrySubscribes(runCtx, …)` (`:459-484`) calls `subscribeSubject(runCtx, …)` at `:481` every 100 ms
  (`:177`).

How `Stop` orders things (`cleanup`, `:518-547`):

- `joinRetry` at `:519` runs first. Its in-progress subscribe "runs to its own end" (`:457-458`). `runCtx` cannot end
  it, because
  `runCtx` is cancelled only at `:527-533`.
- `drainSubscriptions` follows (`:522`, `:573-591`).
- Then the `runCtx` cancel. So `runCtx`'s cancellation never reaches a running handler today: every drain succeeds
  before it, or
  `cleanup` returns first.

`runCtx` has no other reader: its only uses in `service/message_logger.go` are at `:276`, `:288`, `:297`, `:436-437`,
`:459` and `:481`.

**Tests: 19 references.** None reads the handler's context:

- `natsclient/client_close_final_test.go:225`, `:762`, `:816`, `:841`, `:1031`
- `client_close_held_test.go:64`, `:106`
- `client_lifecycle_test.go:464`
- `client_test.go:246`
- `nil_context_siblings_test.go:217`, `:296`
- `subscribe_registration_test.go:213` (`subscribeAsync`; its callers are at `:327`, `:372`, `:402`, `:446`, `:484`,
  `:595`), `:258`,
  `:304`, `:353`
- integration-tagged: `client_integration_test.go:106`, `integration_test.go:213`,
  `subscription_integration_test.go:35`, `:68`

Every handler either ignores ctx or calls `held.hold()` / `msg.Respond`. They rely on the setup behaviour (refusal
order, the PONG
wait, `Close` joining the handler), not on how the handler context is parented.

**Logger tests whose stated sensitivity depends on today's parenting:**

- `service/message_logger_subscription_integration_test.go:159-165`,
  `TestMessageLoggerKeepsCapturingAfterStartContextEnds`. It says a
  logger that "subscribes under the Start context … stores nothing here" and cites `client.go:1036-1047`.
- `:184-192`, `TestMessageLoggerStopDrainsInFlightMessage`. It says "A Stop that ends the run context before it drains
  drops
  message 2".
- These match mutants in `openspec/changes/archive/2026-10-22-setup-04a-03-boot-service-message-logger/design.md:481`
  and `:484`,
  whose premise is at `:85-87`.
- Stale line cites: `message_logger_http_test.go:209` (`client.go:1042-1043`) and
  `message_logger_subscription_integration_test.go:324`
  (`client.go:1074-1078`).
- Other logger guards: `TestMessageLoggerPassesTheLifecycleSuite` (`message_logger_suite_integration_test.go:68`) and
  `TestMessageLoggerRetriesSubscribeUntilConnected` (`:329`).

**Logger spec and docs:**

- `openspec/specs/service-lifecycle/spec.md:528-583`. `:531-532` says "SHALL check its context before each subscribe".
  `:540` says
  "How long `Start` can run past its context's end is not promised". `:580-583` lists the paths no test covers: a
  context ending
  between subscribes, and `Stop` waiting on a running retry.
- `service/README.md:438-441` — "a subscribe already running is not shortened by it".

## 3a. Consequences for the message logger

Facts only; the design decides.

**(a) Start can return nil after its context has ended (ADR-113).**

- ADR-113 (`docs/adr/113-start-context-bounds-start.md:59-60`): "if ctx ends first, Start returns that error and holds
  nothing".
- The logger's `start` checks its context only before each subscribe (`service/message_logger.go:284-286`). From `:292`
  to `:305` there
  is no `ctx.Err()`: it starts the retry and returns nil. `Guard.Start` commits without reading the context
  (`internal/lifecycleguard/guard.go:86-89`). The spec says only "How long Start can run past its context's end is not
  promised"
  (`openspec/specs/service-lifecycle/spec.md:540`).
- Today: a Start context that ends during or after the last subscribe still gives `Start` a nil return, because the
  subscribe runs under
  `runCtx` (`:288`), which the Start context cannot end. This is a gap on today's code.
- If the design passes the Start context to `Subscribe` (graph-ingest's `SubscribeForRequests` shape,
  `processor/graph-ingest/component.go:1025`, `:1028`): a subscribe cut by the Start context returns a transient error
  (`natsclient/client.go:1086-1089` before the call acts, `:1160-1161` after the round trip). `subscribeSubject` records
  the subject as
  pending (`service/message_logger.go:441-442`), `failed++` (`:288-289`), the retry starts (`:292-297`) and `Start`
  returns nil (`:305`).
- The existing alternative is graph-ingest's kept final check (`processor/graph-ingest/component.go:1031`), guarded by
  `TestGraphIngest_StartFailsWhenItsContextEndsAfterHandlerSetup`
  (`processor/graph-ingest/lifecycle_integration_test.go:536`; T6/M7 in
  the archived `natsclient-handler-root` design). Keeping `runCtx` for the Start subscribes is the other existing shape.
- If this change leaves the gap unfixed, it is filed as an issue (protocol, File step; #212).

**(b) Close would end a logger handler's context.** Once the client root is cancelled when `Close` begins
(`natsclient/client.go:1188-1189`), the guard at `service/message_logger.go:651` (`if ctx.Err() != nil {`) returns
without storing any
message nats.go still delivers during `Close`'s drain. Today such a message's context descends from `runCtx`, which
`Close` does not end,
so it is stored. How many messages nats.go delivers during the drain is not promised (ADR-112). No test drives a logger
message through
`natsclient.Client.Close`.

**(c) Trace inheritance: the logger loses nothing.** `runCtx` keeps the Start context's values
(`service/message_logger.go:276`,
`context.WithoutCancel(ctx)`). No production code outside `natsclient` writes a context value:
`git grep -n "ContextWithTrace(\|context.WithValue(" -- '*.go' ':!*_test.go' ':!natsclient/'` returns nothing. The trace
the logger
reads at `:676` comes from the message headers (`natsclient/client.go:1047-1049`).

**(d) The run context would stop doing anything.** After the change, `runCtx` only bounds the setup of the retry's
subscribes (`:297`,
`:459`, `:481`) and, unless the design changes it, the subscribes at Start (`:288`). Its cancel at `:527-533` runs after
`joinRetry`
(`:519`) and `drainSubscriptions` (`:522`), when nothing runs under it, so it ends nothing.
The local `runCtx` and the field `runCancel` (`:205-206`) become vestigial; the design keeps,
renames or drops them. The comment at `:274-275` goes stale whatever it decides.

**(e) Test sensitivity lost.** `TestMessageLoggerKeepsCapturingAfterStartContextEnds`
(`service/message_logger_subscription_integration_test.go:165`) and `TestMessageLoggerStopDrainsInFlightMessage`
(`:192`) no longer
detect the mutants at rows `:481` and `:484` of the archived logger design; those become survivors.

## 4. Who it is for

The fix shape adds no exported symbol, subject, bucket or config field. `Subscribe` is framework API: at the pin it has
20 production
references (gopls on a pin snapshot, `natsclient/client.go:826`):

- `natsclient/typed.go:81`, `:94` (not ported: `natsclient/typed.go` is absent here)
- `output/file/file.go:365`, `output/httppost/httppost.go:351`, `output/websocket/websocket.go:1082`
- `processor/json_filter/json_filter.go:330`, `json_generic/json_generic.go:304`, `json_map/json_map.go:349`
- `processor/rule/processor.go:1076`
- `service/message_logger.go:282`
- `storage/objectstore/component.go:542`
- five `processor/research-graph-*` callers, `cmd/e2e-semstreams/mission/command.go:291`, and three
  `examples/processors/*`

All pass a variable named `ctx`. Open evidence question: whether each of those is a Start context or a run context was
not measured.

## 5. Problem shape

A setup context bounds registration while a client-owned root parents the handlers, cancelled when `Close` begins.

- Closest instance: `SubscribeForRequests` (`request.go:339-340`), built on the same `subscribeOwned`.
- Its proof is `natsclient/request_handler_root_test.go:28`, `:87` and `:101` (T1–T3, on an embedded server, not under
  synctest,
  #156).
- How the caller adopted it: graph-ingest passes its Start context (`processor/graph-ingest/component.go:1025`, `:1028`)
  and keeps a
  final check (`:1031`), guarded by `TestGraphIngest_StartFailsWhenItsContextEndsAfterHandlerSetup`
  (`processor/graph-ingest/lifecycle_integration_test.go:536`).
- Archived design: `openspec/changes/archive/2026-10-23-natsclient-handler-root/design.md`. D1 is the split. D2 covers
  the caller and
  the kept final check (T6, M7). It has mutants M1–M3, where M3 survived (`notes/2.3.md`), and residual risks: trace
  inheritance is
  lost, a caller can no longer stop in-flight handlers by cancelling, and `Subscribe` kept the old shape until #378.

## 6. Pin ADRs

This change carries no package from the pin. `natsclient` and `service/message_logger.go` are already ported. The
governing record is
the owner ruling, #9 comment 5994720412 item 2, applied under ADR-113.

## Adjacent claims

From `gh pr list --state open`, re-measured:

- **#382** (this claim): 0 files.
- **#376** (service PR 5, storage observability): open and ready, not a draft. Head `aecf79b`;
  `git diff --stat 46d3cd5...aecf79b`
  gives 31 files, +6387 −30. Archived and synced. Files shared with this change's expected set:
  - `openspec/specs/service-lifecycle/spec.md`: one hunk, `@@ -658,3 +658,128 @@`, after `:658`. It does not touch the
    logger
    requirement at `:528-583`.
  - `docs/admission-ledger.yaml`: hunks at `:845` (adds one line at `:848` in the natsclient row) and `:2824`, `:2843`,
    `:2851`,
    `:2865`, `:2976`, `:3002` (service rows). The `:848` insert moves this change's stale lines `:1013-1017` and
    `:1128-1131` down by 1.
  - `service/README.md`: hunks at `:5` (+3), `:80`, `:515` (+134) and `:522`. The logger text at `:438-441` is in no
    hunk but moves to
    about `:441-444`.
  - `docs/repository-map.md`: hunks at `:30`, `:49`, `:56`, `:79`.
  - Its design cites `service/message_logger.go:257-258` and `:276` as precedent; edits there leave its archived cites
    stale, which is
    history.
  - It touches no `natsclient/*`, `service/message_logger*` or `openspec/specs/transport-client/` file.
- **#381** (draft, spec queue, #262, head `8cde24c`): 0 files. It will touch `scripts/` and the `spec-queue` spec; a
  `docs/repository-map.md` row at archive is the only possible overlap.

Facts for the merge order: #376 is ready and this change has no design yet, so #376 is expected to land first.
This change then merges
`origin/main` and re-measures its ledger, README and spec cites on the merged tree. No code file overlaps.

## Open issues

- Searches: `gh issue list --state open --search` for `Subscribe`, `message_logger`, `message logger`, `MessageLogger`,
  `subscribeOwned`, `handler root`, `runCtx` and `natsclient`.
- Kept: #378 only.
- Names the word, not this path: #106, #9, #280, #195, #196, #300, #301, #314, #281, #216, #83, #141, #68, #162,
  #24, #60, #103, #134.

## Not triggered

- Collision table: no new primitive. The fix shape reuses `clientOwnedHandlerContext` (`stream.go:461`).
- Intent check: no boundary moves.

## Adopter seam

`Subscribe` is reached from outside the repo.

1. **What an adopter must know today:**
   - Pass a run context, never the Start context.
   - Cancelling the subscribe context ends running and future handler contexts.
   - Handlers inherit the subscribe context's values (a trace, for example).
   - `Close` does not end a handler's context.
2. **If they do nothing:** passing the Start context, as graph-ingest now does for
   `SubscribeForRequests`, means every handler after Start has an ended context. A handler that checks
   `ctx.Err()`, as the logger does at `:651`, drops every message silently.
3. **Where they find out:** a doc (`client.go:997`) and the `transport-client` spec at `:181-182`.
   That is doc rank, for a correctness fact.
4. **What they should have to know:** the four items in point 1, which `SubscribeForRequests` and the
   consume calls no longer require.

## Open evidence questions

- At the pin, which context each of the 20 production `Subscribe` callers in section 4 passes: a Start context or a run
  context. Not measured.
