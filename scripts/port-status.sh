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
            c_partial.append(f"service ({svc_done}/7 PRs in tree)")
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
    state = "present in tree" if os.path.exists(path) else "pending"
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

read_gh "open pull requests" gh pr list --state open --limit 500 --json number,title,isDraft,headRefName,milestone,labels
prs_json=$answer

read_gh "open issues" gh issue list --state open --limit 500 --json number,title,milestone,labels
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

def valid_list(items):
    return isinstance(items, list) and all(
        isinstance(x, dict)
        and isinstance(x.get("number"), int)
        and not isinstance(x.get("number"), bool)
        and isinstance(x.get("title"), str)
        for x in items
    )

if not valid_list(milestones) or not valid_list(prs) or not valid_list(issues):
    print("port-status: unavailable: expected JSON arrays of numbered records from gh", file=sys.stderr)
    sys.exit(2)

if len(milestones) >= 100:
    print("port-status: unavailable: milestones read returned 100 of 100 asked for and may be incomplete", file=sys.stderr)
    sys.exit(2)
if len(prs) >= 500:
    print("port-status: unavailable: open PR read returned 500 of 500 asked for and may be incomplete", file=sys.stderr)
    sys.exit(2)
if len(issues) >= 500:
    print("port-status: unavailable: open issues read returned 500 of 500 asked for and may be incomplete", file=sys.stderr)
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
  if [ -n "$PROJECT_NUM" ]; then
    case "$PROJECT_NUM" in
      ''|*[!0-9]*|0*)
        echo "port-status: unavailable: invalid project number: ${PROJECT_NUM}" >&2
        exit 2
        ;;
    esac
  else
    if ! proj_list=$(gh project list --owner C360Studio --limit 100 --format json 2>"$errfile"); then
      echo "port-status: unavailable: cannot read GitHub Projects for C360Studio: $(cat "$errfile")" >&2
      echo "port-status: run 'gh auth refresh -s project' in your terminal to grant project scope." >&2
      exit 2
    fi
    PROJECT_NUM=$(printf '%s' "$proj_list" | python3 -c '
import json, sys
try:
    data = json.loads(sys.stdin.read())
except Exception as e:
    print(f"port-status: unavailable: invalid JSON from gh project list: {e}", file=sys.stderr)
    sys.exit(2)
if not isinstance(data, dict) or not isinstance(data.get("projects"), list):
    print("port-status: unavailable: gh project list did not return a projects array", file=sys.stderr)
    sys.exit(2)
projects = data["projects"]
for p in projects:
    if (
        not isinstance(p, dict)
        or not isinstance(p.get("number"), int)
        or isinstance(p.get("number"), bool)
        or p["number"] <= 0
        or not isinstance(p.get("title"), str)
    ):
        print("port-status: unavailable: malformed project entry in gh project list", file=sys.stderr)
        sys.exit(2)
total_count = data.get("totalCount")
if (isinstance(total_count, int) and total_count > len(projects)) or len(projects) >= 100:
    print("port-status: unavailable: gh project list returned incomplete results", file=sys.stderr)
    sys.exit(2)
matches = [p["number"] for p in projects if p.get("title") == "SemEngine Port Tracker"]
if len(matches) > 1:
    nums = ", ".join(f"#{n}" for n in matches)
    print(f"port-status: unavailable: ambiguous projects titled '\''SemEngine Port Tracker'\'' ({nums}); pass --sync-project <number>", file=sys.stderr)
    sys.exit(2)
if len(matches) == 1:
    print(matches[0])
')
    status=$?
    [ "$status" -eq 0 ] || exit "$status"
  fi
  if [ -z "$PROJECT_NUM" ]; then
    echo "Creating GitHub Project 'SemEngine Port Tracker' under C360Studio..."
    if ! created=$(gh project create --owner C360Studio --title "SemEngine Port Tracker" --format json 2>"$errfile"); then
      echo "port-status: unavailable: failed to create project: $(cat "$errfile")" >&2
      exit 2
    fi
    PROJECT_NUM=$(printf '%s' "$created" | python3 -c '
import json, sys
try:
    data = json.loads(sys.stdin.read())
except Exception as e:
    print(f"port-status: unavailable: invalid JSON from gh project create: {e}", file=sys.stderr)
    sys.exit(2)
num = data.get("number") if isinstance(data, dict) else None
if not isinstance(num, int) or isinstance(num, bool) or num <= 0:
    print("port-status: unavailable: gh project create did not return a valid project number", file=sys.stderr)
    sys.exit(2)
print(num)
')
    status=$?
    [ "$status" -eq 0 ] || exit "$status"
    gh project link "$PROJECT_NUM" --owner C360Studio --repo C360Studio/semengine >/dev/null 2>&1 || true
  fi
  echo "Syncing open issues and PRs to C360Studio project #${PROJECT_NUM} (https://github.com/orgs/C360Studio/projects/${PROJECT_NUM})..."
  if ! fields_json=$(gh project field-list "$PROJECT_NUM" --owner C360Studio --format json 2>"$errfile"); then
    echo "port-status: unavailable: cannot read fields of project #${PROJECT_NUM}: $(cat "$errfile")" >&2
    exit 2
  fi
  if ! proj_view_json=$(gh project view "$PROJECT_NUM" --owner C360Studio --format json 2>"$errfile"); then
    echo "port-status: unavailable: cannot read project #${PROJECT_NUM}: $(cat "$errfile")" >&2
    exit 2
  fi

  python3 - "$PROJECT_NUM" "$proj_view_json" "$fields_json" "$prs_json" "$issues_json" <<'PY'
import datetime, json, re, subprocess, sys

def die_unavail(msg):
    print(f"port-status: unavailable: {msg}", file=sys.stderr)
    sys.exit(2)

proj_num = int(sys.argv[1])
try:
    proj_view = json.loads(sys.argv[2])
    fields_obj = json.loads(sys.argv[3])
    expected_prs = json.loads(sys.argv[4])
    expected_issues = json.loads(sys.argv[5])
except Exception as e:
    die_unavail(f"invalid JSON in project metadata: {e}")

if not isinstance(proj_view, dict) or not isinstance(proj_view.get("id"), str) or not proj_view["id"]:
    die_unavail(f"project #{proj_num} view JSON is missing a string id")
if not isinstance(fields_obj, dict) or not isinstance(fields_obj.get("fields"), list):
    die_unavail(f"project #{proj_num} field-list JSON is missing a fields array")

proj_id = proj_view["id"]
fields = fields_obj["fields"]

ITEM_FIELDS_GQL = """
projectItems(first: 10) {
  totalCount
  pageInfo { hasNextPage }
  nodes {
    id
    project { number }
    status: fieldValueByName(name: "Status") {
      ... on ProjectV2ItemFieldSingleSelectValue { name optionId }
    }
    stage: fieldValueByName(name: "Stage") {
      ... on ProjectV2ItemFieldSingleSelectValue { name optionId }
    }
    startDate: fieldValueByName(name: "Start Date") {
      ... on ProjectV2ItemFieldDateValue { date }
    }
    targetDate: fieldValueByName(name: "Target Date") {
      ... on ProjectV2ItemFieldDateValue { date }
    }
  }
}
"""

INITIAL_QUERY = f"""query {{
  repository(owner: "C360Studio", name: "semengine") {{
    pullRequests(first: 100, states: OPEN) {{
      totalCount
      pageInfo {{ hasNextPage endCursor }}
      nodes {{
        id
        number
        title
        milestone {{ title }}
        closingIssuesReferences(first: 20) {{
          totalCount
          pageInfo {{ hasNextPage }}
          nodes {{ number }}
        }}
        {ITEM_FIELDS_GQL}
      }}
    }}
    issues(first: 100, states: OPEN) {{
      totalCount
      pageInfo {{ hasNextPage endCursor }}
      nodes {{
        id
        number
        title
        milestone {{ title }}
        {ITEM_FIELDS_GQL}
      }}
    }}
  }}
}}"""

def run_gql(query):
    proc = subprocess.run(
        ["gh", "api", "graphql", "-f", f"query={query}"],
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        die_unavail(f"GraphQL request failed: {proc.stderr.strip()}")
    try:
        payload = json.loads(proc.stdout)
    except Exception as e:
        die_unavail(f"invalid JSON from GraphQL: {e}")
    if not isinstance(payload, dict) or payload.get("errors"):
        die_unavail(f"GraphQL errors: {payload.get('errors') if isinstance(payload, dict) else payload}")
    data = payload.get("data")
    if not isinstance(data, dict):
        die_unavail("GraphQL response missing data object")
    return data

repo_data = run_gql(INITIAL_QUERY).get("repository")
if not isinstance(repo_data, dict):
    die_unavail("GraphQL response missing repository object")

def collect_connection(conn_name, initial_conn, page_fields_gql):
    if not isinstance(initial_conn, dict) or not isinstance(initial_conn.get("nodes"), list):
        die_unavail(f"GraphQL {conn_name} missing nodes array")
    nodes = list(initial_conn["nodes"])
    seen_ids = {n.get("id") for n in nodes if isinstance(n, dict) and n.get("id")}
    seen_cursors = set()
    conn = initial_conn
    while True:
        pi = conn.get("pageInfo")
        has_next = pi.get("hasNextPage") if isinstance(pi, dict) else False
        if not has_next:
            break
        cursor = pi.get("endCursor") if isinstance(pi, dict) else None
        if not isinstance(cursor, str) or not cursor or cursor in seen_cursors:
            die_unavail(f"GraphQL {conn_name} reported hasNextPage=true without an advancing endCursor")
        seen_cursors.add(cursor)
        q = f"""query {{
  repository(owner: "C360Studio", name: "semengine") {{
    {conn_name}(first: 100, states: OPEN, after: {json.dumps(cursor)}) {{
      totalCount
      pageInfo {{ hasNextPage endCursor }}
      nodes {{
        {page_fields_gql}
      }}
    }}
  }}
}}"""
        next_repo = run_gql(q).get("repository")
        conn = next_repo.get(conn_name) if isinstance(next_repo, dict) else None
        if not isinstance(conn, dict) or not isinstance(conn.get("nodes"), list):
            die_unavail(f"GraphQL {conn_name} page missing nodes array")
        page_nodes = conn["nodes"]
        new_count = 0
        for n in page_nodes:
            if not isinstance(n, dict) or not n.get("id"):
                die_unavail(f"GraphQL {conn_name} node missing id")
            if n["id"] not in seen_ids:
                seen_ids.add(n["id"])
                nodes.append(n)
                new_count += 1
        if new_count == 0:
            die_unavail(f"GraphQL {conn_name} pagination returned no new nodes while hasNextPage=true")
    total_cnt = conn.get("totalCount")
    if isinstance(total_cnt, int) and len(nodes) != total_cnt:
        die_unavail(f"GraphQL {conn_name} count {len(nodes)} does not match totalCount {total_cnt}")
    return nodes

PR_NODE_GQL = f"""id number title milestone {{ title }}
closingIssuesReferences(first: 20) {{ totalCount pageInfo {{ hasNextPage }} nodes {{ number }} }}
{ITEM_FIELDS_GQL}"""

ISSUE_NODE_GQL = f"""id number title milestone {{ title }}
{ITEM_FIELDS_GQL}"""

prs = collect_connection("pullRequests", repo_data.get("pullRequests"), PR_NODE_GQL)
issues = collect_connection("issues", repo_data.get("issues"), ISSUE_NODE_GQL)

def validate_nodes(nodes, expected_list, kind, check_closing=False):
    got_nums = set()
    for n in nodes:
        if (
            not isinstance(n, dict)
            or not isinstance(n.get("id"), str)
            or not n["id"]
            or not isinstance(n.get("number"), int)
            or isinstance(n.get("number"), bool)
        ):
            die_unavail(f"GraphQL {kind} node has invalid id or number")
        got_nums.add(n["number"])
        pi_conn = n.get("projectItems")
        if not isinstance(pi_conn, dict) or not isinstance(pi_conn.get("nodes"), list):
            die_unavail(f"GraphQL {kind} #{n['number']} missing projectItems.nodes")
        pi_page = pi_conn.get("pageInfo")
        if isinstance(pi_page, dict) and pi_page.get("hasNextPage"):
            die_unavail(f"GraphQL {kind} #{n['number']} projectItems connection is truncated")
        pi_tot = pi_conn.get("totalCount")
        if isinstance(pi_tot, int) and pi_tot > len(pi_conn["nodes"]):
            die_unavail(f"GraphQL {kind} #{n['number']} projectItems count is incomplete")
        if check_closing:
            cl_conn = n.get("closingIssuesReferences") or {}
            cl_page = cl_conn.get("pageInfo") if isinstance(cl_conn, dict) else None
            if isinstance(cl_page, dict) and cl_page.get("hasNextPage"):
                die_unavail(f"GraphQL {kind} #{n['number']} closingIssuesReferences connection is truncated")
            cl_tot = cl_conn.get("totalCount") if isinstance(cl_conn, dict) else None
            cl_nodes = cl_conn.get("nodes") if isinstance(cl_conn, dict) else None
            if isinstance(cl_tot, int) and isinstance(cl_nodes, list) and cl_tot > len(cl_nodes):
                die_unavail(f"GraphQL {kind} #{n['number']} closingIssuesReferences count is incomplete")
    want_nums = {x["number"] for x in expected_list}
    if got_nums != want_nums:
        missing = sorted(want_nums - got_nums)
        extra = sorted(got_nums - want_nums)
        die_unavail(f"GraphQL {kind} set mismatch vs list read (missing={missing}, extra={extra})")

validate_nodes(prs, expected_prs, "pullRequest", check_closing=True)
validate_nodes(issues, expected_issues, "issue", check_closing=False)

status_field_id = None
stage_field_id = None
start_field_id = None
target_field_id = None
todo_opt = None
in_prog_opt = None
stage_opts = {}
for f in fields:
    if not isinstance(f, dict):
        continue
    fname = f.get("name")
    if fname == "Status":
        status_field_id = f.get("id")
        for opt in f.get("options") or []:
            if opt.get("name") == "Todo":
                todo_opt = opt.get("id")
            elif opt.get("name") == "In Progress":
                in_prog_opt = opt.get("id")
    elif fname == "Stage":
        stage_field_id = f.get("id")
        for opt in f.get("options") or []:
            oname = opt.get("name", "")
            for idx in range(1, 7):
                if oname.startswith(f"{idx}."):
                    stage_opts[idx] = opt.get("id")
    elif fname == "Start Date":
        start_field_id = f.get("id")
    elif fname == "Target Date":
        target_field_id = f.get("id")

today = datetime.date.today()
def iso(days):
    return (today + datetime.timedelta(days=days)).isoformat()

stage_dates = {
    1: (iso(-3), iso(2)),
    2: (iso(-7), iso(3)),
    3: (iso(0),  iso(8)),
    4: (iso(8),  iso(18)),
    5: (iso(18), iso(32)),
    6: (iso(32), iso(46)),
}
ch4_7_re = re.compile(
    r"(rule|fusion|buffer|websocket|embedding|clustering|spatial|temporal|geojson|acme|maxdelivery|cli|datalog|inference|IndexEntry|hierarchy|sibling|query\.entity|path\s+search|PathSearch)",
    re.I,
)

claimed_issues = {7, 9}
for pr in prs:
    for ref in ((pr.get("closingIssuesReferences") or {}).get("nodes") or []):
        if ref and ref.get("number"):
            claimed_issues.add(ref["number"])

added = 0
status_mutations = []
mut_idx = 0

def ensure_item(node, is_pr, want_in_prog):
    global added, mut_idx
    item_id = None
    cur_status = None
    cur_stage = None
    cur_start = None
    cur_target = None
    for pi in ((node.get("projectItems") or {}).get("nodes") or []):
        if pi and (pi.get("project") or {}).get("number") == proj_num:
            item_id = pi.get("id")
            cur_status = (pi.get("status") or {}).get("name")
            cur_stage = (pi.get("stage") or {}).get("name")
            cur_start = (pi.get("startDate") or {}).get("date")
            cur_target = (pi.get("targetDate") or {}).get("date")
            break
    if not item_id:
        q = f'mutation {{ addProjectV2ItemById(input: {{projectId: "{proj_id}", contentId: "{node["id"]}"}}) {{ item {{ id }} }} }}'
        data = run_gql(q)
        item_id = ((data.get("addProjectV2ItemById") or {}).get("item") or {}).get("id")
        if not isinstance(item_id, str) or not item_id:
            die_unavail(f"addProjectV2ItemById for {node['id']} returned no item id")
        added += 1
    ms = ((node.get("milestone") or {}).get("title") or "").lower()
    title = node.get("title") or ""
    if is_pr or want_in_prog:
        st_idx = 1
    elif ms == "setup":
        st_idx = 2
    elif ms == "slice-04b":
        st_idx = 5
    elif ms == "slice-04c":
        st_idx = 6
    else:
        st_idx = 4 if ch4_7_re.search(title) else 3
    if status_field_id and item_id:
        want_opt = in_prog_opt if want_in_prog else todo_opt
        if want_opt and (cur_status is None or (want_in_prog and cur_status != "In Progress")):
            mut_idx += 1
            status_mutations.append(
                f'm{mut_idx}: updateProjectV2ItemFieldValue(input: {{projectId: "{proj_id}", itemId: "{item_id}", fieldId: "{status_field_id}", value: {{singleSelectOptionId: "{want_opt}"}}}}) {{ projectV2Item {{ id }} }}'
            )
    if stage_field_id and st_idx in stage_opts and (cur_stage is None or (st_idx == 1 and not cur_stage.startswith("1."))):
        mut_idx += 1
        status_mutations.append(
            f's{mut_idx}: updateProjectV2ItemFieldValue(input: {{projectId: "{proj_id}", itemId: "{item_id}", fieldId: "{stage_field_id}", value: {{singleSelectOptionId: "{stage_opts[st_idx]}"}}}}) {{ projectV2Item {{ id }} }}'
        )
    start_d, target_d = stage_dates[st_idx]
    if start_field_id and cur_start is None:
        mut_idx += 1
        status_mutations.append(
            f'd1_{mut_idx}: updateProjectV2ItemFieldValue(input: {{projectId: "{proj_id}", itemId: "{item_id}", fieldId: "{start_field_id}", value: {{date: "{start_d}"}}}}) {{ projectV2Item {{ id }} }}'
        )
    if target_field_id and cur_target is None:
        mut_idx += 1
        status_mutations.append(
            f'd2_{mut_idx}: updateProjectV2ItemFieldValue(input: {{projectId: "{proj_id}", itemId: "{item_id}", fieldId: "{target_field_id}", value: {{date: "{target_d}"}}}}) {{ projectV2Item {{ id }} }}'
        )

for pr in prs:
    ensure_item(pr, True, True)
for iss in issues:
    ensure_item(iss, False, iss.get("number") in claimed_issues)

for i in range(0, len(status_mutations), 25):
    chunk = status_mutations[i:i+25]
    q = "mutation {\n  " + "\n  ".join(chunk) + "\n}"
    run_gql(q)

total = len(prs) + len(issues)
indexed = (proj_view.get("items") or {}).get("totalCount", 0)
print(f"Synced {total} open items ({added} added, {len(status_mutations)} status updates) to C360Studio project #{proj_num}.")
if indexed < total:
    print(f"Note: all {total} items are attached on their issues/PRs; GitHub's async ProjectV2 board index currently shows {indexed}/{total} (check https://www.githubstatus.com if GitHub Projects indexing is delayed).")
PY
  status=$?
  [ "$status" -eq 0 ] || exit "$status"
fi
