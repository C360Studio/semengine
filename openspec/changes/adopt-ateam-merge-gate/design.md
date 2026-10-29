# Design: adopt-ateam-merge-gate

## Context

- `scripts/merge-check.sh:1-270`: three rules (ruleset strict with `Required`; non-draft code PR needs
  `implemented-by:`/`reviewed-by:` naming different agents; no open `class:flake` unless closed by the PR), exit 2 on
  an unavailable read. `ateam/internal/review/gate.go:20-40` at v0.3.4 states the same three, plus the record and
  strength checks; `gate.go:600-605` carries the same documents-only classifier (`.md`, or `.yml`/`.yaml`/`.json`
  under `openspec/`, never `.claude/agents/` or `.agents/contracts/`).
- `.github/workflows/ci.yml` `merge-check` job: checks out the PR head, runs the script, `required` needs it.
  `ateam/.github/workflows/merge-check.yml` at v0.3.4: own workflow, `edited` trigger, `cancel-in-progress: false`,
  draft skipped, base-commit checkout, binary built from base, `ATEAM_REQUIRED_CHECKS: Required,Merge check`.
- Ruleset on `main` today (read 2026-10-29): strict, required checks `["Required"]` only.
- `internal/harness/toolcheck/mergecheck_test.go` (662 lines: `TestMergeCheckKnownFlake`, `...UpToDateRule`,
  `...Review`) and `mergegate_test.go` (749 lines: `TestUnitInvocationsPinned`, `TestCIWorkflowPinned`, each with a
  sensitivity test).
- `.agents/contracts/semengine-reviewer.md:520` ends a review with `APPROVE` / `CHANGES REQUESTED`; ateam's
  `reviewer.md` § Record uses `Verdict: APPROVE | REQUEST CHANGES` and `Reviewed at <sha>`.
- `.agents/review-policy.json` (PR A): tiers and bands, `cross_provider: true`. Once it exists the gate requires
  `(model, effort)` on both attestation lines; PR bodies here already carry that form.
- ateam v0.3.3 → v0.3.4 template changes (`internal/bootstrap/templates.go`): `developer.md`, `reviewer.md`,
  `review-pr` skill; `TestAteamTemplatesUnchanged` compares them byte for byte against the pinned binary.

## Goals / Non-Goals

Goals: the gate is ateam's at the v0.3.4 pin; the gate judges from the base commit; the record grammar has one home.
Non-goals: enabling ateam's dormant `routes` / `ateam review check` path; the spec queue (PR C, #450); the protocol
rulings (PR D, #451); any change to what counts as a code PR.

## Decisions

**D1. Where the merge check runs.** Options: (a) keep the job inside `ci.yml` with `Required` needing it, swap only
the script; (b) its own workflow as ateam runs it. (a) is the smaller diff and keeps the ruleset untouched, but a
reviewer's description edit starts no run, so every PR needs a manual rerun, and a cancelled CI run leaves a red
`Merge check` beside the green one at the same head (ateam #87). (b) costs one ruleset edit by the owner and a second
required check, and gives the `edited` trigger, queued runs, and a draft that carries no `Merge check` run at all.
Decision: (b), matching ateam; the gate's own `ATEAM_REQUIRED_CHECKS` then proves the ruleset holds both checks on
every run. Adopter seam: a developer who never reads this sees `Merge check` red once after marking ready and green
after the reviewer's record and description edit, with no rerun step.

**D2. Where the binary comes from.** Options: build from the checked-out base (ateam's own repo) or
`go install github.com/c360studio/ateam/cmd/ateam@<pin>` (an adopter has no ateam source). Decision: `go install`
at the pin written in the workflow file; the pin appears in `ci.yml` (verify job) and `merge-check.yml`, and the
"all install lines name one version" rule from PR A (`scripts/doctor.sh`, `TestAteamTemplatesUnchanged`) extends
to both files.

**D3. What the spec keeps.** The script's mechanics requirements (run kind, printing, fail-closed reads, the
changed-file read, finding text) are removed: the pin's own tests (`gate_test.go`, `compare_test.go`,
`record_test.go`) are the oracle, as semteams ruled in its #367. What stays as SemEngine's own requirements: the
three rules as the gate checks them, the documents-only classifier (restated as the gate's), the workflow shape
(D1, D2, read-only token, 5-minute limit), `Required` needing `verify`, and the contract tests that guard both
workflow files. The flake-label policy requirement is unchanged.

**D4. Record grammar.** `semengine-reviewer.md` stops spelling its own final verdict words and points at ateam's
`reviewer.md` § Record; its finding format (BLOCKING/HIGH/MEDIUM/NIT, `file:line`, minimal correction) stays. The
protocol's "Cross-agent review" item is rewritten to the gate: implementer posts a `Review request` comment; the
reviewing agent posts one record comment with `reviewed-by: <agent> (<model>, <effort>)`, `Reviewed at <sha>`,
`Verdict:`; a later code commit needs a new record, a clean merge of `main` or a docs-only commit carries it over.

**D5. Draft handling.** The gate does not fail a draft and the workflow skips drafts, so `#437`-style drafts post
their record when they go ready. No transition period: the rule applies to every pull request not yet merged. Open
PRs at merge time include Dependabot's #464 and #465; the gate skips only `implemented-by:` for a bot author
(`gate.go` v0.3.4, lines 406-417), so each still needs a `reviewed-by:` line with its model and an approving record in
ateam's grammar (lines 284 and 428-440) before it merges.

## Risks / Trade-offs

- The ruleset edit is outside the repository. Until `Merge check` is required, the gate's rule 1 fails on every PR
  judged by `ateam review gate`, which is every PR after this one merges. This PR is judged by `main`'s old script
  (the workflow checks out the base commit), which checks `Required` alone, so its own runs cannot show rule 1;
  sequencing is in the Migration Plan.
- Local `task verify` on a host whose `ateam` on PATH is a development build fails `TestAteamTemplatesUnchanged`
  (version mismatch). Mitigation: install the pin with `go install` and put `$(go env GOPATH)/bin` first on PATH
  for the run; stated in `docs/testing.md`.
- Effort now matters: a `reviewed-by:` whose effort band is below the implementer's fails. The role adapters pin
  reviewer `high`, so the default routing passes.

## Migration Plan

1. Land this PR's commits; its own `merge-check.yml` runs from the PR (pull_request events use the PR's workflow
   file): skipped on the draft, and once ready it runs `main`'s old `scripts/merge-check.sh` from the base commit.
2. Owner adds `Merge check` to the `main` ruleset's required checks (strict stays on). The ruleset read
   (`gh api repos/{owner}/{repo}/rules/branches/main`) lists both checks, and the v0.3.4 gate run locally with
   `ATEAM_REQUIRED_CHECKS='Required,Merge check'` passes rule 1.
3. Codex record, archive, ready, merge. From then on `Required` and `Merge check` are both required on every PR.

## Open Questions

- None for the owner beyond approving D1 and the ruleset edit it needs.
