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
`restore` forwards again. Its doc comment says why it exists: `natsfixture.Restart` cannot hold the broker down, and
an embedded server brought back would reuse a released port (#89). Registration order: the reset, then the relay,
then `natsfixture.Open`, so cleanup closes the client before the relay and the relay before the reset's connection
check. Cleanup closes the listener and every connection and joins the relay's goroutines. No `// Own server` is
needed: the broker never restarts.

The test client goes through `natsfixture.Open` to the relay URL with natsclient defaults: unlimited reconnects, a
short `ReconnectWait`, `HealthInterval` 0. The ENTITY stream is created before the cut.

- T1 `TestGraphIngest_StartWaitsForAReconnectingClient`. Cut, then `probe.Await` until `Status() ==
  StatusReconnecting` (observed, not predicted). `Start` runs in a goroutine. `Start` must not return within an
  observation window of 1 s, listed here per `docs/testing.md:574-576`. Its job is to let `Start` reach the connect
  block before the restore; no correct `Start` can return while no connection is possible, so the window's expiry
  never fails a correct implementation (T2 is the deterministic fail-first). Restore. `Start` returns nil within a
  failure bound of at least 10 s; then `Stop` returns nil and nothing is held. Fails first: today's `Start` returns
  "NATS connection failed" wrapping `ErrAlreadyStarted` inside the window.
- T2 `TestGraphIngest_StartContextEndsWhileWaitingForReconnect`. Cut and await Reconnecting as in T1. `Start` runs
  under a 1 s context and must return an error matching `context.DeadlineExceeded`; `Observe().Unresolved` is empty
  and `Stop` returns nil (the shape of `TestGraphIngest_StartBoundedByItsContext`, `:390`). Before it ends, the test
  always restores the relay and `probe.Await`s `Status() == StatusConnected` under a failure bound of at least 10 s:
  at the pinned nats.go (v1.54.0, `nats.go:6406-6409`, a code read), `Drain` on a reconnecting connection returns
  `ErrConnectionReconnecting`, which natsclient's `Close` does not treat as benign (`client.go:834-836`, `:855-863`),
  so `natsfixture.Open`'s cleanup would fail a correct implementation if the client were still reconnecting. If the
  failure bound fires, the test restores the relay and joins `Start` so nothing outlives the test. Fails first, deterministically:
  today's `Start` returns `ErrAlreadyStarted` at once.

The developer records the fail-first runs in `notes/2.1.md`.

## D3. Mutants

`GOFLAGS=-tags=integration task mutate:check -- -pkg ./processor/graph-ingest -file component.go -mutant <copy outside
the repo> -test <name> -expect-text <text>`.

- M1: the tolerance removed (today's code). Detected by T2 (and T1).
- M2: `WaitForConnection` under `runCtx`. Detected by T2: `Start` does not end with its context.
- M3: tolerated, but `WaitForConnection` skipped. Detected by T2: the next broker call, `initStorage` through
  `natsclient.EnsureFrameworkBucket` and `CreateKeyValueBucket`, refuses at once on any status but Connected
  (`natsclient/client.go:1520-1522`, `ErrNotConnected`), so `Start` returns `ErrNotConnected` instead of
  `DeadlineExceeded` (expect-text names that error). T1 sees the same error inside its window. The wait is what makes
  D1's behaviour true.

## D4. Library statements

- That the client reads Reconnecting after the relay cuts the connection is not promised. T1 and T2 observe it before
  acting and fail loudly at the await if it never happens.
- That nats.go reconnects once the relay forwards again is observed by T1 itself. The nearest probe is T5
  (`client_connect_test.go:216`), a reconnect after a server-side drop with the server still answering, not after
  repeated refused attempts. Pacing is not promised (ADR-112 section 5), hence the failure bound of at least 10 s.
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
- A health-flipped Reconnecting (a failed RTT on a live connection): only the health monitor's next successful RTT
  writes Connected (`client.go:1916-1917`), so `Start` waits up to `HealthInterval` (default 10 s, `:310`) and a
  shorter Start context fails. An unchosen condition (#185); declared, not handled.

## D6. Spec

No spec states `Start`'s connect behaviour. `git grep -n -i connect openspec/specs/graph-ingest-recovery/spec.md`
returns only `:122`, about stream recreation. The general rule "if ctx ends first, Start returns that
error and holds nothing" is ADR-113's Decision (`docs/adr/113-start-context-bounds-start.md:59-60`), and T2 is an
instance of it. `skip_specs: true` stays.

## Merge order

No overlapping PR. #311 may touch `component.go` later; its region (`:1016-1021`) does not overlap.

## Review record

Round 1 (semengine-reviewer, 2026-10-19, at `c0299fd`): `INVENTORY PASS`; design `CHANGES REQUESTED` (HIGH: T2's cleanup
would fail a correct implementation while the client is still reconnecting; MEDIUMs: M3 framing, the proposal's trigger,
task 3.1's gate, defects A and B unfiled; LOWs). All applied in round 2; defects filed as #317. Round 2 at `a5d7d52`:
`DESIGN REVIEW PASS`.

## Owner questions

Owner answer (2026-10-19): accepted as designed; no `graph-ingest-recovery` requirement.

None required. Optional: should "Start waits for a reconnecting client" become a `graph-ingest-recovery`
requirement? The default is no (D6).
