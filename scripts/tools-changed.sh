#!/usr/bin/env bash
# Run task test:tools when a changed file is under a trigger path, else say the tool
# tests were skipped and exit 0 (openspec change fix-416-tool-test-gating; spec
# merge-gate, "Tool tests run when their files change" and "How the changed files are
# found"). The trigger paths are the real-tree files the tool tests read
# (internal/harness/toolcheck/doc.go), plus go.mod and go.sum, which can break their build.
#
#   Outside GitHub Actions: the files git diff reports against the merge base of HEAD
#     and origin/main, plus untracked files that git does not ignore.
#   In Actions, on a pull_request: the files git diff reports against the merge base
#     with origin/$GITHUB_BASE_REF (the verify job checks out with fetch-depth: 0).
#   In Actions, on a push or any other event: every tool test runs.
#
# Diffs use --no-renames, so a file moved out of a trigger path still counts. A missing
# merge base or a failing git command runs every tool test and says why.
# Usage: tools-changed.sh
set -euo pipefail
cd "$(dirname "$0")/.."

readonly triggers=(scripts/ internal/harness/ .github/ Taskfile.yml go.mod go.sum
  docs/admission-ledger.yaml service/base.go service/service_manager.go service/component_manager.go
  .agents/ .claude/ .codex/ .gemini/ .mcp.json)

run_tools() {
  echo "tool tests run: $1"
  exec task test:tools
}

base_ref=origin/main
if [ "${GITHUB_ACTIONS:-}" = true ]; then
  case "${GITHUB_EVENT_NAME:-}" in
    push) run_tools "a push run in CI runs every tool test" ;;
    pull_request) base_ref="origin/${GITHUB_BASE_REF:-}" ;;
    *) run_tools "CI event '${GITHUB_EVENT_NAME:-}' is not push or pull_request" ;;
  esac
fi

if ! base=$(git merge-base HEAD "$base_ref"); then
  run_tools "no merge base with $base_ref was found"
fi
if ! changed=$(git diff --name-only --no-renames "$base"); then
  run_tools "git diff against $base_ref failed"
fi
if [ "${GITHUB_ACTIONS:-}" != true ]; then
  if ! untracked=$(git ls-files --others --exclude-standard); then
    run_tools "git ls-files failed"
  fi
  changed=$(printf '%s\n%s\n' "$changed" "$untracked")
fi

while IFS= read -r file; do
  for t in "${triggers[@]}"; do
    # A trigger ending in / is a directory and matches every file under it.
    if [[ "$file" == "$t" || ( "$t" == */ && "$file" == "$t"* ) ]]; then
      run_tools "$file changed"
    fi
  done
done <<<"$changed"

echo "tool tests skipped: no changed file under ${triggers[*]}"
