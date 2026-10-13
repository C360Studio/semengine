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

# python3: no pin; scripts/openspec-queue.sh parses JSON and timestamps with it. task spec:check runs that
# script with --check, so task verify needs python3 too.
if command -v python3 >/dev/null; then ok python3 "$(python3 --version 2>&1 | sed 's/^Python //')"
else bad python3 "not found (task spec:check, task verify and task spec:queue need it)"; fi

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
daemon=false
if command -v docker >/dev/null; then
  ok docker "client $(docker --version | sed 's/^Docker version //; s/,.*//')"
  if docker info >/dev/null 2>&1; then ok docker-daemon "reachable"; daemon=true
  else warn docker-daemon "not reachable (task test:integration needs it)"; fi
else bad docker "not found"; fi

# Everything below reads Docker and host state for the integration lane and changes
# none of it: no pull, no container.

# The effective daemon: DOCKER_HOST wins over the current context, as it does for
# both the docker CLI and testcontainers.
if $daemon; then
  context=$(docker context show 2>/dev/null || echo unknown)
  endpoint=$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || echo unknown)
  ok docker-context "$context ($endpoint)"
  if [ -n "${DOCKER_HOST:-}" ]; then warn docker-host "DOCKER_HOST=$DOCKER_HOST overrides the context"; fi
fi

# internal/harness/natsfixture/nats-image is the one image pin, embedded by natsfixture; the cache
# is checked by digest, never by tag.
pin_file=internal/harness/natsfixture/nats-image
pin=$(grep -v '^[[:space:]]*#' "$pin_file" 2>/dev/null | grep -v '^[[:space:]]*$' || true)
if [[ "$pin" =~ ^nats:[A-Za-z0-9][A-Za-z0-9._-]*@(sha256:[0-9a-f]{64})$ ]]; then
  digest=${BASH_REMATCH[1]}
  if $daemon && docker image inspect "nats@$digest" >/dev/null 2>&1; then ok nats-image "cached by digest: $pin"
  elif $daemon; then warn nats-image "not cached; testcontainers pulls it on first use: $pin"
  else ok nats-image "$pin (daemon not reachable; cache unknown)"; fi
else bad nats-image "$pin_file does not hold one nats:<tag>@sha256:<digest> line"; fi

if [ "$fail" -ne 0 ]; then echo "doctor: FAILED"; exit 1; fi
echo "doctor: ok"
