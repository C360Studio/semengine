# Proposal: simplify-04a-14-natsclient-request

## Why

PR 6b of epic #223 (#223 comments 6225664166 and 6232454693): #222 part b. `natsclient.Client.Request` and
`RequestWithRetry` return a handler's error reply as data with a nil error. A caller that decodes it gets a zero-valued
success. `natsclient.RetryConfig` repeats `pkg/retry.Config` with different rules for the count and for zero values.

## What Changes

- `Request` and `RequestWithRetry` are removed. `RequestClassified` and `RequestWithRetryClassified` do the same
  requests and return a handler error as an error. No production code here, at the pin, or in a consumer calls the
  removed methods (inventory § 4).
- `RequestWithHeaders` is unchanged. It returns the reply as received, headers included, and its doc comment says to
  run `ClassifyReply` on it. A new spec requirement states this.
- `RetryConfig` and `DefaultRetryConfig` are removed. `RequestWithRetryClassified` takes a `retry.Config` and makes its
  attempts through `retry.Do`. `MaxAttempts` counts every attempt; zero or a negative count makes one attempt.
  It still retries only when no reply arrived; a reply, error replies included, ends the call.
- The footgun notes in `errors.go` and `doc.go` name only `RequestWithHeaders`.

## Capabilities

- Modified: `transport-client` (nil-context scenario, retrying-request requirement, held-write list; one requirement
  added).

## Impact

- Files: `natsclient/request.go`, `errors.go`, `doc.go`, `README.md` and natsclient tests;
  `openspec/specs/transport-client`; `docs/adr/112-natsclient-contract.md` (two lists); `docs/admission-ledger.yaml`
  (natsclient row); `.agents/contracts/semengine-developer.md`, `semengine-reviewer.md` (one sentence each).
- Breaking at cutover (compiler-found): semsource integration tests calling `Request` (12 sites) and semteams'
  `addsource` (`RetryConfig`, `DefaultRetryConfig`) change. semconnect is unaffected.
- Part b of #222; #222 stays open for 6c. Addresses #223.
