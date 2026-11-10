# Tasks: setup-04a-07-substrate-seam-graph-clustering

Tests come first. Each code task ends at a clean-tree local `task verify` (#171); integration tests run under
`task test:integration -- ./processor/graph-clustering/ ./graph/follower/ ./internal/smoke/`. "Shown to fail" is
`task mutate:check` or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no test deadline of its
own (no `context.WithTimeout(waitBound)` await helper, #554), no `context.WithValue`, no `t.Parallel` in an
integration file. Review stops: after 2.2, after 3.1, before archive.

## 1. Design

- [x] 1.1 Pre-owner design review passes (round 2, at b2c3700); the owner rules on anomaly detection and #32 and
  approves (`Approved-by:` in `proposal.md`). Gate: `task spec:check`, `task docs:check`.

## 2. Follower and component

- [x] 2.1 Follower (design D1, D2): `Entry.Value`, its doc (valid only during `Deliver`; not applied by a plug-in that
  re-reads, ADR-108 D4); `Stuck`'s plug-in-neutral sentence with reasons, updating the five assertions on "index writes
  unresolved" (`graph/follower/follower_test.go:461`, `processor/graph-index/index_integration_test.go:406`,
  `lifecycle_integration_test.go:376`, `processor/graph-embedding/health_integration_test.go:41`,
  `withhold_integration_test.go:191`); `processor/graph-embedding/hop1.go:30` ("The entry carries no value") reworded;
  `IndexStatusResponse.IndexedRevision`'s doc (`graph/index_status.go:62-65`) names the graph-clustering meaning. Check:
  `TestEntryCarriesDeliveredValue`, `TestFollowerDeclaresLostFeedOnOutOfOrderDelivery` (extended to the value),
  `TestStuckNamesReasons`, the follower suite, `-race`.
- [ ] 2.2 Config, factory, `Register`, shell and feed (D1, D3). Check: `TestConfigRefusesRemovedKeys` (one case per
  key, naming it), `TestConfigRefusesOutputPorts`, `TestConfigStartupWait` (defaults and set values, as
  graph-embedding's), `TestRefusedValueReportedNotWithheld`, lifecycle suite (`TestStopLeavesNothing`, `synctest`).
- [ ] 2.3 Ticker and publication (D2) and #525 (D4). Check: `TestDetectionIntervalIsRead`, `TestRecordsFollowChanges`,
  `TestNothingPublishedBeforeReplay`, `TestLostFeedStopsPublication`, `TestResetStopsPublication`,
  `TestPersistentPublicationFailureIsStuck` (Health, envelope reason, the counter), and the integration probe
  `TestWatchOrderAcrossKeys` at the pinned nats.go and server image, its observed order recorded in `notes/2.3.md` as
  an observation, not a promise.

## 3. Composition and seam

- [ ] 3.1 Composed stack: graph-clustering joins `internal/smoke`'s one-config stack; merge `origin/main` and
  re-apply the `composed-stack` delta to the spec as #583 left it. Hold: #583 merges first (it edits
  `internal/smoke` and the `composed-stack` spec). Check: `TestSmoke_OneConfigAnswers` (localSearch and a text
  question answer), `TestSmoke_StackRestartKeepsAnswers`, `TestSmoke_KillMidIngestKeepsAcknowledgedWrites`,
  `TestSmoke_BrokerRestartBetweenRuns`, `TestSmoke_BrokerRestartUnderRunningStack` (five components).
- [ ] 3.2 In `internal/harness/contract/embeddingprovider_test.go`: `TestTreeImportsNoProviderPlumbing` over
  `go list -deps ./...` (no `graph/llm`, `model/wire`, `github.com/sashabaranov/go-openai`), reusing `goList` and
  the provider reader, shown to fail on a fixture listing; and processor/graph-clustering under the direct-`model`
  guard `TestGraphEmbeddingImportsNoProviderModule` has (generalized to take the package and rule). It replaces no
  per-package guard: those also forbid `model`, which `component` reaches for every component (`:47-48`).

## 4. Close-out

- [ ] 4.1 Record the anomaly drop (design D5): the ledger's `graph/inference` row
  (`docs/admission-ledger.yaml:2292-2320`, consumer purpose and known risks), `graph/structural` row (`:2630-2652`),
  `graph/llm` and `model/wire` rows (`:2596-2629`, #32's new exit condition), and every other row naming change 7's
  anomaly work (`git grep -n -i "anomal\|change 7" docs/admission-ledger.yaml docs/tier1-cross-check.md
  docs/repository-map.md`, each hit read); remove `graph.BucketAnomalyIndex` and its catalog row
  (`graph/constants.go:39`, `graph/kvcatalog/kvcatalog.go:191-192`, `kvcatalog_test.go:167`); the ADR-081 and ADR-110
  port notes say the `graph/inference` sites they name are never ported. Comment on #141 that its
  `graph/inference/storage.go:159` site is gone and spatial and temporal remain. PR body: `Closes #525`, `Closes #32`,
  `Closes #198`; #488 stays open (a tier-2 slot, not ruled here).
  Check: `task ledger:diff`, `task test:unit` (`kvcatalog`), `task docs:check`.
- [ ] 4.2 `processor/graph-clustering/MIGRATION.md`, `README.md`, `doc.go`; ledger row (`processor/graph-clustering`
  `adapt`); ADR-108 and ADR-099 port notes; `.agents/skills/query-pattern/SKILL.md:77,99`; `docs/repository-map.md`;
  spec sync (the `composed-stack` Purpose names five components) and archive as the last content commit.
