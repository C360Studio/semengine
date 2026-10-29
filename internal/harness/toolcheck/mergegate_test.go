package toolcheck

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// merge-gate › "Varied and repeated unit runs" and "Each test task reads its own package list", and
// integration-test-runner › "Integration test command" and "Only packages with integration tests":
// the test tasks task verify runs, held to the exact lines of the specs. Each test task reads its
// package list into PKGS from scripts/test-pkgs.sh and runs plain go test over it. test:unit runs
// once under the race detector at one CPU; test:repeat runs five times without it at one CPU in
// shuffled order, over the repeat packages (the unit packages but the repository guards) or the
// packages given after --; test:tools runs the tool
// packages as test:unit runs the unit ones; test:tools:changed runs only scripts/tools-changed.sh.
// harness-boundaries › "ateam doctor gates verify": ateam:doctor runs ateam doctor --check with the
// four session variables unset and is verify's first step.
// Neither line is read from the file under test: these constants are the spec.
const (
	requiredUnitCmd         = "go test -race -count=1 -cpu 1 {{.PKGS}}"
	requiredRepeatCmd       = "go test -count=5 -cpu 1 -shuffle=on {{.CLI_ARGS | default .PKGS}}"
	requiredToolsCmd        = "go test -race -count=1 -cpu 1 {{.PKGS}}"
	requiredToolsChangedCmd = "scripts/tools-changed.sh"
	requiredAteamDoctorCmd  = "env -u ATEAM_SESSION_ID -u CLAUDE_SESSION_ID -u CODEX_SESSION_ID -u GEMINI_SESSION_ID ateam doctor --check"
)

// The fingerprint cover:check compares is written only after go test passed, because Task runs a
// task's commands in order and stops at the first that fails.
var requiredIntegrationCmds = []string{
	"rm -f coverage/integration.tree && mkdir -p coverage",
	"go test -race -failfast -tags=integration -count=1 -p 2 -shuffle=on -timeout 10m -coverprofile=coverage/integration.coverprofile {{.CLI_ARGS | default .PKGS}}",
	"scripts/tree-state.sh > coverage/integration.tree",
}

// pinnedTestTasks is each test task, and ateam:doctor, with the spec that pins it, the list its PKGS reads ("" for
// no vars at all) and its exact commands.
var pinnedTestTasks = []struct {
	task, spec, list string
	cmds             []string
}{
	{"test:unit", "merge-gate", "unit", []string{requiredUnitCmd}},
	{"test:repeat", "merge-gate", "repeat", []string{requiredRepeatCmd}},
	{"test:integration", "integration-test-runner", "integration", requiredIntegrationCmds},
	{"test:tools", "merge-gate", "tools", []string{requiredToolsCmd}},
	{"test:tools:changed", "merge-gate", "", []string{requiredToolsChangedCmd}},
	{"ateam:doctor", "harness-boundaries", "", []string{requiredAteamDoctorCmd}},
}

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
	pkgs := func(list string) string {
		return "    vars:\n      PKGS:\n        sh: scripts/test-pkgs.sh " + list + "\n"
	}
	cmds := func(cs ...string) string {
		s := "    cmds:\n"
		for _, c := range cs {
			s += "      - '" + c + "'\n"
		}
		return s
	}
	good := map[string]string{}
	for _, want := range pinnedTestTasks {
		body := cmds(want.cmds...)
		if want.list != "" {
			body = pkgs(want.list) + body
		}
		good[want.task] = body
	}
	// taskfile is the good Taskfile with each named task's body replaced; an empty body drops the task.
	taskfile := func(replace map[string]string) []byte {
		tasks := maps.Clone(good)
		for name, body := range replace {
			if body == "" {
				delete(tasks, name)
			} else {
				tasks[name] = body
			}
		}
		s := "version: '3'\ntasks:\n"
		for _, name := range slices.Sorted(maps.Keys(tasks)) {
			s += "  " + name + ":\n" + tasks[name]
		}
		return []byte(s)
	}
	verify := func(steps string) []byte {
		return []byte("#!/usr/bin/env bash\nsteps=(ateam:doctor spec:check build\n  " + steps + ")\n")
	}
	goodTaskfile, goodVerify := taskfile(nil), verify("test:unit test:tools:changed test:integration cover:check test:repeat")
	requireNoViolations(t, "clean fixture", unitInvocationViolations(goodTaskfile, goodVerify))
	rm, goTest, fingerprint := requiredIntegrationCmds[0], requiredIntegrationCmds[1], requiredIntegrationCmds[2]
	integration := func(cs ...string) map[string]string {
		return map[string]string{"test:integration": pkgs("integration") + cmds(cs...)}
	}

	for _, tc := range []struct {
		name             string
		taskfile, verify []byte
		wants            []string
	}{
		{"count lowered", taskfile(map[string]string{"test:repeat": pkgs("repeat") + cmds(strings.Replace(requiredRepeatCmd, "-count=5", "-count=1", 1))}), goodVerify,
			[]string{"test:repeat", requiredRepeatCmd}},
		{"cpu setting dropped", taskfile(map[string]string{"test:unit": pkgs("unit") + cmds(strings.Replace(requiredUnitCmd, " -cpu 1", "", 1))}), goodVerify,
			[]string{"test:unit", requiredUnitCmd}},
		{"race added to repeat", taskfile(map[string]string{"test:repeat": pkgs("repeat") + cmds(strings.Replace(requiredRepeatCmd, "go test", "go test -race", 1))}), goodVerify,
			[]string{"test:repeat", requiredRepeatCmd}},
		{"repeat task missing", taskfile(map[string]string{"test:repeat": ""}), goodVerify,
			[]string{"test:repeat", "no such task"}},
		{"integration shuffle dropped", taskfile(integration(rm, strings.Replace(goTest, " -shuffle=on", "", 1), fingerprint)),
			goodVerify, []string{"test:integration", "integration-test-runner", goTest}},
		{"integration fingerprint written before go test", taskfile(integration(rm, fingerprint, goTest)),
			goodVerify, []string{"test:integration", fingerprint}},
		{"integration fingerprint not removed first", taskfile(integration(goTest, fingerprint)),
			goodVerify, []string{"test:integration", rm}},
		{"integration through a wrapper", taskfile(integration(rm, "scripts/run.sh "+goTest, fingerprint)),
			goodVerify, []string{"test:integration", goTest}},
		{"integration task missing", taskfile(map[string]string{"test:integration": ""}), goodVerify,
			[]string{"test:integration", "no such task"}},
		{"step removed from verify", goodTaskfile, verify("test:unit test:tools:changed cover:check"),
			[]string{"scripts/verify.sh", "test:repeat", "missing"}},
		{"repeat step not last", goodTaskfile, verify("test:unit test:tools:changed test:repeat cover:check"),
			[]string{"scripts/verify.sh", "test:repeat must be the last step", "cover:check"}},
		// "Each test task reads its own package list" and "Only packages with integration tests".
		{"unit reads the tool list", taskfile(map[string]string{"test:unit": pkgs("tools") + cmds(requiredUnitCmd)}), goodVerify,
			[]string{"test:unit", "scripts/test-pkgs.sh unit"}},
		{"test:repeat reads the unit list", taskfile(map[string]string{"test:repeat": pkgs("unit") + cmds(requiredRepeatCmd)}), goodVerify,
			[]string{"test:repeat", "scripts/test-pkgs.sh repeat"}},
		{"repeat has no PKGS and defaults to every package", taskfile(map[string]string{"test:repeat": cmds(`go test -count=5 -cpu 1 -shuffle=on {{.CLI_ARGS | default "./..."}}`)}), goodVerify,
			[]string{"test:repeat", requiredRepeatCmd}},
		{"integration reads the unit list", taskfile(map[string]string{"test:integration": pkgs("unit") + cmds(requiredIntegrationCmds...)}), goodVerify,
			[]string{"test:integration", "scripts/test-pkgs.sh integration"}},
		{"tools reads the unit list", taskfile(map[string]string{"test:tools": pkgs("unit") + cmds(requiredToolsCmd)}), goodVerify,
			[]string{"test:tools", "scripts/test-pkgs.sh tools"}},
		{"tools:changed runs the tool tests directly", taskfile(map[string]string{"test:tools:changed": cmds("task test:tools")}), goodVerify,
			[]string{"test:tools:changed", requiredToolsChangedCmd}},
		{"tool step removed from verify", goodTaskfile, verify("test:unit test:integration cover:check test:repeat"),
			[]string{"scripts/verify.sh", "test:tools:changed", "missing"}},
		{"tool step before the unit step", goodTaskfile, verify("test:tools:changed test:unit test:integration cover:check test:repeat"),
			[]string{"scripts/verify.sh", "test:tools:changed must follow test:unit"}},
		{"doctor keeps a session variable", taskfile(map[string]string{"ateam:doctor": cmds(strings.Replace(requiredAteamDoctorCmd, "-u CLAUDE_SESSION_ID ", "", 1))}), goodVerify,
			[]string{"ateam:doctor", "harness-boundaries", requiredAteamDoctorCmd}},
		{"doctor task missing", taskfile(map[string]string{"ateam:doctor": ""}), goodVerify,
			[]string{"ateam:doctor", "no such task"}},
		{"doctor step not first", goodTaskfile, []byte("#!/usr/bin/env bash\nsteps=(spec:check ateam:doctor build test:unit test:tools:changed test:repeat)\n"),
			[]string{"scripts/verify.sh", "ateam:doctor must be the first step"}},
		{"tools command drift", taskfile(map[string]string{"test:tools": pkgs("tools") + cmds(strings.Replace(requiredToolsCmd, " -race", "", 1))}), goodVerify,
			[]string{"test:tools", requiredToolsCmd}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireViolation(t, unitInvocationViolations(tc.taskfile, tc.verify), tc.wants...)
		})
	}
}

var verifySteps = regexp.MustCompile(`(?s)\bsteps=\(([^)]*)\)`)

// unitInvocationViolations checks the vars and commands of the test tasks in a Taskfile and the step
// list of a verify.sh against the specs.
func unitInvocationViolations(taskfile, verify []byte) []string {
	var violations []string
	var tf struct {
		Tasks map[string]struct {
			Vars map[string]any `yaml:"vars"`
			Cmds []any          `yaml:"cmds"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &tf); err != nil {
		return []string{fmt.Sprintf("Taskfile.yml: %v", err)}
	}
	for _, want := range pinnedTestTasks {
		required := "`" + strings.Join(want.cmds, "`, then `") + "`"
		task, ok := tf.Tasks[want.task]
		if !ok {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: no such task %s; the %s spec requires it to run %s", want.task, want.spec, required))
			continue
		}
		var wantVars map[string]any
		requiredVars := "no vars"
		if want.list != "" {
			wantVars = map[string]any{"PKGS": map[string]any{"sh": "scripts/test-pkgs.sh " + want.list}}
			requiredVars = "exactly `PKGS: {sh: scripts/test-pkgs.sh " + want.list + "}`"
		}
		if len(task.Vars) != len(wantVars) || (wantVars != nil && !reflect.DeepEqual(task.Vars, wantVars)) {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: task %s sets vars %v; the %s spec requires %s", want.task, task.Vars, want.spec, requiredVars))
		}
		if !slices.EqualFunc(task.Cmds, want.cmds, func(got any, cmd string) bool { return got == cmd }) {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: task %s runs %v; the %s spec requires exactly %s", want.task, task.Cmds, want.spec, required))
		}
	}

	m := verifySteps.FindSubmatch(verify)
	if m == nil {
		return append(violations, "scripts/verify.sh: no steps=(...) list")
	}
	steps := strings.Fields(string(m[1]))
	if len(steps) == 0 || steps[0] != "ateam:doctor" {
		violations = append(violations, fmt.Sprintf("scripts/verify.sh: ateam:doctor must be the first step; the steps are %v", steps))
	}
	for _, step := range []string{"test:unit", "test:tools:changed", "test:repeat"} {
		if !slices.Contains(steps, step) {
			violations = append(violations, fmt.Sprintf("scripts/verify.sh: step %s is missing from %v", step, steps))
		}
	}
	if u, c := slices.Index(steps, "test:unit"), slices.Index(steps, "test:tools:changed"); u >= 0 && c >= 0 && c < u {
		violations = append(violations, fmt.Sprintf("scripts/verify.sh: test:tools:changed must follow test:unit; the steps are %v", steps))
	}
	if i := slices.Index(steps, "test:repeat"); i >= 0 && i != len(steps)-1 {
		violations = append(violations, fmt.Sprintf("scripts/verify.sh: test:repeat must be the last step; %v follow it", steps[i+1:]))
	}
	return violations
}

// merge-gate › "A contract test guards both workflows", "Required needs verify", "The merge-check
// workflow" and "Pinned runner image". .github/workflows/ci.yml is held to six facts: no
// permission anywhere in the workflow is a write; required needs verify; verify's limit is 15
// minutes; required runs under if: always(), and its step, which takes its results from
// needs.*.result, exits 0 only when every needed job succeeded; the pull_request trigger lists
// exactly four activity types; every job runs on the one label ubuntu-24.04; verify's checkout
// fetches full history (fetch-depth: 0), so scripts/tools-changed.sh finds the base. The step is
// shown by running its script as the workflow writes it, with each set of results planted.
// .github/workflows/merge-check.yml is held to the merge-check workflow requirement (see
// mergeCheckWorkflowViolations), and every ateam install line in the two files names one release.
// As above, the expected values are constants here, never read from the files under test.
var (
	requiredNeeds            = []string{"verify"}
	mergeCheckPermissions    = map[string]string{"contents": "read", "issues": "read", "pull-requests": "read"}
	pullRequestTypes         = []string{"opened", "synchronize", "reopened", "ready_for_review"}
	mergeCheckTypes          = []string{"opened", "synchronize", "reopened", "ready_for_review", "edited"}
	mergeCheckRequiredChecks = "Required,Merge check"
)

const (
	verifyTimeoutMinutes     = 15
	mergeCheckTimeoutMinutes = 5
	runnerLabel              = "ubuntu-24.04"
	mergeCheckDraftCondition = "!github.event.pull_request.draft"
	mergeCheckBaseRef        = "${{ github.event.pull_request.base.sha }}"
	mergeCheckGroupKey       = "github.event.pull_request.number"
)

// workflowFiles are the two workflow files under test, by the name violations use.
var workflowFiles = []string{"ci.yml", "merge-check.yml"}

func TestCIWorkflowPinned(t *testing.T) {
	files := map[string][]byte{}
	for _, name := range workflowFiles {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = data
	}
	requireNoViolations(t, "workflows", workflowsViolations(t, files))
}

// workflowsViolations checks both workflow files and the one ateam release their install lines name.
func workflowsViolations(t *testing.T, files map[string][]byte) []string {
	violations := ciWorkflowViolations(t, files["ci.yml"])
	violations = append(violations, mergeCheckWorkflowViolations(files["merge-check.yml"])...)
	_, pinViolations := ateamPins(files)
	return append(violations, pinViolations...)
}

func TestCIWorkflowPinnedSensitivity(t *testing.T) {
	const goodCI = `on:
  push:
    branches: [main]
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review]
permissions:
  contents: read
jobs:
  verify:
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
        with:
          fetch-depth: 0
      - run: go install github.com/c360studio/ateam/cmd/ateam@v0.3.4
      - run: task verify
  required:
    runs-on: ubuntu-24.04
    needs: [verify]
    if: always()
    steps:
      - name: Require every needed job to succeed
        env:
          RESULTS: ${{ join(needs.*.result, ' ') }}
        run: |
          echo "needed job results: ${RESULTS:-<none>}"
          if [ -z "$RESULTS" ]; then echo "no needed jobs reported"; exit 1; fi
          for r in $RESULTS; do
            if [ "$r" != "success" ]; then echo "required check did not succeed: $r"; exit 1; fi
          done
`
	const goodMC = `on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review, edited]
concurrency:
  group: merge-check-${{ github.event.pull_request.number }}
  cancel-in-progress: false
permissions:
  contents: read
jobs:
  merge-check:
    runs-on: ubuntu-24.04
    if: ${{ !github.event.pull_request.draft }}
    timeout-minutes: 5
    permissions:
      contents: read
      issues: read
      pull-requests: read
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
        with:
          ref: ${{ github.event.pull_request.base.sha }}
          persist-credentials: false
      - run: go install github.com/c360studio/ateam/cmd/ateam@v0.3.4
      - name: Check
        env:
          ATEAM_REQUIRED_CHECKS: Required,Merge check
        run: scripts/merge-check.sh "$PR_NUMBER"
`
	good := map[string]string{"ci.yml": goodCI, "merge-check.yml": goodMC}
	files := func(m map[string]string) map[string][]byte {
		out := map[string][]byte{}
		for k, v := range m {
			out[k] = []byte(v)
		}
		return out
	}
	requireNoViolations(t, "clean fixture", workflowsViolations(t, files(good)))

	// plant replaces old with repl in one file of the clean pair.
	plant := func(t *testing.T, file, old, repl string) map[string][]byte {
		t.Helper()
		if !strings.Contains(good[file], old) {
			t.Fatalf("%s fixture lacks %q", file, old)
		}
		planted := maps.Clone(good)
		planted[file] = strings.Replace(good[file], old, repl, 1)
		return files(planted)
	}
	type plantCase struct {
		name, old, repl string
		wants           []string
	}
	for _, tc := range []plantCase{
		// "A contract test guards both workflows", the ci.yml half.
		{"required needs nothing", "needs: [verify]", "needs: []",
			[]string{"job required", "verify", "missing"}},
		{"write permission on a job", "    timeout-minutes: 15\n", "    timeout-minutes: 15\n    permissions:\n      issues: write\n",
			[]string{"ci.yml", "job verify", "issues: write"}},
		{"verify limit changed", "timeout-minutes: 15", "timeout-minutes: 30",
			[]string{"job verify", "30", "15 minutes"}},
		{"edited added to ci.yml", "types: [opened, synchronize, reopened, ready_for_review]", "types: [opened, synchronize, reopened, ready_for_review, edited]",
			[]string{"ci.yml", "pull_request", "edited is extra"}},
		// The same holes by other spellings.
		{"required needs another job", "needs: [verify]", "needs: build",
			[]string{"job required", "verify", "missing"}},
		{"write-all on the workflow", "permissions:\n  contents: read\njobs:", "permissions: write-all\njobs:",
			[]string{"ci.yml", "workflow", "write-all"}},
		{"verify limit removed", "    timeout-minutes: 15\n", "",
			[]string{"job verify", "15 minutes"}},
		// The three planted workflows added to tasks.md 7.4 after implementation: required must run
		// whatever its needed jobs did, and its step must fail on anything but success from all of them.
		{"required without if: always()", "    if: always()\n", "",
			[]string{"job required", "if: always()"}},
		{"step exits 0 for skipped", `if [ "$r" != "success" ]`, `if [ "$r" != "success" ] && [ "$r" != "skipped" ]`,
			[]string{"job required", "exits 0", "skipped"}},
		{"step reads verify alone, by its source", "${{ join(needs.*.result, ' ') }}", "${{ needs.verify.result }}",
			[]string{"job required", "does not take its results from needs.*.result"}},
		// The same holes by other spellings.
		{"required runs on success only", "    if: always()\n", "    if: success()\n",
			[]string{"job required", "if: always()", "success()"}},
		{"step has a condition", "      - name: Require every needed job to succeed\n", "      - name: Require every needed job to succeed\n        if: false\n",
			[]string{"job required", "condition"}},
		{"step allowed to fail", "      - name: Require every needed job to succeed\n", "      - name: Require every needed job to succeed\n        continue-on-error: true\n",
			[]string{"job required", "continue-on-error"}},
		{"step passes an empty list", `if [ -z "$RESULTS" ]; then echo "no needed jobs reported"; exit 1; fi`, "",
			[]string{"job required", "exits 0", "no results"}},
		{"step reads an expression the test cannot evaluate", "${{ join(needs.*.result, ' ') }}", "${{ toJSON(needs) }}",
			[]string{"job required", "toJSON(needs)"}},
		{"required's steps run under another shell", "    if: always()\n", "    if: always()\n    defaults:\n      run:\n        shell: sh\n",
			[]string{"job required", "defaults.run.shell", "sh"}},
		{"step always fails", "        run: |\n", "        run: |\n          exit 1\n",
			[]string{"job required", "every needed job succeeded"}},
		// "A run when a pull request is marked ready": the scenarios "Ready type missing" (three
		// workflows) and "A default type missing".
		{"ready type missing", "types: [opened, synchronize, reopened, ready_for_review]", "types: [opened, synchronize, reopened]",
			[]string{"pull_request", "ready_for_review"}},
		{"no activity types", "  pull_request:\n    types: [opened, synchronize, reopened, ready_for_review]\n", "  pull_request:\n",
			[]string{"pull_request", "ready_for_review"}},
		{"no pull_request trigger", "  pull_request:\n    types: [opened, synchronize, reopened, ready_for_review]\n", "",
			[]string{"pull_request", "ready_for_review"}},
		{"default type missing", "types: [opened, synchronize, reopened, ready_for_review]", "types: [opened, reopened, ready_for_review]",
			[]string{"pull_request", "synchronize"}},
		// "Pinned runner image": each plant defeats one weaker check. The fourth job defeats a check
		// of the three named jobs only; the missing runs-on, one that runs only when runs-on is
		// present; ubuntu-26.04, a refusal of -latest labels only; ubuntu-24.04-arm, a prefix match;
		// the list, a "contains ubuntu-24.04" test of the printed value; the group, a check that
		// handles a list but not a mapping; the expression, a check that skips expressions.
		{"a fourth job on the moving label", "jobs:\n", "jobs:\n  lint: {runs-on: ubuntu-latest, steps: [{run: echo}]}\n",
			[]string{"job lint", "ubuntu-latest", "ubuntu-24.04"}},
		{"required has no runs-on", "  required:\n    runs-on: ubuntu-24.04\n", "  required:\n",
			[]string{"job required", "no runs-on", "ubuntu-24.04"}},
		{"verify on another release", "  verify:\n    runs-on: ubuntu-24.04\n", "  verify:\n    runs-on: ubuntu-26.04\n",
			[]string{"job verify", "ubuntu-26.04", "ubuntu-24.04"}},
		{"verify on a list of labels", "  verify:\n    runs-on: ubuntu-24.04\n", "  verify:\n    runs-on: [self-hosted, ubuntu-24.04]\n",
			[]string{"job verify", "[self-hosted ubuntu-24.04]"}},
		{"required on an expression", "  required:\n    runs-on: ubuntu-24.04\n", "  required:\n    runs-on: ${{ vars.RUNNER_IMAGE }}\n",
			[]string{"job required", "runs on ${{ vars.RUNNER_IMAGE }}"}},
		// "How the changed files are found": the scenario "Full history dropped from CI".
		{"fetch-depth dropped", "        with:\n          fetch-depth: 0\n", "",
			[]string{"job verify", "fetch-depth: 0"}},
		// "Pins differ", from the ci.yml side.
		{"ci.yml on another release", "ateam@v0.3.4", "ateam@v0.3.3",
			[]string{"v0.3.3", "v0.3.4", "one version"}},
		{"ci.yml installs no ateam", "      - run: go install github.com/c360studio/ateam/cmd/ateam@v0.3.4\n", "",
			[]string{"ci.yml", "no ateam install line"}},
	} {
		t.Run("ci.yml/"+tc.name, func(t *testing.T) {
			requireViolation(t, workflowsViolations(t, plant(t, "ci.yml", tc.old, tc.repl)), tc.wants...)
		})
	}

	for _, tc := range []plantCase{
		// "A contract test guards both workflows", the merge-check.yml half: the scenarios "Edited
		// type missing", "Head checked out instead of base" and "Pins differ", then one plant per
		// other fact of "The merge-check workflow".
		{"edited type missing", "ready_for_review, edited]", "ready_for_review]",
			[]string{"merge-check.yml", "pull_request", "edited is missing"}},
		{"head checked out instead of base", "        with:\n          ref: ${{ github.event.pull_request.base.sha }}\n", "        with:\n",
			[]string{"merge-check.yml", "step 1", "base commit"}},
		{"head sha named", "pull_request.base.sha", "pull_request.head.sha",
			[]string{"merge-check.yml", "step 1", "head.sha", "base commit"}},
		{"pins differ", "ateam@v0.3.4", "ateam@v0.3.3",
			[]string{"v0.3.3", "v0.3.4", "one version"}},
		{"merge-check.yml installs no ateam", "      - run: go install github.com/c360studio/ateam/cmd/ateam@v0.3.4\n", "",
			[]string{"merge-check.yml", "no ateam install line"}},
		{"credentials persisted", "          persist-credentials: false\n", "",
			[]string{"merge-check.yml", "persist-credentials", "false"}},
		{"draft condition dropped", "    if: ${{ !github.event.pull_request.draft }}\n", "",
			[]string{"merge-check.yml", "if:", "draft"}},
		{"draft condition inverted", "!github.event.pull_request.draft", "github.event.pull_request.draft",
			[]string{"merge-check.yml", "if:", "draft"}},
		{"runs cancelled", "cancel-in-progress: false", "cancel-in-progress: true",
			[]string{"merge-check.yml", "cancel-in-progress", "never cancelled"}},
		{"one group for every pull request", "group: merge-check-${{ github.event.pull_request.number }}", "group: merge-check",
			[]string{"merge-check.yml", "concurrency group", "per pull request"}},
		{"Merge check not required by the gate", "ATEAM_REQUIRED_CHECKS: Required,Merge check", "ATEAM_REQUIRED_CHECKS: Required",
			[]string{"merge-check.yml", "ATEAM_REQUIRED_CHECKS", "Required,Merge check"}},
		{"gate step removed", `        run: scripts/merge-check.sh "$PR_NUMBER"`, "        run: ateam review gate",
			[]string{"merge-check.yml", "no step running scripts/merge-check.sh"}},
		{"push trigger added", "on:\n  pull_request:\n", "on:\n  push:\n  pull_request:\n",
			[]string{"merge-check.yml", "pull_request alone"}},
		{"limit changed", "timeout-minutes: 5", "timeout-minutes: 10",
			[]string{"merge-check.yml", "job merge-check", "5 minutes"}},
		{"fourth permission", "      pull-requests: read\n", "      pull-requests: read\n      actions: read\n",
			[]string{"merge-check.yml", "job merge-check", "actions: read"}},
		{"write permission", "      issues: read\n", "      issues: write\n",
			[]string{"merge-check.yml", "job merge-check", "issues: write"}},
		{"permissions inherited", "    permissions:\n      contents: read\n      issues: read\n      pull-requests: read\n", "",
			[]string{"merge-check.yml", "no permissions of its own"}},
		{"permission dropped", "      issues: read\n", "",
			[]string{"merge-check.yml", "issues: read", "missing"}},
		{"job removed", "  merge-check:\n", "  merge-chek:\n",
			[]string{"merge-check.yml", "no job merge-check"}},
		{"another image of the release", "    runs-on: ubuntu-24.04\n", "    runs-on: ubuntu-24.04-arm\n",
			[]string{"merge-check.yml", "job merge-check", "ubuntu-24.04-arm"}},
		{"a runner group", "    runs-on: ubuntu-24.04\n", "    runs-on: {group: ci, labels: ubuntu-24.04}\n",
			[]string{"merge-check.yml", "job merge-check", "group:ci", "labels:ubuntu-24.04"}},
	} {
		t.Run("merge-check.yml/"+tc.name, func(t *testing.T) {
			requireViolation(t, workflowsViolations(t, plant(t, "merge-check.yml", tc.old, tc.repl)), tc.wants...)
		})
	}
}

type ciJob struct {
	RunsOn          any            `yaml:"runs-on"`
	Needs           any            `yaml:"needs"`
	If              any            `yaml:"if"`
	ContinueOnError any            `yaml:"continue-on-error"`
	Permissions     any            `yaml:"permissions"`
	TimeoutMinutes  any            `yaml:"timeout-minutes"`
	Env             map[string]any `yaml:"env"`
	Defaults        ciDefaults     `yaml:"defaults"`
	Steps           []ciStep       `yaml:"steps"`
}

type ciDefaults struct {
	Run struct {
		Shell string `yaml:"shell"`
	} `yaml:"run"`
}

type ciStep struct {
	Name            string         `yaml:"name"`
	If              any            `yaml:"if"`
	ContinueOnError any            `yaml:"continue-on-error"`
	Uses            string         `yaml:"uses"`
	With            map[string]any `yaml:"with"`
	Shell           string         `yaml:"shell"`
	Env             map[string]any `yaml:"env"`
	Run             string         `yaml:"run"`
}

// ciWorkflowViolations checks a GitHub Actions workflow against the merge-gate spec.
func ciWorkflowViolations(t *testing.T, data []byte) []string {
	var wf struct {
		On          any              `yaml:"on"`
		Permissions any              `yaml:"permissions"`
		Defaults    ciDefaults       `yaml:"defaults"`
		Jobs        map[string]ciJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return []string{fmt.Sprintf("ci.yml: %v", err)}
	}
	violations := pullRequestTriggerViolations("ci.yml", wf.On, pullRequestTypes)
	violations = append(violations, jobsGrantAndRunnerViolations("ci.yml", wf.Permissions, wf.Jobs)...)

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
		for _, d := range []struct{ where, shell string }{{"workflow", wf.Defaults.Run.Shell}, {"job required", req.Defaults.Run.Shell}} {
			if d.shell != "" && d.shell != "bash" {
				violations = append(violations, fmt.Sprintf("ci.yml: %s sets defaults.run.shell %q; the test runs required's step as GitHub runs bash", d.where, d.shell))
			}
		}
		violations = append(violations, requiredStepViolations(t, req, needs)...)
	}

	if v, ok := wf.Jobs["verify"]; !ok {
		violations = append(violations, "ci.yml: no job verify; the merge-gate spec requires it")
	} else if limit, ok := v.TimeoutMinutes.(int); !ok || limit != verifyTimeoutMinutes {
		violations = append(violations, fmt.Sprintf("ci.yml: job verify has timeout-minutes %v; the merge-gate spec requires %d minutes", v.TimeoutMinutes, verifyTimeoutMinutes))
	}
	if v, ok := wf.Jobs["verify"]; ok {
		i := slices.IndexFunc(v.Steps, func(s ciStep) bool { return strings.HasPrefix(s.Uses, "actions/checkout@") })
		if i < 0 {
			violations = append(violations, "ci.yml: job verify has no actions/checkout step; the merge-gate spec requires one with fetch-depth: 0")
		} else if depth, ok := v.Steps[i].With["fetch-depth"].(int); !ok || depth != 0 {
			violations = append(violations, fmt.Sprintf("ci.yml: job verify checks out with fetch-depth %v; the merge-gate spec (\"How the changed files are found\") requires fetch-depth: 0", v.Steps[i].With["fetch-depth"]))
		}
	}
	return violations
}

// jobsGrantAndRunnerViolations holds a workflow to no write anywhere, the workflow's default and
// every job's own grant, and every job to the one runner label: a list, a runner group or an
// expression is refused even when it would resolve to that label.
func jobsGrantAndRunnerViolations(file string, perms any, jobs map[string]ciJob) []string {
	violations := writeGrants(file, "workflow", perms)
	for _, name := range slices.Sorted(maps.Keys(jobs)) {
		job := jobs[name]
		violations = append(violations, writeGrants(file, "job "+name, job.Permissions)...)
		if job.RunsOn == nil {
			violations = append(violations, fmt.Sprintf("%s: job %s has no runs-on; the merge-gate spec (\"Pinned runner image\") requires runs-on: %s", file, name, runnerLabel))
		} else if label, ok := job.RunsOn.(string); !ok || label != runnerLabel {
			violations = append(violations, fmt.Sprintf("%s: job %s runs on %v; the merge-gate spec (\"Pinned runner image\") requires the one label %s", file, name, job.RunsOn, runnerLabel))
		}
	}
	return violations
}

// mergeCheckWorkflowViolations checks .github/workflows/merge-check.yml against the merge-gate
// spec, "The merge-check workflow": the pull_request trigger alone, with exactly five activity
// types, edited among them; one queued run per pull request (concurrency grouped by its number,
// cancel-in-progress false); no write anywhere and every job on the one runner label; a job
// merge-check skipped on a draft, limited to 5 minutes, under exactly three read permissions,
// checking out the base commit without credentials, and running scripts/merge-check.sh with
// ATEAM_REQUIRED_CHECKS naming Required and Merge check. The pin is checked by ateamPins.
func mergeCheckWorkflowViolations(data []byte) []string {
	const file = "merge-check.yml"
	var wf struct {
		On          any              `yaml:"on"`
		Concurrency any              `yaml:"concurrency"`
		Permissions any              `yaml:"permissions"`
		Jobs        map[string]ciJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return []string{fmt.Sprintf("%s: %v", file, err)}
	}
	violations := pullRequestTriggerViolations(file, wf.On, mergeCheckTypes)
	if on, ok := wf.On.(map[string]any); ok && len(on) != 1 {
		violations = append(violations, fmt.Sprintf("%s: triggers on %v; the merge-gate spec requires pull_request alone", file, slices.Sorted(maps.Keys(on))))
	}

	c, _ := wf.Concurrency.(map[string]any)
	if group, _ := c["group"].(string); !strings.Contains(group, mergeCheckGroupKey) {
		violations = append(violations, fmt.Sprintf("%s: concurrency group %q is not per pull request; the merge-gate spec requires one run per pull request (%s)", file, group, mergeCheckGroupKey))
	}
	if cancel, ok := c["cancel-in-progress"].(bool); !ok || cancel {
		violations = append(violations, fmt.Sprintf("%s: concurrency cancel-in-progress is %v; the merge-gate spec requires false, so a run is queued and never cancelled", file, c["cancel-in-progress"]))
	}
	violations = append(violations, jobsGrantAndRunnerViolations(file, wf.Permissions, wf.Jobs)...)

	job, ok := wf.Jobs["merge-check"]
	if !ok {
		return append(violations, file+": no job merge-check; the merge-gate spec requires it")
	}
	cond := strings.TrimSpace(fmt.Sprint(job.If))
	if inner, found := strings.CutPrefix(cond, "${{"); found {
		cond = strings.TrimSpace(strings.TrimSuffix(inner, "}}"))
	}
	if job.If == nil || cond != mergeCheckDraftCondition {
		violations = append(violations, fmt.Sprintf("%s: job merge-check runs under if: %v; the merge-gate spec requires ${{ %s }}, so a draft is skipped", file, job.If, mergeCheckDraftCondition))
	}
	if limit, ok := job.TimeoutMinutes.(int); !ok || limit != mergeCheckTimeoutMinutes {
		violations = append(violations, fmt.Sprintf("%s: job merge-check has timeout-minutes %v; the merge-gate spec requires %d minutes", file, job.TimeoutMinutes, mergeCheckTimeoutMinutes))
	}
	violations = append(violations, mergeCheckPermissionViolations(file, job.Permissions)...)

	checkouts, gates := 0, 0
	for i, st := range job.Steps {
		where := fmt.Sprintf("%s: job merge-check, step %d (%s)", file, i+1, cmp.Or(st.Name, st.Uses, st.Run))
		if strings.HasPrefix(st.Uses, "actions/checkout@") {
			checkouts++
			if ref := fmt.Sprint(st.With["ref"]); ref != mergeCheckBaseRef {
				violations = append(violations, fmt.Sprintf("%s checks out ref %v; the merge-gate spec requires the base commit, %s", where, st.With["ref"], mergeCheckBaseRef))
			}
			if creds, ok := st.With["persist-credentials"].(bool); !ok || creds {
				violations = append(violations, fmt.Sprintf("%s sets persist-credentials %v; the merge-gate spec requires false", where, st.With["persist-credentials"]))
			}
		}
		if strings.Contains(st.Run, "scripts/merge-check.sh") {
			gates++
			if got := fmt.Sprint(st.Env["ATEAM_REQUIRED_CHECKS"]); got != mergeCheckRequiredChecks {
				violations = append(violations, fmt.Sprintf("%s sets ATEAM_REQUIRED_CHECKS %v; the merge-gate spec requires %q", where, st.Env["ATEAM_REQUIRED_CHECKS"], mergeCheckRequiredChecks))
			}
		}
	}
	if checkouts == 0 {
		violations = append(violations, file+": job merge-check has no actions/checkout step; the merge-gate spec requires one of the base commit")
	}
	if gates == 0 {
		violations = append(violations, file+": job merge-check has no step running scripts/merge-check.sh; the merge-gate spec requires one")
	}
	return violations
}

// pullRequestTriggerViolations names each activity type the workflow's pull_request trigger does
// not list, and each it lists beyond want. A trigger with no types list starts on GitHub's
// defaults, which lack ready_for_review.
func pullRequestTriggerViolations(file string, on any, want []string) []string {
	var trigger any
	found := false
	switch o := on.(type) {
	case string:
		found = o == "pull_request"
	case []any:
		found = slices.Contains(o, any("pull_request"))
	case map[string]any:
		trigger, found = o["pull_request"]
	}
	if !found {
		return []string{fmt.Sprintf("%s: no pull_request trigger; the merge-gate spec requires one listing the activity types %v", file, want)}
	}
	var types []string
	if m, ok := trigger.(map[string]any); ok {
		switch ty := m["types"].(type) {
		case string:
			types = []string{ty}
		case []any:
			for _, v := range ty {
				types = append(types, fmt.Sprint(v))
			}
		}
	}
	var violations []string
	for _, w := range want {
		if !slices.Contains(types, w) {
			violations = append(violations, fmt.Sprintf("%s: the pull_request trigger lists activity types %v; %s is missing, and the merge-gate spec requires %v", file, types, w, want))
		}
	}
	for _, ty := range types {
		if !slices.Contains(want, ty) {
			violations = append(violations, fmt.Sprintf("%s: the pull_request trigger lists activity types %v; %s is extra, and the merge-gate spec requires exactly %v", file, types, ty, want))
		}
	}
	return violations
}

// writeGrants names every write in a permissions value: a write-all string, or a scope set to write.
func writeGrants(file, where string, perms any) []string {
	switch p := perms.(type) {
	case string:
		if p != "read-all" {
			return []string{fmt.Sprintf("%s: %s is granted permissions: %s; the merge-gate spec allows no write", file, where, p)}
		}
	case map[string]any:
		var out []string
		for _, scope := range slices.Sorted(maps.Keys(p)) {
			if level := fmt.Sprint(p[scope]); level == "write" {
				out = append(out, fmt.Sprintf("%s: %s is granted %s: %s; the merge-gate spec allows no write", file, where, scope, level))
			}
		}
		return out
	}
	return nil
}

// mergeCheckPermissionViolations requires the merge-check job's own grant to be exactly the three
// reads. Inheriting the workflow's default does not count: the job's token is pinned where the job is.
func mergeCheckPermissionViolations(file string, perms any) []string {
	p, ok := perms.(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("%s: job merge-check has no permissions of its own (%v); the merge-gate spec requires exactly %v", file, perms, mergeCheckPermissions)}
	}
	var out []string
	for _, scope := range slices.Sorted(maps.Keys(p)) {
		level := fmt.Sprint(p[scope])
		if mergeCheckPermissions[scope] != level {
			out = append(out, fmt.Sprintf("%s: job merge-check is granted %s: %s; the merge-gate spec allows only %v", file, scope, level, mergeCheckPermissions))
		}
	}
	for _, scope := range slices.Sorted(maps.Keys(mergeCheckPermissions)) {
		if _, ok := p[scope]; !ok {
			out = append(out, fmt.Sprintf("%s: job merge-check is missing %s: %s; the merge-gate spec requires exactly %v", file, scope, mergeCheckPermissions[scope], mergeCheckPermissions))
		}
	}
	return out
}

// requiredResultSets are the results planted for the step of required, one set per run, keyed by
// needed job. Only the first, every job a success, may exit 0; nil is a run with no results at all.
func requiredResultSets() []map[string]string {
	sets := []map[string]string{{}}
	for _, j := range requiredNeeds {
		sets[0][j] = "success"
	}
	for _, bad := range []string{"failure", "cancelled", "skipped"} {
		for _, j := range requiredNeeds {
			set := maps.Clone(sets[0])
			set[j] = bad
			sets = append(sets, set)
		}
	}
	return append(sets, nil)
}

var ghExpression = regexp.MustCompile(`\$\{\{\s*(.*?)\s*\}\}`)

var (
	joinNeedsResults = regexp.MustCompile(`^join\(\s*needs\.\*\.result\s*(?:,\s*'([^']*)'\s*)?\)$`)
	oneNeedResult    = regexp.MustCompile(`^needs\.([A-Za-z0-9_-]+)\.result$`)
)

// evalNeedsExpression evaluates the two expressions a step may use to read its needed jobs'
// results: join(needs.*.result, 'sep') over the jobs in needs order, and needs.<job>.result. Any
// other expression is refused, so a step the test cannot run as written fails rather than passes.
func evalNeedsExpression(expr string, needs []string, results map[string]string) (string, bool) {
	if m := joinNeedsResults.FindStringSubmatch(expr); m != nil {
		sep := ","
		if strings.Contains(expr, "'") {
			sep = m[1]
		}
		var rs []string
		for _, j := range needs {
			if r, ok := results[j]; ok {
				rs = append(rs, r)
			}
		}
		return strings.Join(rs, sep), true
	}
	if m := oneNeedResult.FindStringSubmatch(expr); m != nil {
		return results[m[1]], true
	}
	return "", false
}

// requiredStepViolations holds the job required to run whatever its needed jobs did and to fail by
// its own step unless each of them succeeded. The condition is read; the step is run.
func requiredStepViolations(t *testing.T, req ciJob, needs []string) []string {
	var out []string
	if cond := strings.TrimSpace(fmt.Sprint(req.If)); req.If == nil || (cond != "always()" && cond != "${{ always() }}") {
		out = append(out, fmt.Sprintf("ci.yml: job required runs under if: %v; the merge-gate spec requires if: always(), or a failed or cancelled needed job leaves Required skipped, which GitHub reports as success", req.If))
	}
	if req.ContinueOnError != nil {
		out = append(out, fmt.Sprintf("ci.yml: job required sets continue-on-error: %v; the merge-gate spec requires it to fail", req.ContinueOnError))
	}
	if len(req.Steps) == 0 {
		return append(out, "ci.yml: job required has no steps; the merge-gate spec requires a step that reads the results of its needed jobs")
	}

	// Every expression the job's steps carry, in their env and their scripts, must be one the test
	// can evaluate, and at least one must read needs.*.result.
	var texts []string
	for _, v := range req.Env {
		texts = append(texts, fmt.Sprint(v))
	}
	runnable := true
	for i, st := range req.Steps {
		where := fmt.Sprintf("ci.yml: job required, step %d (%s)", i+1, st.Name)
		switch {
		case st.If != nil:
			out = append(out, fmt.Sprintf("%s has the condition %v; the merge-gate spec requires the step to run on every result", where, st.If))
		case st.ContinueOnError != nil:
			out = append(out, fmt.Sprintf("%s sets continue-on-error: %v; the merge-gate spec requires the step's failure to fail Required", where, st.ContinueOnError))
		case st.Uses != "" || st.Run == "":
			out = append(out, fmt.Sprintf("%s is not a run step (uses %q); the test runs the step's script as the workflow writes it", where, st.Uses))
			runnable = false
		case st.Shell != "" && st.Shell != "bash":
			out = append(out, fmt.Sprintf("%s runs under shell %q; the test runs it as GitHub runs bash", where, st.Shell))
			runnable = false
		}
		texts = append(texts, st.Run)
		for _, v := range st.Env {
			texts = append(texts, fmt.Sprint(v))
		}
	}
	readsAll := false
	for _, text := range texts {
		for _, m := range ghExpression.FindAllStringSubmatch(text, -1) {
			if joinNeedsResults.MatchString(m[1]) {
				readsAll = true
			}
			if _, ok := evalNeedsExpression(m[1], needs, nil); !ok {
				out = append(out, fmt.Sprintf("ci.yml: job required carries the expression ${{ %s }}, which the test cannot evaluate; the merge-gate spec requires its step to read join(needs.*.result, ...)", m[1]))
				runnable = false
			}
		}
	}
	if !readsAll {
		out = append(out, "ci.yml: job required: its step does not take its results from needs.*.result; the merge-gate spec requires it to read the result of every job it needs")
	}
	if !runnable {
		return out
	}

	for _, results := range requiredResultSets() {
		label := "no results"
		if results != nil {
			var parts []string
			for _, j := range requiredNeeds {
				parts = append(parts, j+"="+results[j])
			}
			label = strings.Join(parts, " ")
		}
		subst := func(text string) string {
			return ghExpression.ReplaceAllStringFunc(text, func(e string) string {
				v, _ := evalNeedsExpression(ghExpression.FindStringSubmatch(e)[1], needs, results)
				return v
			})
		}
		ok, output, err := runRequiredSteps(t, req, subst)
		if err != nil {
			return append(out, fmt.Sprintf("ci.yml: job required: running its step for %s: %v", label, err))
		}
		allSuccess := results != nil && !slices.ContainsFunc(requiredNeeds, func(j string) bool { return results[j] != "success" })
		switch {
		case allSuccess && !ok:
			out = append(out, fmt.Sprintf("ci.yml: job required: its step exits non-zero when every needed job succeeded (%s): %s", label, output))
		case !allSuccess && ok:
			out = append(out, fmt.Sprintf("ci.yml: job required: its step exits 0 for %s; the merge-gate spec requires it to fail unless every needed job succeeded", label))
		}
	}
	return out
}

// runRequiredSteps runs the job's steps in order as GitHub runs a bash step (bash -e), with the
// expressions in their env and scripts already substituted, and an environment of PATH and the
// job's and step's env alone, in a fresh directory so a step cannot touch the source tree. It
// reports whether every step exited 0, as GitHub would.
func runRequiredSteps(t *testing.T, req ciJob, subst func(string) string) (bool, string, error) {
	for _, st := range req.Steps {
		env := []string{"PATH=" + os.Getenv("PATH")}
		for _, m := range []map[string]any{req.Env, st.Env} {
			for _, k := range slices.Sorted(maps.Keys(m)) {
				env = append(env, k+"="+subst(fmt.Sprint(m[k])))
			}
		}
		cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-c", subst(st.Run))
		cmd.Env = env
		cmd.Dir = t.TempDir()
		output, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		switch {
		case errors.As(err, &exit):
			return false, strings.TrimSpace(string(output)), nil
		case err != nil:
			return false, "", err
		}
	}
	return true, "", nil
}
