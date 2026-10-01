package contract

import (
	"fmt"
	"strings"
	"testing"
)

// harness-boundaries › "No sleeps in tests": no *_test.go file and no Go file under
// internal/harness/ contains a sleep call. There is no baseline, allowlist or inline marker. The
// check matches text, so a renamed time import or a wait built from a timer is outside it; the
// literal is assembled at run time so this file does not contain what it forbids.
var sleepLiteral = "time" + ".Sleep"

func TestNoSleepsInTests(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "sleep", sleepViolations(t, root, repoFiles(t, root)))
}

func TestNoSleepsInTestsSensitivity(t *testing.T) {
	clean := map[string]string{
		"x/x_test.go": "package x\n\nvar d = time.Millisecond\n",
		// Production code outside the harness may wait; the rule is for tests and harness code.
		"x/x.go": "package x\n\nfunc f() { " + sleepLiteral + "(1) }\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", sleepViolations(t, root, files))

	for _, tc := range []struct {
		name  string
		tree  map[string]string
		wants []string
	}{
		{"sleep in a test file", map[string]string{
			"x/x_test.go": "package x\n\nfunc TestX(t *testing.T) {\n\t" + sleepLiteral + "(10 * time.Millisecond)\n}\n",
		}, []string{"x/x_test.go:4", sleepLiteral}},
		{"sleep in harness code", map[string]string{
			"x/x_test.go":                "package x\n",
			"internal/harness/h/wait.go": "package h\n\nfunc wait() {\n\t" + sleepLiteral + "(time.Second)\n}\n",
		}, []string{"internal/harness/h/wait.go:4", sleepLiteral}},
		{"no test file", map[string]string{"x/x.go": "package x\n"}, []string{"scanned no test file"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, files := writeTree(t, tc.tree)
			requireViolation(t, sleepViolations(t, root, files), tc.wants...)
		})
	}
}

// sleepViolations scans every *_test.go file and every Go file under internal/harness/.
func sleepViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	inScope := func(name string) bool {
		return strings.HasSuffix(name, ".go") && strings.HasPrefix(name, "internal/harness/")
	}
	return scanTestText(t, root, files, inScope, func(line string) string {
		if strings.Contains(line, sleepLiteral) {
			return "`" + sleepLiteral + "`; wait on a channel, callback or synctest bubble instead"
		}
		return ""
	})
}

// scanTestText applies match to every line of every *_test.go file and of every other file
// alsoScan accepts, and reports each non-empty result as file:line. A scan that read no test file
// is itself a violation: a check that matched nothing because it saw nothing is not a pass.
func scanTestText(t *testing.T, root string, files []string, alsoScan func(string) bool, match func(string) string) []string {
	t.Helper()
	var violations []string
	tests := 0
	for _, name := range files {
		isTest := strings.HasSuffix(name, "_test.go")
		if isTest {
			tests++
		}
		if !isTest && (alsoScan == nil || !alsoScan(name)) {
			continue
		}
		for i, line := range readLines(t, root, name) {
			if why := match(line); why != "" {
				violations = append(violations, fmt.Sprintf("%s:%d: %s", name, i+1, why))
			}
		}
	}
	if tests == 0 {
		violations = append(violations, fmt.Sprintf("scanned no test file among %d file(s)", len(files)))
	}
	return violations
}
