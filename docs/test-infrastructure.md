# Test infrastructure

How SemEngine runs tests that need a real NATS server, and the rules that keep those tests from disturbing each other.
Read it before you write a test that talks to NATS, or before you add code under `internal/harness`.

## Words used on this page

- **Container**: a process Docker runs from an image, here the NATS server image pinned in
  `internal/harness/natsfixture/nats-image`.
- **testcontainers**: the Go library (`testcontainers-go`) our test code calls to start and remove containers.
- **Ryuk**: a helper container testcontainers can start to remove a test process's containers after it dies. Ours is
  off; see "How cleanup works".
- **Host port**: the port on your machine that Docker forwards to the container's NATS port. The fixture asks the
  operating system for a free one and gives it to Docker.
- **Account**: NATS keeps every stream, bucket and connection in an account. Our servers have one account for tests,
  so "the account" means everything a test can create or open on that server.
- **Shared server**: one NATS container that all the integration tests of one Go package use, one test at a time.
- **Own server**: a NATS container that one test starts for itself, because it does something the others must not
  see, such as restarting the server.
- **Reset**: the call each test makes to get the shared server's address. It empties the server first and checks that
  it is empty.
- **Label**: a key and value Docker stores on a container. Ours is `org.c360studio.semengine.test`.
- **Sweep**: the step that removes labelled containers older than 30 minutes, judged by age alone (see
  [How cleanup works](#how-cleanup-works) for what that means for a long run).

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

A test that needs server permissions the fixture cannot set, such as denying one subject to one user, starts an
embedded server with `natsserver.NewServer` instead, as natsclient's embedded-server tests do
(`natsclient/client_lifecycle_test.go`), and writes the same `// Own server:` line above that call. It runs
the nats-server version the Go module `go.mod` pins, not the Docker image pin.

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

- `task test:integration` runs every package that has an `integration`-tagged test (`scripts/test-pkgs.sh integration`
  prints the list), each package in a random order it prints as `-test.shuffle <seed>`. Rerun one package's order with
  `go test -tags integration -shuffle=<seed> ./<package>`.
- `task test:integration -- ./natsclient` runs one package.
- Plain `go test -tags integration ./natsclient` works too, with the same behaviour: natsfixture turns Ryuk off
  itself.
- Two runs at the same time, from two worktrees, are fine. Nothing waits for another run.

## The whole-stack smoke test

`internal/smoke` is the one test that starts the graph components the way a product's own `main` would. One JSON
configuration starts graph-ingest, graph-index, graph-embedding and graph-query together, in a child process, through
the real startup path: `config.Manager`, then `ServiceManager`, then `ComponentManager`. The test then publishes a
fixed set of entities and checks the answers a consumer reads: the `graph.query.*` verbs (search among them), the
readiness keys in `GRAPH_STATUS`, component health over HTTP, and the embedding records.

Four tests restart something, and after each the stack must give the same answers and apply new writes:

1. stop the stack cleanly and start it again;
2. kill the stack while it is ingesting, then start it again;
3. stop the stack, restart the NATS server, start the stack;
4. restart the NATS server while the stack keeps running.

The NATS restarts are quick: the server is down for about 0.2 s. The tests do not prove the stack survives a longer
outage; that is issue #574.

Run it with `task test:integration -- ./internal/smoke/`. It takes about two minutes, most of it the stall after
each NATS restart while the components' watches reconnect.

## Sharing with other runs and other repositories

A run shares the Docker daemon with every other run and repository on the machine, and coordinates with none of them.
The fixture asks the operating system for a free host port for each container, so runs do not collide. Our code
removes only containers that carry our label, and never runs `docker … prune` or removes anything by name.

## Testing a consumer against a broker

`natsfixture` is internal to this repository. A consumer (SemSource, SemTeams, or any module that imports SemEngine)
cannot import it, and SemEngine does not ship a test fixture (owner ruling on #568). The client is the part that
belongs to the framework, and it connects to any broker URL in three calls:

```go
client, err := natsclient.NewClient(url,
    natsclient.WithTimeout(10*time.Second), // bound the connect
    natsclient.WithMaxReconnects(0),        // a test wants a failure, not a retry
    natsclient.WithHealthInterval(0),       // no background monitor to drain
)
if err != nil {
    return nil, err
}
if err := client.Connect(ctx); err != nil {
    return nil, errors.Join(err, client.Close(ctx))
}
t.Cleanup(func() {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := client.Close(ctx); err != nil {
        t.Errorf("close: %v", err)
    }
})
```

Where the broker comes from is the consumer's choice. Two ready options:

- a container through testcontainers-go, which ships a NATS module (`modules/nats`); pick the image this repository
  pins (`internal/harness/natsfixture/nats-image`) so the consumer tests the server version the graph components are
  proven against;
- an embedded `nats-server/v2` in the test process, for tests that do not want Docker.

Whichever the consumer picks, the shape above still holds: one server per package emptied between tests costs far less
than one container per test, and a client closed on `t.Cleanup` before the server stops keeps the run free of leaked
connections.

## How cleanup works

1. An own server is removed by the test's cleanup when the test ends.
2. A shared server is removed by `TestMain` after the package's tests.
3. A test process that is killed (Ctrl-C, a crash, or its `-timeout`) runs no cleanup, so its containers stay. The
   next test process to start a container removes labelled containers that Docker created more than 30 minutes ago.
   The sweep looks at age alone: a run under the default ten-minute `-timeout` has ended long before its containers
   are 30 minutes old, but a longer run (`-timeout 0` while debugging, say) gets no protection and can lose its
   containers to another test process's sweep. To see leftovers now:
   `docker ps -a --filter label=org.c360studio.semengine.test`.

## Known problem: Docker Desktop on macOS loses a host port

When Docker picks the host port and a Mac program already listens on it, Docker Desktop reports the container's port
as missing (#180, #209). The fixture no longer lets Docker pick: it asks the operating system for a free port and
gives Docker that exact port (#209, #210), so Docker's pick no longer applies. It asks with `tcp4`, a listener on every
IPv4 address, because the one Docker bind error seen named an IPv4 address (#345); Docker's choice of family is not
promised. An every-address `tcp` pick was once handed a port that a daemon held on `127.0.0.1` (#390). That a `tcp4`
pick skips such a port was measured on one Mac (PR #404 comment 6386556253, go1.26.10, macOS 26.5.2), not promised.
That Docker then publishes the port, and keeps it across a stop and start, is not promised by Docker: it rests on one
macOS probe (Docker Desktop 29.8.2, testcontainers v0.44.0, PR #210 comment 6151736554) and on every run since, and
`TestRestartURLDials` checks it. A port another program takes in the moment before Docker binds it fails the container
start with "address already in use", and `Start` removes that unstarted container, takes a new port, and tries once
more (#345). The `tcp4` pick does not close the gap between the fixture releasing the port and Docker binding it, so
the retry stays. Linux has not been measured. When a mapped port is missing, the error says what an inspect of the
container showed.

## When we write harness code

Harness code is code under `internal/harness` that tests use to run. Add it only for a failure that happens on
today's tree and that testcontainers, `go test`, `t.Cleanup` or Task does not already handle. Its doc comment names
that failure and its issue. Prefer, in order: a `go test` flag, `t.Cleanup`, a testcontainers feature, a Task
command, and only then code of our own, kept to the fewest lines.

## Retiring harness code

When the failure a piece of harness code names can no longer happen (the upstream bug is fixed, the test that needed
it is gone), remove the code in its own pull request and cite the evidence: a release note, a measurement, or the
removed test. Start's replacement container and Restart's extra stop and start for #180 went this way: once the
fixture picked the port, #210's probe (PR #210 comment 6151736554) showed a picked port coming up and a held port
failing at the start, where neither retry ran, and the owner ruled both out (#205 comment 6173171452).
