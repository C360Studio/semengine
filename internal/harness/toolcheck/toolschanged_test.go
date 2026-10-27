package toolcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTask stands in for task: it records its arguments, so a case can tell whether
// scripts/tools-changed.sh ran task test:tools.
const fakeTask = `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$FAKE_TASK_LOG"
`

// TestToolsChanged (merge-gate › "Tool tests run when their files change" and "How the changed
// files are found"): scripts/tools-changed.sh runs task test:tools when a changed file is under a
// trigger path, and otherwise says the tool tests were skipped. Each case plants a git repository
// whose base commit holds the script and one file under scripts/ and one outside every trigger
// path. GITHUB_ACTIONS, GITHUB_EVENT_NAME and GITHUB_BASE_REF are removed from the inherited
// environment (CI sets them) and set only as the case says.
func TestToolsChanged(t *testing.T) {
	for _, tc := range []struct {
		name string
		// baseRef is the remote-tracking branch pointed at the base commit; "" plants none.
		baseRef string
		// change runs in the repository after the base commit.
		change  func(t *testing.T, root string)
		env     []string
		wantRun bool
		want    string
	}{
		{
			name:    "a script change",
			baseRef: "main",
			change: func(t *testing.T, root string) {
				plant(t, root, map[string]string{"scripts/merge-check.sh": "echo changed\n"})
				gitIn(t, root, "commit", "-q", "-am", "change")
			},
			wantRun: true,
			want:    "scripts/merge-check.sh",
		},
		{
			name:    "docs and product code only",
			baseRef: "main",
			change: func(t *testing.T, root string) {
				plant(t, root, map[string]string{"graph/query_verbs.go": "package graph\n", "docs/testing.md": "# Testing\n"})
				gitIn(t, root, "add", ".")
				gitIn(t, root, "commit", "-q", "-m", "change")
			},
			want: "tool tests skipped: no changed file under",
		},
		{
			name:    "an untracked file under a trigger path",
			baseRef: "main",
			change: func(t *testing.T, root string) {
				plant(t, root, map[string]string{"internal/harness/new_test.go": "package harness\n"})
			},
			wantRun: true,
			want:    "internal/harness/new_test.go",
		},
		{
			name:    "an uncommitted go.mod edit",
			baseRef: "main",
			change: func(t *testing.T, root string) {
				plant(t, root, map[string]string{"go.mod": "module example.com/m\n\ngo 1.23\n"})
			},
			wantRun: true,
			want:    "go.mod",
		},
		{
			name:    "a rename out of scripts/",
			baseRef: "main",
			change: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, "tools"), 0o755); err != nil {
					t.Fatal(err)
				}
				gitIn(t, root, "mv", "scripts/merge-check.sh", "tools/merge-check.sh")
				gitIn(t, root, "commit", "-q", "-m", "move")
			},
			wantRun: true,
			want:    "scripts/merge-check.sh",
		},
		{
			name:    "no origin/main",
			wantRun: true,
			want:    "no merge base",
		},
		{
			name:    "a CI push",
			baseRef: "main",
			env:     []string{"GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=push"},
			wantRun: true,
			want:    "push",
		},
		{
			// The base branch is not main and origin/main is absent, so a skip shows the script
			// compared against origin/$GITHUB_BASE_REF.
			name:    "a CI pull request with docs only",
			baseRef: "release",
			change: func(t *testing.T, root string) {
				plant(t, root, map[string]string{"docs/testing.md": "# Testing\n"})
				gitIn(t, root, "add", ".")
				gitIn(t, root, "commit", "-q", "-m", "change")
			},
			env:  []string{"GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=pull_request", "GITHUB_BASE_REF=release"},
			want: "tool tests skipped: no changed file under",
		},
		{
			name:    "an unknown event",
			baseRef: "main",
			env:     []string{"GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=workflow_dispatch"},
			wantRun: true,
			want:    "workflow_dispatch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := copyScript(t, "tools-changed.sh")
			plant(t, root, map[string]string{
				"go.mod":                 "module example.com/m\n\ngo 1.22\n",
				"scripts/merge-check.sh": "echo base\n",
				"README.md":              "# m\n",
			})
			gitIn(t, root, "init", "-q")
			gitIn(t, root, "add", ".")
			gitIn(t, root, "commit", "-q", "-m", "base")
			if tc.baseRef != "" {
				gitIn(t, root, "update-ref", "refs/remotes/origin/"+tc.baseRef, "HEAD")
			}
			if tc.change != nil {
				tc.change(t, root)
			}
			log := filepath.Join(t.TempDir(), "task.log")
			var env []string
			for _, kv := range fakeBin(t, map[string]string{"task": fakeTask}, "FAKE_TASK_LOG="+log) {
				if strings.HasPrefix(kv, "GITHUB_ACTIONS=") || strings.HasPrefix(kv, "GITHUB_EVENT_NAME=") ||
					strings.HasPrefix(kv, "GITHUB_BASE_REF=") {
					continue
				}
				env = append(env, kv)
			}
			out, status := runIn(t, root, append(env, tc.env...), "tools-changed.sh")
			if status != 0 {
				t.Fatalf("exit %d, want 0\n%s", status, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("output does not contain %q:\n%s", tc.want, out)
			}
			ran, err := os.ReadFile(log)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			switch {
			case tc.wantRun && string(ran) != "test:tools\n":
				t.Errorf("task ran with %q, want exactly one run of test:tools\n%s", ran, out)
			case !tc.wantRun && len(ran) > 0:
				t.Errorf("task ran with %q, want no run\n%s", ran, out)
			}
		})
	}
}
