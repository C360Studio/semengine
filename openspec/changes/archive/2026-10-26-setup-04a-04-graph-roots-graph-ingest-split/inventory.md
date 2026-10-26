# Inventory: setup-04a-04-graph-roots-graph-ingest-split

base: `c1caa459f3bd403140c1678c5593672abb2b41ee` (origin/main after PR #409). Every `path:line` below is at the base
unless it names another commit.

Status: not yet reviewed. No `INVENTORY PASS`; the design was drafted in the same pass at the caller's direction, as
on `2026-10-26-setup-04a-04-graph-roots-processor-shell`.

Kind of change: tree code, moves only. No pin package is carried, so the pin probe, the shape sweep and the per-package
pin ADR listing (architect contract, Extraction slices, category 6) do not apply. No exported symbol, port, subject,
bucket or config field is added, so category 4 and the adopter seam inventory are empty (section 7).

Briefing hypotheses that did not hold:

- "`git log --follow` works" for the new files: it follows renames only, and `component.go` stays. Measured on a
  scratch repository holding the base and the move as two commits: `git log --follow --oneline --
  processor/graph-ingest/ingest.go` lists the move commit alone. `git blame -C -C` is the tool that traces moved lines
  (design D3).
- "One commit for the move plus one for docs": the repository squash-merges (`AGENTS.md`, "Merge"), so `main` gets
  one commit. The proof of design D3 runs against the move commit, which stays reachable with the pull request
  (`refs/pull/<n>/head`).
- "Moves only, no edits": one test outside the package names `component.go` as the file holding graph-ingest's stream
  consumer, and fails once the consumer setup moves (section 3). Its two expected file names change in the move
  commit.
- "Docs that cite `processor/graph-ingest/component.go:NNN` are many": in the current docs the brief names
  (`docs/*.md`, `.agents/`, `openspec/specs/`) there are none (section 4). The hits are in ADRs and one concept page.
- D8's line numbers are from before PR #409 (the file had 2,643 lines then, `git show a5780bc:processor/graph-ingest/
  component.go | wc -l`); every range below is re-measured. The `component.go` ledger row is at `:2396`, not `:2381`.
- The `processor/graph-ingest` ledger row is `repair-before-port` (`docs/admission-ledger.yaml:2394`), not `adapt`;
  the `component.go` row is `adapt` (`:2419`). Neither is `carry`, so neither is compared with the pin (section 5).

## 1. The file today

`wc -l processor/graph-ingest/component.go`: 2,558. Top-level declarations, with their doc comments, from a `go/ast`
walk of the file (`go/parser.ParseFile` with `ParseComments`, each declaration from its doc comment to its end; the
same list as `grep -n '^func \|^type \|^var \|^const ' processor/graph-ingest/component.go`, which gives the first
line of each declaration instead):

| Lines | Declarations |
| --- | --- |
| `:1-37` | package doc comment, package clause, imports |
| `:39-44` | `var ( _ component.Discoverable = (*Component)(nil) ... )`, the interface assertions |
| `:46-245` | fourteen metric constructors, `newEntitiesUpdatedMetric` to `newStaleSetsMetric` |
| `:247-255` | `type dedupLane`, `const dedupLaneAddBatch` (the label type of the next constructor) |
| `:257-275` | `newDuplicateTriplesSuppressedMetric` |
| `:277-384` | `type Config`, `Config.Validate`, `Config.ApplyDefaults`, `DefaultConfig`, `const (defaultIngestLanes, ingestLaneQueueDepth, ingestGuardMemMaxPerLane)`, `var schema` |
| `:386-395` | `type hierarchyStore`, `hierarchyStore.CreateEntity` |
| `:397-568` | `type Component`, `type graphIngestConsumerBinding`, `type graphIngestCoreSubscription` |
| `:570-692` | `DeclarePorts`, `resolveConfig`, `CreateGraphIngest` |
| `:694-791` | `const metricService`, `Component.registerMetrics` |
| `:793-854` | `Register`; banner "Discoverable Interface" (`:807-811`); `InputPorts`, `OutputPorts`, `Health`, `DataFlow` |
| `:855-1124` | banner "LifecycleComponent Interface" (`:855-859`); `Initialize`, `Start`, `start`, `subscribeForRequests`, `startStatusMetricsLoop`, `Stop`, `release`, `cleanup`, `clearLifecycleHandles`, `initStorage` |
| `:1126-1285` | the boot sweep: `startEntityStateGuard`, `stopEntityStateGuardWatcher`, `sweepEntityStateGuardEntry`, `recordBootSweepPoison`, `classifyStoredStateRMWError` |
| `:1287-1305` | `initHierarchyInference` |
| `:1306-1576` | banner "Subscription Management" (`:1306-1310`); `setupSubscriptions`, `const defaultIngestAckWait`, `ingestDeliveryTiming`, `setupJetStreamConsumer` (with the consume closure), `waitForStream` |
| `:1578-1957` | `decodeEntity`, `wireMessageType`, `ingestEntity`, `prepareFactProjection`, `fillFactProjectionSubjects`, `extractEntityFromMessage`, and the indexing-profile helpers `hasIndexingProfileTriple` to `indexingProfileMetricLabel` (nine) |
| `:1958-2239` | banner "Entity Operations" (`:1958-1962`); `validateEntityID`, `CreateEntity`, `mergeEntityOnLane`, `addToContainers`, `createEntityWithReceipt`, `deleteEntityAtRevision` |
| `:2240-2244` | banner "Triple Operations" |
| `:2245-2319` | cache coherence: `invalidateEntityCacheEntry`, `entityCacheHandle`, `loadEntityCacheGen`, `repopulateEntityCacheEntry` |
| `:2321-2530` | `recordSuppressedDuplicates`, `type addTriplesResult`, `addTriplesLane` |
| `:2532-2558` | `newBatchMissingMetric` |

Lines not listed are blank. The five banners are free-standing comments, attached to no declaration.

Files in `processor/graph-ingest/` (`ls`): 11 non-test files. Of D8's destinations, two exist:
`poison_inventory.go` (290 lines) and `query.go` (653 lines). `metrics.go`, `config.go`, `consume.go` and `ingest.go`
do not exist (`ls processor/graph-ingest/{metrics,config,consume,ingest}.go`: no such file). Test files with a
similar stem exist and do not collide: `metrics_test.go`, `metrics_registry_test.go`, `ingest_metrics_test.go`,
`config_defaults_test.go`, `config_fields_test.go`, `config_strict_test.go`.

Three files open with a package doc comment: `processor/graph-ingest/doc.go:1` — `// Package graphingest provides
graph-ingest, the component that writes entities and`; `component.go:1` — `// Package graphingest provides the
graph-ingest component for entity and triple ingestion.`; `query.go:1` — `// Package graphingest query handlers`.
Not changed here (moves only); the new files carry none.

## 2. Who calls what moves across a file line

Same package, so no call needs a change; listed for the design's grouping.

- `component.go:919` — `if err := c.startEntityStateGuard(ctx, c.entityBucket); err != nil {` (in `start`).
- `component.go:931` — `if err := c.setupSubscriptions(ctx, submitCtx); err != nil {` (in `start`).
- `component.go:1322` — `if err := c.setupJetStreamConsumer(ctx, submitCtx, port); err != nil {` (only caller).
- `component.go:1344` — `ackWait = defaultIngestAckWait` (the constant's only reader, in `ingestDeliveryTiming`).
- `component.go:1370` — `waitForStream := c.waitForStream` (the method's only reader, in `setupJetStreamConsumer`).
- `component.go:2273` and `:2310` — `entityCache := c.entityCacheHandle()`; `query.go:478` — the same line.
  `entityCacheHandle` came with PR #409 (#306), after D8 was written.
- `query.go:540` — `gen0 := c.loadEntityCacheGen(entityID)`; `query.go:562` —
  `c.repopulateEntityCacheEntry(entityID, entity, gen0)`; `invalidateEntityCacheEntry` is called from
  `entity_writes.go:476`, `:485`, `:501` and `poison_inventory.go:103`.
- `classifyStoredStateRMWError` is called at `entity_writes.go:101`, `:105` and `:341` (`return ...,
  c.classifyStoredStateRMWError(ctx, key, current, revision, err)`) and named in comments at `entity_writes.go:97`,
  `:211`, `keyed_ingest.go:308` and `component.go:2017`.

## 3. Rules and guards that name a file

- `natsclient/consumer_policy_callsite_test.go:323` — `wantPort :=
  map[string]int{"processor/graph-ingest/component.go": 1}` and `:331` — `wantPortConfig :=
  map[string]struct{}{"processor/graph-ingest/component.go": {}}`, in `TestConsumerPolicyProductionCallsiteCensus`
  (`:222`). Its comment, `:317` — `// GetConsumerConfig reader (the pin's
  census names the same file once).` Measured on a scratch copy of the base with the split applied: `go test ./...`
  fails only this test, with `canonical port consumer census = map[string]int{"processor/graph-ingest/consume.go":1},
  want map[string]int{"processor/graph-ingest/component.go":1}` and the same for `GetConsumerConfig`; with both names
  changed to `consume.go`, `go test ./natsclient/` passes.
- No other guard names a graph-ingest file: `git grep -n 'processor/graph-ingest/[a-z_]*\.go' -- '*.go'
  ':!processor/graph-ingest'` returns the two lines above only. `internal/harness/contract/cover_test.go:45` names the
  package (`{"processor/graph-ingest", "merged"}`), not a file.
- `openspec/specs/`: no graph-ingest file name (`git grep -n 'processor/graph-ingest/' -- openspec/specs`: empty).
  `graph-transport-boundary` and `graph-entity-writes` name graph-ingest only as a component (33 and 20 lines with the
  word).
- `Taskfile.yml`, `scripts/`, `.github/`: `git grep -n 'component\.go\|graph-ingest/' -- Taskfile.yml scripts .github`:
  empty. `scripts/port-status.sh:88` names the package only.

## 4. Citations of the file

`git grep -n 'component\.go' -- ':!openspec/changes/archive'`, kept where it is graph-ingest's file in this tree:

- Current docs (`docs/*.md`, `.agents/`, `openspec/specs/`, `processor/graph-ingest/README.md`): none. The two hits in
  `docs/admission-ledger.yaml` are the `component.go` row (`:2396`, `:2403`, section 5).
  `git grep -n -E 'graph-ingest/[a-z_]+\.go:[0-9]' -- openspec/specs .agents 'docs/*.md'
  processor/graph-ingest/README.md`: empty.
- ADRs, records as of their date: `docs/adr/108-processor-shell.md:47`, `:79` and
  `docs/adr/113-start-context-bounds-start.md:35`, `:105`, `:117` cite this tree at earlier commits; every cited
  declaration (`Component`, `start`, `cleanup`) stays in `component.go`. ADR-108 `:86` cites graph-index's file.
  `docs/adr/058-*`, `066-*`, `110-*`, `111-*` cite the pin; `109-*` cites `#93:` and `pin:` paths.
- `docs/concepts/16-federation.md:94-95` cites the pin ("At the pin, graph-ingest's `authorizeSubject` ...").
- Go comments: `processor/graph-ingest/hierarchy_replay_integration_test.go:7` — `// createEntity calls
  AddToContainers UNCONDITIONALLY (component.go, before the` (the code is `createEntityWithReceipt`, which moves);
  `processor/graph-ingest/readiness_integration_test.go:204` — `// (component.go), which lifecycle_integration_test.go
  also wraps.` (the `consumeStream` acquisition, `component.go:1442-1443`, which moves); `graph/readiness/
  gauges_test.go:44` — `// graph-ingest's set, built as component.go builds it, exposes exactly its five` (built in
  `registerMetrics`, `component.go:731`, which moves).
- No test cites a line number of `component.go`: `git grep -n -E 'component\.go:[0-9]' -- '*_test.go'` returns only
  `internal/harness/contract/authority_test.go:180`, `:193`, which name a fixture's `types/component.go`.
- Line shifts in the two existing destinations: the moved code needs imports `query.go` and `poison_inventory.go` lack
  (`internal/cache` in `query.go`; `errors`, `pkg/errs` and `nats.go/jetstream` in `poison_inventory.go`, measured by
  `go build` on the scratch split). Every line after their import blocks moves down by one and three lines.

## 5. Ledger

- `docs/admission-ledger.yaml:2337` — `- source_path: processor/graph-ingest`; `:2343` — `destination:
  processor/graph-ingest`; `:2394` — `disposition: repair-before-port`. Names no file of the package.
- `:2396` — `- source_path: processor/graph-ingest/component.go` (SemStreams PR #1437, `source_sha` `0ea823a6...`);
  `:2402-2404` — `destination: >-` / `processor/graph-ingest/component.go (decodeEntity and consume closure), with` /
  `processor/graph-ingest/fact_lane_fence_test.go and fact_lane_fence_integration_test.go`; `:2419` — `disposition:
  adapt`.
- `task ledger:check` compares `carry` entries only: `internal/harness/pindiff/main.go:85-86` — `// runCheck compares
  every carry entry with the pin ... Other dispositions are not compared.` `ledger:diff` resolves an `adapt` row's
  destination by its leading path (`internal/harness/pindiff/ledger.go:42-44`, `:57-78`); `component.go` still
  exists after the split, and so would `ingest.go`.

## 6. Adjacent claims

### Open pull requests

`gh pr list --state open --json number,title,isDraft,headRefName,changedFiles` on 2026-10-26: `[]`. No overlap.

### Open issues naming `processor/graph-ingest`

Queries: `gh issue list --state open --search "graph-ingest" --limit 200` (29 hits), `--search "component.go"` (21),
`--search "graphingest"` (0). Each hit's body and comments were read for a graph-ingest `path:line`.

| Issue | What it cites in graph-ingest | Touched by this PR |
| --- | --- | --- |
| #300 | section G, "splitting the 2,659-line `component.go`"; section D, "the file's own seams"; owner ruling 2026-10-26 (comment 6416204673) gives D8's map | G and D's seams are this PR's work |
| #109 | comment 6250546017 routes #300's G and D seams here; comment 6386549177 `component.go:446`, `:841`, `:889` | the seams; the cited lines stay in `component.go` |
| #111 | body: hierarchy birth gate `component.go:2088-2095,2240-2245,2112` (no commit named) | moves: `mergeEntityOnLane`, `createEntityWithReceipt` to `ingest.go` |
| #134 | body: `component.go:2006, 2090` (`hierarchyInference != nil`), `:1311-1314`, `:1316-1322` and `:326-363` at `6ed84b2` | the two checks move to `ingest.go`; `initHierarchyInference` and `hierarchyStore` stay |
| #301 | body at `8af2d15`: the four test-only fields; comment 6258561493 `component.go:425-427` | readers move: `watchEntityStates` (`:1139`) to `poison_inventory.go`; `waitForStreamInput` (`:1371`) and `consumeStream` (`:1443`) to `consume.go`; `subscribeRequests` (`:970`) stays |
| #106 | `processor/graph-ingest/query.go:27-55`, `:44` | shifts one line (section 4) |
| #9 | epic status comment 6386556850: `component.go:446`, `:841`, `:889`, `:1162-1170`, `query.go:487-488` | a dated status table; cited code stays or is #306's, closed |
| #33 | body: the boot refusal `component.go:739` | stays (`CreateGraphIngest`, now `:665`) |
| #110 | comments 6271450747 `component.go:866-871`, 6416210646 "around `:869`" (`Health`) | stays |
| #196 | body `component.go:444`, `:981`, `:1092` (guard composition) | stays |
| #309 | body: `release` at `component.go:1087-1096` | stays |
| #141, #195, #107, #108, #20, #216 | `entity_writes.go`, `fixture_integration_test.go`, `keyed_ingest.go`, `canonical_mutations.go`, `readiness.go` | files this PR does not change |
| #270 | a test name only | no |

Names the word, not the package: #405 (graph-query; #406, named in the brief, is graph-query too and is not a hit),
issue #105 (`graph-query`'s `component.go`), #280 (`objectstore`'s), #155 (`model`'s), #103, #15, #16, #24, #32,
issues #35, #94, #95, #96, #198, #350 and #7. Issues #302, #303 and #139 are closed (`gh issue view <n> --json
state`: `CLOSED`).

The sections of #300, from its body and the ruling of 2026-10-18 (comment 6250544834): B is done (PR #307); C waits for
a measurement; D lists "fact-lane re-validation; the `pkg/retry` loop; the repeated shapes (`errs.Wrap*` op names, the
pre-write sequence, counter updates, metric constructors); the file's own seams; the fixed inner `WithTimeout`s in the
query handlers; `marshalFittingPrefixPage`'s double marshal"; F and H stay; G is the split. #139's text of the seams
(closed issue, line 39 of its body): "the boot sweep plus `classifyStoredStateRMWError` to `poison_inventory.go` ...;
cache coherence to `query.go`, its only reader; metrics to `metrics.go`; config to `config.go`; the consume closure to
`keyed_ingest.go`". D8 sent the consume closure to `consume.go` instead, and the owner's ruling of 2026-10-26 on #300
restates D8's six names.

## 7. Categories closed empty

- Category 1, the claimed gap: the gap is a file's size, measured above.
- Category 2, other spellings: a move adds no spelling. No declaration is renamed or copied (design D3 proves it).
- Category 4, who it is for: nothing new. `go doc -all -u ./processor/graph-ingest` is identical before and after on
  the scratch split (design D3), so no exported or unexported symbol is added or removed.
- Category 5, the problem shape: a move-only file split. Closest instance in this tree: none.
  `git log --oneline -i --grep='split\|move only\|moves only\|file split' c1caa45` returns no move-only change.
- Category 6, pin ADRs: no pin package is carried. ADR-108 and ADR-113 govern the code that stays in `component.go`;
  this PR changes no decision they record.
- Adopter seam inventory: graph-ingest's exported surface (`CreateGraphIngest`, `Register`, `DeclarePorts`,
  `DefaultConfig`, `Config`, `Component` and its methods) is unchanged, so a consumer has nothing to learn.
- Collision table and intent check: not triggered (no new primitive, no boundary moved).
