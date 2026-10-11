# Codex interim review R2 notes

A retried update ends on a write error that may have been stored: `natsclient.KVStore.UpdateWithRetryRead` runs its
callback again only after a revision conflict, a failed read, or `jetstream.ErrNoStreamResponse`.

The finding is Codex's interim record, PR #93 comment 6112098987, R2 (blocking), and its portable probe
`TestReviewCommitAmbiguityGaps/append-retry-after-commit`. The owner ruled option A2 (#91 comment 6112779037). The task
is 4.18. The base is `7e07fea`. The design is the architect's round-2 draft, approved by the reviewer with one MEDIUM
for these notes (the probe sources, below) and NITs 3 to 8, applied.

## What changed

- `natsclient/kv.go`, only the lines named in the coordination note on #177 (PR #177 comment 6112780468): the loop's
  old comment (`:367-370`) is replaced by `rerun`, the one predicate (`IsKVConflictError(err) ||
  errors.Is(err, jetstream.ErrNoStreamResponse)`); the conflict checks after the create and the update (`:428`,
  `:452`) call it; the two returns after them (`:439-441`, `:463-465`) wrap the error in `retry.NonRetryable`;
  `UpdateWithRetryRev`'s doc (`:323-326`) states the rule. Line numbers are the base's. Nothing exported is added.
- `processor/graph-ingest/entity_writes.go:161`: the birth comment names the reruns that remain.
- `processor/graph-ingest/entity_writes_test.go`: see choice 7.
- D23, the `transport-client` requirement "An unconfirmed write ends a retried update", the `projection-mutation`
  scenario "An append's write stands and the entity then changes", the ledger item
  `natsclient-update-unconfirmed-write-ends` with its proving tests, ADR-110's count, task 4.18.

## Choices the design does not spell out

1. **`rerun` is a function value local to the loop**, declared where the stale comment stood. The note on #177 names
   only the loop's lines, and a package-level function would add a hunk outside them. Both checks call it, and M4
   mutates it; there is no inline copy (reviewer NIT 4).
2. **`IsKVConflictError` stays at `kv.go:476`** (base `:469`; reviewer NIT 3). A no-responders error on every run ends
   after `MaxRetries`+1 runs with that write's error, not `ErrKVMaxRetriesExceeded`.
3. **Unchanged lines that now say less than the truth.** The comments "Conflict error - WILL be retried" (`kv.go:434`,
   `:458`) and the debug lines "KV create conflict, retrying" and "KV update conflict, retrying" (`:437`, `:461`) also
   cover a no-responders rerun. They are outside the lines named on #177, so they are left; the log is debug level.
4. **The error the caller gets** is `retry.NonRetryable(fmt.Errorf("kv update failed: %w", err))`: its text reads
   `non-retryable: kv update failed: kv update <key>: <error>`, as a callback error's text already started
   `non-retryable:`. `errors.Is` reaches the write's error; graph-ingest's append maps it to transient `internal`, so
   commit-unknown.
5. **D23's wording.** The rerun clause sat inside "Unchanged from `UpdateWithRetryRev` at `930bf49`"; it is no longer
   unchanged, so it moves to a sentence that starts "Changed by task 4.18". The draft's two texts are otherwise
   verbatim. The old cites `kv.go:369`, `:417`, `:439` (at `930bf49`) go with the old clause. P-R2a is cited to this
   file.
6. **`entity_writes.go:161`** reads "after a create conflict or a create no stream received" where the draft dropped
   the second clause: the loop still reruns after a create's `ErrNoStreamResponse`, and the reused birth covers it.
7. **A test the design did not list failed under A2.** `TestReplaceEntityRetryAfterCreateErrorStoresNoStatementTwice`
   failed the birth's create once with a plain error ("nats: stream temporarily unavailable"), which now ends the
   call: `CAS update failed: non-retryable: kv create failed: nats: stream temporarily unavailable`. It now fails the
   create with `jetstream.ErrNoStreamResponse`, the one error besides a conflict that still reruns, so it keeps
   testing what #91 wrote it for: a retried birth stores no statement twice. The file's comment says the same. M4
   fails it too.
8. **ADR-110's ledger cite** was already stale (`:1599-1605` held other text at the base); it now cites `:1617-1625`
   and `:1639-1642`, at this commit.
9. **T2's wrapper** is armed just before the append, so the create is not touched. It keeps the stored bytes only after
   the other writer's change succeeded, so a non-nil copy proves both happened. The refused value is a valid entity
   under another ID, built on the test goroutine with `graph.MarshalEntityState`; graph-ingest's key check refuses it
   (`checkStoredEntityKey`, `mutation_runtime.go:244-253`).
10. **T1** wraps one bucket in a `FaultKV` per row, with `MaxRetries` 2, so an extra run shows.
11. **Before adding anything new.** No exported symbol; `rerun` is local to its one owner, the loop that ADR-110
    Decision 2 makes every read-modify-write go through. Not a new primitive, so no adoption list. processor/rule's
    `UpdateJSON` (pin `kv_writer.go:117`) inherits the rule when it is ported.

## Probes P-R2a and P-R2b (local only)

Both are in one file the architect wrote, `/tmp/r2-probe/main.go`, local only (not in the repository), sha256
`9068f80c7e6eb11957b6a70422a5ff8ebda7fa2bf98e051ba24cd45671e4ad56`, in a module of its own with nats-server v2.14.7
and nats.go v1.54.0. It starts an embedded JetStream server on an ephemeral port.

- **P-R2a:** puts a key in bucket `P`, deletes the stream `KV_P`, then calls `kv.Update` and `kv.Create` under a 5 s
  context, printing the error, the time taken, `errors.Is` against `jetstream.ErrNoStreamResponse` and
  `nats.ErrNoResponders`, `errors.Is` through two `%w` wraps, and whether the context was still live.
- **P-R2b:** a stream `S` with a durable consumer (`AckExplicitPolicy`, `MaxDeliver` 3, `AckWait` 30 s, no BackOff);
  one message is fetched and Nak'd four times, each fetch waiting up to 3 s.

Re-run once here, 2026-10-11 (`GOFLAGS=-mod=mod go run .` in that directory; exit 0):

```text
P-R2a update: err=nats: no response from stream elapsed=506ms is(ErrNoStreamResponse)=true
  is(nats.ErrNoResponders)=false wrapped-is=true ctxLiveAtReturn=true
P-R2a create: err=nats: no response from stream elapsed=506ms is(ErrNoStreamResponse)=true
  is(nats.ErrNoResponders)=false wrapped-is=true ctxLiveAtReturn=true
P-R2b fetch 1: delivered=1 at=1ms
P-R2b fetch 2: delivered=2 at=1ms
P-R2b fetch 3: delivered=3 at=2ms
P-R2b fetch 4: nothing within 3s (at=3.004s), err=<nil>
```

(The two P-R2a lines are wrapped here; each is one line in the output.) The architect's three runs gave 502 to 507 ms
and the same P-R2b pattern.

## Tests

- `TestUpdateWithRetryRead_UnconfirmedWriteEndsTheCall` (`natsclient/kv_update_revision_integration_test.go`):
  - **update:** `FailAfter(KVUpdate, errLost)`: `(0, err)` with `errors.Is(err, errLost)`, one run, one real update,
    and the raw bucket holds the first run's value above the revision read.
  - **create:** `FailAfter(KVCreate, errLost)`: `(0, err)`, one run, one real create and no update, the key holds the
    first run's value.
  - **no stream response:** `FailBefore(KVUpdate, jetstream.ErrNoStreamResponse)`, cleared by the callback's second
    run: two runs, no error, the bucket's last sequence one higher, the result the stored revision.
- `TestIntegration_AppendAfterUnconfirmedWriteIsCommitUnknown`
  (`processor/graph-ingest/commit_ambiguity_integration_test.go`), Codex's probe adapted, two rows, **deleted** and
  **refused value**: the stored update holds the appended statement; the receipt is commit-unknown; the error is a
  `MutationError` of kind commit-unknown and, by `errors.As` on `*errs.ClassifiedError`, class transient, code
  `internal`. The test logs the receipt, kind, class and code.
- Generated checks: not used. The inputs are a finite set: three kinds of write error (conflict, no stream response,
  any other) on two writes (create, update), each a named row or an existing test. No new parsed input, so no fuzz
  target.

## Evidence

Fail before, on `7e07fea` with the tests added and `kv.go` unchanged (each row run alone; the runner stops at the
first failure):

- T1 update: `kv_update_revision_integration_test.go:407` expected 1 run, actual 3; `:408` expected 1 update, actual
  3; `:412` the stored value `{"v":3}`, not `{"v":1}`.
- T1 create: `:424` `Expected error with "reply lost after the write" in chain but got nil`.
- T1 no stream response: passes, as the design declares.
- T2 deleted: `commit_ambiguity_integration_test.go:374` expected `unknown`, actual `not-committed`; logged `receipt
  not-committed; kind not-found, class invalid, code "entity_not_found"`.
- T2 refused value: `:374` the same; logged `receipt not-committed; kind internal, class fatal, code
  "graph_state_reset_required"`, as the design predicted.

After the change: all rows pass, and the logged outcome of both T2 rows is `receipt unknown; kind commit-unknown,
class transient, code "internal"`.

Mutations, by hand (#129): `cp natsclient/kv.go` to a backup outside the repository, SHA-256 before, the wrong change
by a script, the named test run alone through `task test:integration`, the backup copied back, SHA-256 after. Before
and after were `0de6fa997c3fc25d1d2198eecf70f656f16fa80a12c60a9da1c1260102271ac1` each time.

- **M1**, the update's write error retryable again (`kv.go:472` without `retry.NonRetryable`): detected by T1 update
  (`:407`, 3 runs), T2 deleted (`:374`, `not-committed`, `entity_not_found`) and T2 refused value (`:374`,
  `not-committed`, `graph_state_reset_required`).
- **M2**, the create's write error retryable again (`kv.go:448`): detected by T1 create (`:424`, no error).
- **M3**, a conflict ends the call (`kv.go:468` returns `retry.NonRetryable(err)`): detected by
  `TestUpdateWithRetryRead_ConflictPassesTheNewerRevision` (`:313`, `kv: max retries exceeded`).
- **M4**, `rerun` without the `ErrNoStreamResponse` clause (`kv.go:376`): detected by T1 no stream response (`:449`,
  `non-retryable: kv update failed: kv update unanswered: nats: no response from stream`) and by
  `TestReplaceEntityRetryAfterCreateErrorStoresNoStatementTwice` (`entity_writes_test.go:107`).

Gates on the uncommitted tree, each its own command: `task fmt:check`, `task build`, `task vet`, `task lint`,
`task spec:check`, `task docs:check`, `task ledger:check`, `task test:unit` and `task test:integration`, each exit 0.
`task verify` runs on the commit that holds these notes, so its result is reported with that commit.

## What is not covered

- How often a quick write error that can follow a stored write happens with real nats.go. The reviewer's reading of
  the source: a malformed acknowledgement (`ErrInvalidJSAck`) is the only one found; ack refusals and no-responders
  store nothing, and after a closed connection the rerun's read fails too. Not measured.
- The stream-lane cost, declared in the ruling: such an error now ends the call, and the arrival gets three immediate
  redeliveries and is then dropped with nothing in graph-ingest noticing (P-R2b), where each delivery retried for up
  to 5 s. No test.
- An append whose write stood and errored with no other writer now reports commit-unknown where it reported verified
  (`notes/4.1.md`, "Append's retry loop"). T1's update row shows the call ends; no graph-ingest test of that case.
- Whether a real leader change shows up as no-responders: not measured; the basis is nats.go's comment at
  `publish.go:252`.
- nats.go's own `Create` writes a second time when its first write fails and the key then reads as deleted
  (`jetstream/kv.go:1072-1079`), so "no run of this call wrote" is claimed for `Update` only.
- #174 stays open: the bookkeeping skipped after an unconfirmed write (`notes/4.1.md`, lines 80-83).
- The comments and debug lines of choice 3.
