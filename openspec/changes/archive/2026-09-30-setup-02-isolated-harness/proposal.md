# setup-02-isolated-harness

## Why

SemEngine has no Go packages and no test harness (`docs/repository-map.md:9-10`, `:15`). SETUP 02 (epic #6) adds the
first ones before any slice is ported. It must not recreate SemStreams' harness defects at `5457b345`: production
packages importing test libraries (`natsclient/test_client.go:10,16`; `component/lifecycle_test_suite.go:11,14-15`); a
29-file owner-lifecycle copy-paste (#1411); 334 unbounded cleanup roots (#1417); four unsanctioned real-NATS spellings
including 135 fixed `nats://localhost:4222` dials; fixed e2e host-port bands and container names; volume deletion by
substring (`--filter name=ops` among others) and daemon-wide `docker builder prune -f` from routine tasks; a runner
whose INT/TERM never reach `go test`.

## What Changes

- Test-only packages under `internal/harness`: `natsfixture`, `probe`, `lifecycletest`, `runner` (tests), `contract`
  (tests).
- `scripts/test-integration.sh`: SemStreams' host lock adopted byte-compatibly at `/tmp/semstreams-integration.lock`
  (exclusive), process-group ownership with signal forwarding, digest image pin from `.nats-image`, evidence
  directory, post-run leak check by testcontainers session label (IDs only, never name filters).
- Naming invariant: every SemEngine-assigned Docker name starts with `semengine-` and contains none of the SemStreams
  destroyer substrings; tests never dial fixed broker addresses or bind fixed ports (`scripts/lint-test-ports.sh`
  carried).
- `docs/admission-ledger.yaml` with 12 entries and a schema check in `task verify`.
- Gates: `task test:integration` joins `verify` after `test:unit`; `cover:check` enforces 80% on `natsfixture`,
  `lifecycletest`, `probe`; `ledger:check` alias; `doctor` reports lock owner, Docker host/context, Ryuk env, image
  cache.
- `go.mod` gains `nats.go`, `testcontainers-go`, `gopkg.in/yaml.v3` (test-only). No testify, no nats-server.

## Owner rulings applied

The owner ruled on 2026-09-30 on [issue #6][setup-02-rulings]; earlier plan rulings are
in the [rulings log on PR #1](https://github.com/C360Studio/semengine/pull/1#issuecomment-5917717356).

- The admission ledger is `docs/admission-ledger.yaml`, machine-checked in `task verify`.
- Docker admission adopts SemStreams' lock at `/tmp/semstreams-integration.lock` as-is; exclusive is the initial
  bound; no SemStreams rename now.
- NATS fixtures use testcontainers only.
- 80% coverage is enforced now on `natsfixture`, `lifecycletest`, and `probe`.
- A SemEngine dev NATS task is deferred.
- The NATS image is pinned by digest.
- The production owner-lifecycle guard (semstreams#1411) is out of scope and becomes `component`'s repair-before-port
  ledger row at 03B/04A.

## Non-goals

Production owner-lifecycle guard (ruled: `component`'s repair-before-port row at 03B/04A); `pkg/errs`;
Graphable/triple/metadata/content fixtures (types absent; 03B semantics); Compose consumer stack (naming rule only);
SemEngine dev NATS (ruled deferred; SemStreams tests dial `localhost:4222`); embedded nats-server; AST cleanup guard;
a SemStreams lock rename (ruled: not now).

## Known limits declared

The lock covers only SemStreams' integration runner; its e2e, services, dev and release Docker workloads are
unadmitted. Docker's dynamic host ports overlap SemStreams' fixed 34xxx–59xxx e2e bands (E4). The `/tmp` lock does not
follow `DOCKER_HOST` to a remote daemon. `-p 2` is inherited from SemStreams gh#736, unmeasured here.

## Capabilities

### New Capabilities

- `integration-test-runner`: the admitted Docker test entry — shared host lock, process-group ownership, leak check by
  session, canonical invocation and pins.
- `nats-fixture`: one owned disposable NATS container per test — admission, phased start with typed failure, safe
  run-unique names, checked cleanup.
- `lifecycle-suite`: the portable owner-lifecycle floor and the probes that observe joins and retained state.
- `harness-boundaries`: machine-checked limits — import graph, no retained context, one image pin, SemEngine-assigned
  names, no fixed addresses, bounded cleanup roots, no broad Docker cleanup, admission ledger.

### Modified Capabilities

None; `openspec/specs/` is empty.

## Impact

New Go code only under `internal/harness/`; new scripts `scripts/test-integration.sh`,
`scripts/cleanup-roots-check.sh`, and the carried `scripts/lint-test-ports.sh` with its fixture test; new files
`.nats-image` and `docs/admission-ledger.yaml`; `Taskfile.yml`, `scripts/verify.sh`, `scripts/doctor.sh`,
`.gitignore`, and the CI workflow gain the new steps. `task verify` then needs a reachable Docker daemon.

[setup-02-rulings]: https://github.com/C360Studio/semengine/issues/6#issuecomment-5921046663
