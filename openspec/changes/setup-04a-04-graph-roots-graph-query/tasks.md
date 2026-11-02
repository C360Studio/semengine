# Tasks: setup-04a-04-graph-roots-graph-query

Step 1 of `split.md`. Tests come first. Each code task ends at a clean-tree local `task verify` at its commit (#171);
integration tests run under `task test:integration -- <pkgs>`. "Shown to fail" means `task mutate:check`, or the
failing-first output, recorded in the task's note. One task is one developer's unit.

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner fills `Approved-by:`. Gate: `task spec:check`, `task docs:check`.

## 2. Table and index

- [x] 2.1 `graph.QueryVerb` gains `Enveloped` and `Reply`; existing rows marked; `graph.ResponderGraphQuery`,
  `graph.RelationshipEdge`, `graph.MinRevisionField`, `graph.ErrorCodeRevisionNotReached`. New rows go after existing
  ones, since `processor/graph-ingest/failed_start_callbacks_integration_test.go:61` finds `batch` by name alone; a
  real name lookup checks the responder. Premise: `graph/query_verbs.go:5-11` (D1). Check:
  `TestQueryVerbsDeclareEachRespondersVerbs`.
- [ ] 2.2 graph-index compares `min_revision` on all eight verbs. Premise: `processor/graph-index/query.go:79`,
  `:398-399` (D3). Check: `TestMinRevisionBoundary`, shown to fail with the comparison removed.

## 3. Component

- [ ] 3.1 Shell, strict configuration, `Register`, `doc.go`, `README.md`. No handler reads downstream yet; table rows
  arrive with 3.2. Premise: `processor/graph-index/component.go:43`, `:107` (D7); P1, P2, P3. Check: lifecycle suite
  with the failed-cleanup branch, `TestUnknownConfigKeyIsRefused`.
- [ ] 3.2 Forwards: `entity`, `batch`, `prefix`, `byName`, with their table rows. Premise:
  `pin:processor/graph-query/query.go:299-345` not carried (D2). Check: `TestForwardPassesBytesBothWays` on all four
  verbs, `TestForwardedRefusalKeepsClassCodeDetail`, `TestNoRespondersIsTransient`, `TestQueryTimeoutIsRead`,
  `TestGraphQueryServesExactlyTheDeclaredVerbs`.
- [ ] 3.3 `entityByAlias` (ADR-114) and `relationships`, with their table rows. Premise: P5, P5b,
  `graph-index/query.go:272`, `graph-ingest/query.go:79-82` (D2, D4). Check: `TestEntityByAlias`, on real ingest and
  index except the rows D4 names, `TestRelationshipsEnvelopeKeepsIndexRevision`, `TestRelationshipsRowKeys`,
  `TestRelationshipsRefusesUnknownDirection`, `TestRelationshipsRefusesBrokenIndexReply`, each failing case shown to
  fail.
- [ ] 3.4 `request_subject_conflict` in `composition.Analyze`, with the service's boot refusal; needs the
  `graph.query.entity` row from 3.2. Premise: `composition/analyze.go:28`, `:35`, `service/component_manager.go:328`
  (D6). Check: `TestAnalyzeReportsSecondResponder`, `TestBootRefusesSecondResponder`,
  `TestValidateFindingsVocabularyIsClosed`.
- [ ] 3.5 End to end, real ingest and index: `TestReadYourWrites`, `TestMinRevisionRefusalThroughGraphQuery`
  (`byName`, `relationships` both ways, `entityByAlias`, M far above R, shown to fail with the field re-marshalled
  away), `TestGraphQueryRepliesDecodeByTheTable`. A wait in these tests is on the reply itself, never `time.Sleep`; a
  deadline is an outer bound only (#501).

## 4. Records and guidance

- [ ] 4.1 Port ADR-060 with a note under its title (pin path and SHA, packages, ledger row, ADR-091 amends it and is
  not here, `060-pr-d-*` not carried). Ledger: new `processor/graph-query` row, disposition `adapt`, saying the
  package is whole at step 5; ADR-060 row; `defer-exclude` rows for `answer.go`'s language-model half,
  `component.go`'s wiring and `summary_view.go` (#485, #486, #487, #32); dropped `router.go`; `graph` row no longer
  defers `MinRevisionField`. Check: `task ledger:diff`.
- [ ] 4.2 `.agents/skills/query-pattern/SKILL.md`: the six verbs, `min_revision`, decoding by verb. `AGENTS.md` § 2
  and `.agents/rules-index.md`: the reserved-subject row names `graph.query.*`. Check: `task ateam:doctor`, `task
  docs:check`.
- [ ] 4.3 Amend ADR-114's "Not found keeps falling through" (`docs/adr/114`:113): valid IDs fall through, an unknown
  non-ID is `entity_not_found`, citing this change (D4). `graph/README.md` and `processor/graph-index/README.md:60-66`
  gain `min_revision`, `revision_not_reached` and the table's marks. Check: `task docs:check`.
- [ ] 4.4 Move `consumer-migration.md` to `processor/graph-query/MIGRATION.md`, so it survives this change's archive, and
  point `split.md` at it. Each later step appends what it changes. Check: `task docs:check`.

## 5. Land

- [ ] 5.1 Cross-agent review of record passes at the last code commit (`reviewed-by:`). The PR #504 body carries
  `implemented-by:` and no closing keyword; `split.md`, "Step 1 dispositions", lists what stays open. Check: read
  `closingIssuesReferences` back, empty.
