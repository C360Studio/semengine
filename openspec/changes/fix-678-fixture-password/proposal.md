# Proposal: fix-678-fixture-password

Approved-by:

Track: `quick`. Capability modified: `nats-fixture` (one requirement added).

## Why

graph-query's `TestCountsAddUpAcrossPages` failed once when a graph-ingest from another test process joined its
shared NATS server (#678, diagnosis in comment 6941117780). That graph-ingest answered one seeding create from its
own payload registry ("not registered") and stayed attached past the test's end, so the connection check failed too.

The path: graph-ingest's restart tests stop their own container while a client redials its host port with no limit.
`natsfixture` takes host ports from the operating system (`freePort`, `internal/harness/natsfixture/fixture.go`), so
a broker another package starts while that container is down can be handed the same port, and the redialling client
joins it. No fixture broker asks a client who it is.

## What Changes

- `natsfixture` starts each broker with a random password (`nats-server --auth`, from `crypto/rand.Text`). `URL()`
  carries it in the user part (`nats://<password>@host:port`), which nats.go sends as the connection's token, so every
  caller that dials `URL()` connects unchanged. Restart reads the password back from the old URL; the container keeps
  its command line, so the broker keeps it.
- A client without the password is refused (`nats: authorization violation`), whatever port it dials.
- graph-ingest's relay test (`processor/graph-ingest/lifecycle_integration_test.go`), the one caller that split the
  URL by hand, keeps the password in the relay's URL.
- graph-query's test needs no change.

## Non-goals

Per-test passwords on the shared server (one broker per package process is the unit a foreign client can join);
changing how host ports are picked; capping reconnects in the restart tests.

## Impact

`internal/harness/natsfixture` (`fixture.go`, `restart.go`, one integration test); one graph-ingest test helper; the
`nats-fixture` spec. Test-only: no production code changes. A test that builds a NATS URL from the fixture's host and
port by hand, rather than from `URL()`, would now be refused; the relay test was the only one.
