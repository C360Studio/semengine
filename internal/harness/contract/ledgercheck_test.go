package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// harness-boundaries › "Carried entries match the pin": `task ledger:check` runs the schema tests
// and then the comparison program, and nothing in the task can discard the program's exit status.
// `task ledger:diff` runs the same program and says it reads GitHub. The command lines are the
// spec's, written here, not read from the file under test.
const (
	ledgerSchemaCmd = `go test -count=1 -run '^TestAdmissionLedger' ./internal/harness/contract/`
	ledgerCheckCmd  = `go run ./internal/harness/pindiff check`
	ledgerDiffCmd   = `go run ./internal/harness/pindiff diff {{.CLI_ARGS}}`
)

func TestLedgerCheckWiring(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	requireNoViolations(t, "ledger task wiring", ledgerWiringViolations(data))
}

func TestLedgerCheckWiringSensitivity(t *testing.T) {
	taskfile := func(check, diff string) []byte {
		return []byte("version: '3'\ntasks:\n" + check + diff)
	}
	checkTask := func(body string) string { return "  ledger:check:\n    desc: d\n" + body }
	cmds := func(lines ...string) string {
		s := "    cmds:\n"
		for _, l := range lines {
			s += "      - " + l + "\n"
		}
		return s
	}
	goodCheck := checkTask(cmds(ledgerSchemaCmd, ledgerCheckCmd))
	goodDiff := "  ledger:diff:\n    desc: Print the difference from the pin; fetches it from GitHub\n" +
		"    cmds:\n      - '" + ledgerDiffCmd + "'\n"
	requireNoViolations(t, "clean fixture", ledgerWiringViolations(taskfile(goodCheck, goodDiff)))

	for _, tc := range []struct {
		name  string
		file  []byte
		wants []string
	}{
		{"program command dropped", taskfile(checkTask(cmds(ledgerSchemaCmd)), goodDiff),
			[]string{"ledger:check", ledgerCheckCmd}},
		{"exit status discarded with || true", taskfile(checkTask(cmds(ledgerSchemaCmd, ledgerCheckCmd+" || true")), goodDiff),
			[]string{"ledger:check", ledgerCheckCmd}},
		{"ignore_error on the task", taskfile(checkTask(cmds(ledgerSchemaCmd, ledgerCheckCmd)+"    ignore_error: true\n"), goodDiff),
			[]string{"ledger:check", "ignore_error"}},
		{"ignore_error on the command", taskfile(checkTask(cmds(ledgerSchemaCmd)+
			"      - cmd: "+ledgerCheckCmd+"\n        ignore_error: true\n"), goodDiff),
			[]string{"ledger:check", ledgerCheckCmd}},
		{"a status check that can skip the task", taskfile(checkTask(cmds(ledgerSchemaCmd, ledgerCheckCmd)+"    status:\n      - 'true'\n"), goodDiff),
			[]string{"ledger:check", "status"}},
		{"program before the schema tests", taskfile(checkTask(cmds(ledgerCheckCmd, ledgerSchemaCmd)), goodDiff),
			[]string{"ledger:check", ledgerSchemaCmd}},
		{"diff task missing", taskfile(goodCheck, ""), []string{"ledger:diff", "no such task"}},
		{"diff description silent about GitHub", taskfile(goodCheck, strings.Replace(goodDiff, "GitHub", "the network", 1)),
			[]string{"ledger:diff", "GitHub"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireViolation(t, ledgerWiringViolations(tc.file), tc.wants...)
		})
	}
}

// ledgerWiringViolations checks the two ledger tasks of a Taskfile. Each must be a mapping with only
// desc, summary and cmds, so no key can skip a command or ignore its failure, and its commands must
// be exactly the spec's, each a plain string.
func ledgerWiringViolations(taskfile []byte) []string {
	var tf struct {
		Tasks map[string]yaml.Node `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &tf); err != nil {
		return []string{"Taskfile.yml: parse: " + err.Error()}
	}
	var violations []string
	for _, want := range []struct {
		name string
		cmds []string
	}{
		{"ledger:check", []string{ledgerSchemaCmd, ledgerCheckCmd}},
		{"ledger:diff", []string{ledgerDiffCmd}},
	} {
		node, ok := tf.Tasks[want.name]
		if !ok || node.Kind != yaml.MappingNode {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: no such task", want.name))
			continue
		}
		var cmds []string
		desc := ""
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			switch key {
			case "desc":
				desc = value.Value
			case "summary":
			case "cmds":
				for _, c := range value.Content {
					if c.Kind != yaml.ScalarNode {
						cmds = append(cmds, "<not a plain command string>")
						continue
					}
					cmds = append(cmds, c.Value)
				}
			default:
				violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: key %q can skip a command or discard its exit status; only desc, summary and cmds are allowed", want.name, key))
			}
		}
		if !slices.Equal(cmds, want.cmds) {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: commands are %q, want exactly %q", want.name, cmds, want.cmds))
		}
		if want.name == "ledger:diff" && !strings.Contains(desc, "GitHub") {
			violations = append(violations, fmt.Sprintf("Taskfile.yml: %s: the description must say it reads GitHub; it is %q", want.name, desc))
		}
	}
	return violations
}
