# Inventory: simplify-04a-14-natsclient-request

base: a163c61153e4f23c751d267d72932cd74d8e3a04 (claim head 6ce2302, no content)

Question: what making natsclient's request calls stop returning a handler-error reply as data, and removing
`natsclient.RetryConfig`, touches (#222 part b). Repositories read (docs/inventory-scope.md): this one; SemStreams at
the pin `8b99efe9` (`gh api repos/C360Studio/semstreams/tarball/8b99efe9…`); semsource `e4febc0`, semconnect
`d0d06e0`, semteams `0f1a20b`, semboids `8c03cc5` at symbol level (`gh api …/tarball/<sha>`, file grep only; semteams
read beyond its scoped row, as simplify-04a-13's inventory did).

Callers were measured with `gopls references` under `GOFLAGS=-tags=integration` (without it, integration-tagged files
are missed: `Request` showed 4 references instead of 11).

## 1. The claimed gap

- `natsclient/request.go:191` —
  `func (c *Client) Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error) {`
  ends `:232-233` — `c.resetCircuit()` / `return reply.Data, nil`: no `X-Status` check. Confirmed.
- `natsclient/request.go:529` — `func (c *Client) RequestWithRetry(` returns `msg.Data` from `requestMsgWithRetry`
  (`:536-540`): no check. Confirmed; its doc comment declares it (`:520-527`, "Footgun").
- `natsclient/request.go:244` — `func (c *Client) RequestWithHeaders(` returns the whole `*nats.Msg` (`:305`), headers
  included; the caller can classify it with `ClassifyReply`.
- `natsclient/request.go:491` — `type RetryConfig struct {` (`MaxRetries`, `InitialBackoff`, `MaxBackoff`,
  `BackoffMultiplier`); `:499` — `func DefaultRetryConfig() RetryConfig {` (3, 100 ms, 2 s, 2.0).
- `pkg/errs.RetryConfig`: absent (`git grep -n RetryConfig pkg/errs` empty); removed by #218, PR #236
  (`docs/admission-ledger.yaml:447-448`).
- Declared footgun text: `natsclient/errors.go:38` —
  `// # Footgun — plain Request() / RequestWithHeaders() still don't classify` (`:38-50`); `natsclient/doc.go:425` —
  `// FOOTGUN: plain Request() / RequestWithHeaders() do NOT inspect the X-Status` (`:425-429`).

## 2. Every spelling of the fact

The fact: "a reply carrying `X-Status: error` is a failure".

- Written: `natsclient/errors.go:162` — `func RespondError(msg *nats.Msg, err error) error {`, called by
  `request.go:450` (handler error) and `:475` (response too large).
- Read: `natsclient/errors.go:193` — `func ClassifyReply(msg *nats.Msg) ([]byte, error) {`, called by
  `RequestClassified` `:237`, `RequestWithRetryClassified` `:277`, `RequestReadyClassified` `:298`. No other reader in
  production (`gopls references`).
- Request paths: `Request` (`request.go:191-234`) and `RequestWithHeaders` (`:244-306`) make the same steps (nil-ctx
  refusal, connected check, circuit check, default timeout, trace, `conn.RequestMsgWithContext`, failure count unless
  the caller's ctx ended, `resetCircuit`); they differ only in request headers and the return type.
  `RequestClassified` = `RequestWithHeaders(…, nil, …)` + `ClassifyReply` (`errors.go:232-238`). `RequestWithRetry` and
  `RequestWithRetryClassified` share `requestMsgWithRetry` (`request.go:552-639`) and differ only in `ClassifyReply`.

The fact: "a retry policy". Two spellings:

- `pkg/retry/retry.go:40` — `type Config struct {` — `MaxAttempts` (total; `<= 0` → 1, `:80`), `InitialDelay` (0 →
  100 ms), `MaxDelay` (0 → 5 s), `Multiplier` (0 → 2.0), `AddJitter`; `Do` (`:61`) refuses nil ctx, negative
  delay/multiplier, `MaxDelay < InitialDelay`; checks ctx before each attempt; stops on `NonRetryable`; wraps the last
  error `retry failed after %d attempts: %w`; ctx errors wrapped with `%w` (transient by `errs.IsTransient`'s
  `context.Canceled`/`DeadlineExceeded` sentinels, `pkg/errs/errs.go:170-171`). `DefaultConfig`: 3, 100 ms, 5 s, 2.0,
  jitter.
- `natsclient.RetryConfig` + the loop at `request.go:552-639`: `MaxRetries` counts attempts after the first (negative
  refused, `:563`); zero `InitialBackoff`/`MaxBackoff`/`BackoffMultiplier` taken literally (no defaults); no jitter;
  returns the last error unwrapped; ctx errors `errs.WrapTransient`.
- `natsclient/kv.go:273-289` already maps `KVOptions` onto `retry.Config` and runs `retry.Do` (`:378`).
- A third loop, budget-bounded rather than counted: `requestMsgReady` (`request.go:107-175`), behind
  `RequestReadyClassified`. Different shape (per-probe timeout, total budget); not in scope.

## 3. Adjacent claims

- Spec `openspec/specs/transport-client/spec.md`: `:193` "An exported call refuses a nil context before it acts"
  (scenario `:201` names `Request`); `:205` "A retrying request refuses what it cannot attempt" (`MaxRetries`, both
  retrying calls, `Request`); `:350` "A write the server is not reading holds the connection" (`:354` lists
  `Request`); `:473` "An unclassified handler error is sent as fatal" (`RequestClassified`; untouched). No other spec
  names these calls (`git grep` over `openspec/specs`).
- ADR-112 `docs/adr/112-natsclient-contract.md:110` and `:185` list `Request` (with P13, P15); `:59` T5
  `TestReconnectAfterConnectContextEnded`, whose body calls `client.Request` (`client_connect_test.go:261`).
- Ledger natsclient row `docs/admission-ledger.yaml:828-1735`, `disposition: adapt`; `:1164` (doc correction "Request
  returns the reply and an error"); `:1177-1181` (negative `MaxRetries` refused; `Request` and `RequestWithHeaders` do
  not count a caller's cancellation).
- Rules: `AGENTS.md:93` and `.agents/rules-index.md:39` (classified handlers use `RequestClassified` /
  `RequestWithRetryClassified`; review only). `.agents/contracts/semengine-developer.md:257` and
  `semengine-reviewer.md:275-277` name raw `Request`.
- Docs: `natsclient/README.md:10` (`Publish`, `Request` and `Subscribe` return `ErrNotConnected` while reconnecting,
  P15), `:146` (`client.Request` in the tracing example); `natsclient/doc.go:394`; `request.go:69-79` comment;
  `errors.go:212`, `:241-249` comments; `errors.go:307` cites `TestIntegration_Request_Timeout`.
- Coverage: `scripts/cover-check.sh:29` — `"natsclient merged"`.
- Open PRs (`gh pr list --state open`, files filtered for natsclient, ledger, transport-client, ADR-112, AGENTS.md,
  rules-index, contracts, pkg/retry, the graph-ingest wire tests): none besides this claim (#288, no files)
  and #286. #286 (8c, `graph`) had 72 files at `eeed555` (2026-10-17), two in `natsclient` (`kvspec.go`, `kvspec_integration_test.go`):
  no shared file, but the same package and its coverage floor (merge order in design.md). Dependabot #242–#248: no
  overlap.
- Open issues (`gh issue list --state open --search` for natsclient, RequestWithRetry, RetryConfig, RequestClassified,
  X-Status, ClassifyReply, RequestWithHeaders, request.go): #222 (this, part b). #139 cites `natsclient/request.go:396`
  (the handler timeout) as a line pin only; the line moves. #105 and #132 name the words, not this code. Name the
  package, not this code: #195, #169, #74, #281, #216, #223, #75, #83, #174, #141, #196, #133, #162, #280, #68, #24, #135,
  #9, #60, #103, #134.

## 4. Who it is for

| Symbol | In-repo production readers | Pin readers outside natsclient (production) | Consumer production readers |
| --- | --- | --- | --- |
| `Client.Request` | none (tests: `client_connect_test.go:261`, `errors_integration_test.go:133`, `request_cancel_breaker_test.go:34`, `request_integration_test.go:45,:66,:89,:134,:222,:276`, `reviewer_nil_context_probe_test.go:24,:33`) | `test/e2e/client/nats.go:210` (e2e harness, no ledger row); declared, never called: `processor/graph-query/component.go:35`, `gateway/graph-gateway/component.go:55` | none; semsource tests 12 sites (`processor/*/stop_drain_integration_test.go:27` ×3, `source-manifest/workbench_capabilities_integration_test.go:61`, `query_integration_test.go:115`, `internal/governance/source_removal_integration_test.go:134,:156,:185,:222`, `supersession_integration_test.go:178`, `version_diff_integration_test.go:191`) |
| `Client.RequestWithRetry` | none (tests: `nil_context_siblings_test.go:48,:132,:167`, `request_cancel_breaker_test.go:46`, `request_integration_test.go:378,:412`, `reviewer_full_nats_probe_test.go:42`) | none | none |
| `RetryConfig`, `DefaultRetryConfig` | `errors.go:271` (the `RequestWithRetryClassified` parameter) | none | semteams `cmd/semteams/tools/addsource/executor.go:55` (interface), `:183` (`DefaultRetryConfig()`) |
| `Client.RequestWithRetryClassified` | none (tests only) | none in code; named by pin ADR-060 and ADR-070:103 | semteams `addsource/executor.go:183` |
| `Client.RequestWithHeaders` | `errors.go:233` | none | semconnect `gateway/cs-api/spatial.go:303`, `systems.go:787,:853,:964`, `graph_mutations.go:101`, each followed by `natsclient.ClassifyReply`; graph-ingest tests read raw headers: `query_wire_contract_integration_test.go:66,:121,:143,:164`, `structural_gate_wire_integration_test.go:50` |
| `Client.RequestClassified` | `internal/graphmutation/client.go:162`, `graph/exact_entity.go:73`, `pkg/projection/mutation_client.go:19` (interface) | 40+ (graph-query, graph-clustering, gated-dag, fusion, gateways) | semsource `graph/lifecycle.go:100`, `processor/supersession/prefixquery.go:61`, `processor/mcp-gateway/component.go:191` |
| `ClassifyReply`, `RespondError`, `Header*`, `IsNoResponders` | `errors.go:237,:277,:298`; `request.go:450,:475`; `internal/graphmutation/client.go:170`, `pkg/projection/mutation_client.go:443,:513` | — | semconnect `ClassifyReply` ×6 |

semboids: no reference to any of these names.

Pin tests in admitted packages not yet ported also call the removed methods (not readers under #190; a pointer for
their port designs): `processor/graph-query/batch_passthrough_integration_test.go:89,:153`,
`component_integration_test.go:118`, `component_test.go:76` (`Request`; its `natsRequester` interface at
`component.go:35` declares it); `processor/agentic-tools/tools_integration_test.go:752,:815` (`RequestWithRetry` with a
retry config); `processor/agentic-loop/loop_integration_test.go:830` (`Request`).

## 5. Problem shape

Removing a second spelling in favour of the existing one: the same shape as #218 (PR #236), which deleted
`errs.RetryConfig` for `retry.Config`, and as `natsclient/kv.go:273-289,:378`, which already runs `retry.Do` from
natsclient. Removing an unsafe call so misuse is a compile error: the pin's guard
`test/natsclient/request_guard_test.go` (gh#337) flagged raw `Request`/`RequestWithRetry` call sites. It was not
ported (`git grep -n request_guard` empty).

## 6. Pin decision records

All 85 pin `docs/adr/*.md` were fetched. A grep for RequestWithRetry|RetryConfig|RequestClassified|X-Status|ClassifyReply
returned these:

- ADR-060 "Unified RPC Error Contract" (with `060-pr-d-breaking-change-summary.md`) governs the request surface. It
  decides that a handler error travels as `X-Status` / `X-Error-Class` / `X-Error-Code` headers plus a
  `{message, detail}` body, offers `RequestClassified` / `RequestWithRetryClassified` / `ClassifyReply` /
  `RespondError`, and gives a recipe: custom headers via `RequestWithHeaders`, then `ClassifyReply` (PR-D summary
  `:73-76`). Not in this repo's `docs/adr`.
- ADR-053, 062, 065, 071 name `RequestClassified` as callers. ADR-070:103 says the gated-dag marker write uses
  `RequestWithRetryClassified`; no pin code does (`processor/gated-dag/reader.go:93,:95` use the Ready/plain
  classified calls).

## Shape sweep (request surface, at base)

- Second spelling: `Request` ≡ `RequestWithHeaders(nil headers)` minus headers in the return (§ 2), and
  `RequestWithRetry` ≡ `RequestWithRetryClassified` minus `ClassifyReply`. `RetryConfig` duplicates `retry.Config`
  with different zero and count rules (§ 2).
- No classification by error text in `request.go`/`errors.go` request paths.
- `requestMsgWithRetry` and `requestMsgReady` are two loops; the second is a different shape (optional; stays).

## Adopter seam

The adopter is a product calling `natsclient` (today semsource, semconnect, semteams at the pin).

1. What they must know: (a) call `RequestClassified` / `RequestWithRetryClassified`; (b) after `RequestWithHeaders`,
   call `ClassifyReply`; (c) `retry.Config.MaxAttempts` counts every attempt.
2. If they do nothing: at base, a `Request` caller decodes an error body as a zero-valued success (silent). With
   `Request`/`RequestWithRetry` removed, the build fails. With them made to classify instead, semsource's tests that
   read error bodies through `Request` silently get errors instead.
3. Where they find out: (a) compile error once removed; (b) doc and the review rule (`AGENTS.md:93`); this is the
   remaining gap; (c) field name (`MaxAttempts` vs `MaxRetries`) and doc.
4. What they should know: only (b). `RequestWithHeaders` returns the reply as a message, so the headers are in the
   caller's hand. Making it classify would break semconnect's correct callers (§ 4) and the graph-ingest wire tests.
