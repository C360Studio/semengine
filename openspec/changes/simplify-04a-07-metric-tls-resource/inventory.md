# Inventory: simplify-04a-07-metric-tls-resource

base: `70513cf6777cc20a851dd90504ac8d804df40069` (main `e3aa72b` plus the empty claim commit of PR #266)

Question: what `metric`, `internal/tlsutil` and `internal/resource` hold that #207 and #221 name, who reads each
surface the change removes, and which tests and documents describe it. The change carries no package from the
SemStreams pin; it edits three packages already ported (admission-ledger rows `metric`, `pkg/tlsutil`,
`pkg/resource`, all `adapt`).

## 1. The claimed gaps

- `metric/handler.go:30-33` — `// opHook, when set before Start, …` / `opHook   func(op string)`; called through
  `recordOp` (`:338-342`) at `:175` (listen), `:190` (serve), `:247` (shutdown), `:270` (close server), `:276`
  (close listener). Set only at `metric/lifecycle_test.go:37`.
- `metric/handler.go:88-96` — `if socket, ok := supplied.(syscall.Conn); ok {` … `"listener is closed"`: refuses a
  closed listener only when it is a bare `syscall.Conn` (a `*net.TCPListener`); a wrapper such as the tests'
  `observedTCPListener` (`metric/handler_test.go:40-44`) does not implement `syscall.Conn` and skips the probe.
  Only `metric/handler_test.go:232-236` exercises it.
- `metric/core.go:25-29` — the fields `NATSConnected`, `NATSRTT`, `NATSReconnects`, `NATSCircuitBreaker`; built at
  `:117-152`; registered at `metric/registry.go:170-173`; written only by `RecordNATSStatus`, `RecordNATSRTT`,
  `RecordNATSReconnect`, `RecordCircuitBreakerState` (`core.go:195-217`). Being plain (non-vec) collectors, they are
  gathered with no write: `metric/registry_test.go:123-145` asserts all four on a registry whose NATS methods were
  never called.
- `internal/tlsutil/tlsutil.go:14-36` — `LoadServerTLSConfig` never reads `cfg.MTLS`; `:87-109`
  `LoadServerTLSConfigWithMTLS(cfg, mtlsCfg)` applies it, and refuses mTLS without TLS (`:88-91`).
  `:38-82` `LoadClientTLSConfig` never reads `cfg.MTLS`; `:166-187` `LoadClientTLSConfigWithMTLS` loads the key
  pair. #221's "re-reads CA PEM files from disk" is not so: `:166-187` calls `LoadClientTLSConfig` once and reads only
  the key pair. The duplicated code is the PEM loop at `:58-71` (RootCAs) and `:115-127` (ClientCAs).
- `internal/resource/watcher.go:109-136` — `WaitForStartup(ctx) bool` has no nil check; `checkFn(ctx)` runs at `:111`
  and `ctx.Done()` at `:121`; `time.After(w.startupInterval)` at `:126` per attempt. `Run` refuses nil at `:146-148`.

## 2. Every caller of each removed surface

Searches:

- `git grep -n -E 'NATSConnected|NATSRTT|NATSReconnects|NATSCircuitBreaker|RecordNATS|RecordCircuitBreaker|semengine_nats'`
- `git grep -n -E 'opHook|recordOp|syscall\.Conn'`
- `git grep -n 'tlsutil\.' -- '*.go'`
- `git grep -n WaitForStartup`

| Surface | Production readers | Tests | Docs |
| --- | --- | --- | --- |
| four NATS collectors + Record* | none here; none at pin `8b99efe9` (snapshot grep, only `metric/registry_test.go:461-492`); none in semsource `e4febc0`, semconnect `d0d06e0` (Go grep; non-Go hits only raw `.prom` evidence files) | `registry_test.go:132-135`, `:188-191`, `:215-219` | `metric/README.md:188-192`, `:203-206`; `metric/doc.go:38`, `:59-60`, `:76-80`, `:206` |
| `opHook`/`recordOp` | none | `lifecycle_test.go:25-28`, `:36-42`, `:48-54`, `:70`, `:234-235` | none |
| fd probe | none (no production caller of `StartWithListener` here; at the pin only `service/metrics.go:323-324` through the test field `testListener`) | `handler_test.go:232-236` | none |
| `LoadServerTLSConfigWithMTLS` | `metric/handler.go:163` | `tlsutil_test.go:409,433,458,483,506,658`; `mtls_default_test.go:42`; `minversion_test.go:49`; `reviewer_tls_probe_test.go:15`; `mtls_integration_test.go:54,133,206,284,358,437,505` | `internal/tlsutil/doc.go:24`, `:52-61`; ledger `:674-678`, `:703-704` |
| `LoadClientTLSConfigWithMTLS` | none here; pin adapters `input/websocket/websocket_input.go:1103`, `output/httppost/httppost.go:230`, each passing `cfg.MTLS` | `tlsutil_test.go:576,598,621,639,679`; `minversion_test.go:66`; `mtls_integration_test.go:84,155,231,308,389,465,562` | `internal/tlsutil/doc.go:24`, `:78-87`; ledger `:674-678` |
| base `LoadClientTLSConfig` (fold changes it) | none reading `cfg.MTLS` here; pin `pkg/tlsutil/tlsutil.go:286` (ACME branch, deferred to change 5) expects no client certificate from it | unchanged callers in `tlsutil_test.go`, `minversion_test.go` | ledger `:674-678` |
| `WaitForStartup` | `natsclient/client.go:1954`, after its own nil refusal at `:1923-1925` | `internal/resource/watcher_test.go:17-139` | `internal/resource/doc.go:21`, `:54`, `:72-86`, `:138`, `:153`; ledger `:353` |

Every pin caller of a `WithMTLS` loader passes the embedded block (`websocket_input.go:836-839`, `:1103-1106`,
`output/websocket/websocket.go:778-781`, `httppost.go:230-233`). One pin caller of the base client loader does not
mean the MTLS block it carries: `pkg/tlsutil/tlsutil.go:286` — `tlsConfig, err := LoadClientTLSConfig(cfg)` — in
`LoadClientTLSConfigWithACME`'s ACME branch, where `cfg.MTLS` names a manual key pair kept only as a fallback
(`:294`, `:311`) and the ACME certificate replaces `Certificates` later. That loader is admitted but deferred to
change 5 (ledger `pkg/tlsutil` known_risks), so it has no caller here. Once folded, a port that keeps line 286 would
load the fallback pair first, and fail fatal if those files are missing. The server ACME loader builds its own
`tls.Config` and calls `applyMTLSConfig` directly (`:239-245`), so it is unaffected.

## 3. Adjacent claims

- Specs: this search

  ```bash
  git grep -n -E 'WithMTLS|tlsutil|opHook|RecordNATS|semengine_nats|WaitForStartup|resource\.Watcher|StartWithListener' -- openspec/specs
  ```

  It finds only `harness-boundaries/spec.md:333-334` (a pin-path rewrite example, unaffected). `metric-registry` speaks
  of "a core metric" generically (`:25`, `:49`); no delta.
- Ledger: `pkg/resource` `:346-366`, `pkg/tlsutil` `:667-690`, `metric` `:692-722`, all `disposition: adapt`.
- Open PRs (`gh pr list --state open`): #266 (this claim); #265 (PR 9a; files:
  `openspec/changes/simplify-04a-06-projection/{proposal,design,inventory,tasks}.md` and a `projection-mutation` spec
  delta; it will later edit the ledger's `pkg/projection` row);
  Dependabot #242-#248 touch `go.mod`/`go.sum`/`package*.json`/workflow only. No overlap in
  files or capabilities.
- Open issues (`gh issue list --state open --search <metric|tlsutil|resource|mTLS|WaitForStartup>`), read:
  #207, #221 (retired); #68, #84, #196, #135, #136 (name `metric` but their work is elsewhere); #9, #223 (epics);
  word only: #75, #85, #132, #139, #153, #169, #183, #222, #36.

## 4. Who it is for

The change adds no exported symbol. It may add one unexported helper (the CA loop, Q2).

## 5. Problem shape

- Removing a hook that only tests set: `openspec/changes/archive/2026-10-13-remove-natsclient-test-seams/design.md:25`,
  `:28` (the lifecycle adapter stops counting operations; the suite compares nil `Calls` with nil). Adopted.
- An optional variant folded into the base call: the same change folds `connectWith` into `Connect`
  (`design.md:18-20`). Adopted.
- Nil-context refusal at the entry: `internal/resource/watcher.go:146-148`; `background-work` spec "Nil context at
  the entry". Adopted as far as a `bool` return allows (Q3).

## 6. Pin decision records

No package is carried from the pin. For completeness, the pin's 85 ADRs were searched for
`tlsutil|mtls|LoadServerTLSConfig`, `pkg/resource|resource\.Watcher|WaitForStartup`, and
`metric/|metric\.Server|CoreMetrics|RecordNATS|nats_connected`. The hits were ADR-032 and ADR-057, which mention mTLS
as a deployment choice, and ADR-098, which says metrics are the contract for substrate observability. None of them
decides anything these packages' code implements.

## Probe

P1 (go1.26.9 darwin/arm64, run outside the repo): a closed `*net.TCPListener` reports `Addr()` as `*net.TCPAddr`
with port > 0; `http.Server.Serve` on it returns at once with an error where `errors.Is(err, net.ErrClosed)`.
Source, command and output: PR #266 comment
<https://github.com/C360Studio/semengine/pull/266#issuecomment-6200426143>.

## Adopter seam inventory (metric is public; tlsutil and resource are internal)

1. **What must they know?** Nothing new. A product calling a removed `Record*` method or field gets a compile error.
   None was found.
2. **If they do nothing:** a scrape no longer carries `semengine_nats_connected`, `_rtt_seconds`, `_reconnects_total`
   or `_circuit_breaker`. Nothing ever wrote those series, so `semengine_nats_connected` always read 0, which falsely
   says "disconnected". A caller that passes an already-closed bare TCP listener to `StartWithListener` gets nil and a
   server that serves nothing (P1), where it used to get an invalid error.
3. **Where do they find out?** For the symbols, a compile error. For the closed listener, nowhere except a failing
   health check. That is Q1.
4. **What should they have to know?** Nothing, except that `StartWithListener` takes an open listener. The doc
   comment says so. `StartWithListener` stays: `metric/README.md:220` offers it to products, and it is the only way a
   product gets an ephemeral port, because `NewServer` maps port 0 to 9090 (`metric/handler.go:46-48`).
