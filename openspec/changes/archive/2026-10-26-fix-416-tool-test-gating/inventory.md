# Inventory: fix-416-tool-test-gating

base: 39a17a959216fe4f82564b89caf6df5ea671c889 (main; the PR branch adds only the empty claim commit 0eb2026)

Retires #416 (the one open `class:flake`). This carries no SemStreams pin package, so the shape sweep, the pin
probe and category 6 (pin ADRs) do not apply. Adopter seam inventory: not triggered. Every surface here is repo
tooling (`Taskfile.yml`, `scripts/`, CI) and nothing outside the repo calls it. Collision table: not triggered
(no durable, communication or runtime-coordination primitive).

## 1. Where the test invocations are spelled or pinned

- `Taskfile.yml:84` — `desc: Run unit tests once with the race detector at one CPU (the integration run covers every CPU)`
- `Taskfile.yml:86` — `scripts/gopkgs.sh go test -race -count=1 -cpu 1 ./...`
- `Taskfile.yml:93` — `'scripts/gopkgs.sh go test -count=5 -cpu 1 -shuffle=on {{.CLI_ARGS | default "./..."}}'`
- `Taskfile.yml:101` — `'go test -race -failfast -tags=integration -count=1 -p 2 -shuffle=on -timeout 10m
  -coverprofile=coverage/integration.coverprofile {{.CLI_ARGS | default "./..."}}'`
- `Taskfile.yml:65` — `go test -count=1 -run '^TestAdmissionLedger' ./internal/harness/contract/` (ledger:check,
  pinned by `TestLedgerCheckWiring`; runs one contract test whatever the gating does)
- `Taskfile.yml:112` — `scripts/cover-check.sh`; `scripts/cover-check.sh:71` — `go test -count=1
  -coverprofile="$unit" "${unit_packages[@]}"` (cover targets only; contract, mutcheck and pindiff are not among them)
- `scripts/verify.sh:12-13` — `steps=(spec:check docs:check ... ledger:check test:unit` / `test:integration
  cover:check test:repeat)`
- `scripts/gopkgs.sh:8` — `n=$(go list ./... | wc -l | tr -d ' ')`; `:14` — `exec "$@"` (counts packages, then passes
  its arguments through unchanged)
- `.github/workflows/ci.yml:30` — `uses: actions/checkout@3d3c42e...` with no `fetch-depth`, so the clone has depth 1;
  `:55` — `run: task verify`. Triggers: `:4-5` push to main, `:10-11` pull_request.
- `internal/harness/contract/mergegate_test.go:23-24` — `requiredUnitCmd` / `requiredRepeatCmd` constants (the two
  lines above, byte for byte); `:30-34` — `requiredIntegrationCmds` (the three test:integration lines);
  `:144` — `for _, step := range []string{"test:unit", "test:repeat"}` (verify steps); `:150` — test:repeat last.
- `TestCIWorkflowPinned` (`mergegate_test.go:174`) pins needs, permissions, timeouts and runner label. It does not pin
  checkout options or the verify job's steps beyond its existence.

Sentences that become untrue:

- `openspec/specs/merge-gate/spec.md:14-15` — "`task test:unit` SHALL run `scripts/gopkgs.sh go test -race -count=1
  -cpu 1 ./...`. `task test:repeat` SHALL run `scripts/gopkgs.sh go test -count=5 -cpu 1 -shuffle=on` over `./...`"
- `merge-gate/spec.md:36-38` — "every test without a build tag then runs once under the race detector at one CPU,
  once under the race detector on every CPU, and five times ..." (owner choice 3 removes the every-CPU run for every
  package without an integration file. Owner choices 1-2 remove all three runs for the tool tests when their paths
  are unchanged.)
- `openspec/specs/integration-test-runner/spec.md:11-12` — "... `<packages>`, with `./...` when no package is given."
- `integration-test-runner/spec.md:49` — "The task SHALL take no lock, set no environment variable ..." (stays true
  only if the package list is not handed over in an environment variable)
- `harness-boundaries/spec.md:751` ("both run by `task test:unit`"), `:862` ("`task test:unit` SHALL run a contract
  test (`TestAgentSetupParity` ...)"); `graph-transport-boundary/spec.md:36`, `:91`; `metric-registry/spec.md:90`
  ("A contract test run by `task test:unit` SHALL fail ..."). These are untrue whenever contract is gated.
- `merge-gate/spec.md:52` "Tests tagged `integration` SHALL NOT be repeated" stays true. So does
  `mutation-check/spec.md:6`.
- Docs and comments: `docs/testing.md:41` ("Runs every package with `-race`"), `:42`, `:44` ("Structural guards |
  `task test:unit`, `task verify`"); `docs/test-infrastructure.md:114` ("runs every package, integration tests
  included"); `.agents/README.md:138` ("run by `task test:unit`"); `internal/harness/contract/ledger_test.go:16`
  ("`task test:unit` runs it inside `task verify`"); `.agents/skills/semengine-preflight/SKILL.md:41-44`;
  `docs/admission-ledger.yaml:212-217` (the test:integration contract row; its `contract:` lists flags only).

## 2. Which packages are tool tests

- `mutcheck` and `pindiff`: nothing outside their own directories imports either (searched with
  `git grep -l 'internal/harness/<p>"'`). Both test only their own programs. Repo files they read:
  mutcheck reads `go.mod` and `go.sum` (`mutcheck/rapid_test.go:28-49`, the Rapid version) and
  `scripts/tree-state.sh` (`mutcheck/helpers_test.go:76`). pindiff's tests use temporary trees only.
- Fixtures that product tests import, which must keep running: natsfixture (9 product packages), lifecycletest (7),
  probe (9), payloadfixture (3), prochost (graph-ingest), semantictest (6). None of them is a tool test.
- **`internal/harness/contract` is not only tool tests (finding).** About half its files run a script on a temporary
  copy: mergecheck, specqueue, cover, portstatus, mergegate, ledgercheck, cleanuproots, addresses
  (lint-test-ports.sh). Most of the rest scan the whole tracked tree through `repoFiles` (`repo_test.go:37`) or
  `packages.Load`/`go list` over product code: testtext (no `time.Sleep`, no Skip, build tags, bare select,
  t.Parallel, own-server), imports, boundaries, signatures, context, authority, componentdeps, servicedeps,
  graphtransport, promglobal, reservedsubjects, imagepin, docker. Also: agentsetup (`AGENTS.md`, `.agents/`,
  `.claude/agents/`, `.claude/skills/`) and ledger (`docs/admission-ledger.yaml`).
  These guards are the `verify` enforcer of 25 rows in `.agents/rules-index.md`, each citing "(`task test:unit`)":
  lines 17 19-25 32 33 41-45 56-58 60 61 65 66 101 110 111. If contract runs only when `scripts/`,
  `internal/harness/`, `Taskfile.yml` or `.github/` change, a product PR that adds `time.Sleep` to a test, a
  `t.Skip`, or a production import of `internal/harness` passes `task verify` and CI.
- Measured here (the whole contract package, `-cpu 1`, no `-race`, otherwise idle machine; 45.7 s): the
  script-driving files took 35.6 s (mergecheck 16.2, specqueue 7.5, cover 6.1, portstatus 4.2, mergegate 0.9, the
  rest under 0.5). The tree guards took about 9.5 s (signatures 4.0, context 2.7, authority 1.1, the rest under 0.5).
- Repo inputs of the script-driving tests (trigger paths beyond the four named). `portstatus_test.go:26-31` copies
  `docs/admission-ledger.yaml`, `service/base.go`, `service/service_manager.go` and `service/component_manager.go`.
  `specqueue_test.go:472` and `ledgercheck_test.go` read `Taskfile.yml`. `copyScript` (`repo_test.go:155`) reads
  `scripts/`. All three packages build against `go.mod`/`go.sum`, and contract imports product packages
  (`go list -deps -test`: graph, message, payloadregistry, pkg/types, pkg/errs, vocabulary and others), so a
  product change can break contract's build.

## 3. Integration-tagged packages and what a narrower run loses

- The packages with `//go:build integration` test files (all 110 tagged files are `_test.go`; no production file
  carries a tag): component, config, graph/kvcatalog, internal/harness/natsfixture, internal/tlsutil, natsclient,
  natsclient/storagereport, pkg/lifecycle, processor/graph-ingest, service. Draft #414 adds graph/follower
  (`graph/follower/follower_integration_test.go`), so a hardcoded list would go stale on its merge.
- Each package's integration tests sit in its own directory. Coverage is measured without `-coverpkg`, so each
  profile covers only its own package, and nothing depends on one package's integration run covering another.
- TestMains: all are in integration-tagged files of those same packages (component, config, graph/kvcatalog,
  natsclient, natsclient/storagereport, pkg/lifecycle, processor/graph-ingest, service). The two under
  `mutcheck/testdata` are outside `./...`.
- cover:check (`scripts/cover-check.sh:23-49`): the `integration` target is natsfixture, which is tagged. The
  `merged` targets that are tagged: natsclient, graph-ingest, graph/kvcatalog, component, pkg/lifecycle,
  storagereport, config and service. `graph merged` (`:32`) has no tagged file. Without it, its integration profile
  is just a duplicate of its unit tests, and `percent()` (`:87-104`) still finds its blocks in the unit profile, so no
  failure and no lost coverage. Packages in the list still re-run their untagged tests under `-tags`, which go test
  cannot avoid.
- `coverage/integration.tree` (`Taskfile.yml:100,102`, `scripts/tree-state.sh`) depends on the tree, not on the
  package list, so it is unaffected.

## 4. Computing the changed files

- No existing script computes a git diff base. `scripts/merge-check.sh:120` lists PR files through `gh api`, which
  needs a token and a PR number (verify has neither). It reads `GITHUB_ACTIONS` and `GITHUB_EVENT_NAME` at `:35-45`.
- Locally: `git merge-base HEAD origin/main`, then `git diff --name-only <base>` (committed plus uncommitted tracked
  changes), plus `git ls-files --others --exclude-standard` for untracked files. A stale `origin/main` can only widen
  the set. On a local main checkout the set is empty, so the tool tests are skipped.
- CI pull_request: the checkout is the merge commit at depth 1, so neither `HEAD^1` nor the base is present.
  `GITHUB_BASE_REF` and `GITHUB_EVENT_NAME` are default Actions variables. CI push to main: run everything.
- `scripts/verify.sh:18,34` compares tracked state before and after. A read-only diff writes nothing, so the rule is
  unaffected.

## 5. Overlapping open PRs and issues

- #413 (draft, head 6911630): composition/, service/ and its own openspec change. It touches no gated file. It does
  touch `service/component_manager.go`, which TestPortStatusCheck copies. Plan: #419 lands first.
- #414 (draft, head 5c49a2f): graph/ and composition/. It adds an integration-tagged package (§3) and no cover
  target. No file conflict.
- Issues: #416 is retired (`Closes`; owner choice 2 removes contract from test:repeat). #129 (mutate:check and
  integration tests), #161, #162 and #80 (proposed new contract guards; they would inherit contract's gating) stay.
  `gh issue list --label class:flake`: #416 only.

## 6. Rule rows whose wording changes (AGENTS.md row = rules-index row, 1-to-1)

- `AGENTS.md:114` / `rules-index.md:59` — "Unit tests also run five times at one CPU, without `-race`, shuffled"
- `AGENTS.md:116` / `rules-index.md:61` — "Integration tests run under plain `go test -tags integration`;
  `task test:integration` only pins flags ..."
- If contract is gated: the enforcer column of the 25 rules-index rows in §2, and each matching AGENTS.md
  `verify`/`verify+review` gate cell (35 rows carry one).
- AGENTS.md commands block `:45-51`: add `task test:tools`. Row counts are checked by `TestAgentSetupParity`
  (in contract), so an AGENTS.md-only edit must trigger contract or that check does not run.

## Open questions

- Owner: owner choice 1 names `internal/harness/contract`, but most of its files guard product code (§2). Does "tool
  tests" mean the whole package or only its tests that run a script? This decides whether 25 `verify` rules keep
  running on product PRs.
- Developer: how CI obtains the base (checkout `fetch-depth`, or fetching `GITHUB_BASE_REF`), and whether the
  verify job's token needs anything beyond `contents: read`.
- Developer: how test:integration finds its package list (`go list` `IgnoredGoFiles` without the tag, or grep for
  the build line), while keeping `-- <pkgs>` and integration-test-runner `:49` (no environment variable).
- Developer: the trigger list must also include `go.mod`, `go.sum`, `docs/admission-ledger.yaml` and the three
  `service/` files that TestPortStatusCheck copies. If contract stays whole, `AGENTS.md`, `.agents/` and `.claude/`
  too. State any path left out, and why.
