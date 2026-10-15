# Proposal: simplify-04a-07-metric-tls-resource

## Why

Three floor packages carry surface that misleads or is used only by tests (epic #223, PR 5; the owner waived the
one-package rule #159 for this PR):

- `metric.Server` has a test-only hook, `opHook`/`recordOp` (#207). Owner rulings #184 and #192 remove such hooks
  from production types.
- `metric.NewMetricsRegistry` registers four NATS collectors that nothing writes. In this repo, at the SemStreams pin
  and in semsource and semconnect, the `Record*` methods have no caller. So every scrape reports
  `semengine_nats_connected 0`, which reads as "disconnected" (#221).
- `tlsutil.LoadServerTLSConfig(cfg)` ignores `cfg.MTLS`. A caller has to know to call
  `LoadServerTLSConfigWithMTLS(cfg, cfg.MTLS)`, passing the same block twice. The client loader has the same split
  (#221).
- `resource.Watcher.WaitForStartup` accepts a nil context and passes it to the check, while `Run` refuses one (#221).

## What Changes

- `metric`: remove `opHook` and `recordOp`. Remove the `syscall.Conn` socket probe in `StartWithListener`. Remove the
  fields `NATSConnected`, `NATSRTT`, `NATSReconnects`, `NATSCircuitBreaker` and the methods `RecordNATSStatus`,
  `RecordNATSRTT`, `RecordNATSReconnect`, `RecordCircuitBreakerState`.
- `internal/tlsutil`: `LoadServerTLSConfig` and `LoadClientTLSConfig` apply the `MTLS` block of the config they are
  given. `LoadServerTLSConfigWithMTLS` and `LoadClientTLSConfigWithMTLS` are removed. One loop reads CA files into a
  pool, for both the client's trusted CAs and the server's client CAs.
- `internal/resource`: `WaitForStartup(nil)` returns false without calling the check.
- `metric/README.md`, `metric/doc.go`, `internal/tlsutil/doc.go`, `internal/resource/doc.go` and the three
  admission-ledger rows are updated to match.

Exported symbols removed: the eight `metric` symbols above and the two `tlsutil` functions (the latter internal).
None added.

## Capabilities

None changed. No current spec names these surfaces.

## Impact

- `metric/core.go`, `metric/registry.go`, `metric/handler.go`, `metric/registry_test.go`, `metric/lifecycle_test.go`,
  `metric/handler_test.go`, `metric/README.md`, `metric/doc.go`.
- `internal/tlsutil/tlsutil.go`, `tlsutil_test.go`, `minversion_test.go`, `mtls_default_test.go`,
  `mtls_integration_test.go`, `reviewer_tls_probe_test.go`, `doc.go`.
- `internal/resource/watcher.go`, `watcher_test.go`, `doc.go`.
- `docs/admission-ledger.yaml` (rows `pkg/resource`, `pkg/tlsutil`, `metric`).
- Scrape output: the four `semengine_nats_*` series are gone.
- Closes #207, #221; addresses #223.
