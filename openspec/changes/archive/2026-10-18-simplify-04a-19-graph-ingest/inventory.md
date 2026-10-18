# Inventory: simplify-04a-19-graph-ingest

base: `8c2064e580201f829a10daf9b89c6f7abfc4b53a` (origin/main `c3c3222f9295af04aaa46dd7c2ae83a54bf43aad` plus the empty
claim commit; draft PR #299)

Status: drafted together with the design at the orchestrator's direction; it has not had its `INVENTORY PASS` yet
(architect contract, Required workflow step 3). Every pin is at the base commit unless it names another.

Question: what in `processor/graph-ingest` the six scoped issues still find on today's tree, who reads each surface the
change touches, and which items of #139 remain. Scope: #174, #183, #139, #287, #295, the #216 test (owner,
2026-10-18, #216 comment 6249597351), epic #223's PR 10 line, and the #190 export listing (#223 comment
6214503370). Not in scope: the #216 fix, #291, #282, #294.

This change brings in no package from the pin. `processor/graph-ingest` is ported (ledger row
`docs/admission-ledger.yaml:2278`, disposition `repair-before-port`), so the pin probe runs are not owed. Pin reads below
are from `gh api repos/C360Studio/semstreams/contents/<path>?ref=8b99efe9` (local copy `/tmp/pr10-pin`, local only).

Repositories read (`docs/inventory-scope.md`): this tree; the pin at `8b99efe9`; working trees, read without git commands,
of semsource `4093d3c`, semconnect `dff1265`, semteams `2a565c25`, semboids `37dbdb0`, semvocab `c8a07be`, semsparql
`51ac535`.

The package holds 6,217 non-test lines in 11 files (`component.go` 2,659) and 81 test files.

## 1. The claimed gaps (re-measured)

### #183 item 1: the backlog latch

- `readiness.go:249` — `func (c *Component) latchBootstrap(outstanding uint64, observationFailed bool) (uint64, bool) {`
  sets `bootBacklogDrained` when outstanding first reads 0 (`:268-270`) and returns it as `bootstrap_complete` (`:272`).
- State: `component.go:464` `bootBacklog`, `:465` `bootBacklogKnown`, `:470` `bootBacklogPartial`, `:475`
  `bootBacklogDrained`. Writers: `readiness.go:191` (bind read failed), `:199` (bind read), `:259-266` (first full tick).
- The boot sweep runs in `start` before the status loop: `component.go:999` — `if err := c.startEntityStateGuard(runCtx,
  c.entityBucket); err != nil {`, loop started at `:1021`. So `Start` returning means the sweep finished (ADR-109
  Decision 3).
- `readiness.go:262` — `} else if c.bootBacklog.Load() == 0 && outstanding > 0 {` is #139 B4's fallback branch, still
  present. `registerBoundConsumer` is followed inline by `recordBindBacklog` in `setupJetStreamConsumer` (`component.go`
  after `:1527`), and the loop starts after `setupSubscriptions` returns (`:1011`, `:1021`).
- Test readers: `readiness_test.go` calls `latchBootstrap` 13 times (`:18-211`) and builds 12 `&Component{}` literals.
  `readiness_integration_test.go:135`, `:249`, `:298`, `:380` and `readiness_gauges_integration_test.go:68` assert
  `BootstrapComplete` true once caught up; none asserts it false.
- Envelope doc that names the latch: `graph/index_status.go:106-107` — "BootstrapScope is the SIZE OF THE INITIAL BUILD
  this producer latched / BootstrapComplete against".

### #183 item 2: `published_at`

- It lives outside graph-ingest: `graph/index_status.go:135` — `` PublishedAt string `json:"published_at"` ``; writer
  `graph/readiness/publisher.go:76` — `status.PublishedAt = time.Now().UTC().Format(time.RFC3339Nano)`, doc `:64-67`.
- Tests: `graph/readiness/publisher_test.go:209` `TestPublishStampsPublishedAt`, `:130-133`;
  `graph/index_status_test.go:137-143`.
- Spec: `openspec/specs/graph-transport-boundary/spec.md:95` — "### Requirement: The readiness envelope carries its
  publish time and no legacy fields", scenario `:102-106`, Purpose `:8-9`.
- Ledger: `docs/admission-ledger.yaml:1990` (`graph` row), `:2041` (`graph/readiness` row: "ADR-109 removes it in #183").
- Readers: none in this tree outside the writer and the tests (`git grep -n -i 'published_at\|PublishedAt'`, results
  above); none in the six consumer trees (`grep -rn published_at` hits only semteams' unrelated GitHub-release client,
  `cmd/semteams/tools/githubread/client.go:82`). Ruled: ADR-109 Q1 on #9 comment 6111608867
  (`docs/adr/109-ready-after-boot.md:218`).

### #183 item 3: `Health()`

- `component.go:877` — `Healthy:    c.running && errorCount == 0 && c.entityPoisonSize.Load() == 0,`; message branch
  `:871-872` ("errors occurred during processing").
- `c.errors` writers: nine `atomic.AddInt64(&c.errors, 1)` sites, all in `component.go` (`:1536`, `:1567`, `:1669`,
  `:2190`, `:2293`, `:2304`, `:2344`, `:2551`, `:2557`); the declaration is `:527`; readers are `Health` and `DataFlow`.
  No reset.
- The failed-observation state reaches only the envelope: `readiness.go:143` — `observationFailed = true`, passed to
  `ComputeBacklogStatus` (`:159`), which forces `degraded` (`graph/readiness/index_status.go:115`). Nothing records
  it where `Health()` can read it.
- `ErrorCount` stays on `component.HealthStatus` (`component/discovery.go:66` area). Test readers of `ErrorCount`:
  `component_test.go:615`, `interrupted_work_test.go:194-425` (deltas, unaffected).
- Doc: `README.md:177-181` — "reports healthy when it is running, no processing error has been counted, and no stored
  entity fails to decode". Spec wording: `openspec/specs/graph-ingest-recovery/spec.md:84` — "the error count its health
  reads" and `:92` "the health error count is unchanged".
- No test today drives the failed-observation state into `Health()`, or a nonzero `ErrorCount` with `Healthy` true
  (`grep -n Healthy *_test.go`: poison tests at `poison_scoping_test.go:87`, `:111`, `:684`;
  `poison_scoping_integration_test.go:90`, `:132`, `:158`; lifecycle at `component_integration_test.go:57-60`).

### #216: a lost consumer

- `readiness.go:215-217` — `consumer, err := stream.Consumer(ctx, consumerName)` / `return 0, errs.WrapTransient(err,
  …, "look up consumer "+consumerName)`. A deleted durable consumer makes each tick's observation fail. Owner ruling and
  the unprobed nats.go reading: #216 comment 6249597351. This change states no nats.go behaviour; the test deletes the
  consumer through the JetStream API and observes graph-ingest's own lookup.

### #174: bookkeeping after an unconfirmed write

- The write seam's five writes, each with bookkeeping only on success: `entity_writes.go:122-129` (`createEntity`,
  `c.committed` at `:127`), `:281` (`replaceEntity`), `:375-382` (`replaceEntityAtRevision`), `:400-464`
  (`appendEntityTriples`, `:462`), `:468-476` (`deleteEntity`, `:473-474`). `committed` at `:481-484` clears poison and
  invalidates the cache entry.
- `natsclient/kv.go:324-328` — "a write that returns an error may have committed. So the callback runs again only after
  a failed read, a revision conflict, or jetstream.ErrNoStreamResponse … any other write error ends the call". The report
  half is fixed (#174 comment, PR #93 `1873b26`).
- Repair helper that already re-reads: `poison_inventory.go:148` — `func (c *Component) verifyEntityPoisonRecord(ctx
  context.Context, entityID string) {`, one `Get`, drops the record on not-found or valid bytes.
- Cache: one reader, `query.go:465` `fetchEntitiesConcurrent` (batch and prefix); the stale-repopulation guard
  `component.go:2384-2422`.
- Fault injection the tests already use: `internal/harness/natsfixture/faultkv.go:56` — `func (k *FaultKV) FailAfter(op
  KVOp, err error)`, ops put/create/update/delete (`:15-18`); used at `commit_ambiguity_integration_test.go:127`.

### #287: help text

- `component.go:86` — `Help: "Graph-mutation requests rejected (success=false), by subject and reason (MutationResponse
  ErrorCode; 'unclassified' when absent)",`. Recorded value: `mutation_runtime.go:56-60` (classified code, else
  `graph.ErrorCodeInternal`). No test or doc pins the help text (`git grep -n "unclassified\|MutationResponse
  ErrorCode"` returns only `component.go:86`).

### #295 and the other test-only fields on `Component`

| Field | Decl | Production read | Test writers | At the pin |
| --- | --- | --- | --- | --- |
| `statusInterval` | `component.go:481` | `readiness.go:93` | `readiness_integration_test.go:80`, `:178`, `:288`, `:367`; `readiness_gauges_integration_test.go:57` | `readiness.go:94` |
| `statusNowFn` | `:482` | `readiness.go:307` | none | `readiness.go:326` |
| `repopulateHook` | `:513` | `query.go:559` | `cache_stale_repopulation_integration_test.go:54`; `failed_start_callbacks_integration_test.go:150` | `query.go:737` |
| `waitForStreamInput` | `:414` | `component.go:1452` | `config_fields_test.go:214`; `guard_generation_test.go:234`; `statement_metadata_test.go:83`; `failed_start_callbacks_integration_test.go:365` | `component.go:497` |
| `consumeStream` | `:415` | `component.go:1524` | 9 test files (`config_fields_test.go:217`, `failed_start_callbacks_integration_test.go:255`, `:369`, `guard_generation_test.go:236`, `lifecycle_adapter_test.go:36`, `lifecycle_integration_test.go:124`, `statement_metadata_test.go:84`, `readiness_integration_test.go:194`, `settlement_integration_test.go:122`, `long_apply_integration_test.go:74`) | `component.go:498` |
| `watchEntityStates` | `:422` | `component.go:1220` | `lifecycle_integration_test.go:230` | absent |
| `subscribeRequests` | `:425` | `component.go:1049` | `failed_start_callbacks_integration_test.go:163`, `:388` | absent |

Search: `grep -n -w -e <field>` over the package; pin: `grep -n -w` over `/tmp/pr10-pin/{component,readiness,query}.go`.
`handleMessage` (`component.go:1659`) has no production caller; test callers `fact_lane_fence_test.go:246`,
`merge_entity_integration_test.go:230`.

### #139 at the base, item by item

Items are #139's own letters (body, and comment 6080798880 for F–J). "Fixed" items are closed by earlier commits.

| Item | Status at base | Pin |
| --- | --- | --- |
| A test hooks | present (table above) | — |
| A1 nil guards on what the factory refuses or always builds (logger, metrics, payload registry) | present | `canonical_mutations.go:39`, `:49`, `:105`, `:184-190` (ruling J), `authority_gate.go:80`, `:83`, `poison_inventory.go:71`, `:114`, `keyed_ingest.go:317`, `:392`, `:463`, `:483`, `entity_writes.go:170`, `:267`, `mutation_runtime.go:67`, `:95`, `:163`, `:166`, `readiness.go:108`, `:334`, `query.go:179`, `component.go:2029`, `:2040` |
| A2 nil guards on handles `Start` builds | present; reachable before `Start` through exported `CreateEntity` (`component.go:2075` to `:2336`) | `entityBucket`: `poison_inventory.go:149`, `component.go:2336` (set at `:1175`); `entityCache`: `component.go:1144`, `:2385`, `:2411`, `query.go:486`; `hierarchyInference` (nil when `enable_hierarchy` is off): `component.go:2167`, `:2224` |
| A `handleMessage`; `c.name` | `handleMessage` present; `c.name` now read (`component.go:1527`) | `component.go:1659` |
| A hand-built literals | 27 in 9 files (`grep -o '&Component{'`): `readiness_test.go` 12, `lifecycle_owner_test.go` 4, `test_owner_test.go` 4, `keyed_ingest_test.go` 2, one each in 5 more | — |
| A `panicTB` | present | `component_fixture_test.go:105` |
| B one read | fixed | `mutation_runtime.go:195` `readEntity` |
| B two metering wrappers, re-decode | present; label is the classified code (ruling L done, `canonical_mutations.go:61-67`) | `canonical_mutations.go:31-112`, `mutation_runtime.go:41-63` |
| B invalid/internal spellings | present | `query.go:114`, `:225`, `:242`, `:272`, `:284`, `:307`, `:422` |
| B readiness fallback branch | present | `readiness.go:262-266` |
| C three watch flags (ruling H) | fixed (no hits for `entityWatchLost`, `entityBootstrapStarted`, `entityBootstrapComplete`) | — |
| C entity cache | present | `component.go:433`, `:2384-2422`, `query.go:465` |
| C `handlesMu` (ruling I) | present | `component.go:451`; 10 production lines; `running` `:440` |
| D fact lane validated repeatedly | present | `component.go:1732` `ingestEntity`, `keyed_ingest.go:293` |
| D two append functions | fixed (`addTripleLane` gone) | — |
| D dropped revisions | fixed | `entity_writes.go:481` |
| D `waitForStream` re-implements `pkg/retry` | present | `component.go:1621-1657` |
| D config defaults (ruling K) | present: `Validate` clamps (`component.go:319-321`), `ApplyDefaults` makes an empty port set (`:328-330`) | — |
| D status counters | present: `messagesProcessed` counts writes (`canonical_mutations.go:688`, `component.go:2194`, `:2310`, `:2348`) | — |
| D profile loops, reserved-source literal (= F7) | present | `component.go:1964` — `Source:     "graph-ingest-indexing-profile",` beside `graph/sources.go` `SourceIndexingProfile` |
| D repeated shapes | present | — |
| D methods that never read the receiver | four: `sweepEntityStateGuardEntry` (`component.go:1285`), `extractEntityFromMessage` (`:1831`), `validateTriplePredicates` (`mutation_runtime.go:174`), `classifyEntityQueryError` (`query.go:445`) | AST scan, below |
| D file seams; G file split | present (`component.go` 2,659 lines) | — |
| D small: inner timeouts, prefix page | present | `query.go:64`, `:108`, `:236`; `query.go:356` |
| D `DebugStatus` | present; pin reader `service/component_manager_http.go:496-497` (`service` admitted, change 3) | `poison_inventory.go:278` |
| E `doc.go` contradicts code | present: "Triple mutations (add, remove)" (`doc.go:28`), example config with no mutation port (`:37-48`) | — |
| E carried cites | present (e.g. `readiness.go:69`, `:186-190`, `:278`; `keyed_ingest.go` gh#480) | — |
| F1 append outcome in six places | present | `canonical_mutations.go:413` `validateCanonicalAppendAccounting`; `FailedSubjects` 5 production lines |
| F2, F3 replace rule placement | present | `canonical_mutations.go:586-671`; `entity_writes.go:153-288` |
| F4 single-caller wrappers | `deleteEntityAtRevision` present (`component.go:2322`) | — |
| F5 two refusal types for one metadata rule | present | `entity_writes.go:69-80` vs `statement_metadata.go:28` |
| F6 "wrote nothing" two ways | present | `entity_writes.go:153` (revision 0), `:388-392` (`outcome`) |
| F8 `readFound` | present | `entity_writes.go:153` parameter |
| F9 three poison-clear functions | present | `poison_inventory.go:197`, `:209`, `:220` |
| F10 `staleSets` nil check; nil guard bucket branch | present; the nil-bucket branch is required by spec | `entity_writes.go:267`; `keyed_ingest.go:430`, `:497`; `openspec/specs/graph-ingest-recovery/spec.md:107-108` ("when graph-ingest has no applied-sequence bucket at all, it SHALL log and count") |
| F11 natsclient wrappers | in `natsclient`, outside this package | `natsclient/kv.go:298-348` |
| F12 nits | present | `canonical_mutations.go:509`; test fakes |
| H exported `CreateEntity` | present; consumer caller semsparql `cmd/semsparql/main_test.go:314` | `component.go:2075`, `:2234` |
| I generated-code headers | present: revive skips these files | `component_test.go:1`, `entity_validation_test.go:1`, `hierarchy_sync_integration_test.go:3`, `metrics_test.go:1` |
| I tests that cannot fail | present | `metrics_test.go:31-80`; `structural_predicate_gate_test.go:293` |
| J nits | two comments on `platform` present (`component.go:515-522`); README Health line rewritten by #183 | — |

Receiver scan: a `go/ast` program over the package's non-test files (`/tmp/pr10-tools/recv/main.go`, local only) listing
`*Component` methods whose body never names the receiver; the four above are all. `entity_writes.go` has none: each of
its methods reads `c.entityBucket`, a metric, or `classifyStoredStateRMWError`.

## 2. Every spelling of the facts being modeled

- **"Is graph-ingest stuck?"** `Health()` (`component.go:849-884`) and the envelope's `state`
  (`readiness.go:123-163` via `ComputeBacklogStatus`). Two readers of one observation after #183: the envelope's
  `observationFailed` and the new `Health()` term must come from one tick's result, not two computes
  (`readiness.go:99-102` already states the one-compute rule for gauges and key).
- **"Is the boot work done?"** `startCompleted` (`component.go:448`, refuses requests), `running` (`:440`), the guard's
  state (`lifecycle` `:444`), and `bootBacklogDrained` (`:475`, the envelope). After #183 the envelope's flag reads the
  same fact as `Start` returning.
- **Write committed / write unknown.** `committed` (`entity_writes.go:481`), `clearEntityPoisonOnDelete` (`:473`), and
  the three poison-clear functions (`poison_inventory.go:197-227`); the typed client's view in
  `pkg/projection/mutation_client.go:420-429`.

## 3. Adjacent claims

### Open pull requests

`gh pr list --state open --json number,title,changedFiles,files` at the base: #299 (this claim, 0 files); Dependabot PRs
242–248 (`go.mod`, `go.sum`, `package*.json`, one workflow). No overlap with `processor/graph-ingest`, `graph`,
`graph/readiness` or `openspec/specs`. A Dependabot merge that changes `go.sum` is merged in with `origin/main`.

### Open issues naming the package

Query: `gh issue list --state open --search "graph-ingest" --limit 100` (31 hits) and `--search "graphingest"` (0).
Hits that name the package by path, ledger row or title:

| Issue | What it says about graph-ingest |
| --- | --- |
| #139, #174, #183, #287, #295 | scope (above) |
| #216 | stall after a consumer loss; test only here |
| #291 | `Start` connects while reconnecting (`component.go:982-984`); out of scope (brief) |
| #282 | what `Start`'s ctx owns; out of scope (brief) |
| #270 | `TestIntegration_TypedClientPreservesCommitAmbiguity` failed once, cause unknown; no `class:flake` label |
| #20 | `status:needs-decision`: whether graph-ingest owes `rejectInternal` at `canonical_mutations.go` (comment) |
| #111 | silent degrades: hierarchy at birth, guard value (`component.go` cites at `0bbd78f`-era lines) |
| #196 | lifecycleguard adoption; graph-ingest passes two refusal values |
| #141 | adoption of `UpdateWithRetryRead`; graph-ingest's site done (#139 dedupe) |
| #195 | `natsfixture.Open`; graph-ingest already on it |
| #106 | responder-owned verb catalog; `query.go:44` |
| #107 | graph-index adopts `KeyedPool` as graph-ingest did |
| #109, #110 | processor shell; readiness trio (envelope part done) |
| #33 | refusal text at the factory |
| #134 | graph/inference config built in graph-ingest |
| #103 | query replies envelope |
| #15, #16 | recovery across stream generations; reserved subjects |

Names the word, not the package: #7, #9, #24, #223 (epics), #94, #95, #96 (query features), #198.

### Specs, ADRs, ledger

- Specs: `graph-transport-boundary` (`:95-106`, envelope), `graph-ingest-recovery` (`:78-92`, error count wording),
  `graph-readiness` (`:12-19`, the gate reads `bootstrap_complete`; no SemEngine production caller of
  `EvaluateReadinessGate`, `git grep -n 'EvaluateReadinessGate('` non-test: only its definition), `lifecycle-suite`.
- ADRs: 109 (binding for #183), 083 (envelope), 108 (processor shell), 112 (library statements), 079 at the pin
  (poison as a Health term, cited by ADR-109 Decision 2).
- Ledger: `processor/graph-ingest` row `:2278-2329` (`known_risks` names #174; `proving_tests` list), `graph` `:1990`,
  `graph/readiness` `:2041`.
- Coverage floor: `scripts/cover-check.sh:31` — `"processor/graph-ingest merged"`; also `graph merged`,
  `graph/readiness unit`.

## 4. Readers of the surface (#190)

Exported symbols of the package (`grep -n '^func [A-Z]\|^func (c \*Component) [A-Z]\|^type [A-Z]'`) and the two
outside it this change touches:

| Symbol | Reader | Kind |
| --- | --- | --- |
| `Config`, `DefaultConfig` | semconnect `gateway/cs-api/beta160_structural_integration_test.go:124`; semsparql `reader/nats_test.go:623` | API: `README.md` configuration section |
| `Config.Validate`, `Config.ApplyDefaults` | `component.go:621-622` (`resolveConfig`), `:938` | plumbing |
| `DeclarePorts`, `CreateGraphIngest`, `Register` | `component.go:812-829`; semconnect `cmd/cs-graph-backend/main.go:126`; semboids, semsource, semsparql tests | API: `component-registration` spec |
| `Component` and its `Discoverable`/lifecycle methods (`InputPorts`, `OutputPorts`, `Health`, `DataFlow`, `Initialize`, `Start`, `Stop`) | `component` interfaces; consumers call `Initialize`/`Start`/`Stop` (semsparql `reader/nats_test.go:638-647`, semsource `internal/governance/live_graph_integration_test.go:94-101`) | API: `lifecycle-suite` spec; `Health` meaning: ADR-109 Decision 2, `README.md:177` |
| `CreateEntity` | `component.go:392` (`hierarchyStore`); semsparql `cmd/semsparql/main_test.go:314` | plumbing |
| `DebugStatus` | pin `service/component_manager_http.go:496-497` via `component.DebugStatusProvider` (`component/discovery.go:76`) | plumbing (admitted `service`, change 3) |
| `graph.IndexStatusResponse.PublishedAt` | none | removal ruled (ADR-109 Q1) |
| `readiness.Publisher.Publish` | `readiness.go:329` | plumbing |

No consumer reads `Health()`, `DataFlow()`, `PublishedAt` or `published_at` (consumer `grep -rn` above). No production
caller of `Health()` exists in this tree yet (`git grep -n '\.Health()' -- '*.go' ':!*_test.go'`: none); `service`
(change 3) will be its reader.

## 5. Problem shape

- **Stuck state read by `Health()`**: the poison term (`component.go:862-877`, a counter the writer maintains, read with
  one atomic load). The failed-observation term adopts the same shape: state the status loop writes, `Health()` reads.
- **Bookkeeping after a write whose outcome is unknown**: the closest instance is `verifyEntityPoisonRecord`
  (`poison_inventory.go:148`), a re-read that settles a record it cannot prove from the write.
- **Driving a periodic loop in a test without a test-only interval**: `graph/readiness/watcher_test.go` and
  `publisher_test.go` use `testing/synctest` (PR #289); `readiness.Publisher` takes a `StatusWriter` interface
  (`graph/readiness/publisher.go:41`).

## 6. Pin decision records

No package is carried from the pin. ADRs governing what the change touches: ADR-109 (this repository, binding: Decisions
2 and 3, Q1); ADR-083 (envelope, this repository); pin ADR-079 D3 (poison degrades Health; kept by ADR-109).

## 7. Shape sweep (current tree)

The sweep's findings are #139's table above; this change adds none of its own beyond:

1. `Health()`'s message branch (`component.go:871-872`) names "errors occurred during processing" while the component
   is healthy in every other respect — it goes with the error-count term.
2. `computeBacklogStatus` logs one WARN per consumer per tick on a failed observation (`readiness.go:144-147`): with a
   lost consumer that is one line every 5 s (`graph/readiness/watcher.go:71` `DefaultHeartbeat = 5 * time.Second`).
   Not changed here; noted for #216's fix.

## Adopter seams

- **`Health()`** (component authors, `service` in change 3, operators): after the change, `Healthy` false means a
  named stuck state holds (poison, failed backlog observation) and clears without restart. Default path for someone who
  learns nothing: `/readyz` (change 3) follows it; nothing to know. A count of errors no longer lowers readiness; it
  stays visible as `ErrorCount` and in metrics.
- **The envelope** (semsource's MCP gateway reads graph-index and graph-embedding keys, ADR-109 Decision 3; semconnect's
  conformance probe asks the gate across a process): `published_at` disappears from the JSON (no reader found), and
  graph-ingest's `bootstrap_complete` is true from its first publish. A gate reading graph-ingest's key with a backlog
  standing answers `DeferNone` where it answered `bootstrap_incomplete` (ADR-109 Decision 3, ruled).
- **Metrics**: one help string changes; no name or label changes.
- No new exported symbol, config key, subject or bucket.
