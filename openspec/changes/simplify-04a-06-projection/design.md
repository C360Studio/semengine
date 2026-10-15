# Design: simplify-04a-06-projection

Status: draft. Inventory review passed (`INVENTORY PASS`, findings applied); pre-owner design review pending.
Inventory: `inventory.md` (base `a9aaf68`).

## Context

Issue #138, items 1–5 and comment items a–b. The owner's ruling on #9 (comment 6097814764, question 7) released
items 1–2 to land after #93. Two terms used below: a **pre-write refusal** is a reply code that graph-ingest sends only
before it writes (`isDefiniteFailure`'s list, `mutation_client.go:459-462`); a **transport outcome** is
`graphmutation.TransportError.Outcome`. D1 and D2 keep ADR-091's outcomes (pin `:69`, `:100`): a lost reply after
possible delivery is commit-unknown and is not retried. ADR-111 needs no amendment: D2 moves where the commit state
is reported, not what is decided, and ADR-111's `:66` cite is pinned to `522bf3a`.

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
| transport `deadline` (the context ended before sending) | not committed | unavailable | transient |
| transport `commit_unknown` wrapping natsclient's `ErrNotConnected` or `ErrCircuitOpen` | unknown | commit-unknown | fatal (unchanged until #268) |
| anything else, including a classified reply with another code | unknown | commit-unknown | fatal, cause held as today |

`Code` and `Detail` are the reply's (`Detail` is copied with `maps.Clone`). `Operation` is the argument. `Kind` is
computed once. `ReadAuthoritative` keeps its own rule and does not go through D1: a read writes nothing, so a
classified read reply is not committed as today, and every read failure is not committed (#267, owner question 3).
A context that ended before sending stays `MutationUnavailable`, with no new kind. That is what
today's code reports, and semboids retries on it (inventory § 4). The constant's doc comment changes to "the request
was not delivered: no responder took it, or the caller's context ended before it was sent".

Proving test: `TestClassifyMutationErrorMapsEachOutcome` in package `projection_test`, one row per table line and per
refusal code, run the way a product calls it. `TestTransportOutcomesKeepTheirCommitState` gains a row for "context
ended before send": Create, Append and Delete send nothing and read not committed, `MutationUnavailable`, and
`errors.Is(err, context.Canceled)`. Reconcile is excluded from that row because it reads first (D-1).

Who it is for: framework API. Its contract line is the spec delta's new requirement. Readers: `pkg/projection`'s
methods (this change), `pkg/lifecycle/graph_emit.go:88-115` (9b, #257), and semteams
`approvalpause/projection.go:160-172` (at cutover).

### D2 The commit state lives on the error

`MutationReceipt` drops `Commit`. Each method returns a zero receipt with any error, and a nil error means the commit
is verified. Proving tests: the existing receipt assertions move to the error's `Commit`, and the spec scenarios change
to match (spec delta). The integration file's nine `receipt.Commit` reads move the same way, under
`go test -tags integration ./processor/graph-ingest/`. No consumer reads the field (inventory § 4).

### D3 Unread surface goes

The five interfaces and the revive directives are removed. Proof: `task build` and `task lint` pass; no reader exists
(inventory § 1, § 4).

### D4 Dead layers go; behaviour stays

`canonicalizeTriples` returns `([]message.Triple, error)`. `canonicalizeGroupMutation` drops its metadata return.
The `Create` entity check, `Append`'s `default:`, `isNilRequester` and the sort in `cloneDetail` are removed.
`NewMutationClient` refuses a nil NATS client with a plain error, the same shape as every other constructor error
(item b). Proof: every existing test passes unchanged except for the D2 assertions. New
`TestCreateReplyNamingAnotherEntityIsCommitUnknown` and an append "unknown outcome" row pass both before and after,
which shows graphmutation is the guard. Each fails under a mutant that drops graphmutation's check (`task mutate:check`).

### D5 One contract clone

`contract.Contract.Clone()` returns a copy whose slices are independent of the original. Its readers are
`pkg/projection/mutation_client.go:94` and `payloadregistry/registry.go:192`, `:241`. The leaf's ledger row changes
from `carry` to `adapt`, and its stale `known_risks` is fixed (owner question 2). Proof:
`TestContractCloneIsIndependent` (mutating the clone's slices leaves the original unchanged), plus the existing
registry and client tests.

### D6 The aliases stay

Four consumers write `projection.Contract` literals in production. Pin ADR-103 Decision 5 promises those literals keep
compiling, and `TestContractLiteralCompilesAgainstAliases` pins that. The release's premise ("nothing reads them")
holds for the interfaces only (owner question 1).

### D7 Declared failure outcomes (item 5)

Each exported method's doc says what it refuses before sending (invalid, not committed), that its mapped failures
follow D1, and, for `Reconcile`, that it reads first and a failed read reports operation `read-authoritative`. This is
review only; D1's spec requirement is the enforced half.

Generated checks: not used. The mapping's inputs are a finite set of codes and transport outcomes, and the table
covers each one (`docs/testing.md`).

## Issues

| Issue | Disposition |
| --- | --- |
| #138 | Retired (`Closes #138`), tasks 3.1–3.6. Lifecycle's half of item 4 is recorded on #257 for 9b (task 4.2) |
| #257, #154 | Stay, for 9b; 9b calls D1 |
| #258 | Stays, for `pkg/errs`; D1 always sets `Class` |
| #20 | Stays: the open question is graph-ingest's server half |
| #132 | Stays, for PR 8 (`ExactEntityReader`) |

## Not in this change

Lifecycle's adoption of D1, its `ErrEmitFailed` path, and the `default:` of `lifecycleMutationOperation` (9b).
Defect D-2 (#268). Defect D-1 (#267) is in (owner question 3).

## Merge order

No open PR shares a code file. #266 also edits `docs/admission-ledger.yaml`, in other rows, and its design states the
same rule: whichever PR lands second merges `origin/main` first. 9b follows this PR.

## Owner questions

Answered 2026-10-15 (#138 comment 6201479528): 1 keep the names; 2 yes, `Clone` here; 3 yes, fix #267 here.

1. **Keep the old names for the contract types.** Recommended: keep them. On 2026-10-10 you released the removal of
   `projection.Contract`, `projection.PredicateGroup` and the other short names, because nothing seemed to use them.
   That turns out to be wrong for these names. semconnect, semsource, semteams and semboids all write them in their
   production code. SemStreams' decision record 103 promised they would keep compiling, and a test here checks it.
   The five unused interfaces are still removed.
2. **Give the contract type a `Clone` method and use it in both places.** Recommended: yes, in this PR. Today the same
   ten-line copy function is written twice: once here and once in the payload registry. Adding the method to the small
   `pkg/projection/contract` package means that package no longer matches SemStreams line for line. Its ledger entry
   then changes from "copied as is" to "adapted". The payload registry belongs to the later PR 3, but switching it is
   a two-line change. The alternative is to keep both copies.
3. **Fix the read defect in this PR? (yes or no)** Recommended: yes. When `Reconcile`'s own first read fails (a
   timeout, a reconnect, the circuit breaker open), the client says the write may have happened, although nothing was
   sent, so callers never retry it. It is filed as #267. The fix sits in the code this PR rewrites, so fixing it here
   costs one test and one spec scenario.

For information, not a question: a second defect of the same kind affects writes refused before sending, while
reconnecting or with the breaker open (#268). Its home is the internal mutation client, because that is where the
never-sent request is labelled commit-unknown (`internal/graphmutation/client.go:162`). It goes in its own small PR,
landing before or with 9b.
