# Inventory: retire-bind-generation-check

base: 0bbd78f
tree: /Users/coby/Code/c360/semengine-wt/claude/retire-bind-generation-check (branch claude/retire-bind-generation-check;
HEAD 5790254 merges `origin/main` at 0bbd78f into the empty claim commit 27e7703; `git diff --stat 0bbd78f HEAD` prints
nothing). Revision 2: first written at d2ba1a7 (sha256 43712b39...7879), INVENTORY PASS in
`/tmp/rbgc/inventory-review.md` with one MEDIUM and eight NITs, applied here. Between d2ba1a7 and 0bbd78f only
`.agents/contracts/semengine-reviewer.md`, `.agents/protocol.md` and `AGENTS.md` changed (#213, the File step), so every
`path:line` pin below is unchanged.
change id: retire-bind-generation-check; draft PR #215 (`Closes #214`, read back: closingIssuesReferences = #214)
phase: inventory only (architect contract, Required workflow step 2). No options, target state or tasks.

## The question and what was read

The question: what exists today for graph-ingest's recreated-before-bind check (PR #93 task 4.17, commit `7e07fea`),
and what else touches it, so that the owner's ruling can be carried out.

The ruling, quoted from #214 comment 6157056960 (2026-10-13, cglusky):

> **Ruled: retire the check.** graph-ingest reads the generation once, before it binds, as it did before task 4.17.
> The second read, the hold on deliveries and the `Start` failure go, with the tests that hold them,
> `TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart` included. The case is written down as a declared
> limitation beside the restored stream (#91 comment 6112550268) and the stream and consumer recreated during a
> disconnect (#91 comment 6113959050): new input in the recreated stream, at sequences the old generation's record
> covers, can be acknowledged without being applied. This replaces the part of 6112550268 that let task 4.17 proceed.
>
> **Scope note (Claude):** the `waitForStreamInput` field on `Component` stays in this change. It still stands in for
> the first read, beside `consumeStream`, in unit tests that bind without a server (`config_fields_test.go:214`,
> `statement_metadata_test.go:83`, `guard_generation_test.go:236`). Removing it is #139's item ("Fields nothing sets").
> The pull request that retires the check closes this issue.

Read: this repository at `d2ba1a7` (ranges, not whole files); task 4.17's commit `7e07fea` through `gh api` (PR #93's
branch, so the text before 4.17 is measured, not recalled); PR #93 compare `7e07fea...9b98371` (head of PR #93) for
test files changed after 4.17; the pin's `processor/graph-ingest/component.go` at `8b99efe9` through `gh api`; open
pull requests #194, #206, #215; open issues from the contract's query; #91 comments 6112550268 and 6113959050;
`go.mod` of semsource and semconnect (default branch; dependency closure is in scope for both,
`docs/inventory-scope.md:15-16`). Not read: any other sister repository; no test was run.

Contract obligations that do not apply, one line each:

- Pin probe, shape sweep, surface audit, "One package per pull request", "Open issues against the package": this
  change carries no package from the pin. It removes code SemEngine added in PR #93 task 4.17. Pin check:
  `gh api 'repos/C360Studio/semstreams/contents/processor/graph-ingest/component.go?ref=8b99efe9'` shows
  `waitForStreamInput func(context.Context, string) error` (pin `:497`) and one read,
  `if err := waitForStream(ctx, streamName); err != nil {` (pin `:1494`), returning only an error (pin `:1615`). The
  creation-time read is task 4.3's (#15); the second read is task 4.17's. Item 5 still lists the issues, as briefed.
- Collision table: not triggered. The change adds no durable, communication or coordination primitive.
- Intent check: not triggered. No boundary, tier or capability membership moves.
- Category 4 (who it is for): the change adds no exported symbol, port, subject, bucket or config field.
- Category 6 (pin ADRs): no pin package is carried. In this tree, `git grep -l -i -e 'stream generation' -e
  'creation time' -- docs/adr` returns nothing: no ADR here records the generation or the check.

---

## Item 1. Production lines (category 2: every spelling of the fact)

### 1a. Lines that exist only for the check (each one goes)

`processor/graph-ingest/component.go`:

- `component.go:1542` — `// An input the closure accepts waits for the second read of the generation (design D8, R3).`
- `component.go:1543` — `// confirmed closes when this function returns, after generationConfirmed is set, or not.`
- `component.go:1544` — `confirmed := make(chan struct{})`
- `component.go:1545` — `defer close(confirmed)`
- `component.go:1546` — `generationConfirmed := false`
- `component.go:1597` — `// Held until Start has read the generation again, or the delivery's context ends: the`
- `component.go:1598` — `// client's Close waits for this callback during setup (natsclient commitConsumer).`
- `component.go:1599` — `sameGeneration := false`
- `component.go:1600-1604` — `select { case <-confirmed: sameGeneration = generationConfirmed  case <-msgCtx.Done(): }`
- `component.go:1605-1610` — `if !sameGeneration { if nakErr := msg.Nak(); ... c.logger.Error("Failed to Nak an input
  whose stream generation was not confirmed", ...) ... return }` (the Nak of unconfirmed input; no counter)
- `component.go:1641` — `// The second read, once the rollback can drain the consumer: a stream deleted and created again`
- `component.go:1642` — `// since the first read has a new creation time, and this consumer belongs to it (design D8, R3).`
- `component.go:1643` — `again, err := waitForStream(ctx, streamName)` (the second read)
- `component.go:1644-1646` — `return fmt.Errorf("stream %s: read its creation time again after binding: %w",
  streamName, err)`
- `component.go:1647-1650` — `if !again.Equal(created) { return fmt.Errorf("stream %s was deleted and created again while
  its consumer was bound "+ "(created %s, then %s)", ...) }` (the mismatch error; the `Start` failure)
- `component.go:1651` — `generationConfirmed = true`

### 1b. Lines 4.17 changed that change back

Measured against `7e07fea`'s patch.

- `component.go:1476-1478` —
  `// reads (design D8, #15), never from configuration. It is read before the consumer is bound and`
  / `// again after, below: a consumer is deleted with its stream, and graph-ingest binds none while it` /
  `// runs, so when the two reads agree every input this consumer delivers comes from this generation.` Before 4.17
  the comment read: "It is read before the consumer is bound, a consumer is deleted with its stream, and graph-ingest
  binds none while it runs, so every input this consumer delivers comes from this generation. The exception is a
  stream deleted and created again between this read and the bind (openspec notes/4.3.md)." (`7e07fea` patch, hunk
  `@@ -1452,10 +1452,9 @@`). The old text cites an archived note by a path that no longer exists outside the archive.
- `component.go:1551` — `handle, err := consumeStream(ctx, natsclient.PortConsumerContext{...}, cfg, func(msgCtx
  context.Context, msg jetstream.Msg) {`. `msgCtx` is read only at `:1603`; after the removal it is unused. Before
  4.17 the parameter was `_ context.Context` (same hunk).

### 1c. What stays and what it keeps doing

- First read: `component.go:1471-1474` — `waitForStream := c.waitForStream` / `if c.waitForStreamInput != nil {` /
  `waitForStream = c.waitForStreamInput` / `}`; `component.go:1479` — `created, err := waitForStream(ctx, streamName)`;
  `component.go:1680-1686` — `stream, err := js.Stream(ctx, streamName)` ... `return stream.CachedInfo().Created, nil`
  (the wait retries 30 times with backoff capped at 2 s, `:1677-1679`).
- Zero creation time refused before any bind: `component.go:1483-1485` — `if created.IsZero() { return
  fmt.Errorf("stream %s reports no creation time; its generations cannot be told apart", streamName) }`.
- The generation: `component.go:1486` — `generation := uint64(created.UnixNano())`; carried on every input,
  `component.go:1623` — `generation:  generation,`; field `keyed_ingest.go:34-36` — `// generation is the input
  stream's generation: its creation time on the server, in` / `// nanoseconds, read when the consumer was bound
  (design D8, #15).` / `generation uint64`.
- The generation-aware guard (stays, #15): `keyed_ingest.go:86-88` — `func (r guardRecord) covers(work ingestWork)
  bool { return r.generation == work.generation && work.seq <= r.seq }`; drop path `keyed_ingest.go:275-277` —
  `if stale { c.redeliveriesDropped.Inc(); return natsclient.DeliveryDecisionAck, nil }`; stamps
  `keyed_ingest.go:325` (memory tier) and `keyed_ingest.go:490` — `record := guardRecord{generation:
  work.generation, seq: work.seq}`.
- The failed-start Nak that predates 4.17 (checkpoint 3 B2) stays and becomes the only failed-start Nak path for a
  callback that runs after the rollback: `component.go:1537-1541` — `// The closure submits to this pool and
  submitCtx, both from this Start, never to` ... `// By then this pool has shut down, so the submit is refused and
  the message is Nak'd.` / `pool := c.ingestPool`; `component.go:1628-1631` — `if serr := pool.SubmitBlocking(submitCtx,
  work); serr != nil { if nakErr := msg.Nak(); ...`. Refusal is measured in code: the rollback calls
  `c.ingestPool.Shutdown(ctx)` (`component.go:1139-1141`), `Shutdown` sets `p.stopped = true`
  (`internal/dispatch/keyed_pool.go:323-324`), and `SubmitBlocking` returns `ErrStopped` when `p.stopped`
  (`keyed_pool.go:216-220`).
- Handle recording, readiness registration and the bind-time backlog read stay: `component.go:1637-1639`,
  `:1656` — `c.registerBoundConsumer(streamName, consumerName)`, `:1661` — `c.recordBindBacklog(ctx, streamName,
  consumerName)`.
- The seam field stays (ruling's scope note): `component.go:412-414` — `// Test-only acquisition seams. Production
  falls through to the exact Client` / `// methods; no exported or adopter-facing lifecycle surface is introduced.` /
  `waitForStreamInput func(context.Context, string) (time.Time, error)`.

---

## Item 2. Tests

Rule used: a test goes when it asserts the second read, the hold, the hold's Nak or the mismatch failure; it stays
when it only uses `waitForStreamInput` or `consumeStream` to stand in for the first read or the bind. Evidence that
tests older than 4.17 do not need the hold: task 4.16's `task verify` on `f9ac34f` (the base 4.17 was written on)
"ended `verify: ok`" (archived `tasks.md:689`), and 4.17's notes list only the three new tests as failing first on
that base (archived `notes/cp4i-R3.md`, "Evidence"). Outcomes after removal below are predictions from the code;
none was run.

| Test (file:line at d2ba1a7) | Disposition | Why |
| --- | --- | --- |
| `guard_generation_integration_test.go:219` — `func TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart(t *testing.T) {` (doc `:213-218`) | goes | Named by the ruling. Deletes and recreates the stream inside the first read (`:255-271`), asserts `require.Error(t, startErr, "Start must fail: ...")` (`:273`). The known flake #214 fails at `:286`. |
| `guard_generation_test.go:338` — `func TestConsumerInputHeldUntilGenerationConfirmed(t *testing.T) {` (doc `:332-337`); subtests `:342` "confirmed", `:391` "changed", `:419` "delivery context ends" | goes, all three | "confirmed" asserts nothing is settled before the second read (`:364-369`); "changed" asserts the mismatch error and a Nak (`:406-415`); "delivery context ends" asserts the hold's `msgCtx` escape Naks (`:464-466`). With no hold, "delivery context ends" would see an ack (submitCtx `t.Context()` is live), so it cannot stay as written. |
| `failed_start_callbacks_integration_test.go:336` — `t.Run("generation changed", func(t *testing.T) {` (to `:387`) | goes | Each read returns a different time (`:346-349`); asserts Start fails naming `ENTITY` (`:368-376`), one drain (`:377`), Nak (`:384`). |
| `failed_start_callbacks_integration_test.go:389` — `t.Run("a later registration fails", func(t *testing.T) {` (to `:450`) | stays; body unchanged | Uses `waitForStreamInput` (`:405`, `return time.Unix(1, 0), nil`) only as the first-read stand-in. The input is held by the test's own `heldMetadataMsg` until after the rollback, then reaches the stopped pool and is Nak'd (item 1c). 4.17's notes: "Holds on the base". |
| `failed_start_callbacks_integration_test.go:331` — `func TestFailedStartDrainsAndNaksHeldInput(t *testing.T) {` and its doc `:324-330` | stays but changes | The doc describes the hold and the second read ("is held until Start has read the stream's creation time again (design D8, Codex interim review R3)"). One subtest remains; whether to keep the name, flatten the subtest or rename is a design choice. |
| `failed_start_callbacks_integration_test.go:229` — `func TestFailedConsumerSetupCallbackNaksAfterRollback(t *testing.T) {` | stays; unchanged | Today its Nak comes from the hold (consumeStream fails, so `generationConfirmed` stays false, `component.go:1605-1609`; 4.17 notes: "now that T2 goes through the hold"). After removal the callback, released after `Start` returns (`:285`), reaches the stopped pool and Naks at `component.go:1628-1631`, the pre-4.17 path. Its doc `:223-228` says "Once Start has returned, the hold is released" (`:226-227`): that is the test's own `heldMetadataMsg` hold (`:45-58`), not the generation hold, so the doc stays. The two lines 4.17 wrote here (`:270-271`, the comment and `handler(t.Context(), msg)`) stay and stay true. |
| `guard_generation_test.go:224` — `func TestStreamWithoutCreationTimeIsRefused(t *testing.T) {` | stays; unchanged | `:236` — `c.waitForStreamInput = func(context.Context, string) (time.Time, error) { return time.Time{}, nil }`: first-read stand-in; refusal at `component.go:1483-1485` precedes any bind. |
| `config_fields_test.go:212` — `func bindEntityStream(t *testing.T, c *Component) (natsclient.StreamConsumerConfig, bool, error) {` (callers `:251`, `:270`) | stays; unchanged | `:214` — `c.waitForStreamInput = func(context.Context, string) (time.Time, error) { return time.Unix(1, 0), nil }`; the callback is discarded (`_ func(...)`, `:221`). |
| `statement_metadata_test.go:66` — `func startStreamLane(t *testing.T, opts ...testComponentOption) (*Component, func(context.Context, jetstream.Msg)) {` | stays; unchanged | `:83` — `c.waitForStreamInput = ... return time.Unix(1, 0), nil }`; the handler is called only after `setupSubscriptions` returns (`:95-100`), when the hold is already open, so it never depended on it. 15 lines in 4 files name `startStreamLane` (`derived_time_test.go` 3, `fact_lane_fence_test.go` 2, `statement_metadata_test.go` 9, `write_modes_test.go` 1). |
| `guard_generation_integration_test.go:70` — `TestIntegration_IngestGuard_StreamRecreatedWithLowerSequences`; `:155` — `TestIntegration_IngestGuard_FileStreamKeepsItsGenerationAcrossRestart` | stay; unchanged | Hold the generation-aware guard through the real first read. |
| `guard_generation_test.go:100` — `TestGuardRecordIsPerGeneration`; `:179` — `TestNoGuardBucketLogsAndCountsFirstSeenInputs` | stay; unchanged | Drive `processIngest`, not setup. |
| `consumeStream` stand-ins that wrap the real client: `lifecycle_adapter_test.go:36`, `lifecycle_integration_test.go:123`, `long_apply_integration_test.go:75`, `readiness_integration_test.go:194`, `settlement_integration_test.go:122` | stay; unchanged | Real reads of one stream agree today; no assertion names the second read or the hold. `long_apply_integration_test.go` was added after 4.17 (compare `7e07fea...9b98371`: `added`); it publishes after `Start` (`:129-132`). `readiness_integration_test.go:194-206` holds the first input in its own wrapper, before graph-ingest's callback. |
| `interrupted_work_test.go` (added after 4.17) | stays; unchanged | Drives the pool and `processIngest` directly (`:196`, `:254`, `:280`). |

Helpers that go with the going tests (no other reference; `git grep -n -w` in `processor/graph-ingest`):

- `guard_generation_test.go:258` — `const heldEntity = "c360.test.held.generation.entity.001"`
- `guard_generation_test.go:263` — `func heldInputComponent(t *testing.T) (*Component, *mockKVBucket, *mockKVBucket) {`
- `guard_generation_test.go:284` — `func heldInputMsg(t *testing.T) *streamLaneMsg {`
- `guard_generation_test.go:294` — `func handlerOnItsOwnGoroutine(handlerCtx context.Context, msg jetstream.Msg) func(`

Helpers that stay (other references):

- `guard_generation_test.go:310` — `func onlyDisposition(t *testing.T, msg *streamLaneMsg) string {`. After the
  removal its only caller is `failed_start_callbacks_integration_test.go:447`, in a file built only with the
  `integration` tag, while the helper is defined in the untagged `guard_generation_test.go`. No gate fails
  (`revive.toml` has no unused-function rule; Go allows an unused function). Where it lives is a design choice.
- `guard_generation_test.go:326` — `func keyCount(m *mockKVBucket) int {` (also `interrupted_work_test.go:285`, `:287`)
- `guard_generation_test.go:31-32` — `generationFirst`, `generationSecond` (used at `:112-164`, `:191`, `:210`)
- `failed_start_callbacks_integration_test.go:307` — `type drainedConsumeContext struct {` and `:313`
  `newDrainedConsumeContext` (used by the remaining subtest, `:406`)
- `guard_generation_integration_test.go:21`, `:25`, `:42`, `:53` — `restartConsumer`, `startIngestOn`,
  `publishTriples`, `waitSettled` (used by the two staying tests and by `long_apply_integration_test.go:130`,
  `settlement_integration_test.go:188`, `:265`, `:359`)

Imports left unused (counted outside the removed ranges):

- `guard_generation_test.go:13` — `"errors"` (0 uses outside `:256-469`) and `:17` — `"testing/synctest"` (0 uses).
- `failed_start_callbacks_integration_test.go:16` — `"strings"` (only use `:374`, in the going subtest).

Line-pinned evidence that names the going tests: `docs/admission-ledger.yaml:3252-3254` (item 3).

---

## Item 3. Specs, designs, docs, ledger

### 3a. Current text that states the check (all must change or go)

`openspec/specs/graph-ingest-recovery/spec.md` (diffs against `7e07fea`'s patch show which lines 4.17 wrote):

- `spec.md:44-47` — `keeps that copy from applying twice. An input the callback accepts (it decodes and passes the
  callback's checks) SHALL` / `be negatively acknowledged, never acknowledged, terminated or applied, when its
  consumer's setup fails for any cause,` / `or when the callback reaches its submit to the ingest pool after a failed
  `Start`'s rollback has returned. ...`. Written by 4.17. Before 4.17: "An input that decodes and is accepted by the
  callback of a consumer whose setup failed, and that the callback submits to the ingest pool after the failed
  `Start`'s rollback has returned, SHALL be negatively acknowledged, never acknowledged or terminated; an input the
  callback settles before that submit keeps its own settlement." See point 1 under "Open points".
- `spec.md:60-64` — `#### Scenario: A delivery after a failed consumer setup` / `- **WHEN** a consumer's setup runs its
  callback on an input that decodes and is accepted, and the setup then fails` / `- **THEN** the input is negatively
  acknowledged, not acknowledged, terminated or applied, ...`. Written by 4.17; before it the WHEN ended "..., the
  setup then fails, and the callback reaches its submit to the ingest pool after the failed `Start`'s rollback has
  returned" and the THEN read "not acknowledged or terminated".
- `spec.md:71-73` — `because of a sequence recorded in an older one. An input SHALL carry the generation of the stream
  its consumer is` / `bound to. When the input stream is deleted and created again between graph-ingest's read of its
  generation and its` / ``consumer's creation, `Start` SHALL fail.`` Both sentences written by 4.17. The first becomes
  untrue in the window case after the removal (the input carries the first read's generation while the consumer
  belongs to the new stream).
- `spec.md:84-89` — `#### Scenario: Stream recreated while graph-ingest binds its consumer` / `- **WHEN** the stream is
  deleted and created again in that window, ...` / ``- **THEN** `Start` fails naming the stream, ...``. Written by 4.17;
  held by the going integration test.

`docs/admission-ledger.yaml`, row `- source_path: processor/graph-ingest` (`:3153`):

- `:3165-3170` — `generation then sequence in 16 bytes (abb302f); Start reads the creation time before it binds the
  input's` / `consumer and again after (component.go:1479, :1643) and fails, naming the stream, when the two differ,
  and` / `an input accepted before the second read waits for it and is applied only when the reads agree, else` /
  `negatively acknowledged; it reads twice so that a stream deleted and created again while Start binds is` / `not
  read as the generation before it (design D8, "The generation belongs to the bound consumer"; PR #93` / `comment
  6112098987, R3; 7e07fea); ...`
- `:3252` — `TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart for the two reads),`
- `:3253-3254` — `guard_generation_test.go (TestGuardRecordIsPerGeneration, TestConsumerInputHeldUntilGenerationConfirmed`
  / `for the held input), ...`

### 3b. Current text whose wording or pins the removal touches

These lines do not name the check.

- `docs/admission-ledger.yaml:3240-3242` — `Declared limitations (owner rulings, #91 comments 6112550268 and 6113959050;
  design D8, "The generation` / `belongs to the bound consumer", and D23): cases where a recreated stream or key is
  not told apart from a` / `redelivery or a refused create; ...`. The one current place both stream limitations are
  named (see 3d).
- `docs/admission-ledger.yaml:3283` — `processor/graph-ingest/component.go (decodeEntity, :1741, and its call in the
  consume closure, :1575),` (row `processor/graph-ingest/component.go`, `:3275`). `:1741` sits after the removed
  lines and moves up by about 30; `:1575` moves only if the first-read comment (`:1475-1478`) changes length.
- `processor/graph-ingest/README.md:29-32` — ``and the last sequence applied from it in `GRAPH_INGEST_APPLIED_SEQ`, ...``
  / `The generation is the` / `stream's creation time on the server, read when graph-ingest binds its consumer: a
  stream deleted and created again` / `is a new generation whose sequences restart at 1, and its inputs are never
  dropped because of the sequences of the` / `stream it replaced.` "Never" is already contradicted by the two declared
  limitations; the third adds a case.
- `processor/graph-ingest/keyed_ingest.go:81-84` — `// from the same stream generation, at a sequence not newer than
  the last applied. An input` / `// from any other generation is not covered, so a recreated stream, whose sequences
  restart` / `// at 1, is never suppressed by the sequences of the stream it replaced (#15). ...`. Same "never".
- `processor/graph-ingest/keyed_ingest.go:35` — `// nanoseconds, read when the consumer was bound (design D8, #15).`
- `openspec/specs/graph-ingest-recovery/spec.md:5-8` (Purpose) — `... (the last stream sequence applied, kept` / `per
  stream generation so that a stream deleted and created again does not inherit it) are committed. ...`
- `docs/contract.md:79-80` — `If a stream is deleted and recreated, re-sending your current data applies it. Old
  sequence numbers do not cause it` / `to be skipped.` The consumer-facing page; states no exception today, for
  either existing limitation.
- ADRs: none (searches above).
- `docs/setup-plan.md:479` mentions "Recreation" of a source, not of a stream; not affected.

### 3c. Archived text

History, listed so the design can say whether any of it is touched.

`openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/`:

- `design.md:581` — ``**The generation belongs to the bound consumer (R3, PR #93 comment 6112098987).** `Start` reads
  each input stream's``; `:593` — `- **Different reads, or a second read that fails.** ...`; `:596` — `- **Accepted
  inputs.** An input the callback accepts is neither submitted nor settled until the second read returns`; `:603-612`
  — the "Not covered" list (restore, `:605-606`; pull that outlives its consumer, `:607-608`; disconnect, `:609-612`);
  `:614-617` — "Rejected: ...".
- `notes/4.3.md:20-21` — `... (task 4.17 adds` / ``a second read after the bind; `notes/cp4i-R3.md`)``; `:50-57` — the
  window case, "Task 4.17 does it ..., and leaves two cases open"; `:58-59` — stream recreated while graph-ingest
  runs; `:60-64` — the disconnect limitation.
- `notes/cp4i-R3.md` (whole file) — task 4.17's notes, mutation records and "What is not covered".
- `tasks.md:690-704` — ``- [x] 4.17 (D) Codex interim review R3 (PR #93 comment 6112098987; design D8, D13):
  `setupJetStreamConsumer` reads`` ... ``Done in `7e07fea`; ...``.
- `specs/graph-ingest-recovery/spec.md:35`, `:62`, `:74-78` — the archived delta carrying the same 4.17 text.

### 3d. Where the two existing stream limitations live today

In current (non-archived) text.

- Spelled out case by case: only in archived text (`design.md:605-606` and `:609-612`; `notes/4.3.md:54-55` and
  `:60-64`; `notes/cp4i-R3.md`, "What is not covered").
- In current text: only `docs/admission-ledger.yaml:3240-3242`, a summary by comment number that points at archived
  design D8. The current spec `graph-ingest-recovery` states neither (`git grep -n -i -e restore -e disconnect -e
  limitation -- openspec/specs` finds `projection-mutation/spec.md:8` and `:47`, and `transport-client/spec.md:127`,
  `:134`, `:281`, `:329`, which use "disconnect" and "restore" in the NATS client's sense; no graph-ingest spec states
  a stream limitation). `README.md` and
  `docs/contract.md` state neither.
- The nearest current precedent for a declared limitation written into a spec requirement is the other half of
  ruling 6113959050 (F1): `openspec/specs/projection-mutation/spec.md:47-51` — `One exception is a declared limitation
  (owner ruling, #91 comment 6113959050). A create refused with` ... and its Purpose, `:8` — `write may or may not have
  been stored), with one declared limitation; ...`. (Category 5, problem shape: "declared limitation in a capability
  spec"; this is the closest existing instance.)

---

## Item 4. Open pull requests

Command: `gh pr list --state open --json number,title,isDraft,headRefName,changedFiles,files --limit 100` (3 open; every
`changedFiles` is under 100, so the lists are whole). Overlap filter: `processor/graph-ingest/`,
`graph-ingest-recovery`, `docs/admission-ledger.yaml`, `docs/contract.md`, `natsclient/owned_delivery.go`,
`natsclient/stream.go`, the archived ingest-kernel change.

| PR | State | Overlap | Detail |
| --- | --- | --- | --- |
| #215 | draft, 0 files | this claim | `Closes #214`. |
| #206 `claude/test-infra-audit` | draft, 64 files | `docs/admission-ledger.yaml` (hunks `@@ -205,22`, `@@ -240,12`, `@@ -1782,9`: the runner and natsclient rows, not graph-ingest's `:3153-3273`); `processor/graph-ingest/query_verbs_integration_test.go:79-84` (stream naming, a file this change does not touch) | Removes `internal/harness/natsfixture/names.go`, including `func (f *Fixture) Name(base string) string` and `CheckName`; the files this change edits call neither (`grep -n 'f\.Name('` on both: none). Capability deltas: harness-boundaries, integration-test-runner, nats-fixture, process-host; none is graph-ingest-recovery. Its own `openspec/changes/test-infra-audit/inventory.md` counts this test file's calls (`CreateMemoryStream` x4; "`JetStream()` used directly: guard_generation 4"); both become 2 after this change. Evidence counts only, no conflict; whichever merges second may leave them. |
| #194 `claude/natsclient-test-seams` | ready, 18 files | `docs/admission-ledger.yaml` (hunks `@@ -1649`, `@@ -2048`, `@@ -2268`, `@@ -2317`, `@@ -2342`: the natsclient row, `:1549-2427`) | No graph-ingest file; does not change `natsclient/owned_delivery.go` or `stream.go`. |

No textual overlap with lines this change edits. Merge order is set by the known-flake rule, not by the files:
while #214 is open, neither #194 nor #206 can merge unless it closes #214 (`.agents/protocol.md` § "Known flakes";
`scripts/merge-check.sh`), and #215 closes it.

---

## Item 5. Open issues

Command: `gh issue list --state open --search "graph-ingest" --limit 200` (34 hits). Each body and comment set was
grepped for `setupJetStreamConsumer`, `waitForStream*`, `consume closure|callback`, `stream generation`,
`creation time`, `second read`, `generationConfirmed`, `recordBindBacklog`, `registerBoundConsumer`,
`failed_start_callbacks`, `guard_generation`, `setupSubscriptions`, `consumeStream`, `graph-ingest-recovery`,
`Settlement order`. Extra searches: `waitForStreamInput` → #214 #139; `"second read"`, `4.17`, `"creation time"`,
`6112550268` → #214 only; `StreamRecreatedBeforeBind`, `generationConfirmed`, `commitConsumer` → none.

Issues that touch this change:

- **#214** (bug, class:flake) — the flake in the going test; ruling comment 6157056960. Retired by #215. Its body also
  names an unconfirmed mechanism (open point 5).
- **#139** (port-refactor, graph-ingest after the write seam) — body: "Fields nothing sets: `waitForStreamInput`
  (nothing sets it here or at the pin), ...". Its later "Dedupe at `88d0d3b`" comment: "`waitForStreamInput`: the
  premise is fixed (`statement_metadata_test.go:79` sets it); the seam stays." Also: "`waitForStream` re-implements
  `pkg/retry`" (D) and moves "the consume closure to `keyed_ingest.go`" (E), both in code this change edits. Stays.
  See open point 4.
- **#15** (bug, status:blocked) — "repair: preserve graph recovery across transport stream generations"; the
  generation-aware guard that stays. It does not name the check. Stays; the third limitation narrows the
  recovery promise its repair produced, in the window case only.
- **#131** (bug) — "internal/dispatch — KeyedPool.SubmitBlocking accepts work after the run context is cancelled and
  never processes it"; "Where graph-ingest is exposed: ... the consume callback is told the message was accepted".
  The post-rollback Nak this change falls back to relies on `Shutdown`'s `stopped` flag, which #131 says is checked
  (`keyed_pool.go:218`); the rollback calls `Shutdown` (`component.go:1139-1141`), so the path #131 describes (abort
  without `Shutdown`) is not the rollback's. Stays.
- **#183** (port-refactor, Health and the backlog latch) — touches `readiness.go` and the latch, not the check; the
  bind-time backlog read (`component.go:1661`) sits just after the removed lines. Stays.

Hits that name the word, not this code: #199, #174, #141, #106, #195, #137, #197, #107, #132, #208, #196, #135,
then #33, #111 (it names `graph-ingest-recovery` for the guard record's decode refusal, not the check), #153, #136,
then #134, #189, #103, #198, #20, #110, #16, #138, #96, #95, #94, #7, #9 and #24.

---

## Item 6. Other callers of the behaviour

- **natsclient's `commitConsumer`** — `natsclient/owned_delivery.go:177-180` — `// commitConsumer hands the started
  native handle to the ownership goroutine. If Close has begun` ... `// held until the handlers have returned and
  Close joins them.`; `:181-198` waits on `o.released` or `ctx.Done()`. natsclient does not depend on graph-ingest's
  hold; the hold depended on natsclient cancelling the handler context when `Close` begins
  (`owned_delivery.go:139-140`, `case <-closing: cancelHandlers()`), which is why it had the `msgCtx` branch (archived
  `design.md:596-601`). With no hold the callback no longer waits on `Start`, so that wait cycle does not exist; no
  natsclient code or test changes.
- **The other post-bind stream read** — `readiness.go:205-210` (`observeOutstandingWork`, `stream, err :=
  js.Stream(ctx, streamName)`), called from `recordBindBacklog` (`readiness.go:178`). Stays; its failure only degrades
  readiness (`:180-196`). 4.17's notes chose not to reuse it (`notes/cp4i-R3.md`, choice 7).
- **Error text** — `git grep` for `read its creation time again`, `while its consumer was bound`, `stream generation
  was not confirmed` outside the archive: only `component.go:1607`, `:1645`, `:1648` and the two going tests' failure
  messages (`failed_start_callbacks_integration_test.go:372`, `guard_generation_integration_test.go:273`). No caller
  matches on it.
- **Stream deletion around Start** — `git grep -n -e DeleteStream -e CreateMemoryStream -e CreateFileStream --
  processor/graph-ingest`: only `guard_generation_integration_test.go` (`:83`, `:116` stay; `:229`, `:262`, `:265` go).
  Nothing else in the tree relies on `Start` failing for a recreated stream.
- **Consumers** — `go.mod` on each default branch: semsource `:8` — `github.com/c360studio/semstreams
  v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`; semconnect `:6` — `github.com/c360studio/semstreams
  v1.0.0-beta.160`. Neither imports semengine, and the pin has no second read (above), so no consumer can rely on
  the `Start` failure today. A consumer qualification branch (#15's body mentions one) was not read.

---

## Adopter seam inventory

graph-ingest is imported by consumers: ledger `:3155-3158`, "consumers import it".

Answers are inferences from the code above; none was run.

1. **What must they know** after the change: do not delete and create again the input stream while graph-ingest is
   starting. If it happens, inputs in the new stream at sequences the old generation's record covers are acknowledged
   without being applied. They are not delivered again: the durable consumer's acknowledgement floor has passed them.
   A later `Start` reads the new generation, which stops further loss but recovers nothing; the producer must publish
   them again after that `Start`. Publishing them again during the same run is dropped again while the new sequences
   are still covered. One new fact, beside the two existing limitations (restore; stream and consumer recreated
   during a disconnect). The nearest named workload is #15's: SemSource with a memory input stream created again
   after a broker restart (`guard_generation_integration_test.go:66-69`; `docs/contract.md:73-79`). It reaches the
   window only if the broker restarts between `Start`'s first read and the consumer's creation and natsclient's
   consumer creation waits through the reconnect instead of failing `Start`; that second condition is not measured.
2. **If they do nothing**: no error. The input is acknowledged and counted as a redelivery,
   `semengine_graph_ingest_redeliveries_dropped_total` (built at `component.go:163-169`, assigned at `:736`, registered
   at `:785`; incremented at `keyed_ingest.go:275-277`), the same counter a true redelivery uses. Before the
   change, `Start` failed naming the stream (`component.go:1647-1650`).
3. **Where they find out**: wherever the declared limitation is written (doc), and a counter whose name says
   "redelivery". Under the contract's ranking that is "doc" for a correctness fact; the ruling already settles it as a
   declared limitation under the owner's #185 rule, so it is recorded here, not reopened.
4. **What they should have to know**: nothing; the gap between 1 and 4 is the declared limitation, by ruling.

---

## Open points the ruling does not settle

For the design and, where marked, the owner.

1. **Settlement order was widened by 4.17 and rests on the hold.** `spec.md:44-46` promises a Nak, never an apply,
   "when its consumer's setup fails for any cause", and the scenario `spec.md:60-64` dropped "after the rollback has
   returned". With no hold, a callback that natsclient runs during a consumer setup that then fails (for example
   `commitConsumer` when `Close` begins during setup, `owned_delivery.go:181-198`; that natsclient can run a callback
   during an acquisition that then fails is the premise of `failed_start_callbacks_integration_test.go:5-7`) reaches a
   live pool and can be applied and acknowledged. The pre-4.17 wording (quoted in 3a) is what the code still holds,
   and it is already reviewed: it is the fix for Codex's interim finding R4, task 4.16 (archived `tasks.md:686-687`,
   "the failed-start Nak is promised only for an input that decodes, is accepted, and is submitted after the
   rollback"; `notes/cp4i-R1.md:64-66`). Keeping the wider promise would need new mechanism, which #185 and the
   ruling's "Why" exclude. No owner question is needed (inventory review, MEDIUM).
2. **Where "beside the two" is.** Neither existing stream limitation is written in a current spec, README or consumer
   page; current text has only the ledger summary (`:3240-3242`) pointing at archived D8. The design must choose the
   home(s); `projection-mutation/spec.md:47-51` is the in-tree precedent for a spec-level declared limitation.
3. **A third "not covered" case has no ruling**: a pull that outlives its consumer's deletion and is later answered
   by a same-named consumer on a new stream (archived `design.md:607-608`, `notes/4.3.md:56-57`, citing nats.go
   v1.54.0 `jetstream/pull.go:225`, `:740`; unprobed here). The ruling names two existing limitations, not this one.
4. **`waitForStreamInput` and #139: settled.** #139 comment 6157272349 (2026-10-13 23:20:25Z) names
   `waitForStreamInput` and `consumeStream` (`component.go:414-420`) as back doors under #186, puts their removal under
   #139 item A ("tests build through `CreateGraphIngest`; delete the hooks"), and records that PR #215 keeps both
   fields and does not schedule their removal.
5. **#214's unconfirmed mechanism.** #214's body offers, unconfirmed, that a failed `Start`'s consumer may hold a
   fetched input until the ack wait (30 s, `component.go:1435`) without its callback Nak'ing it. The reproducing test
   goes with this change; if the mechanism is real it applies to any failed `Start` after a real bind (for example a
   later registration failure). Unmeasured; nats.go behaviour, so a probe at the pinned version or the words "not
   promised" would be needed before any text relies on it. When #215 closes #214 this loses its only record. Under
   the File step as #213 rewrote it (`.agents/protocol.md:104-129`), an unmeasured cost becomes a *Declared cost*
   section in the design (`:126`); an issue only if it is measured as a defect. It is a redelivery after the
   acknowledgement wait, not a loss, so MEDIUM at most under #185.
6. **Absolute wording already contradicted by the existing limitations**: `README.md:31` ("never dropped"),
   `keyed_ingest.go:83` ("is never suppressed"), `spec.md:70-71` ("SHALL never be dropped"), `docs/contract.md:79-80`
   (no exception). Whether this change touches them is a design scope question.
7. **Mechanical consequences** (no decision needed, listed so the task list is complete): `msgCtx` unused
   (`component.go:1551`); three imports unused (item 2); ledger line pins `:3166` and `:3283` drift; `cover:check`
   gates `processor/graph-ingest merged` at 80% (`scripts/cover-check.sh:17`, `:30`), current figure not measured here.
