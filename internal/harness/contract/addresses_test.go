package contract

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// T-B6: SemEngine tests never reach a fixed broker. SemStreams tests dial the dev broker's fixed
// address in 38 files (inventory A2.1(b)); any broker on that port, including SemStreams' own dev
// NATS, would silently receive SemEngine test traffic. The literals are assembled at run time so
// this file does not contain what it forbids.
var fixedAddressLiterals = []string{
	"nats://" + "localhost:",
	"nats://" + "127.0.0.1:",
	":" + "4222\"",
	":" + "8222\"",
}

func TestNoFixedAddressesInTests(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "fixed addresses", fixedAddressViolations(t, root, repoFiles(t, root)))
}

func TestNoFixedAddressesInTestsSensitivity(t *testing.T) {
	clean := map[string]string{
		"x/x_test.go": "package x\n\nvar url = f.URL()\n",
		"x/x.go":      "package x\n\n// production code is not a test: \"" + fixedAddressLiterals[0] + "4222\"\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", fixedAddressViolations(t, root, files))

	for i, literal := range fixedAddressLiterals {
		content := ""
		if i < 2 {
			content = "package x\n\nvar a = \"" + literal + "4222\"\n"
		} else {
			content = "package x\n\nvar a = \"0.0.0.0" + literal + "\n"
		}
		root, files := writeTree(t, map[string]string{"x/x_test.go": content})
		requireViolation(t, fixedAddressViolations(t, root, files), "x/x_test.go:3", literal)
	}
}

// TestLintTestPortsPasses runs the carried SemStreams guard (ledger L9) so a fixed net.Listen port
// in any test fails the contract suite as well as `task lint`.
func TestLintTestPortsPasses(t *testing.T) {
	root := repoRoot(t)
	if out, err := runLintTestPorts(root); err != nil {
		t.Fatalf("scripts/lint-test-ports.sh: %v\n%s", err, out)
	}
}

// fixedAddressViolations is the T-B6 literal check over every *_test.go file.
func fixedAddressViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	for _, name := range files {
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}
		for i, line := range readLines(t, root, name) {
			for _, literal := range fixedAddressLiterals {
				if strings.Contains(line, literal) {
					violations = append(violations, fmt.Sprintf("%s:%d: fixed broker address `%s`; use the fixture's URL", name, i+1, literal))
				}
			}
		}
	}
	return violations
}

func runLintTestPorts(root string) ([]byte, error) {
	cmd := exec.Command("bash", "scripts/lint-test-ports.sh")
	cmd.Dir = root
	return cmd.CombinedOutput()
}
