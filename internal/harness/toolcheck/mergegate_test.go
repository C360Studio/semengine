package toolcheck

import (
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
// shuffled order, over the unit packages or the packages given after --; test:tools runs the tool
// packages as test:unit runs the unit ones; test:tools:changed runs only scripts/tools-changed.sh.
// Neither line is read from the file under test: these constants are the spec.
const (
	requiredUnitCmd         = "go test -race -count=1 -cpu 1 {{.PKGS}}"
	requiredRepeatCmd       = "go test -count=5 -cpu 1 -shuffle=on {{.CLI_ARGS | default .PKGS}}"
	requiredToolsCmd        = "go test -race -count=1 -cpu 1 {{.PKGS}}"
	requiredToolsChangedCmd = "scripts/tools-changed.sh"
)

// The fingerprint cover:check compares is written only after go test passed, because Task runs a
// task's commands in order and stops at the first that fails.
var requiredIntegrationCmds = []string{
	"rm -f coverage/integration.tree && mkdir -p coverage",
	"go test -race -failfast -tags=integration -count=1 -p 2 -shuffle=on -timeout 10m -coverprofile=coverage/integration.coverprofile {{.CLI_ARGS | default .PKGS}}",
	"scripts/tree-state.sh > coverage/integration.tree",
}

// pinnedTestTasks is each test task with the spec that pins it, the list its PKGS reads ("" for
// no vars at all) and its exact commands.
var pinnedTestTasks = []struct {
	task, spec, list string
	cmds             []string
}{
	{"test:unit", "merge-gate", "unit", []string{requiredUnitCmd}},
	{"test:repeat", "merge-gate", "unit", []string{requiredRepeatCmd}},
	{"test:integration", "integration-test-runner", "integration", requiredIntegrationCmds},
	{"test:tools", "merge-gate", "tools", []string{requiredToolsCmd}},
	{"test:tools:changed", "merge-gate", "", []string{requiredToolsChangedCmd}},
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
		return []byte("#!/usr/bin/env bash\nsteps=(spec:check build\n  " + steps + ")\n")
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
		{"count lowered", taskfile(map[string]string{"test:repeat": pkgs("unit") + cmds(strings.Replace(requiredRepeatCmd, "-count=5", "-count=1", 1))}), goodVerify,
			[]string{"test:repeat", requiredRepeatCmd}},
		{"cpu setting dropped", taskfile(map[string]string{"test:unit": pkgs("unit") + cmds(strings.Replace(requiredUnitCmd, " -cpu 1", "", 1))}), goodVerify,
			[]string{"test:unit", requiredUnitCmd}},
		{"race added to repeat", taskfile(map[string]string{"test:repeat": pkgs("unit") + cmds(strings.Replace(requiredRepeatCmd, "go test", "go test -race", 1))}), goodVerify,
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

// merge-gate › "Required needs both jobs", "A run when a pull request is marked ready" and "Pinned
// runner image": .github/workflows/ci.yml held to seven facts. The job merge-check runs under
// exactly three read permissions; no permission anywhere in the workflow is a write; required needs
// verify and merge-check; verify's limit is 15 minutes; required runs under if: always(), and its
// step, which takes its results from needs.*.result, exits 0 only when every needed job succeeded;
// the pull_request trigger lists four activity types; every job runs on the one label ubuntu-24.04;
// verify's checkout fetches full history (fetch-depth: 0), so scripts/tools-changed.sh finds the base.
// The step is shown by running its script as the workflow writes it, with each set of results
// planted. As above, the expected values are constants here, never read from the file under test.
var (
	requiredNeeds         = []string{"verify", "merge-check"}
	mergeCheckPermissions = map[string]string{"contents": "read", "issues": "read", "pull-requests": "read"}
	pullRequestTypes      = []string{"opened", "synchronize", "reopened", "ready_for_review"}
)

const (
	verifyTimeoutMinutes = 15
	runnerLabel          = "ubuntu-24.04"
)

func TestCIWorkflowPinned(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	requireNoViolations(t, "ci.yml", ciWorkflowViolations(t, data))
}

func TestCIWorkflowPinnedSensitivity(t *testing.T) {
	const good = `on:
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
      - run: task verify
  merge-check:
    runs-on: ubuntu-24.04
    timeout-minutes: 5
    permissions:
      contents: read
      issues: read
      pull-requests: read
    steps: [{run: scripts/merge-check.sh}]
  required:
    runs-on: ubuntu-24.04
    needs: [verify, merge-check]
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
	requireNoViolations(t, "clean fixture", ciWorkflowViolations(t, []byte(good)))

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
		// The three planted workflows added to tasks.md 7.4 after implementation: required must run
		// whatever its needed jobs did, and its step must fail on anything but success from all of them.
		{"required without if: always()", "    if: always()\n", "",
			[]string{"job required", "if: always()"}},
		{"step exits 0 for skipped", `if [ "$r" != "success" ]`, `if [ "$r" != "success" ] && [ "$r" != "skipped" ]`,
			[]string{"job required", "exits 0", "skipped"}},
		{"step reads verify alone", "${{ join(needs.*.result, ' ') }}", "${{ needs.verify.result }}",
			[]string{"job required", "exits 0", "merge-check=failure"}},
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
		{"merge-check on another image of the release", "  merge-check:\n    runs-on: ubuntu-24.04\n", "  merge-check:\n    runs-on: ubuntu-24.04-arm\n",
			[]string{"job merge-check", "ubuntu-24.04-arm"}},
		{"verify on a list of labels", "  verify:\n    runs-on: ubuntu-24.04\n", "  verify:\n    runs-on: [self-hosted, ubuntu-24.04]\n",
			[]string{"job verify", "[self-hosted ubuntu-24.04]"}},
		{"merge-check on a runner group", "  merge-check:\n    runs-on: ubuntu-24.04\n", "  merge-check:\n    runs-on: {group: ci, labels: ubuntu-24.04}\n",
			[]string{"job merge-check", "group:ci", "labels:ubuntu-24.04"}},
		{"required on an expression", "  required:\n    runs-on: ubuntu-24.04\n", "  required:\n    runs-on: ${{ vars.RUNNER_IMAGE }}\n",
			[]string{"job required", "runs on ${{ vars.RUNNER_IMAGE }}"}},
		// "How the changed files are found": the scenario "Full history dropped from CI".
		{"fetch-depth dropped", "        with:\n          fetch-depth: 0\n", "",
			[]string{"job verify", "fetch-depth: 0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireViolation(t, ciWorkflowViolations(t, plant(t, tc.old, tc.repl)), tc.wants...)
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
	violations := pullRequestTriggerViolations(wf.On)

	// No write anywhere: the workflow's default and every job's own grant. Every job on the one
	// runner label: a list, a runner group or an expression is refused even when it would resolve
	// to that label.
	violations = append(violations, writeGrants("workflow", wf.Permissions)...)
	names := make([]string, 0, len(wf.Jobs))
	for name := range wf.Jobs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		job := wf.Jobs[name]
		violations = append(violations, writeGrants("job "+name, job.Permissions)...)
		if job.RunsOn == nil {
			violations = append(violations, fmt.Sprintf("ci.yml: job %s has no runs-on; the merge-gate spec (\"Pinned runner image\") requires runs-on: %s", name, runnerLabel))
		} else if label, ok := job.RunsOn.(string); !ok || label != runnerLabel {
			violations = append(violations, fmt.Sprintf("ci.yml: job %s runs on %v; the merge-gate spec (\"Pinned runner image\") requires the one label %s", name, job.RunsOn, runnerLabel))
		}
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

// pullRequestTriggerViolations names each activity type the workflow's pull_request trigger does
// not list. A trigger with no types list starts on GitHub's defaults, which lack ready_for_review.
func pullRequestTriggerViolations(on any) []string {
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
		return []string{fmt.Sprintf("ci.yml: no pull_request trigger; the merge-gate spec requires one listing the activity types %v", pullRequestTypes)}
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
	for _, want := range pullRequestTypes {
		if !slices.Contains(types, want) {
			violations = append(violations, fmt.Sprintf("ci.yml: the pull_request trigger lists activity types %v; %s is missing, and the merge-gate spec requires %v", types, want, pullRequestTypes))
		}
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
