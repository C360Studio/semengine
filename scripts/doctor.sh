#!/usr/bin/env bash
# Report tool versions and compare each against its single pin. Starts no workloads.
set -uo pipefail
cd "$(dirname "$0")/.."

fail=0
ok()   { printf '  ok    %-13s %s\n' "$1" "$2"; }
bad()  { printf '  FAIL  %-13s %s\n' "$1" "$2"; fail=1; }
warn() { printf '  warn  %-13s %s\n' "$1" "$2"; }

# version_ge A B: true when A >= B (dotted numeric versions).
version_ge() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]; }
pkg_pin() { node -p "require('./package.json').devDependencies['$1']" 2>/dev/null; }

echo "SemEngine doctor"

# go: go.mod's go directive is the pin; the toolchain must be at least that.
if command -v go >/dev/null; then
  want=$(awk '$1=="go"{print $2; exit}' go.mod)
  have=$(go env GOVERSION | sed 's/^go//')
  if version_ge "$have" "$want"; then ok go "$have (go.mod: $want)"; else bad go "$have < go.mod $want"; fi
else bad go "not found"; fi

# task: .task-version is the pin (CI installs it from the same file).
if command -v task >/dev/null; then
  want=$(tr -d '[:space:]' < .task-version)
  have=$(task --version | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+' | head -n1)
  if [ "$have" = "$want" ]; then ok task "$have"; else bad task "$have != .task-version $want"; fi
else bad task "not found"; fi

# node: .nvmrc is the pin, compared by major version.
if command -v node >/dev/null; then
  want=$(tr -d '[:space:]v' < .nvmrc)
  have=$(node --version | sed 's/^v//')
  if [ "${have%%.*}" = "${want%%.*}" ]; then ok node "$have (.nvmrc: $want)"; else bad node "$have != .nvmrc $want"; fi
else bad node "not found"; fi

if command -v npm >/dev/null; then ok npm "$(npm --version)"; else bad npm "not found"; fi

# npm-installed tools resolve from node_modules, pinned in package.json.
check_npm_tool() { # name bin
  local want have
  want=$(pkg_pin "$1")
  if [ ! -x "node_modules/.bin/$2" ]; then bad "$2" "not installed: run npm ci"; return; fi
  have=$(node -p "require('./node_modules/$1/package.json').version")
  if [ "$have" = "$want" ]; then ok "$2" "$have"; else bad "$2" "$have != package.json $want"; fi
}
check_npm_tool @fission-ai/openspec openspec
check_npm_tool markdownlint-cli2 markdownlint-cli2

# docker: version and daemon reachability only; no containers are started.
if command -v docker >/dev/null; then
  ok docker "client $(docker --version | sed 's/^Docker version //; s/,.*//')"
  if docker info >/dev/null 2>&1; then ok docker-daemon "reachable"
  else warn docker-daemon "not reachable (no current gate needs it)"; fi
else bad docker "not found"; fi

if [ "$fail" -ne 0 ]; then echo "doctor: FAILED"; exit 1; fi
echo "doctor: ok"
