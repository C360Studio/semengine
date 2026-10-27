# harness-boundaries

## ADDED Requirements

### Requirement: Consumer creation goes through the policy-applying entry points

A guard in `internal/harness/contract` SHALL fail, naming the file and the call, when a production Go file outside
its allowlist calls a natsclient policy-applying entry point (`ConsumeInternalStreamWithConfig`,
`ConsumeStreamWithConfig`, `ConsumeStreamWithConfigContexts`, `GetConsumerConfig`) or creates a JetStream consumer
directly (`CreateOrUpdateConsumer`, `CreateConsumer`, `OrderedConsumer`). The allowlist is
`processor/graph-ingest/consume.go` and `natsclient/stream.go`.

#### Scenario: A caller outside the allowlist

- **WHEN** a production file outside the allowlist calls `GetConsumerConfig` or `CreateOrUpdateConsumer`
- **THEN** the guard fails naming that file and the call

#### Scenario: A listed file changes its call count

- **WHEN** `processor/graph-ingest/consume.go` gains a second call to a policy-applying entry point
- **THEN** the guard passes, because it checks which files call, not how often

#### Scenario: An ignored worktree holds a copy of the tree

- **WHEN** `.worktrees/<branch>/natsclient/stream.go` exists under a git-ignored directory
- **THEN** the guard does not read it, because production files are the `.go` files that
  `git ls-files --cached --others --exclude-standard` lists and that are not `_test.go`
