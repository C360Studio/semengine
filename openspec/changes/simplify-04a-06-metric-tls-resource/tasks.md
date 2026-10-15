# Tasks: simplify-04a-06-metric-tls-resource

Each task names its outcome and its gate. A test that can fail on the base is shown failing there first. Record the
failing output in `notes/<task>.md`.

## 1. Design

- [ ] 1.1 Inventory, proposal and design pass inventory review and pre-owner design review, and the owner accepts them
  on #221 with answers to Q1-Q3. Gate: `task spec:check` and `task docs:check`.
- [ ] 1.2 Probe P1 (source, command, go1.26.9, output) is posted on PR #266, and `inventory.md` cites the comment's
  URL. Gate: the URL opens.

## 2. metric

- [ ] 2.1 D1: `registry_test.go` asserts that a fresh registry gathers no `semengine_nats_*` family (shown failing on
  the base). Then remove the four fields, their registration and the four `Record*` methods, and the test lines that
  name them (`registry_test.go:132-135`, `:188-191`, `:215-219`). Gate: `go test -race ./metric/`.
- [ ] 2.2 D2: remove `opHook`, `recordOp` and its five calls. The lifecycle adapter reports no `Calls`, and its doc
  comment no longer describes them. Remove the `Calls` assertion at `lifecycle_test.go:234-235`. Gate:
  `go test -race ./metric/`; `git grep -n -E 'opHook|recordOp' -- '*.go'` is empty.
- [ ] 2.3 D3: remove the `syscall.Conn` probe and the `syscall` import. Delete the closed-listener case
  (`handler_test.go:232-236`). The `StartWithListener` doc comment says the caller passes an open listener and Start
  does not check it. Gate: `go test -race ./metric/`.

## 3. Review stop

- [ ] 3.1 Codex reviews tasks 2.1-2.3 on their commits. Gate: review comment linked from the PR.

## 4. tlsutil and resource

- [ ] 4.1 D4 failing tests: `mtls_default_test.go` through `LoadServerTLSConfig` with the MTLS block embedded;
  `reviewer_tls_probe_test.go` refusing mTLS without TLS through `LoadServerTLSConfig`; `LoadClientTLSConfig` with
  the client MTLS block yields one certificate. Gate: all three fail on the base; output in `notes/4.1.md`.
- [ ] 4.2 D4: `LoadServerTLSConfig` and `LoadClientTLSConfig` apply `cfg.MTLS`. Remove both `WithMTLS` functions.
  `metric/handler.go:163` and every test call site use the base loaders, with their assertions unchanged. One
  unexported loop loads CA files into a pool (Q2). Gate: 4.1 passes; `go test -race ./internal/tlsutil/ ./metric/`;
  `go test -tags integration -race ./internal/tlsutil/`; `git grep -n WithMTLS -- '*.go'` is empty.
- [ ] 4.3 D5: `TestWatcher_WaitForStartup_NilContextRefused` (shown failing on the base). `WaitForStartup(nil)` then
  returns false and calls no check. Gate: `go test -race ./internal/resource/ ./natsclient/`.

## 5. Review stop

- [ ] 5.1 Codex reviews tasks 4.1-4.3 on their commits. Gate: review comment linked from the PR.

## 6. Documentation and ledger

- [ ] 6.1 Update the docs:
  - `metric/README.md`: drop the NATS fields and methods.
  - `metric/doc.go` `:38`, `:59-60`, `:76-80`, `:206`: drop the NATS examples and series names.
  - `internal/tlsutil/doc.go` `:24`, `:52-61`, `:78-87`: the base loaders apply the MTLS block; mTLS without TLS is
    refused.
  - `internal/resource/doc.go`: a nil context returns false.

  Gate: `task docs:check`; `git grep -n -E 'WithMTLS|RecordNATS|semengine_nats' -- metric internal docs` is empty
  outside `docs/admission-ledger.yaml` history prose.
- [ ] 6.2 Update the admission-ledger rows, in short bullets:
  - `pkg/tlsutil`: the contract names `LoadServerTLSConfig` and `LoadClientTLSConfig` applying `MTLS`; the two
    `WithMTLS` loaders are dropped (#221).
  - `metric`: contract item (2) names `tlsutil.LoadServerTLSConfig`; `known_risks` lists the dropped NATS collectors
    and methods and the dropped socket probe (#207, #221).
  - `pkg/resource`: the contract says `WaitForStartup` returns false on a nil context without calling the check.

  Gate: `task ledger:check`.
- [ ] 6.3 File every defect this change found and did not fix (protocol File step), at least: `StartWithListener` is
  reached only by tests here and at the pin (`service/metrics.go:323-324`). Link each from the PR body without a
  closing keyword. Gate: each number is in the PR body's follow-up line.

## 7. Verify, review and close

- [ ] 7.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 7.2 Cross-agent review of record on the implementation head; `reviewed-by:` in the PR body.
- [ ] 7.3 Archive the change (no spec to sync) as the last content commit. Gate: `task spec:check`.
