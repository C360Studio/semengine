# Proposal: fix-390-391-port-pick-family

## Why

Six of the thirteen `class:flake` issues since 2026-10-01 (#180, #209, #254, #345, #372, #388) came from tests picking
ports on the owner's Mac; none showed in CI on Linux. PR #389 measured why: in 20,000 binds beside a daemon holding
`127.0.0.1:63150`, a `net.Listen("tcp", ":0")` pick (every address, both IPv4 and IPv6) was handed 63150 once, and
a `tcp4` or `127.0.0.1:0` pick never was. `natsfixture.freePort` picks that way and hands the number to Docker, which
binds IPv4 (#345's error, `listen tcp 0.0.0.0:63150`), so the kernel's own conflict check is skipped (#390). Nothing
stops a new or ported test bringing the same pick back, and the fixed-port guard's header names it as the required
pattern (#391).

## What Changes

- `freePort` picks with `tcp4`; the integration test's `deadPort` reuses `freePort`. The #345 retry stays: the gap
  between `freePort` closing its listener and Docker binding the port remains.
- A contract test refuses a port-0 `net.Listen("tcp", ...)` on every address in a `*_test.go` file or under
  `internal/harness/` unless the line above reads `// Every address: <reason>`; the seven held-port sites get one.
- A release check dials the listener's own loopback address: a review line in `docs/testing.md`.
- `scripts/lint-test-ports.sh`'s header stops naming the refused pick and its false sourcing claim.

## Capabilities

### Modified Capabilities

- `harness-boundaries`: ADDED "A port-0 pick on every address in test code says why".
- `nats-fixture`: MODIFIED "Start takes its host port from the operating system" (the pick is IPv4).

## Impact

- Code: `internal/harness/natsfixture/fixture.go`, `fixture_integration_test.go`, `start_port_integration_test.go`;
  `metric/lifecycle_test.go`; `service/metrics_owner_test.go`, `service/pprof_test.go` (comments only);
  `scripts/lint-test-ports.sh` (header); new `internal/harness/contract/portpick_test.go`.
- Docs: `docs/testing.md`, `docs/test-infrastructure.md`, `AGENTS.md`, `.agents/rules-index.md`, the
  `lint-test-ports.sh` ledger row.
- Behaviour: none in production. A test that picks a port on every address without saying why fails `task verify`.
