# natsfixture-picked-port

Issue #209; claim PR #210. The design is #209's diagnosis (comment 6151740502), with its probe on PR #210 (comment
6151736554). Owner ruling: "Yes, build it now; Codex reviews the code" (#209 comment 6151770135).

## Why

Issue #209 is labelled `class:flake`, and the owner ruled it strict: "fix it before anything merges". When natsfixture
leaves the host port to Docker, Docker Desktop for Mac takes the next port of a counter of its own, which does not see
the Mac's listeners. When a Mac process listens on all addresses on that port, the container runs and NATS is ready,
but its port list is empty, so Start's port read fails. A process that holds several ports in a row catches Start's
replacement container too, and Start fails on both attempts. The probe reproduced this with nothing planted, and
showed that giving Docker a port the Mac picked avoids it.

## What Changes

- **Start:** the start phase asks the operating system for a free port (`net.Listen("tcp", ":0")`), reads it, closes
  the listener, and gives Docker that exact host port for `4222/tcp` through the request's `HostConfigModifier`. The
  URL stays `localhost`. A port another process took in between fails the start phase loudly; no retry is added.
- **Restart:** the container keeps that host port, so `URL()` no longer changes across a restart. Texts that said it
  may move are corrected.
- **Unchanged:** Start's one replacement container and Restart's one more stop and start; the test-infra audit (#205)
  decides them. Docker still publishes on all addresses.
- **Tests:** `TestStartPastHeldPorts` holds the ports after a running fixture's own and starts a second fixture; it
  fails on `main`'s code with #209's error. `TestRestartURLDials` asserts the URL is kept. `TestRestartPastAHeldPort`
  is removed: with the port kept, the port it held plays no part.

## Capabilities

### Modified Capabilities

- `nats-fixture`: "Phased start with typed failure" (modified) says the start phase publishes on a host port the
  fixture picked, with a scenario for held ports. "Restart keeps the container and re-reads its binding" is removed
  and re-added as "Restart keeps the container and its host port": the same text, one more sentence saying `URL()`
  is unchanged, and "Binding kept across a restart" in place of the scenario "Binding changes across a restart".
  OpenSpec's archive refuses a modified requirement that drops a scenario.

## Impact

- **Code pull request**, reviewed by Codex. The developer runs on Opus.
- `internal/harness/natsfixture` (`fixture.go`, `restart.go` comments, tests), `docs/testing.md`, a comment in
  `natsclient/integration_test.go`, and the header of `scripts/lint-test-ports.sh`.
- `go.mod`: `github.com/moby/moby/api v1.55.0` moves from indirect to direct; `go.sum` is unchanged.
- Measured on macOS with Docker Desktop only. Linux CI is not measured.
