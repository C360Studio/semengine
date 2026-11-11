# Inventory: fix-599-ci-split

base: 2b803166f019fe7d5ccc2fa6d91fc1e06b5cf552

Problem: CI's one `Verify` job runs all of `task verify` inside 15 minutes and was cancelled at the limit on main run
43103302538 after `verify: ok` (#599). Owner ruling: #599 comment 6807839061.

## 1. The claimed gap (a way for a CI job to run part of verify)

- `scripts/verify.sh:14` — `steps=(ateam:doctor spec:check docs:check fmt:check tidy:check cleanup-roots:check build vet lint vuln ledger:check`
- `scripts/verify.sh:41` — `for s in "${steps[@]}"; do` (no argument is read; `git grep -n '\$1' scripts/verify.sh` finds none)
- `Taskfile.yml:195` — `- scripts/verify.sh` (no `CLI_ARGS`)
- `.github/workflows/ci.yml:65` — `- run: task verify`

No group selection exists today.

## 2. Every current spelling of the facts touched

Step list: `scripts/verify.sh:14-15`, read by `internal/harness/toolcheck/mergegate_test.go:217` —
`m := verifySteps.FindSubmatch(verify)`; the preflight table describes it (`.agents/skills/preflight/SKILL.md:49`).
CI job and Required's needs:
- `.github/workflows/ci.yml:28` — `timeout-minutes: 15`
- `.github/workflows/ci.yml:71` — `needs: [verify]`
- `internal/harness/toolcheck/mergegate_test.go:251` — `requiredNeeds         = []string{"verify"}`
- `internal/harness/toolcheck/mergegate_test.go:262` — `verifyTimeoutMinutes     = 15`
- `openspec/specs/merge-gate/spec.md:51` — `- **THEN** go test ends it at five minutes and prints every goroutine's stack, inside CI's 15-minute job; the bound is`
- `openspec/specs/merge-gate/spec.md:120` — `The job \`verify\` SHALL have a limit of 15 minutes.`
- `openspec/specs/merge-gate/spec.md:136` — `- **WHEN** a job in \`.github/workflows/ci.yml\` has \`runs-on: ubuntu-latest\`, whether it is \`verify\`, \`merge-check\`,`
- `openspec/specs/merge-gate/spec.md:252` — `SHALL be the merge base with \`origin/$GITHUB_BASE_REF\`, and CI's verify job SHALL check out with \`fetch-depth: 0\`.`
- `openspec/specs/merge-gate/spec.md:349` — `The job \`required\` in \`.github/workflows/ci.yml\` SHALL need \`verify\` and SHALL fail unless it succeeded, running`
- `openspec/specs/merge-gate/spec.md:366` — `A contract test SHALL fail when \`ci.yml\` differs from "Required needs verify" or "The verify job has a time limit",`
- `AGENTS.md:53` — ``Run `task verify` before every implementation push. CI: `ci.yml` runs `verify` and `Required` fails unless it``
- `.agents/protocol.md:286` — ``Two workflows report the checks the ruleset on `main` requires. In `ci.yml`, `verify` runs `task verify`, the same``
- `.agents/rules-index.md:109` — `` | A merge needs `Required` and `Merge check` green | ... `merge-gate` spec, "Required needs verify" | ``
- `.agents/skills/preflight/SKILL.md:51` — ``CI has two workflows: in `ci.yml`, `verify` runs `task verify` and `required` fails if `verify` failed, is missing, or``
- `docs/repository-map.md:61` — `` | `.github/` | Two workflows, `ci.yml` (jobs `verify` and `required`) and ... ``
- `scripts/tools-changed.sh:11` — `#     with origin/$GITHUB_BASE_REF (the verify job checks out with fetch-depth: 0).`
Unit profile for `cover:check`: `scripts/cover-check.sh:89` — `go test -count=1 -coverprofile="$unit" "${unit_packages[@]}"`;
`Taskfile.yml:110` — `- go test -race -count=1 -cpu 1 -timeout 5m {{.PKGS}}` (no profile).
Integration fingerprint: `Taskfile.yml:133` — `- scripts/tree-state.sh > coverage/integration.tree`; read at `scripts/cover-check.sh:96`.
Host line and tracked-files check: `scripts/verify.sh:37`, `:51`, `:53`.
`ateam:doctor` first: `openspec/specs/harness-boundaries/spec.md:1069` — ``\`scripts/verify.sh\` SHALL run \`task ateam:doctor\` first, ...``.
Ateam install lines: `scripts/doctor.sh:44-47` greps every install line in `ci.yml`; two identical lines pass.
Searches: `git grep -n -i -E '15-minute|15 minutes|timeout-minutes|job verify|verify job|Required needs|needs: \[verify\]|name: Verify' -- openspec/specs .github docs AGENTS.md .agents scripts Taskfile.yml internal/harness`;
`git grep -n -E 'ci\.yml|Required' -- .claude .codex .gemini` (none).

## 3. Adjacent claims

Ruleset `main-required-checks` requires `["Required","Merge check"]` (`gh api repos/C360Studio/semengine/rulesets/<id>`).
Open PRs: #597, #598 (Dependabot, `go.mod`/`go.sum` only); none touches the files above
(`gh pr list --state open --json number,files`). In-flight changes: none (`ls openspec/changes` shows `archive` only).
Open issues: #599; #576 (merge-check re-review after a merge of main) names `ci.yml`, not the verify job, and is
untouched (`gh issue list --state open --search "ci.yml"`, `"verify job"`).

## 4. Who it is for

New surface: the `checks`/`integration` argument of `scripts/verify.sh` (callers: the two `ci.yml` jobs) and the job
ids `checks`, `integration` (caller: `required`'s `needs`). No exported Go symbol, subject, bucket or config field.

## 5. Problem shape

Select a subset of one ordered list by a name. Closest instance: `Taskfile.yml:121` — `{{.CLI_ARGS | default .PKGS}}`
(`test:repeat` and `test:integration` take a narrower set after `--`). Adopted: the group arrives after `--`.

## 6. Pin decision records

None: the change carries no package from the pin.

Collision table: not triggered (no durable, communication or runtime-coordination primitive). Intent check: no
boundary moves. Adopter seam: no surface reached from outside this repository.
