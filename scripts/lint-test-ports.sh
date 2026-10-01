#!/usr/bin/env bash
# lint-test-ports.sh — substrate-flake guard for gh#220 Subclass 2.
#
# Fails (exit 1) if any *_test.go file binds a fixed TCP port via
# net.Listen. Ephemeral allocation via net.Listen("tcp", ":0") or
# "127.0.0.1:0" is the required pattern. Fixed-port literals collide
# under parallel test execution and were the source of gh#209 and the
# websocket integration flakes.
#
# Sourced from BOTH taskfiles/lint.yml and .github/workflows/ci.yml so
# the regex has a single source of truth (gh#220 review I4).
#
# Known false-negative shapes (acceptable for Step 1 — sweep in
# follow-up if violations surface):
#   - tcp6 protocol variant: net.Listen("tcp6", ":1234")
#   - Multi-line: net.Listen("tcp",\n\t":1234")
#   - String concatenation: net.Listen("tcp", host+":"+portStr)
#   - Variable indirection: addr := ":1234"; net.Listen("tcp", addr)
#   - Non-net.Listen APIs: http.Server{Addr: fmt.Sprintf(":%d", N)} etc.
#     (the github-webhook tests use this — sweep in Subclass 2 follow-up)
#   - Bind-then-close-then-bind: net.Listen("tcp", ":0"), read the port,
#     close the listener, and bind that port again. Another process can
#     take the port in between; SemStreams removed its freePort helper
#     for this race (its #1120).
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
LITERAL_PATTERN='net\.Listen\("tcp[46]?",\s*"[^"]*:[1-9][0-9]*"'

# Pattern 2: Sprintf form, both bare (":%d") and host-prefixed
# ("127.0.0.1:%d"). The greedy [^"]* between quotes accepts any
# host segment (or none) before the :%d.
SPRINTF_PATTERN='net\.Listen\("tcp[46]?",\s*fmt\.Sprintf\("[^"]*:%d'

# No line is exempt: there is no inline marker.
matches=$(grep -rnE "$LITERAL_PATTERN" --include='*_test.go' . 2>/dev/null || true)
sprintf_matches=$(grep -rnE "$SPRINTF_PATTERN" --include='*_test.go' . 2>/dev/null || true)
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
