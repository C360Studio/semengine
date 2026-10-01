#!/usr/bin/env bash
# The one admitted entry to SemEngine's Docker-backed tests (openspec change
# setup-02-isolated-harness, capability integration-test-runner). Adapted from
# SemStreams scripts/run-integration-tests.sh at 5457b345 (ledger row L6):
#   - the host lock is SemStreams' own, byte-compatible, so the two repositories
#     serialise on one daemon and each sees the other as an ordinary owner;
#   - go test runs in its own process group and INT/TERM reach every process in
#     it (SemStreams' traps never forward, so test binaries outlive an interrupt);
#   - after the group is reaped, containers labelled with this run's
#     testcontainers session are waited for, then removed by ID, never by name;
#   - evidence for every run lands in one directory.
# Usage: scripts/test-integration.sh [packages...]   (default ./...)
# Must stay bash 3.2 compatible: it is macOS's /bin/bash.
#
# Interrupting a run: Ctrl-C at a terminal, or SIGTERM from a script. A script that
# starts the runner as an `&` job of a non-interactive shell starts it with SIGINT
# ignored, and no shell can trap a signal ignored on entry; such a runner warns,
# records int_ignored_on_entry=yes, and answers only to SIGTERM.
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

# shellcheck source=scripts/admission-lock.sh
. scripts/admission-lock.sh

readonly max_wait_seconds=3600
readonly pull_budget_seconds=300 # a ceiling on registry latency, not a delay
# TERM to KILL for the go test process group. SEMENGINE_TEST_SIGNAL_GRACE_SECONDS is
# reserved for the runner contract tests, so the escalation is proven without waiting
# out the production grace.
signal_grace_seconds="${SEMENGINE_TEST_SIGNAL_GRACE_SECONDS:-20}"
if [[ ! "$signal_grace_seconds" =~ ^[0-9]+$ ]] || ((signal_grace_seconds < 1 || signal_grace_seconds > 20)); then
  echo "[INTEGRATION] invalid SEMENGINE_TEST_SIGNAL_GRACE_SECONDS=$signal_grace_seconds (expected 1-20)" >&2
  exit 2
fi
readonly signal_grace_seconds
readonly leak_wait_seconds=15    # Ryuk reaps a dead session's containers after 10s

# Only SEMENGINE_* variables are read, so a shell tuned for SemStreams cannot change
# a SemEngine run. The lock-dir override exists for the runner contract tests.
lock_dir="${SEMENGINE_DOCKER_ADMISSION_LOCK_DIR:-$admission_lock_default}"
wait_seconds="${SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS:-0}"
if [[ ! "$wait_seconds" =~ ^[0-9]+$ ]] || ((wait_seconds > max_wait_seconds)); then
  echo "[INTEGRATION] invalid SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS=$wait_seconds (expected 0-$max_wait_seconds)" >&2
  exit 2
fi

packages=("$@")
((${#packages[@]} > 0)) || packages=(./...)

# The image is spelled once, in .nats-image. SEMENGINE_NATS_IMAGE may replace it for
# one run (the forced-failure protocol), but only with another digest reference and a
# stated SEMENGINE_NATS_IMAGE_OVERRIDE_REASON, so a stale export in a shell fails
# loudly instead of silently running another image. The override is warned and recorded.
pin=$(grep -v '^[[:space:]]*#' .nats-image | grep -v '^[[:space:]]*$')
image=$pin
image_override=none
image_override_reason=""
if [ -n "${SEMENGINE_NATS_IMAGE:-}" ]; then
  if [ -z "${SEMENGINE_NATS_IMAGE_OVERRIDE_REASON:-}" ]; then
    echo "[INTEGRATION] SEMENGINE_NATS_IMAGE is set without SEMENGINE_NATS_IMAGE_OVERRIDE_REASON; refusing to replace the pin $pin" >&2
    exit 2
  fi
  image=$SEMENGINE_NATS_IMAGE
  image_override=$image
  image_override_reason=$SEMENGINE_NATS_IMAGE_OVERRIDE_REASON
fi
if [[ ! "$image" =~ ^nats(:[A-Za-z0-9][A-Za-z0-9._-]*)?@(sha256:[0-9a-f]{64})$ ]]; then
  echo "[INTEGRATION] NATS image '$image' is not a digest reference (nats[:tag]@sha256:<64 hex>)" >&2
  exit 2
fi
digest=${BASH_REMATCH[2]}
if [ "$image_override" != none ]; then
  echo "[INTEGRATION] WARN: SEMENGINE_NATS_IMAGE replaces the pin for this run: $image (pin: $pin; reason: $image_override_reason)" >&2
fi

# Clock: bash 5's EPOCHREALTIME, GNU date's %3N, python3, else whole seconds.
now_ms() {
  if [ -n "${EPOCHREALTIME:-}" ]; then
    local s=${EPOCHREALTIME%.*} f=${EPOCHREALTIME#*.}000
    printf '%s' "$((10#$s * 1000 + 10#${f:0:3}))"
  elif d=$(date +%s%3N 2>/dev/null) && [[ "$d" =~ ^[0-9]{13}$ ]]; then
    printf '%s' "$d"
  elif command -v python3 >/dev/null; then
    python3 -c 'import time; print(int(time.time() * 1000))'
  else
    printf '%s' "$(($(date +%s) * 1000))"
  fi
}

owner_host=$(hostname)
owner_pid=$$
owner_started=$(date +%s)
owner_identity=$(ps -o lstart= -p "$owner_pid" 2>/dev/null | sed 's/^[[:space:]]*//' || true)
[ -n "$owner_identity" ] || owner_identity="unknown"
owner_token="${owner_host}:${owner_pid}:${owner_started}:${RANDOM:-0}"
owner_command="semengine $root/scripts/test-integration.sh"

# Evidence: the caller's directory, or one per run under the worktree. Colons are
# replaced because artifact uploads refuse them in paths.
evidence_default=no
if [ -n "${SEMENGINE_EVIDENCE_DIR:-}" ]; then
  evidence_dir=$SEMENGINE_EVIDENCE_DIR
else
  evidence_default=yes
  evidence_dir="$root/.evidence/$(printf '%s' "$owner_token" | tr ':/' '--')"
fi
mkdir -p "$evidence_dir"
record() { printf '%s=%s\n' "$1" "$2" >> "$evidence_dir/runner.env"; }
record token "$owner_token"
record command "$owner_command"
record lock_dir "$lock_dir"
record packages "${packages[*]}"
record git_head "$(git rev-parse HEAD 2>/dev/null || echo unknown)"
record tree_state "$(scripts/tree-state.sh 2>/dev/null || echo unknown)"
record signal_grace_s "$signal_grace_seconds"

lock_held=false
pull_pid=""
tee_pid=""
child=""
received=""
signalled_at_ms=""
leak_status=0
status=0

# ---- lock (byte-compatible with SemStreams run-integration-tests.sh:78-242) ----

read_owner() {
  observed_host=unknown observed_pid=unknown observed_started=0
  observed_identity=unknown observed_token=unknown observed_command=unknown
  [ -f "$lock_dir/owner" ] || return 0
  while IFS='=' read -r key value; do
    case "$key" in
      host) observed_host=$value ;;
      pid) observed_pid=$value ;;
      started) observed_started=$value ;;
      identity) observed_identity=$value ;;
      token) observed_token=$value ;;
      command) observed_command=$value ;;
    esac
  done < "$lock_dir/owner"
}

owner_elapsed() {
  local now elapsed=0
  now=$(date +%s)
  if [[ "$observed_started" =~ ^[0-9]+$ ]] && ((now >= observed_started)); then
    elapsed=$((now - observed_started))
  fi
  printf '%s' "$elapsed"
}

describe_owner() {
  echo "[INTEGRATION] lock owner host=$observed_host pid=$observed_pid elapsed=$(owner_elapsed)s command=$observed_command" >&2
}

# Stale only when provably dead on this host: the pid is gone, or it now names a
# process with a different start time. Another host's owner is never judged here.
owner_is_stale() {
  [ "$observed_host" = "$owner_host" ] || return 1
  [[ "$observed_pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$observed_pid" 2>/dev/null || return 0
  local live
  live=$(ps -o lstart= -p "$observed_pid" 2>/dev/null | sed 's/^[[:space:]]*//' || true)
  [ "$observed_identity" != "unknown" ] && [ -n "$live" ] && [ "$live" != "$observed_identity" ]
}

clean_stale_lock() {
  local stale_dir="${lock_dir}.stale.${owner_pid}.${owner_started}"
  if mv "$lock_dir" "$stale_dir" 2>/dev/null; then
    rm -f "$stale_dir/owner"
    if ! rmdir "$stale_dir"; then
      echo "[INTEGRATION] refused to remove non-empty stale lock quarantine $stale_dir" >&2
      exit 1
    fi
    echo "[INTEGRATION] cleaned stale lock from host=$observed_host pid=$observed_pid elapsed=$(owner_elapsed)s"
    record lock_quarantined "host=$observed_host pid=$observed_pid command=$observed_command"
    return 0
  fi
  return 1
}

acquire_lock() {
  local deadline=$((owner_started + wait_seconds)) announced=false
  while true; do
    if mkdir "$lock_dir" 2>/dev/null; then
      {
        printf 'host=%s\n' "$owner_host"
        printf 'pid=%s\n' "$owner_pid"
        printf 'started=%s\n' "$owner_started"
        printf 'identity=%s\n' "$owner_identity"
        printf 'token=%s\n' "$owner_token"
        printf 'command=%s\n' "$owner_command"
      } > "$lock_dir/owner"
      lock_held=true
      cp "$lock_dir/owner" "$evidence_dir/lock-owner"
      record lock_wait_s "$(($(date +%s) - owner_started))"
      return 0
    fi
    read_owner
    if owner_is_stale && clean_stale_lock; then continue; fi
    if ((wait_seconds == 0)); then
      echo "[INTEGRATION] host lock is busy and no wait budget was requested: $lock_dir" >&2
      describe_owner
      record lock_refused "host=$observed_host pid=$observed_pid command=$observed_command"
      return 1
    fi
    if ! $announced; then
      echo "[INTEGRATION] host lock is busy; waiting up to ${wait_seconds}s" >&2
      describe_owner
      record lock_waited_on "host=$observed_host pid=$observed_pid command=$observed_command"
      announced=true
    fi
    if (($(date +%s) >= deadline)); then
      echo "[INTEGRATION] host lock wait budget ${wait_seconds}s exhausted: $lock_dir" >&2
      describe_owner
      return 1
    fi
    [ -z "$received" ] || return 1
    sleep 1
  done
}

# Release only while the owner record still carries this run's token.
release_lock() {
  $lock_held || return 0
  local current=""
  [ -f "$lock_dir/owner" ] && current=$(awk -F= '$1 == "token" {sub(/^token=/, ""); print; exit}' "$lock_dir/owner")
  if [ "$current" != "$owner_token" ]; then
    echo "[INTEGRATION] lock ownership changed; refusing to remove $lock_dir" >&2
    return 0
  fi
  rm -f "$lock_dir/owner"
  rmdir "$lock_dir" || echo "[INTEGRATION] lock directory is unexpectedly non-empty: $lock_dir" >&2
  lock_held=false
  record lock_released_ms "$(now_ms)"
}

# ---- background jobs: bash's job table is the ownership authority -------------

job_running() {
  local p
  for p in $(jobs -pr); do [ "$p" = "$1" ] && return 0; done
  return 1
}

# Wait for a background job, polling so a trapped signal is acted on promptly.
# Returns 124 if the deadline (epoch seconds, or empty for none) passes first.
wait_job() {
  local pid=$1 deadline=${2:-}
  while job_running "$pid"; do
    if [ -n "$deadline" ] && (($(date +%s) >= deadline)); then return 124; fi
    [ -z "$received" ] || [ "$pid" = "$child" ] || return 125
    sleep 0.1
  done
  wait "$pid"
}

terminate_pull() {
  [ -n "$pull_pid" ] || return 0
  if job_running "$pull_pid"; then
    kill -TERM "$pull_pid" 2>/dev/null || true
    local until=$(($(date +%s) + 2))
    while job_running "$pull_pid" && (($(date +%s) < until)); do sleep 0.05; done
    # Recheck the job table immediately before escalating, so a job bash has already
    # reaped can never turn this into a signal to a recycled pid.
    job_running "$pull_pid" && kill -KILL "$pull_pid" 2>/dev/null
  fi
  wait "$pull_pid" 2>/dev/null
  pull_pid=""
}

# ---- signals ------------------------------------------------------------------

on_signal() {
  [ -n "$received" ] && return 0
  received=$1
  signalled_at_ms=$(now_ms)
  record signal "$1"
  record signal_at_ms "$signalled_at_ms"
  echo "[INTEGRATION] received SIG$1; forwarding to the go test process group" >&2
  if [ -n "$child" ]; then
    kill "-$1" -- "-$child" 2>/dev/null || true
    record group_signalled_ms "$(now_ms)"
  fi
}
trap 'on_signal INT' INT
trap 'on_signal TERM' TERM
# A signal ignored on entry cannot be trapped (POSIX), and bash then leaves the trap
# unset; trap -p is the only portable way to see it under bash 3.2.
if [ -z "$(trap -p INT)" ]; then
  record int_ignored_on_entry yes
  echo "[INTEGRATION] WARN: SIGINT was ignored when this runner started (an & job of a non-interactive shell?); it cannot be interrupted with SIGINT, send SIGTERM" >&2
else
  record int_ignored_on_entry no
fi

finish() {
  terminate_pull
  # Normally already waited for; on an early exit it may still be blocked opening the
  # FIFO, and it ignores TERM.
  if [ -n "$tee_pid" ] && job_running "$tee_pid"; then kill -KILL "$tee_pid" 2>/dev/null; fi
  release_lock
}
trap finish EXIT

exit_for_signal() {
  case "$received" in INT) exit 130 ;; TERM) exit 143 ;; esac
}

# ---- run ----------------------------------------------------------------------

acquire_lock || { exit_for_signal; exit 1; }
exit_for_signal

# Ryuk stays on in every run: test cleanup is primary, Ryuk is crash safety. This
# export wins over the caller's environment and ~/.testcontainers.properties.
export TESTCONTAINERS_RYUK_DISABLED=false
export SEMENGINE_DOCKER_ADMISSION_TOKEN="$owner_token"
export SEMENGINE_DOCKER_ADMISSION_LOCK_DIR="$lock_dir"
export SEMENGINE_EVIDENCE_DIR="$evidence_dir"
export SEMENGINE_NATS_IMAGE="$image"

# Preflight, all under the lock and all recorded.
started_ms=$(now_ms)
if ! docker info > "$evidence_dir/docker-info.txt" 2>&1; then
  echo "[INTEGRATION] docker info failed after $(($(now_ms) - started_ms))ms:" >&2
  sed -n '1,40p' "$evidence_dir/docker-info.txt" >&2
  record preflight "docker info failed"
  exit 1
fi
record docker_info_ms "$(($(now_ms) - started_ms))"
docker version > "$evidence_dir/docker-version.txt" 2>&1 || true
record docker_context "$(docker context show 2>/dev/null || echo unknown)"
record docker_endpoint "$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || echo unknown)"
record docker_host_env "${DOCKER_HOST:-unset}"
record ryuk_env "TESTCONTAINERS_RYUK_DISABLED=false (exported)"
record tc_properties "$([ -f "$HOME/.testcontainers.properties" ] && echo present || echo absent)"
record nats_image "$image"
record image_override "$image_override"
[ "$image_override" = none ] || record image_override_reason "$image_override_reason"
echo "[INTEGRATION] docker info latency: $(sed -n 's/^docker_info_ms=//p' "$evidence_dir/runner.env")ms"

# The cache is checked by digest, so another repository re-pulling a mutable tag
# cannot change what this run uses. A missing image is pulled under the lock, bounded.
if docker image inspect "nats@$digest" > /dev/null 2>&1; then
  record nats_digest_cached yes
else
  record nats_digest_cached no
  echo "[INTEGRATION] pulling $image (budget ${pull_budget_seconds}s)"
  started_ms=$(now_ms)
  docker pull "$image" > "$evidence_dir/pull.log" 2>&1 &
  pull_pid=$!
  wait_job "$pull_pid" "$(($(date +%s) + pull_budget_seconds))"
  pull_status=$?
  record pull_ms "$(($(now_ms) - started_ms))"
  record pull_status "$pull_status"
  # 124 is the budget, 125 a signal: either way the pull is still running, and it is
  # killed and reaped here, before the lock can be released. pull_pid is cleared only
  # by terminate_pull or after a pull wait_job has reaped.
  if ((pull_status == 124 || pull_status == 125)); then
    terminate_pull
    exit_for_signal
    echo "[INTEGRATION] $image pull timed out after ${pull_budget_seconds}s" >&2
    exit 1
  fi
  pull_pid=""
  exit_for_signal
  if ((pull_status != 0)); then
    echo "[INTEGRATION] $image pull failed after $(sed -n 's/^pull_ms=//p' "$evidence_dir/runner.env")ms:" >&2
    sed -n '1,40p' "$evidence_dir/pull.log" >&2
    exit 1
  fi
fi

# Listings for the pass-evidence protocol: every row before and after, so a diff
# shows exactly what this run touched.
listing() {
  {
    echo "# containers"
    docker ps -a --no-trunc --format '{{.ID}} {{.Names}} {{.Status}} {{.Labels}}' 2>&1
    echo "# volumes"
    docker volume ls --format '{{.Name}}' 2>&1
    echo "# networks"
    docker network ls --no-trunc --format '{{.ID}} {{.Name}}' 2>&1
  } > "$evidence_dir/$1"
}
listing listing-before.txt

argv=(test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m
  "-coverprofile=$evidence_dir/integration.coverprofile" "${packages[@]}")
record go_test_argv "go ${argv[*]}"
echo "[INTEGRATION] running Docker-backed tests (-race, integration tag, at most 2 packages at a time)"

# -p 2 is inherited from SemStreams gh#736 (container starts queue behind each other
# when every package boots its own); unmeasured here, so per-package wall time is kept.
# The log's tee ignores INT and TERM: a terminal Ctrl-C reaches the runner's whole
# foreground group, and a tee that died first would SIGPIPE the output go test writes
# while shutting down. It is a job reading a FIFO, not a process substitution, so it
# can be waited for and the log is complete before it is read.
# set -m gives go test its own process group, whose id is its pid.
fifo="$evidence_dir/.go-test.fifo"
rm -f "$fifo"
mkfifo "$fifo"
(trap '' INT TERM; exec tee "$evidence_dir/go-test.log") < "$fifo" &
tee_pid=$!
started_ms=$(now_ms)
set -m
go "${argv[@]}" > "$fifo" 2>&1 &
child=$!
set +m
record go_test_pgid "$child"
exit_for_signal_pending=false
[ -n "$received" ] && { kill "-$received" -- "-$child" 2>/dev/null; exit_for_signal_pending=true; }

killed=false
while job_running "$child"; do
  if [ -n "$received" ] && ! $killed && (($(now_ms) - signalled_at_ms >= signal_grace_seconds * 1000)); then
    echo "[INTEGRATION] go test group still running ${signal_grace_seconds}s after SIG$received; sending KILL" >&2
    kill -KILL -- "-$child" 2>/dev/null
    record group_killed_ms "$(now_ms)"
    killed=true
  fi
  sleep 0.1
done
wait "$child"
status=$?
record go_test_status "$status"
record go_test_ms "$(($(now_ms) - started_ms))"

# Reap the rest of the group: test binaries whose go parent has already exited.
reap_deadline=$(($(date +%s) + signal_grace_seconds))
while kill -0 -- "-$child" 2>/dev/null; do
  if (($(date +%s) >= reap_deadline)) && ! $killed; then
    kill -KILL -- "-$child" 2>/dev/null
    record group_killed_ms "$(now_ms)"
    killed=true
    reap_deadline=$(($(date +%s) + 5))
  elif (($(date +%s) >= reap_deadline)); then
    echo "[INTEGRATION] process group $child did not exit after KILL" >&2
    record group_reaped no
    break
  fi
  sleep 0.1
done
kill -0 -- "-$child" 2>/dev/null || record group_reaped_ms "$(now_ms)"
# The group is gone, so no writer holds the FIFO and tee reaches EOF.
wait "$tee_pid" 2>/dev/null
tee_pid=""
rm -f "$fifo"
grep -E '^(ok|FAIL|---)[[:space:]]' "$evidence_dir/go-test.log" > "$evidence_dir/per-package.txt" 2>/dev/null || true

# Leak check by session: the fixture wrote each test process's testcontainers session
# label as key=value, read from the labels testcontainers applies. Ryuk removes a
# finished session's containers after its 10s grace; wait for that, then remove any
# survivor by ID and fail the run. No name filter, no prune.
leak_check() {
  local labels label survivors deadline
  if [ ! -s "$evidence_dir/testcontainers-session" ]; then
    record leak_check "clean (no session recorded: no fixture started)"
    return 0
  fi
  labels=$(sort -u "$evidence_dir/testcontainers-session")
  for label in $labels; do
    if [[ ! "$label" =~ ^[A-Za-z0-9._-]+=[A-Za-z0-9._-]+$ ]]; then
      echo "[INTEGRATION] malformed session label '$label'; cannot leak-check this run" >&2
      record leak_check "failed (malformed session label)"
      return 1
    fi
  done
  record session_labels "$(echo $labels)"
  deadline=$(($(date +%s) + leak_wait_seconds))
  while true; do
    survivors=""
    for label in $labels; do
      survivors="$survivors $(docker ps -aq --no-trunc --filter "label=$label")"
    done
    survivors=$(echo $survivors)
    [ -z "$survivors" ] && break
    (($(date +%s) >= deadline)) && break
    sleep 0.5
  done
  record leak_wait_s "$((leak_wait_seconds - (deadline - $(date +%s))))"
  if [ -z "$survivors" ]; then
    record leak_check clean
    return 0
  fi
  echo "[INTEGRATION] containers of this run's session survived ${leak_wait_seconds}s; removing by id: $survivors" >&2
  for id in $survivors; do
    docker inspect --format '{{.Id}} {{.Name}} {{.State.Status}} {{.Config.Image}}' "$id" >> "$evidence_dir/leaks.txt" 2>&1
    if docker rm -f "$id" >> "$evidence_dir/leaks.txt" 2>&1; then
      echo "removed $id" >> "$evidence_dir/leaks.txt"
    fi
  done
  record leak_check "survivors $survivors"
  return 1
}
leak_check || leak_status=1
listing listing-after.txt

# The last completed run in this worktree, for task cover:check. Only default-location
# runs are pointed at; a caller-chosen directory (the contract tests) is not.
if [ "$evidence_default" = yes ]; then
  printf '%s\n' "$evidence_dir" > "$root/.evidence/last-run"
fi

exit_for_signal
if ((status != 0)); then
  echo "[INTEGRATION] tests failed with status $status; evidence: $evidence_dir" >&2
  exit "$status"
fi
if ((leak_status != 0)); then
  echo "[INTEGRATION] leak check failed; evidence: $evidence_dir" >&2
  exit 1
fi
echo "[INTEGRATION] tests complete; evidence: $evidence_dir"
