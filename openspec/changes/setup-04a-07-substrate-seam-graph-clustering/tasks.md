# Tasks: setup-04a-07-substrate-seam-graph-clustering

Tests come first. Each code task ends at a clean-tree local `task verify` (#171); integration tests run under
`task test:integration -- ./processor/graph-clustering/ ./graph/follower/ ./internal/smoke/`. "Shown to fail" is
`task mutate:check` or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no test deadline of its
own, no `context.WithValue`, no `t.Parallel` in an integration file. Review stops: after 2.2, after 3.1, before archive.

## 1. Design

- [ ] 1.1 Pre-owner design review passes; the owner answers the three questions in `proposal.md` and fills
  `Approved-by:`. Hold: OPEN-Q proposal awaits owner approval. Gate: `task spec:check`, `task docs:check`.

## 2. Component

- [ ] 2.1 `follower.Entry.Value` (design option a). Check: `TestEntryCarriesDeliveredValue`, the existing follower
  suite unchanged, `-race`.
- [ ] 2.2 Config, factory, `Register`, shell and feed (D1, D3). Check: `TestConfigRefusesRemovedKeys` (one case per
  key, naming it), `TestConfigRefusesOutputPorts`, `TestDetectionIntervalIsRead`, `TestRecordsFollowChanges`,
  `TestRefusedValueReportedNotWithheld`, lifecycle suite (`TestStopLeavesNothing`, `synctest`).
- [ ] 2.3 Publication (D2) and #525 (D4). Check: `TestNothingPublishedBeforeReplay`, `TestLostFeedStopsPublication`,
  `TestResetStopsPublication`, `TestHealthIgnoresTickFailures`, `TestWatchDeliversAscendingAcrossKeys` (integration,
  pinned nats.go and server image; cites the result in `notes/2.3.md`).

## 3. Composition and seam

- [ ] 3.1 Composed stack: graph-clustering joins `internal/smoke`'s one-config stack (after #583 merges). Check:
  `TestSmoke_OneConfigAnswers` (localSearch answers the lead's level-0 group, a text question answers),
  the restart scenarios keep those answers, `/components/health` lists five.
- [ ] 3.2 `TestTreeImportsNoProviderPlumbing` (`internal/harness/contract`): `go list -deps ./...` reaches no
  `graph/llm`, `model/wire`, `github.com/sashabaranov/go-openai`; shown to fail on a fixture that imports one.

## 4. Close-out

- [ ] 4.1 `processor/graph-clustering/MIGRATION.md`, `README.md`, `doc.go`; ledger rows (`processor/graph-clustering`
  `adapt`; `graph/llm`, `model/wire` homes per question 3; `graph/inference`, `graph/structural` per question 1);
  ADR-108 and ADR-099 port notes; `.agents/skills/query-pattern/SKILL.md:77,99`; `docs/repository-map.md`; spec sync
  and archive as the last content commit.
