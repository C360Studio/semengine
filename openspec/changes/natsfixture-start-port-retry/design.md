# Design: natsfixture-start-port-retry

## Context and root-cause analysis (#180, #209, #254, #345)

1. **Why the host must pick the port (#180, #209):** On Docker Desktop for Mac, `dockerd` runs inside the Linux VM and
   assigns ephemeral host ports from a VM counter that cannot see macOS host listeners. When Docker picks a port held on
   macOS, `com.docker.backend` silently drops the port forwarding and leaves the container running with an empty port
   map (`ports: {"4222/tcp": []}`). Asking the macOS kernel for an ephemeral port (`freePort()`) and passing that
   explicit `HostPort` to Docker (#210) eliminated silent empty port maps.
2. **Why the TOCTOU window exists (#345):** Because Docker binds `0.0.0.0:<port>` in its own process during
   `ContainerStart`, `natsfixture` must close its port-0 listener before Docker starts the container. Unlike #180/#209,
   a collision on an explicit `HostPort` fails synchronously during `ContainerStart` with Docker's daemon error:
   `Error response from daemon: ports are not available: exposing port TCP 0.0.0.0:<port> -> 127.0.0.1:0: listen tcp
   0.0.0.0:<port>: bind: address already in use`.
3. **Why `fixture.go` widened the window:** In `fixture.go:208-216`, `prepareDocker(ctx, f.logf)` ran *after*
   `freePort()` closed the listener and *before* `testcontainers.GenericContainer(ctx, req)`, placing the first-call
   Docker client initialization, `ContainerList`, and 30-minute container sweep inside the unbound port window.

## Decisions

### D1. Run `prepareDocker` first and retry `start` once on port in use

In `Fixture.attempt` (`internal/harness/natsfixture/fixture.go`):

1. Call `prepareDocker(ctx, f.logf)` at the beginning of the `start` phase, before picking a host port.
2. Bound the `start` phase loop to `startAttempts = 2`:
   - Pick a host port via `pickPort()` (`freePort` in production; replaceable in-package by tests, matching `imagePin`
     in `fixture.go:46`).
   - When `f.maxPayloadSet` is true, assign `req.Files` a fresh `strings.NewReader` inside each attempt (`testcontainers`
     copies `Files` in a `PostCreates` hook before `ContainerStart`, draining the reader on attempt 1, #352).
   - Configure `req.HostConfigModifier` with `HostPort: hostPort` and call `testcontainers.GenericContainer(ctx, req)`.
   - Record any returned container on `f.container` and `f.containerID` immediately so ownership is never lost.
   - If `err == nil`, return `nil`.
   - Join `err` into `startErr`. If `!isAddressInUse(err)` (where `isAddressInUse` checks
     `strings.Contains(err.Error(), "address already in use")`; a cancelled caller context fails `GenericContainer`
     with `context canceled`, which does not match), return `startErr` without retrying; `run("start", ...)` then runs
     `f.rollbackAttempt(ctx)` to remove the failed container.
   - On attempt 1 (`attempt == 0`), when `isAddressInUse(err)` is true, run `f.rollbackAttempt(ctx)` before attempt 2
     to remove the unstarted first container under a fresh bounded rollback context and clear `f.container` /
     `f.containerID`. If that rollback fails, return `startErr` joined with the cleanup error rather than starting a
     second container while the first could not be removed.

All later phases (`host`, `mapped-port`, `connect`, `jetstream-ready`) and `Restart` remain single-attempt.

## Observable behaviour and proving tests

| Behaviour | Proving test (`internal/harness/natsfixture/start_port_integration_test.go`) |
| --- | --- |
| When the first host port picked by `Start` is held by another listener (causing Docker's real `"address already in use"` failure) and `ctx` is live, `Start` removes the first unstarted container, retries once with a fresh port and fresh config reader, and succeeds with only the second container present | `TestStartRetriesOnceOnPortInUse/recovers_on_second_attempt`: starts with `WithMaxPayload(4096)`, holds an ephemeral port via `net.Listen("tcp", ":0")`, returns it on call 1 of `pickPort` and delegates call 2 to `freePort()`; asserts `Start` returns `nil`, `pickPort` was called twice, JetStream answers on `URL()`, the broker announces `nc.MaxPayload() == 4096` and rejects a `4097`-byte publish with `nats.ErrMaxPayload`, and `processContainers(t)` lists only `f.containerID`. Fails 5/5 runs on `main`; passes 5/5 runs with the fix |
| When both attempts in the `start` phase hit `"address already in use"`, `Start` fails in phase `start` after 2 attempts with both causes joined and removes both unstarted containers | `TestStartRetriesOnceOnPortInUse/fails_after_two_attempts`: returns two distinct held ports on calls 1 and 2; asserts `Start` fails naming `phase start`, `address already in use`, and both ports, `pickPort` was called twice, `processContainers(t)` is empty, and `stopHoldsNothing(t, f)` is true |
| When `ctx` is cancelled or `GenericContainer` fails for a reason other than `"address already in use"`, `Start` does not retry and leaves no container behind | `TestStartRetriesOnceOnPortInUse/no_retry_when_context_cancelled` and `TestStartRetriesOnceOnPortInUse/no_retry_on_non_port_in_use_error`: assert `pickPort` was called once, `processContainers(t)` is empty, and `stopHoldsNothing(t, f)` is true |

## Why bounded examples suffice

The retry state space in `Fixture.attempt` is finite and small: at most two start attempts (`startAttempts = 2`), a
boolean error predicate (`isAddressInUse(err)`), caller context liveness (`ctx.Err() == nil` vs cancelled), and the
`WithMaxPayload` reader reset across retry (#352). The four subtests of `TestStartRetriesOnceOnPortInUse` exercise each
reachable transition against a real Docker daemon, so deterministic examples exhaust the state graph without property
generation.
