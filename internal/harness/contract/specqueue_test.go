package contract

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// spec-queue › all three requirements: scripts/openspec-queue.sh runs from a throwaway root with a
// stand-in openspec CLI and one planted tasks.md, as openspec/changes/fx/tasks.md. The stand-in
// answers `list --json` as the case says and refuses any other call, so a case cannot pass on an
// answer to another question. Its JSON has no lastModified, so no staleness note depends on the
// date. Expected lines are written by hand from each fixture, never computed by parsing it.

const openspecStandIn = `#!/bin/sh
[ "$*" = "list --json" ] || { echo "stand-in openspec: unexpected call: $*" >&2; exit 3; }
`

// The stand-in's answers to `list --json`.
const (
	listOneChange = `echo '{"changes":[{"name":"fx","completedTasks":0,"totalTasks":2}]}'`
	listNoChange  = `echo '{"changes":[]}'`
	listFails     = `echo 'stand-in openspec: list failed' >&2; exit 1`
	listNotJSON   = `echo 'not json'`
)

// The message the check prints after "<path>:<line>: " for each misplaced hold (design D9).
const holdOutsideMsg = "Hold: outside every task; task spec:queue cannot show it. Put it in the task it stops."

type queueRun struct {
	stdout, stderr string
	status         int
}

// runSpecQueue copies the script into a throwaway root, plants the stand-in CLI with the given
// answer to `list --json` and tasks as openspec/changes/fx/tasks.md, and runs the script with
// bash and args.
func runSpecQueue(t *testing.T, list, tasks string, args ...string) queueRun {
	t.Helper()
	root := copyScript(t, "openspec-queue.sh")
	bin := filepath.Join(root, "node_modules", ".bin")
	change := filepath.Join(root, "openspec", "changes", "fx")
	for _, dir := range []string{bin, change} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "openspec"), []byte(openspecStandIn+list+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(change, "tasks.md"), []byte(tasks), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{filepath.Join("scripts", "openspec-queue.sh")}, args...)...)
	cmd.Dir = root
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	r := queueRun{stdout: stdout.String(), stderr: stderr.String()}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		r.status = exit.ExitCode()
	case err != nil:
		t.Fatalf("run openspec-queue.sh: %v", err)
	}
	return r
}

// lines joins a fixture's lines into a file, each ended by a newline, so a case's line numbers can
// be counted by eye.
func lines(ls ...string) string { return strings.Join(ls, "\n") + "\n" }

// queueLine is one line the queue reports: its label, the tasks.md line it names, and its text.
// In a wanted line, text is a prefix of the reported text.
type queueLine struct {
	label string
	line  int
	text  string
}

func (l queueLine) String() string { return fmt.Sprintf("%s L%d %q", l.label, l.line, l.text) }

var (
	reportedRE = regexp.MustCompile(`^      (\S+) +L(\d+) +(.*)$`)
	okLineRE   = regexp.MustCompile(`(?m)^      ok +no halt/hold/deliberate marker in the open tasks$`)
)

// requireReported fails unless the queue exited 0 and reported exactly want, in order. With
// nothing wanted it also requires the ok line, and with something wanted it requires its absence.
func requireReported(t *testing.T, r queueRun, want ...queueLine) {
	t.Helper()
	if r.status != 0 {
		t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
	}
	var got []queueLine
	for _, s := range strings.Split(r.stdout, "\n") {
		m := reportedRE.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		got = append(got, queueLine{m[1], n, m[3]})
		if len(m[3]) > 104 {
			t.Errorf("reported text is %d bytes, over the queue's 104: %q", len(m[3]), m[3])
		}
	}
	same := len(got) == len(want)
	for i := 0; same && i < len(want); i++ {
		same = got[i].label == want[i].label && got[i].line == want[i].line && strings.HasPrefix(got[i].text, want[i].text)
	}
	if !same {
		t.Fatalf("reported %v\nwant %v (each text a prefix)\nstdout:\n%s", got, want, r.stdout)
	}
	if ok := okLineRE.MatchString(r.stdout); ok != (len(want) == 0) {
		t.Fatalf("ok line printed: %t, want %t\nstdout:\n%s", ok, len(want) == 0, r.stdout)
	}
}

// Task 3.1 as it reads at b91c60d, openspec/changes/authority-one-spelling/tasks.md:55-65, with
// its Hold: on the block's eighth line, then the first line of task 3.2 (:66).
var taskB91c60d = []string{
	"- [ ] 3.1 (D) `TestNoSecondAuthorityNameSensitivity`, written first, in `authority_test.go`: the fixture of",
	"      `design.md` D2 (the seven matching names across a public, an internal and a `main` package; the four",
	"      non-matches: an unexported `federationMeta`, lowercase `federation`, a `_test.go` `TestFederation`, a comment",
	"      and a string literal saying `BuildGlobalID`). Seen to fail first. Then `TestNoSecondAuthorityName` over the",
	"      repository, as a second predicate over the public-signature loader (`signatures_test.go:212-258`): every",
	"      exported object of every loaded module package — package scope, methods of named types, struct fields — whose",
	"      name contains `Federation`, `GlobalID` or `EntityIRI`, with D2's failure line. Gate: `task test:unit`.",
	"      Hold: until PR #48 merges — it carries `signatures_test.go`, and at its pushed head `c64ac338` the check names",
	"      seven identifiers #48 is ruled to remove (design P2). When the hold lifts, the `signatures_test.go` lines this",
	"      file and `design.md` cite (`:212-258` at `c64ac338`) are re-cited at #48's merged head, where the load has",
	"      moved into `loadModuleTypes`.",
	"- [ ] 3.2 (D) Shown able to fail, as 2.3: drop the internal-package scope; drop methods; drop struct fields; match",
}

// The sentence that explains holds, as line 3 of every recent tasks.md.
var tasksPreamble = []string{
	"# Tasks: fx",
	"",
	"Each task names the outcome and the gate that proves it. An unticked task that carries `Hold:` waits for what it",
	"names.",
	"",
}

func TestSpecQueueHolds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tasks string
		want  []queueLine
	}{
		{"H1 hold on a continuation line", lines(taskB91c60d...),
			[]queueLine{{"BLOCKED", 8, "3.1 Hold: until PR #48 merges"}}},
		// Task 3.6 at a7dbf9d, openspec/changes/setup-04a-01-floor/tasks.md: its first line (:217), then
		// its last two nested bullets (:253-259).
		{"H2 hold in a nested bullet", lines(
			"- [ ] 3.6 (D) `message`, `pkg/cache` (level 5), destinations by design D5 (#9 comment 5953295358, refining",
			"      - Every `internal/cache` constructor that returns `Cache` with an error (`NewSimple`, `NewLRU`, `NewTTL`, the",
			"        hybrid constructor, and `NewFromConfig` through them) returns a nil `Cache` on error, not a nil pointer",
			"        inside a non-nil interface (`TestCacheConstructorsReturnNilCacheOnError`). Implementer-reported: it failed",
			"        first for `NewSimple`, `NewLRU` and `NewFromConfig`'s simple and lru paths; the TTL and hybrid ones were",
			"        fixed in c6d7108.",
			"      - Hold: `message` waits for task 2.8. Its tests import `internal/semantictest`",
			"        (`payload_test.go:10`, `triple_helpers_test.go:9` at the pin), and the harness copy is task 2.8's.",
		), []queueLine{{"BLOCKED", 7, "3.6 Hold: `message` waits for task 2.8"}}},
		{"H3 what the hold waits for is on the next line", lines(
			"- [ ] 3.12b (D) Read the whole block. Hold:",
			"      task 3.12a.",
		), []queueLine{{"BLOCKED", 1, "3.12b Hold: task 3.12a."}}},
		// Task 3.1 at b0bb713, openspec/changes/authority-one-spelling/tasks.md:55-64, then task 3.2's
		// first line (:65).
		{"H4 hold on the first line", lines(
			"- [ ] 3.1 Hold: until PR #48 merges — it carries `signatures_test.go`, and at its pushed head `c64ac338` the",
			"      check names seven identifiers #48 is ruled to remove (design P2); those lines are re-cited at #48's merged",
			"      head, where the load moved into `loadModuleTypes`.",
			"      (D) `TestNoSecondAuthorityNameSensitivity`, written first, in `authority_test.go`: the fixture of",
			"      `design.md` D2 (the seven matching names across a public, an internal and a `main` package; the four",
			"      non-matches: an unexported `federationMeta`, lowercase `federation`, a `_test.go` `TestFederation`, a comment",
			"      and a string literal saying `BuildGlobalID`). Seen to fail first. Then `TestNoSecondAuthorityName` over the",
			"      repository, as a second predicate over the public-signature loader (`signatures_test.go:212-258`): every",
			"      exported object of every loaded module package — package scope, methods of named types, struct fields — whose",
			"      name contains `Federation`, `GlobalID` or `EntityIRI`, with D2's failure line. Gate: `task test:unit`.",
			"- [ ] 3.2 (D) Shown able to fail, as 2.3: drop the internal-package scope; drop methods; drop struct fields; match",
		), []queueLine{{"BLOCKED", 1, "3.1 Hold: until PR #48 merges"}}},
		{"H5 the hold belongs to the next task", lines(
			"- [ ] 2.1 (D) Write the reader.",
			"      Gate: `task test:unit`.",
			"- [ ] 2.2 (D) Write the writer.",
			"      Hold: until #12 merges.",
		), []queueLine{{"BLOCKED", 4, "2.2 Hold: until #12 merges."}}},
		{"H6 ticked task", lines(
			"- [x] 1.2 (D) Write the reader.",
			"      Hold: until #12 merges.",
			"- [ ] 1.3 (D) Write the writer.",
		), nil},
		{"H7 hold outside any task", lines(slices.Concat(tasksPreamble, []string{
			"## 1. Work",
			"",
			"- [ ] 1.1 (D) Write the reader.",
			"      Gate: `task test:unit`.",
			"",
			"Hold: task 1.1 waits for #12.",
		})...), nil},
		{"H8 other words on a continuation line", lines(
			"- [ ] 1.1 (D) Write the reader.",
			"      hold: until #12",
			"- [ ] 1.2 (D) Write the writer.",
			"      blocked on #12",
		), nil},
		{"H9 a partly done task with a hold", lines(
			"- [~] 4.2 (D) Port the reader.",
			"      Hold: until #12 merges.",
		), []queueLine{{"WONTDO", 1, "4.2 (D) Port the reader."}, {"BLOCKED", 2, "4.2 Hold: until #12 merges."}}},
		{"H10 a held task with a red first line", lines(
			"- [ ] 4.2 (D) The end-to-end test is failing on main.",
			"      Hold: until #12 merges.",
		), []queueLine{{"RED", 1, "4.2 (D) The end-to-end test is failing on main."}, {"BLOCKED", 2, "4.2 Hold: until #12 merges."}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireReported(t, runSpecQueue(t, listOneChange, tc.tasks), tc.want...)
		})
	}

	t.Run("H11 strict run", func(t *testing.T) {
		if r := runSpecQueue(t, listOneChange, lines(taskB91c60d...), "--strict"); r.status != 1 {
			t.Errorf("--strict: exit %d, want 1\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
		if r := runSpecQueue(t, listOneChange, lines(taskB91c60d...)); r.status != 0 {
			t.Errorf("without --strict: exit %d, want 0\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
		}
	})
}

// spec-queue › "Labels on a task's first line": the nine cases of SemStreams' fixture test at
// 8b99efe9, scripts/openspec-queue_fixture_test.sh:79-113, with its texts word for word.
func TestSpecQueueFirstLineCaveats(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tasks string
		want  []queueLine
	}{
		{"lowercase halt mid-sentence in an open task",
			lines("- [ ] 4.3 If the pre-v1 wipe window closed before 3.1, halt: record the missed window."),
			[]queueLine{{"HALT", 1, "4.3 If the pre-v1 wipe window closed before 3.1, halt: record the missed window."}}},
		{"partial marker regardless of wording",
			lines("- [~] 4.2 Verifying Workflow.Name equals the Schema own Workflow()."),
			[]queueLine{{"WONTDO", 1, "4.2 Verifying Workflow.Name equals the Schema own Workflow()."}}},
		{"explicit RED gate",
			lines("- [ ] 4.2 RED — semantic e2e is failing, see gh#830."),
			[]queueLine{{"RED", 1, "4.2 RED — semantic e2e is failing, see gh#830."}}},
		{"HOLD wording",
			lines("- [ ] 8.3 On HOLD pending the owner ruling."),
			[]queueLine{{"BLOCKED", 1, "8.3 On HOLD pending the owner ruling."}}},
		{"STILL OPEN wording",
			lines("- [ ] 8.3 **STILL OPEN** — decide whether it rides the sister replay."),
			[]queueLine{{"OPEN-Q", 1, "8.3 STILL OPEN — decide whether it rides the sister replay."}}},
		{"deliberate not-done wording",
			lines("- [ ] 4.2 Not enforced, deliberately, because it converts a posture into a boot failure."),
			[]queueLine{{"WONTDO", 1, "4.2 Not enforced, deliberately, because it converts a posture into a boot failure."}}},
		{"completed task mentioning halt is history, not a live condition",
			lines("- [x] 4.3 The wipe window halt condition was evaluated and did not fire."), nil},
		{"ordinary open task with no caveat",
			lines("- [ ] 2.1 Run one comparative benchmark and record it as ADR evidence."), nil},
		{"clean change emits an explicit no-marker line",
			lines("- [ ] 2.1 Run one comparative benchmark and record it as ADR evidence.", "- [x] 2.2 Enumerate the consumers."), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireReported(t, runSpecQueue(t, listOneChange, tc.tasks), tc.want...)
		})
	}
}

// spec-queue › "A hold outside every task fails spec:check": scripts/openspec-queue.sh --check.
func TestSpecQueueCheck(t *testing.T) {
	const path = "openspec/changes/fx/tasks.md"
	strayParagraph := lines(
		"# Tasks: fx",
		"",
		"## 1. Work",
		"",
		"- [ ] 1.1 (D) Write the reader.",
		"      Gate: `task test:unit`.",
		"",
		"Hold: task 1.1 waits for #12.",
	)
	for _, tc := range []struct {
		name   string
		list   string
		tasks  string
		status int
		stdout string
	}{
		{"C1 hold in a paragraph after the task list", listOneChange, strayParagraph,
			1, path + ":8: " + holdOutsideMsg + "\n"},
		{"C2 hold in a heading", listOneChange, lines(
			"# Tasks: fx",
			"",
			"## 1. Work",
			"",
			"- [x] 1.1 (D) Write the reader.",
			"",
			"## 3. Hold: waits for #12",
			"",
			"- [ ] 3.1 (D) Write the writer.",
		), 1, path + ":7: " + holdOutsideMsg + "\n"},
		{"C3 the sentence that explains holds", listOneChange, lines(slices.Concat(tasksPreamble, []string{
			"## 1. Work",
			"",
			"- [ ] 1.1 (D) Write the reader.",
		})...), 0, "holds: ok (1 tasks.md read)\n"},
		{"C4 hold in a ticked task", listOneChange, lines(
			"# Tasks: fx",
			"",
			"## 1. Work",
			"",
			"- [x] 1.2 (D) Write the reader.",
			"      Hold: until #12 merges.",
			"- [ ] 1.3 (D) Write the writer.",
		), 0, "holds: ok (1 tasks.md read)\n"},
		{"C5 hold in an open task", listOneChange,
			lines(slices.Concat([]string{"# Tasks: fx", "", "## 3. C-1: the name check", ""}, taskB91c60d)...),
			0, "holds: ok (1 tasks.md read)\n"},
		// The planted file has a misplaced hold, but the stand-in lists no change: the check reads
		// the changes openspec lists, not every directory under openspec/changes.
		{"C6 no change in flight", listNoChange, strayParagraph,
			0, "holds: ok (0 tasks.md read)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runSpecQueue(t, tc.list, tc.tasks, "--check")
			if r.status != tc.status || r.stdout != tc.stdout {
				t.Fatalf("exit %d, stdout %q\nwant exit %d, stdout %q\nstderr:\n%s", r.status, r.stdout, tc.status, tc.stdout, r.stderr)
			}
		})
	}

	for _, tc := range []struct{ name, list string }{
		{"C7 the change list exits non-zero", listFails},
		{"C7 the change list is not JSON", listNotJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runSpecQueue(t, tc.list, strayParagraph, "--check")
			if r.status != 2 || !strings.HasPrefix(r.stderr, "queue unavailable:") || strings.Contains(r.stdout, "holds: ok") {
				t.Fatalf("exit %d, stdout %q, stderr %q\nwant exit 2, stderr starting %q, no %q on stdout",
					r.status, r.stdout, r.stderr, "queue unavailable:", "holds: ok")
			}
		})
	}
}

// spec-queue › "spec:check runs the check": the task runs openspec validate, then the check, and
// nothing in it can discard their exit status. The command lines are the spec's, written here, not
// read from the file under test.
const (
	specValidateCmd = `npx --no-install openspec validate --all --strict --no-interactive`
	specHoldCmd     = `scripts/openspec-queue.sh --check`
)

func TestSpecCheckWiring(t *testing.T) {
	t.Run("Taskfile.yml", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), "Taskfile.yml"))
		if err != nil {
			t.Fatal(err)
		}
		requireNoViolations(t, "spec:check wiring", specCheckWiringViolations(data))
	})

	taskfile := func(extra string, cmds ...string) []byte {
		s := "version: '3'\ntasks:\n  spec:check:\n    desc: d\n" + extra + "    cmds:\n"
		for _, c := range cmds {
			s += "      - " + c + "\n"
		}
		return []byte(s)
	}
	t.Run("clean fixture", func(t *testing.T) {
		requireNoViolations(t, "clean fixture", specCheckWiringViolations(taskfile("", specValidateCmd, specHoldCmd)))
	})
	for _, tc := range []struct {
		name  string
		file  []byte
		wants []string
	}{
		{"check line removed", taskfile("", specValidateCmd),
			[]string{"spec:check", "command 2", `want "` + specHoldCmd + `"`}},
		{"the two lines swapped", taskfile("", specHoldCmd, specValidateCmd),
			[]string{"spec:check", "command 2", `want "` + specHoldCmd + `"`}},
		{"--check dropped", taskfile("", specValidateCmd, "scripts/openspec-queue.sh"),
			[]string{"spec:check", "command 2", `want "` + specHoldCmd + `"`}},
		{"ignore_error on the task", taskfile("    ignore_error: true\n", specValidateCmd, specHoldCmd),
			[]string{"spec:check", "ignore_error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireViolation(t, specCheckWiringViolations(tc.file), tc.wants...)
		})
	}
}

// specCheckWiringViolations checks the spec:check task of a Taskfile: a mapping with only desc,
// summary and cmds, whose commands are exactly the validation and then the check, each a plain
// string.
func specCheckWiringViolations(taskfile []byte) []string {
	var tf struct {
		Tasks map[string]yaml.Node `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &tf); err != nil {
		return []string{"Taskfile.yml: parse: " + err.Error()}
	}
	node, ok := tf.Tasks["spec:check"]
	if !ok || node.Kind != yaml.MappingNode {
		return []string{"Taskfile.yml: spec:check: no such task"}
	}
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
				cmds = append(cmds, c.Value)
			}
		default:
			violations = append(violations, fmt.Sprintf("Taskfile.yml: spec:check: key %q can skip a command or discard its exit status; only desc, summary and cmds are allowed", key))
		}
	}
	for i, want := range []string{specValidateCmd, specHoldCmd} {
		got := "nothing"
		if i < len(cmds) {
			got = strconv.Quote(cmds[i])
		}
		if got != strconv.Quote(want) {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: spec:check: command %d is %s, want %q", i+1, got, want))
		}
	}
	if len(cmds) > 2 {
		violations = append(violations, fmt.Sprintf("Taskfile.yml: spec:check: runs %d commands, want exactly 2: %q", len(cmds), cmds))
	}
	return violations
}
