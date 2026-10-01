package contract

import (
	"fmt"
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
