# Proposal: natsclient-subscribe-handler-root

## Why

`Subscribe` builds every handler's context from the context given to the call (`natsclient/client.go:1041-1044` at
`46d3cd5`). The owner ruled (#9 comment 5994720412, item 2) that a context bounding setup must not parent handlers, for
every function of that shape. `ConsumeStreamWithConfig` and `SubscribeForRequests` follow the ruling; `Subscribe` is the
one exception, and the `transport-client` spec names it, pointing at #378.

The cost lands on callers. Under ADR-113 an owner whose `Subscribe` handlers outlive `Start` must pass its run context,
so its Start context does not bound the subscribe. The message logger, the one production caller, does this
(`service/message_logger.go:288`, `:437`). Its `Start` also has no check of its context after the last subscribe
(`:292-305`), so it can return nil after its context ended, which ADR-113 forbids (inventory §3a(a)). Severity MEDIUM
(#185): the trigger is a Start deadline ending during the subscribe round trip; the cost is a bounded late return.

## What Changes

- `Subscribe` takes the same split as `SubscribeForRequests`. Its context bounds the subscribe and the wait for the
  server's answer (PING/PONG) only. Each handler runs under a context the client owns, which `Close` ends. No new
  surface; the signature is unchanged.
- The message logger subscribes at `Start` under its Start context. It checks that context once more at the end of
  `Start`, so a `Start` whose context ended fails. Its retry keeps the run context.
- The message logger declares that a message delivered during the client's `Close` is not stored.
- Docs, ledger text and an ADR-113 cite that describe the old shape are corrected.

## Capabilities

- `transport-client` (modified):
  - "A setup context does not parent its handlers" covers `Subscribe`, and the exception is removed.
  - "Subscribe returns after the server has read the SUB" changes one clause.
- `service-lifecycle` (modified): "The message logger's Start bounds Start and Stop is the only stop".

## Non-goals

- Moving the logger's run-context cancel ahead of the retry join (D4).
- Any other `natsclient` behaviour.
- The pin's other `Subscribe` callers. They are ported later, and either context works for them after this change.

Closes #378.
