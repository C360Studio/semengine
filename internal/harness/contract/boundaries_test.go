package contract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// semstreamsModule is the sister repository SemEngine forks from at the pin and never imports.
const semstreamsModule = "github.com/c360studio/semstreams"

func underModule(importPath, module string) bool {
	return importPath == module || strings.HasPrefix(importPath, module+"/")
}

// I8: no Go file in the module, test or non-test, imports SemStreams, no package in the build
// graph (integration-tagged files and tests included) is under it, and go.mod does not require it
// (harness-boundaries › "A file imports SemStreams"). SemEngine forks at the pin; it never links
// the sister repository.
func TestNoSemStreamsImport(t *testing.T) {
	root := repoRoot(t)
	files := repoFiles(t, root)
	requireNoViolations(t, "SemStreams import", semstreamsViolations(t, root, files))
	requireNoViolations(t, "SemStreams in the build graph", semstreamsDeps(t, root))
}

// T-B8: no production package aggregates the Register functions of two component packages
// (harness-boundaries › "An aggregator package appears"); the consumer's composition root is the
// only aggregator.
func TestNoAggregatorPackage(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "aggregator", aggregatorViolations(t, root, repoFiles(t, root)))
}

// natsfixture imports no package of this module outside internal/harness/ (harness-boundaries ›
// "The fixture imports a ported package"): natsclient's tests use the fixture, so the reverse edge
// would be a cycle.
//
// Requirement: nats-fixture/Connected value for a package's tests; Scenario: Import bound
func TestFixtureImportsNoPortedPackage(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "fixture import", fixtureImportViolations(t, root, repoFiles(t, root)))
}

// The three rules over a planted tree: each rejects its planted case naming the file or package,
// and none reports an allowed shape.
func TestBoundarySensitivity(t *testing.T) {
	const mod = "github.com/c360studio/semengine"
	root, files := writeTree(t, map[string]string{
		"go.mod": "module " + mod + "\n\nrequire (\n\tgithub.com/nats-io/nats.go v1.54.0\n\tgithub.com/c360studio/semstreams v1.0.0\n)\n",
		// I8: a non-test file, a test file behind the integration tag, and an aliased import.
		"pkg/a/a.go":                   "package a\n\nimport \"github.com/c360studio/semstreams/natsclient\"\n\nvar _ = natsclient.New\n",
		"pkg/a/a_integration_test.go":  "//go:build integration\n\npackage a\n\nimport ss \"github.com/c360studio/semstreams\"\n\nvar _ = ss.X\n",
		"pkg/a/near.go":                "package a\n\nimport _ \"github.com/c360studio/semstreamsish\"\n",
		"internal/harness/h/h_test.go": "package h\n\nimport _ \"github.com/c360studio/semstreams/message\"\n",
		"component/alpha/alpha.go":     "package alpha\n\n// Register adds alpha's factory.\nfunc Register(r any) error { return nil }\n",
		"component/beta/beta.go":       "package beta\n\nfunc Register(r any) error { return nil }\n",
		"component/gamma/gamma.go":     "package gamma\n\nfunc New() {}\n",
		"metricish/metricish.go":       "package metricish\n\nimport \"github.com/prometheus/client_golang/prometheus\"\n\nvar _ = prometheus.Register\n",
		"pkg/agg/one.go":               "package agg\n\nimport \"" + mod + "/component/alpha\"\n\nvar _ = alpha.Register\n",
		"pkg/agg/two.go":               "package agg\n\nimport b \"" + mod + "/component/beta\"\n\nfunc init() { _ = b.Register(nil) }\n",
		"pkg/single/single.go":         "package single\n\nimport (\n\t\"" + mod + "/component/alpha\"\n\t\"" + mod + "/component/gamma\"\n)\n\nvar _, _ = alpha.Register, gamma.New\n",
		"pkg/testagg/testagg_test.go":  "package testagg\n\nimport (\n\t\"" + mod + "/component/alpha\"\n\t\"" + mod + "/component/beta\"\n)\n\nvar _, _ = alpha.Register, beta.Register\n",
		// The pin's payloadbuiltins shape: one Register chaining two packages' RegisterPayloads.
		"message/message.go":                 "package message\n\nfunc RegisterPayloads(r any) error { return nil }\n",
		"storage/objectstore/objectstore.go": "package objectstore\n\nfunc RegisterPayloads(r any) error { return nil }\n",
		"payloadbuiltins/builtins.go": "package payloadbuiltins\n\nimport (\n\t\"" + mod + "/message\"\n\t\"" + mod + "/storage/objectstore\"\n)\n\n" +
			"func Register(r any) error {\n\tif err := message.RegisterPayloads(r); err != nil {\n\t\treturn err\n\t}\n\treturn objectstore.RegisterPayloads(r)\n}\n",
		// A composition root is a main package: it may aggregate.
		"cmd/consumer/main.go": "package main\n\nimport (\n\t\"" + mod + "/component/alpha\"\n\t\"" + mod + "/component/beta\"\n\t\"" + mod + "/message\"\n)\n\n" +
			"func main() { _, _, _ = alpha.Register(nil), beta.Register(nil), message.RegisterPayloads(nil) }\n",
		"internal/harness/natsfixture/f.go":      "package natsfixture\n\nimport _ \"" + mod + "/natsclient\"\n",
		"internal/harness/natsfixture/ok.go":     "package natsfixture\n\nimport _ \"" + mod + "/internal/harness/probe\"\n",
		"internal/harness/natsfixture/x_test.go": "package natsfixture\n\nimport _ \"" + mod + "/message\"\n",
		"internal/harness/other/other.go":        "package other\n\nimport _ \"" + mod + "/natsclient\"\n",
	})

	ss := semstreamsViolations(t, root, files)
	requireViolation(t, ss, "pkg/a/a.go", "github.com/c360studio/semstreams/natsclient")
	requireViolation(t, ss, "pkg/a/a_integration_test.go", `"github.com/c360studio/semstreams"`)
	requireViolation(t, ss, "internal/harness/h/h_test.go", "semstreams/message")
	requireViolation(t, ss, "go.mod", "github.com/c360studio/semstreams")
	requireNotReported(t, ss, "near.go")

	agg := aggregatorViolations(t, root, files)
	requireViolation(t, agg, "pkg/agg", "component/alpha", "component/beta")
	requireViolation(t, agg, "payloadbuiltins", "message", "storage/objectstore")
	for _, allowed := range []string{"pkg/single", "pkg/testagg", "metricish", "cmd/consumer"} {
		requireNotReported(t, agg, allowed)
	}

	fx := fixtureImportViolations(t, root, files)
	requireViolation(t, fx, "internal/harness/natsfixture/f.go", mod+"/natsclient")
	requireViolation(t, fx, "internal/harness/natsfixture/x_test.go", mod+"/message")
	for _, allowed := range []string{"ok.go", "internal/harness/other"} {
		requireNotReported(t, fx, allowed)
	}
}

// The build-graph half over planted go list output: a dependency under SemStreams, plain or as a
// test variant, is reported; a lookalike module is not; an empty listing fails.
func TestDepViolationsSensitivity(t *testing.T) {
	v := depViolations([]string{
		"github.com/nats-io/nats.go",
		"github.com/c360studio/semstreams/natsclient",
		"github.com/c360studio/semstreams [github.com/c360studio/semengine/x.test]",
		"github.com/c360studio/semstreamsish",
	})
	requireViolation(t, v, "semstreams/natsclient")
	requireViolation(t, v, "github.com/c360studio/semstreams [")
	requireNotReported(t, v, "semstreamsish")
	requireViolation(t, depViolations(nil), "listed no package")
}

func requireNotReported(t *testing.T, violations []string, fragment string) {
	t.Helper()
	for _, v := range violations {
		if strings.Contains(v, fragment) {
			t.Errorf("violation reported for an allowed shape %q: %s", fragment, v)
		}
	}
}

// modulePath reads the module path from root's go.mod.
func modulePath(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("go.mod: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "module" {
			return f[1]
		}
	}
	t.Fatal("go.mod declares no module")
	return ""
}

// fileImports parses one file's imports without evaluating build constraints, so a file behind a
// tag is checked too.
func fileImports(fset *token.FileSet, root, name string) ([]*ast.ImportSpec, error) {
	f, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(name)), nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	return f.Imports, nil
}

// semstreamsViolations is I8's text half: every Go file, test or not, harness or not, and go.mod.
func semstreamsViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	fset := token.NewFileSet()
	for _, name := range files {
		switch {
		case name == "go.mod":
			data, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				violations = append(violations, fmt.Sprintf("go.mod: %v", err))
				continue
			}
			for i, line := range strings.Split(string(data), "\n") {
				for _, field := range strings.Fields(line) {
					if underModule(field, semstreamsModule) {
						violations = append(violations, fmt.Sprintf("go.mod:%d: names %s; SemEngine never requires SemStreams", i+1, field))
					}
				}
			}
		case strings.HasSuffix(name, ".go"):
			imports, err := fileImports(fset, root, name)
			if err != nil {
				violations = append(violations, fmt.Sprintf("%s: parse: %v", name, err))
				continue
			}
			for _, imp := range imports {
				if p, _ := strconv.Unquote(imp.Path.Value); underModule(p, semstreamsModule) {
					violations = append(violations, fmt.Sprintf("%s:%d: imports %q; SemEngine forks at the pin and never imports SemStreams",
						name, fset.Position(imp.Pos()).Line, p))
				}
			}
		}
	}
	return violations
}

// semstreamsDeps is I8's build-graph half: every package any build of the module reaches, tests
// and integration-tagged files included, so a dependency cannot bring SemStreams in either.
func semstreamsDeps(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-test", "-tags", "integration", "-f", "{{.ImportPath}}", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	return depViolations(strings.Split(strings.TrimSpace(string(out)), "\n"))
}

// depViolations reads go list -deps output, one package per line; a test variant reads
// "pkg [pkg.test]", and its first field is the package.
func depViolations(lines []string) []string {
	var violations []string
	listed := 0
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		listed++
		if underModule(f[0], semstreamsModule) {
			violations = append(violations, fmt.Sprintf("build graph includes %s", line))
		}
	}
	if listed == 0 {
		violations = append(violations, "go list -deps listed no package; a check that saw nothing is not a pass")
	}
	return violations
}

// productionFilesByDir groups the module's non-test Go files outside internal/harness/ by directory.
func productionFilesByDir(files []string) map[string][]string {
	dirs := map[string][]string{}
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "internal/harness/") {
			continue
		}
		dirs[path.Dir(name)] = append(dirs[path.Dir(name)], name)
	}
	return dirs
}

// registrations are the functions through which a package registers its factories or payloads.
var registrations = []string{"Register", "RegisterPayloads"}

// aggregatorViolations is T-B8: a registering package is a package of this module that declares a
// top-level Register or RegisterPayloads; a production package (non-test files outside
// internal/harness/, other than a main package) that refers to the registration of two or more of
// them is an aggregator, as the pin's payloadbuiltins.Register was. A composition root is a main
// package, and it is the one place that aggregates. Register functions outside the module
// (prometheus.Register) are not counted.
func aggregatorViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	mod := modulePath(t, root)
	byDir := productionFilesByDir(files)
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	parse := func(name string) (*ast.File, error) {
		if f, ok := parsed[name]; ok {
			return f, nil
		}
		f, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(name)), nil, parser.SkipObjectResolution)
		parsed[name] = f
		return f, err
	}
	// registering returns the package name of the module package at importPath and the
	// registration functions it declares at top level; none means it is not a registering package.
	registering := func(importPath string) (string, []string) {
		pkgName, declared := "", []string(nil)
		for _, name := range byDir[strings.TrimPrefix(importPath, mod+"/")] {
			f, err := parse(name)
			if err != nil {
				continue
			}
			pkgName = f.Name.Name
			for _, d := range f.Decls {
				if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && slices.Contains(registrations, fn.Name.Name) {
					declared = append(declared, fn.Name.Name)
				}
			}
		}
		return pkgName, declared
	}
	var violations []string
	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	for _, dir := range dirs {
		var used []string
		root := false
		for _, name := range byDir[dir] {
			f, err := parse(name)
			if err != nil {
				violations = append(violations, fmt.Sprintf("%s: parse: %v", name, err))
				continue
			}
			if f.Name.Name == "main" {
				root = true // a composition root
				break
			}
			type registered struct {
				path     string
				declared []string
			}
			local := map[string]registered{} // name in this file -> registering package
			for _, imp := range f.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if !strings.HasPrefix(p, mod+"/") {
					continue
				}
				pkgName, declared := registering(p)
				if len(declared) == 0 {
					continue
				}
				if imp.Name != nil {
					pkgName = imp.Name.Name
				}
				local[pkgName] = registered{p, declared}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); ok {
					if r, ok := local[id.Name]; ok && slices.Contains(r.declared, sel.Sel.Name) && !slices.Contains(used, r.path) {
						used = append(used, r.path)
					}
				}
				return true
			})
		}
		if !root && len(used) >= 2 {
			slices.Sort(used)
			violations = append(violations, fmt.Sprintf("%s: production package refers to the Register or RegisterPayloads of %d packages %v; "+
				"only a consumer's composition root aggregates registrations", dir, len(used), used))
		}
	}
	return violations
}

// fixtureImportViolations: every Go file under internal/harness/natsfixture/, test files included,
// imports no package of this module outside internal/harness/.
func fixtureImportViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	mod := modulePath(t, root)
	var violations []string
	fset := token.NewFileSet()
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") || !strings.HasPrefix(name, "internal/harness/natsfixture/") {
			continue
		}
		imports, err := fileImports(fset, root, name)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: parse: %v", name, err))
			continue
		}
		for _, imp := range imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, mod+"/") && !strings.HasPrefix(p, mod+"/internal/harness/") {
				violations = append(violations, fmt.Sprintf("%s:%d: natsfixture imports %q; it may import only internal/harness/ packages",
					name, fset.Position(imp.Pos()).Line, p))
			}
		}
	}
	return violations
}
