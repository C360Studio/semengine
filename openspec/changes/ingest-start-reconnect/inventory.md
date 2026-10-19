# Inventory: ingest-start-reconnect

base: `b46cab4fccc383325213cdd5e64bb4fb283c46e5` (origin/main `63328c4` plus the claim commit, which adds only files
under `openspec/changes/ingest-start-reconnect/`)

Status: pending review (semengine-architect, 2026-10-19). Every cite is at the base commit.

## Question

graph-ingest's `Start` fails with "already connected" when its injected client has a connection installed but is not
reporting `Connected`. What is the smallest change that makes `Start` call `Connect` only when no connection is
installed, and otherwise wait under the Start context (ADR-113)?

## Scope and non-goals

In scope: the connect block of `start` in `processor/graph-ingest/component.go`, `Start`'s doc comment, and tests.
Out of scope: #309, #311, #312, #313, #314, change 3 (the manager), and every natsclient change.

Repositories read: this tree only. No statement depends on the pin, and the change carries no pin package.

## The trigger, corrected

graph-ingest is one-shot: a `Start` after `Stop` is refused (`lifecycle_integration_test.go:72-74`). So the trigger
that #291 names ("a component restart, Stop then Start") is a `Start` of a fresh component instance on a client that was
already connected and is now reconnecting. graph-ingest never closes its client; the client belongs to the
composition (no `natsClient.Close` anywhere in `processor/graph-ingest/*.go`).

## The connect block

- `processor/graph-ingest/component.go:988`: `if c.natsClient.Status() != natsclient.StatusConnected {`.
- `:989`: `if err := c.natsClient.Connect(ctx); err != nil {`; `:993`:
  `return errs.Wrap(err, "Component", "Start", "NATS connection failed")`.
- `:995`: `if err := c.natsClient.WaitForConnection(ctx); err != nil {`; `:997`, `:999` wrap `ctx.Err()` or `err`.
- Doc comment `:958-959`: "ctx bounds Start only. Connecting, bucket setup, the ENTITY_STATES sweep and consumer
  setup run under it, and a ctx that ends first fails Start."

## Statuses

PR #290 merged 2026-10-17: the circuit breaker is gone and four statuses remain (`natsclient/client.go:26-29`:
`StatusDisconnected`, `StatusConnecting`, `StatusConnected`, `StatusReconnecting`). `Status()` at `:342-343` reads an
atomic. "Installed" means `m.conn != nil`. Only `Close`'s cleanup clears it (`:723-724` region, `m.conn = nil`). A
connection is installed at admission (`:549`, `m.conn = conn`).

| Status | When it is written | Installed? | What `Connect` does |
| --- | --- | --- | --- |
| Disconnected | `NewClient` `:329`; failed Connect `connectFailed` `:582-586`; Close cleanup `:745` | no | dials (`:476-552`) |
| Disconnected | `handleClosed` `:1815-1818`: nats.go closed the installed connection (a finite MaxReconnects ran out) | yes (closed) | refuses `:466-467`, `errs.ErrAlreadyStarted`, "already connected" |
| Connecting | `:481`, a Connect in progress before admission | no | dials; loses admission at `:543-547` and returns `ErrAlreadyStarted` |
| Connected | admission `:552`; `handleReconnect` `:1726`; health monitor `:1916-1917` | yes | refuses `:467` |
| Reconnecting | `handleDisconnect` `:1704-1711`; health monitor on a failed RTT `:1918-1919` | yes | refuses `:467` (the #291 case) |

After `Close` begins, `Connect` returns `nats.ErrConnectionClosed` (`:463-464`), permanently.

`WaitForConnection` (`:378-393`) polls `IsHealthy()` (`Status() == StatusConnected`, `:373-374`) every 10 ms. It
returns nil once Connected, or `fmt.Errorf("connection timeout: %w", ctx.Err())` (`:388`) when ctx ends. It has no
timeout of its own and never returns early on a status. On an installed but closed connection it waits until ctx
ends.

The refusal sentinel is part of natsclient's tested contract: `natsclient/client_lifecycle_test.go:319`
(`require.ErrorIs(t, err, errs.ErrAlreadyStarted, …)`) and `client_close_final_test.go:702`. `errs.WrapInvalid` keeps
it reachable by `errors.Is` (`pkg/errs/errs.go:117`, `Unwrap`).

## Other `Connect` callers after a `Status` check

`git grep -n "\.Connect(" -- '*.go' ':!*_test.go'` finds one production caller, `component.go:989`; the other hits are
doc examples (`pkg/errs/doc.go:263`, `pkg/retry/doc.go:24`). `WaitForConnection` likewise has one production caller,
`component.go:995`. Nobody else shares the bug. `GetConnection()` (`client.go:347`) has no production caller.
natsclient's own calls check `Status() != StatusConnected` and refuse with `ErrNotConnected` (e.g. `client.go:1228`),
a refusal, not a Connect.

## Existing tests that reach the block

- `TestGraphIngestLifecycleSuite` (`lifecycle_integration_test.go:306`): `mustFail` builds a never-connected client
  on `refusingBrokerURL` (`:336`, helper `:548`), so Connect runs and fails on the handshake. This covers the error
  path, which stays as it is.
- `TestGraphIngest_StartBoundedByItsContext` (`:390`): a connected fixture client; covers the Start-context bound
  after the block.
- Every other integration test hands Start a client that `openFixtureClient` already connected
  (`fixture_integration_test.go:49-66`, `WithMaxReconnects(0)`, `WithHealthInterval(0)`), so they skip the block.
  Unit tests build `natsclient.NewClient("")` (e.g. `component_test.go:698`), which never has an installed connection.
- No test calls Start while the client is reconnecting. `git grep -ln StatusReconnecting -- '*_test.go'` finds only
  `natsclient/client_test.go`, which sets the status by hand. No test restarts a component (one-shot).

## Test infrastructure available

- `natsfixture.Restart` (`internal/harness/natsfixture/restart.go:39`) stops and starts the container in one call, so
  the broker cannot be held down while Start is called.
- `refusingBrokerURL` (`lifecycle_integration_test.go:548`): a loopback listener on `127.0.0.1:0` that closes every
  connection it accepts. The closest existing shape for a broker the test can make unreachable.
- `probe.Await` (`internal/harness/probe/await.go:21`) waits for an observed status.
- natsclient's reconnect proof is T5, `TestReconnectAfterConnectContextEnded` (`natsclient/client_connect_test.go:216`,
  an embedded server and `WithReconnectWait(10*time.Millisecond)`). ADR-112 section 5: reconnect pacing is not
  promised.
- `docs/testing.md:572-576` allows a real-clock observation window only when its expiry can never fail a correct
  implementation, and only where the design lists it.

## Problem shape

Create-or-exists: act, then treat the "already exists" answer as success, observed with `errors.Is`. The closest
instance in the same package is `component.go:2287`:
`if writeErr != nil && errors.Is(writeErr, natsclient.ErrKVKeyExists) {`.

## Open issues

- #291: retired by this PR.
- #311 (`SubscribeForRequests` parents handlers on the setup context): unrelated; it concerns `setupQueryHandlers`
  and `setupMutationHandlers`, not the connect block.
- #301 (retire the test-only NATS fault fields on `Component`): a constraint only; the tests add no field.
- #216, #196, #300, #9 match the words "reconnect" or "Start" but not the connect block.
- #309, #312, #314 are out of scope.
- Searches run: "already connected", "connect block", `ErrAlreadyStarted`, `Reconnecting`, `WaitForConnection`,
  natsclient, "graph-ingest Start". No open `class:flake` issue exists.

## Overlapping open PRs

`gh pr list --state open`: #316 only (this change, openspec files only).

## Adopter seam (graph-ingest `Start`, reached through the component manager)

1. What they must know: nothing new.
2. If they do nothing: today, Start fails with "NATS connection failed … already connected", an error naming a
   framework internal. After the change, Start waits under their Start context.
3. Where they find out: the Start context's own error, a typed runtime error.
4. What they should have to know: nothing. The change closes that gap.

## Size

Production: about 2 changed lines plus 2 doc-comment lines in `component.go`. Tests: about 50 lines of relay helper
plus two tests of about 50 lines each. One package; no exported change.

## Defects found, not fixed here

- A (candidate, for the owner's call): natsclient's `Connect` refuses "already connected" on a client whose
  connection nats.go has closed. When a finite `WithMaxReconnects` runs out, `handleClosed` writes
  `StatusDisconnected` (`natsclient/client.go:1815-1818`) but leaves `m.conn` set, so every later `Connect` returns
  `ErrAlreadyStarted` (`:466-467`) and `WaitForConnection` waits until its context ends. The only recovery is a new
  client, which `TestIntegration_ReconnectionIsRedial` (`natsclient/integration_test.go:53`) does. No production code
  sets a finite MaxReconnects (default -1, `client.go:308`), so MEDIUM at most (#185). If it is the intended
  "re-dial with a new client" contract, it is a doc sentence, not a fix.
- B (minor): ADR-112 section 5 (`docs/adr/112-natsclient-contract.md:203`) cites `client.go:355-356` for the
  reconnect defaults; at base those lines are in `MaxPayload`, and the defaults are at `:308-309`. Cite drift.
