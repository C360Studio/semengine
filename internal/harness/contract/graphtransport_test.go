package contract

import (
	"os/exec"
	"strings"
	"testing"
)

// graphTransportRule is the requirement every violation names.
const graphTransportRule = "(graph-transport-boundary › The graph root imports no transport)"

// TestGraphImportsNoTransport holds the rule that the graph root carries the data model and the
// wire types only: a reader of a graph type must not pull in a NATS client to get it.
//
// Requirement: graph-transport-boundary/The graph root imports no transport
func TestGraphImportsNoTransport(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "graph root transport imports", graphTransportViolations(t, root, "./graph"))
}

// Requirement: graph-transport-boundary/The graph root imports no transport
func TestGraphImportsNoTransportSensitivity(t *testing.T) {
	const mod = "go.mod"
	clean := map[string]string{
		mod:               "module example.com/fixture\n\ngo 1.26\n",
		"graph/g.go":      "package graph\n\nimport _ \"example.com/fixture/model\"\n",
		"model/m.go":      "// Package model mentions natsclient and github.com/nats-io in a comment only.\npackage model\n",
		"natsclient/n.go": "package natsclient\n",
	}
	root, _ := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", graphTransportViolations(t, root, "./graph"))

	// The root reaches natsclient through a package it imports: the transitive edge is the case.
	reaching := map[string]string{
		mod:               "module example.com/fixture\n\ngo 1.26\n",
		"graph/g.go":      "package graph\n\nimport _ \"example.com/fixture/model\"\n",
		"model/m.go":      "package model\n\nimport _ \"example.com/fixture/natsclient\"\n",
		"natsclient/n.go": "package natsclient\n",
	}
	root, _ = writeTree(t, reaching)
	requireViolation(t, graphTransportViolations(t, root, "./graph"), "example.com/fixture/natsclient")

	// A github.com/nats-io path in the listing, read from planted go list output: the fixture
	// module cannot fetch the real one.
	requireViolation(t, transportPaths("example.com/fixture",
		[]string{"example.com/fixture/graph", "github.com/nats-io/nats.go/jetstream"}),
		"github.com/nats-io/nats.go/jetstream")
	if got := transportPaths("example.com/fixture", nil); len(got) == 0 {
		t.Fatal("an empty go list -deps listing passed; a check that saw nothing is not a pass")
	}
}

// graphTransportViolations lists every transport package go list -deps reports for pkg in root.
func graphTransportViolations(t *testing.T, root, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pkg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return transportPaths(modulePath(t, root), strings.Fields(string(out)))
}

// transportPaths reads go list -deps output and names each transport path in it: the module's
// natsclient package or anything under github.com/nats-io.
func transportPaths(module string, deps []string) []string {
	if len(deps) == 0 {
		return []string{"go list -deps listed no package; a check that saw nothing is not a pass " + graphTransportRule}
	}
	var violations []string
	for _, dep := range deps {
		if dep == module+"/natsclient" || strings.HasPrefix(dep, module+"/natsclient/") ||
			strings.HasPrefix(dep, "github.com/nats-io/") {
			violations = append(violations, "graph reaches "+dep+" "+graphTransportRule)
		}
	}
	return violations
}
