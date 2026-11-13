package contract

import (
	"os/exec"
	"strings"
	"testing"
)

// embeddingProviderRule is the requirement every violation names.
const embeddingProviderRule = "(embedding-index › Tier 0 imports no provider code)"

// openAIModule is the OpenAI provider SDK, which tier 0 code never reaches.
const openAIModule = "github.com/sashabaranov/go-openai"

// TestEmbeddingImportsNoProviderModule holds the rule that graph/embedding builds at tier 0:
// it reaches neither the OpenAI SDK nor the module's model package, directly or through a
// package it imports.
//
// Requirement: embedding-index/Tier 0 imports no provider code
func TestEmbeddingImportsNoProviderModule(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "graph/embedding provider imports",
		embeddingProviderViolations(t, root, "./graph/embedding"))
}

// queryProviderRule is the requirement every graph/query violation names.
const queryProviderRule = "(query-classification › Tier 0 classification imports no provider code)"

// TestQueryImportsNoProviderModule holds the rule that graph/query builds at tier 0: it reaches
// neither the OpenAI SDK nor the module's model package, directly or through a package it imports.
// It reuses the embedding guard's listing and reader;
// TestEmbeddingImportsNoProviderModuleSensitivity shows that reader can fail.
//
// Requirement: query-classification/Tier 0 classification imports no provider code
func TestQueryImportsNoProviderModule(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "graph/query provider imports",
		providerViolations(t, root, "./graph/query", queryProviderRule))
}

// graphEmbeddingProviderRule is the requirement every processor/graph-embedding violation names.
const graphEmbeddingProviderRule = "(graph-embedding › Tier-0 config)"

// TestGraphEmbeddingImportsNoProviderModule holds the rule that processor/graph-embedding builds at
// tier 0 without an HTTP embedder: it does not import the module's model package, and it reaches
// the OpenAI SDK through nothing it imports.
// model is held to graph-embedding's own imports because component reaches it for every
// component (component.Dependencies' ModelRegistry), with no provider code behind it at tier 0.
//
// Requirement: graph-embedding/Tier-0 config
func TestGraphEmbeddingImportsNoProviderModule(t *testing.T) {
	requireNoDirectModelProvider(t, "./processor/graph-embedding", graphEmbeddingProviderRule)
}

// llmPlumbingRule is the rule every processor/graph-clustering and tree-wide violation names: tier
// 0 builds without the LLM plumbing, so it needs no external provider.
const llmPlumbingRule = "(#32 › tier 0 builds without the LLM plumbing)"

// TestGraphClusteringImportsNoProviderModule holds processor/graph-clustering to graph-embedding's
// guard: it does not import the module's model package, and it reaches the OpenAI SDK through
// nothing it imports. graph-clustering has no LLM summarizer or anomaly review, the code that would
// need graph/llm.
func TestGraphClusteringImportsNoProviderModule(t *testing.T) {
	requireNoDirectModelProvider(t, "./processor/graph-clustering", llmPlumbingRule)
}

// requireNoDirectModelProvider fails t on each provider path in pkg's listings, naming rule.
func requireNoDirectModelProvider(t *testing.T, pkg, rule string) {
	t.Helper()
	root := repoRoot(t)
	requireNoViolations(t, strings.TrimPrefix(pkg, "./")+" provider imports", directModelProviderPaths(
		modulePath(t, root), pkg, rule, goList(t, root, pkg, "{{join .Imports \" \"}}"),
		goList(t, root, pkg, "-deps", "{{.ImportPath}}")))
}

// Requirement: graph-embedding/Tier-0 config
func TestGraphEmbeddingImportsNoProviderModuleSensitivity(t *testing.T) {
	const module = "example.com/fixture"
	for _, pkg := range []string{"./processor/graph-embedding", "./processor/graph-clustering"} {
		t.Run(strings.TrimPrefix(pkg, "./processor/"), func(t *testing.T) {
			name := strings.TrimPrefix(pkg, "./")
			self := module + "/" + name
			component := []string{module + "/component", module + "/model", self}
			paths := func(direct, deps []string) []string {
				return directModelProviderPaths(module, pkg, "(rule)", direct, deps)
			}
			requireNoViolations(t, "model reached only through component",
				paths([]string{module + "/component", module + "/modelregistry"}, component))
			requireViolation(t, paths([]string{module + "/model"}, component), module+"/model", name, "(rule)")
			requireViolation(t, paths([]string{module + "/model/wire"}, component), module+"/model/wire")
			requireViolation(t, paths([]string{module + "/component"}, append(component, openAIModule+"/jsonschema")),
				openAIModule+"/jsonschema", name, "(rule)")
			requireViolation(t, paths([]string{module + "/component"}, nil), "listed no package")
			requireViolation(t, paths(nil, component), "direct .Imports listing was empty")
		})
	}
}

// directModelProviderPaths names each provider path in pkg's listings: the module's model package,
// or anything under it, among direct, its own imports; the OpenAI SDK, or anything under it, among
// deps, everything it reaches. Each violation names pkg and rule; an empty listing is a violation.
func directModelProviderPaths(module, pkg, rule string, direct, deps []string) []string {
	var violations []string
	if len(direct) == 0 {
		violations = append(violations, "go list's direct .Imports listing was empty; a check that saw nothing is not a pass "+
			rule)
	} else {
		violations = providerPathsFor(module, pkg, rule, direct)
	}
	if len(deps) == 0 {
		return append(violations, "go list -deps listed no package; a check that saw nothing is not a pass "+rule)
	}
	for _, dep := range deps {
		if under(dep, openAIModule) {
			violations = append(violations, strings.TrimPrefix(pkg, "./")+" reaches "+dep+" "+rule)
		}
	}
	return violations
}

// TestTreeImportsNoProviderPlumbing holds the module, every package go list ./... builds, to the
// owner ruling that tier 0 builds without the LLM plumbing. Nothing reaches the LLM client
// (graph/llm), its wire types (model/wire) or the OpenAI SDK, directly or through a package
// it imports. Neither graph/llm nor model/wire is in the tree; this guards their return.
// It replaces no per-package guard: those also forbid model, which component reaches for every
// component (TestGraphEmbeddingImportsNoProviderModule), so the tree-wide listing cannot.
// TestTreeImportsNoProviderPlumbingSensitivity shows the reader can fail.
func TestTreeImportsNoProviderPlumbing(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "tree-wide provider plumbing", treeProviderPlumbingPaths(modulePath(t, root),
		goList(t, root, "./...", "-deps", "{{.ImportPath}}")))
}

func TestTreeImportsNoProviderPlumbingSensitivity(t *testing.T) {
	const module = "example.com/fixture"
	clean := []string{module + "/component", module + "/model", module + "/graph", module + "/graph/llmx",
		module + "/model/wirex"}
	requireNoViolations(t, "model and name-prefixed siblings", treeProviderPlumbingPaths(module, clean))
	for _, plumbing := range []string{module + "/graph/llm", module + "/graph/llm/openai", module + "/model/wire",
		module + "/model/wire/v2", openAIModule, openAIModule + "/jsonschema"} {
		requireViolation(t, treeProviderPlumbingPaths(module, append(clean, plumbing)), plumbing, llmPlumbingRule)
	}
	requireViolation(t, treeProviderPlumbingPaths(module, nil), "listed no package", llmPlumbingRule)
}

// treeProviderPlumbingPaths reads go list -deps ./... output and names each LLM plumbing path in it:
// the module's graph/llm or model/wire, or anything under either, or the OpenAI SDK or anything
// under it. The module's model package is allowed. An empty listing is a violation.
func treeProviderPlumbingPaths(module string, deps []string) []string {
	if len(deps) == 0 {
		return []string{"go list -deps ./... listed no package; a check that saw nothing is not a pass " +
			llmPlumbingRule}
	}
	var violations []string
	for _, dep := range deps {
		if under(dep, module+"/graph/llm") || under(dep, module+"/model/wire") || under(dep, openAIModule) {
			violations = append(violations, "the module reaches "+dep+" "+llmPlumbingRule)
		}
	}
	return violations
}

// under reports whether path is root or a package under it; a name-prefixed sibling is not.
func under(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}

// goList runs go list in root for pkg with args, the last a -f template, and returns its fields.
func goList(t *testing.T, root, pkg string, args ...string) []string {
	t.Helper()
	format := args[len(args)-1]
	cmd := exec.Command("go", append(append([]string{"list"}, args[:len(args)-1]...), "-f", format, pkg)...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list %v %s: %v", args, pkg, err)
	}
	return strings.Fields(string(out))
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
		if under(dep, module+"/model") || under(dep, openAIModule) {
			violations = append(violations, strings.TrimPrefix(pkg, "./")+" reaches "+dep+" "+rule)
		}
	}
	return violations
}
