# Tasks: setup-02-isolated-harness

Each task names the outcome that proves it. Pass-evidence tasks (group 6) are recorded on PR #13 with the evidence
directories attached as artifacts, not committed.

## 1. Pins, ledger, boundaries

- [ ] 1.1 `.nats-image` holds the one digest pin with the tag as a comment; `task doctor` reports, read-only, whether
      the NATS image is cached by digest, whether the ryuk image is cached, the lock owner, the effective Docker host
      and context, the Ryuk env, and whether `~/.testcontainers.properties` is present
- [ ] 1.2 `go.mod` requires `nats.go`, `testcontainers-go`, and `gopkg.in/yaml.v3`; `task tidy:check` and `task vuln`
      pass; govulncheck findings are recorded on PR #13
- [x] 1.3 `docs/admission-ledger.yaml` holds the 12 drafted entries; the T-B7 schema test is shown failing first
      against a short-SHA fixture and then passes against the ledger; `task ledger:check` runs it
- [x] 1.4 Contract tests T-B1..T-B6 are shown failing first, then pass; T-B5 includes the adversarial `Name()` test
      and fails on a destroyer substring inside a longer word (`stops`, `loops`), not only on a whole segment; T-B6
      runs `scripts/lint-test-ports.sh`
- [x] 1.5 `scripts/lint-test-ports.sh` and its fixture test are carried byte-identical to the pinned SHA; `task lint`
      and CI run both and pass
- [x] 1.6 `scripts/cleanup-roots-check.sh` runs as a `task verify` step and fails on a seeded `defer
      o.Stop(context.Background())`; `.gitignore` ignores `.evidence/`

## 2. Probes and suite (unit lane)

- [ ] 2.1 `internal/harness/probe` provides `Callback`, `ObservedContext`, and `Await`; a test shows `Await`'s failure
      message carries the last observation and error
- [ ] 2.2 `internal/harness/lifecycletest` provides the checks, `Run`, and the `refowner` failpoint double; one
      sensitivity test per failpoint trips exactly its check, and every check passes against the clean double

## 3. Runner

- [ ] 3.1 `scripts/test-integration.sh` implements the byte-compatible lock, reads only `SEMENGINE_*` env, owns the
      process group with signal forwarding, runs preflight, writes the evidence dir, uses the canonical argv, checks
      the image cache by digest with a bounded pull, checks leaks by session (IDs only), and releases the lock only
      with a matching token
- [ ] 3.2 `internal/harness/runner` tests R1–R4 run with a fake toolchain and temp lock dir, are shown failing first,
      then pass; R4 pins the six owner keys against the recorded SemStreams format

## 4. NATS fixture (integration lane)

- [ ] 4.1 `natsfixture` implements the admission check (S1-9 written and passing first), the start phases, the typed
      error, evidence records, and the `deps` seam
- [ ] 4.2 `Name()` applies the `semengine-` prefix, sanitizer, and hex suffix; the fixture records owned resources;
      `CreateStream` and `CreateKeyValue` set the bounds; unit tests cover each
- [ ] 4.3 Checked `Stop` is implemented; owner tests S1-1..S1-8 pass under `-race`; `lifecycletest.Run` passes over
      the fixture adapter with `Promise{Restart:false}`

## 5. Gates

- [ ] 5.1 `task test:integration` exists; `scripts/verify.sh` runs it after `test:unit`; the CI workflow uploads
      `.evidence` as an artifact with a retention period
- [ ] 5.2 `task cover:check` fails below 80% on `natsfixture` (integration profile), `lifecycletest`, and `probe`
      (unit profile), and passes on the branch
- [ ] 5.3 Local and CI `task verify` durations with the new steps (E2) and the observed dynamic host-port range (E4)
      are recorded on PR #13

## 6. Pass evidence (recorded on PR #13, artifacts attached)

- [ ] 6.1 Two-worktree run with a wait budget: both green, the second waited on the first's owner line, identities
      disjoint
- [ ] 6.2 Interrupt protocol: signal and group timings recorded, before/after listings identical outside the
      interrupted run's session label, the other worktree completes
- [ ] 6.3 Forced failure at script and Go level: bounded pull failure, lock released, nothing left by session, phase
      records and logs captured
- [ ] 6.4 Persistent data survives, both directions: KV entries round-trip unchanged, and a read-only evaluation of
      every SemStreams destroyer filter lists no SemEngine resource
- [ ] 6.5 `docs/repository-map.md`, the `docs/provenance.md` status, and the preflight skill gate table describe the
      implemented harness; the change is archived with its specs synced in the last commit
