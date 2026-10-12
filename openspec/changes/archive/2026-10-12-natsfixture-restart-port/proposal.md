# Proposal: natsfixture-restart-port

## Why

Issue #180 is labelled `class:flake`, so every merge waits on it. Its cause is measured (PR #200, comment
6128094108). On macOS, Docker Desktop's engine hands out host ports in sequence and cannot see which ports Mac
processes hold. When a Mac process listens on all addresses on the port Docker picks, Docker Desktop cannot forward
it, and the port lookup comes back empty: `port "4222/tcp" not found`. The next container start gets the next port,
which is why Start's one replacement attempt already recovers. Restart has no second try, so it fails; both
`TestRestartURLDials` failures were this.

## What changes

- When the mapped-port phase fails after a restart and the caller's context is still live, Restart stops and starts
  the same container once more and reads the port again. It keeps the container and its data; it is not a
  replacement container.
- The mapped-port failure's error carries what an inspect of the container showed (state and published ports).

Owner ruling: #180 comment 6129968019. It supersedes the fixture-picked port design (comments 6128123143 and
6128409603), which is withdrawn.

## Impact

`internal/harness/natsfixture` (`restart.go`, the mapped-port error), its tests, the `nats-fixture` spec's Restart
requirement, and doc text that says Restart never tries again. No new dependency. Linux CI is not measured; on Linux
dockerd binds the host port itself, so the condition is not expected there, and the change does nothing when the
first read succeeds.
