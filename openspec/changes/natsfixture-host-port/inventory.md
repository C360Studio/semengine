# PR #200 / #180: inventory for the host-port fix

Two candidates: loopback-only publishing (probed) and the fixture-picked host port.

This copy is revision r2 as it passed inventory review (sha256
`93c06fbc0a944f41a94bf9bcbe80f479bd0dec6cf199a25adede51223a247600`),
reflowed to the repository's 120-column Markdown rule, with review r2's two wording notes folded in
(the runner's exports; `TestFixtureRoundTrip`).

Revision r2. r1 is kept at `/tmp/pr200/inventory.r1.md` (local only; sha256
`c4c5983413012f6d029d29791aab3464a413f9b5ef7990d70f6d4f8f1bd4e05b`).
r2 answers review r1 (`/tmp/pr200/inventory-review.r1.md`, local only: F1 widened, F2, F3, F4) and adds the owner's revised
ruling and the loopback probe.

base: `90f79035355ba8a94912530688611955aadfb8cf` (claim commit, empty) on `main`
`841bf4db9c0d932ef99d3a50fb0c74543a646ba3`;
worktree `/Users/coby/Code/c360/semengine-wt/claude/natsfixture-mapped-port`, `git status --porcelain` empty.
Library sources read from the module cache: testcontainers-go `v0.44.0`, `github.com/moby/moby/api` `v1.55.0`
(`go.mod:17`, `go.mod:61`), nats.go `v1.54.0`, nats-server `v2.15.0`. Pin reads: SemStreams `8b99efe9` through
`gh api .../contents/<path>?ref=8b99efe9` only. Open PRs read at: #93 `6670ffcacab4459075963cfbc4ef64e5b8312963`,
PR #188 `b3bba218a1652e3f6367568b187004228edad80e`, #194 `47e194022c51d59aa27fc01eda0c32ede90968c2`.

Inventory only (architect contract, Required workflow step 2): no options, no recommendation, no tasks.

## 0. The artifact and the problem, as given

- #180 is labelled `class:flake` (owner ruling, #180 comment 6127718979). While it is open, `merge-check` stops every
  pull request that does not close it (`.agents/protocol.md:189-191`). PR #200's body declares `Closes #180`
  (`closingIssuesReferences`: `[180]`).
- Diagnosis (PR #200 comment 6128094108): on Docker Desktop for Mac (Engine 29.8.2), dockerd in the VM picks the
  published host port for `HostPort: "0"` in sequence, without knowing which ports macOS processes hold. When a Mac
  process holds that port, Docker Desktop cannot listen on it. Its API proxy then waits about 3 s on every inspect
  and answers `NetworkSettings.Ports: {"4222/tcp": []}`, while dockerd itself reports the port. testcontainers'
  `MappedPort` then returns `port "4222/tcp" not found`.
- Owner ruling, #180 comment 6128123143, verbatim: "the fix direction is that the fixture picks the host port itself
  (asks the host OS for a free port, releases it, and asks Docker for exactly that port), instead of Docker's blind
  sequential pick inside the Docker Desktop VM. Whatever the fix, the fixture's not-found error also carries what the
  inspect showed. Rejected: giving Restart the replacement attempt Start has (a retry around Docker Desktop's
  behaviour)."
- Owner ruling, #180 comment 6128409603, revising it: "**probe first, then decide.** The probe, time-boxed to about 20
  minutes, asks Docker to publish 4222 on loopback only (host address `127.0.0.1`, host port still Docker's pick) and
  checks whether that avoids the clash with a host listener on all addresses. If it does, that is the fix; if not,
  the fixture-picked port of comment 6128123143 stands." The not-found-error requirement and the rejected Restart
  retry from 6128123143 are not withdrawn by it.
- Loopback probe result, PR #200 comment 6128508133 (`semengine-developer`; source, command, versions and output
  posted in the comment; program `/tmp/pr200/h6/main.go`, local copy). "Loopback publishing" below means
  `PortBindings {"4222/tcp": [{HostIp: "127.0.0.1", HostPort: "0"}]}`. Run 2, each row 1 Start + 1 Restart:

  | binding / Mac holder on the next port | Start, Restart | `127.0.0.1:P` answers | `localhost:P` (Go dial) answers |
  | --- | --- | --- | --- |
  | today (`HostIp ""`) / none | ok, ok | NATS | `[::1]`, NATS |
  | loopback / none | ok, ok (4 pairs over runs 1-2, plus 1 after) | NATS | `127.0.0.1`, NATS |
  | loopback / all addresses `:P` | ok, ok (`HostIp 127.0.0.1` reported in 2 ms) | NATS | `[::1]`, **holder** |
  | today / all addresses `:P` (control) | `not found` after ~6.1 s + ~3.0 s, both | holder | `[::1]`, holder |
  | loopback / loopback `127.0.0.1:P` | `not found` after ~6.1 s + ~3.0 s, both | holder | `127.0.0.1`, holder |

  Also from the probe: dockerd keeps one port sequence per binding address (all-addresses bindings ran at 59174...,
  loopback bindings at 55004..., run 1); nats.go's `nats.Connect("nats://localhost:P")` reached the container at
  `127.0.0.1` in 4 of 4 tries in the loopback / all-addresses row, which the probe says is not shown to be guaranteed.
- How the coordinator reads the probe (relayed to me, not an owner ruling): loopback publishing is not the fix, so the
  fixture-picked port of 6128123143 stands. The owner has been told and may rule otherwise. This inventory covers
  what both bindings touch, because either may be designed.

## 1. Code paths that bind, read or build the port and URL

Files and sizes: `fixture.go` 670, `restart.go` 193, `deps.go` 167, `errors.go` 92, `evidence.go` 108,
`rollback.go` 25, `stop.go` 295 lines. No `README.md` in the package (`ls`).

### Start and its replacement attempt

- `internal/harness/natsfixture/fixture.go:28` — `// maxAttempts allows one replacement container, and only after a
  mapped-port failure with a`
- `internal/harness/natsfixture/fixture.go:29` — `// live parent and a clean rollback: the one transient SemStreams
  observed under Docker API`
- `internal/harness/natsfixture/fixture.go:30` — `// pressure (gh#736). Every other failure is reported, not retried.`
- `internal/harness/natsfixture/fixture.go:31` — `maxAttempts = 2`
- `internal/harness/natsfixture/fixture.go:149` — `for attempt := 1; attempt <= maxAttempts; attempt++ {`
- `internal/harness/natsfixture/fixture.go:157` — `if !errors.As(err, &fe) || fe.Phase != PhaseMappedPort ||
  fe.ParentErr != nil || fe.Cleanup != nil {`
  The only failure Start replaces is `PhaseMappedPort`. A failure at `PhaseStart` ends Start with no replacement. A
  host port Docker refuses to bind fails at `PhaseStart` (h5b, section 2).
- `internal/harness/natsfixture/fixture.go:272` — `cmd := []string{"--port", "4222", "--js"}` (container port, first
  spelling)
- `internal/harness/natsfixture/fixture.go:285` — `req := testcontainers.GenericContainerRequest{` (built inside
  `attempt`, so once per attempt)
- `internal/harness/natsfixture/fixture.go:289` — `ExposedPorts: []string{clientPort},`
- `internal/harness/natsfixture/fixture.go:292` — `WaitingFor:   wait.ForLog("Server is ready"),`
- `internal/harness/natsfixture/fixture.go:299` — `c, err = f.deps.start(ctx, req)`
- `internal/harness/natsfixture/fixture.go:301` — `// GenericContainer can return a live container together with its
  error (generic.go:89,94);`
- `internal/harness/natsfixture/fixture.go:316` — `host, err = f.deps.host(ctx, c)`
- `internal/harness/natsfixture/fixture.go:322` — `if err := run(PhaseMappedPort, func() error {`
- `internal/harness/natsfixture/fixture.go:324` — `port, err = f.deps.mappedPort(ctx, c)`
- `internal/harness/natsfixture/fixture.go:330` — `url := fmt.Sprintf("nats://%s:%s", host, port)`
- `internal/harness/natsfixture/fixture.go:365` — `f.url = url`
- `internal/harness/natsfixture/fixture.go:366` — `f.rec.Container, f.rec.MappedPort = f.containerID, port`

### The dependency seam (every Docker or NATS call, counted)

- `internal/harness/natsfixture/deps.go:19` — `clientPort = "4222/tcp"` (container port, second spelling)
- `internal/harness/natsfixture/deps.go:27` — `// deps is every call the fixture makes to Docker or NATS. Tests replace
  entries to fail or block`
- `internal/harness/natsfixture/deps.go:28` — `// a phase; every call is counted, so a test can prove a path made none.`
- `internal/harness/natsfixture/deps.go:51` — `host: func(ctx context.Context, c testcontainers.Container) (string,
  error) { return c.Host(ctx) },`
- `internal/harness/natsfixture/deps.go:53` — `p, err := c.MappedPort(ctx, clientPort)`
- `internal/harness/natsfixture/deps.go:54` — `return p.Port(), err`
- `internal/harness/natsfixture/deps.go:96` — `raw, err := d.DialContext(ctx, "tcp", u.Host)` (the fixture's own dial;
  section 1A)
  By `deps.go:27-28`, any new Docker call (an inspect, or a port pick if it counts as a dependency) is a new `deps`
  entry with a count. Tests that compare call counts: `fixture_integration_test.go:228` (start and terminate counts
  per attempt), `:231` and `:587` (`maps(calls, f.callCounts())`).

### Rollback after a failed attempt

- `internal/harness/natsfixture/fixture.go:238` — `fe := &Error{Attempt: n, Phase: p, ContainerID: id, ParentErr:
  ctx.Err(), Cause: cause}`
- `internal/harness/natsfixture/fixture.go:239` — `ar.Logs, fe.Cleanup = f.rollbackAttempt(ctx, n)`
- `internal/harness/natsfixture/rollback.go:22` — `ctx, cancel := context.WithTimeout(context.WithoutCancel(parent),
  rollbackBudget)`
  Rollback captures logs, terminates, observes absence (`fixture.go:375-415`). It touches no port or URL.

### Restart

- `internal/harness/natsfixture/restart.go:28` — `// URL and JetStream return the new binding once Restart returns nil,
  and stay valid until the next`
- `internal/harness/natsfixture/restart.go:29` — `// Restart; Docker may map a different host port. A caller stops what
  it built on the old URL before`
- `internal/harness/natsfixture/restart.go:30` — `// Restart and starts it on the new one after.`
- `internal/harness/natsfixture/restart.go:33` — `// failure in a phase returns an *Error naming it; no replacement
  container is created, the fixture`
- `internal/harness/natsfixture/restart.go:97` — `f.nc, f.js, f.url = nil, nil, ""`
- `internal/harness/natsfixture/restart.go:100` — `u, err := neturl.Parse(oldURL)`
- `internal/harness/natsfixture/restart.go:104` — `host := u.Hostname()` (host re-derived from the old URL; Start reads
  it from `deps.host`)
- `internal/harness/natsfixture/restart.go:114` — `return &Error{Attempt: 1, Phase: p, ContainerID: id, ParentErr:
  ctx.Err(), Cause: err}`
- `internal/harness/natsfixture/restart.go:147` — `port, err = f.deps.mappedPort(ctx, c)`
- `internal/harness/natsfixture/restart.go:153` — `url := fmt.Sprintf("nats://%s:%s", host, port)`
- `internal/harness/natsfixture/restart.go:178` — `f.js, f.url, f.rec.MappedPort = js, url, port`
  Restart stops and starts the same container (`deps.go:70-71`, `c.Stop(ctx, nil)` / `c.Start(ctx)`). It never
  rebuilds a request, so it cannot ask Docker for a different binding. With `HostPort "0"`, Docker hands out a new
  port on each start (probe h6: 59179 → 59180; 55014 → 55015). Restart writes no attempt or phase record to evidence
  (`restart.go:60` writes the record; `evidence.go:15-27` has no restart field).

### Stop

- `internal/harness/natsfixture/stop.go:121` — `c, id := f.container, f.containerID`, then terminate and observe absent
  (`stop.go:123-135`). Stop reads no port and does not clear `f.url` (`grep -n 'f\.url' stop.go`: no hit).

### Every promise about ports, hosts and URLs in this package's doc comments

- `internal/harness/natsfixture/fixture.go:417` — `// URL is the broker's client URL on a Docker-assigned host port.`
- `internal/harness/natsfixture/restart.go:29` — `// Restart; Docker may map a different host port. ...` (above)
- `internal/harness/natsfixture/fixture.go:28-30` — the gh#736 attribution of the replacement (above)
- `internal/harness/natsfixture/errors.go:21` — `// Restart runs PhaseStopContainer and PhaseStartContainer, then
  PhaseMappedPort, PhaseConnect and`
- `internal/harness/natsfixture/errors.go:26` — `PhaseHost           Phase = "host"`
- `internal/harness/natsfixture/errors.go:27` — `PhaseMappedPort     Phase = "mapped-port"`

The same promises outside the package:

- `openspec/specs/nats-fixture/spec.md:59` — `Start SHALL proceed image → create/start → internal ready log → host →
  mapped client port → connect → JetStream ready,`
- `openspec/specs/nats-fixture/spec.md:64` — `directory. At most one replacement attempt is permitted, only for the
  mapped-port phase with a live parent and`
- `openspec/specs/nats-fixture/spec.md:89` — `- **THEN** their container ids, host ports, and Name values are disjoint`
- `openspec/specs/nats-fixture/spec.md:137` — `### Requirement: Restart keeps the container and re-reads its binding`
- `openspec/specs/nats-fixture/spec.md:146` — `` phase failure SHALL return a `FixtureError` carrying the phase; the
  fixture SHALL NOT create a replacement ``
- `openspec/specs/nats-fixture/spec.md:159` — `- **WHEN** Docker maps the container's client port to a different host
  port after the restart`
- `openspec/specs/nats-fixture/spec.md:160` — `` - **THEN** `URL()` returns the new address and a dial to it succeeds ``
- `docs/testing.md:80` — `` memory-backed one does not. Docker may map a new host port, so `URL()` and `JetStream()` are
  valid only until the ``
- `natsclient/integration_test.go:48` — `// Docker may map the restarted broker to another host port, so the first
  client's URL can point`
- `natsclient/integration_test.go:49` — `// nowhere; that is why the pin skipped this test (integration_test.go:63-65)
  and why the first`
- Spelling drift seen in passing: the spec names the error type `FixtureError` (`spec.md:146`, `:176`); the code's type
  is `Error` (`errors.go:39`). `git grep -n FixtureError` hits only spec and archive text.

## 1A. The URL's host, and where `localhost` goes (both bindings)

Where the host comes from:

- `internal/harness/natsfixture/deps.go:51` → `c.Host(ctx)` → `testcontainers-go@v0.44.0/docker.go:1607` `DaemonHost`
  → `daemonHostLocked` (`docker.go:1614-1657`): `TESTCONTAINERS_HOST_OVERRIDE` if set (`:1619-1623`); the daemon's host
  name for an `http`, `https` or `tcp` daemon (`:1633-1634`); for a `unix` or `npipe` socket, the literal
  `"localhost"` (`:1650`), or the bridge gateway IP when running inside a container (`:1636-1648`). On this Mac the
  socket is `unix:///var/run/docker.sock` (h5b log: "Resolved Docker Host"), so the host is `localhost`. Restart reuses
  it from the old URL (`restart.go:100-104`). The value is cached per provider (`:1615-1616`).
- `scripts/doctor.sh:78` — `for v in TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE; do`:
  doctor checks neither `TESTCONTAINERS_HOST_OVERRIDE` nor `DOCKER_HOST`'s effect on the URL host (it warns on
  `DOCKER_HOST` for the daemon choice, `:73`). The only `TESTCONTAINERS_*` variable the runner exports is
  `TESTCONTAINERS_RYUK_DISABLED`
  (`scripts/test-integration.sh:362`); its other exports are `SEMENGINE_*` variables (`:363-366`).

How `localhost` resolves and is dialled here (measured on this Mac, Go 1.26.4 in a scratch module, no Docker; the
reviewer's probes `/tmp/rv200probe*` got the same):

- `/etc/hosts` maps `localhost` to `127.0.0.1` and `::1`. `net.LookupHost("localhost")` returns `[::1 127.0.0.1]`.
- `net.Listen("tcp", ":0")` and `net.Listen("tcp", "0.0.0.0:0")` both give `[::]:P`, which accepts a dial to `[::1]:P`.
- A second listener on `127.0.0.1:P` binds beside that wildcard one without error. Then `127.0.0.1:P` reaches the
  loopback listener, and `localhost:P` and `[::1]:P` reach the wildcard one.
- The fixture's own connection: `deps.go:95-96`, `net.Dialer.DialContext(ctx, "tcp", u.Host)`. For `localhost` Go
  tries `::1` first and falls back to `127.0.0.1`. That is the path the probe's raw `localhost` dial took: in the
  loopback / all-addresses row it reached the holder (2 of 2). The fixture's `connect` itself was not run against
  a holder.
- Every other client of `f.URL()`: nats.go resolves the host and shuffles the addresses before dialling
  (`nats.go@v1.54.0/nats.go:2485` `addrs, _ := net.LookupHost(u.Hostname())`; `:2509-2512` `rand.Shuffle` unless
  `NoRandomize`; dial loop `:2514-2515`). natsclient and the `nats.Connect(f.URL())` tests take this path
  (`fixture_integration_test.go:850`, `:1106`, `:1139`; 61 natsclient references). Which address goes first is random.
- Today, with `HostIp ""`, Docker Desktop publishes on both `0.0.0.0` and `::` (probe h6, proxy inspect:
  `[{"HostIp":"0.0.0.0","HostPort":"59179"},{"HostIp":"::","HostPort":"59179"}]`; raw dockerd lists `0.0.0.0` only).
  Every `localhost` dial reaches the container when no one else holds the port (probe h6, first row).

What each binding leaves to a host listener on the same port:

- Loopback publishing (`HostIp 127.0.0.1`): `[::1]:P` is free for anyone holding `[::]:P`. The URL `nats://localhost:P`
  then reaches the holder on the `::1` path (probe h6). A Mac listener on `127.0.0.1:P` breaks the binding exactly as
  #180 does (probe h6, 2 of 2).
- Today's binding (`HostIp ""`): a wildcard holder breaks it (#180). A loopback holder did not stop `MappedPort` (h5,
  "loopback listen 127.0.0.1:P ... ok"). In that case, who answers a dial to `127.0.0.1:P` was not measured; the
  scratch probe above shows that, between two Go listeners, the loopback one wins `127.0.0.1:P`.
- A fixture-picked host port (`HostPort Q`, `HostIp ""` unless set): Docker refused to start when a Mac process held
  `*:Q` (h5b: `ports are not available: exposing port TCP 0.0.0.0:64887 ... bind: address already in use`). With a
  holder on `127.0.0.1:Q` or `[::1]:Q`: not measured. Which address the fixture's pick binds decides which holders
  the kernel can see. One data point: a `127.0.0.1:0` pick beside a `[::]:P` wildcard listener returned a different
  port (one sample; proves no rule).

In-tree listeners that can be holders while integration runs are on the host (the integration lock admits one
Docker run per host, not one unit run):

- All addresses: `natsclient/client_connect_test.go:114-118` (`natsserver.Options` with no `Host`;
  `nats-server@v2.15.0/server/const.go:86` — `DEFAULT_HOST = "0.0.0.0"`, which Go makes `[::]`);
  `metric/lifecycle_test.go:84` (`net.Listen("tcp", ":0")`); `metric/handler.go:156` —
  `Addr:        fmt.Sprintf(":%d", s.port),` run with port 0 by `newEphemeralServer` (`metric/lifecycle_test.go:75-79`).
- Loopback: `net.Listen("tcp", "127.0.0.1:0")` at `internal/harness/natsfixture/fixture_test.go:316`, `:349`,
  `internal/harness/pindiff/check_test.go:236`, `metric/handler_test.go:232`, `:508`,
  `natsclient/client_close_held_test.go:301`, `natsclient/client_connect_test.go:367`,
  `natsclient/test_helpers_test.go:33`; nats-server on `Host: "127.0.0.1", Port: -1` at
  `natsclient/client_close_final_test.go:186`, `natsclient/client_close_held_test.go:152`, `:210`,
  `natsclient/client_connect_test.go:287`, `natsclient/client_lifecycle_test.go:110`.
- How often each kind of holder is present during a real run: not measured (probe h6, "Not measured").

Surface a change of host, or of how the host is dialled, reaches:

- `PhaseHost` (`errors.go:26`) and its fault-matrix row `fixture_integration_test.go:185` —
  `{PhaseHost, func(d *deps) { d.host = failAfter(d.host, injected) }, 1},`; the phase order `nats-fixture/spec.md:59`.
- Unit fakes that pin "same host": `fixture_test.go:124` and `:187` (`f.url = ... "nats://fixture-host:1"`),
  `fixture_test.go:155-156` (`if dialled != "nats://fixture-host:2" {` / `t.Fatalf("dialled %q, want the re-read mapped
  port on the same host", dialled)`).
- The evidence record keeps the port and no host (`evidence.go:22` `MappedPort`; no host field in `evidence.go:15-27`).
- `TestNoFixedAddressesInTests` refuses a `*_test.go` line containing `nats://127.0.0.1:` or `nats://localhost:`
  (`internal/harness/contract/addresses_test.go:18-19`), so no test may spell either URL prefix as a literal.
- `TestFixtureRoundTrip` (`fixture_integration_test.go:100`) checks only the scheme (`:103`, `strings.HasPrefix(f.URL(),
  "nats://")`).
- Linux CI: `.github/workflows/ci.yml:27` — `runs-on: ubuntu-24.04` (job `verify`). Its `localhost` resolution and
  which addresses a published port listens on (with `HostIp ""`, `127.0.0.1` or an explicit port): not measured.

## 2. How the request asks for ports today, and what testcontainers-go v0.44.0 offers

Today: `ExposedPorts: []string{clientPort}` (`fixture.go:289`) and nothing else. No `PortBindings`,
`HostConfigModifier` or `WithHostConfigModifier` anywhere in the tree
(`git grep -n -E 'WithHostConfigModifier|HostConfigModifier|PortBindings|ExposedPorts|HostPortAccess' -- '*.go'`: one
hit, `fixture.go:289`). No Go file imports `github.com/moby/...` or `github.com/docker/...`
(`git grep -n -E '"github.com/(moby|docker)/' -- '*.go'`: exit 1).

testcontainers-go v0.44.0 (`/Users/coby/go/pkg/mod/github.com/testcontainers/testcontainers-go@v0.44.0`):

- `container.go:139` — `ExposedPorts             []string // allow specifying protocol info`
- `container.go:168` — `HostConfigModifier       func(*container.HostConfig)                // Modifier for the host
  config before container creation`
- `options.go:109` — `func WithHostConfigModifier(modifier func(hostConfig *container.HostConfig))
  CustomizeRequestOption {`
- `docker.go:1264-1266` — the initial `hostConfig := &container.HostConfig{ Tmpfs: req.Tmpfs, }`
- `lifecycle.go:513` — `` if req.HostConfigModifier == nil {` then `:514` the default; `:516` —
  `req.HostConfigModifier(hostConfig) ``.
  A request that sets `HostConfigModifier` replaces `defaultHostConfigModifier` (`lifecycle.go:662-674`), which only
  copies deprecated request fields (`AutoRemove`, `CapAdd`, `CapDrop`, `Binds`, `ExtraHosts`, `NetworkMode`,
  `Resources`, `Privileged`, `ShmSize`). The fixture's request sets none of them (`fixture.go:285-295`).
- `lifecycle.go:546` — `hostConfig.PortBindings = mergePortBindings(hostConfig.PortBindings, exposedPortSet)`, after
  the modifier. An explicit binding for an exposed port is kept, and an empty `HostPort` becomes `"0"`
  (`lifecycle.go:639-645`). An exposed port with no binding gets `lifecycle.go:635` —
  `exposedPortMap[p] = []network.PortBinding{{HostPort: "0"}}`. A binding for a port not in `ExposedPorts` is dropped
  (`lifecycle.go:618`).
- `lifecycle.go:599` — `pr, err := network.ParsePortRange(s)`: an `ExposedPorts` entry is a container port or a range
  only, with no host address and no host port. `ParsePortRange` (`moby/moby/api@v1.55.0/types/network/port.go:187-212`)
  passes the text before `/` to `parsePortNumber`, which is `strconv.ParseUint` (`port.go:346-350`). Read from
  source, not run. User `PreCreates` hooks receive the `ContainerRequest` by value and cannot reach `hostConfig`
  (review r1, `lifecycle.go:23`).
- `options.go:126` — `func WithHostPortAccess(ports ...int) CustomizeRequestOption {` works in the reverse direction
  (the container reaching host ports through an SSHd sidecar). It does not bind a host port.
- **Both bindings need the modifier.** In v0.44.0 a host address (`HostIp 127.0.0.1`) and a host port (`HostPort Q`)
  are both reachable only through `HostConfigModifier`. The probe h6 shows the loopback form
  (`hc.PortBindings = network.PortMap{network.MustParsePort(port): {{HostIP: netip.MustParseAddr(hostIP), HostPort:
  "0"}}}`,
  PR #200 comment 6128508133), and h5b the host-port form (`/tmp/pr200/h5b/main.go:32-33`). The function literal
  names `*container.HostConfig` and `network.PortMap` / `network.PortBinding` (`port.go:164-169`, `:172`). So
  `github.com/moby/moby/api` (`go.mod:61`, `// indirect`) becomes a direct requirement either way.
- This package has twice kept Docker-side modules out of its direct requirements:
  - `internal/harness/natsfixture/deps.go:144` — `// types so the Docker client packages stay indirect dependencies.`
    covers the client module (`github.com/moby/moby/client`, `go.mod:62`).
  - `internal/harness/natsfixture/deps.go:158` — `// The daemon's not-found error is recognisable only through
    containerd's errdefs sentinel,`
    and `:159` — `// which would make another module a direct dependency. Its message has been stable since`.
    Here it chose to classify by error text (`:163`) over a direct `containerd/errdefs` requirement.
  - The ledger row's dependency field names the two direct modules the fixture uses (`docs/admission-ledger.yaml:42-44`:
    "testcontainers-go v0.44.0 (...) and nats.go v1.54.0; creates a container per fixture; writes evidence files").
- Docker keeps a container's `HostConfig` across stop and start. `UpdateConfig`
  (`moby/moby/api@v1.55.0/types/container/hostconfig.go:409-413`) carries only `Resources` and `RestartPolicy`, so
  the published address and port of an existing container cannot be changed. Read from source. So with a
  fixture-picked port, Restart asks for the same `HostPort Q` again; that Docker Desktop re-binds it, and what happens
  if it was taken while stopped, were not measured. With loopback publishing and `HostPort "0"`, Restart got a new
  port (probe h6).

Ports exposed besides 4222: the image declares `6222/tcp` and `8222/tcp` (raw inspect in probe h6:
`"6222/tcp":null,"8222/tcp":null`). testcontainers publishes only what `req.ExposedPorts` lists when that list is not
empty (`lifecycle.go:527-538`). Nothing in the tree reads 6222 or 8222 (`git grep -n -E '\b(6222|8222)\b'`: exit 1;
`":8222\""` is assembled at `internal/harness/contract/addresses_test.go:21` as a forbidden literal, not a reader).

## 3. The no-fixed-ports rule: what each check covers

- `openspec/specs/harness-boundaries/spec.md:105` — `` No `*_test.go` file SHALL contain a fixed broker address
  (`nats://localhost:`, `nats://127.0.0.1:`, `:4222"`, ``
- `openspec/specs/harness-boundaries/spec.md:106` — `` `:8222"`) or bind a fixed TCP port (scripts/lint-test-ports.sh
  SHALL pass). `scripts/lint-test-ports.sh` SHALL ``
- `openspec/specs/harness-boundaries/spec.md:107-108` — the failure output tells the author "to bind port 0 and hand the
  listener itself to the code under test".
- `internal/harness/contract/addresses_test.go:24` — `func TestNoFixedAddressesInTests(t *testing.T) {`;
  `:63` — `if !strings.HasSuffix(name, "_test.go") {`. Non-test files are skipped, and the sensitivity test proves
  a production file holding the literal is not flagged (`:32`). It matches four literal strings (`:17-22`).
- `scripts/lint-test-ports.sh:38` — `LITERAL_PATTERN='net\.Listen\("tcp[46]?",\s*"[^"]*:[1-9][0-9]*"'`;
  `:43` — the `fmt.Sprintf("...:%d"` form; `:46-47` — `--include='*_test.go'` only.
- `scripts/lint-test-ports.sh:21` — `#   - Bind-then-close-then-bind: net.Listen("tcp", ":0"), read the port,`
- `scripts/lint-test-ports.sh:22` — `#     close the listener, and bind that port again. Another process can`
- `scripts/lint-test-ports.sh:23` — `#     take the port in between; SemStreams removed its freePort helper`
- `scripts/lint-test-ports.sh:24` — `#     for this race (its #1120).`
- `scripts/lint-test-ports.sh:56` — `echo 'under test. Do not close it to reuse its port: another process can take the
  port in between.'`

Where each binding falls:

- Fixture-picked port: code in `internal/harness/natsfixture/*.go` (non-test files) is outside both checks by file
  scope. Inside a test file, `net.Listen("tcp", "127.0.0.1:0")` matches neither pattern; `:0` is the accepted shape
  (`scripts/lint-test-ports_fixture_test.sh:60-62`). Binding, closing and handing the number on is the documented
  false negative (`:21-24`). No URL literal arises, because the URL is built with `fmt.Sprintf` (`fixture.go:330`).
  So neither command fails on the ruled direction. The script's own guidance (`:55-56`) and header (`:21-24`) name
  that shape as a race. The rule's AGENTS.md row is `AGENTS.md:83`.
- Loopback publishing with `HostPort "0"`: no port is chosen by the fixture. Nothing in the rule or its guidance
  addresses a host address on a published port.

Related guidance that says the same in prose: `.agents/contracts/semengine-developer.md:286` ("Use ephemeral ports"),
`.agents/contracts/semengine-reviewer.md:277` ("Network listeners use ephemeral ports."), `docs/testing.md:465`
("Listen on `127.0.0.1:0` and read back the port"), `docs/setup-plan.md:261` ("Use dynamic ports"),
`docs/setup-plan.md:265` ("Avoid fixed sleeps and shared default ports").

## 4. Callers of URL, Restart and the port

Measured with `GOFLAGS=-tags=integration gopls references` on `fixture.go:418:20` (`URL`) and `restart.go:40:20`
(`Restart`), plus PR #93's added files (read at `47ca8c3`; the two later commits up to `6670ffc` touch no Go file,
section 6).

`Restart` callers (13 on this tree, 2 more in #93):

- natsfixture unit: `fixture_test.go:96`, `:99`, `:109`, `:147`, `:161`, `:200`, `:215`. These are fakes: `:140` and
  `:195` return port `"2"`, and `:155-156` assert the re-read port is dialled on the same host.
- natsfixture integration: `fixture_integration_test.go:766` (helper `restart`, used at `:799` and `:835`), `:892`,
  `:1005`, `:1020`.
- `natsclient/integration_test.go:75` (`TestIntegration_ReconnectionIsRedial`) and `:141`
  (`TestIntegration_AcknowledgedIsNotDurableOnMemoryStream`).
- PR #93: `processor/graph-ingest/guard_generation_integration_test.go:115` and `:194`.

`URL()` readers:

- natsfixture tests, 14: `fixture_integration_test.go:103,104,653,654,834,846,847,849,850,852,1106,1139`;
  `fixture_test.go:212,213`.
- natsclient integration tests: 61 references in 17 files.
- #93 adds `internal/harness/natsfixture/open.go:32` (`url := f.URL()`) and
  `processor/graph-ingest/guard_generation_integration_test.go:87`, `:166`.
- A cached copy: `natsclient/test_helpers_integration_test.go:74` stores `URL: f.URL()`. Its readers
  (`delivery_settlement_integration_test.go:145,216`, `kv_watcher_ownership_integration_test.go:61`,
  `request_response_bounds_integration_test.go:230`, `stream_visibility_integration_test.go:48`) never call `Restart`.

Port readers: `f.rec.MappedPort` at `fixture_integration_test.go:135` (non-empty after Start) and `:671` (log only).
`docker port` serves as an independent oracle at `fixture_integration_test.go:836`.

Depends on the port changing: none found. Every Restart caller stops or abandons what it built on the old URL and
dials `f.URL()` again afterwards (`natsclient/integration_test.go:91`, `:143`; #93's
`guard_generation_integration_test.go:118`, `:196` through `natsfixture.Open`; `fixture_integration_test.go:850`).
`TestRestartURLDials` asserts that the URL equals `docker port`'s answer and logs old → new
(`fixture_integration_test.go:846-849`). It does not assert a change.

Depends on the port staying the same: none found. Every client that crosses a Restart sets no reconnects:

- `natsclient/integration_test.go:58` (`WithMaxReconnects(0),`); natsclient passes the value straight to
  `nats.MaxReconnects` (`natsclient/client.go:657`), with default `-1` (`:355`).
- #93's `openFixtureClient` sets `natsclient.WithMaxReconnects(0)`.
- The fixture's own connection does the same (`deps.go:105`).

Text whose premise is a changing port: `natsclient/integration_test.go:47-51` (the first client "can point nowhere"),
`restart.go:29`, `docs/testing.md:80`, `nats-fixture/spec.md:157-160`. The spec scenario `spec.md:86-89` and
`TestS1_8Isolation` (`fixture_integration_test.go:651-654`) require two fixtures' host ports, and URLs, to differ.

## 5. The not-found error: where it is built and what an inspect can carry

- Origin: `testcontainers-go@v0.44.0/docker.go:188` — `inspect, err := c.Inspect(ctx)`. That is one inspect, with no
  wait and no retry. Then `:227` — `return network.Port{}, errdefs.ErrNotFound.WithMessage(fmt.Sprintf("port %q not
  found", nwPort))`
  (`errdefs` is `github.com/containerd/errdefs`, `docker.go:25`). The inspect it read is not returned to the caller.
- The fixture passes it through: `deps.go:53-54`, then wraps it as `Cause` at `fixture.go:238` (Start) and
  `restart.go:114` (Restart). Message format: `errors.go:53` — `fmt.Fprintf(&b, "natsfixture: attempt %d phase %s",
  e.Attempt, e.Phase)`
  and `errors.go:62` — `fmt.Fprintf(&b, ": %v", e.Cause)`. Field set: `errors.go:39-49` (`Attempt`, `Phase`,
  `ContainerID`, `ParentErr`, `Cause`, `Cleanup`). The observed strings in #180 match this format.
- Phase names: `errors.go:23-35` (`image`, `start`, `host`, `mapped-port`, `connect`, `jetstream-ready`,
  `create-stream`, `create-kv`, `consume`, `stop-container`, `start-container`).
- The Start error is also written to evidence: `fixture.go:240` copies `errString(cause)` into the attempt record
  (`evidence.go:33`). Restart's is not recorded per phase (section 1).
- `TestErrorReportsEveryField` (`fixture_test.go:274-290`) pins the message's substrings and `Unwrap`.
- What an inspect result offers (`*container.InspectResponse`, `testcontainers-go@v0.44.0/container.go:47` —
  `Inspect(context.Context) (*container.InspectResponse, error)`):
  - `State` (`moby/moby/api@v1.55.0/types/container/container.go:75-88`): `Status`, `Running`, `Paused`, `Restarting`,
    `OOMKilled`, `Dead`, `Pid`, `ExitCode`, `Error`, `StartedAt`, `FinishedAt`, `Health`.
  - `Config.ExposedPorts` (`config.go:32`).
  - `HostConfig.PortBindings` (`hostconfig.go:424`) and `HostConfig.NetworkMode`.
  - `NetworkSettings.Ports` (`network_settings.go:12-13`).
  These are the items the diagnosis and probe h6 captured. Through Docker Desktop's proxy, `NetworkSettings.Ports`
  showed `[]` in the failing cases while raw dockerd showed the port (probe h6). The proxy is what testcontainers uses.
- Reading the fields does not need an import: Go allows field access on the returned value without naming its
  package. Naming the type does need one.
- The only inspect the fixture makes itself today: `deps.go:155` — `_, err = probe.Inspect(ctx)`, inside
  `containerAbsent`. It runs through a fresh client (`deps.go:146-153`) and classifies by error text at `deps.go:163` —
  `if strings.Contains(err.Error(), "No such container") {` (reason at `deps.go:158-162`).
- Measured cost on the failing host: each proxy inspect of an affected container took 3004-3010 ms (diagnosis h5;
  probe h6 3004-3010 ms).
- The pin's precedent for an error that carries the inspect: section 7.

## 6. Open pull requests that overlap

`gh pr list --state open` gives #200 (draft, 0 files), #194 (draft, 5 files), #188 (ready, 17 files) and #93 (draft,
365 files; read in full with `gh api --paginate .../pulls/93/files`).

- #93 at `6670ffc`. `compare 47ca8c3...6670ffc`: 2 commits ahead, touching `.agents/skills/entity-or-bucket/SKILL.md`,
  `.agents/skills/kv-or-stream/SKILL.md`, `AGENTS.md`, `docs/admission-ledger.yaml` and three files under
  `openspec/changes/setup-04a-02-ingest-kernel/`. No Go file. Files #93 shares with what this change can touch:
  - `internal/harness/natsfixture/open.go` (added, 52 lines), `open_test.go` (added, 111), and
    `fixture_integration_test.go` (+41 at the end, hunk `@@ -1145,3 +1145,44 @@`,
    `TestOpenClosesBeforeTheFixtureStops`).
    These call `f.URL()` (`open.go:32`), `startFixture`, `Open`, `conn(f)` and `f.remaining()`. None calls `Restart`,
    `deps.mappedPort`, `deps.host`, `deps.start` or the request.
  - `go.mod` (+1 in the direct `require` block: `golang.org/x/net v0.60.0`) and `go.sum` (+2). Either binding moves
    `github.com/moby/moby/api` into the same block (section 2).
  - `docs/testing.md` (hunk `@@ -556,8 +556,12 @@`, the cover-check list). Its `:80` text is named in section 1.
  - `docs/admission-ledger.yaml` (+926/-8). Its hunks start at `-21`, `-155`, `-493`, `-1716`, `-2356` and `-2483`;
    the first (`-21,7`) ends at line 27, before the natsfixture row (`:31-61`) whose dependency field is `:42-44`.
  - `AGENTS.md` (+14/-1), `Taskfile.yml` (+1/-1, the `cover:check` description), `scripts/cover-check.sh` (+14/-2,
    new targets), `internal/harness/contract/cover_test.go` (+82/-14) and `boundaries_test.go` (+2).
  - Spec deltas: `openspec/changes/setup-04a-02-ingest-kernel/specs/nats-fixture/spec.md` (adds "Connected value for
    a package's tests") and `.../specs/harness-boundaries/spec.md` (modifies "One image pin").
  - #93's Restart callers: `processor/graph-ingest/guard_generation_integration_test.go:115`, `:194` (section 4).
- #188 at `b3bba21` shares `docs/admission-ledger.yaml`, with hunks at `-1639`, `-1703`, `-1770` and `-2205`, none in
  the natsfixture row. It edits `natsclient/request_response_bounds_integration_test.go` and keeps
  `newFixtureClient(t, natsfixture.WithMaxPayload(...))`. It also edits `natsclient/client_close_final_test.go`, one of
  the in-tree loopback nats-server sites (section 1A). It changes no natsfixture file.
- #194 at `47e1940` changes only `openspec/changes/remove-natsclient-test-seams/*`. It shares no file.
- Merge order is for the design to state. The merge stop: #93 and #188 cannot merge while #180 is open unless they
  close it (`.agents/protocol.md:189-191`).

Open issues on the same ground (`gh issue list --state open`, filtered by title): #180 (the bug, `class:flake`, the
only open `class:flake`); #195 (`adoption: natsfixture.Open ...`, blocked on #93; touches `Open`, not ports).

## 7. The pin: SemStreams `8b99efe9`

- SemEngine's fixture is not a port. Its ledger row is `docs/admission-ledger.yaml:31-61`:
  `source_path: natsclient/test_client.go`, `source_sha: 8b99efe9...`, disposition `defer-exclude`, destination
  "internal/harness/natsfixture (reimplemented over nats.go and testcontainers-go; no natsclient.Client)", and a
  consumer purpose naming "one mapped-port replacement". `natsclient/mapped_port_retry_test.go` is `defer-exclude`
  at `docs/admission-ledger.yaml:136-146`. The package comment says "adapted from SemStreams
  natsclient/test_client.go at 5457b345" (`fixture.go:3`). That commit is earlier than the pin and was not read
  (`docs/inventory-scope.md:24` scopes SemStreams to the pin snapshot).
- At the pin, `natsclient/test_client.go` (987 lines) neither picks a host port nor sets a host address. It uses
  `ExposedPorts` only (`test_client.go:418` — `ExposedPorts: append([]string(nil), requiredPortsForConfig(cfg)...),`);
  a grep for `PortBindings` and `HostConfigModifier` finds nothing.
- It polls Inspect for one coherent port-map snapshot, for up to a budget, before failing:
  - `test_client.go:29` — `requiredPortObservationBudget    = 10 * time.Second`
  - `:30` — `requiredPortObservationInterval  = 50 * time.Millisecond`
  - `:20-27` gives the rationale: "a mapping that is momentarily unresolvable under Docker API pressure".
  - `:93-115` takes one inspect per observation.
  - Then it allows one replacement container, only for the mapped-port phase and only when a successful inspect
    observed the port absent: `test_client.go:607-639` and `eligibleForMappedPortReplacement` `:642-658`.
- Its error carries what the observations showed: `test_client.go:134-143` (`requiredPortResolutionError`: attempts,
  budget, elapsed, last successful observation with missing ports, last inspect error) and `:158-189` (`Error()`).
- gh#736 appears at the pin in `test_client.go:421-423`:
  `// LogStrategy still checks container state through Inspect, but it` /
  `// does not call MappedPort and can accept this exact readiness log` /
  `// without waiting for a successful state inspect (gh#736).`
  The fixture attributes its replacement to gh#736 (`fixture.go:28-30`). Setup-02 cites gh#736 for `-p 2`
  (`openspec/changes/archive/2026-09-30-setup-02-isolated-harness/design.md:158`, `:351`). SemStreams issue #736
  itself was not read; it is outside `docs/inventory-scope.md:24`.
- Pin ADRs (contract category 6): this change carries no package from the pin. Searched anyway:
  - All 85 records under `docs/adr` at `8b99efe9` were fetched. `grep -l -i -E
    'testcontainers|test_client|MappedPort|mapped port|mapped-port|PortBindings|host port|ephemeral port|freePort|#736'`
    hit only `025-semteams-consolidation.md:138` ("integration tests (testcontainers)", in passing).
  - Titles naming test, harness, fixture, Docker, container, flake, integration or port: 027, 047, 049, 051, 085, 097,
    107. None is about the test fixture; 047 and 049 are the workflow lifecycle harness.
  - SemEngine's `docs/adr/` (12 records) names neither natsfixture nor host ports
    (`git grep -n -i -E 'natsfixture|testcontainers|host port|mapped port' -- docs/adr`: no hit).

## 8. Coverage

- `scripts/cover-check.sh:24` — `` "internal/harness/natsfixture integration"`; threshold `:16` — `readonly threshold=80
  ``.
  natsfixture is measured from the integration profile only, and `internal/harness/contract/cover_test.go:198-199`
  fails if the unit run covers it. `Taskfile.yml:106` and `.agents/skills/semengine-preflight/SKILL.md:43` say the
  same. #93 edits that description and the script (section 6) but keeps the natsfixture target.
- No `.evidence/last-run` in this worktree, so there is no gate-grade figure. The diagnosis' instrumented run gives
  a rough one (`/tmp/pr200/ev/run-1/integration.coverprofile`, local only; `deps.go` was replaced by a hook in that
  run): 831/962 statements, of which the diagnostic file `zz_pr200_diag.go` is 16/73. That leaves about 815/889
  (92%) for the package's own files. Not evidence for the gate.
- Unit tests cannot reach Start's Docker phases, because Start admits first (`fixture.go:129`). Restart's later phases
  are unit-tested with fakes (`fixture_test.go:122-222`).

## Ryuk (testcontainers' reaper): observed, out of scope

Read, not run (review r1; probe h6 "Ryuk"):

- `testcontainers-go@v0.44.0/reaper.go:393-405` builds the reaper's request: `ExposedPorts` `8080/tcp` (`:395`) and a
  `HostConfigModifier` (`:399-404`) that sets no `PortBindings`. So `mergePortBindings` gives it `HostPort "0"` on all
  addresses, today's binding.
- Its wait is `wait.ForAll(wait.ForLog("Started"), wait.ForListeningPort(port))` (`reaper.go:379-384`).
  `HostPortStrategy` re-polls `MappedPort` until it is non-zero or the context ends (`wait/host_port.go:205-222`;
  default timeout 60 s, `wait/wait.go:59-61`). The reaper is then reached through `PortEndpoint` → `MappedPort`
  (`reaper.go:442`, `docker.go:152`).
- One reaper serves a whole `go test` session (diagnosis: session id from the parent process), and the fixture's
  binding does not reach it. #180 comment 6126769069's reaper failure ("No such container") is a different symptom.

## Contract categories, cross-referenced

1. Claimed gap. Neither "the fixture sets a host address" nor "the fixture picks the host port" holds today; both are
   confirmed absent (section 2 searches). "The not-found error does not carry the inspect" is also confirmed:
   `errors.go:39-49` has no such field, and the only fixture-side inspect is `deps.go:155`, for absence.
2. Every spelling of "the fixture's host, port and URL":
   - Request: `fixture.go:289`, plus testcontainers' `HostPort: "0"` (`lifecycle.go:635`).
   - Container port, spelled twice: `fixture.go:272` and `deps.go:19`.
   - Port read: `deps.go:53`.
   - Host read: `deps.go:51` (Start, via `docker.go:1614-1657`) and `restart.go:100-104` (Restart, from the old URL).
   - URL assembled twice: `fixture.go:330` and `restart.go:153`.
   - Stored: `fixture.go:365`, `restart.go:97`, `restart.go:178`.
   - Evidence: `fixture.go:366`, `restart.go:178`, `evidence.go:22` (port only, no host).
   - Cached copy: `natsclient/test_helpers_integration_test.go:74`.
   - Docker's own `HostConfig.PortBindings` and `NetworkSettings.Ports`, read by `docker port` at
     `fixture_integration_test.go:836`.
   - Address resolution of the host: `deps.go:95-96` (Go dialer) and nats.go `nats.go:2485-2515` (resolve, shuffle,
     dial).
   - "How the fixture inspects a container": `deps.go:145-167` only, plus the inspect inside testcontainers'
     `MappedPort`.
3. Adjacent claims (prior decisions on this exact ground, reported plainly):
   - `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:271` — `` - *Stable host port* (the previous
     draft): bind `4222/tcp` to a host port chosen at `Start` so `URL()` survives the ``.
     Its stated cost is at `:272-277`: a host-port bind collision fails at create/start, where no replacement is
     allowed, and on `Restart` "the container cannot be replaced without losing the store the primitive exists to
     preserve, so a collision fails the restart outright". The decision is at `:289` (`Decision: re-read and
     reconnect.`)
     and the review record at `:793-795` ("stable host port withdrawn"). That design weighed a host port chosen at
     Start. It did not weigh the Docker Desktop failure this issue found, nor a host address on the binding.
   - `openspec/changes/archive/2026-09-30-setup-02-isolated-harness/design.md:344-345` (I12: tests never "bind a fixed
     host port") and `:422-423` (ruled Q8: "Constraining Docker's mapping range would need daemon configuration or
     fixed ports, which the plan forbids."); `:355-358` (E4: Docker-assigned ports can overlap SemStreams' e2e bands,
     a declared cost); `:353` (E1: "testcontainers behaviour on the `desktop-linux` context is read from source, not
     run").
   - `openspec/changes/archive/2026-10-01-flake-defense/design.md:294` — `- Bind-then-close-then-bind stays a false
     negative of the guard and is added to its header list.`
     Its inventory: `:109`; `:591` (SemStreams #1120: "probe-then-bind helper (`freePort`) ... fix: the listener is
     handed over, not a port"); `:594` (SemStreams #1279: "published host ports inside the kernel's ephemeral range;
     the preflight ran 77 s before the bind ... reserve the ports").
   - Specs:
     - `nats-fixture`: "Phased start" (`spec.md:57-65`, phase order `:59`), "Run-unique ownership" (`:79-89`),
       "Restart keeps the container and re-reads its binding" (`:137-177`).
     - `harness-boundaries`: "No fixed addresses in tests" (`:103-123`); "No sleeps in tests" (`:165-171`, covers
       every Go file under `internal/harness/`); "Import graph" (`:11-20`: natsfixture may import no module package
       outside `internal/harness/`; external modules are not limited).
   - Plan: `docs/setup-plan.md:260-261`, `:265-266`, `:283` ("Add retries only for a real caller with proven
     cancellation semantics").
   - Protocol: `.agents/protocol.md:191-192` — a flake-closing PR "shows the reproduction: the command, and how often
     it failed before the fix and after."
   - Library-probe rule (`AGENTS.md:101`): any edited text on how Docker, Docker Desktop or testcontainers binds,
     keeps or fails a port, or how `localhost` is dialled, needs a probe at the pinned versions or must say the
     behaviour is not promised. Examples: `restart.go:29`, `fixture.go:28-30`, `fixture.go:417`, `spec.md:157-160`.
     Probe h6 meets the rule's form (source, command, versions, output on PR #200 comment 6128508133). The h5 and
     h5b sources are local only.
   - Ledger rows: `docs/admission-ledger.yaml:31-61` (dependency field `:42-44`), `:136-146`, `:242-259`
     (lint-test-ports).
   - Open PRs and issues: section 6.
4. Who it is for. natsfixture lives under `internal/`, which Go forbids importing from outside this module.
   Production imports of it are refused (`harness-boundaries` "Import graph"; `TestImportGraph`). Every reader is an
   in-repo test (section 4). Any surface the design adds is plumbing for those tests. None is drafted here.
5. Problem shape. The ruled direction is "one party chooses a host port and hands the number to another binder"
   (reserve-then-hand-over; SemStreams calls it probe-then-bind). Loopback publishing is "narrow the address a
   binder publishes on and let it assign the port".
   - Reserve-then-hand-over has no instance in this tree. `git grep -n -E 'TCPAddr\)\.Port'` finds
     `metric/handler_test.go:115` and `metric/lifecycle_test.go:89`, and both keep their listener open.
   - The tree's established shapes are two others:
     - Hand the listener itself to the code under test: `natsclient/test_helpers_test.go:33`,
       `natsclient/client_connect_test.go:367`, `metric/handler_test.go:232`, `:508`.
     - Let the binder assign, then observe: `natsserver.Options{Host: "127.0.0.1", Port: -1}` at
       `natsclient/client_close_held_test.go:152`, which is also the narrow-address form; and the fixture today,
       `HostPort: "0"` then `MappedPort`.
   - Docker accepts no listener from a client, so hand-the-listener has no Docker form. Whether the change adopts an
     existing shape is for the design to state.
6. Pin ADRs: none govern (section 7).

Not triggered, with reasons (review r1 agreed):

- Same-class collision table: the change adds no durable, communication or runtime-coordination primitive to the
  framework. It changes how one test fixture asks Docker for a binding.
- Adopter seam inventory: no surface is reached from outside this repository (category 4's measurement).
- Intent check: no boundary (port set, tier, exclusion, deferral) moves.
- Pin probe, surface audit and shape sweep: this is not a porting change.

## Searches that closed empty

- `git grep -n -E 'WithHostConfigModifier|HostConfigModifier|PortBindings|HostPortAccess' -- '*.go'`: no hit.
- `git grep -n -E '"github.com/(moby|docker)/' -- '*.go'`: no hit.
- `git grep -n -E '\b(6222|8222)\b'`: no hit.
- `git grep -n -E 'Inspect|NetworkSettings|freePort|FreePort|pickPort|hostPort|HostPort' -- 'internal/harness/**'`:
  natsfixture hits only `deps.go:155` (and `stop.go:210`, a stream's `info.State`).
- `git grep -n -E 'mapped_port|MappedPort'` outside fixture.go and restart.go: readers only in natsfixture tests and
  the ledger.
- `git grep -n -i -E 'natsfixture|testcontainers|host port|mapped port|mapped-port' -- docs/adr`: no hit.
- `grep -n -E 'TESTCONTAINERS|HOST_OVERRIDE' scripts/doctor.sh scripts/test-integration.sh`: no `HOST_OVERRIDE`.
- Pin `test_client.go` and `mapped_port_retry_test.go`: no `PortBindings`, `HostConfigModifier` or `Restart`.
- Restart callers in #93's 365 files: added lines were searched through the API patches. The 22 Go files GitHub
  returns without a patch were fetched at `47ca8c3` and grepped. Hits: only `guard_generation_integration_test.go:115`
  and `:194`. The commits up to `6670ffc` change no Go file.

## Open evidence questions

1. Fixture-picked port, healthy path on Docker Desktop: not exercised. h5b ran an explicit port only against a port
   already held on all addresses; start failed in 73 ms ("ports are not available ... address already in use").
2. Fixture-picked port across Restart: does `docker start` re-bind the same host port on Docker Desktop? What does
   start return when a Mac process took the port while the container was stopped?
3. Fixture-picked port, address of the pick: with which address the pick binds, does the kernel refuse ports held on
   `127.0.0.1` or `[::1]`, so the pick matches what Docker Desktop then binds (`0.0.0.0` and `::` for `HostIp ""`)?
   And what does Docker Desktop do with an explicit port held only on `127.0.0.1` or `[::1]`? Not measured.
4. The URL host under either binding: today it is `localhost` (`docker.go:1650`). Go's dialer tries `::1` first, and
   nats.go shuffles the order. Not run against a holder: the fixture's own `connect` (`deps.go:90-121`) and nats.go's
   order (4 of 4 reached `127.0.0.1` in h6; not shown to be guaranteed).
5. Linux CI (`ubuntu-24.04`): `localhost` resolution, which addresses a published port listens on, how an explicit-port
   collision fails, and whether a second pick in the same run can be handed a port already published (the spec
   requires two fixtures' host ports to differ, `nats-fixture/spec.md:86-89`). None measured.
6. Which holders are present during real runs (all addresses, `127.0.0.1`, `[::1]`), and how often. Not measured.
   0 real holders were caught in 2 instrumented full runs.
7. Flake-closing evidence (`.agents/protocol.md:191-192`). The natural rate on this machine is about 16 of ~181
   recorded full runs with at least one occurrence, and 0 in 2 instrumented runs. Only the planted-holder
   reproductions (h5, h6) are deterministic. What "how often it failed before and after" will be measured on is open.
8. Does an inspect taken after a `MappedPort` failure show the state the failing lookup saw? `MappedPort` does not
   return its inspect (`docker.go:187-228`). Snapshots were identical from +0 to +5 s in h5, and the proxy answered
   `[]` again after ~3 s in h6. That is evidence for the planted-holder condition only.
9. The h5 and h5b sources and logs are under `/tmp/pr200` (local only), not on the PR. The probe rule (`AGENTS.md:101`)
   asks for them on the PR if a design or doc statement rests on them.
