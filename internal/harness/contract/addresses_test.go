package contract

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
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

// harness-boundaries › "A port-0 pick on every address in test code says why": a net.Listen call
// with network "tcp" and an every-address literal ("", or a host of "", 0.0.0.0 or [::] with port
// "" or 0) in a *_test.go file or any Go file under internal/harness/ has, on the line above it, a
// comment beginning "// Every address:" followed by a reason (#390, #391). The call text is
// assembled at run time so this file does not contain what it checks for.
var listenCall = "net." + "Listen("

var tcpListenLiteral = regexp.MustCompile(regexp.QuoteMeta(listenCall+`"tcp",`) + `\s*"([^"]*)"\)`)

func TestEveryAddressPortPickSaysWhy(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "every-address port pick", everyAddressViolations(t, root, repoFiles(t, root)))
}

func TestEveryAddressPortPickSaysWhySensitivity(t *testing.T) {
	call := func(network, address string) string {
		return listenCall + strconv.Quote(network) + ", " + strconv.Quote(address) + ")"
	}
	clean := map[string]string{
		"x/x_test.go": "package x\n\nfunc TestX(t *testing.T) {\n" +
			"\ta, _ := " + call("tcp4", ":0") + "\n" +
			"\tb, _ := " + call("tcp", "127.0.0.1:0") + "\n" +
			"\t// Every address: holds the port so the server's \":port\" bind fails.\n" +
			"\tc, _ := " + call("tcp", ":0") + "\n" +
			"\t// A comment naming " + call("tcp", ":0") + " is not a call.\n" +
			"\t_, _, _ = a, b, c\n}\n",
		"x/x.go": "package x\n\nvar l, _ = " + call("tcp", ":0") + "\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", everyAddressViolations(t, root, files))

	for _, address := range []string{"", ":0", ":", "0.0.0.0:0", "0.0.0.0:", "[::]:0", "[::]:"} {
		t.Run(fmt.Sprintf("address %q", address), func(t *testing.T) {
			root, files := writeTree(t, map[string]string{
				"x/x_test.go": "package x\n\nvar l, _ = " + call("tcp", address) + "\n",
			})
			requireViolation(t, everyAddressViolations(t, root, files), "x/x_test.go:3", "// Every address:", "tcp4", "127.0.0.1:0")
		})
	}
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"other comment above", map[string]string{
			"x/x_test.go": "package x\n\n// Holds a port.\nvar l, _ = " + call("tcp", ":0") + "\n",
		}, "x/x_test.go:4"},
		{"empty reason", map[string]string{
			"x/x_test.go": "package x\n\n// Every address:   \nvar l, _ = " + call("tcp", ":0") + "\n",
		}, "x/x_test.go:4"},
		{"second call on the line", map[string]string{
			"x/x_test.go": "package x\n\nvar a, _, b, _ = " + call("tcp", "127.0.0.1:0") + ", " + call("tcp", ":0") + "\n",
		}, "x/x_test.go:3"},
		{"non-test file under internal/harness", map[string]string{
			"x/x_test.go":             "package x\n",
			"internal/harness/h/h.go": "package h\n\nvar l, _ = " + call("tcp", ":0") + "\n",
		}, "internal/harness/h/h.go:3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, files := writeTree(t, tc.files)
			requireViolation(t, everyAddressViolations(t, root, files), tc.want, "// Every address:")
		})
	}
	t.Run("no test file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/x.go": "package x\n"})
		requireViolation(t, everyAddressViolations(t, root, files), "scanned no test file")
	})
}

func everyAddressViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	tests := 0
	for _, name := range files {
		isTest := strings.HasSuffix(name, "_test.go")
		if !isTest && !(strings.HasPrefix(name, "internal/harness/") && strings.HasSuffix(name, ".go")) {
			continue
		}
		if isTest {
			tests++
		}
		lines := readLines(t, root, name)
		for i, line := range lines {
			addr, found := "", false
			for _, m := range tcpListenLiteral.FindAllStringSubmatch(codeBeforeLineComment(line), -1) {
				if isEveryAddressPick(m[1]) {
					addr, found = m[1], true
					break
				}
			}
			if !found {
				continue
			}
			prev := ""
			if i > 0 {
				prev = strings.TrimSpace(lines[i-1])
			}
			reason, ok := strings.CutPrefix(prev, "// Every address:")
			if !ok || strings.TrimSpace(reason) == "" {
				violations = append(violations, fmt.Sprintf("%s:%d: port-0 pick on every address (%q) without `// Every address: <reason>` on the line above; "+
					"pick in the family that is bound (`tcp4` for a port handed to Docker, `127.0.0.1:0` or the listener itself for a test server)",
					name, i+1, addr))
			}
		}
	}
	if tests == 0 {
		violations = append(violations, fmt.Sprintf("scanned no test file among %d file(s)", len(files)))
	}
	return violations
}

// isEveryAddressPick reports whether a listen address is "" or has an empty, 0.0.0.0 or [::] host
// and an empty or 0 port.
func isEveryAddressPick(address string) bool {
	if address == "" {
		return true
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	return (host == "" || host == "0.0.0.0" || host == "::") && (port == "" || port == "0")
}

func runLintTestPorts(root string) ([]byte, error) {
	cmd := exec.Command("bash", "scripts/lint-test-ports.sh")
	cmd.Dir = root
	return cmd.CombinedOutput()
}
