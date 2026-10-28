# Tasks: setup-04a-04-graph-roots-objectstore-component

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171).

## 1. Design and rulings

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review; owner
  acceptance. Gate: `task spec:check` and `task docs:check`.
- [x] 1.2 Owner rulings on design questions 1-3, recorded on PR #437 and folded into the design.

## 2. Tests first

- [x] 2.1 Unit tests (no Docker): `TestComponentConfigRefusesUnknownKey` (`data_cache`, a misspelt key; error names it),
  `TestComponentRefusesOutputOtherThanStored`, `TestDeclarePortsMatchesComponentPorts`, `TestRegisterRegistersObjectstore`,
  `TestClassifyWriteDisposition`, `TestStoredMessageRoundTripsThroughDecoder`, `TestStoredMessageValidate`,
  `TestComponentRefusesInputWithoutStoredOutput`, `TestFailedCleanupRetainsInputsForLaterStop` (integration since D11,
  `notes/4.3.md`; design D8: `Start`
  returns the start and cleanup errors, `Observe` still lists the unreleased inputs, a later `Stop` with a fresh finite
  context returns nil and leaves nothing). Each shown failing first; pin tests dropped or rewritten named in `notes/2.1.md`.
- [x] 2.2 Integration tests (`natsfixture`, shared server via the reset): `TestObjectStoreLifecycleSuite` (core and
  JetStream inputs), `TestStartContextEndKeepsBothLanesWriting` (fails against the pin's shape),
  `TestFailedStartReleasesBoundInputs`, `TestProvidedStoresUnderObjectstore`, `TestSecondComponentRefusedAtStart`, `TestLostJetStreamInputReportsUnhealthy`
  (its durable consumer deleted while running: Health unhealthy naming the input and the restart; #216's ruling),
  `TestMemoryInputLostAtBrokerRestartStaysUnhealthy` (own server, restarted keeping its URL as graph-ingest's T1
  does: memory stream lost and created again, loop still running; unhealthy naming the consumer, before and after a
  publish),
  `TestWriteLaneStoresBodyAndEmitsReference` (reference opens to the body), `TestWriteLaneRefusals` (table: not
  decodable, not `ContentStorable`, non-body content, invalid entity ID × core and JetStream; Term or drop, Error log,
  `ErrorCount` +1, nothing stored), `TestStoredEmitFailureNaksThenRedeliveryStores`. Uncovered: clearing the stuck
  state after a successful lookup (design D2). Recorded in `notes/2.2.md`.

## 3. The Component

- [x] 3.1 `stored_message.go` and `RegisterPayloads` without the map builder; doc names the composition root (#33).
  Gate: `go test -race -count=1 ./storage/objectstore`.
- [x] 3.2 Config, ports, `Register`, `DeclarePorts` (design D6, D7). Gate: as 3.1 and the 2.1 tests.
- [x] 3.3 Lifecycle on `component.Lifecycle`, input binding, the run context and the lost-input tick
  (design D2, D5); allowlist entry (D9).
  Gate: as 3.1, `task test:integration -- ./storage/objectstore`, `task lint`, the contract tests.
- [x] 3.4 Write lane, emit and refusals (design D3, D4). Gate: as 3.3 plus `go test -count=5 -cpu 1 -shuffle=on
  ./storage/objectstore`.

## 4. Waiting for one consumer's handlers (design D11; owner ruling on #443)

- [x] 4.1 Tests first in `natsclient`, each failing first (the method does not exist):
  `TestWaitConsumerHandlersWaitsForRunningHandler`, `TestWaitConsumerHandlersAfterConnectionLoss`,
  `TestWaitConsumerHandlersContextEndsFirst`, `TestWaitConsumerHandlersRefusesUnknownHandle`,
  `TestWaitConsumerHandlersAfterClose`, `TestWaitConsumerHandlersRefusesNilContext`; re-run the adopter search of D11
  and record it on #441. Gate: each test red against a stub that returns nil at once, by an assertion failure, not
  only a compile error (`…AfterClose` against a stub that forgets a handle at release; `…ContextEndsFirst` also against
  a version that stops the handle first), recorded in `notes/4.1.md`.
- [x] 4.2 `Client.WaitConsumerHandlers` in `natsclient` (D11 contract; no other natsclient change). Gate:
  `go test -race -count=1 ./natsclient`, `task test:integration -- ./natsclient`, the 4.1 tests pass.
- [x] 4.3 The Component's `cleanup` calls it; `handlerJoin`, its field and its refusal path go;
  `TestHandlerJoinWaitsForRunningHandlersAndRefusesLaterOnes` dropped; `TestStopJoinsHandlerAfterConnectionLoss`
  unchanged and passing. ADR-112 § 4 and the `natsclient` ledger row name the method. Gate: `task verify`.

## 5. Records and docs

- [x] 5.1 Remove `storage.KeyGenerator`, `storage.MetadataExtractor` and their `storage/doc.go` examples (D6). Gate:
  `task build`, `task docs:check`.
- [ ] 5.2 Ledger rows (`storage/objectstore`, `storage`; `proving_tests` names
  `TestFailedCleanupRetainsInputsForLaterStop`), ADR-063 port note (D7), package `doc.go` and `README.md`
  (D6, N2), `docs/repository-map.md`. Gate: `task ledger:check`, `task docs:check`.

## 6. Evidence and review

- [ ] 6.1 Coverage (`task cover:check` at 80%) and failing-first evidence in the notes; `task verify` on a clean tree.
- [ ] 6.2 Cross-agent review stop; `reviewed-by:` in the PR body.
- [ ] 6.3 Comments on #195 and #196 (advanced), read back.

## 7. Close

- [ ] 7.1 Archive as the last content commit, syncing `object-store`. Gate: `task spec:check`, `task docs:check`.
