# Tasks: alpha4-output-websocket

Tests come first. Each code task ends at a clean-tree local `task verify` (#171); integration tests run under
`task test:integration -- ./output/websocket/ ./internal/smoke/`. "Shown to fail" is `task mutate:check` output or
failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no test-owned deadline, no `context.WithValue`,
no `t.Parallel` in an integration file, no fixed port, no `localhost` dial. Review stops: after 2.1, after 3.2, after
4.1, before archive.

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner answers the design's open questions and approves
  (`Approved-by:` in `proposal.md`); ADR-115 is accepted with it. Gate: `task spec:check`, `task docs:check`.

## 2. Config, ports, registration

- [x] 2.1 `output/websocket` config, declarer, factory and `Register` (design D1, D5); `github.com/gorilla/websocket
  v1.5.3` enters `go.mod` with its first importer in 3.1 (notes/2.1.md). Check: `TestConfigRefusals` (one case per
  removed key, each naming it; `jetstream` input naming its replacement; kv-watch of another bucket, with keys, with
  history; two inputs; port 0 and below 1024; bad `ping_interval`; nil NATS client), `TestConfigKeysAreRead` (`path`,
  `ports`, `ping_interval`), `TestDefaultConfig`, declarer/constructor parity.

## 3. Server and inputs

- [ ] 3.1 Server, client writer, subject input, keep-alive, lifecycle (D2, D3; S1, S5, S7, S12, S13, S18); gorilla in
  `go.mod`; the 2.1 factory refusal and `TestFactoryBuildsNothingYet` removed. Check:
  lifecycle suite, `TestStartFailureReleasesPort`, `TestStartContextEndingStopsNothing`, `TestSubjectFanOut` (counter
  and warning for `not_json`), `TestStalledClientDoesNotSlowOthers` (subject: counter and warning for
  `client_queue_full`), `TestQuietClientStaysConnected` (shown to fail without the pong extension),
  `TestSilentClientIsClosed`, `-race`. Each warning emit is shown to fail when removed.
- [ ] 3.2 TLS and metrics (D6). Check: `TestTLSServesWSS`, `TestACMEModeRefusedAtStart`, `TestMetricsCountTraffic`
  (two instances on one registry stay apart by `listener`).
- [ ] 3.3 Applied-state input (D1, D2). Check: `TestAppliedStateSnapshotThenChanges` (synced marker shown to fail
  when removed), `TestAppliedStateConverges` (same-key put/put, put/delete, put/poison/heal in one burst; folded frames
  against the bucket read back), `TestNotReadyRefusesUpgrade`, `TestPoisonedValueIsAnnounced`,
  `TestWatchLossClosesAndRecovers`, `TestStalledClientDoesNotSlowOthers` (applied state: `changes_coalesced_total`
  from `Hooks.OnFanOut` and its warning), `-race`.

## 4. Stack, docs, ledger

- [ ] 4.1 Composed smoke stack adds an applied-state websocket output after #588 has merged and `origin/main` is
  merged in: a client receives each fixed entity's `put` and `synced`, and its folded frames follow the edit and delete;
  the broker-restart scenario lists seven healthy components (`specs/composed-stack/spec.md`). Check:
  `task test:integration -- ./internal/smoke/`.
- [ ] 4.2 `output/websocket/README.md` and `doc.go` written against the code: wire format, inputs, at-most-once,
  every origin accepted and no authentication (#84), TLS, metrics, migration notes for semsource and semboids.
  Check: `task docs:check`.
- [ ] 4.3 Ledger (D10): new `output/websocket` `adapt` row; `internal/tlsutil` stale "change 5" text and
  `consumer_purpose`; `pkg/buffer` `consumer_purpose`; new `pkg/acme` `defer-exclude` row. `docs/repository-map.md`
  entries (package, ADR-115); `query-pattern` skill line. Check: `task verify` (ledger tests), `task status` counts
  `pkg/acme` excluded.
- [ ] 4.4 PR body: `Closes #34`; migration notes (D4); comments on #196, #195, #477 naming what this PR retired or
  added; probe P1 posted on PR #592 with source, command, versions, output. Check: read back each `gh` write.

## 5. Archive

- [ ] 5.1 Specs synced (`output-websocket` added, `composed-stack` modified, its Purpose naming the websocket) and
  the change archived as the last content commit. Check: `task spec:check`, `task docs:check`.
