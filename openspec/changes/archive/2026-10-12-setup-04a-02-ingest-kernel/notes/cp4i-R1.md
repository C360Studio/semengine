# Codex interim review R1 and R4 notes

Only graph-ingest's pre-write refusals read as not committed; the failed-start Nak is promised for accepted input.

The findings are Codex's interim record, PR #93 comment 6112098987, R1 (blocking) and R4 (medium), and its portable
probe `TestReviewCommitAmbiguityGaps/oversized-response`. The task is 4.16. The base is `409c671`. R1 is the
defect #173 describes; R2 and R3 are designed separately and are not touched here.

## The codes graph-ingest sends before its write

Read from the four mutation-lane handlers (`processor/graph-ingest/canonical_mutations.go`) and their helpers
(`mutation_runtime.go`, `entity_writes.go`, `authority_gate.go`), with `gopls references` on each code constant.
Each is sent only on a path that has not called the bucket's write, or whose write the bucket refused.

Bare line numbers are `canonical_mutations.go`'s. The spec's kind follows each code.

- **An invalid request.**
  - `invalid_request` (invalid), all four operations: decode and validation (`:229-281`, `:302-323`, `:375-381`,
    `:463-472`); the statement-metadata rule (`entity_writes.go:70`, `:99`); `rejectFromError` of an invalid-class
    error (`mutation_runtime.go:273`).
  - `message_type_unregistered` (invalid), create: the registry gate (`:219`).
  - `entity_id_invalid` (invalid), create: `authorizeSubject` parsing the ID (`authority_gate.go:51`,
    `pkg/types/entity_id.go:150`).
  - `entity_id_authority_invalid` (invalid), all four: `authorizeSubject` (`pkg/types/entity_id_authority.go:70`).
  - `structural_invalid` (invalid): not on the mutation lane; the stream lane's predicate gate
    (`mutation_runtime.go:179`).
- **A revision conflict.** `revision_mismatch` (invalid): reconcile (`:335`, `:354`), delete (`:488`, `:494`).
  `entity_already_exists` (invalid): create, the bucket's create refused (`:286`).
- **An entity not found.** `entity_not_found` (invalid): reconcile (`:329`, `:359`), delete (`:483`, `:499`).
- **A stored value refused.** `graph_state_reset_required` (fatal): reconcile's read (`rejectFromError`,
  `mutation_runtime.go:270`); an append subject's failure (`:535`).

Not on the list, so commit-unknown: `internal` (transient, `rejectInternal`), natsclient's `response_too_large`
(invalid, `natsclient/request.go:455-468`, sent after the handler returned a success response), any other code, and an
invalid reply without a code. Reconcile and delete check the ID before `authorizeSubject` (`:305`, `:466`), so a
malformed ID there is `invalid_request`; an append's ID is checked by the state contract first (`:381`). An append's
not-found subject is an outcome the client maps itself (`mutation_client.go:244`), not through `isDefiniteFailure`.

## Choices the finding does not spell out

1. **A switch on the code, not on class and code.** graph-ingest pairs each code with one class, so the code alone
   names the refusal; a second condition would need its own test and say nothing more. No new symbol, exported or
   not: the list is the existing constants of `graph` and `pkg/types`, and natsclient's private code is not spelled
   in production.
2. **`structural_invalid` stays on the list** though no mutation-lane handler sends it: its contract
   (`graph/mutation_responses.go:34-38`) is a pre-write refusal by graph-ingest, the existing table row
   (`commit_ambiguity_test.go`) already pins it as not committed, and dropping it would change behaviour the finding
   does not ask about.
3. **`entity_already_exists` is the spec's "revision conflict"**: the bucket's create is a write on the condition
   that the key is absent (design D15's mode table, "create"), refused like a revision mismatch. The spec delta needs
   no change; its governing clause is "proven to precede any storage effect".
4. **The entity ID codes are added**: under the class rule they were not committed already; without them the
   allowlist would turn a refused create into commit-unknown. Their two rows hold on `409c671` and pin the list.
5. **Tests through the production client.** The rows extend
   `TestClassifiedReplyIsNotCommittedOnlyWhenRefusedBeforeWrite` (every row runs Create, Reconcile, Append and
   Delete through `newMutationClient`), not a direct call of
   `isDefiniteFailure`. The test spells `response_too_large` once, as a test constant naming natsclient's
   `errorCodeResponseTooLarge`, as `processor/graph-ingest`'s query tests already do.
6. **The integration test** is Codex's probe narrowed to the oversized case, beside
   `TestIntegration_TypedClientPreservesCommitAmbiguity`, over the plain bucket. It proves the commit before the
   receipt (a later revision, the state statement cleared, the stored value below half the payload limit) and that
   the reply is natsclient's (class invalid, `response_too_large`), so it cannot pass on another failure. Its bound
   is 120 seconds, setup included, as the probe needed.
7. **R4** qualifies the requirement as its scenario does, and says that an input the callback settles before the
   submit keeps its own settlement: the callback acks missing JetStream metadata (`component.go:1535`) and terminates
   invalid payloads (`:1557`, `:1566`) before the pool. Wording only.
8. **Text that stands.** Task 4.1 and `notes/4.1.md` describe `7f00244`'s class rule and the declared gap; they are
   that commit's record, and 4.16 supersedes them. `isDefiniteFailure`'s comment no longer declares the gap.

## Evidence

- Fail before, on `409c671` with the tests added: the three new rows failed at `commit_ambiguity_test.go:106` for all
  four operations, `receipt commit "not-committed", error commit "not-committed", kind "invalid"; want "unknown",
  "unknown", "commit-unknown"`.
  `task test:integration -- '-run=^TestIntegration_OversizedResponseAfterCommitIsCommitUnknown$' -v
  ./processor/graph-ingest/` failed at `commit_ambiguity_integration_test.go:249` (`expected: "unknown"`, `actual
  : "not-committed"`), `:250` and `:251` (`expected: "commit-unknown"`, `actual : "invalid"`); the commit proof and the
  class and code assertions before them held.
- After the fix: `go test -race -count=1 ./pkg/projection/...` passes; the two commit-ambiguity integration tests pass
  through `task test:integration`.

## Mutation records

Each `task mutate:check -- -pkg ./pkg/projection -test
TestClassifiedReplyIsNotCommittedOnlyWhenRefusedBeforeWrite/<row> -file pkg/projection/mutation_client.go -mutant
<copy> -expect commit_ambiguity_test.go:106`, the copy kept outside
the repository, at commit `409c671` with the change uncommitted; target SHA-256 equal before and after. Each printed
`verdict: detection: each of the 3 mutant runs failed the named test with an expected line, and the after-run passed`
and exited 0.

| Wrong change | Row | Verdict |
| --- | --- | --- |
| The class rule of `409c671` restored | `response_too_large_after_a_commit` | detection |
| `entity_id_authority_invalid` dropped from the list | `entity_ID_authority_refused` | detection |
| `graph_state_reset_required` dropped from the list | `stored_value_refused` | detection |

The integration test, by hand (#129: the edit in place after a `cp` backup, restored; SHA-256 of
`mutation_client.go` `739c98f3…53503bf` before and after; one run through `task test:integration`): with the class
rule of `409c671` restored it failed at `:249`, `:250` and `:251`, as on the base.

## Not covered

- An append whose retry follows an unconfirmed write can still send a pre-write code for a write that happened (R2);
  that is R2's design, not this list's.
- That an invalid-class error from the bucket's create or update (`rejectFromError`, `:289`, `:362`) is always a
  refusal before the write rests on natsclient's KV errors; it was not re-audited here.
- Rows for `invalid_request`, `revision_mismatch`, `entity_not_found` and `entity_already_exists` hold but were not
  each mutation-checked; one dropped code of each class (invalid, fatal) was.
- The integration mutant ran once, not three times.
