#!/usr/bin/env bash
# Run every gate cheapest first, report wall time per step, then require that
# verification left tracked files unchanged. Keeps going after a failure so one
# run reports every broken gate.
set -uo pipefail
cd "$(dirname "$0")/.."

# Order is the contract: cheapest first, ateam:doctor first of all (harness-boundaries spec,
# "ateam doctor gates verify"). test:integration needs a reachable Docker
# daemon; cover:check reads the profile it just wrote.
# test:tools:changed runs the tool tests only when their files changed, after test:unit.
# test:repeat is the most expensive step and runs last (merge-gate spec, pinned by
# toolcheck's TestUnitInvocationsPinned).
steps=(ateam:doctor spec:check docs:check fmt:check tidy:check cleanup-roots:check build vet lint vuln ledger:check
  test:unit test:tools:changed test:integration cover:check test:repeat)

# Tracked-file state before the run. Comparing before/after (not just "is dirty")
# keeps verify usable on a working tree with edits in progress; on a clean
# checkout, as in CI, it is identical to requiring a clean tree.
tracked_state() { git status --porcelain --untracked-files=no; git diff HEAD | shasum; }
before=$(tracked_state)

# Host state, printed at the start and beside the timings: a red in a run whose one-minute load
# average is above twice the core count is a host-load suspect before it is a flake
# (.agents/protocol.md, "Known flakes"; the three 2026-11-04 flakes fired at load 37-52 on 12 cores).
# macOS prints "load averages: a b c", Linux "load average: a, b, c".
host_state() {
  local cores load load1
  cores=$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 0)
  load=$(uptime | sed -E 's/.*load averages?: *//; s/,//g')
  load1=${load%% *}
  echo "host: $cores cores, load average $load"
  if [ "$cores" -gt 0 ] && [ "${load1%%.*}" -ge $((cores * 2)) ]; then
    echo "host: WARNING load average $load1 is at least twice the $cores cores; a red here is a host-load suspect first (.agents/protocol.md, \"Known flakes\")"
  fi
}
host_state

failed=()
timings=()
for s in "${steps[@]}"; do
  echo "==> task $s"
  start=$(date +%s)
  if ! task "$s"; then failed+=("$s"); fi
  timings+=("$(printf '%-20s %3ds' "$s" $(($(date +%s) - start)))")
done

echo
echo "step timings:"
printf '  %s\n' "${timings[@]}"
host_state

if [ "$(tracked_state)" != "$before" ]; then
  echo "verify: tracked files changed during verification:"
  git status --porcelain --untracked-files=no
  failed+=("tree-unchanged")
fi
untracked=$(git status --porcelain | grep '^??' || true)
[ -z "$untracked" ] || { echo "untracked files (reported, allowed):"; echo "$untracked" | sed 's/^/  /'; }

if [ ${#failed[@]} -ne 0 ]; then
  echo "verify: FAILED: ${failed[*]}"
  exit 1
fi
echo "verify: ok"
