package toolcheck

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// port-status › "Port scoreboard from the admission ledger" and "Issue and milestone hygiene check":
// scripts/port-status.sh reads docs/admission-ledger.yaml and the tree in --offline mode, and checks
// milestones and open issues via gh in --check mode.

type statusRun struct {
	stdout, stderr string
	ghLog          string
	status         int
}

func runPortStatus(t *testing.T, ghScript string, args ...string) statusRun {
	t.Helper()
	root := copyScript(t, "port-status.sh")
	for _, rel := range []string{
		"docs/admission-ledger.yaml",
		"service/base.go",
		"service/service_manager.go",
		"service/component_manager.go",
	} {
		src := filepath.Join(repoRoot(t), filepath.FromSlash(rel))
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(root, "fakebin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(root, "gh.log")
	if ghScript != "" {
		ghPath := filepath.Join(bin, "gh")
		script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> %q\n%s\n", logFile, ghScript)
		if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", append([]string{filepath.Join("scripts", "port-status.sh")}, args...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	logData, _ := os.ReadFile(logFile)
	r := statusRun{stdout: stdout.String(), stderr: stderr.String(), ghLog: string(logData)}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		r.status = exit.ExitCode()
	case err != nil:
		t.Fatalf("run port-status.sh: %v", err)
	}
	return r
}

// port-status › "Port scoreboard from the admission ledger": --offline prints all seven Slice 04A
// changes, the service PR 1-7 breakdown, and the 65-package summary without calling gh.
func TestPortStatusOffline(t *testing.T) {
	r := runPortStatus(t, `echo "unexpected gh call: $*" >&2; exit 3`, "--offline")
	if r.status != 0 {
		t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
	}
	for _, want := range []string{
		"setup-04a-01-floor",
		"setup-04a-02-ingest-kernel",
		"setup-04a-03-boot",
		"setup-04a-04-graph-roots",
		"setup-04a-05-first-consumer",
		"setup-04a-06-rule-core",
		"setup-04a-07-substrate-seam",
		"Service PR 1/7",
		"Service PR 4/7",
		"present in tree",
		"/ 65 tier-0 packages",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("offline stdout missing %q:\n%s", want, r.stdout)
		}
	}
	for _, unwanted := range []string{"merged on main", "PRs on main"} {
		if strings.Contains(r.stdout, unwanted) {
			t.Errorf("offline stdout must not claim local files are %q:\n%s", unwanted, r.stdout)
		}
	}
	// The floor cut pkg/acme and its ACME loaders; the internal/tlsutil ledger row defers both to
	// change 5, so the scoreboard counts pkg/acme there and not against change 1. The remaining
	// list is printed only for short lists, so the check reads each change's package total.
	floor := between(r.stdout, "1. setup-04a-01-floor", "2. setup-04a-02-ingest-kernel")
	consumer := between(r.stdout, "5. setup-04a-05-first-consumer", "6. setup-04a-06-rule-core")
	if !strings.Contains(floor, "/15 ") || strings.Contains(floor, "pkg/acme") {
		t.Errorf("change 1 should count 15 packages without pkg/acme:\n%s", floor)
	}
	if !strings.Contains(consumer, "/9 ") {
		t.Errorf("change 5 should count 9 packages, pkg/acme among them:\n%s", consumer)
	}
}

// between returns the text of s from the start of from up to the start of to, or "" when either is missing.
func between(s, from, to string) string {
	i := strings.Index(s, from)
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i:], to)
	if j < 0 {
		return ""
	}
	return s[i : i+j]
}

const cleanMilestonesJSON = `[{"number":1,"title":"setup","open_issues":9,"closed_issues":15},{"number":2,"title":"slice-04a","open_issues":72,"closed_issues":63},{"number":3,"title":"slice-04b","open_issues":1,"closed_issues":0},{"number":4,"title":"slice-04c","open_issues":1,"closed_issues":0}]`

const cleanIssuesJSON = `[{"number":9,"title":"epic: Slice 04A","milestone":{"title":"slice-04a"},"labels":[{"name":"type:epic"}]},{"number":372,"title":"service flake","milestone":{"title":"slice-04a"},"labels":[{"name":"class:flake"}]}]`

const cleanPRsJSON = `[{"number":367,"title":"feat(service): port message logger","isDraft":true,"headRefName":"claude/port-service-message-logger","milestone":{"title":"slice-04a"},"labels":[{"name":"go"}]}]`

func fakeGHScript(milestones, issues, prs string) string {
	return `case "$*" in
  *milestones*) printf '%s\n' '` + milestones + `' ;;
  *"issue list"*) printf '%s\n' '` + issues + `' ;;
  *"pr list"*) printf '%s\n' '` + prs + `' ;;
  *) echo "unexpected gh call: $*" >&2; exit 3 ;;
esac`
}

// port-status › "Issue and milestone hygiene check": --check exits 0 when milestones and issues are
// clean, exits 1 when an issue lacks a milestone or label or a milestone title contains whitespace,
// and exits 2 when a gh read fails.
func TestPortStatusCheck(t *testing.T) {
	t.Run("clean milestones and issues pass", func(t *testing.T) {
		r := runPortStatus(t, fakeGHScript(cleanMilestonesJSON, cleanIssuesJSON, cleanPRsJSON), "--check")
		if r.status != 0 {
			t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stdout, "hygiene: ok") {
			t.Errorf("stdout missing 'hygiene: ok':\n%s", r.stdout)
		}
	})

	t.Run("issue missing milestone fails with exit 1", func(t *testing.T) {
		issues := `[{"number":368,"title":"flowgraph whitespace","milestone":null,"labels":[{"name":"bug"}]}]`
		r := runPortStatus(t, fakeGHScript(cleanMilestonesJSON, issues, cleanPRsJSON), "--check")
		if r.status != 1 {
			t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stdout, "#368") || !strings.Contains(r.stdout, "milestone") {
			t.Errorf("stdout should name #368 and milestone:\n%s", r.stdout)
		}
	})

	t.Run("issue missing labels fails with exit 1", func(t *testing.T) {
		issues := `[{"number":369,"title":"subject grammar","milestone":{"title":"slice-04a"},"labels":[]}]`
		r := runPortStatus(t, fakeGHScript(cleanMilestonesJSON, issues, cleanPRsJSON), "--check")
		if r.status != 1 {
			t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stdout, "#369") || !strings.Contains(r.stdout, "label") {
			t.Errorf("stdout should name #369 and label:\n%s", r.stdout)
		}
	})

	t.Run("milestone title with whitespace fails with exit 1", func(t *testing.T) {
		ms := `[{"number":2,"title":"Slice 04A: tier-0","open_issues":1,"closed_issues":0}]`
		r := runPortStatus(t, fakeGHScript(ms, cleanIssuesJSON, cleanPRsJSON), "--check")
		if r.status != 1 {
			t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stdout, "Slice 04A: tier-0") {
			t.Errorf("stdout should name offending milestone title:\n%s", r.stdout)
		}
	})

	t.Run("gh read failure exits 2", func(t *testing.T) {
		r := runPortStatus(t, `echo "gh: network unreachable" >&2; exit 1`, "--check")
		if r.status != 2 {
			t.Fatalf("exit %d, want 2\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stderr, "unavailable") {
			t.Errorf("stderr should report unavailable read:\n%s", r.stderr)
		}
	})
}

func makeIssuesJSON(n int) (string, string, string) {
	var listItems, gqlPage1, gqlPage2 []string
	for i := 1; i <= n; i++ {
		listItems = append(listItems, fmt.Sprintf(`{"number":%d,"title":"issue %d","milestone":{"title":"slice-04a"},"labels":[{"name":"bug"}]}`, i, i))
		node := fmt.Sprintf(`{"id":"I%d","number":%d,"title":"issue %d","milestone":{"title":"slice-04a"},"projectItems":{"nodes":[]}}`, i, i, i)
		if i <= 100 {
			gqlPage1 = append(gqlPage1, node)
		} else {
			gqlPage2 = append(gqlPage2, node)
		}
	}
	return "[" + strings.Join(listItems, ",") + "]",
		"[" + strings.Join(gqlPage1, ",") + "]",
		"[" + strings.Join(gqlPage2, ",") + "]"
}

// port-status › "GitHub Project v2 sync": --sync-project selects the exact "SemEngine Port Tracker"
// project (or explicit number), refuses malformed discovery or truncated GraphQL collections before
// any write, paginates open issues/PRs beyond 100, and updates item fields.
func TestPortStatusSyncProject(t *testing.T) {
	t.Run("selects exact SemEngine Port Tracker title over substring match", func(t *testing.T) {
		gh := `case "$*" in
  *milestones*) printf '%s\n' '` + cleanMilestonesJSON + `' ;;
  *"issue list"*) printf '%s\n' '` + cleanIssuesJSON + `' ;;
  *"pr list"*) printf '%s\n' '` + cleanPRsJSON + `' ;;
  *"project list"*) printf '%s\n' '{"projects":[{"number":99,"title":"SemEngine experiments"},{"number":7,"title":"SemEngine Port Tracker"}],"totalCount":2}' ;;
  *"project field-list 7"*) printf '%s\n' '{"fields":[{"id":"F_STATUS","name":"Status","options":[{"id":"OPT_TODO","name":"Todo"},{"id":"OPT_INPROG","name":"In Progress"}]},{"id":"F_STAGE","name":"Stage","options":[{"id":"OPT_S1","name":"1. In Progress"},{"id":"OPT_S3","name":"3. Slice 04A"}]},{"id":"F_START","name":"Start Date"},{"id":"F_TARGET","name":"Target Date"}]}' ;;
  *"project view 7"*) printf '%s\n' '{"id":"P7","items":{"totalCount":3}}' ;;
  *addProjectV2ItemById*) printf '%s\n' '{"data":{"addProjectV2ItemById":{"item":{"id":"PVTI_NEW"}}}}' ;;
  *updateProjectV2ItemFieldValue*) printf '%s\n' '{"data":{"m1":{"projectV2Item":{"id":"PVTI_NEW"}}}}' ;;
  *graphql*) printf '%s\n' '{"data":{"repository":{"pullRequests":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"id":"PR367","number":367,"title":"port message logger","milestone":{"title":"slice-04a"},"closingIssuesReferences":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"projectItems":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]},"issues":{"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"id":"I9","number":9,"title":"epic","milestone":{"title":"slice-04a"},"projectItems":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}},{"id":"I372","number":372,"title":"service flake","milestone":{"title":"slice-04a"},"projectItems":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}}' ;;
  *) echo "unexpected gh call: $*" >&2; exit 3 ;;
esac`
		r := runPortStatus(t, gh, "--sync-project")
		if r.status != 0 {
			t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s\nghLog:\n%s", r.status, r.stdout, r.stderr, r.ghLog)
		}
		if !strings.Contains(r.stdout, "project #7") {
			t.Errorf("expected sync to select project #7:\n%s", r.stdout)
		}
		if strings.Contains(r.ghLog, "field-list 99") || strings.Contains(r.ghLog, "view 99") {
			t.Errorf("must not touch substring-matched project #99:\n%s", r.ghLog)
		}
		if !strings.Contains(r.ghLog, "updateProjectV2ItemFieldValue") {
			t.Errorf("expected field update mutations in gh log:\n%s", r.ghLog)
		}
	})

	t.Run("malformed project list JSON exits 2 with zero writes", func(t *testing.T) {
		gh := `case "$*" in
  *milestones*) printf '%s\n' '` + cleanMilestonesJSON + `' ;;
  *"issue list"*) printf '%s\n' '` + cleanIssuesJSON + `' ;;
  *"pr list"*) printf '%s\n' '` + cleanPRsJSON + `' ;;
  *"project list"*) printf '%s\n' 'not json' ;;
  *) echo "unexpected write/follow-up gh call: $*" >&2; exit 3 ;;
esac`
		r := runPortStatus(t, gh, "--sync-project")
		if r.status != 2 {
			t.Fatalf("exit %d, want 2\nstdout:\n%s\nstderr:\n%s\nghLog:\n%s", r.status, r.stdout, r.stderr, r.ghLog)
		}
		if !strings.Contains(r.stderr, "unavailable") {
			t.Errorf("stderr should report unavailable discovery:\n%s", r.stderr)
		}
		if strings.Contains(r.ghLog, "project create") || strings.Contains(r.ghLog, "mutation") {
			t.Errorf("malformed discovery must not trigger project create or mutations:\n%s", r.ghLog)
		}
	})

	t.Run("refuses truncated GraphQL issues before any write", func(t *testing.T) {
		issuesList, gqlPage1, _ := makeIssuesJSON(101)
		gh := `case "$*" in
  *milestones*) printf '%s\n' '` + cleanMilestonesJSON + `' ;;
  *"issue list"*) printf '%s\n' '` + issuesList + `' ;;
  *"pr list"*) printf '%s\n' '[]' ;;
  *"project list"*) printf '%s\n' '{"projects":[{"number":7,"title":"SemEngine Port Tracker"}],"totalCount":1}' ;;
  *"project field-list 7"*) printf '%s\n' '{"fields":[]}' ;;
  *"project view 7"*) printf '%s\n' '{"id":"P7","items":{"totalCount":0}}' ;;
  *addProjectV2ItemById*) printf '%s\n' '{"data":{"addProjectV2ItemById":{"item":{"id":"PVTI_NEW"}}}}' ;;
  *graphql*) printf '%s\n' '{"data":{"repository":{"pullRequests":{"nodes":[]},"issues":{"pageInfo":{"hasNextPage":true},"nodes":` + gqlPage1 + `}}}}' ;;
  *) echo "unexpected gh call: $*" >&2; exit 3 ;;
esac`
		r := runPortStatus(t, gh, "--sync-project")
		if r.status != 2 {
			t.Fatalf("exit %d, want 2 on truncated GraphQL issues\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if strings.Contains(r.ghLog, "addProjectV2ItemById") {
			t.Errorf("truncated GraphQL read must fail before any item mutation:\n%s", r.ghLog)
		}
	})

	t.Run("paginates multi-page issues beyond first 100", func(t *testing.T) {
		issuesList, gqlPage1, gqlPage2 := makeIssuesJSON(101)
		gh := `case "$*" in
  *milestones*) printf '%s\n' '` + cleanMilestonesJSON + `' ;;
  *"issue list"*) printf '%s\n' '` + issuesList + `' ;;
  *"pr list"*) printf '%s\n' '[]' ;;
  *"project list"*) printf '%s\n' '{"projects":[{"number":7,"title":"SemEngine Port Tracker"}],"totalCount":1}' ;;
  *"project field-list 7"*) printf '%s\n' '{"fields":[]}' ;;
  *"project view 7"*) printf '%s\n' '{"id":"P7","items":{"totalCount":101}}' ;;
  *addProjectV2ItemById*) printf '%s\n' '{"data":{"addProjectV2ItemById":{"item":{"id":"PVTI_NEW"}}}}' ;;
  *cursor-100*) printf '%s\n' '{"data":{"repository":{"issues":{"totalCount":101,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":` + gqlPage2 + `}}}}' ;;
  *graphql*) printf '%s\n' '{"data":{"repository":{"pullRequests":{"totalCount":0,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]},"issues":{"totalCount":101,"pageInfo":{"hasNextPage":true,"endCursor":"cursor-100"},"nodes":` + gqlPage1 + `}}}}' ;;
  *) echo "unexpected gh call: $*" >&2; exit 3 ;;
esac`
		r := runPortStatus(t, gh, "--sync-project")
		if r.status != 0 {
			t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stdout, "Synced 101 open items") {
			t.Errorf("expected all 101 open items synced:\n%s", r.stdout)
		}
		if !strings.Contains(r.ghLog, `contentId: "I101"`) {
			t.Errorf("expected mutation for page-2 item I101 in gh log:\n%s", r.ghLog)
		}
	})

	t.Run("refuses ambiguous duplicate SemEngine Port Tracker projects before any write", func(t *testing.T) {
		gh := `case "$*" in
  *milestones*) printf '%s\n' '` + cleanMilestonesJSON + `' ;;
  *"issue list"*) printf '%s\n' '` + cleanIssuesJSON + `' ;;
  *"pr list"*) printf '%s\n' '` + cleanPRsJSON + `' ;;
  *"project list"*) printf '%s\n' '{"projects":[{"number":4,"title":"SemEngine Port Tracker"},{"number":7,"title":"SemEngine Port Tracker"}],"totalCount":2}' ;;
  *) echo "unexpected gh call: $*" >&2; exit 3 ;;
esac`
		r := runPortStatus(t, gh, "--sync-project")
		if r.status != 2 {
			t.Fatalf("exit %d, want 2\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stderr, "ambiguous") {
			t.Errorf("stderr should report ambiguous match:\n%s", r.stderr)
		}
	})

	t.Run("malformed project create JSON exits 2 before any item write", func(t *testing.T) {
		gh := `case "$*" in
  *milestones*) printf '%s\n' '` + cleanMilestonesJSON + `' ;;
  *"issue list"*) printf '%s\n' '` + cleanIssuesJSON + `' ;;
  *"pr list"*) printf '%s\n' '` + cleanPRsJSON + `' ;;
  *"project list"*) printf '%s\n' '{"projects":[],"totalCount":0}' ;;
  *"project create"*) printf '%s\n' 'not json' ;;
  *) echo "unexpected gh call: $*" >&2; exit 3 ;;
esac`
		r := runPortStatus(t, gh, "--sync-project")
		if r.status != 2 {
			t.Fatalf("exit %d, want 2\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if strings.Contains(r.ghLog, "project link") || strings.Contains(r.ghLog, "graphql") {
			t.Errorf("malformed create response must not proceed to link or graphql:\n%s", r.ghLog)
		}
	})
}
