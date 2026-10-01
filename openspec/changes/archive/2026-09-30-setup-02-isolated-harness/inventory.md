# Inventory: setup-02-isolated-harness

- base: `e5d549cd23b43264d781e8266afff4731923fae3`
- semstreams: `5457b3458936f668b71d2fea061f67f8d7d01e67`

This is the architect's revision-2 inventory (Part A), which passed independent inventory review (`INVENTORY PASS`).
SemStreams paths are read at the pin above with `git show`; SemEngine paths at the base. Items marked **[REV]**
changed since revision 1. Tables with long rows are rendered as lists; the content is unchanged except for the three
post-pass corrections below.

## Post-pass corrections (reviewer nits N1–N3)

- **N1.** The "zero Go packages" premise is `docs/repository-map.md:9`; `:10` is "No test harness". Corrected in A0;
  the delta-summary "Nits" entry that defended `:10` is superseded.
- **N2.** The embedded-server census is 20 files, not 19: `internal/maxdelivery/runtime_integration_test.go` starts
  its server with `natstest.RunServerWithConfig` (import `natstest "github.com/nats-io/nats-server/v2/test"`, `:18`)
  at `:320` and `:365`, a spelling the `NewServer` grep does not match. Corrected in A2.1(b) and A6.
- **N3.** The Destroyers row also lists the exact-name removers `taskfiles/dev.yml:35` (`docker rm semstreams-nats`)
  and `taskfiles/e2e/core.yml:131-132` (`semstreams-early-sigterm-{app,blackhole,net}-$suffix`, names set `:126-128`),
  and T-B5's allow-list check must catch destroyer substrings inside words such as `stops` or `loops` in lane
  segments. Corrected in A6 and A7 Seam 4.

## Delta summary (revision 1 → revision 2)

- **B1 ports / Compose project / broker names missing from collision table.** Confirmed: `Taskfile.yml:22-23` `env:
  COMPOSE_PROJECT_NAME: semstreams`; 0 of 12 compose files has top-level `name:`; port owners and bands verified (A6
  rows 2–4). Changed: A2.3, A6 (three new rows), A7 Seam 2/4, B4, B5, B7 I10–I12, B8 specs.
- **B2 SemStreams cleanup prunes by substring.** Confirmed: `taskfiles/e2e/common.yml:77-79`, `ops.yml:80`,
  `agentic.yml:78`, `research-graph.yml:79`, `deep-research.yml:80`, `crud-tools.yml:80`, `clean.yml:17-19` (`docker
  builder prune -f`). Changed: A2.3 corrected, A6 "Destroyers" row, A7 Seam 2 rewritten, B7 I10, contract test T-B5,
  spec `harness-boundaries` "SemEngine-assigned names", evidence item 4.
- **H1 lock scope overstated.** Confirmed: only `scripts/run-integration-tests.sh` takes the lock; callers
  `taskfiles/test.yml:19,24,29`, `.github/workflows/ci.yml:163`; 30+ `compose up`/`docker run` sites do not. Changed:
  A6 Owners row, B4 "Known limit", new declared cost (dynamic-port channel).
- **M1 helper census misdescribed.** Mostly confirmed; one disagreement. `agentic-loop:41`, `agentic-model:31` wrap
  `NewTestClient`; `graph-gateway:31`, `graph-ingest:161` dial `nats://localhost:4222` (38 test files, 135 hits).
  Disagree: the citation `internal/maxdelivery/runtime_integration_test.go:285` is
  `runAuthorizedServer(...) *natsserver.Server`, an embedded-server helper; the reviewer's
  `observer_integration_test.go:285` is a different file (a bounded-wait generic) never cited. Changed: A2.1(b)
  rewritten with the split census; Q4 consequence; new contract test T-B6.
- **M2 shared testcontainers host state.** Confirmed (`internal/config/config.go:22-23,52,60,66,71,76,81,86,114-139`;
  `internal/core/docker_host.go:78-86`; `~/.testcontainers.properties` absent on this host). Changed: A6 new row, A7
  Seam 5, B4 env namespace, O6 note.
- **M3 `scripts/lint-test-ports.sh` adoptable.** Confirmed (55 lines; fixture test `ci.yml:121`; `ci.yml:133`;
  `taskfiles/lint.yml:47`). Changed: A2.6, B3 (carry), ledger rows L9/L10, T-B6.
- **Nits.** `Taskfile.yml:58-61`, `verify.sh:15`, `doctor.sh:58-59`, `protocol.md:41-42`, `test_client.go:417`, `:598`
  corrected. Revision 2 disagreed on `docs/repository-map.md:10`; post-pass nit N1 settles it as `:9` for "no Go
  packages" (`:10` is "No test harness"). ObservedContext: the search was `git grep -l 'ObservedContext
  struct\|observedContext struct' -- '*_test.go'` → 26 (both spellings deliberately); exact `ObservedContext struct` →
  21; `-i` → 26. Stated in A2.2. Changed: A1, A2.1, A2.2.
- **Owner rulings Q1–Q7.** Applied. Changed: B1 (options collapsed to rulings), B3, B4, B8 ledger and tasks.

## A0. Problem statement

SemEngine has zero Go packages (`docs/repository-map.md:9`, `:15`; `git ls-files | grep '\.go$'` → 0). SETUP 02 must
add the first ones: a harness that owns disposable real-NATS fixtures, bounded waits, checked cleanup, failure
evidence, lifecycle-ownership proofs, and Docker admission shared with SemStreams on one daemon, without admitting any
SemStreams package and without recreating SemStreams' harness defects (#1411 lifecycle copy-paste, #1417 unbounded
cleanup roots, and **[REV]** the substring-filtered volume destroyers and fixed host-port bands found under B1/B2).

## A1. The claimed gap (SemEngine)

Closed empty as before. `git grep -n -i` over tracked content excluding `docs/setup-plan.md`, `.agents/contracts/`,
`package-lock.json`: `testcontainers|Ryuk|Compose|prune|setsid|pgid|process
group|context.Background|context.TODO|dev:` → 0; `nats` → 1 (`docs/repository-map.md:10`); `integration` → 6
(`Taskfile.yml:63`, `scripts/verify.sh:8-9`, `docs/repository-map.md:15`,
`.agents/skills/semengine-preflight/SKILL.md:43`); `lock` → 17, all prose except `semengine-preflight/SKILL.md:59`;
`Cleanup|cleanup` → 2 prose. **[REV]** additional closures for B1:
`COMPOSE_PROJECT_NAME|container_name|--name |volume|network` → 0 in tracked files.

Existing seams (pins **[REV]** corrected where the reviewer was right):

- `Taskfile.yml:58-61` — `test:unit: … scripts/gopkgs.sh go test -race -count=1 ./...`
- `scripts/verify.sh:10` — `steps=(spec:check docs:check fmt:check tidy:check build vet lint vuln test:unit)`; `:15` —
  `tracked_state() { git status --porcelain --untracked-files=no; git diff HEAD | shasum; }`
- `scripts/gopkgs.sh:8-14` — empty-module refusal
- `scripts/doctor.sh:58-59` — `if docker info >/dev/null 2>&1; then ok docker-daemon "reachable" … else warn
  docker-daemon "not reachable (no current gate needs it)"`
- `.github/workflows/ci.yml:20-22` — `verify` job, `timeout-minutes: 15`; `:44-60` `required`
- `.gitignore:11-14` — coverage patterns only
- `go.mod:1-7` — module, `go 1.26.3`, tools; no `nats.go`, `nats-server`, `testcontainers-go`, `testify`, `yaml`
- `revive.toml:36-37` — `function-length [80, 0]`
- `docs/provenance.md:12-17` — rules 2–3 (full SHA, no row no port); `:32-34` ledger location undecided → **[REV]**
  now ruled: `docs/admission-ledger.yaml`
- `.agents/protocol.md:41-42` — "Heavy local gates run one agent at a time on a shared host"

## A2. Every current spelling of the facts being modeled (SemStreams `5457b345`)

### A2.1 "An owned disposable real-NATS fixture"

**(a) `natsclient/test_client.go` (987 lines) + `test_options.go` (61 lines)** — unchanged from revision 1 except
pins: `:2` `package natsclient`; `:10` `"testing"`; `:16` testcontainers import (production, tier-1-frozen package
importing test libraries); phases `:528-538`; `testClientSetupError` `:545-586`; ready log `:384-386`; port
observation `:198-285`; one replacement `:611-640`; fresh per-op cleanup deadlines `:303-324` (`:310`, `:317`);
terminate container returned with error `:334-351`; no retained context `:354-364`; stream bounds `:911-929`;
`Terminate` once `:932-939`; `NewTestClient` from `t.Context()` `:854-870`; `NewSharedTestClient` roots at
`context.Background()` `:849`; eleven caller knobs `:437-526`, `test_options.go:14-61`; `natsclient.Client` coupling
`:757-761`, `:873-909`; image as fragment **[REV]** `:417` — `Image: "nats:" + cfg.natsVersion,` and `:598` —
`natsVersion: "2.14-alpine",` (mutable tag). Reach: 168 test files.

**(b) [REV] Real-NATS access outside (a): four distinct spellings, none with a sanctioned home.**

- *Embedded `nats-server/v2` in-process* — `git grep -l 'natsserver.NewServer\|server.NewServer' 5457b345 --
  '*_test.go'` → 19 files in 17 directories: `internal/boot`, `internal/e2eslowconsumer`, `natsclient`,
  `processor/agentic-governance`, `processor/graph-clustering`, `processor/graph-embedding`, `processor/graph-index`,
  `processor/graph-index-spatial`, `processor/graph-index-temporal`, `processor/graph-query`,
  `processor/research-graph-{assess,classify,execute,route,synthesize}`, `service`, `test/e2e/scenarios`. **[N2]** A
  twentieth file uses a second spelling: `internal/maxdelivery/runtime_integration_test.go:285`
  `runAuthorizedServer(...) *natsserver.Server` (embedded, with authorization) starts its server with
  `natstest.RunServerWithConfig` (`nats-server/v2/test`, imported `:18`) at `:320` and `:365`; adding
  `RunServerWithConfig` to the grep gives **20 files in 18 directories**. Canonical copy
  `processor/graph-query/lifecycle_owner_test.go:49-61`: `natsserver.Options{Port: -1, JetStream: true, StoreDir:
  t.TempDir(), NoLog: true, NoSigs: true}`; `:55` `t.Cleanup(server.Shutdown)` (no `WaitForShutdown`, unjoined); `:59`
  `client.Close(context.Background())` (unbounded, error discarded — the #1417 shape). Eight near-identical
  `newLifecycleNATSClient(t)` helpers: graph-clustering:50, graph-embedding:56, graph-index-spatial:34,
  graph-index-temporal:33, graph-query:49, research-graph-assess:140, research-graph-execute:115,
  research-graph-route:136. Version drift: `go.mod:11` `nats-server/v2 v2.12.4` vs image `2.14-alpine`; the
  convergence guard `test/contract/nats_version_contract_test.go:14,53-61` scans image refs only.
- *Wrappers over (a)* — `processor/agentic-loop/tool_result_redelivery_integration_test.go:41` `newLoopNATS` and
  `processor/agentic-model/provider_settlement_integration_test.go:31` `newProviderSettlementNATS` call
  `natsclient.NewTestClient(t, WithJetStream(), WithStreams(...))`.
- *Fixed-address dial of a broker the test does not own* — `gateway/graph-gateway/classifier_wiring_test.go:31-36` and
  `processor/graph-ingest/metrics_test.go:161-167`: `natsclient.NewClient("nats://localhost:4222")`. Census: `git
  grep -l 'nats://localhost:4222' 5457b345 -- '*_test.go'` → **38 files**, **135 hits**. These tests reach whatever
  broker is on the host's 4222 — SemStreams' `semstreams-nats` (`taskfiles/dev.yml:16`) or anyone else's.
- *Synchronous fakes* — `testutil/nats.go:13-21` `MockNATSClient`; `:224-245` polling under `context.Background()`;
  ruled out by the plan.

### A2.2 "Component lifecycle floor + owner proofs"

Unchanged; search stated. `component/lifecycle.go:43-68`;
`component/lifecycle_test_suite.go:1,11,14-15,23,58-62,64-76,85-93,95-103,105-118,184-222,292-326(297,314),392-499`;
`docs/contributing/01-testing.md:513-534`; `processor/graph-query/component.go:172-183,480-536(482-488,498-501,516),`
`597-648(603-608,613-624,627,638-640),654-700(683-689)`; `internal/lifecyclecleanup/lifecyclecleanup.go:12,17-19,33`;
owner-proof idiom `processor/graph-query/lifecycle_owner_test.go:18-33,147-189,191-232,234-244`. Counts:
`lifecycleUsed` non-test 29 files, `cleanupPending` 33 (issue #1411: 30/34 at `3dc4ccbe`). **[REV]** probe copies:
`git grep -l 'ObservedContext struct\|observedContext struct' 5457b345 -- '*_test.go'` → 26 files (21 with exact
`ObservedContext struct`, 5 lower-case); `entered, release := make(chan struct{}), make(chan struct{})` → 9 files.

### A2.3 "Docker workloads, admission, cleanup on a shared host" [REV — corrected]

*Lock (only one taker).* `scripts/run-integration-tests.sh:8` `default_lock_dir="/tmp/semstreams-integration.lock"`;
`:9-16` env `SEMSTREAMS_INTEGRATION_LOCK_DIR`, `SEMSTREAMS_INTEGRATION_LOCK_WAIT_SECONDS`,
`SEMSTREAMS_INTEGRATION_REFRESH_IMAGE`, `SEMSTREAMS_CONTRACT_IMAGE_PULL_TIMEOUT_SECONDS`; `:47-54` owner token;
`:61-81` `read_owner` (unknown keys ignored); `:97-106` stale rule; `:108-121` quarantine; `:123-140` token-checked
release; `:208-242` acquire; `:252-253` traps (bash defers the trap until the foreground child exits and forwards
nothing); `:259` `TESTCONTAINERS_RYUK_DISABLED=false`; `:302-323` image `nats:2.14-alpine`, cached-or-bounded pull;
`:333` `GRAPH_INDEX_LATENCY_LOG`; `:341` `go test -race -failfast -tags=integration -timeout=20m -count=1 -p
2`. **Callers: `taskfiles/test.yml:19,24,29` and `.github/workflows/ci.yml:163` only.**
`docs/contributing/01-testing.md:338-353` documents it; `:305-306` "Do not invoke … with `go test` directly" is
doc-rank.

*Unadmitted Docker workloads (no lock).* `git grep -n 'docker compose .* up\|docker run ' 5457b345 -- taskfiles
Taskfile.yml .github/workflows` → 30+ sites: `taskfiles/dev.yml:16` (`docker run -d --name semstreams-nats -p
4222:4222 -p 8222:8222 nats:2.14-alpine -js -m 8222`); `taskfiles/services.yml:8,16,24,34` (`services.yml` profiles
embedding/tls/observability/all); `Taskfile.yml:207` (tiered up);
`taskfiles/e2e/{core,agentic,crud-tools,deep-research,lessons,lifecycle,ops,research-graph,semantic}.yml`;
`.github/workflows/release.yml:173-174`.

*Compose project naming.* `Taskfile.yml:22-23` — `env:` / `COMPOSE_PROJECT_NAME: semstreams` for every Task
invocation; rationale `:10-21`: without it Compose derives the project from the parent directory `docker/compose/`,
"so they all resolve to the project name `compose` and Docker treats them as ONE project" across sister repos, and
teardown "removes a sister repo's containers". No compose file sets top-level `name:` (`grep -c '^name:'` → 0 in
all 12). Container names are fixed literals (`e2e.yml:19` `container_name: semstreams-e2e-nats`, `:57`, `:109`).

*Host ports.* Fixed bands published by compose files (host side): `services.yml:30,85,129,156,203` → 8081, 8083, 9000,
9090, 3000; `agentic.yml` 34222 36060 38080 38180 38222 39090; `lifecycle.yml` 34222 34550 38080 38222 39090;
`tiered.yml` 34222–39190 (23 bindings); `e2e.yml` 34222–39090; `e2e-slow-consumer.yml` 38090 39100;
`research-graph.yml` 44222–49090; `deep-research.yml` 54222–59090; `ops.yml` 61080–62222; `crud-tools.yml`
64222–65222; dev broker 4222/8222. Guards: `scripts/e2e-check-ports.sh` (328 lines, gh#1175, derives the set from
`docker compose config`; called `taskfiles/e2e/common.yml:36`); `scripts/e2e-reserve-ports.sh` (251 lines, gh#1279,
writes `net.ipv4.ip_local_reserved_ports`; `common.yml:53`, `e2e-ladder.yml:69,99`; its header `:16-19` records that
31 of 41 published ports fall inside the kernel's default ephemeral range 32768–60999); `scripts/lint-test-ports.sh`
(55 lines, refuses fixed `net.Listen` ports in `*_test.go`; `ci.yml:133`, `taskfiles/lint.yml:47`; fixture test
`ci.yml:121`).

*Cleanup — corrected: SemStreams DOES destroy by substring and DOES prune.* `taskfiles/e2e/common.yml:70-76` per-file
`down -v`; **`:77-79`** — `docker volume ls -q --filter name=semstreams | xargs -r docker volume rm`, same for
`semembed` and `nats-semstreams`; `taskfiles/e2e/agentic.yml:78` (`name=agentic`), `crud-tools.yml:80` (`crud-tools`),
`deep-research.yml:80` (`deep-research`), `ops.yml:80` (**`name=ops`**), `research-graph.yml:79` (`research-graph`);
`taskfiles/clean.yml:16-19` — `down -v`, `volume ls --filter name=semstreams | xargs docker volume rm`, `docker
images --filter reference=semstreams-semstreams | xargs docker rmi -f`, **`docker builder prune -f`** (daemon-wide
build cache). Docker's volume `name` filter matches any part of the name (Docker CLI reference; not exercised here —
no volumes exist on this host). So a routine SemStreams `task e2e:clean` or `task clean:docker` deletes any volume on
the daemon whose name contains `semstreams`, `nats-semstreams`, `semembed`, `agentic`, `crud-tools`, `deep-research`,
`ops`, or `research-graph`, whoever created it. Revision 1's "never prune" claim is withdrawn.

*testcontainers-go v0.40.0 facts (module cache, verified).* Session ID from parent pid + create time
(`internal/core/bootstrap.go:41-95`); Ryuk reaps by session labels only (`reaper.go:545-552`); `ReaperDefaultImage =
"testcontainers/ryuk:0.13.0"` (`internal/config/config.go:14`); `RYUK_RECONNECTION_TIMEOUT` 10s (`:72`); `SessionID()`
exported (`testcontainers.go:52`); `GenericContainer` returns `c, err` non-nil on create/start failure
(`generic.go:89,94`); `Labels` field (`container.go:140`). **[REV]** Host-level shared state: config read from
`~/.testcontainers.properties` then overridden by env (`config.go:22-23`, `:92-95`); env names
`TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX` (`:52`), `TESTCONTAINERS_RYUK_DISABLED` (`:60`, read `:114`),
`TESTCONTAINERS_RYUK_CONTAINER_PRIVILEGED` (`:66`),
`RYUK_RECONNECTION_TIMEOUT`/`RYUK_CONNECTION_TIMEOUT`/`RYUK_VERBOSE` (`:71,76,81`, also accepted with
`TESTCONTAINERS_` prefix `:174`), `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE` (`:86`); Docker host resolution order
`internal/core/docker_host.go:78-86`: `tc.host` property → `DOCKER_HOST` → **current Docker context** → default socket
→ `docker.host` property → rootless. `~/.testcontainers.properties` is absent on this host.

*Go toolchain facts (unchanged).* `cmd/go/internal/base/signal_unix.go:14`, `base/signal.go:17-24`, `test.go:1449`:
the `go` command only stops launching new actions on SIGINT, does not handle SIGTERM, never kills running test
binaries; `t.Cleanup` does not run on a signalled binary.

### A2.4 Unbounded-cleanup guard

Unchanged. `scripts/check-cleanup-roots.sh:8`; `test/testinfra/cleanup_guard_test.go` 1895 lines +
`cleanup_analyzer_test.go` 2133 lines; `time.Sleep` in 119 test files.

### A2.5 Classified errors

Unchanged. `pkg/errs/errs.go:13,94,394-445,447-520`; test-side second spelling `natsclient/test_client.go:545-586`.

### A2.6 Guards a fresh repo could adopt [REV]

- `test/contract/context_ownership_contract_test.go:19-23` (no retained context; 345 lines).
- `scripts/lint-test-ports.sh` (55 lines; `LITERAL_PATTERN='net\.Listen\("tcp[46]?",\s*"[^"]*:[1-9][0-9]*"'` `:29`;
  known false-negative shapes listed `:13-20`) with `scripts/lint-test-ports_fixture_test.sh`; enforces exactly the
  plan's "ask the OS for a port" rule (`01-testing.md:419-420`). Does not cover fixed *dial* literals
  (`nats://localhost:4222`, 135 hits) — a gap SemEngine's T-B6 closes.
- `test/contract/nats_version_contract_test.go` — image-ref scan only; misses the Go-module server version. Not
  adopted (one pin file replaces it).

### A2.7 Not a spelling

Unchanged. `pkg/context` is LLM prompt context; `pkg/lifecycle` is workflow entities.

## A3. Adjacent claims

Unchanged, plus rulings. Plan `:236-255`, `:257-279`, `:242-243`, `:245-247`, `:335-345`; `:226-227` vs brief on
coverage timing (now ruled Q3); owner ruling 4 (#1411/#1417 → repair-before-port rows); issue #6 "reuse helpers inside
the port set" reconciled as file-level adaptation with ledger rows; `scripts/verify.sh:8-9` reserves the
`test:integration` slot; live daemon: Docker `29.8.0 linux/arm64`, context `desktop-linux`, one container
`semsource-setup03a-embed` mapped `127.0.0.1:53492->8081/tcp` (a Docker-assigned host port inside 32768–60999 —
evidence for the dynamic-port channel in A6), networks `bridge host none`, no volumes, lock dir absent; SemStreams has
9 live worktrees here. **[REV]** Owner rulings 2026-09-30 applied: ledger at `docs/admission-ledger.yaml` with a `task
verify` check; lock adopted as-is, exclusive; testcontainers only; 80% now on the three harness packages; dev NATS
deferred; digest pin; production lifecycle guard out.

## A4. Consumer at birth

Unchanged verdicts; **[REV]** additions. Added: name-prefix contract test (consumer: the evidence sentinel container
and every later Compose lane); fixed-dial contract test (consumer: every SemEngine integration test, 135 SemStreams
hits show the demand); ledger file + check (consumer: the 12 rows in `docs/admission-ledger.yaml`);
`lint-test-ports.sh` carry (consumer: harness tests that `net.Listen`). Still out: production lifecycle guard,
`pkg/errs`, graph fixtures (03B), Compose lane, dev NATS, TestMain constructor, fixture knobs, embedded server,
testify, AST guard.

## A5. Problem shape

Unchanged: admit-or-refuse (`run-integration-tests.sh:208-242`; `gopkgs.sh:8-14`); phased acquisition + typed
failure + bounded rollback (`test_client.go:700-846`, `lifecyclecleanup.go:33`); observe-then-compare
(`verify.sh:15,41-45`); checked absence (new); process-group ownership (new). **[REV]** Sixth shape: *namespace
discipline against a foreign substring destroyer* — no instance in either repo protects a name from another repo's
`--filter name=`; SemStreams' own `COMPOSE_PROJECT_NAME` (`Taskfile.yml:10-23`) is the closest instance (namespacing
to avoid the `compose` project collision) and is adopted in spirit (prefix) but not its mechanism (a global env var
that fails silently when a compose file is run outside Task).

## A6. Same-class collision table [REV — extended]

Semantic class: admission, identity, naming, and reaping of disposable test workloads on a host's shared Docker
daemon. One entry per dimension.

- **Owners — admission.** SemStreams host lock `scripts/run-integration-tests.sh:8,208-242` (exclusive; **taken only
  by that script**, callers `taskfiles/test.yml:19,24,29`, `ci.yml:163`); `go test -p 2` (`:341`, in-run
  bound). **Unadmitted Docker starters (H1):** `taskfiles/dev.yml:16`, `taskfiles/services.yml:8,16,24,34`,
  `Taskfile.yml:207`, `taskfiles/e2e/*.yml` (30+ `up`/`run` sites), `.github/workflows/release.yml:173-174`. No
  SemEngine owner.
- **Owners — host ports.** Fixed publishers: SemStreams compose bands (A2.3; 3000/8081/8083/9000/9090; 34xxx–39xxx;
  44xxx–49xxx; 54xxx–59xxx; 61xxx–62xxx; 64xxx–65xxx), dev broker 4222/8222 (`dev.yml:16`), `semsource-setup03a-embed`
  (dynamic 53492). Guards: `scripts/e2e-check-ports.sh` (derived preflight), `scripts/e2e-reserve-ports.sh` (kernel
  `ip_local_reserved_ports`; only in e2e task/CI), `scripts/lint-test-ports.sh` (tests may not `net.Listen` a fixed
  port). Docker's dynamic host-port allocator is the only port owner SemEngine would use; on Linux daemons it draws
  from the kernel ephemeral range (default 32768–60999 — the same band as SemStreams' 34xxx–59xxx fixed ports;
  `e2e-reserve-ports.sh:16-19` says 31 of 41 fall inside it). Unverified here whether the daemon honours
  `ip_local_reserved_ports` for port mapping; treated as a collision channel in B4.
- **Owners — Compose project name.** `Taskfile.yml:22-23` `COMPOSE_PROJECT_NAME: semstreams` (all SemStreams stacks
  share one project); direct `docker compose -f docker/compose/x.yml` outside Task → project `compose` (parent-dir
  default, `Taskfile.yml:12-14`), the documented cross-repo collision. No compose file sets `name:`. SemEngine has no
  compose file.
- **Owners — broker resource names (streams/buckets/consumers).** Only one shareable broker exists:
  `semstreams-nats:4222` (`dev.yml:16`), dialled by 38 SemStreams test files / 135 hits (`nats://localhost:4222`).
  SemStreams' per-test containers (168 files) and embedded servers (**[N2]** 20 files) are private brokers. Naming
  inside a private broker is per-test (`01-testing.md:407-428`). SemEngine owns none.
- **Destroyers (B2).** Substring-filtered: `taskfiles/e2e/common.yml:77-79` (`name=semstreams`, `semembed`,
  `nats-semstreams`); `agentic.yml:78` (`agentic`); `crud-tools.yml:80`; `deep-research.yml:80`; `ops.yml:80` (`ops`);
  `research-graph.yml:79`; `clean.yml:17` (`semstreams`), `:18` (`rmi -f` by reference `semstreams-semstreams`), `:19`
  (`docker builder prune -f`, daemon-wide). All substring-filtered, all `|| true`, all reachable from routine `task
  e2e:clean` / `task clean:docker`. **[N3]** Exact-name removers: `taskfiles/dev.yml:35` (`docker rm semstreams-nats`)
  and `taskfiles/e2e/core.yml:131-132` (`docker rm -f` / `docker network rm` of
  `semstreams-early-sigterm-{app,blackhole,net}-$suffix`, names set `:126-128`). Because Docker's `name` filter
  matches any part of a name, a SemEngine lane segment that merely contains a destroyer (`stops` and `loops` contain
  `ops`) is selectable; the T-B5 allow-list check matches substrings, not whole segments.
- **Catalogs.** None: no registry of names, ports, or projects in either repo (`git grep -n 'lock\|COMPOSE_PROJECT'
  5457b345 -- taskfiles Taskfile.yml` → only `Taskfile.yml:23`); `.nats-image` (B3) will be SemEngine's first.
- **Status.** Lock owner file keys `host,pid,started,identity,token,command` (`:213-220`); `describe_owner` on
  contention (`:93-95`); `e2e-check-ports.sh` reports the holder of a port; no read-only status command in either
  repo.
- **Lifecycle.** Acquire `mkdir` (`:212`); release token-checked (`:123-140`); stale = same-host dead pid / changed
  `lstart` → quarantine (`:97-121`); wait 0–3600s (`:7,12`); compose stacks: `up -d --wait` / `down -v --timeout 15`;
  volumes: substring rm; build cache: prune.
- **Ownership.** Lock: single host-level holder; cross-host unverifiable (`:98`). Compose: one project `semstreams`
  for all SemStreams stacks. Volumes/images: claimed by substring, i.e. by anyone whose name matches.
- **Readers.** SemStreams runner + `test/testinfra/integration_runner_contract_test.go:65,871,933`;
  `e2e-check-ports.sh`; humans.
- **Writers.** Lock: SemStreams runner only. Ports: compose files, `dev.yml`. Volumes: the destroyers above.
- **Recovery.** Stale quarantine (same host); Ryuk reaps a dead session's containers after ~10s (`config.go:72`);
  nothing recovers a volume deleted by a substring filter.
- **Shared host-level testcontainers state (M2).** `~/.testcontainers.properties` (absent here) and env
  `TESTCONTAINERS_RYUK_DISABLED`, `TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX`, `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE`,
  `TESTCONTAINERS_RYUK_CONTAINER_PRIVILEGED`, `RYUK_*`, `DOCKER_HOST`, current Docker context
  (`config.go:52-87,114-139`; `docker_host.go:78-86`); both runners export `TESTCONTAINERS_RYUK_DISABLED=false`
  (`run-integration-tests.sh:259`), which overrides a properties file (`config.go:114`). Env namespaces in use:
  `SEMSTREAMS_INTEGRATION_*`, `SEMSTREAMS_CONTRACT_IMAGE_PULL_TIMEOUT_SECONDS`, `GRAPH_INDEX_LATENCY_LOG`
  (`:11-16,333`).
- **Second spelling of run identity.** testcontainers session label `org.testcontainers.golang.sessionId`
  (`internal/core/labels.go:27`) — the one to extend, not duplicate.
- **Mutable image tag.** `nats:2.14-alpine` re-pointed by `SEMSTREAMS_INTEGRATION_REFRESH_IMAGE=1`
  (`01-testing.md:349`; runner `:307-323`) — a host-cache mutation visible to any repo using the same tag.

## A7. Adopter seam inventory [REV — Seams 2, 4, 5 changed/added]

Adopters: (i) SemStreams developer/agent sharing the daemon; (ii) SemEngine developer running `go test` directly;
(iii) SemSource later (no consumer yet; `internal/` keeps the Go API off the bill).

**Seam 1 — shared lock (unchanged).** Must know: nothing (byte-compatible protocol; their runner reports the owner);
`command=semengine <worktree>/scripts/test-integration.sh` disambiguates the repo. Default path: fail fast with owner.
Rank: typed refusal. Gap: cross-host stale lock is manual.

**Seam 2 — SemStreams' persistent services and other worktrees vs SemEngine's cleanup (rewritten).** Must know:
nothing; SemEngine removes only container IDs observed under its own session label, never filters by name substring,
never prunes. Default path: untouched. Rank: contract test T-B4 (no `prune`, no `--filter name=`, no `down -v` on a
project SemEngine did not create) plus recorded evidence.

**Seam 3 — `go test` run directly (unchanged).** Must know: use `task test:integration -- <pkg>`. Default:
`ErrNotAdmitted` at first fixture use, naming the task. Rank: typed runtime error.

**Seam 4 — SemEngine's resources vs SemStreams' destroyers (new; the reverse direction of pass-evidence 4).** Who
pays: a SemEngine developer who names a volume, network, container, or Compose project. Must know: the eight destroyer
substrings, and that `ops` is one of them. Debt count 8 → a design finding, closed by construction: every
SemEngine-assigned Docker name starts with `semengine-`, lane segments come from a fixed allow-list, run-unique
suffixes are lowercase hex, and contract test T-B5 rejects any literal or generated name containing a destroyer
substring — **[N3]** including inside a longer word such as `stops` or `loops`, so the allow-list check is a substring
check, not a segment match. Default path after the design: a violating name fails `task verify`. Rank:
compile/verify-time. What remains at doc rank: `docker builder prune -f` (`clean.yml:19`) empties the daemon's build
cache — SemEngine builds no images in SETUP 02; when it does, a cache miss is slower, not incorrect.

**Seam 5 — host-level testcontainers state and env (new).** Who pays: a developer whose shell or
`~/.testcontainers.properties` sets `TESTCONTAINERS_RYUK_DISABLED=true`, a hub prefix, or a different `DOCKER_HOST`.
Must know: nothing about Ryuk (the runner exports `false`, which wins over the properties file); a hub prefix
re-points the image path but the digest pin still verifies content or fails the pull; `DOCKER_HOST`/context selects
the daemon and the `/tmp` lock does not follow it (declared limit). SemEngine reads only `SEMENGINE_*` variables plus
the testcontainers ones it sets itself; it never reads `SEMSTREAMS_*`, so a shell tuned for SemStreams cannot silently
change a SemEngine run. Rank: `task doctor` reports the effective Docker host, context, Ryuk setting, properties-file
presence, and image cache state (log rank, diagnostic only — no correctness fact lives there).

**Prefer observation to prediction (unchanged list, plus):** the leak check filters by the session label the fixture
observed and wrote, not by names SemEngine predicted; the name-prefix test checks generated `Name()` output by
construction rather than trusting a convention.

## A8. Open evidence questions

- E1 testcontainers on `desktop-linux` — partially answered: resolution order consults the current Docker context
  (`docker_host.go:82` step 3); still not run.
- E2 CI pull time for `nats` + `ryuk:0.13.0` images vs the 15-minute `verify` timeout.
- **[REV] E4** Whether this daemon's dynamic host-port allocator honours `ip_local_reserved_ports` and what its range
  is (`docker info`/daemon config not inspected for that; one observed mapping at 53492).
