# Tasks: fix-579-graph-query-embedding-enabled

Tests come first. Each code task ends at a clean-tree local `task verify` (#171); integration tests run under
`task test:integration -- ./processor/graph-query/ ./internal/smoke/`. "Shown to fail" is `task mutate:check` output
or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no test-owned deadline, no
`context.WithValue`, no `t.Parallel` in an integration file. Review stops: after 2.2, before archive.

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner approves (`Approved-by:` in `proposal.md`).
  Gate: `task spec:check`, `task docs:check`.

## 2. graph-query

- [x] 2.1 `graph.ErrorCodeNotEnabled` and its doc comment, plus the sender line at `graph/mutation_responses.go:9-17`
  (design D3). `EmbeddingEnabled bool` in `processor/graph-query/config.go` with its schema tag, `DefaultConfig` setting
  true, and
  `resolveConfig` decoding into `Config{EmbeddingEnabled: true}` (D1). Check: wire-value test beside
  `graph/query_verbs_test.go:202`, `TestConfigEmbeddingEnabled`, the `FuzzDeclarePorts` seed; the preset shown to fail
  when removed.
- [x] 2.2 The refusing handler for `semantic` and `similar` in `queryHandlers` (`query.go:23-80`; D2). Check:
  `TestEmbeddingVerbsRefuseWhenNotEnabled`, `TestEveryDeclaredVerbHasAHandler` for both settings,
  `TestNotEnabledRefusesBeforeForwarding` (integration); existing `TestNoRespondersIsTransient` and
  `TestForwardPassesBytesBothWays` unchanged and green.

## 3. Smoke stack

- [x] 3.1 `internal/smoke/stack_integration_test.go` `graphOnlyConfig`: `"embedding_enabled": false` on graph-query,
  and its comment. `TestSmoke_NoEmbedderAnswers`: the NL refusal is invalid `not_enabled` naming `semantic`; drop the
  `graph.embedding.query.search` lookup and the PR #577 citation; the health comment says the refusal is no error.
  `internal/smoke/doc.go:14-16` updated. Check: the smoke run green; the assertion shown to fail with the key removed
  from `graphOnlyConfig`.

## 4. Docs, ledger, archive

- [x] 4.1 `processor/graph-query/doc.go:27`, `processor/graph-query/README.md` (forward table note at :33-38, option
  table at :307-310),
  `processor/graph-query/MIGRATION.md` (:125, :162-163), `pkg/fusion/README.md:124-126` and
  `pkg/fusion/MIGRATION.md:128-131` (the gap is closed; say how to close it),
  `.agents/skills/query-pattern/SKILL.md:31`,
  the `processor/graph-query` ledger row (`docs/admission-ledger.yaml:4323`) and the fusionnats row's "until then"
  (`:4828-4830`). Gate: `task docs:check`, `task spec:check`.
- [ ] 4.2 Archive: sync the deltas into `openspec/specs/graph-query` and `openspec/specs/composed-stack`, and edit
  `composed-stack`'s Purpose (`:7`, "naming the embedding search subject" becomes "refusing natural-language ones as
  not enabled"). Last content commit.
