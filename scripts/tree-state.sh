#!/usr/bin/env bash
# Print a short fingerprint of the working tree: HEAD, the status listing, and the
# tracked diff. The integration runner records it with its evidence and
# cover-check.sh compares it, so coverage is never read from a run of another tree.
set -euo pipefail
cd "$(dirname "$0")/.."
{ git rev-parse HEAD; git status --porcelain; git diff HEAD; } | shasum | cut -c1-12
