# Design: natsfixture-restart-port

## The cause, measured

From PR #200: the diagnosis (comment 6128094108) and probes (comments 6128508133, 6129261890, 6129263070).

- With `HostPort "0"`, Docker Desktop's engine hands out host ports in sequence (58444, 58445, …). If a Mac process
  listens on all addresses on that port, Docker Desktop cannot bind it, and its API answers every inspect after about
  3 s with an empty port list. testcontainers' `MappedPort` makes one inspect and returns `port "4222/tcp" not
  found`.
- A restart of the same container gets a new port the same way (58448 at Start, 58449 at Restart in the
  reproduction; 55016 then 55017 in h6). So one more stop and start skips a held port.
- 16 integration runs on the owner's Mac had a lost lookup; Start's replacement recovered the ones at Start. The
  failures that reached a test were at Restart, which has no second try.

## The change

1. **Restart tries once more.** In `restart.go`, when the mapped-port phase fails and the caller's context is still
   live, Restart runs the stop-container and start-container phases once more on the same container, then the
   mapped-port phase again. A second failure is returned as today. This is the same condition Start uses for its one
   replacement (`fixture.go:28-31`, `:148-161`): which phase failed and whether the parent is live, never the error's
   text. The container, its writable layer and the fixture's ownership record are kept, so the spec's "no
   replacement container during Restart" still holds.
2. **The error says what Docker showed.** When the mapped-port phase fails (Start or Restart), the fixture inspects the
   container once more under a bounded context and adds its state and published ports to the error's detail. If that
   inspect fails, the detail says so.

Nothing else changes: the request, the URL's host, Start's attempts and the port guard stay as they are.

## What a caller observes, and the test that proves it

| Behaviour | Test |
| --- | --- |
| A mapped-port failure after a restart, with a live context, is followed by one more stop and start, and Restart succeeds when the second read does | unit test with the fixture's fakes: the first read after restart fails, the second succeeds; Restart returns nil and the stop and start counts are 2 each. Fails on `main` |
| A second mapped-port failure is returned as a `FixtureError` in the mapped-port phase, with no third try | unit test with fakes: both reads fail; counts 2 each |
| A cancelled context gets no second try | unit test with fakes: cancel before the read fails; counts 1 each |
| A held next port does not fail Restart on Docker Desktop | integration test: Start, read the port P, hold P+1 on all addresses, Restart, dial `URL()`. On `main` on macOS it fails with the mapped-port error; after the fix it passes. On Linux it passes either way |
| The mapped-port error names the container's state and published ports | unit test with a fake inspect |

The integration test binds the held port with `net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(p+1)))`, from a
port read at run time, not a literal.

## Flake evidence

The pull request records the integration test's command and its result on `main` and on the fix, five runs each,
on the owner's Mac under the host lock. If another container takes P+1 first, that run does not exercise the path;
the record says how many runs did.

## Not measured

Linux CI. Whether `rapportd` or another Mac process ever holds two ports in a row.
