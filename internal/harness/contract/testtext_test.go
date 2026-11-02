package contract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
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

// harness-boundaries › "No own deadlines in tests": no *_test.go file has a select arm on the
// literal text below whose body fails the test, unless the line above the arm is a comment
// beginning "// Own deadline:" followed by a reason; go test -timeout is the one deadline. The
// arms matched are the time.After literal below and a timer's channel, a dotted name then ".C:"
// (time.NewTimer, including a field such as s.timer.C). A time.After kept in a variable and a context deadline are outside it. The
// body runs to the next case or default at the arm's depth, or the select's closing brace; the
// contents of string and rune literals are ignored. The literal is assembled so this file does not
// contain what it forbids.
var (
	ownDeadlineArm = "case <-time." + "After("
	timerChanArm   = regexp.MustCompile(`case <-[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*\.` + "C:")
	// Any receiver (t, tb, b, a testify suite). Error, Errorf, Fatal and Fatalf need an argument so
	// that err.Error() is not read as failing the test; Fail and FailNow may have none.
	failsTheTest = regexp.MustCompile(`\.(` + "Fatal|Fatalf|Error|Errorf" + `)\([^)]|\.(` + "Fail|FailNow" + `)\(|\b(` +
		"require|assert" + `)\.|\.` + "Require" + `\(\)`)
	ownDeadlineNote = "// Own deadline:"
)

func TestNoOwnDeadlineInTests(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "own deadline", ownDeadlineViolations(t, root, repoFiles(t, root)))
}

func TestNoOwnDeadlineInTestsSensitivity(t *testing.T) {
	test := func(body string) map[string]string {
		return map[string]string{"x/x_test.go": "package x\n\nfunc TestX(t *testing.T) {\n" + body + "}\n"}
	}
	clean := map[string]string{
		// A negative check: the timer arm only ends the select; the failing call is in the other arm.
		"x/x_test.go": "package x\n\nfunc TestX(t *testing.T) {\n\tselect {\n\tcase <-c:\n\t\tt.Fatal(\"unexpected\")\n" +
			"\t" + ownDeadlineArm + "200 * time.Millisecond):\n\t\tt.Log(err.Error())\n\t}\n" +
			"\tselect {\n\t" + ownDeadlineNote + " Stop must return before the fixture's own 30 s cleanup budget\n" +
			"\t" + ownDeadlineArm + "time.Second):\n\t\tt.Fatal(\"Stop did not return\")\n\tcase <-done:\n\t}\n" +
			"\tselect {\n\tcase <-c:\n\t\tt.Fatal(\"unexpected\")\n\t" + "case <-timer." + "C:\n\t}\n" +
			// The failing call belongs to the arm after the timer arm, a case or a default.
			"\tselect {\n\t" + ownDeadlineArm + "d):\n\t\tt.Log(\"{\")\n\tcase <-c:\n\t\tt.Fatal(\"unexpected\")\n\t}\n" +
			"\tselect {\n\t" + "case <-s.timer." + "C:\n\tdefault:\n\t\tt.Fail()\n\t}\n" +
			"\t// A comment naming " + ownDeadlineArm + "x) is not an arm; t.Fatal(\"x\")\n}\n",
		// Production code is outside the rule.
		"x/x.go": "package x\n\nfunc f() {\n\tselect {\n\t" + ownDeadlineArm + "d):\n\t\trequire.Fail(nil, \"x\")\n\t}\n}\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", ownDeadlineViolations(t, root, files))

	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"fatal on its own deadline", "\tselect {\n\tcase <-done:\n\t" + ownDeadlineArm + "10 * time.Second):\n\t\tt.Fatal(\"timed out\")\n\t}\n", "x/x_test.go:6"},
		{"errorf in a block", "\tselect {\n\t" + ownDeadlineArm + "time.Second): {\n\t\tt.Errorf(\"slow\")\n\t}\n\tcase <-done:\n\t}\n", "x/x_test.go:5"},
		{"timer channel arm fails", "\tselect {\n\t" + "case <-timer." + "C:\n\t\tt.Fatal(\"timed out\")\n\t}\n", "x/x_test.go:5"},
		{"bare fail", "\tselect {\n\t" + ownDeadlineArm + "time.Second):\n\t\tt.Fail()\n\t}\n", "x/x_test.go:5"},
		{"brace in a string before the failing call", "\tselect {\n\t" + ownDeadlineArm + "time.Second):\n\t\tt.Logf(\"want }\")\n\t\tt.Fatal(\"slow\")\n\t}\n", "x/x_test.go:5"},
		{"selector timer channel", "\tselect {\n\t" + "case <-s.timer." + "C:\n\t\tt.Fatal(\"timed out\")\n\t}\n", "x/x_test.go:5"},
		{"assert call", "\tselect {\n\t" + ownDeadlineArm + "time.Second):\n\t\tassert.True(t, false)\n\t}\n", "x/x_test.go:5"},
		{"suite fail", "\tselect {\n\t" + ownDeadlineArm + "time.Second):\n\t\ts.Fail(\"slow\")\n\t}\n", "x/x_test.go:5"},
		{"helper fatalf", "\tselect {\n\t" + ownDeadlineArm + "time.Second):\n\t\ttb.Fatalf(\"slow\")\n\t}\n", "x/x_test.go:5"},
		{"suite require", "\tselect {\n\t" + ownDeadlineArm + "time.Second):\n\t\ts.Require().NoError(err)\n\t}\n", "x/x_test.go:5"},
		{"require call", "\tselect {\n\t" + ownDeadlineArm + "time.Second):\n\t\trequire.FailNow(t, \"slow\")\n\t}\n", "x/x_test.go:5"},
		{"fail after a nested select", "\tselect {\n\t" + ownDeadlineArm + "time.Second):\n\t\tselect {\n\t\tcase <-c:\n\t\tdefault:\n\t\t}\n\t\tt.FailNow()\n\t}\n", "x/x_test.go:5"},
		{"other comment above", "\tselect {\n\t// Waits for done.\n\t" + ownDeadlineArm + "time.Second):\n\t\tt.Fatalf(\"slow\")\n\t}\n", "x/x_test.go:6"},
		{"empty reason", "\tselect {\n\t" + ownDeadlineNote + "   \n\t" + ownDeadlineArm + "time.Second):\n\t\tt.Error(\"slow\")\n\t}\n", "x/x_test.go:6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, files := writeTree(t, test(tc.body))
			requireViolation(t, ownDeadlineViolations(t, root, files), tc.want, ownDeadlineNote)
		})
	}
	t.Run("no test file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/x.go": "package x\n"})
		requireViolation(t, ownDeadlineViolations(t, root, files), "scanned no test file")
	})
}

func ownDeadlineViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	tests := 0
	for _, name := range files {
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}
		tests++
		lines := readLines(t, root, name)
		for i, line := range lines {
			code := codeOutsideLiterals(line)
			body, isArm := timerArmBody(code)
			if !isArm || !armFailsTest(body, lines[i+1:]) {
				continue
			}
			prev := ""
			if i > 0 {
				prev = strings.TrimSpace(lines[i-1])
			}
			reason, ok := strings.CutPrefix(prev, ownDeadlineNote)
			if !ok || strings.TrimSpace(reason) == "" {
				violations = append(violations, fmt.Sprintf("%s:%d: a timer arm that fails the test without `%s <reason>` on the line above; "+
					"wait on the ordering event and let go test -timeout bound it", name, i+1, ownDeadlineNote))
			}
		}
	}
	if tests == 0 {
		violations = append(violations, fmt.Sprintf("scanned no test file among %d file(s)", len(files)))
	}
	return violations
}

// codeOutsideLiterals returns line up to its line comment with the contents of its string and rune
// literals removed (the quotes stay), so a brace or "//" inside a literal is not read as code. A raw
// string spanning lines is read one line at a time; none of the arms this check reads uses one.
func codeOutsideLiterals(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			return b.String()
		case c == '"' || c == '\'' || c == '`':
			b.WriteByte(c)
			for i++; i < len(line) && line[i] != c; i++ {
				if line[i] == '\\' && c != '`' {
					i++
				}
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// timerArmBody reports whether code opens a timer arm and returns the code after the arm's colon.
func timerArmBody(code string) (string, bool) {
	if i := strings.Index(code, ownDeadlineArm); i >= 0 {
		_, body, _ := strings.Cut(code[i:], "):")
		return body, true
	}
	if loc := timerChanArm.FindStringIndex(code); loc != nil {
		return code[loc[1]:], true
	}
	return "", false
}

// armFailsTest reports whether the arm whose code after the colon is body, followed by the lines in
// rest, fails the test before the next case or default at its depth or the brace that closes its select.
func armFailsTest(body string, rest []string) bool {
	if failsTheTest.MatchString(body) {
		return true
	}
	depth := strings.Count(body, "{") - strings.Count(body, "}")
	for _, line := range rest {
		code := codeOutsideLiterals(line)
		trimmed := strings.TrimSpace(code)
		if depth == 0 && (strings.HasPrefix(trimmed, "case ") || strings.HasPrefix(trimmed, "default:")) {
			return false
		}
		for _, r := range code {
			switch r {
			case '{':
				depth++
			case '}':
				depth--
			}
			if depth < 0 {
				return false
			}
		}
		if failsTheTest.MatchString(code) {
			return true
		}
	}
	return false
}

// harness-boundaries › "No skipped or hidden tests": no *_test.go file calls Skip, Skipf or
// SkipNow, and none carries a build constraint other than integration. No baseline, allowlist or
// marker. A skip reached through a helper and a platform suffix in a file name are outside these
// text checks. The pattern is built so this file does not match it.
var skipCall = regexp.MustCompile(`\.(` + "Skip|Skipf|SkipNow" + `)\(`)

func TestNoSkippedTests(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "skip", skipViolations(t, root, repoFiles(t, root)))
}

func TestNoSkippedTestsSensitivity(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"x/x_test.go": "package x\n\n// Skip is a word in a comment, not a call.\nfunc TestX(t *testing.T) {}\n",
		"x/x.go":      "package x\n\nfunc f(t T) { t." + "Skip(\"production code is not a test\") }\n",
	})
	requireNoViolations(t, "clean fixture", skipViolations(t, root, files))

	for _, call := range []string{"Skip(\"flaky\")", "Skipf(\"pid %d\", 1)", "SkipNow()"} {
		t.Run(call, func(t *testing.T) {
			root, files := writeTree(t, map[string]string{
				"x/x_test.go": "package x\n\nfunc TestX(t *testing.T) {\n\tt." + call + "\n}\n",
			})
			requireViolation(t, skipViolations(t, root, files), "x/x_test.go:4", call[:strings.Index(call, "(")])
		})
	}
	t.Run("no test file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/x.go": "package x\n"})
		requireViolation(t, skipViolations(t, root, files), "scanned no test file")
	})
}

func skipViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	return scanTestText(t, root, files, nil, func(line string) string {
		if m := skipCall.FindStringSubmatch(line); m != nil {
			return "skip call `" + m[1] + "`; a test that cannot run fails with its reason"
		}
		return ""
	})
}

func TestNoHiddenTests(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "build constraint", buildTagViolations(t, root, repoFiles(t, root)))
}

func TestNoHiddenTestsSensitivity(t *testing.T) {
	// A test moved behind the integration tag, with its skip call removed, passes both checks.
	moved := map[string]string{
		"x/x_integration_test.go": "//go:build integration\n\npackage x\n\nfunc TestNeedsBroker(t *testing.T) {}\n",
	}
	root, files := writeTree(t, moved)
	requireNoViolations(t, "integration-tagged fixture", buildTagViolations(t, root, files))
	requireNoViolations(t, "integration-tagged fixture", skipViolations(t, root, files))

	for _, tc := range []struct{ name, content, want string }{
		{"other tag", "//go:build flaky\n\npackage x\n", "//go:build flaky"},
		{"integration combined", "//go:build integration && linux\n\npackage x\n", "//go:build integration && linux"},
		{"legacy form", "// +build flaky\n\npackage x\n", "// +build flaky"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, files := writeTree(t, map[string]string{"x/x_test.go": tc.content})
			requireViolation(t, buildTagViolations(t, root, files), "x/x_test.go:1", tc.want)
		})
	}
	t.Run("no test file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/x.go": "//go:build flaky\n\npackage x\n"})
		requireViolation(t, buildTagViolations(t, root, files), "scanned no test file")
	})
}

// buildTagViolations reads each test file's header, the lines before its package clause, where Go
// accepts build constraints.
func buildTagViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	tests := 0
	for _, name := range files {
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}
		tests++
		for i, line := range readLines(t, root, name) {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "package ") {
				break
			}
			if (strings.HasPrefix(trimmed, "//go:build") && trimmed != "//go:build integration") || strings.HasPrefix(trimmed, "// +build") {
				violations = append(violations, fmt.Sprintf("%s:%d: build constraint `%s`; only `//go:build integration` may gate a test", name, i+1, trimmed))
			}
		}
	}
	if tests == 0 {
		violations = append(violations, fmt.Sprintf("scanned no test file among %d file(s)", len(files)))
	}
	return violations
}

// harness-boundaries › "No bare select": no Go file in the module, test or non-test, package main
// included, contains a select statement with no cases. A goroutine parked on one has no way out,
// and in a re-executed test binary, which runs with no test-timeout timer, Go's deadlock detector
// kills the process once every goroutine blocks: that is what made prochost flaky (CI runs
// 37005148521, 37006036797, 37013932497). Park on ctx.Done(), a channel, or signal.Notify; a main
// uses signal.NotifyContext. The check parses each file, so it matches code and never a comment or
// a string, and every spelling of an empty select (spaces, newlines, a comment inside) alike. The
// literal is assembled at run time so this file does not contain what it forbids.
var bareSelect = "select" + " {}"

func TestNoBareSelect(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "bare select", bareSelectViolations(t, root, repoFiles(t, root)))
}

func TestNoBareSelectSensitivity(t *testing.T) {
	clean := map[string]string{
		"x/x_test.go": "package x\n\n// A helper never parks on a bare " + bareSelect + "; it waits on a channel.\n" +
			"func park(c chan int) {\n\tselect {\n\tcase <-c:\n\t}\n}\n",
		"x/x.go": "package x\n\n/* " + bareSelect + " in a block comment */\nvar s = \"" + bareSelect + "\"\n\n" +
			"func wait(c chan int) {\n\tselect {\n\tcase <-c:\n\tdefault:\n\t}\n}\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", bareSelectViolations(t, root, files))

	for _, tc := range []struct {
		name, file, body, want string
	}{
		{"test file", "x/x_test.go", "\t" + bareSelect + "\n", "x/x_test.go:4"},
		{"non-test file", "x/x.go", "\t" + bareSelect + "\n", "x/x.go:4"},
		{"package main", "cmd/tool/main.go", "\t" + bareSelect + "\n", "cmd/tool/main.go:4"},
		{"no space", "x/x.go", "\tselect" + "{}\n", "x/x.go:4"},
		{"spaces inside", "x/x.go", "\tselect" + " {  }\n", "x/x.go:4"},
		{"across lines", "x/x.go", "\tselect" + " {\n\t}\n", "x/x.go:4"},
		{"comment inside", "x/x.go", "\tselect" + " { // park forever\n\t}\n", "x/x.go:4"},
		{"in a goroutine", "x/x.go", "\tgo func() { select" + " {} }()\n", "x/x.go:4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg := "x"
			if strings.HasPrefix(tc.file, "cmd/") {
				pkg = "main"
			}
			tree := map[string]string{tc.file: "package " + pkg + "\n\nfunc f() {\n" + tc.body + "}\n"}
			if !strings.HasSuffix(tc.file, "_test.go") {
				tree["x/x_test.go"] = "package x\n"
			}
			root, files := writeTree(t, tree)
			requireViolation(t, bareSelectViolations(t, root, files), tc.want, "select with no cases")
		})
	}
	t.Run("unparsable file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/x_test.go": "package x\n\nfunc f() {\n"})
		requireViolation(t, bareSelectViolations(t, root, files), "x/x_test.go", "parse")
	})
	t.Run("no Go file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/README.md": bareSelect + "\n"})
		requireViolation(t, bareSelectViolations(t, root, files), "scanned no Go file")
	})
}

// bareSelectViolations parses every Go file and reports each select statement with no cases. A
// file that does not parse is a violation: a check that could not read a file has not passed it.
func bareSelectViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	scanned := 0
	fset := token.NewFileSet()
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		scanned++
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(name)), nil, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: parse: %v", name, err))
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectStmt); ok && len(sel.Body.List) == 0 {
				violations = append(violations, fmt.Sprintf("%s:%d: select with no cases parks forever; "+
					"wait on ctx.Done(), a channel or signal.Notify (a main uses signal.NotifyContext)",
					name, fset.Position(sel.Pos()).Line))
			}
			return true
		})
	}
	if scanned == 0 {
		violations = append(violations, fmt.Sprintf("scanned no Go file among %d file(s)", len(files)))
	}
	return violations
}

// harness-boundaries › "Integration tests in a package run one at a time": no Go file carrying
// //go:build integration calls t.Parallel(), so tests sharing a package's NATS server never run at
// the same time. No allowlist or inline exemption. A Parallel() call in an untagged helper, or on
// a receiver not named t, escapes the check and is review only.
var parallelCall = "t." + "Parallel("

func TestNoParallelIntegrationTests(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "parallel integration test", parallelIntegrationViolations(t, root, repoFiles(t, root)))
}

func TestNoParallelIntegrationTestsSensitivity(t *testing.T) {
	clean := map[string]string{
		"x/x_integration_test.go": "//go:build integration\n\npackage x\n\n// Tests sharing a server do not call " + parallelCall + ").\nfunc TestX(t *testing.T) {}\n",
		"x/x_test.go":             "package x\n\nfunc TestUnit(t *testing.T) {\n\t" + parallelCall + ")\n}\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", parallelIntegrationViolations(t, root, files))

	t.Run("parallel integration test", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{
			"x/x_integration_test.go": "//go:build integration\n\npackage x\n\nfunc TestX(t *testing.T) {\n\t" + parallelCall + ")\n}\n",
		})
		requireViolation(t, parallelIntegrationViolations(t, root, files), "x/x_integration_test.go:6", "t.Parallel()")
	})
	t.Run("no integration-tagged file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{
			"x/x_test.go": "package x\n\nfunc TestUnit(t *testing.T) {}\n",
		})
		requireViolation(t, parallelIntegrationViolations(t, root, files), "scanned no integration-tagged Go file")
	})
}

func parallelIntegrationViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	tagged := 0
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		lines := readLines(t, root, name)
		if !hasIntegrationBuildTag(lines) {
			continue
		}
		tagged++
		for i, line := range lines {
			if strings.Contains(codeBeforeLineComment(line), parallelCall) {
				violations = append(violations, fmt.Sprintf("%s:%d: `t.Parallel()` in an integration-tagged file; "+
					"tests sharing a package's NATS server run one at a time", name, i+1))
			}
		}
	}
	if tagged == 0 {
		violations = append(violations, fmt.Sprintf("scanned no integration-tagged Go file among %d file(s)", len(files)))
	}
	return violations
}

func hasIntegrationBuildTag(lines []string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			return false
		}
		if trimmed == "//go:build integration" {
			return true
		}
	}
	return false
}

func codeBeforeLineComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}

// harness-boundaries › "A test with its own server says why": every call to natsfixture.New in a
// *_test.go file outside internal/harness/natsfixture has, on the line above it, a comment
// beginning "// Own server:" followed by a reason.
var ownServerCall = "natsfixture." + "New("

func TestOwnServerReason(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "own server reason", ownServerViolations(t, root, repoFiles(t, root)))
}

func TestOwnServerReasonSensitivity(t *testing.T) {
	clean := map[string]string{
		"x/x_integration_test.go": "//go:build integration\n\npackage x\n\nfunc TestX(t *testing.T) {\n" +
			"\t// Own server: restarts the server mid-test.\n" +
			"\tf := " + ownServerCall + "t)\n" +
			"\t_ = natsfixture.NewFaultKV(nil)\n" +
			"\t// A comment naming " + ownServerCall + "t) is not a call.\n" +
			"\t_ = f\n}\n",
		"internal/harness/natsfixture/fixture_integration_test.go": "//go:build integration\n\npackage natsfixture\n\nfunc TestOwn(t *testing.T) {\n" +
			"\tf := " + ownServerCall + "t)\n\t_ = f\n}\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", ownServerViolations(t, root, files))

	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"no comment above", "\tf := " + ownServerCall + "t)\n", "x/x_integration_test.go:4"},
		{"other comment above", "\t// Starts a broker.\n\tf := " + ownServerCall + "t)\n", "x/x_integration_test.go:5"},
		{"empty reason", "\t// Own server:   \n\tf := " + ownServerCall + "t)\n", "x/x_integration_test.go:5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, files := writeTree(t, map[string]string{
				"x/x_integration_test.go": "package x\n\nfunc TestX(t *testing.T) {\n" + tc.body + "}\n",
			})
			requireViolation(t, ownServerViolations(t, root, files), tc.want, "// Own server:")
		})
	}
	t.Run("no test file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/x.go": "package x\n"})
		requireViolation(t, ownServerViolations(t, root, files), "scanned no test file")
	})
}

func ownServerViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	tests := 0
	for _, name := range files {
		if !strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "internal/harness/natsfixture/") {
			continue
		}
		tests++
		lines := readLines(t, root, name)
		for i, line := range lines {
			if !strings.Contains(codeBeforeLineComment(line), ownServerCall) {
				continue
			}
			prev := ""
			if i > 0 {
				prev = strings.TrimSpace(lines[i-1])
			}
			reason, ok := strings.CutPrefix(prev, "// Own server:")
			if !ok || strings.TrimSpace(reason) == "" {
				violations = append(violations, fmt.Sprintf("%s:%d: `natsfixture.New` without `// Own server: <reason>` on the line above", name, i+1))
			}
		}
	}
	if tests == 0 {
		violations = append(violations, fmt.Sprintf("scanned no test file outside internal/harness/natsfixture among %d file(s)", len(files)))
	}
	return violations
}
