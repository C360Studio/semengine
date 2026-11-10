# Tasks: setup-04a-07-substrate-seam-graph-clustering

Tests come first. Each code task ends at a clean-tree local `task verify` (#171); integration tests run under
`task test:integration -- ./processor/graph-clustering/ ./graph/follower/ ./internal/smoke/`. "Shown to fail" is
`task mutate:check` or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no test deadline of its
own (no `context.WithTimeout(waitBound)` await helper, #554), no `context.WithValue`, no `t.Parallel` in an
integration file. Review stops: after 2.2, after 3.1, before archive.

## 1. Design

- [ ] 1.1 Pre-owner design review passes; the owner answers the two questions in `proposal.md` and fills
  `Approved-by:`. Hold: OPEN-Q proposal awaits owner approval. Gate: `task spec:check`, `task docs:check`.

## 2. Follower and component

- [ ] 2.1 Follower (design D1, D2): `Entry.Value`, its doc (valid only during `Deliver`; not applied by a plug-in that
  re-reads, ADR-108 D4); `Stuck`'s plug-in-neutral sentence with reasons, updating the five assertions on
  "index writes unresolved" (`graph/follower/follower_test.go:461`, `processor/graph-index/index_integration_test.go:406`,
  `lifecycle_integration_test.go:376`, `processor/graph-embedding/health_integration_test.go:41`,
  `withhold_integration_test.go:191`); `processor/graph-embedding/hop1.go:30` ("The entry carries no value") reworded;
  `IndexStatusResponse.IndexedRevision`'s doc (`graph/index_status.go:62-65`) names the graph-clustering meaning.
  Check: `TestEntryCarriesDeliveredValue`, `TestFollowerDeclaresLostFeedOnOutOfOrderDelivery` (extended to the value),
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

- [ ] 4.1 `processor/graph-clustering/MIGRATION.md`, `README.md`, `doc.go`; ledger rows (`processor/graph-clustering`
  `adapt`; `graph/llm`, `model/wire` per question 2; `graph/inference`, `graph/structural` per question 1); ADR-108
  and ADR-099 port notes; `.agents/skills/query-pattern/SKILL.md:77,99`; `docs/repository-map.md`; spec sync (the
  `composed-stack` Purpose names five components) and archive as the last content commit.
