// Package probe lets a test observe an owner's joins and retained state instead of inferring them
// from timing or goroutine counts (openspec change setup-02-isolated-harness, capability
// lifecycle-suite). Adapted from the per-test shapes in SemStreams
// processor/graph-query/lifecycle_owner_test.go at 5457b345 (ledger row L5), which SemStreams copies
// 26 times (observed context) and 9 times (entered/release). Test-only: contract test T-B1 refuses
// any production import.
package probe
