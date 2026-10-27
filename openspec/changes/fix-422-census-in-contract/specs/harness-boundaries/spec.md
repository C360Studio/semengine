# harness-boundaries

## ADDED Requirements

### Requirement: Consumer creation goes through the policy-applying entry points

The `internal/harness/contract` guard SHALL fail, naming file and method, when a production file references (call or
method value) a policy entry point (`ConsumeInternalStreamWithConfig`, `ConsumeStreamWithConfig`,
`ConsumeStreamWithConfigContexts`, `GetConsumerConfig`) outside `processor/graph-ingest/consume.go`, a direct creation
(`CreateOrUpdateConsumer`, `CreateConsumer`, `OrderedConsumer`) outside `natsclient/stream.go`, or both
`GetConsumerConfig` and `ConsumeInternalStreamWithConfig`.

#### Scenario: A caller outside the allowlist

- **WHEN** a production file outside both allowlists references `GetConsumerConfig` or `CreateOrUpdateConsumer`,
  `processor/graph-ingest/consume.go` creates a consumer directly, or `natsclient/stream.go` references an entry
  point
- **THEN** the guard fails naming that file and the method

#### Scenario: A listed file changes its call count

- **WHEN** `processor/graph-ingest/consume.go` gains a second call to a policy-applying entry point
- **THEN** the guard passes, because it checks which files call, not how often

#### Scenario: A port-backed file on the internal path

- **WHEN** a production file reads `GetConsumerConfig` and consumes through `ConsumeInternalStreamWithConfig`
- **THEN** the guard fails naming that file, even when the file is on the allowlist

#### Scenario: An ignored worktree holds a copy of the tree

- **WHEN** `.worktrees/<branch>/natsclient/stream.go` exists under a git-ignored directory
- **THEN** the guard does not read it, because production files are the `.go` files that
  `git ls-files --cached --others --exclude-standard` lists and that are not `_test.go`
