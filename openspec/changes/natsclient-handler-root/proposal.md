# Proposal: natsclient-handler-root

## Why

Two `natsclient` defects are open for a claim.

- #311: `SubscribeForRequests` derives every request handler's context from the context passed to the subscribe call
  (`natsclient/request.go:322-329`). The owner ruled (#9 comment 5994720412, item 2) that a context bounding setup must
  not parent handlers, for every function with that shape. `ConsumeStreamWithConfig` already follows the ruling: its
  context is setup-only and handlers run under a client-owned root (`natsclient/stream.go:431-438`, `:455-461`). Under
  ADR-113 a lifecycle owner whose handlers outlive `Start` must today pass its run context, so the Start context no
  longer bounds the setup wait.
- #317: `Connect` refuses with "already connected" (`errs.ErrAlreadyStarted`) on a client whose connection nats.go has
  closed itself after a finite `WithMaxReconnects` ran out, while `Status` reports Disconnected. The doc and the error
  text name the wrong state. Separately, ADR-112 cites `client.go:355-356` for the reconnect defaults, which now sit
  elsewhere.

## What Changes

- `SubscribeForRequests` takes the consume-stream split: its context bounds the subscribe and the PING/PONG wait only;
  handlers run under the client-owned root that `Close` ends. Callers that pass a run context today are updated to
  pass their setup context, as the design lists.
- `Connect`'s doc and refusal text name the closed-connection case truthfully. No new mechanism (#185: no production
  code sets a finite MaxReconnects).
- ADR-112's reconnect-default cite is corrected.

## Capabilities

To be filled by the design: the `transport-client` delta for the request-handler context promise. `skip_specs` is set
until the design names the deltas.

## Non-goals

Issue #83 (re-check `ConsumeContext.Closed()` on each nats.go bump). Any other `natsclient` behaviour change.
