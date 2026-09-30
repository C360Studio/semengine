package contract

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// T-B2: no non-test struct retains a context.Context (adapted from SemStreams
// test/contract/context_ownership_contract_test.go at 5457b345, ledger row L7). Context parameters
// on callbacks remain valid; stored values and provider results are rejected, and an exported
// context.CancelFunc field is rejected because a lifecycle record must not hand out cancellation.
func TestNoRetainedContext(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "context ownership", contextViolations(t, root))
}

func TestNoRetainedContextSensitivity(t *testing.T) {
	root, _ := writeTree(t, map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.26\n",
		"fixture/fixture.go": `package fixture

import (
	"context"
	"sync"
)

type Fixture struct {
	mu  sync.Mutex
	ctx context.Context
}

type Embedded struct{ context.Context }

type Alias = context.Context

type Aliased struct{ held Alias }

type Container struct{ held map[string][]context.Context }

type Provider struct{ authority func() context.Context }

type Exported struct{ Cancel context.CancelFunc }

type Allowed struct {
	cancel   context.CancelFunc
	callback func(context.Context) error
}
`,
		"fixture/fixture_test.go": `package fixture

import "context"

type testOnly struct{ ctx context.Context }
`,
	})
	v := contextViolations(t, root)
	requireViolation(t, v, "fixture/fixture.go", "field ctx", "stores context.Context")
	requireViolation(t, v, "fixture/fixture.go", "field Context", "stores context.Context")
	requireViolation(t, v, "fixture/fixture.go", "field held", "stores context.Context")
	requireViolation(t, v, "fixture/fixture.go", "field authority", "provides context.Context")
	requireViolation(t, v, "fixture/fixture.go", "field Cancel", "exports context.CancelFunc")
	for _, line := range v {
		if strings.Contains(line, "fixture_test.go") || strings.Contains(line, "field cancel ") || strings.Contains(line, "field callback") {
			t.Errorf("violation reported for an allowed form: %s", line)
		}
	}
	if len(v) != 6 {
		t.Errorf("want exactly 6 violations (ctx, Context, 2x held, authority, Cancel), got %d:\n  %v", len(v), v)
	}
}

// TestContextFieldDetectorMatrix is carried from SemStreams with the fixture package renamed; it
// pins the detector's reach independently of any repository content.
func TestContextFieldDetectorMatrix(t *testing.T) {
	contextPkg, err := importer.Default().Import("context")
	if err != nil {
		t.Fatalf("import context: %v", err)
	}
	contextType := contextPkg.Scope().Lookup("Context").Type()
	cancelType := contextPkg.Scope().Lookup("CancelFunc").Type()
	fixturePkg := types.NewPackage("example.com/fixture/contextfixture", "contextfixture")
	detector := contextFieldDetector{
		contextInterface: contextType.Underlying().(*types.Interface),
		cancelType:       cancelType,
		modulePath:       "example.com/fixture",
	}
	named := func(name string, underlying types.Type) *types.Named {
		return types.NewNamed(types.NewTypeName(token.NoPos, fixturePkg, name, nil), underlying, nil)
	}
	alias := func(name string, target types.Type) *types.Alias {
		return types.NewAlias(types.NewTypeName(token.NoPos, fixturePkg, name, nil), target)
	}
	provider := func(result types.Type) *types.Signature {
		results := types.NewTuple(types.NewVar(token.NoPos, fixturePkg, "result", result))
		return types.NewSignatureType(nil, nil, nil, nil, results, false)
	}
	callback := func(input types.Type) *types.Signature {
		params := types.NewTuple(types.NewVar(token.NoPos, fixturePkg, "input", input))
		return types.NewSignatureType(nil, nil, nil, params, nil, false)
	}
	providerInterface := func(name string, result types.Type) *types.Named {
		method := types.NewFunc(token.NoPos, fixturePkg, "Authority", provider(result))
		return named(name, types.NewInterfaceType([]*types.Func{method}, nil).Complete())
	}
	callbackInterface := func(name string, input types.Type) *types.Named {
		method := types.NewFunc(token.NoPos, fixturePkg, "Handle", callback(input))
		return named(name, types.NewInterfaceType([]*types.Func{method}, nil).Complete())
	}

	contextAlias := alias("ContextAlias", contextType)
	contextWrapper := named("ContextWrapper", types.NewStruct(
		[]*types.Var{types.NewVar(token.NoPos, fixturePkg, "ctx", contextAlias)}, nil,
	))
	contextProviderResult := named("ContextProviderResult", contextType.Underlying())
	for _, test := range []struct {
		name string
		typ  types.Type
		want bool
	}{
		{name: "direct", typ: contextType, want: true},
		{name: "alias", typ: contextAlias, want: true},
		{name: "pointer", typ: types.NewPointer(contextType), want: true},
		{name: "array", typ: types.NewArray(contextAlias, 1), want: true},
		{name: "slice", typ: types.NewSlice(contextWrapper), want: true},
		{name: "map", typ: types.NewMap(types.Typ[types.String], contextType), want: true},
		{name: "channel", typ: types.NewChan(types.SendRecv, contextAlias), want: true},
		{name: "struct wrapper", typ: contextWrapper, want: true},
		{name: "provider", typ: provider(contextAlias), want: true},
		{name: "interface provider named result", typ: providerInterface("ContextProvider", contextProviderResult), want: true},
		{name: "input callback", typ: callback(contextType), want: false},
		{name: "input interface", typ: callbackInterface("ContextCallback", contextType), want: false},
	} {
		t.Run("context/"+test.name, func(t *testing.T) {
			got := detector.contextReason(test.typ, make(map[types.Type]bool)) != ""
			if got != test.want {
				t.Fatalf("context authority detected = %t, want %t", got, test.want)
			}
		})
	}

	cancelAlias := alias("CancelAlias", cancelType)
	cancelWrapper := named("CancelWrapper", types.NewStruct(
		[]*types.Var{types.NewVar(token.NoPos, fixturePkg, "Cancel", cancelAlias)}, nil,
	))
	for _, test := range []struct {
		name string
		typ  types.Type
		want bool
	}{
		{name: "direct", typ: cancelType, want: true},
		{name: "alias", typ: cancelAlias, want: true},
		{name: "pointer", typ: types.NewPointer(cancelType), want: true},
		{name: "array", typ: types.NewArray(cancelAlias, 1), want: true},
		{name: "slice", typ: types.NewSlice(cancelWrapper), want: true},
		{name: "map", typ: types.NewMap(types.Typ[types.String], cancelType), want: true},
		{name: "channel", typ: types.NewChan(types.SendRecv, cancelAlias), want: true},
		{name: "struct wrapper", typ: cancelWrapper, want: true},
		{name: "provider", typ: provider(cancelAlias), want: true},
		{name: "interface provider", typ: providerInterface("CancelProvider", cancelAlias), want: true},
		{name: "input callback", typ: callback(cancelType), want: false},
		{name: "input interface", typ: callbackInterface("CancelCallback", cancelType), want: false},
		{name: "ordinary no-argument callback", typ: types.NewSignatureType(nil, nil, nil, nil, nil, false), want: false},
	} {
		t.Run("cancel/"+test.name, func(t *testing.T) {
			got := detector.isCancelFunc(test.typ)
			if got != test.want {
				t.Fatalf("cancel authority detected = %t, want %t", got, test.want)
			}
		})
	}
}

// contextExemptTypes are context implementations, not owners: a type that is itself a
// context.Context must hold its parent, exactly as the standard library's derived contexts do.
// Exemption is by exact type name so an owner cannot gain it by embedding context.Context.
var contextExemptTypes = map[string]string{
	"github.com/c360studio/semengine/internal/harness/probe.ObservedContext": "a derived Context that signals its first Done() observation (lifecycle-suite › Probes)",
}

// contextViolations is the T-B2 check over every non-test package in the module at root, built with
// the integration tag so tagged files are type-checked too.
func contextViolations(t *testing.T, root string) []string {
	t.Helper()
	modulePath := modulePathOf(t, root)
	loaded, err := packages.Load(&packages.Config{
		Dir:        root,
		BuildFlags: []string{"-tags=integration"},
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
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
	contextPkg := importedPackage(loaded, "context")
	if contextPkg == nil {
		// No package imports context, so no struct can hold one.
		return nil
	}
	contextType := contextPkg.Scope().Lookup("Context").Type()
	detector := contextFieldDetector{
		contextInterface: types.Unalias(contextType).Underlying().(*types.Interface),
		cancelType:       contextPkg.Scope().Lookup("CancelFunc").Type(),
		modulePath:       modulePath,
	}

	var violations []string
	for _, pkg := range loaded {
		if pkg.Types == nil || !detector.inModule(pkg.PkgPath) {
			continue
		}
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				spec, ok := node.(*ast.TypeSpec)
				if ok && contextExemptTypes[pkg.PkgPath+"."+spec.Name.Name] != "" {
					return false
				}
				structNode, ok := node.(*ast.StructType)
				if !ok {
					return true
				}
				structType, ok := pkg.TypesInfo.TypeOf(structNode).Underlying().(*types.Struct)
				if !ok {
					return true
				}
				for i := 0; i < structType.NumFields(); i++ {
					field := structType.Field(i)
					if reason := detector.contextReason(field.Type(), map[types.Type]bool{}); reason != "" {
						violations = append(violations, formatField(root, pkg, field, reason))
					}
					if field.Exported() && detector.isCancelFunc(field.Type()) {
						violations = append(violations, formatField(root, pkg, field, "exports context.CancelFunc"))
					}
				}
				return true
			})
		}
	}
	sort.Strings(violations)
	return violations
}

func modulePathOf(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	t.Fatal("go.mod has no module line")
	return ""
}

func importedPackage(pkgs []*packages.Package, path string) *types.Package {
	var found *types.Package
	packages.Visit(pkgs, func(pkg *packages.Package) bool {
		if pkg.Types != nil && pkg.Types.Path() == path {
			found = pkg.Types
			return false
		}
		return found == nil
	}, nil)
	return found
}

func formatField(root string, pkg *packages.Package, field *types.Var, reason string) string {
	position := pkg.Fset.Position(field.Pos())
	path, err := filepath.Rel(root, position.Filename)
	if err != nil {
		path = position.Filename
	}
	return fmt.Sprintf("%s:%d: field %s %s", filepath.ToSlash(path), position.Line, field.Name(), reason)
}

type contextFieldDetector struct {
	contextInterface *types.Interface
	cancelType       types.Type
	modulePath       string
}

func (d contextFieldDetector) inModule(path string) bool {
	return path == d.modulePath || strings.HasPrefix(path, d.modulePath+"/")
}

// descend reports whether a named type's underlying structure is inspected: always for non-struct
// types, and for structs only when the module owns them. A foreign struct (sync.Mutex, a library
// client) is opaque; its own authors answer for what it holds.
func (d contextFieldDetector) descend(named *types.Named) bool {
	_, isStruct := named.Underlying().(*types.Struct)
	return !isStruct || (named.Obj().Pkg() != nil && d.inModule(named.Obj().Pkg().Path()))
}

func (d contextFieldDetector) contextReason(typ types.Type, seen map[types.Type]bool) string {
	typ = types.Unalias(typ)
	if seen[typ] {
		return ""
	}
	seen[typ] = true
	if types.Implements(typ, d.contextInterface) {
		return "stores context.Context"
	}
	switch c := typ.(type) {
	case *types.Pointer:
		return d.contextReason(c.Elem(), seen)
	case *types.Named:
		if d.descend(c) {
			return d.contextReason(c.Underlying(), seen)
		}
	case *types.Array:
		return d.contextReason(c.Elem(), seen)
	case *types.Slice:
		return d.contextReason(c.Elem(), seen)
	case *types.Map:
		if r := d.contextReason(c.Key(), seen); r != "" {
			return r
		}
		return d.contextReason(c.Elem(), seen)
	case *types.Chan:
		return d.contextReason(c.Elem(), seen)
	case *types.Struct:
		for i := 0; i < c.NumFields(); i++ {
			if r := d.contextReason(c.Field(i).Type(), seen); r != "" {
				return r
			}
		}
	case *types.Signature:
		for i := 0; i < c.Results().Len(); i++ {
			if d.contextReason(c.Results().At(i).Type(), seen) != "" {
				return "provides context.Context"
			}
		}
	case *types.Interface:
		for i := 0; i < c.NumMethods(); i++ {
			if d.contextReason(c.Method(i).Type(), seen) != "" {
				return "provides context.Context"
			}
		}
	}
	return ""
}

func (d contextFieldDetector) isCancelFunc(typ types.Type) bool {
	return d.containsCancelFunc(typ, map[types.Type]bool{})
}

func (d contextFieldDetector) containsCancelFunc(typ types.Type, seen map[types.Type]bool) bool {
	typ = types.Unalias(typ)
	if seen[typ] {
		return false
	}
	seen[typ] = true
	if types.Identical(typ, types.Unalias(d.cancelType)) {
		return true
	}
	switch c := typ.(type) {
	case *types.Pointer:
		return d.containsCancelFunc(c.Elem(), seen)
	case *types.Named:
		if d.descend(c) {
			return d.containsCancelFunc(c.Underlying(), seen)
		}
	case *types.Array:
		return d.containsCancelFunc(c.Elem(), seen)
	case *types.Slice:
		return d.containsCancelFunc(c.Elem(), seen)
	case *types.Map:
		return d.containsCancelFunc(c.Key(), seen) || d.containsCancelFunc(c.Elem(), seen)
	case *types.Chan:
		return d.containsCancelFunc(c.Elem(), seen)
	case *types.Struct:
		for i := 0; i < c.NumFields(); i++ {
			f := c.Field(i)
			if (f.Exported() || f.Embedded()) && d.containsCancelFunc(f.Type(), seen) {
				return true
			}
		}
	case *types.Signature:
		for i := 0; i < c.Results().Len(); i++ {
			if d.containsCancelFunc(c.Results().At(i).Type(), seen) {
				return true
			}
		}
	case *types.Interface:
		for i := 0; i < c.NumMethods(); i++ {
			if m := c.Method(i); m.Exported() && d.containsCancelFunc(m.Type(), seen) {
				return true
			}
		}
	}
	return false
}
