// Package runner holds the contract tests for scripts/test-integration.sh (openspec change
// setup-02-isolated-harness, capability integration-test-runner): R1 busy lock, R2 stale lock, R3
// signal forwarding to the process group, R4 canonical argv, environment, and owner-file format,
// plus the pull-failure and leak-check paths. They run the real script against fake `docker` and
// `go` executables and a temporary lock directory, so they need neither Docker nor the host lock.
// The pattern, not the code, comes from SemStreams test/testinfra/integration_runner_contract_test.go
// at 5457b345 (ledger row L8).
//
// The package has no non-test code; this file exists so the directory is an ordinary package.
package runner
