#!/usr/bin/env bash
# Enforce 80% statement coverage on the packages listed in targets below: the three
# harness packages every later proof rests on (owner ruling Q3, 2026-09-30), and each
# package on the critical list (SETUP 03B design D10) once it is ported. A package is
# measured from the unit profile, the integration profile (natsfixture: its owner tests
# need Docker), or both merged (natsclient: its unit and integration lanes test
# different code). A package missing from its profile fails: no statements is not coverage.
# Usage: cover-check.sh [unit-profile integration-profile]
#   With no arguments it measures the unit profile itself and reads the integration
#   profile of the last `task test:integration` run in this worktree, which must have
#   passed and must have run against the tree as it is now.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

readonly threshold=80
readonly module=github.com/c360studio/semengine

# The coverage targets, "<package directory> <profile>", profile one of unit, integration
# or merged. A change that ports a package on the critical list adds its line here.
readonly targets=(
  "internal/harness/lifecycletest unit"
  "internal/harness/probe unit"
  "internal/harness/natsfixture integration"
  "message unit"
  "payloadregistry unit"
  "natsclient merged"
)

# The unit profile's packages: every target measured from it, alone or merged.
unit_packages=()
for target in "${targets[@]}"; do
  read -r dir kind <<<"$target"
  case "$kind" in
  unit | merged) unit_packages+=("./$dir/") ;;
  integration) ;;
  *)
    echo "cover: target $dir: unknown profile $kind" >&2
    exit 1
    ;;
  esac
done

if [ $# -eq 2 ]; then
  unit=$1 integration=$2
else
  mkdir -p coverage
  unit=coverage/unit.coverprofile
  # Its output is printed: a test that fails here must be named, not discarded.
  go test -count=1 -coverprofile="$unit" "${unit_packages[@]}"
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

# Percent of statements covered in one package, over one or more profiles. A block
# (file:range) appearing in several package runs or profiles counts once, covered if
# any run covered it.
percent() { # package profile...
  local pkg=$1
  shift
  awk -v pkg="$pkg" '
    FNR == 1 && /^mode:/ { next }
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
    }' "$@"
}

fail=0
check() { # package-directory label profile...
  local name=$1 label=$2 pct
  shift 2
  pct=$(percent "$module/$name" "$@")
  name=${name#internal/harness/}
  if [ "$pct" = none ]; then
    echo "cover: $name: no statements in the $label profile ($*)"
    fail=1
  elif awk -v p="$pct" -v t="$threshold" 'BEGIN { exit !(p < t) }'; then
    echo "cover: $name $pct% < $threshold% ($label profile)"
    fail=1
  else
    echo "cover: $name $pct% ($label profile)"
  fi
}
for target in "${targets[@]}"; do
  read -r dir kind <<<"$target"
  case "$kind" in
  unit) check "$dir" unit "$unit" ;;
  integration) check "$dir" integration "$integration" ;;
  merged) check "$dir" "merged unit and integration" "$unit" "$integration" ;;
  esac
done

if [ "$fail" -ne 0 ]; then echo "cover: FAILED (threshold $threshold%)"; exit 1; fi
echo "cover: ok (threshold $threshold%)"
