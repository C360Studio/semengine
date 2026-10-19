# Tasks: one-mutation-port-check

Task numbers are the design's "Tasks" table. Each task ends when its gate passes; 4.1 ends when `task verify` passes
on a clean tree at its commit (#171).

## 1. Pin today's refusals

- [x] 1.1 `TestConfigValidateMutationProviderRefusals` (graph-ingest) through `Config.Validate`, green on main for
  the existing rows; the multi-subject JetStream row red, the run recorded; the version mutant detected by
  `task mutate:check`. Gate: `go test -race ./processor/graph-ingest`.
- [x] 1.2 `TestJetStreamSubjectMatching`'s subjects moved off `graph.mutation.`, green before and after; the `nats`
  output row in `TestGraphMutationFlowRejectsInvalidProviderTopology` red, the run recorded; the required mutant
  detected by `task mutate:check`. Gate: `go test -race ./internal/flowgraph`.

## 2. The shared decision

- [x] 2.1 `TestClassifyPort`, red (does not compile), the run recorded.
- [x] 2.2 `ClassifyPort` implemented; 2.1 green; mutants (a) and (b) detected by `task mutate:check`; the coverage
  floor holds. Gate: `go test -race ./internal/graphmutation` and `task cover:check`.

## 3. Callers

- [x] 3.1 graph-ingest calls `ClassifyPort`; 1.1 green; the `!valid` mutant detected by `task mutate:check`. Gate:
  `go test -race ./processor/graph-ingest` and `task lint`.
- [x] 3.2 flowgraph calls `ClassifyPort`; `portInfo` carries `facts` and loses `Interface`, `kind`, `natsSubjects`;
  1.2 green; the `!valid` mutant detected by `task mutate:check`. Gate: `go test -race ./internal/flowgraph` and
  `task lint`.

## 4. Docs

- [x] 4.1 Docs, the three ledger rows and the spec delta are true of the code; `task verify` passes on a clean tree.
  Gate: `task verify`.

## 5. Review and close

- [ ] 5.1 Cross-agent review; `reviewed-by:` in the PR body.
- [ ] 5.2 Archive (`openspec archive one-mutation-port-check`) as the last content commit. Gate: `task spec:check`
  and `task docs:check`.
