package contract

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// TestLintTestPortsHonoursNoMarker (harness-boundaries › "Marked fixed port", "Guidance keeps the
// listener"): a fixed port on a line carrying the old inline marker still fails the guard, the
// line is named, and the guidance says to bind port 0 and hand the listener on, naming no file.
// The planted line is assembled at run time so this file does not trip the guard itself.
func TestLintTestPortsHonoursNoMarker(t *testing.T) {
	root := copyScript(t, "lint-test-ports.sh")
	planted := "func f() { net." + "Listen(\"tcp\", \"127.0.0.1:18082\") } // gh#220:allow-fixed-port"
	if err := os.MkdirAll(filepath.Join(root, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "x", "x_test.go"), []byte("package x\n\n"+planted+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runLintTestPorts(root)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("a marked fixed port: err=%v, want exit 1\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "x/x_test.go:3:") {
		t.Errorf("output does not name the marked line x/x_test.go:3:\n%s", text)
	}
	var guidance []string
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "x_test.go:3:") {
			guidance = append(guidance, line)
		}
	}
	g := strings.Join(guidance, "\n")
	for _, want := range []string{"port 0", "listener"} {
		if !strings.Contains(g, want) {
			t.Errorf("guidance does not mention %q:\n%s", want, g)
		}
	}
	if strings.Contains(g, ".go") {
		t.Errorf("guidance names a Go file:\n%s", g)
	}
}
