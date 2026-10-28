# Proposal: fix-195-natsclient-fixture-open

## Why

Issue #195 lists the test code that should open its client through `natsfixture.Open`, the shared helper that
opens a value against a test broker under a time limit, fails the test naming the URL when the open fails, and
registers the close under its own time limit (`internal/harness/natsfixture/open.go:29`; `nats-fixture` requirements
"Connected value for a package's tests" and "Connected value is closed under a deadline"). One package already in
this repository is not on it: `natsclient`'s `newFixtureClientAt` (`natsclient/test_helpers_integration_test.go:81`)
builds its `Client`, registers its close and connects by hand, the three steps `Open` owns. Every other admitted
package with a fixture client (`component`, `config`, `graph/follower`, `graph/kvcatalog`, `pkg/lifecycle`,
`processor/graph-index`, `processor/graph-ingest`, `service`, `storage/objectstore`) already calls `Open`.

Light process, owner 2026-10-28: proposal and tasks; no design document and no spec delta, because no requirement
changes; Codex reviews the code.

## What Changes

- `newFixtureClientAt` gets its `Client` from `natsfixture.Open(t, url, openFixtureClient)`. `openFixtureClient`
  builds the `Client` with the options it has today (`WithTimeout(fixtureClientTimeout)`, `WithMaxReconnects(0)`,
  `WithHealthInterval(0)`, as the pin's `NewTestClient` built it, `test_client.go:777-781`), connects it and waits
  for the connection, and on a failed connect or wait closes the client and returns both errors, as graph-ingest's
  `openFixtureClient` does (`processor/graph-ingest/fixture_integration_test.go:51`).
- What a test sees is unchanged: the same connected `Client` and URL, its close run on cleanup under a 10 s bound
  before the shared reset's connection check. One difference: the open is bounded by `Open`'s 15 s budget, the
  same value as `fixtureClientTimeout`, and a failure message now names the URL.
- Not in scope: `natsclient/health_monitor_integration_test.go:29` builds its own client with a health monitor on
  purpose and stays as it is. Issue #195 stays open for the packages it lists at their port.

## Capabilities

None. No requirement under `openspec/specs/` changes.
