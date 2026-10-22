#!/usr/bin/env bash
# Port status and issue/milestone hygiene tracker (openspec change port-status-tracker;
# spec port-status).
#
# Usage:
#   scripts/port-status.sh [--offline] [--check] [--sync-project [PROJECT_NUMBER]]
#
# Modes:
#   default          Print the 65-package Slice 04A scoreboard from docs/admission-ledger.yaml
#                    plus the service 7-PR lifecycle-owner breakdown, then query GitHub for
#                    milestones, active PR claims, open class:flake blockers, and issue hygiene.
#   --offline        Print only the local admission-ledger and tree scoreboard (no gh calls).
#   --check          Verify milestone titles contain no whitespace and every open issue has a
#                    milestone and at least one label; exit 1 naming each violation, or exit 2
#                    if a GitHub read fails.
#   --sync-project   Ensure a GitHub Project v2 board (default title "SemEngine Port Tracker")
#                    exists under C360Studio and add every open issue and PR to it. Requires
#                    gh token scope `project` (`gh auth refresh -s project`).

set -uo pipefail

OFFLINE=0
CHECK=0
SYNC_PROJECT=0
PROJECT_NUM=""

while [ $# -gt 0 ]; do
  case "$1" in
    --offline) OFFLINE=1; shift ;;
    --check) CHECK=1; shift ;;
    --sync-project)
      SYNC_PROJECT=1
      if [ $# -ge 2 ] && [[ "${2:-}" =~ ^[0-9]+$ ]]; then
        PROJECT_NUM="$2"
        shift 2
      else
        shift
      fi
      ;;
    -h|--help)
      sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "port-status: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

[ -f "docs/admission-ledger.yaml" ] || {
  echo "port-status: unavailable: docs/admission-ledger.yaml not found (run from repo root)" >&2
  exit 2
}

# 1. Local Slice 04A 65-package scoreboard from docs/admission-ledger.yaml and service/ files.
python3 - <<'PY' || exit 2
import os, re, sys

entries = {}
cur = {}
with open("docs/admission-ledger.yaml", encoding="utf-8") as f:
    for line in f:
        m = re.match(r"^- source_path:\s*(.+)$", line)
        if m:
            if cur:
                entries[cur["source_path"]] = cur
            cur = {"source_path": m.group(1).strip()}
        elif line.startswith("  destination:"):
            cur["destination"] = line.split("destination:", 1)[1].strip()
        elif cur.get("destination") in (">-", ">", "|") and line.startswith("    "):
            cur["destination"] = line.strip()
        elif line.startswith("  disposition:"):
            cur["disposition"] = line.split("disposition:", 1)[1].strip()
if cur:
    entries[cur["source_path"]] = cur

changes = [
    ("1. setup-04a-01-floor", [
        "message", "metric", "natsclient", "payloadregistry", "pkg/acme", "pkg/cache",
        "pkg/errs", "pkg/platform", "pkg/projection/contract", "pkg/resource", "pkg/retry",
        "pkg/security", "pkg/timestamp", "pkg/tlsutil", "pkg/types", "vocabulary",
    ]),
    ("2. setup-04a-02-ingest-kernel", [
        "component", "graph", "graph/inference", "graph/readiness", "graph/structural",
        "internal/componentadmission", "internal/graphmutation", "internal/lifecyclecleanup",
        "model", "pkg/dispatch", "pkg/lifecycle", "pkg/projection", "pkg/worker",
        "processor/graph-ingest", "storage", "storage/storeregistry", "types",
    ]),
    ("3. setup-04a-03-boot", [
        "component/flowgraph", "composition", "config", "health",
        "internal/logforwarderpolicy", "pkg/rulepack", "service",
    ]),
    ("4. setup-04a-04-graph-roots", [
        "graph/clustering", "graph/embedding", "graph/query", "pkg/graphview",
        "pkg/revlag", "processor/graph-index", "processor/graph-query", "storage/objectstore",
    ]),
    ("5. setup-04a-05-first-consumer", [
        "output/websocket", "pkg/buffer", "pkg/fusion", "pkg/fusion/fusionnats",
        "pkg/fusion/fusionvocab", "processor/graph-embedding", "vocabulary/bfo", "vocabulary/cco",
    ]),
    ("6. setup-04a-06-rule-core", [
        "processor/rule", "processor/rule/expression",
    ]),
    ("7. setup-04a-07-substrate-seam", [
        "composition/cli", "graph/geo/geojson", "internal/maxdelivery",
        "processor/graph-clustering", "processor/graph-index-spatial",
        "processor/graph-index-temporal", "vocabulary/export",
    ]),
]

service_prs = [
    ("Service PR 1/7", "BaseService + shared files + pprof", "service/base.go"),
    ("Service PR 2/7", "Manager + Metrics", "service/service_manager.go"),
    ("Service PR 3/7", "ComponentManager + rule_pack_bind", "service/component_manager.go"),
    ("Service PR 4/7", "MessageLogger", "service/message_logger.go"),
    ("Service PR 5/7", "StorageObservabilityService", "service/storage_observability.go"),
    ("Service PR 6/7", "MetricsForwarder + HeartbeatService", "service/metrics_forwarder.go"),
    ("Service PR 7/7", "LogForwarder", "service/log_forwarder.go"),
]
svc_done = sum(1 for _, _, p in service_prs if os.path.exists(p))

print("SemEngine Slice 04A Port Scoreboard (pin 8b99efe9)")
print("==================================================")
total = 0
in_ledger = 0
ported = 0
partial = 0
excluded = 0

for cname, pkgs in changes:
    c_ported, c_partial, c_excl, c_rem = [], [], [], []
    for p in pkgs:
        total += 1
        e = entries.get(p)
        if not e:
            c_rem.append(p)
            continue
        in_ledger += 1
        dest = e.get("destination", "")
        disp = e.get("disposition", "")
        if disp == "defer-exclude" or dest == "none":
            excluded += 1
            c_excl.append(p)
        elif p == "service" and svc_done < len(service_prs):
            partial += 1
            c_partial.append(f"service ({svc_done}/7 PRs on main)")
        elif p == "graph/inference":
            partial += 1
            c_partial.append("graph/inference (hierarchy slice; rest in change 7)")
        else:
            ported += 1
            c_ported.append(p)
    done_cnt = len(c_ported) + len(c_partial) + len(c_excl)
    status_parts = []
    if c_ported:
        status_parts.append(f"{len(c_ported)} ported")
    if c_partial:
        status_parts.append(f"{len(c_partial)} partial")
    if c_excl:
        status_parts.append(f"{len(c_excl)} excluded")
    if c_rem:
        status_parts.append(f"{len(c_rem)} remaining")
    summary = ", ".join(status_parts)
    print(f"  {cname:<32} {done_cnt:>2}/{len(pkgs):<2} ({summary})")
    if c_partial:
        for item in c_partial:
            print(f"      partial:   {item}")
    if c_rem and len(c_rem) <= 6:
        print(f"      remaining: {', '.join(c_rem)}")

print("\nChange 3 `service` Lifecycle-Owner Split:")
for label, desc, path in service_prs:
    mark = "x" if os.path.exists(path) else " "
    state = "merged on main" if os.path.exists(path) else "pending"
    print(f"  [{mark}] {label}: {desc:<38} ({state})")

rem_total = total - in_ledger
print(
    f"\nSummary: {in_ledger} / {total} tier-0 packages in ledger "
    f"({ported} ported, {partial} partial, {excluded} excluded, {rem_total} unstarted)"
)
PY

if [ "$OFFLINE" -eq 1 ]; then
  exit 0
fi

# Helper for fail-closed GitHub reads (exit 2 on failure or non-JSON).
answer=""
errfile=$(mktemp)
trap 'rm -f "$errfile"' EXIT
read_gh() {
  local name=$1
  shift
  if ! answer=$("$@" </dev/null 2>"$errfile"); then
    echo "port-status: unavailable: the read of ${name} failed ($*): $(cat "$errfile")" >&2
    exit 2
  fi
}

read_gh "repository milestones" gh api "repos/:owner/:repo/milestones?state=open&per_page=100"
milestones_json=$answer

read_gh "open pull requests" gh pr list --state open --limit 100 --json number,title,isDraft,headRefName,milestone,labels
prs_json=$answer

read_gh "open issues" gh issue list --state open --limit 200 --json number,title,milestone,labels
issues_json=$answer

python3 - "$CHECK" "$milestones_json" "$prs_json" "$issues_json" <<'PY'
import json, re, sys

check_mode = sys.argv[1] == "1"
try:
    milestones = json.loads(sys.argv[2])
    prs = json.loads(sys.argv[3])
    issues = json.loads(sys.argv[4])
except Exception as e:
    print(f"port-status: unavailable: invalid JSON from gh: {e}", file=sys.stderr)
    sys.exit(2)

if not isinstance(milestones, list) or not isinstance(prs, list) or not isinstance(issues, list):
    print("port-status: unavailable: expected JSON arrays from gh", file=sys.stderr)
    sys.exit(2)

print("\nGitHub Milestones (open):")
bad_milestones = []
for m in milestones:
    title = m.get("title", "")
    op = m.get("open_issues", 0)
    cl = m.get("closed_issues", 0)
    pct = int(round(100.0 * cl / (op + cl))) if (op + cl) > 0 else 0
    print(f"  {title:<16} {op:>3} open / {cl:>3} closed ({pct}% complete)")
    if re.search(r"\s", title):
        bad_milestones.append(title)

print(f"\nOpen Pull Requests ({len(prs)}):")
if not prs:
    print("  (none)")
for pr in prs:
    num = pr.get("number")
    kind = "DRAFT" if pr.get("isDraft") else "READY"
    branch = pr.get("headRefName", "")
    ms = (pr.get("milestone") or {}).get("title") or "no-milestone"
    title = pr.get("title", "")
    print(f"  #{num:<4} [{kind}] ({ms}) {branch} — {title}")

flakes = [
    i for i in issues
    if any(l.get("name") == "class:flake" for l in (i.get("labels") or []))
]
print(f"\nOpen Known Flakes (`class:flake` — blocks merge:check): {len(flakes)}")
for fl in flakes:
    print(f"  #{fl.get('number')}: {fl.get('title')}")

missing_ms = [i for i in issues if not i.get("milestone")]
missing_lbl = [i for i in issues if not (i.get("labels") or [])]

print(f"\nIssue Queue Hygiene ({len(issues)} open issues):")
print(f"  Milestone titles with whitespace: {len(bad_milestones)}")
print(f"  Open issues missing milestone:    {len(missing_ms)}")
print(f"  Open issues missing labels:       {len(missing_lbl)}")

violations = 0
for title in bad_milestones:
    print(f"  VIOLATION: milestone title {title!r} contains whitespace (breaks unquoted search)")
    violations += 1
for i in missing_ms:
    print(f"  VIOLATION: issue #{i.get('number')} is missing a milestone: {i.get('title')}")
    violations += 1
for i in missing_lbl:
    print(f"  VIOLATION: issue #{i.get('number')} has no labels: {i.get('title')}")
    violations += 1

if violations == 0:
    print("hygiene: ok")
elif check_mode:
    sys.exit(1)
PY
status=$?
[ "$status" -eq 0 ] || exit "$status"

if [ "$SYNC_PROJECT" -eq 1 ]; then
  echo ""
  echo "Syncing GitHub Project v2 board..."
  if ! proj_list=$(gh project list --owner C360Studio --format json 2>"$errfile"); then
    echo "port-status: unavailable: cannot read GitHub Projects for C360Studio: $(cat "$errfile")" >&2
    echo "port-status: run 'gh auth refresh -s project' in your terminal to grant project scope." >&2
    exit 2
  fi
  if [ -z "$PROJECT_NUM" ]; then
    PROJECT_NUM=$(printf '%s' "$proj_list" | python3 -c '
import json, sys
data = json.load(sys.stdin)
projects = data.get("projects", []) if isinstance(data, dict) else []
for p in projects:
    if "semengine" in p.get("title", "").lower():
        print(p.get("number", ""))
        break
')
  fi
  if [ -z "$PROJECT_NUM" ]; then
    echo "Creating GitHub Project 'SemEngine Port Tracker' under C360Studio..."
    if ! created=$(gh project create --owner C360Studio --title "SemEngine Port Tracker" --format json 2>"$errfile"); then
      echo "port-status: unavailable: failed to create project: $(cat "$errfile")" >&2
      exit 2
    fi
    PROJECT_NUM=$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("number",""))')
    gh project link "$PROJECT_NUM" --owner C360Studio --repo C360Studio/semengine >/dev/null 2>&1 || true
  fi
  echo "Syncing open issues and PRs to C360Studio project #${PROJECT_NUM}..."
  urls=$(python3 - "$prs_json" "$issues_json" <<'PY'
import json, sys
prs = json.loads(sys.argv[1])
issues = json.loads(sys.argv[2])
for pr in prs:
    print(f"https://github.com/C360Studio/semengine/pull/{pr['number']}")
for i in issues:
    print(f"https://github.com/C360Studio/semengine/issues/{i['number']}")
PY
)
  count=0
  while IFS= read -r url; do
    [ -n "$url" ] || continue
    gh project item-add "$PROJECT_NUM" --owner C360Studio --url "$url" >/dev/null 2>"$errfile" || {
      echo "port-status: failed adding $url to project #${PROJECT_NUM}: $(cat "$errfile")" >&2
      exit 2
    }
    count=$((count + 1))
  done <<< "$urls"
  echo "Synced ${count} open items to C360Studio project #${PROJECT_NUM}."
fi
