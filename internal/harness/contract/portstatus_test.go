package contract

import (
	"errors"
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
	if ghScript != "" {
		ghPath := filepath.Join(bin, "gh")
		if err := os.WriteFile(ghPath, []byte("#!/bin/sh\n"+ghScript+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", append([]string{filepath.Join("scripts", "port-status.sh")}, args...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	r := statusRun{stdout: stdout.String(), stderr: stderr.String()}
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
		"/ 65 tier-0 packages",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("offline stdout missing %q:\n%s", want, r.stdout)
		}
	}
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
