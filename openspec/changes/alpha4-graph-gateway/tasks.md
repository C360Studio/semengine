# Tasks: alpha4-graph-gateway

Tests come first. Each code task ends at a clean-tree local `task verify` (#171); integration tests run under
`task test:integration -- ./gateway/graph-gateway/ ./processor/graph-query/ ./internal/smoke/`. "Shown to fail" is
`task mutate:check` or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no test deadline of its
own, no `context.WithValue`, no `t.Parallel` in an integration file. Review stops: after 2.1, after 2.3, before archive.

## 1. Design

- [ ] 1.1 Architect inventory and one-page design; pre-owner design review passes; the owner fills `Approved-by:`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 `graph.query.predicate` (design D3). Premises: `graph/query_verbs.go:95-126`,
  `processor/graph-query/query.go:27-36`, `processor/graph-index/query.go:285-293`. Add the verb row, the forward and
  `graph.PredicateRequest`; graph-index decodes into it. Check: `TestQueryVerbsDeclareEachRespondersVerbs` (row added),
  `TestForwardPassesBytesBothWays` (predicate case), `TestGraphQueryServesExactlyTheDeclaredVerbs`, the graph-index
  predicate tests unchanged and green.
- [ ] 2.2 Schema and executor, no NATS (D1, D2, D6 rows 1-2 and the last row). Add `github.com/vektah/gqlparser/v2`
  (with `github.com/agnivade/levenshtein`). Check: `TestSchemaRefusesUnknownFieldNamingIt`,
  `TestMutationAndIntrospectionRefused`, `TestOneRootFieldPerRequest`, `TestTypenameAnswers`,
  `TestReplyNotFittingSchemaNamesPath` (a null root field leaves the rest of `data`), and
  `TestProjectionHoldsExactlySelectedNames` (rapid: random selections and replies; seed and run count recorded;
  failing mutant shown).
- [ ] 2.3 Component (D2 resolvers and checks, D5, D6), on real NATS with graph-ingest, graph-index and graph-query
  composed. Check: `TestGatewayAnswersFourOperations` (the semsource assertion's queries in migrated form),
  `TestEntityZeroRevisionRefused`, `TestPrefixReplyValidated`, `TestPathsFollowEdgesBack` (pin alignment, `paths[0]`
  empty), `TestDirectionIsCaseInsensitive`, `TestClassifiedRefusalCarriesClassAndCode`, `TestNoResponderNamesSubject`,
  `TestDeadlineIs504`, `TestConfigRefusesRemovedKeys` (one case per key), `TestConfigRefusesExtraPorts`,
  `TestQueryTimeoutIsRead`, `TestRefusedOutsideStartStop`, `TestStopWaitsForInFlightRequest` (through
  `internal/httpadmission`), lifecycle suite.
- [ ] 2.4 `trajectory` is not served (owner question 1). `MIGRATION.md` tells SemTeams to answer it from a component
  of its own mounted at `/<instance>`. Check: `TestSchemaRefusesUnknownFieldNamingIt` has a `trajectory` case.
- [ ] 2.5 Composed smoke (composed-stack delta). Add graph-gateway to the smoke config; POST the four operations through
  the process's HTTP port at `/graph-gateway/graphql`; six components in health; the composed-stack Purpose
  (`openspec/specs/composed-stack/spec.md:4`) and the "five-component" test comments in `internal/smoke` say six. Check:
  `TestSmoke_OneConfigAnswers`, `TestSmoke_BrokerRestartUnderRunningStack`.
- [ ] 2.6 Contract text. Ledger row `gateway/graph-gateway` → `adapt` (destination, contract `graph-gateway` spec,
  dependencies, proving tests); package `README.md` and `MIGRATION.md`; `.agents/skills/query-pattern/SKILL.md:139`
  updated with the gateway row adapted from the pin skill (`:25`). Check: `task ledger:diff -- gateway/graph-gateway`,
  `task docs:check`.

## 3. Close-out

- [ ] 3.1 Before the merge: specs synced and change archived as the last content commit; semsource #251, semteams #406
  and semconnect told of the migration notes. Check: `task spec:check`.
