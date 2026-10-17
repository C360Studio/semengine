# Design: simplify-04a-14-natsclient-request

This design depends on `INVENTORY PASS` for `inventory.md`; premises cite its sections. It ports nothing from the pin,
so no pin probe applies. Its pin facts are caller counts from the pin snapshot (inventory § 4). It adds no statement
about how nats.go times, orders or fails, and removes `Request` from two lists that carry such statements (D4).

## D1. `Request` and `RequestWithRetry` are removed

Premises: each repeats a classified sibling minus `ClassifyReply` (inventory § 2). Neither has a production reader here,
in the pin's admitted packages or consumer adapters, or in a consumer (§ 4).

Options:

- (a) Remove both. Recommended; owner question 1. Misuse becomes a compile error. The request surface is
  `RequestClassified`, `RequestWithRetryClassified`, `RequestReadyClassified`, and `RequestWithHeaders` +
  `ClassifyReply` for callers that need headers.
- (b) Make both run `ClassifyReply`. Each then has the same behaviour as its classified sibling under a second name.
  Callers that read error bodies (semsource tests) change behaviour without a compile error.
- (c) Unexport them. Nothing in production calls them, so they would be dead private code: (a) with leftovers.
- (d) Also make `RequestWithHeaders` classify. Not recommended: semconnect's `graph_mutations.go:101-111` would report
  a rejected mutation as "commit outcome unknown", because it treats every non-no-responders error from the call as
  transport. The graph-ingest wire tests also need the raw headers, and `RequestClassified` is built on it.
- (e) Do nothing. #222 part b stays open.

Both carry contract lines (transport-client `:197-201`, `:207`; README `:146`), so this is a removal by name, which is
the owner's ruling (#190).

## D2. One retry config: `retry.Config` through `retry.Do`

`RetryConfig`, `DefaultRetryConfig`, `RequestWithRetry` and `requestMsgWithRetry` go.
`RequestWithRetryClassified(ctx, subject, data, timeout, cfg retry.Config)`:

- refuses a nil ctx (invalid-data error, as today); returns `ErrNotConnected` / `ErrCircuitOpen` before any attempt,
  as today; zero timeout is `DefaultRequestTimeout`; one trace for all attempts;
- makes its attempts through `retry.Do`, with that function's rules for the config (inventory § 2). `MaxAttempts`
  counts every attempt, and zero or a negative count makes one attempt. A config `retry.Do` refuses is returned
  before anything is sent;
- treats only "no reply" as a failed attempt. A reply of any kind ends the call, and `ClassifyReply` runs once on it
  after the attempts, so a handler error is never retried (as today);
- counts a failed attempt toward the circuit breaker unless the caller's context has ended (as today). A caller's
  cancellation, before or during an attempt, returns an error matching `context.Canceled` that `errs.IsTransient`
  reports transient;
- after the last failed attempt, returns an error that still matches the transport error (`errors.Is(err,
  nats.ErrNoResponders)`, `IsNoResponders`).

Options:

- (a) `retry.Do`. Recommended; owner question 2. One retry rule in the repo; `kv.go:378` already does this.
- (b) Keep the natsclient loop and read `retry.Config` fields. The same type would then mean two things (jitter
  ignored, zero values taken literally here and defaulted in `retry.Do`).
- (c) Keep `RetryConfig`. #222 part b stays open.
- (d) Remove `RequestWithRetryClassified` too, and let callers compose `retry.Do` + `RequestClassified`. Not
  recommended: it is framework API (spec `:205`, `AGENTS.md:93`, ADR-060, semteams), and callers would have to keep
  handler errors out of the retry themselves.

Changes for callers: the count includes the first attempt (`MaxRetries: 3` → `MaxAttempts: 4`). A negative count is
no longer refused; it makes one attempt, where the pin made none (Codex F31). `DefaultConfig` has jitter and a 5 s cap.

## D3. `RequestWithHeaders` is the raw call, and says so

No code change. Its doc comment states that it returns the reply as received, and that a handler error arrives as a
reply with `X-Status: error`, not as `err`, so callers run `ClassifyReply`. The new requirement "A handler error never
arrives as reply data" is its contract line. `errors.go:38-50` and `doc.go:425-429` are rewritten to name only it.

## D4. Text follows the code

- Spec: the three modified requirements and the one added requirement in the delta.
- ADR-112: `Request` is struck from the lists at `:110` and `:185` (`RequestClassified` stays at `:185`). An
  amendment line under Status records the change. No call is added to either list.
- `README.md:10`: `Request` → `RequestClassified` (ADR-112 `:185` already names it under P15). `README.md:146` and
  `doc.go:394`: the example calls `RequestClassified`.
- Comments: `request.go:69-79` (the readiness note names `RequestClassified` / `RequestWithRetryClassified`);
  `errors.go:212`, `:241-249`.
- Contracts: `semengine-developer.md:257` and `semengine-reviewer.md:275-277` say "`RequestWithHeaders` without
  `ClassifyReply`" where they say "raw `Request`".
- Ledger natsclient row: one changed-surface sentence for D1–D2 in `contract`; `:1164`'s README correction and
  `:1177-1181`'s `MaxRetries` / `Request` sentences are amended to the new names.

## Tests

Tests are written first; a compile failure on base counts as failing (`notes/<task>.md`).

- `TestIntegration_RequestWithRetryClassified_ErrorReplyIsNotRetried`: the handler returns an error, `MaxAttempts`
  3. The handler runs once, and the call returns the classified error. Passes on base. It guards D2's "a reply ends
  the call": moving `ClassifyReply` inside the attempt makes it fail (`task mutate:check`).
- `TestRetryRequestNegativeCountMakesOneAttempt` (replaces `TestNegativeMaxRetriesNeverDispatches` and
  `TestReviewerNegativeRequestRetryRefuses`): `MaxAttempts` 0 and -1, and the handler runs once. Fails to compile on
  base.
- `TestRetryRequestRefusedConfigNeverDispatches`: `InitialDelay: -1`. Returns an error, and the handler never runs.
  Fails to compile on base.
- `TestIntegration_RequestWithHeaders_ErrorReplyKeepsHeaders`: the handler returns an invalid-coded error.
  `RequestWithHeaders` returns nil error and a message with `X-Status: error`, and `ClassifyReply` on it returns the
  class and code. Passes on base (contract test for D3).
- Kept, edited to the new API: `TestRetryRequestEndedContextNeverDispatches`, `TestRequestCancelledInFlightRecordsNoFailure`
  (rows for the removed calls dropped), the nil-context tests (rows dropped), `RetriesThenSucceeds`, `RoundTripPreservesClass`,
  `RequestWithRetry_NoResponder` converted to the classified call (asserts `IsNoResponders` through `retry.Do`'s wrap).
  `TestIntegration_Request`, `_Timeout` (cited at `errors.go:307`), `_DefaultTimeout`, `_NotConnected`,
  `TestReconnectAfterConnectContextEnded` (ADR-112 T5), and the two `SubscribeForRequests` tests call
  `RequestClassified`; names unchanged.
- Deleted: `TestIntegration_LegacyRequest_SuccessBodyUnchanged` (its call is gone; `RequestClassified_SuccessPath`
  covers success), and `TestIntegration_RequestWithRetry_Success` where `RetriesThenSucceeds` covers the same case.

Generated checks: examples suffice. The inputs are one config and one reply kind. `retry.Do`'s config rules are
tested in `pkg/retry`.

## Exports (#190)

| Symbol | Action | Kind | Reader |
| --- | --- | --- | --- |
| `Client.Request` | removed (Q1) | framework API by contract line, with no production reader anywhere measured | tests only (inventory § 4) |
| `Client.RequestWithRetry` | removed (Q1) | same | tests only |
| `RetryConfig`, `DefaultRetryConfig` | removed (Q2) | second spelling of `retry.Config` | `errors.go:271`; semteams `addsource/executor.go:183` migrates |
| `Client.RequestWithRetryClassified` | kept; takes `retry.Config` | framework API: transport-client "A retrying request…", `AGENTS.md:93`, ADR-060 | semteams `addsource/executor.go:183` |
| `Client.RequestWithHeaders` | kept, unchanged | framework API (ADR-060 PR-D `:73-76`; added requirement); plumbing | `errors.go:233`; semconnect ×5 |
| `Client.RequestClassified` | kept, unchanged | plumbing | `internal/graphmutation/client.go:162`, `graph/exact_entity.go:73` |
| `Client.RequestReadyClassified`, `ClassifyReply`, `RespondError`, `Header*`, `ErrorClass*`, `IsNoResponders` | kept, unchanged | plumbing | inventory § 4 |

No back door.

## ADRs

- ADR-060 (pin): kept. Its handler-error format and its `RequestWithHeaders` + `ClassifyReply` recipe are unchanged.
  Its surface list loses nothing it names. Porting it to `docs/adr` is optional and not done here.
- ADR-112: two lists edited (D4). No new statement about nats.go.

## Open issues and merge order

- #222: part b done here (tasks 3.1–3.3). The PR body references it without a closing keyword; 6c remains.
- #139: stays (graph-ingest). Its `request.go:396` line pin moves; a comment gives the new line (task 4.3).
- #75, #169: stay (6d).
- No overlapping open PR. 6c and 6d follow this one.

## Owner questions

1. Remove `Request` and `RequestWithRetry` rather than make them check the error header; `RequestWithHeaders` stays
   raw (D1). Recommended: remove.
2. `RetryConfig` goes; `RequestWithRetryClassified` takes `retry.Config` and follows `retry.Do`'s rules, so a count
   below one makes one attempt instead of being refused (D2). Recommended: yes.
