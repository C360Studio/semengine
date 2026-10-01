package contract

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// T-B1: production code stays free of test machinery. SemStreams' natsclient and component
// packages import testing and testcontainers from production files (natsclient/test_client.go:10,16;
// component/lifecycle_test_suite.go:11,14-15 at 5457b345); this keeps SemEngine from repeating it.
func TestImportGraph(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "import graph", importViolations(t, root, repoFiles(t, root)))
}

func TestImportGraphSensitivity(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"go.mod":                             "module github.com/c360studio/semengine\n",
		"pkg/engine/engine.go":               "package engine\n\nimport _ \"github.com/c360studio/semengine/internal/harness/natsfixture\"\n",
		"pkg/engine/tc.go":                   "package engine\n\nimport tc \"github.com/testcontainers/testcontainers-go/wait\"\n\nvar _ = tc.ForLog\n",
		"pkg/engine/t.go":                    "package engine\n\nimport \"testing\"\n\nvar _ testing.TB\n",
		"pkg/engine/y.go":                    "//go:build tools\n\npackage engine\n\nimport \"gopkg.in/yaml.v3\"\n\nvar _ = yaml.Marshal\n",
		"pkg/engine/engine_test.go":          "package engine\n\nimport \"testing\"\n",
		"internal/harness/probe/probe.go":    "package probe\n\nimport \"testing\"\n\nvar _ testing.TB\n",
		"pkg/clean/clean.go":                 "package clean\n\nimport \"context\"\n\nvar _ context.Context\n",
		"internal/harnessish/not_harness.go": "package harnessish\n\nimport \"testing\"\n\nvar _ testing.TB\n",
	})
	v := importViolations(t, root, files)
	requireViolation(t, v, "pkg/engine/engine.go", "internal/harness/natsfixture")
	requireViolation(t, v, "pkg/engine/tc.go", "testcontainers-go/wait")
	requireViolation(t, v, "pkg/engine/t.go", `"testing"`)
	requireViolation(t, v, "pkg/engine/y.go", "gopkg.in/yaml.v3")
	requireViolation(t, v, "internal/harnessish/not_harness.go", `"testing"`)
	for _, line := range v {
		for _, allowed := range []string{"engine_test.go", "internal/harness/probe", "pkg/clean"} {
			if strings.Contains(line, allowed) {
				t.Errorf("violation reported for an allowed file: %s", line)
			}
		}
	}
}

// forbiddenInProduction names what a non-test file outside internal/harness may not import: the
// harness itself and the test libraries it is built from.
func forbiddenInProduction(path string) bool {
	for _, prefix := range []string{
		"github.com/c360studio/semengine/internal/harness",
		"github.com/testcontainers/testcontainers-go",
		"testing",
		"gopkg.in/yaml.v3",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// importViolations is the T-B1 check. It parses imports without evaluating build constraints, so a
// file behind a tag (integration, tools, an OS) is checked too.
func importViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	fset := token.NewFileSet()
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			strings.HasPrefix(name, "internal/harness/") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(name)), nil, parser.ImportsOnly)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: parse: %v", name, err))
			continue
		}
		for _, imp := range file.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if forbiddenInProduction(path) {
				violations = append(violations, fmt.Sprintf("%s:%d: production file imports %q",
					name, fset.Position(imp.Pos()).Line, path))
			}
		}
	}
	return violations
}
