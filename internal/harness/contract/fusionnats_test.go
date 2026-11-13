package contract

import (
	"maps"
	"strings"
	"testing"
)

// fusionNATSRule is the requirement every violation names.
const fusionNATSRule = "(composed-stack › The graph stack answers fusion without an embedder)"

// fusionNATSForbidden are the module packages pkg/fusion/fusionnats must not reach: the embedding
// index and its component, which a stack without an embedder does not build, and the readiness
// watch, which fusionnats does not use (fusion spec, "No readiness import").
var fusionNATSForbidden = []string{"graph/embedding", "processor/graph-embedding", "graph/readiness"}

// TestFusionNATSImportsNoEmbedding holds the rule that pkg/fusion/fusionnats builds into a stack
// with no embedder: neither it nor any package it imports, directly or transitively, is
// graph/embedding, processor/graph-embedding or graph/readiness, or a package under one of them.
//
// Requirement: composed-stack/The graph stack answers fusion without an embedder
func TestFusionNATSImportsNoEmbedding(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "pkg/fusion/fusionnats embedding imports", fusionNATSEmbeddingPaths(
		modulePath(t, root), goList(t, root, "./pkg/fusion/fusionnats", "-deps", "{{.ImportPath}}")))
}

// Requirement: composed-stack/The graph stack answers fusion without an embedder
func TestFusionNATSImportsNoEmbeddingSensitivity(t *testing.T) {
	const module = "example.com/fixture"
	// The transitive edge, through go list over a real tree: fusionnats imports graph, and graph
	// imports graph/readiness.
	clean := map[string]string{
		"go.mod":                         "module " + module + "\n\ngo 1.26\n",
		"pkg/fusion/fusionnats/c.go":     "package fusionnats\n\nimport _ \"" + module + "/graph\"\n",
		"graph/g.go":                     "// Package graph names graph/embedding in a comment only.\npackage graph\n",
		"graph/readiness/r.go":           "package readiness\n",
		"graph/embeddingindex/e.go":      "package embeddingindex\n",
		"processor/graph-embedding/p.go": "package graphembedding\n",
	}
	root, _ := writeTree(t, clean)
	listing := func() []string {
		return goList(t, root, "./pkg/fusion/fusionnats", "-deps", "{{.ImportPath}}")
	}
	requireNoViolations(t, "clean fixture", fusionNATSEmbeddingPaths(module, listing()))

	reaching := maps.Clone(clean)
	reaching["graph/g.go"] = "package graph\n\nimport _ \"" + module + "/graph/readiness\"\n"
	root, _ = writeTree(t, reaching)
	requireViolation(t, fusionNATSEmbeddingPaths(module, listing()), module+"/graph/readiness", fusionNATSRule)

	// Each forbidden package, and one under it, in a planted listing; a sibling that has a
	// forbidden path as a name prefix is not that package.
	self := module + "/pkg/fusion/fusionnats"
	for _, forbidden := range fusionNATSForbidden {
		for _, dep := range []string{module + "/" + forbidden, module + "/" + forbidden + "/sub"} {
			requireViolation(t, fusionNATSEmbeddingPaths(module, []string{self, dep}), dep)
		}
	}
	requireNoViolations(t, "name-prefixed siblings", fusionNATSEmbeddingPaths(module, []string{self,
		module + "/graph/embeddingindex", module + "/graph/readinessx", module + "/processor/graph-embeddings"}))
	requireViolation(t, fusionNATSEmbeddingPaths(module, nil), "listed no package")
}

// fusionNATSEmbeddingPaths names each forbidden package, or package under one, in fusionnats'
// go list -deps listing deps. An empty listing is a violation: a check that saw nothing is not a pass.
func fusionNATSEmbeddingPaths(module string, deps []string) []string {
	if len(deps) == 0 {
		return []string{"go list -deps listed no package for pkg/fusion/fusionnats; a check that saw nothing is not a pass " +
			fusionNATSRule}
	}
	var violations []string
	for _, dep := range deps {
		for _, forbidden := range fusionNATSForbidden {
			path := module + "/" + forbidden
			if dep == path || strings.HasPrefix(dep, path+"/") {
				violations = append(violations, "pkg/fusion/fusionnats reaches "+dep+" "+fusionNATSRule)
			}
		}
	}
	return violations
}
