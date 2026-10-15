# Design: simplify-04a-06-projection

Status: draft, not reviewed. Its pre-owner review waits for `INVENTORY PASS` on `inventory.md` (base `a9aaf68`).

## Context

Issue #138, items 1–5 and comment items a–b; the owner's ruling on #9 (comment 6097814764, Q7) released items 1–2 to
land after #93. Two terms used below: a **pre-write refusal** is a reply code that graph-ingest sends only before it
writes (`isDefiniteFailure`'s list, `mutation_client.go:459-462`); a **transport outcome** is
`graphmutation.TransportError.Outcome`.

## Options for the shared mapping (item a / item 4)

1. **Export one projection function (recommended).** Lifecycle (9b) and consumers stop building `MutationError` by
   hand. Cost: one exported symbol.
2. **Move the code list into `graphmutation.IsDefinitelyNotCommitted`.** No new exported symbol, but it edits PR 8's
   package, and the code-to-kind mapping still exists twice, because lifecycle would still build the error by hand.
3. **Do nothing.** 9b would then copy the code list into lifecycle (a second spelling), and #257 would wait.

## Decisions

### D1 One mapping from a request's error to its commit state

`ClassifyMutationError(operation MutationOperation, err error) *MutationError` returns nil for nil. Otherwise:

| The error's chain holds | Commit | Kind | Class |
| --- | --- | --- | --- |
| a classified reply with a pre-write refusal code | not committed | by code (invalid, not-found, conflict, revision-conflict, internal), as today | the reply's |
| transport `unavailable`, or raw `nats.ErrNoResponders` | not committed | unavailable | transient |
| transport `deadline` (context done before send) | not committed | unavailable (Q3) | transient |
| anything else, including a classified reply with another code | unknown | commit-unknown | fatal, cause held as today |

`Code` and `Detail` are the reply's (`Detail` is copied with `maps.Clone`). `Operation` is the argument. `Kind` is
computed once. Proving test: `TestClassifyMutationErrorMapsEachOutcome` in package `projection_test`, one row per
table line and per refusal code, run the way a product calls it. `TestTransportOutcomesKeepTheirCommitState` gains a
row for "context ended before send" (Create, Append and Delete send nothing and read not committed and
`errors.Is(err, context.Canceled)`). Reconcile is excluded from that row: it reads first and hits D-1. Adopters
(inventory § 5): `pkg/lifecycle/graph_emit.go:88-115` in 9b; semteams at cutover.

### D2 The commit state lives on the error

`MutationReceipt` drops `Commit`. Each method returns a zero receipt with any error. A nil error means the commit is
verified. Proving tests: the existing receipt assertions move to the error's `Commit`, and the spec scenarios change to
match (spec delta). The integration file's eight `receipt.Commit` reads move the same way, under
`go test -tags integration ./processor/graph-ingest/`.

### D3 Unread surface goes

The five interfaces and the revive directives are removed. Proof: `task build` and `task lint` pass; no reader exists
(inventory § 1).

### D4 Dead layers go; behaviour stays

`canonicalizeTriples` returns `([]message.Triple, error)`. `canonicalizeGroupMutation` drops its metadata return.
The `Create` entity check, `Append`'s `default:`, `isNilRequester` and the sort in `cloneDetail` are removed.
`NewMutationClient` refuses a nil NATS client with a plain error, the same shape as every other constructor error
(item b). Proof: every existing test passes unchanged except for the D2 assertions. New
`TestCreateReplyNamingAnotherEntityIsCommitUnknown` and an append "unknown outcome" row pass both before and after,
which shows graphmutation is the guard. Each fails under a mutant that drops graphmutation's check (`task mutate:check`).

### D5 One contract clone

`contract.Contract.Clone()` returns a copy whose slices are independent of the original. `pkg/projection` and
`payloadregistry` both call it. The leaf's ledger row changes from `carry` to `adapt`, and its stale `known_risks` is
fixed (Q2). Proof: `TestContractCloneIsIndependent` (mutating the clone's slices leaves the original unchanged); the
existing registry and client tests.

### D6 The aliases stay

Four consumers write `projection.Contract` literals in production. Pin ADR-103 Decision 5 promises those literals keep
compiling. `TestContractLiteralCompilesAgainstAliases` pins that. Q7's premise ("nothing reads them") holds for the
interfaces only (Q1).

### D7 Declared failure outcomes (item 5)

Each exported method's doc says what it refuses before sending (invalid, not committed), that its mapped failures
follow D1, and, for `Reconcile`, that it reads first and a failed read reports operation `read-authoritative`. Review
only; D1's spec requirement is the enforced half.

Generated checks: not used. The mapping's inputs are a finite set of codes and transport outcomes, and the table
covers each one (`docs/testing.md`).

## Not in this change

Lifecycle's adoption of D1, its `ErrEmitFailed` path, and the `default:` of `lifecycleMutationOperation` (9b,
issues #154 and #257). Defects D-1 and D-2 (inventory § 7; to be filed). #258 (`pkg/errs`). #132 item 3 (PR 8).

## Merge order

No open PR shares a code file. If #266 edits `docs/admission-ledger.yaml`, whichever PR lands second merges
`origin/main` first. 9b follows this PR.

## Owner questions

1. Keep the aliases (D6), even though Q7 released them, because the ruling's premise is false for them? Removing them
   would change ADR-103 Decision 5 and the literals of four consumers.
2. Add `Contract.Clone` to the carried leaf (its row goes `carry` → `adapt`), and switch `payloadregistry` to it here,
   a two-line change in PR 3's package? The alternative is to keep both copies.
3. Report a caller's context that ended before sending as `MutationUnavailable`, widened to mean "not delivered" (no
   new constant), rather than adding a new kind?
4. File D-1 (a failed read in `Reconcile` reported as commit-unknown) and let it ride here, since D1 is the same
   function? Otherwise it waits for its own PR.
