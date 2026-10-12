# remove-setconnection

Status: design revision 1, draft for pre-owner review. Issue #184; claim PR #188. Rests on `inventory.md` (INVENTORY
PASS r1, corrections folded) and `design.md` revision 1.

## Why

`natsclient.Client.SetConnection` is a setter whose doc says "(for testing)". Nothing but tests calls it, here, in
SemSource, SemConnect, SemTeams or SemBoids, or at the SemStreams pin. Because any caller could swap the connection
under a running client, SemEngine wrote special cases for that into Close, the subscription watcher, the consumer
goroutine and the JetStream accessor. It also wrote seven spec lines, an ADR-112 bullet and four tests about it. The
owner, 2026-10-12: "I have no idea why we have a setter in Go for SetConnection — this is not Java." Exported
production code that only tests call is a `BLOCKING` finding (#186).

## What Changes

- **Removed:** `natsclient.Client.SetConnection`, and the code that exists only because a connection could be swapped
  (design, "What goes").
- **Removed tests:** the three Close tests that prove only swap behaviour, the `SetConnection` column of
  `TestClientLifecycleOperationTable`, and `TestIntegration_SubscribeForRequestsUsesSubscriptionConnectionForOversizeDiagnostic`
  (owner question Q1).
- **Spec:** two `transport-client` requirements lose their `SetConnection` sentences. "A nil Close means everything
  the client owns has finished" is removed and re-added as "Close returns nil only after everything the client owns
  has finished", without its `SetConnection` sentence and two scenarios. OpenSpec's archive refuses a modified
  requirement that drops scenarios.
- **ADR-112:** a dated note strikes the `SetConnection` sentence. History is not rewritten.
- **Ledger:** the natsclient row loses its changed-behaviour item (9), and `SetConnection` joins its "pin → deleted"
  list.
- **Unchanged:** the README (it never named the method), `doc.go`, and `request.go`.

## Capabilities

### Modified Capabilities

- `transport-client`: two modified requirements ("Close and Drain are bounded by the caller's context", "Close
  refuses new work"), one removed ("A nil Close means everything the client owns has finished") and one added
  ("Close returns nil only after everything the client owns has finished", the same text without the swap). The
  Purpose is unchanged.

## Impact

- **Code pull request.** It changes Go files and the ledger, so Codex reviews it. The developer runs on Opus, because
  the change edits Close's join and ownership paths and removes exported surface.
- **What callers see.** A call to `SetConnection` no longer compiles. No caller exists (design P1). Close, Connect,
  Subscribe and the Consume methods behave as before, as the design's table of observable behaviour and its tests
  show.
- **#169** follows this change and also edits `natsclient/client.go`. Its `SetConnection` case closes with this change
  (#169 comment 6124053574).
- **#93** shares no Go file with this change. It edits the same ledger row in other hunks, and it adds requirements
  to `transport-client` where this change modifies others (design, "Order").
