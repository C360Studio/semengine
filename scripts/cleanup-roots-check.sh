#!/usr/bin/env bash
# Refuse the #1417 shape: a test that stops, closes, or terminates something under
# context.Background() or context.TODO(). That cleanup has no bound, so one wedged
# owner hangs the test binary until the 10-minute go test timeout (SemStreams
# counted 334 of these). Cleanup takes a fresh finite context instead:
#   ctx, cancel := context.WithTimeout(context.Background(), budget); defer cancel(); o.Stop(ctx)
# Zero baseline: every hit fails. The harness's own cleanup roots are bounded and
# live in non-test files, so they need no exemption.
# Usage: cleanup-roots-check.sh [repo-root]   (defaults to this repository)
set -euo pipefail
cd "${1:-$(dirname "$0")/..}"

# One line holding both the call and the unbounded root. --untracked includes new
# test files that are not yet added, so the guard fires before the commit does.
pattern='\.(Stop|Close|Terminate)\(.*context\.(Background|TODO)\(\)'
hits=$(git grep -n --untracked -E "$pattern" -- '*_test.go' || true)

files=$(git ls-files --cached --others --exclude-standard -- '*_test.go' | wc -l | tr -d ' ')
if [ -n "$hits" ]; then
  echo "cleanup-roots: unbounded cleanup in tests (use context.WithTimeout for Stop/Close/Terminate):"
  echo "$hits" | sed 's/^/  /'
  exit 1
fi
echo "cleanup-roots: ok ($files test file(s) scanned)"
