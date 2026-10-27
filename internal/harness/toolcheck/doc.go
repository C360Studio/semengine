// Package toolcheck holds the tests of SemEngine's own tooling: the scripts, Taskfile.yml and the
// CI workflow (openspec change fix-416-tool-test-gating). They moved out of package contract so a
// change that touches none of the files they read need not run them (#416).
//
// The trigger paths are scripts/, internal/harness/, .github/, Taskfile.yml, go.mod, go.sum,
// docs/admission-ledger.yaml, service/base.go, service/service_manager.go and
// service/component_manager.go. A test belongs here when every file of the real tree it reads is
// under a trigger path; anything else it needs is planted in a temporary tree. A test that reads
// any other file of the real tree stays in contract. Only review enforces this rule, so each test
// file's reads are listed here for the reviewer to check:
//
//   - cleanuproots_test.go: scripts/cleanup-roots-check.sh.
//   - cover_test.go: scripts/cover-check.sh and scripts/tree-state.sh.
//   - ledgercheck_test.go: Taskfile.yml.
//   - lintports_test.go: scripts/lint-test-ports.sh.
//   - mergecheck_test.go: scripts/merge-check.sh.
//   - mergegate_test.go: Taskfile.yml, scripts/verify.sh and .github/workflows/ci.yml.
//   - portstatus_test.go: scripts/port-status.sh, docs/admission-ledger.yaml,
//     service/base.go, service/service_manager.go and service/component_manager.go.
//   - specqueue_test.go: scripts/openspec-queue.sh and Taskfile.yml.
//   - testpkgs_test.go: scripts/test-pkgs.sh.
//   - toolschanged_test.go: scripts/tools-changed.sh.
//
// repo_test.go reads go.mod's location only, to find the repository root. The package has no
// non-test code; this file exists so the directory is an ordinary package.
package toolcheck
