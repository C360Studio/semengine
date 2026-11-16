# Inventory: setup-04c-01-http-embedder

base: 4cd1a64bca047f004455fba6c1a1f8c947b2677b (origin/main c6ef1d84a7d3b7260bf306e10179f605d7b4f06d + empty claim
commit; worktree .worktrees/claude-tier1-http-embedder; claim draft #676). Track: design. Pin: SemStreams
8b99efe9c66a4faa4fa509f9f62cc6bad8392128, read with `gh api .../contents/<path>?ref=8b99efe9` and a scratch clone
checked out at the pin (local only, `/tmp/pin489/ss`). semembed read with `git -C semembed show 7ceb5281:<path>`.
SemSource read at its local checkout 4ca2072.

## Carried from the pin

| Pin file | Lines | Disposition today |
| --- | --- | --- |
| `graph/embedding/http_embedder.go` | 220 | `defer-exclude` (`docs/admission-ledger.yaml:3905`) |
| `graph/embedding/http_embedder_test.go` | 89 | excluded with it |
| `graph/embedding/http_embedder_dimensions_test.go` | 239 | excluded with it; its `TestDedupKey_EmptyWhenIdentityUnresolved` is already here (`graph/embedding/dedup_key_test.go:112`), its worker case is covered by `graph/embedding/review_findings_test.go:318` |

Pin caller: `processor/graph-embedding/component.go:944` — `httpEmbedder, err := embedding.NewHTTPEmbedder(httpCfg)`,
which fills the config from `model.ResolveEndpointWithConfig` (`:925`) and `model.ResolveCapabilityTimeout` (`:930`,
default `30 * time.Second` at `:929`). SemEngine has no such caller: change 4 removed `embedder_type`
(`processor/graph-embedding/MIGRATION.md`, "`embedder_type` is refused").

## 1. The claimed gap

- No HTTP embedder exists: `git grep -n -w -e HTTPEmbedder -e HTTPConfig -e NewHTTPEmbedder -- '*.go'` → 0 hits.
- Contract today: `docs/admission-ledger.yaml:3911-3913` — `not ported; graph/embedding carries no HTTP embedder, no
  HTTP* types and no provider import`; `graph/embedding/doc.go:2` — `nearest ones. It holds no embedder of its own: a
  product supplies one, so it imports no`; `graph/embedding/README.md:114` — ``- No HTTP embedder, and no import of an
  embedding provider; a product supplies its own `Embedder`.``; `docs/repository-map.md:74` (`There is no HTTP embedder`).
- The issue's premise "At tier 0, graph/embedding uses its BM25 embedder" is stale: the ledger's graph/embedding
  contract says `bm25_embedder.go and its test (deleted by setup-04b-04-removal-keywords design D5: no in-process
  embedder, a product supplies one)`.
- The fallback test is absent: `git grep -n -i "fallback" -- processor/graph-query/*_test.go` names no provider-loss
  test; `processor/graph-query/query.go:67` — `// searchGraph answers as globalSearch, counted and logged under its own
  name, with no`.

## 2. Every current spelling of the fact

The fact: "how a product's text becomes a vector through an outside provider".

- The interface: `graph/embedding/embedder.go:9` — `type Embedder interface {`, with `Generate` (`:16`),
  `GenerateQuery` (`:24`), `Dimensions` (`:40`; `:31-36` require 0 until the first response), `Model` (`:45`), `Close` (`:51`).
- How a product supplies one: `processor/graph-embedding/component.go:230` — `func RegisterWithEmbedder(registry
  *component.Registry, embedderType string,`; built once per Start at `:315` — `embedder, err := c.newEmbedder()`.
- Callers of the interface: `graph/embedding/worker.go:536` — `vectors, err := w.embedder.Generate(ctx,
  []string{sourceText})` (the Worker's run context, no deadline); `processor/graph-embedding/query.go:83` —
  `vectors, err := c.embedder.GenerateQuery(ctx, []string{req.Query})` (the request's context).
- Endpoint config: `model/registry.go:72` — `type ResolvedEndpoint struct {` (URL, Model, APIKey);
  `model/registry.go:264` — ``QueryPrefix string `json:"query_prefix,omitempty"` ``; resolution at `:115`
  (`ResolveEndpointWithConfig`). SemEngine has no production reader of model's resolution (#155 drift note).
- HTTP client hygiene: `model/httpclient.go` (`NewHTTPClient`), the pin's transport for this file.
- Test embedders (not production): `processor/graph-query/embedding_integration_test.go:26` (`fixedEmbedder`),
  `processor/graph-embedding/embedder_test.go:59` (`wordEmbedder`), `graph/embedding/worker_fakes_test.go:160`.
- Error classing of embedder failures: `graph/embedding/worker.go:740` — `func classifyEmbedErr(err error) string {`
  (deadline by `errors.Is`, the rest by message text: `"connection refused"`, `"timeout"`, `"dimension"`).

## 3. Adjacent claims

- Specs: `openspec/specs/embedding-index/spec.md:259` — `### Requirement: Tier 0 imports no provider code` (`:261-262`
  forbid `go-openai` and `model`); `openspec/specs/graph-embedding/spec.md:202` — `### Requirement: graph-embedding
  runs only with a product's embedder`; `:187` — `### Requirement: A body store that hangs frees the Worker's lane`;
  `openspec/specs/graph-query/spec.md:528` — `### Requirement: searchGraph answers as global search` (`:530-531`: `It
  SHALL make no semantic request.`); `:538` — `### Requirement: Embedding verbs can be turned off`.
- Guards: `internal/harness/contract/embeddingprovider_test.go:20` — `func TestEmbeddingImportsNoProviderModule(t
  *testing.T) {` (deps of `./graph/embedding` only); `:179` — `func TestTreeImportsNoProviderPlumbing(t *testing.T) {`,
  which forbids `go-openai` in `go list -deps ./...` (`:207`); `:90` — `TestGraphEmbeddingImportsNoProviderModule`
  (processor/graph-embedding imports no `model`). `go.mod` has no `go-openai` (`grep -n go-openai go.mod` → 0).
- Active changes: `openspec/changes/` holds only `archive`. Archived design that fixes the plug-in point:
  `openspec/changes/archive/2026-11-14-setup-04b-03-graph-query-lexical/design.md:78` — `**D7. searchGraph and the
  tier 1 fallback (#563)**` and `:115` — `1. **Ruled (a):** the tier 1 fallback is built with #489's provider and
  degrade test in 04C`.
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`): #676 (this claim, 0 files); #597 and #598
  (Dependabot, `go.mod`, `go.sum`). No capability delta overlaps. This change adds no module, so it does not touch go.mod.
- Owner rulings: #11 comment 2026-11-14 (no in-process model; tier 1 through `embedding.Embedder`; a product's model
  is held to the same scorecard); #666 comment 2026-11-14 (the run takes any `Embedder`; a floor names its model);
  #563 comments 2026-11-14 (drop the fallback at tier 0; keep it labelled at tier 1; build it with #489);
  milestone v0.1.0-alpha.6 (#489, #666; #668 out).
- Plan: `docs/setup-plan.md:136-144` (qualify served artifacts, not HTTP success; semembed ignores request model
  selection and can echo a configured name while serving a fallback; a configured but missing provider is
  unavailable evidence, never a passing or skipped gate); `:454-457` (losing the provider degrades to tier 0 with
  search intact).
- Consumer: SemSource composes the pin's `embedder_type: http` from its model registry (`configs/mvp.json:36-55`:
  url `http://semembed:8081/v1`, model `Snowflake/snowflake-arctic-embed-s`, `query_prefix` "Represent this sentence
  for searching relevant passages: ", with its trailing space); `cmd/semsource/run.go:621`
  (`"embedding" (→ semembed)`); its gateway reads the pin's fallback label:
  `processor/mcp-gateway/disclosure.go:31` — `const substrateSemanticFallbackReason =
  "global_search_empty_semantic_fallback"`.

## 4. Who it is for

New exported surface (the pin's names or the chosen package's):

| Symbol | For |
| --- | --- |
| `New`/`NewHTTPEmbedder`, `Config`/`HTTPConfig` (BaseURL, Model, APIKey, QueryPrefix), the embedder type | framework API: SemSource's tier 1 composition (`configs/mvp.json:40-55`) builds it inside its `RegisterWithEmbedder` constructor; contract line is the new embedding-index requirement; product-shaped caller is the PR B integration test that registers graph-embedding with it |
| `graph.GlobalSearchData.Fallback` and its type (PR B) | SemSource's `graph_search` disclosure (`disclosure.go:29-32`); contract line is the modified graph-query requirement |

Pin config fields with no reader here, which this change drops: `Timeout`, `Logger`, `IdleConnTimeout`,
`ResponseHeaderTimeout`, `DisableKeepAlives`. Their only pin writer is the removed `embedder_type: http` branch
(`component.go:931-943`); the last three exist to feed `model.NewHTTPClient`.

## 5. The problem shape

The shape is an outbound JSON call to a remote provider, bounded by its caller's context, plus a per-call bound on a
lane with no deadline. Closest instances: `model/httpclient.go` (transport builder: not adopted, because
embedding-index forbids `model`); `graph/entitytext/body.go:17` — `const readTimeout = 10 * time.Second`, with
`ReadBody` at `:38` (a per-read bound on the same Worker lane; the Worker-bound option adopts this shape).
No reusable primitive is established, so no adoption sweep is owed.

## 6. The pin's decision records

ADRs fetched at the pin (85 files) and searched for `http_embedder`, `HTTPEmbedder`, `NewHTTPEmbedder`, `HTTPConfig`:
only `037-self-hosted-llm-wire-package.md` matches (`:73`, `:412`, `:485`, `:616`). Titles searched for embed,
provider, model, tier, fallback, http: 023, 030, 037, 043, 059, 061, 073, 106. Of these, only ADR-037 governs this
file.

- ADR-037 (Proposed, 2026-05-10), "Self-Hosted LLM Wire-Format Package (`model/wire`)": retire `go-openai`, and move
  `http_embedder.go` to `wire.EmbeddingsClient` over `model.NewHTTPClient`. Not binding: its code, `model/wire`, is
  not ported (ledger `model/wire` `defer-exclude`).
- ADR-043 names semembed as a consumer; ADR-073 names embedding erase fan-out. Neither governs the client.
- SemEngine ADRs that bear on the fallback: ADR-084 (readiness licenses health, not absence; `docs/adr/084-*`) and
  ADR-060 (unified RPC error contract).

## Shape sweep (pin `graph/embedding/http_embedder.go`, scratch pin copy)

Probe: a throwaway test in the scratch copy ran the pin's `NewHTTPEmbedder` against httptest replies (`go1.26.4`,
`go test -count=1 -v -run TestProbe489 ./graph/embedding/`). Its source and output are in
`notes/probes/pin-http-embedder.md`.

| # | Pin line | Finding | Probe result |
| --- | --- | --- | --- |
| S1 | `:184-185` — `for i, data := range resp.Data { embeddings[i] = data.Embedding` | `index` ignored: vectors are placed in reply order | reversed reply → `[[2,2],[1,1]]`, no error |
| S2 | `:190-195` — `if len(data.Embedding) > 0 { if h.dimensions.CompareAndSwap(0, ...` | a vector of another width after the first is accepted | `[[1,1],[1,1,1]]`, dims 2, no error |
| S3 | `:190` | an empty vector is accepted and returned | `[[],[1,1]]`, no error |
| S4 | `:169-172` — `Model: openai.EmbeddingModel(h.model),` | the reply's `model` is never read | reply `served-other` accepted |
| S5 | `:111` — `apiKey = "dummy-key" // Local services don't need real key` | a fake key is sent | `Authorization: Bearer dummy-key` |
| S6 | `:114-115` | request goes to `<BaseURL>/embeddings`; semembed serves `/v1/embeddings` (`main.rs:194`) | path `/embeddings` |
| S7 | `:84-91`, `:121-126` | three connection knobs exist only to feed `model.NewHTTPClient` (shape 4, no reader here) | — |
| S8 | `:47`, `:130-133` | `slog.Default()` fallback for an optional logger used by one Info line (shape 4) | — |
| S9 | `:24-25` — `client *openai.Client` | an SDK for one request shape (`go-openai`, forbidden by `TestTreeImportsNoProviderPlumbing`) | — |

Pin errors as probed: `embedding API call failed: error, status code: 400, status: 400 Bad Request, message: Input
cannot be empty`; refused dial → `... dial tcp 127.0.0.1:1: connect: connection refused` (so `classifyEmbedErr`'s text
match finds `connection_refused`). Shape 5 (classification by error text) exists downstream, in
`graph/embedding/worker.go:746-758`. That code was ported earlier and this change does not carry it.

Pin probe runs (scratch copy, `-run 'HTTPEmbedder|DedupKey_EmptyWhenIdentityUnresolved|WorkerSkipsDedupWhenEmbedderUnresolved'`):
`go test -race -count=1 -cpu 1` → ok 2.570s; `go test -count=5 -cpu 1 -shuffle=on` → ok 4.750s. The three
`Dimensions*` tests each take about 0.98 s; the cause was not measured. Test-text rules: no `time.Sleep`, no skip, no
build tag; `t.Parallel` at `http_embedder_dimensions_test.go:51,105,134,166,189` (unit tests, allowed). No lifecycle
suite was run: the file is not a service.

## semembed at 7ceb5281 (`src/main.rs`)

- `:194` — `.route("/v1/embeddings", post(create_embeddings))`; request `:19-24` (`input` a string or list, `model`
  optional and unread, `encoding_format` float|base64, unread, so floats are always returned); reply `:42-54`
  (`object`, `data[{object, embedding, index}]`, `model`, `usage`); `index` is the input position (`:265-273`).
- `:225-236` — empty input → 400 `{"error":{"message":"Input cannot be empty","type":"invalid_request_error"}}`;
  `:251-259` — embed failure → 500 `internal_error`.
- `:246` — `match embedder.embed(text_refs, None) {`: no query/document distinction, so the query prefix is the
  client's job. `:162-173`: an unknown `SEMEMBED_MODEL` serves arctic-embed-s and still reports the configured name
  (`:278`).
- `:153-154` default model `Snowflake/snowflake-arctic-embed-s`; no published image; the model downloads at first
  start (Dockerfile entrypoint note). So `task verify` cannot run it.

## Open issues naming a carried package

Query: `gh issue list --state open --search "<term>"` for embedding, embedder, semembed, http_embedder,
GenerateQuery, classifyEmbedErr, "tier 1", searchGraph.

| Issue | Names | Disposition |
| --- | --- | --- |
| #489 | the file | retired by PR B (`Closes #489`) |
| #563 | searchGraph (graph-query) | retired by PR B (`Closes #563`) |
| #155 | `graph/embedding/http_embedder.go` (drift note: a would-be `model` reader) | stays: the client reads no `model`; model's port-refactor is its own |
| #564 | `graph/embedding/worker.go:314-317` | stays: Worker stop warning, unrelated to the client |
| #667 | `graph/embedding/worker.go` | stays: long text past the model's limit; tier 1, own design |
| #141, #195, #477 | graph/embedding (adoption) | stay: adoption issues, not this file |
| #658 | processor/graph-embedding | stays: damaged-record rule (#657) |
| #554 | processor/graph-embedding tests | stays |
| #666, #668, #11 | words only (tier 1) | #666 follows PR A; #668 out (owner); #11 addressed |

Names the word, not the package: #9, #10, #24, #109, #111, #134, #484, #485, #487, #574, #630, #641, #665.

## Intent check (the change admits a tier 1 file)

| Capability (AGENTS.md) | Status | Ruling |
| --- | --- | --- |
| ingest, index, query, vocabulary, provenance, fusion | admitted | epic #8; #9 |
| tier ladder: tier 0 word search | admitted | #10; #11 comment 2026-11-14 |
| tier ladder: tier 1 embeddings from outside the process | admitted at tier 1, this change | #11 comment 2026-11-14; #489 |
| in-process embedding model | excluded | #11 comment 2026-11-14 |
| hybrid word+vector merge | deferred | milestone v0.1.0-alpha.6 description (#668 out) |
| tier 2 LLM features | deferred | #484 |
| durable execution: workflows, replay, settlement, retries, rules | admitted, untouched | #8; #24 |

Collision table: not triggered. The client adds no durable, communication or coordination primitive.

## Adopter seam inventory (a product registering graph-embedding at tier 1)

1. **Must know:** the base URL including `/v1` (the client appends `/embeddings`); the model name the provider
   serves; the model's query prefix.
2. **Doing nothing:** no model → refused at `Start`; a base URL without `/v1` → 404 on each embed, each entity failed
   `embedder_error`, GRAPH_STATUS `degraded`; no query prefix → queries embedded raw. That last one is a silent
   quality loss, and only #666's scorecard sees it.
3. **Where they find out:** a boot error (`Start`), the readiness status (`failed_reasons`), nowhere (the prefix).
4. **Should know:** the URL and the model. The gap is the prefix. It is a fact the provider holds and semembed does
   not serve (`main.rs:246`); closing it is a semembed question for the owner.
