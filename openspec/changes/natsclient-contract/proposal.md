# natsclient-contract

Status: design revision 3, draft for pre-owner review. Issue #170; claim PR #177; addresses #168. Rests on
`inventory.md` revision 3 and `design.md` revision 3.

## Why

natsclient promises that `Close` returns within the caller's deadline. It does not. nats.go v1.54.0 takes one lock
for every write on a connection, and holds it while it dials a reconnect. When the server stops reading, a write keeps
that lock until nats.go's one-minute write timeout ends it, and `Close` waits for the lock.

- A `Close` with a 50 ms context was held for more than 3 minutes (P14), and is still held at a 5 s cap on `main`
  (P17).
- During a reconnect dial, `Close` took 4.69 s (P23).
- `Connect` ignores its context while it dials: 5 s against a silent server whatever the deadline, and 10 s for two
  URLs (P18).

The inventory found 16 statements in natsclient's spec, comments and README that probes show false, and 30 that no
probe or test measures.

The owner ruled on 2026-10-11 (#170 comment 6109833136): fix natsclient so its calls do not sit for minutes, with
pragmatic code and without rewriting nats.go; correct all of the statements now; one pull request. He then kept
nats.go's write timeout, but asked that it be settable so no test waits a minute (comment 6110309758).

## What Changes

- **`Close` cuts the connection when its context ends.** natsclient dials through `nats.SetCustomDialer` with
  `net.Dialer.Cancel`. `Close` closes the socket and ends any reconnect dial when its context ends.
- **`Connect` honours its context.** An ended context is refused before any dial, and a deadline or a cancellation
  ends the dial or the handshake.
- **natsclient holds no lock of its own across a nats.go call.** `subscribeOwned`, `MaxPayload` and `SetConnection`
  move their `IsConnected` call outside natsclient's lock.
- **Two options, `WithWriteTimeout` and `WithPingInterval`.** They set nats.go's write timeout and PING interval. The
  defaults stay at one minute and 30 s, and the tests set them so that none waits a minute.
- **Two sentinels carry their class.** `ErrNotConnected` wraps `errs.ErrConnectionLost`, and `natsclient.ErrCircuitOpen`
  becomes `errs.ErrCircuitOpen`. No responders is unchanged: its class waits for #93.
- **`isAlreadyExistsError` gives way to nats.go's typed error** (`jetstream.ErrBucketExists`).
- **ADR-112** records what natsclient promises over nats.go after the fix. Each line names its probe or test, and what
  is not measured is listed as not promised.
- **48 statements** are corrected, cited or marked: the 46 the inventory found, and 2 sentinel definitions that this
  change makes false (`cleanup.md`).
- **The rule** (a library claim cites a probe) goes into all four role contracts, and its row into `AGENTS.md`.
- **Specs.** `transport-client` changes three requirements and adds four; `nats-fixture` changes one scenario.

## Capabilities

### Modified Capabilities

- `transport-client`: three modified requirements and four added ones. The Purpose is edited at the archive.
- `nats-fixture`: one modified requirement ("Broker max payload is settable", its scenario).

## Impact

- **Code pull request.** It changes Go files, so Codex reviews it (`AGENTS.md`, "Merge"). The developer runs on Opus,
  because the change touches the shutdown path, locking and exported surface.
- **Exported surface.** Two new options, and two sentinels whose values change. No production code compares either
  sentinel with `==` or by its text.
- **What callers see.**
  - `Close` and `Connect` return within their context.
  - A call other than `Close` still waits behind a write the server is not reading, up to the write timeout for each
    write ahead of it, unless the caller shortens the write timeout and the PING interval.
  - `errors.Is(err, errs.ErrCircuitOpen)` now matches natsclient's circuit refusal, and `ErrNotConnected` is transient
    by `IsTransient`.
- **#93.** It rewrites `pkg/errs`, and changes three natsclient files this pull request also edits. The hunks do not
  overlap except at `kv.go:323`, which is left to #93 (`design.md`, "Order with #93").
- **#118.** The nats.go and nats-server upgrade waits. The in-tree tests T1 to T9 replace the probes it would re-run.
