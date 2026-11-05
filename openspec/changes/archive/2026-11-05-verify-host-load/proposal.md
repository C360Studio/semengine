# Proposal: verify-host-load

Track: quick (one scenario on an existing `merge-gate` requirement; no new Requirement, one capability).

## Why

The three flakes filed on 2026-11-04 (#513, #514, #518) all fired on the Mac that runs every local `task verify`, under
sixteen stray `yes` processes that had saturated all 12 cores for 34 hours (one-minute load average 37–52). Each was a
timing budget expiring: a 60 s cleanup, a 40 s fixture seed, a 20 s stream probe. None fired in CI. From 2026-10-28 to
2026-11-02 the local integration step took 168–272 s on 28 runs and every one passed; on 2026-11-04 it took 346–477 s
and three of four runs failed. The load number was one `uptime` away and none of the three filings read it; the merge
gate then stopped every pull request on each. Owner's ask, 2026-11-04 (#519).

Approved-by: Coby Leuschke, 2026-11-04, in the Enjoy conversation "Flake pattern diagnosis" (#519 records the ask).

## What Changes

- `scripts/verify.sh` prints a `host:` line (core count, load averages) at the start of the run and in its step-timings
  block, so every verify log a flake filing quotes carries the host state. A one-minute load average above twice the
  core count adds a warning line naming the protocol item. The run is not refused: concurrent sessions kept load above
  the core count through Oct 28 – Nov 2 with 28 green runs, so any refusal threshold named today would be a guess.
- `.agents/protocol.md` "Known flakes": a local red whose verify log shows that load is a host-load suspect first;
  rerun on an idle host before filing, and the issue quotes the `host:` lines either way. The preflight skill's flake
  paragraph points at it.
- `openspec/specs/merge-gate/spec.md`: the flake-label requirement gains the scenario. Review-only, like the
  requirement's existing scenario; no new rule row (`AGENTS.md`, `.agents/rules-index.md` keep the one flake row,
  which is the merge gate, not filing).

## Capabilities

- Modified `merge-gate` (one requirement changed: one scenario added).

## Impact

- `scripts/verify.sh`, `.agents/protocol.md`, `.agents/skills/preflight/SKILL.md`, `openspec/specs/merge-gate/spec.md`.
  No Go line changes; the toolcheck guard reads only the `steps=(...)` list, which is unchanged.
- Closes #519.
