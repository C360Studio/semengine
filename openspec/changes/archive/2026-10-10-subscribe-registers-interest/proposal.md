# subscribe-registers-interest

Status: revision 5, accepted (Codex's approval, PR #156 comment 6098757365). Revision 3 was accepted with the
design (PR #156 comment 6085722854); revision 5 narrows two promises after Codex's implementation review
(comment 6097783327). It rests on `inventory.md` (revision 2) and `design.md` (revision 5). Issue #144
(`class:flake`); owner ruling 2026-10-09 (fix it once, in natsclient's subscribe, as its own pull request on
`main`); claim PR #156.

## Why

`Client.Subscribe` and `Client.SubscribeForRequests` return while the subscription can still be in nats.go's write
buffer (`natsclient/client.go:1267`), before the server knows it. A request sent from another connection in that
moment gets "no responders", and a publish is dropped without an error. #144 is this: PR #93's
`TestGraphIngestServesExactlyTheDeclaredVerbs` asks from the fixture's connection right after graph-ingest's `Start`
and failed 149 of 300 runs at `-cpu=1` locally, and twice in CI. Measured without natsclient, 831 of 2,000 requests
from a second connection sent right after a subscribe found no responder at `-cpu=1`; after one round trip to the
server, none did (`inventory.md`, P1). The SemStreams pin has the same gap. The owner ruled the fix belongs in
natsclient's subscribe, once, for every caller.

## What Changes

- **One round trip before the return.** Every core subscribe natsclient makes sends a PING on the subscription's
  connection and waits for the server's PONG before it returns the subscription. The server handles one connection's
  lines in order, so the PONG proves it has read the SUB (`design.md`, D1). It does not prove the server accepted it:
  a SUB refused for permissions or for the limit on subscriptions still gets a PONG; the call then returns the
  subscription and nil, and a client that connected with `Connect` logs the refusal (L9).
- **A bounded wait for the answer.** The wait for the PONG ends when the server answers, when the call's context ends,
  or after `DefaultRequestTimeout` (5 s), whichever comes first (D2). The context does not end a wait for the
  connection itself: while the server is not reading and the send buffer is full, the call is held, as every call that
  writes on the connection is, until each held write ahead of it, its own or another goroutine's, completes or
  nats.go's one-minute write timeout ends it (L8, #168).
- **An ended context is refused** before anything is subscribed (D3).
- **Fail closed.** When the round trip does not complete, or Close has begun, the call ends the subscription (it
  unsubscribes it and queues its UNSUB, the protocol line that removes it) and returns no subscription and an error.
  Unlike a consumer setup refused during Close, it does not wait for a handler of it still running, which Close joins:
  it holds no claim, and Close already joins every subscription of a failed setup (D4). The error is Close's refusal
  when Close has begun; otherwise it is transient and matches the context's error, or `ErrNotConnected` when the
  connection was lost (D4, D5).
- **Tests and the #144 reproduction.** Eight unit tests, one per observable, written first; ten wrong changes run
  with `task mutate:check`; and the #144 command run against PR #93's head before and with this change (D7).
- **The ledger.** natsclient's row gains adapt item `(13) natsclient-subscribe-registers-interest`.

Not in this change: when a component may rely on another component's responder (ADR-083, ADR-088), anything after a
reconnect or across a cluster, a requester-side retry, a new exported method, the fifteen test sites that already
flush after a natsclient subscribe (D8), a bound on waits for nats.go's connection lock (L8, #168), and reporting a
refused SUB as an error (L9).

## Capabilities

### Modified Capabilities

- `transport-client`: one added requirement, "Subscribe returns after the server has read the SUB", with eight
  scenarios.

## Impact

- `natsclient/client.go`, `natsclient/request.go` (doc comments), a new natsclient test file,
  `docs/admission-ledger.yaml` (natsclient's row); at the archive, the `transport-client` spec, `natsclient/README.md`
  and `docs/repository-map.md`.
- No exported name, option, environment variable or subject is added. `Subscribe` and `SubscribeForRequests` take up
  to one round trip longer (21 µs median on loopback, P5) and can now return an error when the server does not
  answer.
- A code pull request: Codex's review of record is needed before merge.
- Closes #144, the open `class:flake` issue, so it merges before PR #93, which then drops its planned
  `natsclient.Client.Flush` and its round trip in graph-ingest's `Start` (`design.md`, "Overlaps and order").
