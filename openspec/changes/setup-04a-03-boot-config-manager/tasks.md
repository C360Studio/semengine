# Tasks: setup-04a-03-boot-config-manager

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit, or which mutant it detects. Review stops (Codex) come after 2.2 and 3.1. Each task ends only
when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`). Proposal, design and spec delta pass pre-owner review and are
  accepted by the owner, with Q1, Q2 and Q3 answered. Gate: `task spec:check` and `task docs:check`.

## 2. Port (D1-D5)

- [ ] 2.1 Tests first, in `config/`: the eight pin test files with the module path rewritten and adapted per D5
  (`fixtureClient`, `semengine_config`, `Stop(ctx)`, no `Skip`, no sleeps, no fixed address, the two-boot test as
  subtests); the new tests D2, D3 and D4 name, including the lifecycle-suite adapter. Gate: the tests fail on the
  base (the symbols do not exist; note the compile error).
- [x] 2.2 `manager.go`, `key_family.go`, `bucket_name.go` from the pin with the module path and the four renames, D1's
  surface change, D2's lifecycle, and D3 and D4 (Q2 and Q3 ruled yes), whose tests 2.1 already wrote. Gate:
  `go test -race ./config/` and `go test -tags integration ./config/` pass, `TestManagerPassesTheLifecycleSuite`,
  `TestStopWithAModelRegistryWatcherReturnsPromptly` and `TestKVSelectionReplacesServicesAndComponents` (replacing the
  pin's `TestConfigManagerKVSelectionReplacesOnlyServices`) included; the PR body carries `Closes #17`; `task verify`
  passes on a clean tree (review stop).
- [x] 2.3 Folded into 2.2: the 2.1 tests cover D3 and D4, so 2.2's gate cannot pass without them.
- [x] 2.4 Sensitivity by hand (#129): each mutant D5 lists, applied to the working tree, makes its named test fail
  under `go test -tags integration -run <name> ./config/`, and the revert makes it pass, recorded in `notes/2.4.md`.
  Gate: `task verify` passes on a clean tree.

## 3. Text, ledger, coverage (D5, D6)

- [x] 3.1 `config/doc.go` and `config/README.md` Manager sections; the `config` ledger row edited in place per D6;
  the ADR-104 port note; the `docs/repository-map.md` `config/` row; `"config merged"` in `scripts/cover-check.sh`.
  Gate: `go doc ./config` lists only the kept surface; `task verify` passes on a clean tree, which runs
  `ledger:check`, `cover:check` (`config` at or above 80%) and `docs:check` (review stop).

## 4. Verify, review and close

- [ ] 4.1 `task verify` passes on a clean tree at the implementation head; the coverage of `config` (merged profile)
  is recorded on the PR.
- [ ] 4.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 4.3 Any defect found and not fixed is filed; a comment on #196 records that `config.Manager` composes the guard.
  Then the change is archived (`openspec archive setup-04a-03-boot-config-manager`) as the last content commit.
  Gate: `task spec:check`.
