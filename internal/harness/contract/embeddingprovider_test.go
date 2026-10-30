package contract

import (
	"os/exec"
	"strings"
	"testing"
)

// embeddingProviderRule is the requirement every violation names.
const embeddingProviderRule = "(embedding-index › Tier 0 imports no provider code)"

// openAIModule is the provider SDK the pin's excluded http_embedder.go imported.
const openAIModule = "github.com/sashabaranov/go-openai"

// TestEmbeddingImportsNoProviderModule holds the rule that graph/embedding builds at tier 0:
// it reaches neither the OpenAI SDK nor the module's model package, directly or through a
// package it imports (#35).
//
// Requirement: embedding-index/Tier 0 imports no provider code
func TestEmbeddingImportsNoProviderModule(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "graph/embedding provider imports",
		embeddingProviderViolations(t, root, "./graph/embedding"))
}

// queryProviderRule is the requirement every graph/query violation names.
const queryProviderRule = "(query-classification › Tier 0 classification imports no provider code)"

// TestQueryImportsNoProviderModule holds the rule that graph/query builds at tier 0: without the
// pin's LLM client adapter it reaches neither the OpenAI SDK nor the module's model package,
// directly or through a package it imports. It reuses the embedding guard's listing and reader;
// TestEmbeddingImportsNoProviderModuleSensitivity shows that reader can fail.
//
// Requirement: query-classification/Tier 0 classification imports no provider code
func TestQueryImportsNoProviderModule(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "graph/query provider imports",
		providerViolations(t, root, "./graph/query", queryProviderRule))
}

// Requirement: embedding-index/Tier 0 imports no provider code
func TestEmbeddingImportsNoProviderModuleSensitivity(t *testing.T) {
	const mod = "go.mod"
	clean := map[string]string{
		mod:                    "module example.com/fixture\n\ngo 1.26\n",
		"graph/embedding/e.go": "package embedding\n\nimport _ \"example.com/fixture/vocab\"\n",
		"vocab/v.go":           "// Package vocab mentions model and go-openai in a comment only.\npackage vocab\n",
		"model/m.go":           "package model\n",
	}
	root, _ := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", embeddingProviderViolations(t, root, "./graph/embedding"))

	// The package reaches model through a package it imports: the transitive edge is the case.
	reaching := map[string]string{
		mod:                    "module example.com/fixture\n\ngo 1.26\n",
		"graph/embedding/e.go": "package embedding\n\nimport _ \"example.com/fixture/vocab\"\n",
		"vocab/v.go":           "package vocab\n\nimport _ \"example.com/fixture/model\"\n",
		"model/m.go":           "package model\n",
	}
	root, _ = writeTree(t, reaching)
	requireViolation(t, embeddingProviderViolations(t, root, "./graph/embedding"), "example.com/fixture/model")

	// The OpenAI SDK in the listing, read from planted go list output: the fixture module
	// cannot fetch the real one. A sibling with model as a name prefix is not the model package.
	requireViolation(t, providerPaths("example.com/fixture",
		[]string{"example.com/fixture/graph/embedding", openAIModule}), openAIModule)
	requireViolation(t, providerPaths("example.com/fixture",
		[]string{"example.com/fixture/graph/embedding", "example.com/fixture/model/wire"}),
		"example.com/fixture/model/wire")
	requireNoViolations(t, "model-prefixed sibling", providerPaths("example.com/fixture",
		[]string{"example.com/fixture/graph/embedding", "example.com/fixture/modelregistry"}))
	if got := providerPaths("example.com/fixture", nil); len(got) == 0 {
		t.Fatal("an empty go list -deps listing passed; a check that saw nothing is not a pass")
	}
}

// embeddingProviderViolations lists every provider package go list -deps reports for pkg in root,
// naming the embedding requirement.
func embeddingProviderViolations(t *testing.T, root, pkg string) []string {
	t.Helper()
	return providerViolations(t, root, pkg, embeddingProviderRule)
}

// providerViolations lists every provider package go list -deps reports for pkg in root; each
// violation names pkg and rule.
func providerViolations(t *testing.T, root, pkg, rule string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pkg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return providerPathsFor(modulePath(t, root), pkg, rule, strings.Fields(string(out)))
}

// providerPaths is providerPathsFor graph/embedding and its requirement.
func providerPaths(module string, deps []string) []string {
	return providerPathsFor(module, "./graph/embedding", embeddingProviderRule, deps)
}

// providerPathsFor reads go list -deps output for pkg and names each provider path in it: the
// module's model package or anything under it, or the OpenAI SDK or anything under it. Each
// violation names pkg and rule.
func providerPathsFor(module, pkg, rule string, deps []string) []string {
	if len(deps) == 0 {
		return []string{"go list -deps listed no package; a check that saw nothing is not a pass " + rule}
	}
	var violations []string
	for _, dep := range deps {
		if dep == module+"/model" || strings.HasPrefix(dep, module+"/model/") ||
			dep == openAIModule || strings.HasPrefix(dep, openAIModule+"/") {
			violations = append(violations, strings.TrimPrefix(pkg, "./")+" reaches "+dep+" "+rule)
		}
	}
	return violations
}
