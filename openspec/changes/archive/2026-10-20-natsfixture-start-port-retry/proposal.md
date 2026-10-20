# Proposal: natsfixture-start-port-retry

## Why

Issue #345 is labelled `class:flake`, so `scripts/merge-check.sh` blocks every pull request while it is open. In
`internal/harness/natsfixture`, `Start` asks the operating system for an ephemeral TCP port (`freePort`:
`net.Listen("tcp", ":0")` followed by `l.Close()`) and passes that port number in `HostConfig.PortBindings` to Docker
(#209, #210). Because Docker runs in a separate process (`com.docker.backend` on macOS, `dockerd` on Linux), the test
process must close its listener before Docker binds `0.0.0.0:<port>` during `ContainerStart`. Currently,
`prepareDocker(ctx, f.logf)` (which connects to Docker and sweeps >30-minute-old containers on the first call in a
process) runs after `freePort()` closes the listener, widening the window before `ContainerStart` binds the port, and
`Start` makes only one attempt when another process or container binds that port in the gap (`Error response from
daemon: ports are not available: exposing port TCP 0.0.0.0:<port> -> 127.0.0.1:0: listen tcp 0.0.0.0:<port>: bind:
address already in use`).

Owner ruling (2026-10-20, Clarify gate for #345): move `prepareDocker` before `freePort()` and retry the `start` phase
once on `"address already in use"` (2 attempts total), after removing the unstarted container.

## What Changes

- In `internal/harness/natsfixture/fixture.go`, move `prepareDocker(ctx, f.logf)` before the `start` phase's port
  allocation so the once-per-process Docker client setup and container sweep never run while a picked host port sits
  closed and unbound.
- When `testcontainers.GenericContainer(ctx, req)` fails in the `start` phase with `"address already in use"` and the
  caller's context is still live, remove the unstarted container via `rollbackAttempt(ctx)`, pick a fresh port via
  `freePort()`, and try `GenericContainer` once more (`startAttempts = 2`). Any other failure, a cancelled context, a
  failed rollback before retry, or a second `"address already in use"` failure returns from the `start` phase without
  further attempts.
- Add integration tests in `internal/harness/natsfixture/start_port_integration_test.go` covering recovery on the
  second attempt after a real Docker `"address already in use"` failure, exhaustion after two attempts, and no retry on
  a cancelled context, with zero leaked containers.
- Update `docs/test-infrastructure.md` and `scripts/lint-test-ports.sh` comments to reflect the one `start`-phase
  retry on `"address already in use"`.

## Capabilities

### Modified Capabilities

- `nats-fixture`: modify `Requirement: Phased start in one container` to state that `prepareDocker` runs before taking
  the host port and that the create-and-start phase retries once (up to two attempts total) when Docker fails with
  `"address already in use"` and the caller's context is live, removing the unstarted first container before the second
  attempt.

## Impact

- Code pull request (`Closes #345`), implemented by Gemini and reviewed by Claude or Codex under cross-agent review.
- `internal/harness/natsfixture/fixture.go`, `internal/harness/natsfixture/start_port_integration_test.go`,
  `docs/test-infrastructure.md`, `scripts/lint-test-ports.sh`, and `openspec/specs/nats-fixture/spec.md`.
