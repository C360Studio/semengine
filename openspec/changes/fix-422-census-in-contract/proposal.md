# Proposal: fix-422-census-in-contract

## Why

`task test:unit` fails on any checkout whose `.worktrees/` holds other worktrees (#422). Two natsclient tests,
`TestConsumerPolicyProductionCallsiteCensus` and `TestConsumerPolicyDirectCreationCallCensus`, walk the whole
repository with `filepath.WalkDir` and skip only `.git`, `.claude` and `vendor`, a skip list from PR #48 when agent
worktrees lived under `.claude/worktrees/`. With 19 worktrees under `.worktrees/` the census counted 20 copies of
the production tree and failed; on a bare tree each worktree is another full parse of the module.

The two tests are a repository-wide rule, not a natsclient behaviour: production code creates JetStream consumers
only through natsclient's policy-applying entry points, and those entry points are called from a known set of
files. The repository's other tree-wide rules live in `internal/harness/contract` and list files with
`git ls-files --cached --others --exclude-standard` (`repoFiles`, `contract/repo_test.go`), which never sees an
ignored path. The owner ruled on 2026-10-27 (#422, comment of that date): move the census to the contract package as
an allowlist rule. Light process: proposal, tasks, one spec delta; no design document; Codex reviews the code.

## What Changes

- **The guards move to `internal/harness/contract`.** Every test in `natsclient/consumer_policy_callsite_test.go`
  (the two censuses, the exported-API census, the `NewDurableHandler` and `ConsumeWithHeartbeat` retirement guards,
  the delivery-disposition and child-lifecycle surface guards) and their sensitivity tests move to one or more
  `contract/*_test.go` files. The file leaves natsclient. They run once per verify in `test:unit`, as the other
  guards do, and never in `test:repeat` (which already leaves `contract` out).
- **Files come from `repoFiles`.** The private walker `parseProductionGoFiles` and its skip list go.
  `TestParseProductionGoFilesIgnoresClaudeWorktrees` goes with it: an ignored path cannot reach a `git ls-files`
  listing, and `repoFiles` already drops a deleted tracked file. Production Go files are the listed `.go` files
  that are not `_test.go`, parsed with `go/parser` as before.
- **A rule, not a count.** Each census checks which production files reference its names, a call or a method
  value, against its own allowlist: the policy-applying entry points (`ConsumeInternalStreamWithConfig`,
  `ConsumeStreamWithConfig`, `ConsumeStreamWithConfigContexts`, `GetConsumerConfig`) only in
  `processor/graph-ingest/consume.go`, and direct consumer creation (`CreateOrUpdateConsumer`, `CreateConsumer`,
  `OrderedConsumer`) only in `natsclient/stream.go`. A reference in any other file fails the guard naming the file
  and the method, and so does a file that reads `GetConsumerConfig` and consumes through
  `ConsumeInternalStreamWithConfig`. The number of calls inside a listed file is not checked, so a listed file may
  change without editing the test. Adding a caller is a deliberate edit to the allowlist, reviewed as such.
- **Sensitivity tests stay.** The planted-caller tests plant a file outside the allowlist in a temporary tree
  built with the contract package's `writeTree`, and require the guard to name it. The retirement guards' planted
  cases move unchanged.
- **Ledger and rules.** The `natsclient` row of `docs/admission-ledger.yaml` stops saying "each later port updates
  their expected maps" and says the guards live in `contract` as an allowlist. One row is added in domain 3 of
  `AGENTS.md` and `.agents/rules-index.md`: consumer creation goes through the policy-applying entry points, called
  only from the allowlisted files; enforced by the contract guard. `harness-boundaries` gains one requirement
  stating the rule and its guard.

## Capabilities

### Modified Capabilities

- `harness-boundaries`: added "Consumer creation goes through the policy-applying entry points".
