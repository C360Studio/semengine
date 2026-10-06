package contract

import (
	"go/ast"
	"go/types"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// authorityRule is the requirement every authority violation names.
const authorityRule = "(harness-boundaries › No second spelling of deployment authority)"

// TestNoSecondAuthorityFieldSensitivity plants the fixture of design D3 (change
// authority-one-spelling) in a temporary module: each field the rule forbids must be named by file,
// line and struct type, each field it allows must be silent, and the count must be exact. The
// expected lines are written from the requirement's scenarios 3 and 4, not from the check.
func TestNoSecondAuthorityFieldSensitivity(t *testing.T) {
	root, _ := writeTree(t, map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.26\n",
		// The pin's graph/inference.HierarchyConfig shape, plus lowercase fields that pass.
		"graph/inference/hierarchy.go": `package inference

type HierarchyConfig struct {
	Org      string
	Platform string
}

type cache struct {
	org      string
	platform string
}
`,
		"graph/inference/hierarchy_test.go": `package inference

type TestOnly struct {
	Org string
}
`,
		// The pin's graph/llm.EntityParts shape.
		"graph/llm/prompt_types.go": `package llm

type EntityParts struct {
	Org      string
	Platform string
	Domain   string
}
`,
		"graph/anon/anon.go": `package anon

var Defaults = struct {
	Org string
}{}
`,
		"internal/store/store.go": `package store

type Record struct {
	Platform string
}
`,
		"cmd/tool/main.go": `package main

type options struct {
	Platform string
}

func main() { _ = options{} }
`,
		// The owners: the source and its validators.
		"pkg/types/entity_id.go": `package types

type EntityID struct {
	Org      string
	Platform string
}
`,
		"pkg/platform/platform.go": `package platform

type Config struct {
	Org string
}
`,
		"config/config.go": `package config

type Config struct {
	Platform string
}
`,
		"types/component.go": `package types

type PlatformMeta struct {
	Org      string
	Platform string
}

type Other struct {
	Org string
}
`,
		// The carrier's and the caller claim's type names outside their packages: the exceptions are by
		// package, type and field, so these fail.
		"graph/inference/deps.go": `package inference

import "example.com/fixture/types"

type Dependencies struct {
	Platform types.PlatformMeta
}
`,
		"graph/llm/caller.go": `package llm

type CallerContext struct {
	Org string
}
`,
		// The carrier through the alias passes; a second field of the carrier's type fails.
		"component/dependencies.go": `package component

import "example.com/fixture/types"

type PlatformMeta = types.PlatformMeta

type Dependencies struct {
	Platform PlatformMeta
}

type Registry struct {
	Platform types.PlatformMeta
}
`,
		// A function-local type of the carrier's name is a different type: it gets no exception.
		"component/local.go": `package component

func clone() {
	type Dependencies struct {
		Platform PlatformMeta
	}
	_ = Dependencies{}
}
`,
		"service/dependencies.go": `package service

import "example.com/fixture/types"

type Dependencies struct {
	Platform types.PlatformMeta
}
`,
		"processor/rule/rule.go": `package rule

import "example.com/fixture/types"

type Dependencies struct {
	Platform types.PlatformMeta
}

type CallerContext struct {
	Org string
}

type Claim struct {
	Org string
}
`,
	})
	v, checked := authorityFieldViolations(t, root)

	failing := [][]string{
		{"graph/inference/hierarchy.go:4: field Org on example.com/fixture/graph/inference.HierarchyConfig "},
		{"graph/inference/hierarchy.go:5: field Platform on example.com/fixture/graph/inference.HierarchyConfig "},
		{"graph/llm/prompt_types.go:4: field Org on example.com/fixture/graph/llm.EntityParts "},
		{"graph/llm/prompt_types.go:5: field Platform on example.com/fixture/graph/llm.EntityParts "},
		{"graph/anon/anon.go:4: field Org on example.com/fixture/graph/anon.struct{…} "},
		{"internal/store/store.go:4: field Platform on example.com/fixture/internal/store.Record "},
		{"cmd/tool/main.go:4: field Platform on example.com/fixture/cmd/tool.options "},
		{"component/dependencies.go:12: field Platform on example.com/fixture/component.Registry "},
		{"processor/rule/rule.go:14: field Org on example.com/fixture/processor/rule.Claim "},
		{"types/component.go:9: field Org on example.com/fixture/types.Other "},
		{"graph/inference/deps.go:6: field Platform on example.com/fixture/graph/inference.Dependencies "},
		{"graph/llm/caller.go:4: field Org on example.com/fixture/graph/llm.CallerContext "},
		{"component/local.go:5: field Platform on example.com/fixture/component.Dependencies "},
	}
	for _, want := range failing {
		// The rule's name is written from the spec heading, not taken from authorityRule, so a
		// wrong constant fails here.
		requireViolation(t, v, append(want, "spells the deployment authority outside its owners",
			"(harness-boundaries › No second spelling of deployment authority)")...)
	}
	silent := []string{
		"graph/inference/hierarchy.go:9:", "graph/inference/hierarchy.go:10:", "hierarchy_test.go",
		"pkg/types/", "pkg/platform/", "config/", "types/component.go:4:", "types/component.go:5:",
		"component/dependencies.go:8:", "service/", "processor/rule/rule.go:6:", "processor/rule/rule.go:10:",
	}
	for _, line := range v {
		for _, s := range silent {
			if strings.HasPrefix(line, s) {
				t.Errorf("violation reported for an allowed or out-of-scope field: %s", line)
			}
		}
	}
	if len(v) != len(failing) {
		t.Errorf("want exactly %d violations, got %d:\n  %s", len(failing), len(v), strings.Join(v, "\n  "))
	}
	if checked != 12 {
		t.Errorf("want 12 fixture packages checked, got %d", checked)
	}
	empty, _ := writeTree(t, map[string]string{"go.mod": "module example.com/empty\n\ngo 1.26\n"})
	got, n := authorityFieldViolations(t, empty)
	if n != 0 || len(got) != 1 || got[0] != "checked no package of example.com/empty" {
		t.Errorf("a module with no package: want one \"checked no package\" violation, got %d packages and %q", n, got)
	}
}

// authorityOwnerPackages and authorityOwnerFields mirror, by exact string, the exceptions the
// harness-boundaries requirement "No second spelling of deployment authority" lists; the list is the
// spec's, and a new entry needs a spec change first (design D4). Paths are relative to the module
// path read from go.mod, so a fixture module is excepted the same way this one is. A field's type is
// never consulted: the carrier is excepted field by field (#72 comment 5969757891).
var authorityOwnerPackages = map[string]string{
	"pkg/types":    "the source: the entity identity's positions",
	"pkg/platform": "the source: the platform configuration",
	"config":       "the source: the composition root's configuration and its validators",
}

var authorityOwnerFields = map[string]string{
	"types.PlatformMeta.Org":               "the carrier's own field",
	"types.PlatformMeta.Platform":          "the carrier's own field",
	"component.Dependencies.Platform":      "the carrier deps.Platform",
	"service.Dependencies.Platform":        "the carrier deps.Platform",
	"processor/rule.Dependencies.Platform": "the carrier deps.Platform",
	"processor/rule.CallerContext.Org":     "a caller's organization claim, not the deployment authority",
}

// TestNoSecondAuthorityField fails on an exported struct field named Org or Platform outside the
// owners the requirement lists. It adopts TestNoRetainedContext's shape (context_test.go): the
// same non-test package load and *ast.StructType walk as contextViolations, an exact-name exception
// map as contextExemptTypes, and the writeTree/requireViolation sensitivity pair (repo_test.go).
func TestNoSecondAuthorityField(t *testing.T) {
	v, checked := authorityFieldViolations(t, repoRoot(t))
	requireNoViolations(t, "deployment authority field", v)
	t.Logf("checked %d packages", checked)
}

// authorityFieldViolations walks every struct, named or anonymous, in every non-test file of every
// package of the module at root (built with the integration tag) and returns one sorted line per
// forbidden field, with the number of module packages it checked.
func authorityFieldViolations(t *testing.T, root string) ([]string, int) {
	t.Helper()
	modulePath := modulePathOf(t, root)
	loaded, err := packages.Load(&packages.Config{
		Dir:        root,
		BuildFlags: []string{"-tags=integration"},
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
	}, "./...")
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	var loadErrs []string
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		for _, e := range pkg.Errors {
			loadErrs = append(loadErrs, e.Error())
		}
	})
	if len(loadErrs) > 0 {
		sort.Strings(loadErrs)
		t.Fatalf("type-check packages:\n  %s", strings.Join(loadErrs, "\n  "))
	}

	var violations []string
	checked := 0
	for _, pkg := range loaded {
		rel, ok := moduleRelative(modulePath, pkg.PkgPath)
		if pkg.Types == nil || !ok {
			continue
		}
		checked++
		if authorityOwnerPackages[rel] != "" {
			continue
		}
		for _, file := range pkg.Syntax {
			named := map[*ast.StructType]string{}
			// Only a package-scope declaration is the type an exception names; a type of the same name
			// declared inside a function body is a different type and gets no exception.
			packageScope := map[*ast.StructType]bool{}
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gen.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if structNode, ok := typeSpec.Type.(*ast.StructType); ok {
							packageScope[structNode] = true
						}
					}
				}
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if spec, ok := node.(*ast.TypeSpec); ok {
					if structNode, ok := spec.Type.(*ast.StructType); ok {
						named[structNode] = spec.Name.Name
					}
					return true
				}
				structNode, ok := node.(*ast.StructType)
				if !ok {
					return true
				}
				structType, ok := pkg.TypesInfo.TypeOf(structNode).Underlying().(*types.Struct)
				if !ok {
					return true
				}
				typeName, isNamed := named[structNode]
				if !isNamed {
					typeName = "struct{…}"
				}
				for i := 0; i < structType.NumFields(); i++ {
					field := structType.Field(i)
					if !field.Exported() || (field.Name() != "Org" && field.Name() != "Platform") {
						continue
					}
					if packageScope[structNode] && authorityOwnerFields[rel+"."+typeName+"."+field.Name()] != "" {
						continue
					}
					violations = append(violations, formatField(root, pkg, field,
						"on "+pkg.PkgPath+"."+typeName+" spells the deployment authority outside its owners "+authorityRule))
				}
				return true
			})
		}
	}
	sort.Strings(violations)
	// A load that found no package of the module would pass vacuously; it fails instead.
	if checked == 0 {
		violations = append(violations, "checked no package of "+modulePath)
	}
	return violations, checked
}

// moduleRelative returns path relative to modulePath ("" for the module's root package) and whether
// path is in the module.
func moduleRelative(modulePath, path string) (string, bool) {
	if path == modulePath {
		return "", true
	}
	rel, ok := strings.CutPrefix(path, modulePath+"/")
	return rel, ok
}
