package contract

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// merge-gate › "Varied and repeated unit runs": the two unit invocations task verify runs, held to
// the exact command lines of the spec. test:unit runs once under the race detector at one CPU;
// test:repeat runs five times without it at one CPU in shuffled order, over ./... or the packages
// given after --. Neither line is read from the file under test: these constants are the spec.
const (
	requiredUnitCmd   = "scripts/gopkgs.sh go test -race -count=1 -cpu 1 ./..."
	requiredRepeatCmd = `scripts/gopkgs.sh go test -count=5 -cpu 1 -shuffle=on {{.CLI_ARGS | default "./..."}}`
)

func TestUnitInvocationsPinned(t *testing.T) {
	root := repoRoot(t)
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	requireNoViolations(t, "unit invocation", unitInvocationViolations(read("Taskfile.yml"), read("scripts/verify.sh")))
}

func TestUnitInvocationsPinnedSensitivity(t *testing.T) {
	taskfile := func(unit, repeat string) []byte {
		return []byte("version: '3'\ntasks:\n  test:unit:\n    cmds:\n      - " + unit +
			"\n  test:repeat:\n    cmds:\n      - '" + repeat + "'\n")
	}
	verify := func(steps string) []byte {
		return []byte("#!/usr/bin/env bash\nsteps=(spec:check build\n  test:unit test:integration " + steps + ")\n")
	}
	goodTaskfile, goodVerify := taskfile(requiredUnitCmd, requiredRepeatCmd), verify("cover:check test:repeat")
	requireNoViolations(t, "clean fixture", unitInvocationViolations(goodTaskfile, goodVerify))

	for _, tc := range []struct {
		name             string
		taskfile, verify []byte
		wants            []string
	}{
		{"count lowered", taskfile(requiredUnitCmd, strings.Replace(requiredRepeatCmd, "-count=5", "-count=1", 1)), goodVerify,
			[]string{"test:repeat", requiredRepeatCmd}},
		{"cpu setting dropped", taskfile(strings.Replace(requiredUnitCmd, " -cpu 1", "", 1), requiredRepeatCmd), goodVerify,
			[]string{"test:unit", requiredUnitCmd}},
		{"race added to repeat", taskfile(requiredUnitCmd, strings.Replace(requiredRepeatCmd, "go test", "go test -race", 1)), goodVerify,
			[]string{"test:repeat", requiredRepeatCmd}},
		{"repeat task missing", []byte("version: '3'\ntasks:\n  test:unit:\n    cmds:\n      - " + requiredUnitCmd + "\n"), goodVerify,
			[]string{"test:repeat", "no such task"}},
		{"step removed from verify", goodTaskfile, verify("cover:check"),
			[]string{"scripts/verify.sh", "test:repeat", "missing"}},
		{"repeat step not last", goodTaskfile, verify("test:repeat cover:check"),
			[]string{"scripts/verify.sh", "test:repeat must be the last step", "cover:check"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireViolation(t, unitInvocationViolations(tc.taskfile, tc.verify), tc.wants...)
		})
	}
}

var verifySteps = regexp.MustCompile(`(?s)\bsteps=\(([^)]*)\)`)

// unitInvocationViolations checks the two task commands in a Taskfile and the step list of a
// verify.sh against the spec.
func unitInvocationViolations(taskfile, verify []byte) []string {
	var violations []string
	var tf struct {
		Tasks map[string]struct {
			Cmds []any `yaml:"cmds"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &tf); err != nil {
		return []string{fmt.Sprintf("Taskfile.yml: %v", err)}
	}
	for _, want := range []struct{ task, cmd string }{{"test:unit", requiredUnitCmd}, {"test:repeat", requiredRepeatCmd}} {
		task, ok := tf.Tasks[want.task]
		if !ok {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: no such task %s; the merge-gate spec requires it to run `%s`", want.task, want.cmd))
			continue
		}
		if len(task.Cmds) != 1 || task.Cmds[0] != want.cmd {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: task %s runs %v; the merge-gate spec requires exactly `%s`", want.task, task.Cmds, want.cmd))
		}
	}

	m := verifySteps.FindSubmatch(verify)
	if m == nil {
		return append(violations, "scripts/verify.sh: no steps=(...) list")
	}
	steps := strings.Fields(string(m[1]))
	for _, step := range []string{"test:unit", "test:repeat"} {
		if !slices.Contains(steps, step) {
			violations = append(violations, fmt.Sprintf("scripts/verify.sh: step %s is missing from %v", step, steps))
		}
	}
	if i := slices.Index(steps, "test:repeat"); i >= 0 && i != len(steps)-1 {
		violations = append(violations, fmt.Sprintf("scripts/verify.sh: test:repeat must be the last step; %v follow it", steps[i+1:]))
	}
	return violations
}

// merge-gate › "Required needs both jobs": .github/workflows/ci.yml held to four facts. The job
// merge-check runs under exactly three read permissions; no permission anywhere in the workflow is
// a write; required needs verify and merge-check; verify's limit is 15 minutes. As above, the
// expected values are constants here, never read from the file under test.
var (
	requiredNeeds         = []string{"verify", "merge-check"}
	mergeCheckPermissions = map[string]string{"contents": "read", "issues": "read", "pull-requests": "read"}
)

const verifyTimeoutMinutes = 15

func TestCIWorkflowPinned(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	requireNoViolations(t, "ci.yml", ciWorkflowViolations(data))
}

func TestCIWorkflowPinnedSensitivity(t *testing.T) {
	const good = `on: [push]
permissions:
  contents: read
jobs:
  verify:
    timeout-minutes: 15
    steps: [{run: task verify}]
  merge-check:
    timeout-minutes: 5
    permissions:
      contents: read
      issues: read
      pull-requests: read
    steps: [{run: scripts/merge-check.sh}]
  required:
    needs: [verify, merge-check]
    if: always()
    steps: [{run: "true"}]
`
	requireNoViolations(t, "clean fixture", ciWorkflowViolations([]byte(good)))

	plant := func(t *testing.T, old, repl string) []byte {
		t.Helper()
		if !strings.Contains(good, old) {
			t.Fatalf("fixture lacks %q", old)
		}
		return []byte(strings.Replace(good, old, repl, 1))
	}
	for _, tc := range []struct {
		name, old, repl string
		wants           []string
	}{
		// The four planted workflows of tasks.md 7.4, one per scenario of the requirement.
		{"required needs only verify", "needs: [verify, merge-check]", "needs: [verify]",
			[]string{"job required", "merge-check", "missing"}},
		{"write permission on a job", "    timeout-minutes: 15\n", "    timeout-minutes: 15\n    permissions:\n      issues: write\n",
			[]string{"job verify", "issues: write"}},
		{"fourth permission on merge-check", "      pull-requests: read\n", "      pull-requests: read\n      actions: read\n",
			[]string{"job merge-check", "actions: read"}},
		{"verify limit changed", "timeout-minutes: 15", "timeout-minutes: 30",
			[]string{"job verify", "30", "15 minutes"}},
		// The same holes by other spellings.
		{"required needs a single string", "needs: [verify, merge-check]", "needs: verify",
			[]string{"job required", "merge-check", "missing"}},
		{"write-all on the workflow", "permissions:\n  contents: read\njobs:", "permissions: write-all\njobs:",
			[]string{"workflow", "write-all"}},
		{"write permission on merge-check", "      issues: read\n", "      issues: write\n",
			[]string{"job merge-check", "issues: write"}},
		{"merge-check permissions inherited", "    permissions:\n      contents: read\n      issues: read\n      pull-requests: read\n", "",
			[]string{"job merge-check", "no permissions of its own"}},
		{"merge-check permission dropped", "      issues: read\n", "",
			[]string{"job merge-check", "issues: read", "missing"}},
		{"verify limit removed", "    timeout-minutes: 15\n", "",
			[]string{"job verify", "15 minutes"}},
		{"merge-check job removed", "  merge-check:\n", "  merge-chek:\n",
			[]string{"no job merge-check"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireViolation(t, ciWorkflowViolations(plant(t, tc.old, tc.repl)), tc.wants...)
		})
	}
}

type ciJob struct {
	Needs          any `yaml:"needs"`
	Permissions    any `yaml:"permissions"`
	TimeoutMinutes any `yaml:"timeout-minutes"`
}

// ciWorkflowViolations checks a GitHub Actions workflow against the merge-gate spec.
func ciWorkflowViolations(data []byte) []string {
	var wf struct {
		Permissions any              `yaml:"permissions"`
		Jobs        map[string]ciJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return []string{fmt.Sprintf("ci.yml: %v", err)}
	}
	var violations []string

	// No write anywhere: the workflow's default and every job's own grant.
	violations = append(violations, writeGrants("workflow", wf.Permissions)...)
	names := make([]string, 0, len(wf.Jobs))
	for name := range wf.Jobs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		violations = append(violations, writeGrants("job "+name, wf.Jobs[name].Permissions)...)
	}

	if mc, ok := wf.Jobs["merge-check"]; !ok {
		violations = append(violations, "ci.yml: no job merge-check; the merge-gate spec requires it")
	} else {
		violations = append(violations, mergeCheckPermissionViolations(mc.Permissions)...)
	}

	if req, ok := wf.Jobs["required"]; !ok {
		violations = append(violations, "ci.yml: no job required; the merge-gate spec requires it")
	} else {
		var needs []string
		switch n := req.Needs.(type) {
		case string:
			needs = []string{n}
		case []any:
			for _, v := range n {
				needs = append(needs, fmt.Sprint(v))
			}
		}
		for _, want := range requiredNeeds {
			if !slices.Contains(needs, want) {
				violations = append(violations, fmt.Sprintf("ci.yml: job required needs %v; %s is missing, and the merge-gate spec requires it to need %v", needs, want, requiredNeeds))
			}
		}
	}

	if v, ok := wf.Jobs["verify"]; !ok {
		violations = append(violations, "ci.yml: no job verify; the merge-gate spec requires it")
	} else if limit, ok := v.TimeoutMinutes.(int); !ok || limit != verifyTimeoutMinutes {
		violations = append(violations, fmt.Sprintf("ci.yml: job verify has timeout-minutes %v; the merge-gate spec requires %d minutes", v.TimeoutMinutes, verifyTimeoutMinutes))
	}
	return violations
}

// writeGrants names every write in a permissions value: a write-all string, or a scope set to write.
func writeGrants(where string, perms any) []string {
	switch p := perms.(type) {
	case string:
		if p != "read-all" {
			return []string{fmt.Sprintf("ci.yml: %s is granted permissions: %s; the merge-gate spec allows no write", where, p)}
		}
	case map[string]any:
		var out []string
		for _, scope := range slices.Sorted(maps.Keys(p)) {
			if level := fmt.Sprint(p[scope]); level == "write" {
				out = append(out, fmt.Sprintf("ci.yml: %s is granted %s: %s; the merge-gate spec allows no write", where, scope, level))
			}
		}
		return out
	}
	return nil
}

// mergeCheckPermissionViolations requires the merge-check job's own grant to be exactly the three
// reads. Inheriting the workflow's default does not count: the job's token is pinned where the job is.
func mergeCheckPermissionViolations(perms any) []string {
	p, ok := perms.(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("ci.yml: job merge-check has no permissions of its own (%v); the merge-gate spec requires exactly %v", perms, mergeCheckPermissions)}
	}
	var out []string
	for _, scope := range slices.Sorted(maps.Keys(p)) {
		level := fmt.Sprint(p[scope])
		if mergeCheckPermissions[scope] != level {
			out = append(out, fmt.Sprintf("ci.yml: job merge-check is granted %s: %s; the merge-gate spec allows only %v", scope, level, mergeCheckPermissions))
		}
	}
	for _, scope := range slices.Sorted(maps.Keys(mergeCheckPermissions)) {
		if _, ok := p[scope]; !ok {
			out = append(out, fmt.Sprintf("ci.yml: job merge-check is missing %s: %s; the merge-gate spec requires exactly %v", scope, mergeCheckPermissions[scope], mergeCheckPermissions))
		}
	}
	return out
}
