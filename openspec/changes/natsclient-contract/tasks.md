# Tasks: natsclient-contract

Each code task writes its test first, and records the test failing on the code before the change: the output, and
the commit it ran on. The developer runs on Opus, because the change touches the shutdown path, locking and exported
surface. Test rules are in `docs/testing.md` and in design "Tests that replace the probes". `fix-prototype.diff` is
the reference for every production change.

## 1. Record

- [ ] 1.1 Copy `inventory.md` (revision 3), `design.md`, `cleanup.md` and the two spec deltas into
  `openspec/changes/natsclient-contract/`.
- [ ] 1.2 Post the P17 to P25 READMEs on PR #177. Record the comment ids in ADR-112's Status (orchestrator).
- [ ] 1.3 Correct #168's body to the fix this change makes (orchestrator).

## 2. Connect honours its context (design D2)

- [ ] 2.1 Write T3 `TestConnectRefusesAnEndedContext` and T4 `TestConnectEndsWithItsContext` in
  `natsclient/client_connect_test.go`, and record them failing on `main`'s code.
- [ ] 2.2 Add `natsclient/socket.go`. Its dialer is what `nats.SetCustomDialer` takes:
  - it keeps the last socket it dialled;
  - it dials through `netDial` with `net.Dialer{Timeout, Cancel: done}`;
  - its `cut` closes the socket, closes `done`, and refuses later dials.

  In `connectWith`, refuse an ended context before anything else, pass the dialer, and arm
  `context.AfterFunc(ctx, cut)` around the dial. Then stop and join it. T3 and T4 pass.
- [ ] 2.3 Write T5 `TestReconnectAfterConnectContextEnded`. It passes. Run `task mutate:check` with the `AfterFunc`
  left armed after Connect returns, and record the detection.

## 3. Close cuts the socket and any reconnect dial (design D1, D3)

- [ ] 3.1 Write T1 `TestCloseEndsAHeldWrite` in `natsclient/client_close_held_test.go`, and record it failing on
  `main`'s code (its own 10 s wait).
  - The test owns a proxy on `127.0.0.1:0`. The proxy stops reading after the first bytes of a 16 MiB publish.
  - The proxy registers its cleanup after the client's, so it closes both of its sockets first.
  - The embedded server's `max_payload` is 32 MiB.
- [ ] 3.2 In `Close`, take the dialer only for a connection the client dialled. `cleanup` arms the cut first, and stops
  and joins it before it returns. T1 passes.
- [ ] 3.3 Write T8 `TestCloseEndsAReconnectDial`, and record it failing on 3.2's code with `Cancel` removed from the
  dial.
  - `netDial` blocks every dial after the first until the dialer's `Cancel` closes, and signals when it is entered.
  - The server drops the client.
  - Once the dial is entered, `Close` with 50 ms returns before the test's 3 s wait.
- [ ] 3.4 Write T2 `TestCloseIsNotHeldByAHeldSubscribe`, and record it failing on 3.2's code. It calls `Close` once
  the `Subscribe` has been admitted, as seen by `probe.Await` reading the count of running work under `TryRLock`.
- [ ] 3.5 Move `conn.IsConnected()` out of natsclient's lock in `subscribeOwned`, `MaxPayload` and `SetConnection`.
  When `subscribeOwned`'s check fails after `Close` has begun, return `nats.ErrConnectionClosed`. T2 passes.
  - Run `task mutate:check` with the `subscribeOwned` check moved back under the lock, and record the detection.

## 4. Options, error classes and the classifiers (design D4, D5, D6)

- [ ] 4.1 Write T9 `TestWriteTimeoutAndPingIntervalOptions`, with both options at 300 ms. It shows two things: the
  held publish fails, then the connection carries an echo once the proxy reads again; and each option refuses 0 and
  a negative value. Record it failing to build on `main`'s code.
- [ ] 4.2 Add `WithWriteTimeout` and `WithPingInterval` to `options.go`, and pass `nats.FlusherTimeout` in
  `buildConnectionOptions`. Each doc comment names the other. T9 passes.
- [ ] 4.3 Write T7 `TestClientSentinelsAreTransient` (`ErrNotConnected` and `ErrCircuitOpen` only), and record it
  failing on `main`'s code.
- [ ] 4.4 Make `ErrNotConnected` wrap `errs.ErrConnectionLost`, and set `ErrCircuitOpen = errs.ErrCircuitOpen`. T7
  passes. Then:
  - correct the printed definitions in `README.md:249-250` and `doc.go:247-248` (M105, M106);
  - change `request_integration_test.go:70-74` and `:429-434` from text to `errors.Is(err, nats.ErrNoResponders)`.

  No responders is not wrapped (design D5).
- [ ] 4.5 Write T6 `TestCreateKeyValueOfAnExistingBucketIsErrBucketExists`, then make the change below. This is a
  refactor: T6 pins the nats.go error it relies on, and passes before and after.
  - Replace `isAlreadyExistsError` at `client.go:1887` with `errors.Is(err, jetstream.ErrBucketExists)`.
  - Delete the function and its text-fed test (`client_test.go:560-583`).
- [ ] 4.6 Correct `isCircuitNeutralStreamCapacityError`'s comment (`client.go:516-519`), as row M100 of `cleanup.md`
  says.

## 5. Statements, the record and the rule

- [ ] 5.1 Correct the Go comments in `cleanup.md`, row by row (developer):
  - `doc.go`: M47, M50-M52, M57, M59;
  - `client.go`: M61, M62, M67-M69, M90, M92;
  - `request.go`: M71, M72;
  - `errors.go`: M74, M75;
  - `kv.go`: M76, M81. Do not edit `kv.go:323`, which #93 rewrites;
  - `stream.go`: M85, M86, M104;
  - `delivery_settlement.go`: M87;
  - `storage_inventory.go`: M101-M103.
- [ ] 5.2 Correct the README rows M36, M39, M41, M42 and M46. Add the two options to the option lists at
  `README.md:236-241` and `doc.go:290-294`, with their defaults (technical writer).
- [ ] 5.3 Add `docs/adr/112-natsclient-contract.md` from `adr-112.md`, with T1 to T9 named as they landed (technical
  writer).
- [ ] 5.4 Insert the rule's row into `AGENTS.md`, and its text into the four role contracts, exactly as `rule.md`
  gives them.
- [ ] 5.5 Append an `adapt` item to the natsclient row's contract text in `docs/admission-ledger.yaml`. It covers
  `socket.go`, the two options, the two sentinels and the typed bucket check. Run `task ledger:check`.

## 6. Gates

- [ ] 6.1 Run `task verify` on a clean tree at the final content commit. Record the commit and the result.
- [ ] 6.2 Get Codex's review of record on PR #177. The PR body names `implemented-by: claude (opus)` and
  `reviewed-by: codex`.
- [ ] 6.3 Archive and sync the specs as the last content commit. In the same commit, edit `transport-client`'s Purpose
  so that `Close` stays within the caller's deadline "on a connection it dialled": a `SetConnection` connection is not
  cut.
