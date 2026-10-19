# Design: ingest-start-reconnect

## D1. Behaviour and the change

`Start` calls `Connect` only when no connection is installed. Otherwise it waits for the connection under the Start
context, and a Start context that ends first fails `Start` with that error and holds nothing (ADR-113).

Sketch, `component.go:988-1001`:

```go
if c.natsClient.Status() != natsclient.StatusConnected {
    // Connect refuses a client whose connection is installed, as it is while nats.go
    // reconnects it; Start then waits for that connection (#291).
    if err := c.natsClient.Connect(ctx); err != nil && !errors.Is(err, errs.ErrAlreadyStarted) {
        // unchanged
    }
    if err := c.natsClient.WaitForConnection(ctx); err != nil { /* unchanged */ }
}
```

The `Start` doc comment (`:958`) changes "Connecting," to "Connecting, or waiting for a reconnecting client,".

Options considered:

- Do nothing. `Start` fails whenever the shared client is reconnecting. The trigger is real (#291), so rejected.
- `errors.Is(err, errs.ErrAlreadyStarted)` (recommended). It acts and then reads `Connect`'s own answer. One
  condition covers every state in which a connection is installed (Reconnecting, a health-flipped Reconnecting, and
  a Connect that loses admission to a concurrent Connect at `client.go:543-547`), and the sentinel is already pinned
  by natsclient's tests (`client_lifecycle_test.go:319`). It is the package's existing create-or-exists shape
  (`component.go:2287`).
- A status switch on `StatusReconnecting`. It predicts from a value that does not say whether a connection is
  installed, misses Connecting (a concurrent Connect still dialling, nothing installed), and needs two branches.
- `GetConnection() == nil` before `Connect`. It nil-checks a raw `*nats.Conn` no production code reads, and a
  concurrent Connect between the check and the call still fails Start at `:547`.
- An idempotent `Connect` or a new natsclient accessor. Either changes natsclient's tested contract or adds surface,
  outside the one-package scope.

The status fast path stays, so a Connected client still skips the first 10 ms tick of `WaitForConnection`.

## D2. Proving tests

Integration, in `lifecycle_integration_test.go`, on the shared server through the reset. A test-local relay extends
the `refusingBrokerURL` shape (`:548`): it listens on `127.0.0.1:0` and forwards each accepted connection to the
shared broker; `cut` closes every relayed connection and closes each new one on accept, like `refusingBrokerURL`;
`restore` forwards again. Cleanup closes the listener and every connection and joins the relay's goroutines before
the reset's connection check. No `// Own server` is needed: the broker never restarts.

The test client goes through `natsfixture.Open` to the relay URL with natsclient defaults: unlimited reconnects, a
short `ReconnectWait`, `HealthInterval` 0. The ENTITY stream is created before the cut.

- T1 `TestGraphIngest_StartWaitsForAReconnectingClient`. Cut, then `probe.Await` until `Status() ==
  StatusReconnecting` (observed, not predicted). `Start` runs in a goroutine. `Start` must not return within an
  observation window, listed here per `docs/testing.md:574-576`: no correct `Start` can return while no connection is
  possible, so the window's expiry never fails a correct implementation. Restore. `Start` returns nil within a
  failure bound of at least 10 s; then `Stop` returns nil and nothing is held. Fails first: today's `Start` returns
  "NATS connection failed" wrapping `ErrAlreadyStarted` inside the window.
- T2 `TestGraphIngest_StartContextEndsWhileWaitingForReconnect`. Cut and await Reconnecting as in T1. `Start` runs
  under a 1 s context and must return an error matching `context.DeadlineExceeded`; `Observe().Unresolved` is empty
  and `Stop` returns nil (the shape of `TestGraphIngest_StartBoundedByItsContext`, `:390`). If the failure bound
  fires, the test restores the relay and joins `Start` so nothing outlives the test. Fails first, deterministically:
  today's `Start` returns `ErrAlreadyStarted` at once.

The developer records the fail-first runs in `notes/2.1.md`.

## D3. Mutants

`GOFLAGS=-tags=integration task mutate:check -- -pkg ./processor/graph-ingest -file processor/graph-ingest/component.go
-mutant <copy outside the repo> -test <name> -expect-text <text>`.

- M1: the tolerance removed (today's code). Detected by T2 (and T1).
- M2: `WaitForConnection` under `runCtx`. Detected by T2: `Start` does not end with its context.
- M3: tolerated, but `WaitForConnection` skipped. The outcome is not predicted: KV calls wait for their context while
  the client reconnects (ADR-112 section 5), so `Start` may still succeed once the relay is restored. A survivor is
  reported as a survivor, with that reason. The wait is what removes `Start`'s dependence on an ordering nats.go does
  not promise.

## D4. Library statements

- That the client reads Reconnecting after the relay cuts the connection is not promised. T1 and T2 observe it before
  acting and fail loudly at the await if it never happens.
- That nats.go reconnects once the relay forwards again rests on T5 (`client_connect_test.go:216`) at the pinned
  nats.go. Pacing is not promised (ADR-112 section 5), hence the failure bound of at least 10 s.
- That `Close` ends a reconnect dial when its context ends rests on transport-client "Close while a reconnect is
  dialling" (`openspec/specs/transport-client/spec.md:78`).
- Everything else is natsclient's own code, cited in the inventory.

## D5. Declared limits (#185)

- An installed but closed connection (nats.go gave up after a finite MaxReconnects; `client.go:1815-1818`): `Start`
  now waits until its context ends, where today it fails at once. `Start` still fails and holds nothing. No
  production code sets a finite MaxReconnects (default -1, `client.go:308`). Inventory defect A.
- A disconnect after `WaitForConnection` returned still fails `Start` through the later step's own error. No retry is
  added.
- A concurrent `Connect` from another holder of the client is covered by the same condition. No test is added: there
  is no second production `Connect` caller today.

## D6. Spec

No spec states `Start`'s connect behaviour. `git grep -n -i connect openspec/specs/graph-ingest-recovery/spec.md`
returns only `:122`, about stream recreation. The generic "Start returns its context's error" lives in
`lifecycle-suite` (`spec.md:15-25`), and T2 is an instance of it. `skip_specs: true` stays.

## Merge order

No overlapping PR. #311 may touch `component.go` later; its region (`:1016-1021`) does not overlap.

## Owner questions

None required. Optional: should "Start waits for a reconnecting client" become a `graph-ingest-recovery`
requirement? The default is no (D6).
