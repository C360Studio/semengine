# harness-boundaries

## MODIFIED Requirements

### Requirement: ateam doctor gates verify

`scripts/verify.sh` SHALL run `task ateam:doctor` as its first step, and `task ateam:doctor` SHALL run
`ateam doctor --check` with the session identity variables (`ATEAM_SESSION_ID`, `CLAUDE_SESSION_ID`,
`CODEX_SESSION_ID`, `GEMINI_SESSION_ID`) unset, so that the check never takes the writer lease. The ateam release is
pinned as `v0.3.4` in `.github/workflows/ci.yml` and `.github/workflows/merge-check.yml`, every install line naming
the same version, compared by `scripts/doctor.sh`.

#### Scenario: Doctor violation fails verify

- **WHEN** `ateam doctor --check` reports a violation (a missing skill adapter, a drifted base contract, a protocol
  marker missing, `AGENTS.md` at 24,000 bytes or more, a rule-row count that differs between `AGENTS.md` and
  `.agents/rules-index.md`, or session-start hook latency above its 100 ms ceiling)
- **THEN** `task verify` fails at the `ateam:doctor` step naming the file and the rule

#### Scenario: Doctor warning does not fail verify

- **WHEN** `ateam doctor --check` reports only a warning (hook latency above the 10 ms target but below 100 ms, or a
  shared forge token)
- **THEN** `task verify` continues
