# Checkpoint 3 B2 notes: no request or stream callback work after a failed Start

The finding is Codex's checkpoint 3 record, PR #93 comment 6099921473, B2. The approved design is the B2 draft, round
2, posted as PR #93 comment 6110219686; its sentence is in design D13. The task is 4.10. The files are
`processor/graph-ingest/component.go`, `query.go`, `mutation_runtime.go`, and the tests in
`failed_start_callbacks_integration_test.go`.

## Choices the design does not spell out

1. **One helper does the seam and the check.** `subscribeForRequests` (`component.go`) picks the Client's
   `SubscribeForRequests` or the test seam `subscribeRequests`, and wraps the handler in the flag check. Both
   registration sites call it (`query.go`, `mutation_runtime.go`), so a request subscription made through it cannot
   skip the check. Enumeration: before the change graph-ingest called `SubscribeForRequests` at exactly those two
   sites; after it, `git grep -n SubscribeForRequests -- processor/graph-ingest ':!*_test.go'` lists only the helper
   and the seam's comment. At the mutation site the check wraps `meteredCanonicalMutation`, which wraps
   `meteredMutation`, so a refusal is never metered.
2. **The flag.** `startCompleted`, an `atomic.Bool`, stored as the last statement of `start`, after the "component
   started" log line and before `return nil`. It is never cleared: the guard runs `start` once (D13), and Stop's drain
   must still serve, which is why `running` (cleared by `release`) is not reused.
3. **The refusal.** `errs.WrapTransient(errs.ErrNotStarted, "Component", "request", "serve "+subject)`. Its text reads
   `Component.request: serve SUBJECT failed: component not started`. It carries no code (ADR-060's code set is
   closed), so the requester sees a transient error with no code.
4. **R2's capture.** `pool := c.ingestPool` is read in `setupJetStreamConsumer`, on Start's goroutine, after
   `buildIngestPool` wrote it on the same goroutine. `submitCtx` was already a parameter of that function, so only the
   pool changed. The closure's other reads (`c.logger`, `c.errors`, `c.ingestLag`, `c.decodeEntity` with `c.decoder`,
   the rejection metrics) touch nothing `clearLifecycleHandles` clears; `extractEntityFromMessage` reads no component
   field. That is from reading the code, as in the draft.
5. **Two existing tests stand in for Start.** `TestIntegration_TypedClientPreservesCommitAmbiguity` and
   `TestIntegration_PrefixQuery_IndivisibleEntityTooLarge` subscribe the handlers themselves, without `Start`. They now
   set `startCompleted` before subscribing; without it their requests are refused.
6. **T1's shape.** On the failing subject the seam seeds what the case needs (the query case writes the entity to
   ENTITY_STATES, which `Start` has opened by then), runs the handler it was given on goroutine G, waits for G's return
   or `repopulateHook`'s entry, and returns a transient `DeadlineExceeded` and no subscription. The hook blocks until
   the test releases it after `Start` has returned, so on the base G's later reads come after the rollback and the run
   fails on assertions, not on a data race. The metrics are checked as zero series (`testutil.CollectAndCount`) on a
   fresh component's vectors, which hold no series until a request is metered; this needs no new import.
7. **T2's shape.** The message is a valid Graphable (`mergeTestGraphable` under the fixture's authority, decoded by
   `registerMergeTestPayload`), so the closure reaches its submit (round-2 review). The double waits until the
   `Metadata()` hold is entered before returning its error, and `calls` refuses a second bind.
8. **Bounds.** Each test runs under a 60 s context taken from `t.Context()`; a timer is used only as a failure bound
   (`docs/testing.md`, "Sleep replaced by a signal"). No sleeps.
9. **Generated checks.** None: the inputs are two registration kinds and one consumer, with no grammar, law or
   history; the two named examples are the cases.

## Fail before

Integration, `task test:integration -- '-run=^<test>$' -v ./processor/graph-ingest/`, each test on its own. T2 ran on
`2fe8e32` unchanged. T1 ran on `2fe8e32` with only the `subscribeRequests` seam and `subscribeForRequests` without
its check, so registration behaved as before.

- `TestFailedRegistrationRunsNoRequestWork/query`: `:191`, "the query read the entity before Start completed:
  repopulateHook was entered"; `:204`, "handler error = context canceled, want it to match errs.ErrNotStarted".
- `TestFailedRegistrationRunsNoRequestWork/mutation`: `:204`, `handler error = <nil>, want it to match
  errs.ErrNotStarted`; `:208`, "want a transient classified error"; `:125`, "mutation outcome series = 1, want 0";
  `:131`, "ENTITY_STATES holds c360.platform.robotics.gcs.drone.b2: the mutation ran before Start completed".
- `TestFailedConsumerSetupCallbackNaksAfterRollback`: `:285`, "the callback panicked after the rollback: runtime
  error: invalid memory address or nil pointer dereference".

After the change both tests pass, with the two adjusted tests.

## Mutation records

By hand, because `task mutate:check` reports integration tests as inconclusive (#129). Each mutant was a copy of
`component.go` made outside the repository, copied over the tree file for the runs and replaced by the `cp` backup
after them. The tree file's SHA-256 was `a896e6f2017a725f769bd168e6377ea4435595da3bc9e6beaa011b8a97d4672f` before and
after each mutant's runs.

- M1, the wrapper's flag check removed (mutant `1e0927833e81ec28b1c610e0fbc11a17edf95248f8c38320e1dcbc582260984c`):
  **detection**. The query case failed three of three runs at `:191` and `:204`; the mutation case failed three of
  three at `:204`, `:208`, `:125` and `:131`, with the messages above.
- M2, the closure submits to `c.ingestPool`, not the captured pool (mutant
  `d84d74412064a248b25c9d03b30e66816e3b8961f682428f81acdad27db6627f`): **detection**. T2 failed three of three runs at
  `:285`, the nil-pointer panic above.

## Not covered

- The requester's side: no test sends a request over the wire before `Start` completes and reads the transient,
  code-free error `RequestClassified` returns. It rests on natsclient's reply contract.
- The check moved inside the metering, and the flag stored earlier in `start`, were not run as mutants. T1's mutation
  case would see a metered refusal, and a flag stored before a registration would let its case's handler run.
- The natsclient side (a component on a shared client does not own its Close) is not touched here; the orchestrator
  puts it on the #170/#177 claim.
