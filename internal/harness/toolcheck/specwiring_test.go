package toolcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// spec-queue › "spec:check runs validate then ateam state spec-check": spec:check runs openspec
// validate, then ateam's check; spec:queue runs ateam's queue alone; nothing in either can skip a
// command or discard its exit status. The command lines are the spec's, written here, not read from
// the file under test. The queue and the check themselves are tested in ateam at the pinned release.
const (
	specValidateCmd = `npx --no-install openspec validate --all --strict --no-interactive`
	specCheckCmd    = `ateam state spec-check`
	specQueueCmd    = `ateam state spec-queue`
)

// specTasks names each Task entry this test guards and the commands it must run, in order.
var specTasks = []struct {
	task string
	cmds []string
}{
	{"spec:check", []string{specValidateCmd, specCheckCmd}},
	{"spec:queue", []string{specQueueCmd}},
}

func TestSpecTaskWiring(t *testing.T) {
	t.Run("Taskfile.yml", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), "Taskfile.yml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, st := range specTasks {
			requireNoViolations(t, st.task+" wiring", taskWiringViolations(data, st.task, st.cmds))
		}
	})

	taskfile := func(task, extra string, cmds ...string) []byte {
		s := "version: '3'\ntasks:\n  " + task + ":\n    desc: d\n" + extra + "    cmds:\n"
		for _, c := range cmds {
			s += "      - " + c + "\n"
		}
		return []byte(s)
	}
	checkWants := []string{specValidateCmd, specCheckCmd}
	queueWants := []string{specQueueCmd}
	t.Run("clean fixtures", func(t *testing.T) {
		requireNoViolations(t, "spec:check fixture",
			taskWiringViolations(taskfile("spec:check", "", specValidateCmd, specCheckCmd), "spec:check", checkWants))
		requireNoViolations(t, "spec:queue fixture",
			taskWiringViolations(taskfile("spec:queue", "", specQueueCmd), "spec:queue", queueWants))
	})
	for _, tc := range []struct {
		name  string
		task  string
		file  []byte
		cmds  []string
		wants []string
	}{
		{"check line removed", "spec:check", taskfile("spec:check", "", specValidateCmd), checkWants,
			[]string{"spec:check", "command 2 is nothing", `want "` + specCheckCmd + `"`}},
		{"the two lines swapped", "spec:check", taskfile("spec:check", "", specCheckCmd, specValidateCmd), checkWants,
			[]string{"spec:check", "command 1", `want "` + specValidateCmd + `"`}},
		{"ignore_error on the task", "spec:check",
			taskfile("spec:check", "    ignore_error: true\n", specValidateCmd, specCheckCmd), checkWants,
			[]string{"spec:check", `key "ignore_error"`}},
		{"a third command", "spec:check",
			taskfile("spec:check", "", specValidateCmd, specCheckCmd, "echo done"), checkWants,
			[]string{"spec:check", "runs 3 commands, want exactly 2"}},
		{"a command that is not a plain string", "spec:check",
			taskfile("spec:check", "", specValidateCmd, "{cmd: "+specCheckCmd+", ignore_error: true}"), checkWants,
			[]string{"spec:check", "command 2 is <not a plain command string>", `want "` + specCheckCmd + `"`}},
		{"the retired queue script", "spec:queue", taskfile("spec:queue", "", "scripts/openspec-queue.sh"), queueWants,
			[]string{"spec:queue", `command 1 is "scripts/openspec-queue.sh"`, `want "` + specQueueCmd + `"`}},
		{"no such task", "spec:queue", taskfile("spec:check", "", specValidateCmd, specCheckCmd), queueWants,
			[]string{"spec:queue", "no such task"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireViolation(t, taskWiringViolations(tc.file, tc.task, tc.cmds), tc.wants...)
		})
	}
}

// taskWiringViolations checks one task of a Taskfile: a mapping with only desc, summary and cmds,
// whose commands are exactly want, in order, each a plain string.
func taskWiringViolations(taskfile []byte, task string, want []string) []string {
	var tf struct {
		Tasks map[string]yaml.Node `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &tf); err != nil {
		return []string{"Taskfile.yml: parse: " + err.Error()}
	}
	node, ok := tf.Tasks[task]
	if !ok || node.Kind != yaml.MappingNode {
		return []string{"Taskfile.yml: " + task + ": no such task"}
	}
	// cmds holds each command as the messages show it: quoted, or a marker for a non-string.
	var violations, cmds []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		switch key, value := node.Content[i].Value, node.Content[i+1]; key {
		case "desc", "summary":
		case "cmds":
			for _, c := range value.Content {
				if c.Kind != yaml.ScalarNode {
					cmds = append(cmds, "<not a plain command string>")
					continue
				}
				cmds = append(cmds, strconv.Quote(c.Value))
			}
		default:
			violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: key %q can skip a command or discard its exit status; only desc, summary and cmds are allowed", task, key))
		}
	}
	for i, w := range want {
		got := "nothing"
		if i < len(cmds) {
			got = cmds[i]
		}
		if got != strconv.Quote(w) {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: command %d is %s, want %q", task, i+1, got, w))
		}
	}
	if len(cmds) > len(want) {
		violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: runs %d commands, want exactly %d: %s", task, len(cmds), len(want), strings.Join(cmds, ", ")))
	}
	return violations
}
