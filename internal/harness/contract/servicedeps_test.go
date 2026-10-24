package contract

import (
	"os/exec"
	"strings"
	"testing"
)

// serviceDepsRule is the requirement every violation names.
const serviceDepsRule = "(service-lifecycle › The service package reaches no agentic package)"

// TestServiceReachesNoAgenticPackage holds the rule that the service package does not bring in the
// agentic domain: SemStreams' service/milestone_service.go, the pin's only service-to-agentic
// edge, is not ported (issue #30).
//
// Requirement: service-lifecycle/The service package reaches no agentic package
func TestServiceReachesNoAgenticPackage(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "service dependency", serviceDepViolations(t, root, "./service"))
}

// Requirement: service-lifecycle/The service package reaches no agentic package
func TestServiceReachesNoAgenticPackageSensitivity(t *testing.T) {
	base := map[string]string{
		"go.mod":                       "module example.com/fixture\n\ngo 1.26\n",
		"agentic/a.go":                 "package agentic\n",
		"vocabulary/agentic/v.go":      "package agentic\n",
		"processor/agentic-tools/t.go": "package agentictools\n",
		"agenticx/x.go":                "package agenticx\n",
		"notagentic/n.go":              "package notagentic\n",
		"middle/m.go":                  "package middle\n",
		"service/service.go":           "package service\n\nimport _ \"example.com/fixture/middle\"\n",
		"service/neighbours.go":        "package service\n\nimport (\n\t_ \"example.com/fixture/agenticx\"\n\t_ \"example.com/fixture/notagentic\"\n)\n",
		"service/comment.go":           "// Package service names agentic in a comment only.\npackage service\n",
		"service/service_test.go":      "package service\n\nimport _ \"example.com/fixture/agentic\"\n",
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

	// The clean fixture reaches neighbours whose names contain agentic without an agentic element;
	// a test file's import is not a dependency of the package.
	root, _ := writeTree(t, tree(""))
	requireNoViolations(t, "clean fixture", serviceDepViolations(t, root, "./service"))

	// Each forbidden path is reached through a package service imports: the transitive edge is the
	// case the spec names.
	for _, forbidden := range []string{"agentic", "vocabulary/agentic", "processor/agentic-tools"} {
		t.Run(forbidden, func(t *testing.T) {
			root, _ := writeTree(t, tree(forbidden))
			requireViolation(t, serviceDepViolations(t, root, "./service"),
				"service reaches example.com/fixture/"+forbidden+" ", serviceDepsRule)
		})
	}

	// An agentic element outside the module, read from planted go list output: the fixture module
	// cannot fetch one.
	requireViolation(t, agenticServiceDeps([]string{"example.com/fixture/service", "example.org/tools/agentic/run"}),
		"example.org/tools/agentic/run", serviceDepsRule)
	requireViolation(t, agenticServiceDeps(nil), "listed no package", serviceDepsRule)
}

// serviceDepViolations lists every agentic package go list -deps reports for pkg in root.
func serviceDepViolations(t *testing.T, root, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pkg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return agenticServiceDeps(strings.Fields(string(out)))
}

// agenticServiceDeps reads go list -deps output and names each path with an agentic element: one
// named agentic, or agentic- and a suffix, as the pin's processor/agentic-tools is.
func agenticServiceDeps(deps []string) []string {
	if len(deps) == 0 {
		return []string{"go list -deps listed no package; a check that saw nothing is not a pass " + serviceDepsRule}
	}
	var violations []string
	for _, dep := range deps {
		for _, element := range strings.Split(dep, "/") {
			if element == "agentic" || strings.HasPrefix(element, "agentic-") {
				violations = append(violations, "service reaches "+dep+" "+serviceDepsRule)
				break
			}
		}
	}
	return violations
}
