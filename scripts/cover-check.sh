#!/usr/bin/env bash
# Enforce 80% statement coverage on the three harness packages every later proof
# rests on (owner ruling Q3, 2026-09-30): natsfixture from the integration profile,
# because its owner tests need Docker, and lifecycletest and probe from a unit
# profile. A package missing from its profile fails: no statements is not coverage.
# Usage: cover-check.sh [unit-profile integration-profile]
#   With no arguments it measures the unit profile itself and reads the integration
#   profile of the last `task test:integration` run in this worktree, which must have
#   passed and must have run against the tree as it is now.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

readonly threshold=80
readonly base=github.com/c360studio/semengine/internal/harness

if [ $# -eq 2 ]; then
  unit=$1 integration=$2
else
  mkdir -p coverage
  unit=coverage/unit.coverprofile
  # Its output is printed: a test that fails here must be named, not discarded.
  go test -count=1 -coverprofile="$unit" "./internal/harness/lifecycletest/" "./internal/harness/probe/"
  if [ ! -f .evidence/last-run ]; then
    echo "cover: no integration run recorded in this worktree; run task test:integration first" >&2
    exit 1
  fi
  run=$(cat .evidence/last-run)
  integration="$run/integration.coverprofile"
  if ! grep -qx 'go_test_status=0' "$run/runner.env" 2>/dev/null; then
    echo "cover: the last integration run ($run) did not pass; its profile is not evidence" >&2
    exit 1
  fi
  # The runner recorded the same fingerprint; a stale profile is refused, not trusted.
  if ! grep -qx "tree_state=$(scripts/tree-state.sh)" "$run/runner.env"; then
    echo "cover: the last integration run ($run) measured a different tree; run task test:integration again" >&2
    exit 1
  fi
fi

# Percent of statements covered in one package. A block (file:range) appearing in
# several package runs of a merged profile counts once, covered if any run covered it.
percent() { # package profile
  awk -v pkg="$1" '
    NR == 1 && /^mode:/ { next }
    {
      split($1, loc, ":"); file = loc[1]
      dir = file; sub(/\/[^\/]*$/, "", dir)
      if (dir != pkg) next
      stmts[$1] = $2
      if ($3 > 0) hit[$1] = 1
    }
    END {
      for (b in stmts) { total += stmts[b]; if (b in hit) covered += stmts[b] }
      if (total == 0) { print "none"; exit }
      printf "%.1f\n", 100 * covered / total
    }' "$2"
}

fail=0
check() { # short-name profile label
  local pct
  pct=$(percent "$base/$1" "$2")
  if [ "$pct" = none ]; then
    echo "cover: $1: no statements in the $3 profile ($2)"
    fail=1
  elif awk -v p="$pct" -v t="$threshold" 'BEGIN { exit !(p < t) }'; then
    echo "cover: $1 $pct% < $threshold% ($3 profile)"
    fail=1
  else
    echo "cover: $1 $pct% ($3 profile)"
  fi
}
check lifecycletest "$unit" unit
check probe "$unit" unit
check natsfixture "$integration" integration

if [ "$fail" -ne 0 ]; then echo "cover: FAILED (threshold $threshold%)"; exit 1; fi
echo "cover: ok (threshold $threshold%)"
