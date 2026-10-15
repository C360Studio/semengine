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
2. **A new `ErrorUnknown` class.** It keeps "never classified" distinct, but it adds an exported constant and a fourth
   wire value against ADR-060. Its real cost is in consumer code this repository does not edit: the pin's
   `agentic/agentrun/milestone_settlement.go:188-198` switches on `Classify` with `default:` → Retry (`:197`), so it
   would retry an Unknown error with no change to its code. Go does not check switch coverage, so every such consumer
   switch fails open until someone finds it.
3. **Do nothing to the default; override commit-unknown only.** This fixes half the issue and leaves the fail-open
   default.

## Decisions

### D1 An unclassified error is fatal

`errs.Classify` returns `ErrorFatal` for an unclassified error, whatever its text says. `IsTransient`, `IsFatal` and
`IsInvalid` keep reporting only what the error is marked as, so all three report false for an unclassified error.
`IsFatal` stays strict because graph-ingest's stream lane terminates on `IsFatal` (`keyed_ingest.go:307-312`): a
broader `IsFatal` would drop messages that are retried today.

The consequence differs by where the error is read. In the process that made the error, `errs.IsFatal` is false and
`errs.Classify` says fatal. Once a `SubscribeForRequests` handler returns it, the reply carries `fatal`, so the remote
caller's `errs.IsFatal` is true. A caller that branches on `IsFatal` therefore retries such an error made locally and
quarantines it when it comes from a remote handler. The same split exists today in the other direction (`Classify`
says transient while `IsTransient` says false). A caller that needs one answer in both places branches on
`errs.Classify`.

`classForHeader`'s `default:` and `classifiedAppendFailure`'s starting value become fatal, so there is one answer to
"unrecognised class". `Classify(nil)` is unchanged; nothing calls it with nil.

Proving tests: `TestClassify` in `pkg/errs/errs_test.go`. Rows: a bare `errors.New` → fatal. The text-only row is
replaced by an error whose text says "timeout" and "transient" → fatal, which keeps #146's sensitivity now that "fatal
in the text" and the default agree. A transient `ClassifiedError` behind `%w` → transient. `context.DeadlineExceeded`
→ transient. `natsclient/errors_test.go` `classForHeader` row `unclassified_defaults_fatal`; the round-trip test
(`errors_test.go:227`) shows the requester sees `IsFatal`.

### D2 A commit-unknown error is never retryable

Every `*projection.MutationError` with `Commit == CommitUnknown` (`Kind == MutationCommitUnknown`) that
`pkg/projection` or `pkg/lifecycle` returns has `Class == errs.ErrorFatal`. For it and any error wrapping it:

- `errs.IsTransient` is false, `errs.IsFatal` is true, and `errs.Classify` is `ErrorFatal`, whatever the cause is.
- `errors.As(err, &ce)` with `ce *errs.ClassifiedError` yields class fatal, with the reply's `Code` and `Detail`. This
  is how semsource reads the class (`processor/code-context/http_error.go:70`). The reply's own class is no longer
  the first one `errors.As` finds.
- `errors.Is(err, cause)` and `errors.As` to any other type, `*graphmutation.TransportError` included, still reach the
  cause. `MutationError.Code` and `Detail` are the reply's.

No method is added to `MutationError`. Its `Err` holds the cause wrapped in a fatal `*errs.ClassifiedError`, built
the way `errs.Classified` builds one: it keeps the text and inherits the inner `Code` and `Detail`
(`errs.go:251-260`, `:282-287`).

Proving tests:

- `pkg/lifecycle/graph_emit_contract_test.go`, faults `deadline` (`context.DeadlineExceeded`, the case the default
  change alone leaves transient) and `malformed response`, for create, reconcile and delete.
- `pkg/projection/commit_ambiguity_test.go` `TestClassifiedReplyIsNotCommittedOnlyWhenRefusedBeforeWrite`. The
  commit-unknown rows (transient `internal`, fatal, invalid after a commit) expect fatal, `IsTransient` false, and an
  `errors.As` to `*errs.ClassifiedError` that yields fatal with the reply's code. The not-committed rows keep the
  reply's class. `TestTransportOutcomesKeepTheirCommitState`: `nats.ErrTimeout` → fatal.
- `processor/graph-ingest/commit_ambiguity_integration_test.go` (`-tags integration`). `:135` and `:137-139` (reconcile
  after an unconfirmed update) and `:381-383` (append after an unconfirmed write) expect class fatal with code
  `internal`. `:249` (oversized response after a commit) expects fatal with code `response_too_large`.

### D3 What was transient on purpose stays transient, explicitly

- `projection`'s no-responders error (`MutationUnavailable`, not committed) has `Class` `ErrorTransient`, as
  `pkg/lifecycle/graph_emit.go:102-107` already sets it. Only the field is promised. `errs.IsTransient` on this error
  reads through to the raw `nats.ErrNoResponders`; it is false today and stays false. Raw `nats.ErrNoResponders` is
  unclassified, so `errs.Classify` now says fatal for it. Test: `TestTransportOutcomesKeepTheirCommitState`, row
  `no responders`, asserts the class field.
- Every error matching `lifecycle.ErrUpdateRetriesExhausted` is transient by `errs.IsTransient` and `errs.Classify`, its
  text unchanged ("retryable contention", `manager.go:449-454`). Tests: `manager_test.go:1745` (fails on the new
  default without this) and a new test on `Update`'s exhaustion path (`manager.go:656-657`).

### D4 Generated checks

Not used. `Classify` maps four input shapes (nil, a classified chain, a listed sentinel, neither), and each shape has
a test row. The commit-unknown rule is checked at every build site and for every write operation by the existing
tables. Text cannot affect the class (`classify_by_type_test.go`). Examples suffice (`docs/testing.md`, "Decide
whether generated checks are needed").

## Not in this change

- **A defect of the same class, filed separately.** `pkg/lifecycle/graph_emit.go:98-99` wraps graph-ingest's transient
  `internal` reply in `ErrEmitFailed`, so a lifecycle write that may be stored reads retryable. Fixing it needs the list
  of reply codes that prove nothing was stored. That list is unexported in `pkg/projection` (`isDefiniteFailure`,
  `mutation_client.go:450-462`). Here the fix would either copy the list into lifecycle (a second spelling) or export a
  new projection function (new surface). Recommended: file it as its own defect (task 4.2) and fix it in #138 item a's
  consolidation, where lifecycle calls projection's one mapping. Owner question 2.
- **The zero value.** `ErrorClass`'s zero value is `ErrorTransient` (`errs.go:17`). Owner question 3.
- **Graph-ingest's own classification of `internal` as transient** (pin ADR-060 `:443-444`; #20).

## Merge order

No open PR shares a planned file. #235 edits other rows of `docs/admission-ledger.yaml`; whichever lands second merges
`origin/main` first.

## Owner questions

1. **Should an error nobody classified count as fatal?** Recommended: yes. Read in the process that made it, the
   error is not marked fatal (`errs.IsFatal` is false), but `errs.Classify` calls it fatal. Sent back from a request
   handler, it arrives marked fatal (`errs.IsFatal` is true). Code that checks only `IsFatal` therefore retries such an
   error made locally and stops on it when it comes from another process. `IsFatal` stays strict because graph-ingest's
   stream input drops a message whose error is fatal, and it should keep retrying errors nobody classified. Code that
   needs one answer in both places asks `errs.Classify`.
2. **Lifecycle has the same defect on another path: a write that may be stored is reported as retryable. Fix it here
   or with the projection clean-up?** Recommended: file it now as its own defect, and fix it in the projection
   clean-up (#138 item a), where lifecycle starts using projection's one rule for "did this write happen". A fix here
   would either copy that rule into lifecycle or add a new exported function, and this change adds neither.
3. File a follow-up for `ErrorClass`'s zero value being transient? A `MutationError` or `ClassifiedError` built without
   a class reads retryable; semteams builds one this way.
4. May ADR-112 § 3 get a dated amendment on two bullets? `:152` (the default for unknown errors, now set by the
   `error-classification` spec) and `:144-146` (no responders' class: raw `nats.ErrNoResponders` is now fatal by
   `errs.Classify`, and the no-responders `MutationError` carries the transient class field).
