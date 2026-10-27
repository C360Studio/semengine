#!/usr/bin/env bash
# Print one package list on one line, separated by spaces, each package as ./<dir>
# (openspec change fix-416-tool-test-gating; specs merge-gate, "Tool tests and
# repository guards are separate packages", and integration-test-runner, "Only
# packages with integration tests"). Taskfile.yml reads it into each test task's PKGS.
#
#   tools        the tool packages; this script is the only place they are named.
#   unit         every package go list ./... names, except the tool packages.
#   repeat       the unit list, except the repository guards (internal/harness/contract).
#   integration  the directory of every tracked or untracked, unignored _test.go file
#                outside testdata with a line that, trimmed, is //go:build integration.
#                go list cannot find these: without the tag it leaves out a directory
#                whose only Go files are integration tests, and still exits 0.
#
# An empty list or a failing command exits non-zero, so the task fails before go test.
# Usage: test-pkgs.sh unit|repeat|integration|tools
set -euo pipefail
cd "$(dirname "$0")/.."

readonly tools=(./internal/harness/mutcheck ./internal/harness/pindiff ./internal/harness/toolcheck)
# The repository guards read the tree, not timing, so one run per verify is enough (owner, PR #419).
readonly guards=./internal/harness/contract

case "${1:-}" in
  tools)
    pkgs=$(printf '%s\n' "${tools[@]}")
    ;;
  unit | repeat)
    skip=("${tools[@]}")
    if [ "$1" = repeat ]; then
      skip+=("$guards")
    fi
    module=$(go list -m)
    all=$(go list ./...)
    pkgs=$(while IFS= read -r p; do
      dir=".${p#"$module"}"
      if [[ " ${skip[*]} " != *" $dir "* ]]; then
        echo "$dir"
      fi
    done <<<"$all")
    ;;
  integration)
    if ! files=$(git grep --untracked -l '^[[:space:]]*//go:build integration[[:space:]]*$' \
      -- '*_test.go' ':(exclude,glob)**/testdata/**'); then
      echo "test-pkgs.sh: git grep found no integration-tagged test file, or failed" >&2
      exit 1
    fi
    pkgs=$(while IFS= read -r f; do
      if [[ "$f" == */* ]]; then
        echo "./${f%/*}"
      else
        echo .
      fi
    done <<<"$files")
    ;;
  *)
    echo "usage: test-pkgs.sh unit|repeat|integration|tools" >&2
    exit 2
    ;;
esac

if [ -z "$pkgs" ]; then
  echo "test-pkgs.sh: the $1 list is empty" >&2
  exit 1
fi
printf '%s\n' "$pkgs" | LC_ALL=C sort -u | paste -sd ' ' -
