# Proposal: fix-605-natsclient-poller-readiness

Approved-by: cglusky (2026-11-22) <https://github.com/C360Studio/semengine/issues/605#issuecomment-7085526705> sha256:18e0fef01330

Track: `quick`. Capability modified: `transport-client` (three requirements reworded, none added or removed). One
package, `natsclient`; two exported names removed, plus two exported constants. Parts 1 and 2 of #605; part 3, the
trace context value, is a follow-up PR after its call chain is traced (owner ruling on #605, comment 7079513474).

## Why

natsclient passes nats.go through and adds no mechanism of its own (ADR-112). Two pieces of it do add mechanism.

1. **A health poller on top of nats.go's own.** `startHealthMonitoring` (`natsclient/client.go:1931-1983`) runs every
   10 s by default (`client.go:321`). Each tick calls `conn.RTT()` and, on one failed round trip, sets the status to
   `StatusReconnecting` while nats.go may still be connected. The disconnect and reconnect handlers
   (`client.go:1764`, `client.go:1779`) also set the status, so two writers set it. On a loaded server or a slow link,
   one slow round trip makes a reader of `Status()` report the client unhealthy, and readiness can flap, with no
   disconnect behind it.
2. **A readiness request that retries every error.** `RequestReadyClassified` (`natsclient/errors.go:316`, loop in
   `natsclient/request.go:105-161`) retries any error for its whole budget (30 s by default), although its comment says
   transport failures only: a permanent error loops until the budget ends, and a probe that times out against a slow
   but live responder sends the request again, so the responder runs it twice. Nothing calls it: no production code
   here, and none in semsource, semteams, semconnect or semboids (GitHub code search, 2026-11-22).

## What Changes

- **Remove the poller.** `startHealthMonitoring`, `stopHealthMonitoring`, the `healthInterval`, `healthTicker` and
  `healthDone` fields, the `workHealthMonitor` name and the exported option `WithHealthInterval` go. `Status()` is set
  only by `Connect`, the nats.go connection handlers and `Close`. nats.go's own pings (`WithPingInterval`, unchanged)
  still find a dead connection, and its disconnect handler still starts the connection-loss timer
  (`client.go:1789`), so `WithConnectionLossTimeout` and `WithConnectionLostCallback`, which semsource uses, keep
  working.
- **Drop `RequestReadyClassified`** with `requestMsgReady`, `DefaultReadinessProbeTimeout`, `DefaultReadinessBudget`
  and the two backoff constants, rather than narrow its retry: under the new-surface rule (#190) an exported call
  needs a caller, and it has none. A caller that needs it later brings it back with a test of its retry rule. The
  comment in `natsclient/stream.go:273-278` that points at these constants is reworded.
- **Callers.** Every `WithHealthInterval(0)` in this repository's tests and `internal/harness/scorecard/measure.go:230`
  is deleted; `docs/test-infrastructure.md:167` loses the line. The poller's test
  (`natsclient/health_monitor_integration_test.go`) and the readiness tests
  (`natsclient/readiness_integration_test.go`, the `RequestReadyClassified` rows in
  `natsclient/nil_context_siblings_test.go`) go with the code.
- **Consumers migrate.** semsource (`cmd/semsource/run.go`, `WithHealthInterval(30 * time.Second)`) and semteams
  (`internal/natstest/natstest.go`, `WithHealthInterval(0)`) each delete one line when they move to the release that
  carries this. The PR lists both.

## Proof

The build checks the removals. What stays already has tests: status from the connection handlers, the failed-call
scenario, the connection-loss timer. No new test proves an absence.

## Non-goals

- Part 3 of #605 (trace context value): next PR.
- #605's "also noted" items (the handler-timeout environment variable, unread exported names); nats.go's ping defaults.

## Impact

`natsclient/{client,options,errors,request,stream}.go`, natsclient's tests and README if it names a removed name,
`internal/harness/scorecard/measure.go`, about 30 test files outside natsclient (one line each),
`docs/test-infrastructure.md`, and `transport-client`'s spec. One PR, `Addresses #605` (part 3 still open).
