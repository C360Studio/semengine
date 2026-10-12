# natsfixture-host-port

Status: revision 2, for the owner (design review passed; its findings are folded in). Issue #180 (`class:flake`);
claim PR #200. Rests on `inventory.md` (revision r2, `INVENTORY PASS`) and `design.md`. Two questions are open
(`design.md`, "Owner questions"); this proposal and the spec delta follow the recommended answers and change if the
owner rules otherwise.

## Why

natsfixture's Start and Restart intermittently fail with `port "4222/tcp" not found` after NATS reports ready (#180).
On Docker Desktop for Mac, Docker picks the published host port inside its VM, in sequence, without seeing which ports
Mac processes hold. When a Mac process already listens on that port, Docker Desktop cannot forward it, and its API
hides the binding (diagnosis, PR #200 comment 6128094108). While #180 is open, `merge-check` stops every pull request
that does not close it.

The owner ruled on #180:

- Comment 6128123143: the fixture picks the host port itself and asks Docker for exactly that port; the not-found
  error carries what the inspect showed; a Restart retry is rejected.
- Comment 6128409603: probe loopback-only publishing first.

The probe (PR #200 comment 6128508133) showed loopback publishing with Docker's own pick breaks against a listener on
`127.0.0.1`, so the fixture-picked port stands.

## What Changes

- **Start picks the port and publishes it on `127.0.0.1`** (open question Q1). The fixture binds `127.0.0.1:0`,
  closes the listener, and asks Docker to publish `4222/tcp` at exactly that address and port. `URL()` is
  `nats://127.0.0.1:<port>`.
- **A taken port is reported, not retried** (open question Q2). If the picked port is taken before Docker binds it,
  Start or Restart fails with an error naming the address and port. On Linux CI that can happen between two
  packages' fixtures, rarely (estimated, not measured); such a failure would be a known flake.
- **Restart keeps the port.** `URL()` is the same after a Restart. The text that said Docker may map a new port is
  corrected.
- **The not-found error carries the inspect.** The published port is read from one inspect, and a failure names that
  inspect's state, requested binding and published bindings.
- **A Docker host that is not loopback is refused** at the host phase: a daemon on another machine, or the gateway
  address testcontainers reports when the tests run inside a container. A loopback publish cannot be reached from
  either.
- `github.com/moby/moby/api` moves from an indirect to a direct requirement.
- The fixed-port guard's header names the fixture's pick, and the two tests that bind ports learned at run time,
  as deliberate uses. `docs/testing.md` and the admission ledger follow.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `nats-fixture`:
  - added: "Published on a loopback port the fixture picks" and "Restart keeps the container and its published
    port";
  - modified: "Phased start with typed failure";
  - removed: "Restart keeps the container and re-reads its binding". The renamed requirement replaces it with
    every scenario except "Binding changes across a restart". OpenSpec 1.14 refuses a modified requirement that
    drops a scenario.

## Impact

- Code: `internal/harness/natsfixture` (`fixture.go`, `deps.go`, `restart.go`, `errors.go` comments, and tests);
  the comment at `natsclient/integration_test.go:47-51`.
- Files: `go.mod`, `go.sum`, `scripts/lint-test-ports.sh` (header only), `docs/testing.md`,
  `docs/admission-ledger.yaml`.
- No production package changes. natsfixture is test-only, and every caller is a test in this repository.
- Open pull requests #93 and #188 merge after this one. Their shared files are listed in `design.md`, "Overlaps".
