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
# none of it: no pull, no container, no lock taken or removed.

# The effective daemon: DOCKER_HOST wins over the current context, as it does for
# both the docker CLI and testcontainers. The /tmp lock does not follow a remote host.
if $daemon; then
  context=$(docker context show 2>/dev/null || echo unknown)
  endpoint=$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || echo unknown)
  ok docker-context "$context ($endpoint)"
  if [ -n "${DOCKER_HOST:-}" ]; then warn docker-host "DOCKER_HOST=$DOCKER_HOST overrides the context"; fi
fi

# Host-level testcontainers state other repositories can set. The runner exports
# TESTCONTAINERS_RYUK_DISABLED=false, which wins over the environment and the file.
for v in TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE; do
  if [ -n "${!v:-}" ]; then warn "${v#TESTCONTAINERS_}" "$v=${!v} in this shell"; fi
done
ok ryuk-env "TESTCONTAINERS_RYUK_DISABLED=${TESTCONTAINERS_RYUK_DISABLED:-unset} here; the runner forces false"
if [ -f "$HOME/.testcontainers.properties" ]; then
  warn tc-properties "present: $HOME/.testcontainers.properties (runner env overrides ryuk.disabled)"
else ok tc-properties "absent"; fi

# .nats-image is the one image pin; the cache is checked by digest, never by tag.
pin=$(grep -v '^[[:space:]]*#' .nats-image 2>/dev/null | grep -v '^[[:space:]]*$' || true)
if [[ "$pin" =~ ^nats:[A-Za-z0-9][A-Za-z0-9._-]*@(sha256:[0-9a-f]{64})$ ]]; then
  digest=${BASH_REMATCH[1]}
  if $daemon && docker image inspect "nats@$digest" >/dev/null 2>&1; then ok nats-image "cached by digest: $pin"
  elif $daemon; then warn nats-image "not cached; the runner pulls it under the lock: $pin"
  else ok nats-image "$pin (daemon not reachable; cache unknown)"; fi
else bad nats-image ".nats-image does not hold one nats:<tag>@sha256:<digest> line"; fi

# Ryuk's image is whatever the required testcontainers-go names as its default,
# read from the module source so doctor does not keep a second spelling of it.
tc_dir=$(go list -m -f '{{.Dir}}' github.com/testcontainers/testcontainers-go 2>/dev/null || true)
ryuk=""
if [ -n "$tc_dir" ]; then
  ryuk=$(sed -n 's/^const ReaperDefaultImage = "\(.*\)"$/\1/p' "$tc_dir/internal/config/config.go" 2>/dev/null || true)
fi
if [ -z "$ryuk" ]; then warn ryuk-image "unknown (testcontainers-go not required or not downloaded)"
elif ! $daemon; then ok ryuk-image "$ryuk (daemon not reachable; cache unknown)"
elif docker image inspect "$ryuk" >/dev/null 2>&1; then ok ryuk-image "cached: $ryuk"
else warn ryuk-image "not cached; testcontainers pulls it on first use: $ryuk"; fi

# The shared admission lock, judged the way the runner would judge it.
# shellcheck source=scripts/admission-lock.sh
. scripts/admission-lock.sh
if [ -f "$admission_lock_default/owner" ]; then
  owner=$(tr '\n' ' ' < "$admission_lock_default/owner")
  o_host=$(sed -n 's/^host=//p' "$admission_lock_default/owner")
  o_pid=$(sed -n 's/^pid=//p' "$admission_lock_default/owner")
  if [ "$o_host" != "$(hostname)" ]; then
    warn admission "held from another host (manual recovery if stale): $owner"
  elif [[ "$o_pid" =~ ^[0-9]+$ ]] && ! kill -0 "$o_pid" 2>/dev/null; then
    warn admission "stale (dead pid; the next runner quarantines it): $owner"
  else ok admission "busy: $owner"; fi
elif [ -e "$admission_lock_default" ]; then
  warn admission "$admission_lock_default exists without an owner file (being written, or foreign)"
else ok admission "free ($admission_lock_default)"; fi

if [ "$fail" -ne 0 ]; then echo "doctor: FAILED"; exit 1; fi
echo "doctor: ok"
