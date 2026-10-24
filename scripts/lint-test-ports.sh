#!/usr/bin/env bash
# lint-test-ports.sh — substrate-flake guard for gh#220 Subclass 2.
#
# Fails (exit 1) if any *_test.go file binds a fixed TCP port via
# net.Listen. Fixed-port literals collide under parallel test execution
# and were the source of gh#209 and the websocket integration flakes.
# This guard does not judge port-0 picks: whether a pick on every
# address is refused or allowed is TestEveryAddressPortPickSaysWhy's
# rule (internal/harness/contract/addresses_test.go).
#
# It runs from Taskfile.yml's lint task (run by task verify, which CI
# runs) and from TestLintTestPortsPasses. It scans the *_test.go files
# git would not ignore (git grep --no-index --exclude-standard), so a
# git-ignored copy of the tree, such as .worktrees/, is not read.
#
# Known false-negative shapes (acceptable for Step 1 — sweep in
# follow-up if violations surface):
#   - tcp6 protocol variant: net.Listen("tcp6", ":1234")
#   - Multi-line: net.Listen("tcp",\n\t":1234")
#   - String concatenation: net.Listen("tcp", host+":"+portStr)
#   - Variable indirection: addr := ":1234"; net.Listen("tcp", addr)
#   - Non-net.Listen APIs: http.Server{Addr: fmt.Sprintf(":%d", N)} etc.
#     (the github-webhook tests use this — sweep in Subclass 2 follow-up)
#   - Bind-then-close-then-bind: net.Listen("tcp4", ":0"), read the port,
#     close the listener, and bind that port again. Another process can
#     take the port in between; SemStreams removed its freePort helper
#     for this race (its #1120). natsfixture's freePort (fixture.go, not
#     a test file, so not scanned) is the one deliberate use here, and
#     it is this guard's rule applied: the kernel picks the port, on
#     tcp4 (#390; freePort's doc comment says why). It
#     closes the listener because Docker, another process, can be given
#     only the number; a process that takes the port in between fails
#     Docker's start loudly with "address already in use" (SemEngine #209;
#     measured on macOS by the probe on PR #210, comment 6151736554), and
#     Start removes that unstarted container and tries once more with a
#     fresh port (#345).
#
# Adapted in SemEngine (openspec change flake-defense, design D7): the
# inline exemption marker is gone, and the failure message says to hand
# the listener itself to the code under test instead of naming a
# SemStreams helper that bound, closed and rebound a port.
#
# Run from repo root:
#   scripts/lint-test-ports.sh

set -euo pipefail

# Pattern 1: literal port like ":18082" or "127.0.0.1:8080" (anything
# ending in :NNNN where NNNN is non-zero).
# [[:space:]] rather than \s: git grep's extended regular expressions
# do not match \s.
LITERAL_PATTERN='net\.Listen\("tcp[46]?",[[:space:]]*"[^"]*:[1-9][0-9]*"'

# Pattern 2: Sprintf form, both bare (":%d") and host-prefixed
# ("127.0.0.1:%d"). The greedy [^"]* between quotes accepts any
# host segment (or none) before the :%d.
SPRINTF_PATTERN='net\.Listen\("tcp[46]?",[[:space:]]*fmt\.Sprintf\("[^"]*:%d'

# scan prints the *_test.go lines matching $1 under the working
# directory, skipping what git ignores. --no-index also works in a
# directory that is not a repository. git grep exits 1 when nothing
# matches, which is a clean scan; any other failure (128 on a bad
# pattern, say) fails the guard with git's own error on stderr.
scan() {
  local rc=0
  git grep --no-index --exclude-standard -nE "$1" -- '*_test.go' || rc=$?
  if [ "$rc" -gt 1 ]; then
    echo "FAIL: git grep exited $rc while scanning test files" >&2
    exit "$rc"
  fi
}

# No line is exempt: there is no inline marker.
matches=$(scan "$LITERAL_PATTERN") || exit $?
sprintf_matches=$(scan "$SPRINTF_PATTERN") || exit $?
all=$(printf '%s\n%s\n' "$matches" "$sprintf_matches" | grep -v '^$' || true)

if [ -n "$all" ]; then
  echo 'FAIL: fixed-port net.Listen in test files (substrate-flake guard, gh#220 Subclass 2)'
  echo
  echo "$all"
  echo
  echo 'Fix: bind port 0 with net.Listen("tcp", "127.0.0.1:0") and hand the listener itself to the code'
  echo 'under test. Do not close it to reuse its port: another process can take the port in between.'
  exit 1
fi
