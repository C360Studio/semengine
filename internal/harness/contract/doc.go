// Package contract holds SemEngine's machine-checked harness boundaries (openspec change
// setup-02-isolated-harness, contract tests T-B1..T-B7). Every check lives in a _test.go file and
// runs over the whole repository; each also has a sensitivity test that seeds the violation in a
// temporary tree and requires the check to name it, so a check that silently matches nothing fails.
//
// The package has no non-test code; this file exists so the directory is an ordinary package.
package contract
