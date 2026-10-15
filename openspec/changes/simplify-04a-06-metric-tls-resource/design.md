# Design: simplify-04a-06-metric-tls-resource

Status: draft for inventory review, pre-owner design review and owner acceptance. Inventory: `inventory.md` (base
`70513cf`). Issues #207, #221; epic #223 PR 5; one PR for three packages on the owner's word (#159 waived for it).

## Options

Doing nothing leaves a test hook on a production type (#184, #192), four series that always read 0, and an mTLS block
the base loader ignores. For each item the alternative is the smallest change that removes the surface, so the
decisions below are each the removal, and the open choices are Q1-Q3.

## Decisions

### D1 metric: no NATS core collectors

A fresh `NewMetricsRegistry()` gathers no family named `semengine_nats_*`, and `Metrics` has none of the four fields
or methods. Every other core series is unchanged. Proving test: `registry_test.go` asserts that no gathered family
starts with `semengine_nats_`. It fails on the base, because the plain collectors are gathered without a write
(inventory § 1). The compiler confirms no caller remains.

### D2 metric: no operation hook

`Server` behaves exactly as before. The lifecycle adapter in `lifecycle_test.go` reports no `Calls`, as the natsclient
adapter does since #192 (archived `remove-natsclient-test-seams` design, `:28`), so the suite's no-op checks compare
nil with nil. `TestServerRepeatedStopWaitsForAdmittedRequest` drops its `Calls` assertion. It keeps the two
`context.Canceled` returns and `Unresolved == ["admitted request"]`. Those show that the first Stop released the
listener, server and serve goroutine, so a repeated Stop has nothing to shut down. Gate: `TestServerLifecycleSuite`
and the rest of the package pass; `git grep -n -E 'opHook|recordOp' -- '*.go'` is empty.

### D3 metric: StartWithListener does not probe the socket (Q1)

`StartWithListener` still refuses a nil context, an ended context, a nil listener, and a listener with no assigned
TCP endpoint. It no longer checks that the listener is open. A listener the caller has already closed is accepted,
and the server serves nothing on it. The doc comment says the caller passes an open listener and that Start does not
check it. The closed-listener case at `handler_test.go:232-236` is deleted, not inverted: what `Stop` then returns
depends on whether `Serve` has run yet, so a test pinning it would be flaky. Gate: the remaining refusal cases in
`TestServerStartWithListenerRefusesWithoutTakingOwnership` pass; `syscall` is no longer imported.

### D4 tlsutil: the base loaders apply the MTLS block

`LoadServerTLSConfig(cfg)`:

| `cfg.Enabled` | `cfg.MTLS.Enabled` | Result |
| --- | --- | --- |
| false | false | `nil, nil` (as today) |
| false | true | refused: invalid, wrapping `errs.ErrInvalidConfig` (today `nil, nil`; same as today's `WithMTLS`) |
| true | false | exactly today's `LoadServerTLSConfig(cfg)`: `ClientAuth` `NoClientCert`, `ClientCAs` nil |
| true | true | exactly today's `LoadServerTLSConfigWithMTLS(cfg, cfg.MTLS)`: client CAs, `RequireAndVerifyClientCert` unless `ClientCertOptional`, CN allowlist |

`LoadClientTLSConfig(cfg)` returns today's result when `cfg.MTLS.Enabled` is false. When it is true, it returns
today's `LoadClientTLSConfigWithMTLS(cfg, cfg.MTLS)`, which adds the key pair as its one certificate. Error classes
are unchanged: version refusals are invalid; missing or bad files are fatal and name the file. `metric.Server.Start`
calls `LoadServerTLSConfig(s.security.TLS.Server)`, and its behaviour is unchanged.

CA files (Q2): one unexported loop reads PEM files into a pool for both `RootCAs` and `ClientCAs`. A missing or
unparsable file is still fatal and still names the file.

Proving tests, each failing on the base:

- `mtls_default_test.go`: the real-handshake test builds its server config with `LoadServerTLSConfig`, with the MTLS
  block embedded. On the base, a client with no certificate is served.
- `reviewer_tls_probe_test.go`: `LoadServerTLSConfig` with mTLS enabled and TLS disabled must be refused. On the base
  it returns `nil, nil`.
- `tlsutil_test.go`: `LoadClientTLSConfig` with the client MTLS block enabled has one certificate. On the base it has
  none.

Every other call site moves its second argument into `cfg.MTLS`, with the same assertions. That includes the
integration-tagged `mtls_integration_test.go`. `metric/tls_test.go` passes unchanged.

### D5 resource: WaitForStartup refuses a nil context (Q3)

`WaitForStartup(nil)` returns false and calls no check, which mirrors `Run(nil)` (`watcher.go:146-148`). A caller that
then follows the doc and calls `Run(nil)` gets Run's existing error. Proving test:
`TestWatcher_WaitForStartup_NilContextRefused`, a check that returns nil and counts calls. It fails on the base,
where the result is true with one check. The startup-retry timer is not changed (Q2): no caller can see the
difference, and no test could fail for it.

### D6 Retained, optional

- `Server.start` keeps its own mTLS-without-TLS refusal (`handler.go:116-119`). It refuses before tlsutil is reached
  and keeps the error at the `Server`. Removing it would change the error's class wrapping for no caller's benefit.
- `StartWithListener` stays. That only tests reach it, here and at the pin, is filed as an issue (task 6.3).

## Merge order

No open PR overlaps (inventory § 3). If #265 merges first, this PR merges `origin/main` before its ledger task
(different rows).

## Owner questions

- Q1: Accept that a closed listener passed to `StartWithListener` is no longer refused (D3)?
- Q2: Merge the two CA loops into one (recommended), and drop the timer reuse (recommended)?
- Q3: Should `WaitForStartup(nil)` return false (recommended), or should the signature change to return an error?
