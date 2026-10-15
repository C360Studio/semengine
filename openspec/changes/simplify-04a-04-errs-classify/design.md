# Design: simplify-04a-04-errs-classify

Status: draft, pending inventory review, pre-owner design review and owner acceptance. Inventory: `inventory.md`
(base `db48e5b`).

## Context

Issue #167; epic #223 PR 2b (owner split, #223 comment 6191234934). An **unclassified error** is a non-nil error whose
chain holds no `*errs.ClassifiedError` and none of the sentinels `errs.IsTransient`, `IsFatal` or `IsInvalid` list.
Three production callers read `errs.Classify` (inventory § 2): `natsclient`'s reply header, and the two commit-unknown
builders in `pkg/lifecycle` and `pkg/projection`.

## Options

1. **Fatal default (recommended).** No new surface. ADR-060 (pin, `:394`) fixes the class set at
   {transient, invalid, fatal} and the pin already spells commit-unknown as fatal (agentic-loop
   `component.go:1330-1336`, agentic-tools `write_todos.go:263`). Cost: "known unrecoverable" and "never classified"
   share one class. For commit-unknown, `MutationError.Kind` and `Commit` still tell them apart.
2. **A new `ErrorUnknown` class.** It keeps "never classified" distinct, but it adds an exported constant, a fourth wire
   value against ADR-060, and a case in every class switch. Go does not check switch coverage, so the three switches
   whose default is transient (inventory § 2) would retry it silently: fail-open again.
3. **Do nothing to the default; override commit-unknown only.** This fixes half the issue and leaves the fail-open
   default.

## Decisions

### D1 An unclassified error is fatal

`errs.Classify` returns `ErrorFatal` for an unclassified error, whatever its text says. `IsTransient`, `IsFatal` and
`IsInvalid` keep reporting only what the error is marked as, so all three report false for an unclassified error. The
same split exists today in the other direction (`Classify` says transient while `IsTransient` says false).
`IsFatal` stays strict because graph-ingest's stream lane terminates on `IsFatal` (`keyed_ingest.go:307-312`): a
broader `IsFatal` would drop messages. `classForHeader`'s `default:` and `classifiedAppendFailure`'s starting value
become fatal, so there is one answer to "unrecognised class". `Classify(nil)` is unchanged; nothing calls it with nil.
Proving tests: `TestClassify` in `pkg/errs/errs_test.go`. Rows: a bare `errors.New` → fatal. The text-only row is
replaced by an error whose text says "timeout" and "transient" → fatal, which keeps #146's sensitivity now that "fatal
in the text" and the default agree. A transient `ClassifiedError` behind `%w` → transient. `context.DeadlineExceeded`
→ transient. `natsclient/errors_test.go` `classForHeader` row `unclassified_defaults_fatal`; the round-trip test
(`errors_test.go:227`) shows the requester sees `IsFatal`.

### D2 A commit-unknown error is never retryable

Every `*projection.MutationError` with `Commit == CommitUnknown` (`Kind == MutationCommitUnknown`) that
`pkg/projection` or `pkg/lifecycle` returns has `Class == errs.ErrorFatal`. For it and any error wrapping it,
`errs.IsTransient` is false, `errs.IsFatal` is true and `errs.Classify` is `ErrorFatal`, whatever `Err` is.
`errors.Is(err, cause)` and `errors.As(err, &*graphmutation.TransportError)` still succeed; `Code` and `Detail` are the
reply's. How the error presents that class to `errors.As` is the developer's choice, settled by the tests. The closest
existing shape is `errs.Classified` (`errs.go:282-287`), which sets a class and keeps text and code.
Proving tests: `pkg/lifecycle/graph_emit_contract_test.go`, faults `deadline` (`context.DeadlineExceeded`, the case the
default change alone leaves transient) and `malformed response`, for create, reconcile and delete.
`pkg/projection/commit_ambiguity_test.go` `TestClassifiedReplyIsNotCommittedOnlyWhenRefusedBeforeWrite`: the
commit-unknown rows (transient `internal`, fatal, invalid after a commit) expect fatal and `IsTransient` false, and the
not-committed rows keep the reply's class. `TestTransportOutcomesKeepTheirCommitState`: `nats.ErrTimeout` → fatal.

### D3 What was transient on purpose stays transient, explicitly

- `projection`'s no-responders error (`MutationUnavailable`, not committed) is `ErrorTransient`, as
  `pkg/lifecycle/graph_emit.go:102-107` already sets it. Test: `TestTransportOutcomesKeepTheirCommitState`, row
  `no responders`, asserts the class.
- Every error matching `lifecycle.ErrUpdateRetriesExhausted` is transient by `errs.IsTransient` and `errs.Classify`, its
  text unchanged ("retryable contention", `manager.go:449-454`). Tests: `manager_test.go:1745` (fails on the new
  default without this) and a new test on `Update`'s exhaustion path (`manager.go:656-657`).

### D4 Generated checks

Not used. `Classify` maps four input shapes (nil, a classified chain, a listed sentinel, neither), and each shape has
a test row. The commit-unknown rule is checked at every build site and for every write operation by the existing
tables. Text cannot affect the class (`classify_by_type_test.go`). Examples suffice (`docs/testing.md`, "Decide
whether generated checks are needed").

## Not in this change

- **The other lifecycle path.** `pkg/lifecycle/graph_emit.go:98-99` wraps a classified non-definite reply in
  `ErrEmitFailed`, and it stays transient. This is #138 item a's mapping split; owner question 2.
- **The zero value.** `ErrorClass`'s zero value is `ErrorTransient` (`errs.go:17`). Owner question 3.
- **Graph-ingest's own classification of `internal` as transient** (pin ADR-060 `:443-444`; #20).

## Merge order

No open PR shares a planned file. #235 edits other rows of `docs/admission-ledger.yaml`; whichever lands second merges
`origin/main` first.

## Owner questions

1. Accept option 1 (fatal default)?
2. Should lifecycle's `ErrEmitFailed` path (a possibly-committed write reported transient) ride #138 item a, with a
   note on #138, or be added here?
3. File a follow-up for `ErrorClass`'s zero value being transient (semteams builds a `MutationError` without `Class`)?
4. May ADR-112 § 3's "not promised" bullet on #167 get a dated amendment pointing at the `error-classification` spec?
