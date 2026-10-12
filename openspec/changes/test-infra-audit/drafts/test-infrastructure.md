# Test infrastructure

How SemEngine runs tests that need a real NATS server, and the rules that keep those tests from disturbing each other.
Read it before you write a test that talks to NATS, or before you add code under `internal/harness`.

## Words used on this page

- **Container**: a process Docker runs from an image, here the NATS server image pinned in
  `internal/harness/natsfixture/nats-image`.
- **testcontainers**: the Go library (`testcontainers-go`) our test code calls to start and remove containers.
- **Ryuk**: a helper container testcontainers can start to remove a test process's containers after it dies. Ours is
  off; see "How cleanup works".
- **Host port**: the port on your machine that Docker forwards to the container's NATS port. Docker picks it.
- **Account**: NATS keeps every stream, bucket and connection in an account. Our servers have one account for tests,
  so "the account" means everything a test can create or open on that server.
- **Shared server**: one NATS container that all the integration tests of one Go package use, one test at a time.
- **Own server**: a NATS container that one test starts for itself, because it does something the others must not
  see, such as restarting the server.
- **Reset**: the call each test makes to get the shared server's address. It empties the server first and checks that
  it is empty.
- **Label**: a key and value Docker stores on a container. Ours is `org.c360studio.semengine.test`.
- **Sweep**: the step that removes labelled containers older than 30 minutes, which only a killed run leaves behind.

## Unit test or integration test

A unit test has no build tag and starts no container. A test that needs a real NATS server goes in a file whose
first line is `//go:build integration` and uses `internal/harness/natsfixture`. Production runs NATS in Docker, so
these tests do too.

Some `natsclient` unit tests run nats-server inside the test process; ADR-112 accepts that for those tests. A new test
that needs NATS uses a container.

## Writing an integration test

Each package that has integration tests declares one shared server and stops it in `TestMain`. The server starts the
first time a test calls the reset, so a run that selects no such test starts nothing. Each test calls the reset once,
before it connects, and connects only to the address the reset returns.

```go
//go:build integration

package widget

import (
    "context"
    "fmt"
    "os"
    "testing"
    "time"

    "github.com/c360studio/semengine/internal/harness/natsfixture"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
    code := m.Run()
    ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
    defer cancel()
    if err := shared.Stop(ctx); err != nil {
        fmt.Fprintln(os.Stderr, err)
        return 1
    }
    return code
}

func TestWidgetStoresAFact(t *testing.T) {
    url := shared.Reset(t) // no stream, consumer, stored byte or other connection, or the test fails here
    client := natsfixture.Open(t, url, openClient) // closed when the test ends, before the connection check
    // ... use client; create the streams and buckets the code under test needs, with their real names
}
```

You may use the same stream and bucket names as production code, such as graph-ingest's `ENTITY` stream: the reset
deletes every stream before each test, and checks the account holds no stream, consumer or stored byte, so nothing a
previous test wrote is left.

## When a test gets its own server

Use `natsfixture.New(t)` instead of the reset only when the test:

- restarts or stops the server (`Restart`, `Stop`);
- needs a server setting the shared one does not have, such as `natsfixture.WithMaxPayload`;
- reads a running total the reset cannot clear, such as the account's count of API calls since the server started.

Write the reason on the line above the call: `// Own server: restarts the server mid-test.` A check fails without it.

## The rules, and what enforces each

| Rule | What enforces it |
| --- | --- |
| A test reaches the shared server only through the reset | Review: the server returns its address only from the reset, but a test could keep an address in a variable |
| A test starts on a server with no stream, consumer, stored byte or other connection | The reset fails the test otherwise |
| A test closes every connection it opens before it ends | The reset's cleanup fails the test; a test on its own server is review only |
| Integration tests in a package run one at a time: no `t.Parallel` | `TestNoParallelIntegrationTests`; a call in an untagged helper is review only |
| A test with its own server says why | `TestOwnServerReason` at each `natsfixture.New` call; a helper that calls it for many tests, and whether the reason holds, are review only |
| No sleeps, no fixed ports, no skipped tests, bounded cleanup contexts | `docs/testing.md` and their checks in `task verify` |

When a test leaves a connection open, that test fails, and so does every later test in its package, each saying a
connection from an earlier test is still open. Look for the first failure.

## Running the tests

- `task test:integration` runs every package, integration tests included, each package in a random order it prints
  as `-test.shuffle <seed>`. Rerun one package's order with
  `go test -tags integration -shuffle=<seed> ./<package>`.
- `task test:integration -- ./natsclient` runs one package.
- Plain `go test -tags integration ./natsclient` works too, with the same behaviour: natsfixture turns Ryuk off
  itself.
- Two runs at the same time, from two worktrees, are fine. Nothing waits for another run.

## Sharing with other runs and other repositories

A run shares the Docker daemon with every other run and repository on the machine, and coordinates with none of them.
Docker gives each container its own host port, so runs do not collide. Our code removes only containers that carry
our label, and never runs `docker … prune` or removes anything by name.

## How cleanup works

1. An own server is removed by the test's cleanup when the test ends.
2. A shared server is removed by `TestMain` after the package's tests.
3. A test process that is killed (Ctrl-C, a crash, or its `-timeout`) runs no cleanup, so its containers stay. The
   next test process to start a container removes labelled containers that Docker created more than 30 minutes ago.
   No test binary outlives its `-timeout` of 10 minutes, so a container that old is a leftover. To see leftovers now:
   `docker ps -a --filter label=org.c360studio.semengine.test`.

## Known problem: Docker Desktop on macOS loses a host port

When a Mac program already listens on the host port Docker picks, Docker Desktop reports the container's port as
missing (#180). The fixture handles that once: a start tries one replacement container and Restart tries one more
stop and start. Fewer containers mean fewer chances, which is one reason packages share a server. Whether Linux CI is
affected is not known.

## When we write harness code

Harness code is code under `internal/harness` that tests use to run. Add it only for a failure that happens on
today's tree and that testcontainers, `go test`, `t.Cleanup` or Task does not already handle. Its doc comment names
that failure and its issue. Prefer, in order: a `go test` flag, `t.Cleanup`, a testcontainers feature, a Task
command, and only then code of our own, kept to the fewest lines.

## Retiring harness code

When the failure a piece of harness code names can no longer happen (the upstream bug is fixed, the test that needed
it is gone), remove the code in its own pull request and cite the evidence: a release note, a measurement, or the
removed test. The replacement container for #180 is the next candidate, once a Docker Desktop release fixes the
defect and a recorded run shows it.
