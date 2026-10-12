# Codex interim review R3 notes

An input carries the generation of the stream its consumer is bound to: `Start` reads the stream's creation time again
after the bind, holds accepted inputs until that read returns, and fails on a mismatch.

The finding is Codex's interim record, PR #93 comment 6112098987, R3 (blocking), and its portable probe
`TestReviewGenerationChangesBeforeBind`. The task is 4.17. The base is `f9ac34f`. The design is option A of the
architect's round-2 draft, approved by the reviewer with four MEDIUMs (two test-double fixes, applied here; two
owner-question texts). The owner ruled the single-server restore a declared limitation (#91 comment 6112550268).
R2 is designed separately and is not touched here.

## What changed

All in `setupJetStreamConsumer` (`processor/graph-ingest/component.go`): a second read of the stream's creation time
after the consumer's handle is recorded and before readiness registration; one channel, `confirmed`, closed by a
`defer` when the function returns; one flag, `generationConfirmed`, set before that close only when the two reads are
equal; and in the consume closure, after the input is accepted, a two-way `select` on `confirmed` and the delivery's
context. 35 lines added and 5 removed, about 24 of them code; nothing exported, no new symbol beyond locals.

## Choices the design does not spell out

1. **The hold ends when the function returns, not the moment the read returns.** One `defer` closes `confirmed` on
   every return path, so no error return can leave it open (M4 is what a second close site costs). On success that is
   after `registerBoundConsumer` and `recordBindBacklog`, one more stream and consumer read. The backlog it captures is
   the consumer's pending plus acknowledgement-pending count, and a held input is pending either way. D8's text holds:
   no input is submitted or settled before the second read returns.
2. **The flag is read only in the `confirmed` branch.** The channel's close orders the write before the read; the
   context branch never reads it, so the race detector has nothing to report. When both are ready the `select` may
   take either; a Nak on the context branch is what D8 states for an ended context.
3. **A held input that is not confirmed is Nak'd with no counter.** A Nak is a redelivery, not a drop; the failed
   `Start` reports itself through its error. As with the submit-failure Nak beside it, only a failed Nak is logged.
4. **The reads are compared with `time.Time.Equal`**, the same instant the generation's `UnixNano` encodes. A zero
   second read differs from the first, which is never zero, and fails `Start`.
5. **Error text.** A mismatch: `stream <name> was deleted and created again while its consumer was bound (created <t1>,
   then <t2>)`, both in RFC 3339 with nanoseconds. A failed read: `stream <name>: read its creation time again after
   binding: <err>`. `setupSubscriptions` and `Start` wrap both with the port name.
6. **Design text.** D8's "Rejected" line names the options in words, because the draft's option letters are not in
   `design.md`. D8 cites the stream wait at `component.go:1647-1682`, its lines after this change; the draft cited the
   base's. The first-read comment in `setupJetStreamConsumer` no longer calls the window an exception.
7. **Before adding anything new.** The generation's owner is the first read in this function; the second read goes
   through the same `waitForStream` seam. `readiness.go:207-208` reads the stream after the bind too, but its failure
   only degrades readiness (design A2), so it is not reused. Not a reusable primitive: no adoption list.

## Tests

- `TestConsumerInputHeldUntilGenerationConfirmed` (`guard_generation_test.go`), three cases, each in its own
  `testing/synctest` bubble with the unconnected client (`NewClient("")`). The component, its pool and an
  applied-sequence mock bucket are built inside the bubble; the pool is shut down by a cleanup inside it. Each goes
  through `setupSubscriptions`, with a `consumeStream` double that runs the callback on its own goroutine.
  - **confirmed**: the second read blocks; after `synctest.Wait` nothing is settled and no key is written; after the
    release, setup returns nil, one ack, and the record is (first generation, 1).
  - **changed**: the second read returns another time; setup fails naming `ENTITY`, one Nak, both buckets empty.
  - **delivery context ends**: the double models `commitConsumer` when `Close` begins during setup
    (`natsclient/owned_delivery.go:193-206`): it waits with `synctest.Wait` until the callback is held, cancels the
    callback's context, and then waits for the callback to return or for setup's own context (one minute on the
    bubble's clock) to end, recording which. The test asserts "the handler returned" (reviewer MEDIUM 2), the error,
    and one Nak.
- `TestFailedStartDrainsAndNaksHeldInput` (`failed_start_callbacks_integration_test.go`), two cases through `Start` on
  a fixture broker, with a `ConsumeContext` double whose `Drain` is counted and closes `Closed`. The callback gets
  `t.Context()`, which the rollback does not end, as natsclient's client-owned handler context (reviewer MEDIUM 1).
  - **generation changed**: `Start` fails naming `ENTITY`; one drain; one Nak.
  - **a later registration fails**: every request registration fails; the input, held in `Metadata`, is released
    after the rollback, reaches the shut-down pool `Start` captured, and is Nak'd. Holds on the base; it keeps the
    spec's second clause tested now that T2 goes through the hold.
- `TestFailedConsumerSetupCallbackNaksAfterRollback` (T2) hands its callback `t.Context()` instead of `Start`'s
  context (reviewer MEDIUM 1): with `Start`'s context, M4 still ended in a Nak through that context.
- `TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart` (`guard_generation_integration_test.go`), Codex's
  probe adapted: three inputs at sequences 1 to 3 are applied; a second component's first read returns, the hook
  deletes and recreates the stream and publishes `current` at sequence 1. `Start` fails naming `ENTITY`, the consumer's
  acknowledgement floor is 0, and a fresh component's `Start` applies it: the entity query returns `[current]`.
- Generated checks: not used. The hold has three outcomes (confirmed, changed, context ended), each a named case, and
  the order that matters (the input accepted before the second read returns) is forced by the doubles. Fuzzing: no new
  parsed input.

## Evidence

Fail before, on `f9ac34f` with the tests added and `component.go` unchanged:

- `go test -race -count=1 -run '^TestConsumerInputHeldUntilGenerationConfirmed$' ./processor/graph-ingest/`:
  confirmed at `guard_generation_test.go:365` (`the input was settled (ack) before Start read the generation again`);
  changed at `:407` (`setupSubscriptions error = <nil>`), `:411` (`= ack, want nak`) and `:414` (`2 keys written`);
  delivery context ends at `:465` (`= ack, want nak`).
- `task test:integration -- '-run=^TestFailedStartDrainsAndNaksHeldInput$' -v ./processor/graph-ingest/`: generation
  changed at `failed_start_callbacks_integration_test.go:372` (`Start returned nil`). The run is fail-fast, so the
  second case ran alone (`.../^a_later_registration_fails$`) and passed, as declared.
- `task test:integration -- '-run=^TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart$' -v
  ./processor/graph-ingest/`: `guard_generation_integration_test.go:273`, `An error is expected but got nil`.
- T2 with the changed context passed on the base.

After the change, uncommitted, each its own command with exit 0: `task fmt:check`, `task build`, `task vet`,
`task lint`, `task test:unit`, `task test:integration` (graph-ingest 84 s), `task spec:check`, `task docs:check`.

## Mutation records

Unit cases: `task mutate:check -- -pkg ./processor/graph-ingest -test
TestConsumerInputHeldUntilGenerationConfirmed/<case> -file processor/graph-ingest/component.go -mutant <copy> -expect
guard_generation_test.go:<line>`, the copy kept outside the repository, with the change uncommitted. Each printed
`verdict: detection: each of the 3 mutant runs failed the named test with an expected line, and the after-run passed`
and exited 0. SHA-256 of `component.go` `f58994e7…cd6d4bbc` before and after.

| Mutant | Wrong change | Case | Expected line | Verdict |
| --- | --- | --- | --- | --- |
| M1 | the comparison always passes (`again.Equal(again)`) | changed | `:407` | detection |
| M2 | no hold (a `default` branch confirms at once) | confirmed | `:365` | detection |
| M3 | a mismatch sets `generationConfirmed` before returning | changed | `:411` | detection |
| M6 | the hold ignores its context (no `msgCtx.Done()` branch) | delivery context ends | `:458` | detection |

Integration cases, by hand (#129: `cp` backup, the mutant copied in, one `task test:integration` run, the backup
restored; SHA-256 `f58994e7…cd6d4bbc` before and after each):

| Mutant | Wrong change | Test | Failed at |
| --- | --- | --- | --- |
| M4 | no `defer`; `confirmed` closed after the flag is set | T2 | `:286`, context deadline exceeded, 60.19 s |
| M5 | the second read before the handle is recorded | generation changed | `:378`, drained 0 times, want 1 |
| M1 | as above | broker test | `guard_generation_integration_test.go:273`, no error |

## What is not covered

- A single-server restore inside the window, which keeps the creation time (nats-server v2.14.7
  `server/stream.go:9331-9333`): a declared limitation by the owner's ruling (#91 comment 6112550268).
- A pull that outlives its consumer's deletion and is later answered by a same-named consumer on a new stream
  (nats.go v1.54.0 `jetstream/pull.go:225`, `:740`).
- A second read that fails (rather than differs) has no test. It returns before the flag is set, as a mismatch does,
  and the `defer` closes the hold on that path too.
- That the error names both creation times is not asserted; the tests check the stream's name.
- The 30 s message timeout ending a held input's context is not run; case 1c ends it by cancelling, as `Close` does.
- natsclient's real `commitConsumer` path is modelled by a double, not run; the broker test runs the second read on a
  real server through the first-read seam.
- Each integration mutant ran once, not three times.
