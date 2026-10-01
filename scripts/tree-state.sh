#!/usr/bin/env bash
# Print a short fingerprint of the working tree: HEAD, the status listing, the
# tracked diff, and the content of every untracked, unignored file (the status
# listing names them but does not see an edit to one). The integration runner
# records it with its evidence and cover-check.sh compares it, so coverage is never
# read from a run of another tree.
set -euo pipefail
cd "$(dirname "$0")/.."
{
  git rev-parse HEAD
  git status --porcelain
  git diff HEAD
  git ls-files --others --exclude-standard | git hash-object --stdin-paths
} | shasum | cut -c1-12
