# Tasks: setup-04a-04-graph-roots-objectstore-component

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171).

## 1. Design and rulings

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review; owner
  acceptance. Gate: `task spec:check` and `task docs:check`.
- [ ] 1.2 Owner rulings on design questions 1-3, recorded on PR #437 and folded into the design.

## 2. Tests first

- [ ] 2.1 Unit tests (no Docker): `TestComponentConfigRefusesUnknownKey` (`data_cache`, a misspelt key; error names it),
  `TestComponentRefusesOutputOtherThanStored`, `TestDeclarePortsMatchesComponentPorts`, `TestRegisterRegistersObjectstore`,
  `TestClassifyWriteDisposition`, `TestStoredMessageRoundTripsThroughDecoder`, `TestStoredMessageValidate`,
  `TestComponentRefusesInputWithoutStoredOutput`. Each shown
  failing first; pin tests dropped or rewritten named in `notes/2.1.md`.
- [ ] 2.2 Integration tests (`natsfixture`, shared server via the reset): `TestObjectStoreLifecycleSuite` (core and
  JetStream inputs), `TestStartContextEndKeepsBothLanesWriting` (fails against the pin's shape),
  `TestFailedStartReleasesBoundInputs`, `TestProvidedStoresUnderObjectstore`, `TestSecondComponentRefusedAtStart`, `TestLostJetStreamInputReportsUnhealthy`
  (its durable consumer deleted while running: Health unhealthy naming the input and the restart; #216's ruling),
  `TestMemoryInputLostAtBrokerRestartStaysUnhealthy` (own server, restarted keeping its URL as graph-ingest's T1
  does: memory stream lost and created again, loop still running; unhealthy naming the consumer, before and after a
  publish),
  `TestWriteLaneStoresBodyAndEmitsReference` (reference opens to the body), `TestWriteLaneRefusals` (table: not
  decodable, not `ContentStorable`, non-body content, invalid entity ID × core and JetStream; Term or drop, Error log,
  `ErrorCount` +1, nothing stored), `TestStoredEmitFailureNaksThenRedeliveryStores`. Recorded in `notes/2.2.md`.

## 3. The Component

- [ ] 3.1 `stored_message.go` and `RegisterPayloads` without the map builder; doc names the composition root (#33).
  Gate: `go test -race -count=1 ./storage/objectstore`.
- [ ] 3.2 Config, ports, `Register`, `DeclarePorts` (design D6, D7). Gate: as 3.1 and the 2.1 tests.
- [ ] 3.3 Lifecycle on `component.Lifecycle`, input binding, the run context and the lost-input tick
  (design D2, D5); allowlist entry (D9).
  Gate: as 3.1, `task test:integration -- ./storage/objectstore`, `task lint`, the contract tests.
- [ ] 3.4 Write lane, emit and refusals (design D3, D4). Gate: as 3.3 plus `go test -count=5 -cpu 1 -shuffle=on
  ./storage/objectstore`.

## 4. Records and docs

- [ ] 4.1 Remove `storage.KeyGenerator`, `storage.MetadataExtractor` and their `storage/doc.go` examples (D6). Gate:
  `task build`, `task docs:check`.
- [ ] 4.2 Ledger rows (`storage/objectstore`, `storage`), ADR-063 port note (D7), package `doc.go` and `README.md`
  (D6, N2), `docs/repository-map.md`. Gate: `task ledger:check`, `task docs:check`.

## 5. Evidence and review

- [ ] 5.1 Coverage (`task cover:check` at 80%) and failing-first evidence in the notes; `task verify` on a clean tree.
- [ ] 5.2 Cross-agent review stop; `reviewed-by:` in the PR body.
- [ ] 5.3 Comments on #195 and #196 (advanced), read back.

## 6. Close

- [ ] 6.1 Archive as the last content commit, syncing `object-store`. Gate: `task spec:check`, `task docs:check`.
