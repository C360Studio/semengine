# Design: natsfixture publishes on a loopback port it picks

Status: revision 2, with design review r1's findings folded in. Issue #180 (`class:flake`); claim PR #200. Rests on
`inventory.md` (r2, `INVENTORY PASS`; "inventory § n"); why and what are in `proposal.md`.

Terms: to **publish** is to have Docker forward a host address and port to the container's port 4222; a **holder**
is another process listening on the same host port; to **pick** is to bind port 0, read the port the kernel chose,
and close the listener.

## Decisions

### D1. Pick, publish and dial on `127.0.0.1`

Start picks the port on `127.0.0.1`, asks Docker to publish `4222/tcp` at exactly that address and port, and names
`127.0.0.1` in `URL()`. What a caller observes:

- `URL()` is `nats://127.0.0.1:<port>`, and Docker reports the binding there (P8).
- When the pick succeeds, no other socket holds that address and port (P5).
- A holder on all addresses or on `::1` never receives a connection made to `URL()` (P4, P10).
- At the host phase, Start refuses a Docker host that testcontainers reports as anything but `localhost` or a
  loopback address: a daemon on another machine, or tests running inside a container, where testcontainers reports
  the gateway (`docker.go:1636-1648`).

Q1 compares this with publishing on all addresses. Tests: T2, T3, T8.

### D2. The gap between the pick and Docker's bind

After the fixture closes its listener and before Docker binds, another socket could take `127.0.0.1:<port>`.

- On macOS this needs the kernel's counter, which hands out ports in sequence across processes (P6), to come round
  16,384 ports, or a process that binds that port by number; both are rare.
- On Linux, by reading upstream and not measured, `bind(0)` starts at a random port. Under `-p 2` the other package's
  fixture can pick the same port during the gap: an estimated 1 in 7,000 for each overlapping pair of starts. Other
  port-0 binds in that package during the gap add to it.

A lost race fails Start at phase `start`, attempt 1, with a cause naming `127.0.0.1:<port>`. Docker refuses in 83 ms
and returns the container with the error (P9), so the existing rollback removes it. There is no replacement: that
stays limited to the mapped-port phase (`fixture.go:157`). On CI such a failure is a red run on an unchanged tree,
which `.agents/protocol.md:185-191` treats as a known flake. Q2 asks whether to accept that.

The guard's advice "Do not close it to reuse its port" (`scripts/lint-test-ports.sh:55-56`) stays, because it is
right for code that can take a listener and Docker cannot. Its header names the deliberate uses (D6). Test: T4.

### D3. Restart keeps the port

Docker keeps the requested binding across stop and start (P8), so the restarted broker listens on the same
`127.0.0.1:<port>`. What a caller observes:

- After Restart returns nil, `URL()` is unchanged and `JetStream()` is new. Callers still dial again, because their
  connections broke when the broker stopped.
- A holder on all addresses that arrives while the container is stopped does not fail Restart (P10, P4).
- An exact `127.0.0.1` holder does: Restart returns `*Error` at phase `start-container` naming the address (P11),
  creates no replacement, refuses a further Restart, and Stop removes the container.

Restart stops re-deriving the host from the old URL (`restart.go:100-104`); task 5.1 lists the text that assumed a
new port. The spec removes "Restart keeps the container and re-reads its binding" and adds "... and its published
port" without "Binding changes across a restart" (OpenSpec 1.14 refuses a modification that drops one). Tests: T5, T6.

### D4. The not-found error carries the inspect

The default `deps.mappedPort` reads the port, the first published binding of `4222/tcp`, from one `Inspect`. On
failure it describes that same snapshot; testcontainers' `MappedPort` discards its own (inventory § 5). The signature
does not change, so the S1 tests that fake it stay as they are. Example:
`... phase mapped-port container 086975624ccb (parent context live): 4222/tcp not published on 127.0.0.1: state
running (running true); requested [127.0.0.1:55016]; published []`. Test: T1.

### D5. `github.com/moby/moby/api` becomes a direct requirement

Only `HostConfigModifier` sets a host address or port in testcontainers-go v0.44.0, and it names moby types
(inventory § 2). The module is already in the build at v1.55.0; one `go.mod` line moves from indirect to direct, and
`ledger.diff` records it. `deps.go:158-161` avoided the module only where an error's text was enough.

### D6. Evidence, and the tests that bind ports

T7 reproduces issue #180 inside the tree:

1. A second `Fixture`, whose `deps.start` drops the binding from the request, publishes the old way. Admission, name,
   evidence and Stop stay the fixture's. T7 reads that fixture's port P.
2. T7 holds P+1 to P+5 on all addresses; a port already in use counts as held, the condition being planted.
3. T7 starts and restarts a fixture. On `main` both Start attempts are expected to land on held ports and fail;
   after the fix the fixture no longer uses that sequence.

Tasks 2.1 and 3.5 run T7 five times each. Cost: each local `task verify` holds Docker Desktop's next ports for
about a second, and a Docker client outside the lock that publishes in that second gets #180's vanishing port. On
Linux, T7's five holds are one more competitor in D2's gap (by reading, not measured).

T6 and T7 bind ports learned at run time, spelled with `net.JoinHostPort`, which the guard's patterns
(`lint-test-ports.sh:38`, `:43`) do not match. The guard's header names both, beside the fixture's pick.

## Tests

| Test | Proves | Where |
| --- | --- | --- |
| T1 `TestPublishedPortReadsOneInspect` | One inspect gives port and description: published, empty list, stopped | unit |
| T2 `TestFixtureRoundTrip` (extended) | `URL()`'s host is `127.0.0.1`; `docker port` reports `127.0.0.1:<port>` | Docker |
| T3 `TestHolderOnAllAddressesNeverGetsTheConnection` | `pickPort` forced to a port held on all addresses. darwin: Start succeeds and every `127.0.0.1` dial reaches NATS (P10). linux: Start fails at `start` naming the address (expected; CI measures it) | Docker |
| T4 `TestPickedPortTakenBeforeDockerBinds` | Port held on `127.0.0.1`: phase `start`, one attempt, address named, no container, Stop nil | Docker |
| T5 `TestRestartURLDials` (changed) | After Restart, `URL()` is unchanged, equals `docker port`, and dials | Docker |
| T6 `TestRestartWithItsPortTaken` | Port bound while stopped (`probe.Await`): `start-container`, address named, no second container | Docker |
| T7 `TestNextDockerPortsHeld` | #180's reproduction (D6) | Docker |
| T8 `TestLocalDockerHost` | Loopback hosts are accepted; another host, a gateway address and an empty host are refused | unit |

## Premises

`probes/` holds the programs and outputs task 1.2 posts on PR #200; probe h6 is PR #200 comment 6128508133.

| # | Premise | Measurement |
| --- | --- | --- |
| P1 | With Docker's own pick, a holder on all addresses makes the port vanish | `probes/h5-*`; h6, 2 of 2 twice |
| P2 | With Docker's own pick, a loopback publish fails the same way against a `127.0.0.1` holder | h6, 2 of 2 |
| P4 | On macOS, an exact `127.0.0.1` listener wins `127.0.0.1` dials in either bind order | `probes/p4-*.go` |
| P5 | On macOS, an exact bind on `127.0.0.1:<port>` is refused while a listener holds it | `probes/p5-p6-*.go` |
| P6 | macOS hands out bind(0) ports in sequence, across processes | `probes/p5-p6-*.go`, `probes/p6-across-processes.go`; `net.inet.tcp.randomize_ports: 0` |
| P7 | Docker Desktop refuses an all-address publish of an explicit port held on all addresses, at start | `probes/h5b-*`, 73 ms |
| P8 | Docker Desktop publishes an explicit `127.0.0.1:<port>`, and Restart keeps it | Review probe S1, 418 ms (`probes/rv200dock-*`) |
| P9 | Held on `127.0.0.1` before start: start fails, container returned | S2, 83 ms |
| P10 | Held on all addresses: Start and Restart succeed; `127.0.0.1` dials reach NATS | S3 |
| P11 | Taken on `127.0.0.1` while stopped: the restart's start fails | S4, 4 ms |

Each Docker Desktop case ran once; tasks 3.4 and 4.1 measure P9 and P11 again in the tree. Nothing is measured on
Linux CI; the run before merge is the first measurement. P10 supersedes h6's P3.

## What stays, and overlaps

What stays: Start's one mapped-port replacement (the pick runs on each attempt); the phases, the exported surface
and the evidence schema (`deps` gains `pickPort`); Ryuk, which publishes upstream's way (`reaper.go:393-405`).

Overlaps: issue #180 stops other pull requests at `merge-check`, so #200 merges first. #93 shares the natsfixture
directory, `go.mod`, `docs/testing.md` and the ledger, with no hunk on the same lines (inventory § 6); new tests go
before the end of `fixture_integration_test.go`, where #93 appends. #188 shares only the ledger.

## Owner questions

**Q1. Publish only on `127.0.0.1`, and name `127.0.0.1` in the URL?** Recommended: yes.

- Why: when the port is picked, nothing else holds it on that address (P5). Holders on other addresses cannot take
  the connection (P10). The URL leaves no choice between `::1` and `127.0.0.1`.
- Cost of the other answer (all addresses, URL `localhost`): this repository's tests open thirteen `127.0.0.1`
  listeners, and one can sit unseen on the picked port. nats.go picks `127.0.0.1` or `::1` at random, so a test could
  quietly talk to another server.
- Your cost: the test broker is unreachable from other machines and from tests running inside a container. No caller
  in the tree does either (inventory § 4).

**Q2. When another process takes the picked port before Docker binds it, should Start fail and name the port, with no
retry?** Recommended: yes, until a measurement says otherwise.

- Why: this keeps the fixture's rule that only the mapped-port failure is replaced (`fixture.go:28-30`). On macOS the
  race needs the kernel's counter to come round to this port (P6). Restart cannot get a new port either way.
- Cost of the other answer: a retry has to recognise that Docker refused because the port was taken, either from
  Docker's message text or by binding the port again to see that it is held (about ten lines). Either way the
  replacement rule widens to a second phase.
- Your cost: on Linux CI, two parallel packages' fixtures can pick the same port, about 1 in 7,000 per overlapping
  pair, plus any other port-0 bind in the gap (from reading the kernel, not measured). A lost race there is a red
  run on an unchanged tree. That is a known flake that stops every merge until it is fixed, most likely by the retry
  above. You also accept the cost the 04A design gave for withdrawing a stable port
  (`setup-04a-foundation/design.md:271-277`).
