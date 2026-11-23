# Inventory: fix-724-retry-worker-provider-failures

base: e925194a6b00d1f2e992bfda5e621396b1f90ba5

Question: what exists today for retrying graph-embedding's Worker failures, classifying provider replies, and pacing a
retry, so the design for #724 and #732 adds no second spelling. Repositories read: this one; consumer repositories
only by `gh api search/code` for reason strings and one semsource report (`docs/inventory-scope.md`). No package is
carried from the pin.

## 1. The claimed gap

- The repair lane re-drives only hop 1's own failures:
  `processor/graph-embedding/failed.go:44-50` — `func repaired(reason string) bool { switch reason { case embedding.Re…`
  `processor/graph-embedding/failed.go:209-216` — `func (m *failedMap) repairTargets() []string { ... if o.hopOne() {`
  `processor/graph-embedding/hop1.go:283` — `const repairInterval = 30 * time.Second`
  `processor/graph-embedding/hop1.go:307-313` — `func (c *Component) repair(ctx context.Context) { for _, key := range…`
  `processor/graph-embedding/README.md:155-156` — `The Worker's failures are not retried: they wait for a new revision…`
- The Worker retries failed records only from its initial snapshot, so once per `Run`:
  `graph/embedding/worker.go:418` — `reprocessFailed := record.Status == StatusFailed && duringSnapshot`
- A 429 is not told apart from other replies:
  `graph/embedding/httpembedder/httpembedder.go:210-219` — `if resp.StatusCode < 200 || resp.StatusCode > 299 { ... re…`
- Searches that came up empty: `git grep -n -i 'retry-after\|429\|TooManyRequests\|rate_limit'` under `graph/embedding`,
  `processor/graph-embedding`: no production hit. `git grep -n 'Retryable'` in `graph/`, `processor/`: no hit.

## 2. Every current spelling of "this failure, and whether retrying helps"

- Stored reason set (closed, unexported): `graph/embedding/storage.go:513-518` — `failReasonContentError = "content_er…`
  `failReasonInternal = "internal"`, `failReasonConnectionRefused = "connection_refused"`, `failReasonTimeout = "timeout"`,
  `failReasonDimensionMismatch = "dimension_mismatch"`, `failReasonEmbedderError = "embedder_error"`.
- Scan normalization: `graph/embedding/storage.go:529-535` — `func normalizeFailureReason(reason string) string { swit…`
- Hop 1's in-memory reasons: `graph/embedding/derived_reasons.go:21,25,29` (`delete_failed`, `pending_write_failed`,
  `entity_read_failed`).
- Classifier, by error text: `graph/embedding/worker.go:786-805` — `func classifyEmbedErr(err error) string { ... stri…`
- Classification sites: `worker.go:431` (`content_error`, body read), `:507` (`internal`, dedup check), `:523`
  (`embedder_error`, no vector returned), `:527` (`classifyEmbedErr`), `:656` (`internal`, save).
- What `embedder_error` holds today through httpembedder: every non-2xx reply (4xx and 5xx alike,
  `httpembedder.go:210-219`), every transport error except text "connection refused" or a timeout
  (`httpembedder.go:204-207`), TLS certificate failures among them, a reply decode failure (`:223`), and every
  untrusted reply (`accept`, `:231-270`) except the two width errors, filed `dimension_mismatch` for the word
  "dimension". Unit witness: `graph/embedding/worker_failure_reason_test.go:28` — `{"unrecognized defaults to embedd…`
- `content_error` holds every body-store read error: `worker.go:711-718`, `fetchTextFromStorage` `:747-765`.
- Interpreters of the reason in processor/graph-embedding: `repaired` (`failed.go:44`), `withheld` (`:194`),
  `workerFailures` (`query.go:186-194`), `counts` (`failed.go:177`).
- The framework's retry classification, already imported by the Worker (`graph/embedding/worker.go:23`):
  `pkg/errs/errs.go:73` — `ErrRateLimited       = errors.New("rate limited")`
  `pkg/errs/errs.go:154-171` — `func IsTransient(err error) bool { ... return ce.Class == ErrorTransient ... errors…`
  (a classified error's class first; else `ErrConnectionTimeout`, `ErrConnectionLost`, `ErrStorageUnavailable`,
  `ErrRateLimited`, `context.DeadlineExceeded`, `context.Canceled`); `pkg/errs/errs.go:366` — `func WrapTransient(…`;
  `:223` — `func Classify(err error) ErrorClass {`. It never reads error text (`errs.go:151-153`).
- A provider-error vocabulary with no production reader: `model/health.go:46-67` — `type ErrorKind string` (`rate_limit`,
  `server_error`, `network`, `timeout`, `unknown`); #155 lists it as having no reader. httpembedder may not import
  `model` (`openspec/specs/embedding-index/spec.md:281`).

## 3. Adjacent claims

- Specs: `openspec/specs/graph-embedding/spec.md:31-52` ("The failed map never moves backwards"), `:151-173`
  (a Worker failure does not withhold), `:235-251` (hung embedder is `timeout`);
  `openspec/specs/embedding-index/spec.md:289-304` (HTTP embedder trusts only a whole reply), `:281` (no `model`
  import); `openspec/specs/graph-query/spec.md:855-867` (provider loss, asserts `connection_refused`).
- ADRs: ADR-066 (`docs/adr/066-honest-graph-index-readiness.md:320-336`: every outcome completes; failed reads
  degraded); ADR-109 (ErrorCount never lowers Healthy); ADR-108 Decision 4 (`docs/adr/108-processor-shell.md:129-133`:
  a lane item applies current truth, re-read at apply time); ADR-111 Decision 3
  (`docs/adr/111-durable-execution-primitive.md:86-98`: whether a blocker may be retried is an
  `errs.ClassifiedError` matched with `errors.Is`, never parsed from text); ADR-060
  (`docs/adr/060-unified-rpc-error-contract.md:28-45`: no new error class; consumers branch on `errs.IsTransient`).
- Docs: `docs/contract.md:34` — tier 1 "If the provider is lost: drops to tier 0; keyword search still answers".
- Open PRs (`gh pr list --state open`, 2026-11-23): #733 (this claim); #731 ready for review (graph/follower,
  entity-state-follower spec, smoke test; no shared file or capability); #695-#698 Dependabot (no overlap).
- Open issues naming the packages: #724 and #732 (this change); #722 (httpembedder width 0 until first reply;
  another defect, stays); #714 (loop-supervision skill, carried with the rule engine port; stays, not this
  package); #712, #683, #723, #11, #729 (other code or epics); the rest only use the word.
- Admission ledger: `docs/admission-ledger.yaml:3856` (`graph/embedding`), `:4632` (`processor/graph-embedding`).

## 4. Who it is for

Nothing new is designed in this phase. Candidates the design may add are listed under its Decisions.

## 5. Problem shape: paced retry of a failed set

- `processor/graph-lexical/bodies.go:15-18` — `A read that fails on a store ... skips that store until the next repair…`
  (synchronous: the pass sees each read's outcome before the next read).
- `processor/graph-index/index.go:315-341` — flat 30 s re-drive of every failed entity (own KV faults, unpaced).
- `processor/graph-embedding/hop1.go:290-301` — flat 30 s re-drive of hop 1's failures.

## 6. Pin decision records

Nothing is carried from the pin.

## Collision table: re-drive of a failed embedding

| Dimension | Evidence |
| --- | --- |
| Semantic class | Re-driving an entity whose embedding failed; deciding whether a failure may be retried |
| Owners | hop 1 repair lane (`hop1.go:290-313`); Worker snapshot retry (`worker.go:418`); `errs.IsTransient` |
| Catalogs | stored reason set (`storage.go:513-518`); `errs` sentinels and classes (`errs.go:60-78,154-171`) |
| Status | failed map → follower `failed_count`/`failed_reasons` (`failed.go:177`); `failed_records` (`query.go:186`) |
| Lifecycle | lane started in Start (`component.go:383`), seeded from `ScanFailed` (`component.go:346-350`) |
| Writers | `SavePendingGuarded` overwrites a failed record at any revision (`storage.go:241-243`) |
| Readers | Worker watch processes every live pending put (`worker.go:414-419`) |
| Recovery | restart: seed plus snapshot retry |

## Adopter seam

- Surface: the reasons in GRAPH_STATUS `failed_reasons` and, for a product that registers its own embedder, how its
  errors are classified.
- Readers of reason values: `gh api search/code q='embedder_error org:C360Studio'` and `'failed_reasons ...'` found
  code only in semengine and semstreams. graph-embedding's `IncFailedReason` is a no-op
  (`processor/graph-embedding/metrics.go:71`), so no metric label exists.
- SemSource's tier-1 qualification (#11) records the reason: semsource
  `docs/testing/setup-03a/tier1-provider-faults-report.json:2580-2581,2670-2671` — `"failed_reasons": { "embedder_error"…`,
  from injected 503s.
- What a product embedder author must know today: nothing about retry. Its errors are classified by text
  (`worker.go:786-805`).
