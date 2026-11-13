// Package probe lets a test observe an owner's joins and retained state instead of inferring them
// from timing or goroutine counts (capability lifecycle-suite). It is one shared copy of the
// observed-context and entered/release shapes that tests would otherwise write again each time.
// Test-only: the contract test TestImportGraph refuses any production import.
package probe
