package contract

import (
	"os/exec"
	"strings"
	"testing"
)

// componentDepsRule is the requirement every violation names.
const componentDepsRule = "(component-registration › The component model reaches no agentic or graph package)"

// TestComponentReachesNoAgenticOrGraphPackage holds the rule that the component model is the
// contract every component builds on, so importing it must not pull in the agentic domain or the
// graph family: a component that needs the lifecycle manager or a tool registry gets it from its
// host, not through component.Dependencies (issues #29 and #102).
//
// Requirement: component-registration/The component model reaches no agentic or graph package
func TestComponentReachesNoAgenticOrGraphPackage(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "component dependency", componentDepViolations(t, root, "./component"))
}

// Requirement: component-registration/The component model reaches no agentic or graph package
func TestComponentReachesNoAgenticOrGraphPackageSensitivity(t *testing.T) {
	const mod = "go.mod"
	base := map[string]string{
		mod:                            "module example.com/fixture\n\ngo 1.26\n",
		"pkg/lifecycle/l.go":           "package lifecycle\n",
		"pkg/lifecyclecleanup/c.go":    "package lifecyclecleanup\n",
		"pkg/projection/p.go":          "package projection\n",
		"pkg/projection/contract/c.go": "package contract\n",
		"graph/g.go":                   "package graph\n",
		"graph/kvcatalog/k.go":         "package kvcatalog\n",
		"graphx/g.go":                  "package graphx\n",
		"internal/graphmutation/m.go":  "package graphmutation\n",
		"agentic/a.go":                 "package agentic\n",
		"vocabulary/agentic/v.go":      "package agentic\n",
		"processor/agentic-tools/t.go": "package agentictools\n",
		"payloadregistry/r.go":         "package payloadregistry\n\nimport _ \"example.com/fixture/pkg/projection/contract\"\n",
		"middle/m.go":                  "package middle\n",
		"component/dependencies.go":    "package component\n\nimport _ \"example.com/fixture/middle\"\n",
		"component/registry.go":        "package component\n\nimport _ \"example.com/fixture/payloadregistry\"\n",
		"component/neighbours.go":      "package component\n\nimport (\n\t_ \"example.com/fixture/graphx\"\n\t_ \"example.com/fixture/pkg/lifecyclecleanup\"\n)\n",
		"component/comment.go":         "// Package component names pkg/lifecycle and agentic in a comment only.\npackage component\n",
		"component/component_test.go":  "package component\n\nimport _ \"example.com/fixture/pkg/lifecycle\"\n",
	}
	tree := func(middle string) map[string]string {
		files := make(map[string]string, len(base))
		for name, content := range base {
			files[name] = content
		}
		if middle != "" {
			files["middle/m.go"] = "package middle\n\nimport _ \"example.com/fixture/" + middle + "\"\n"
		}
		return files
	}

	// The clean fixture reaches pkg/projection/contract through payloadregistry, and neighbours whose
	// names begin like a forbidden path; a test file's import is not a dependency of the package.
	root, _ := writeTree(t, tree(""))
	requireNoViolations(t, "clean fixture", componentDepViolations(t, root, "./component"))

	// Each forbidden path is reached through a package component imports: the transitive edge is
	// the case the spec names.
	for _, forbidden := range []string{
		"pkg/lifecycle", "pkg/projection", "graph", "graph/kvcatalog", "internal/graphmutation",
		"agentic", "vocabulary/agentic", "processor/agentic-tools",
	} {
		t.Run(forbidden, func(t *testing.T) {
			root, _ := writeTree(t, tree(forbidden))
			requireViolation(t, componentDepViolations(t, root, "./component"),
				"component reaches example.com/fixture/"+forbidden+" ", componentDepsRule)
		})
	}

	// An agentic element outside the module, read from planted go list output: the fixture module
	// cannot fetch one.
	requireViolation(t, forbiddenComponentDeps("example.com/fixture",
		[]string{"example.com/fixture/component", "example.org/tools/agentic/run"}),
		"example.org/tools/agentic/run")
	if got := forbiddenComponentDeps("example.com/fixture", nil); len(got) == 0 {
		t.Fatal("an empty go list -deps listing passed; a check that saw nothing is not a pass")
	}
}

// componentDepViolations lists every forbidden package go list -deps reports for pkg in root.
func componentDepViolations(t *testing.T, root, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pkg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return forbiddenComponentDeps(modulePath(t, root), strings.Fields(string(out)))
}

// forbiddenComponentDeps reads go list -deps output and names each forbidden path in it: any path
// with an agentic element (one named agentic, or agentic- and a suffix, as the pin's
// processor/agentic-tools is), and the module's graph, internal/graphmutation, pkg/lifecycle and
// pkg/projection packages or any package under them, except pkg/projection/contract, which
// payloadregistry imports.
func forbiddenComponentDeps(module string, deps []string) []string {
	if len(deps) == 0 {
		return []string{"go list -deps listed no package; a check that saw nothing is not a pass " + componentDepsRule}
	}
	under := func(dep, dir string) bool {
		return dep == module+"/"+dir || strings.HasPrefix(dep, module+"/"+dir+"/")
	}
	var violations []string
	for _, dep := range deps {
		forbidden := under(dep, "graph") || under(dep, "internal/graphmutation") || under(dep, "pkg/lifecycle") ||
			(under(dep, "pkg/projection") && dep != module+"/pkg/projection/contract")
		for _, element := range strings.Split(dep, "/") {
			if element == "agentic" || strings.HasPrefix(element, "agentic-") {
				forbidden = true
			}
		}
		if forbidden {
			violations = append(violations, "component reaches "+dep+" "+componentDepsRule)
		}
	}
	return violations
}
