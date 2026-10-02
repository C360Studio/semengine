package contract

import (
	"fmt"
	"go/types"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// The public-signature rule (harness-boundaries › "Public signatures name no internal type"; owner
// rulings #9 comments 5953295358 and 5953477174): no exported identifier of a public package names
// a type declared in an internal package of this module, directly or through anything a caller
// outside the module reaches from it.
func TestPublicSignatures(t *testing.T) {
	root := repoRoot(t)
	violations, public := publicSignatureViolations(t, root)
	// Until the floor's packages are ported the module has no public package, so this run checks
	// nothing; section 3's ports are what it then holds to the rule.
	t.Logf("public packages checked: %d", public)
	requireNoViolations(t, "public signature", violations)
}

// signatureFixture plants one violation per reach the requirement names, a clean public package,
// and three places the rule does not apply (a main package, a test file and a nested internal
// package's own API).
var signatureFixture = map[string]string{
	"go.mod": "module example.com/fixture\n\ngo 1.26\n",
	"internal/secret/secret.go": `package secret

type T struct{ N int }

type Iface interface{ Do() }

type C interface{ ~int }

type Kind int
`,
	"pub/nested/internal/deep/deep.go": "package deep\n\ntype D struct{}\n",
	"pub/pub.go": `package pub

import "example.com/fixture/internal/secret"

// Direct result.
func Result() *secret.T { return nil }

// Exported method of an exported type.
type Method struct{}

func (Method) Get() secret.T { return secret.T{} }

// Embedded field: an internal type, and an unexported type whose exported method is promoted.
type Embedded struct{ secret.T }

type promoted struct{}

func (promoted) Promoted() secret.T { return secret.T{} }

type Promoting struct{ promoted }

// Exported, non-embedded struct field.
type Field struct{ Value secret.T }

// Interface method set, reached only through an unexported embedded interface.
type iinner interface{ Do() secret.T }

type Iface interface{ iinner }

// Type argument of a generic whose field is unexported, so only the argument reaches.
type box[X any] struct{ v X }

func TypeArg() box[secret.T] { return box[secret.T]{} }

// Generic constraint on a function and on a type, neither type parameter used elsewhere, so only
// the constraint reaches.
func Constrained[X secret.C]() {}

type Gen[X secret.C] struct{}

// Alias.
type Alias = secret.T

// Exported variable and constant.
var Var secret.T

const Const secret.Kind = 1

// Unexported type returned by an exported function: its exported method reaches.
type hidden struct{}

func (hidden) Leak() secret.T { return secret.T{} }

func Hidden() hidden { return hidden{} }

// Recursive constraint: the walk must terminate.
func Self[X interface{ Next() X }](x X) X { return x }
`,
	"pub/nested/nested.go": `package nested

import "example.com/fixture/pub/nested/internal/deep"

// An internal package nested below a public one is internal too.
func Nested() deep.D { return deep.D{} }
`,
	"pub/clean/clean.go": `package clean

import "example.com/fixture/internal/secret"

type Clean struct{ held secret.T }

func (c Clean) get() secret.T { return c.held }

func (c Clean) Count() int { return c.get().N }

type private struct{ Value secret.T }

func helper() private { return private{} }

func Body() int {
	var t secret.T
	return t.N + helper().Value.N
}

var unexported secret.T
`,
	"pub/pub_test.go": `package pub

import "example.com/fixture/internal/secret"

func TestOnly() secret.T { return secret.T{} }
`,
	"cmd/tool/main.go": `package main

import "example.com/fixture/internal/secret"

func Exported() secret.T { return secret.T{} }

func main() { _ = Exported() }
`,
	"internal/other/other.go": `package other

import "example.com/fixture/internal/secret"

func Exported() secret.T { return secret.T{} }
`,
}

func TestPublicSignaturesSensitivity(t *testing.T) {
	root, _ := writeTree(t, signatureFixture)
	violations, public := publicSignatureViolations(t, root)
	t.Logf("violations:\n  %s", strings.Join(violations, "\n  "))
	if public != 3 {
		t.Errorf("want 3 public packages (pub, pub/nested, pub/clean), got %d", public)
	}
	const (
		pub    = "example.com/fixture/pub."
		secret = "example.com/fixture/internal/secret."
	)
	want := [][2]string{
		{pub + "Result", secret + "T"},
		{pub + "Method", secret + "T"},
		{pub + "Embedded", secret + "T"},
		{pub + "Promoting", secret + "T"},
		{pub + "Field", secret + "T"},
		{pub + "Iface", secret + "T"},
		{pub + "TypeArg", secret + "T"},
		{pub + "Constrained", secret + "C"},
		{pub + "Gen", secret + "C"},
		{pub + "Alias", secret + "T"},
		{pub + "Var", secret + "T"},
		{pub + "Const", secret + "Kind"},
		{pub + "Hidden", secret + "T"},
		{"example.com/fixture/pub/nested.Nested", "example.com/fixture/pub/nested/internal/deep.D"},
	}
	for _, w := range want {
		if !hasViolation(violations, w[0]+" ", w[1]+" ") {
			t.Errorf("no violation names %s reaching %s", w[0], w[1])
		}
	}
	for _, v := range violations {
		for _, allowed := range []string{"/clean.", "TestOnly", "cmd/tool", "internal/other", pub + "Self "} {
			if strings.Contains(v, allowed) {
				t.Errorf("violation reported where the rule does not apply: %s", v)
			}
		}
	}
	if len(violations) != len(want) {
		t.Errorf("want exactly %d violations, got %d:\n  %s", len(want), len(violations), strings.Join(violations, "\n  "))
	}
}

func hasViolation(violations []string, fragments ...string) bool {
	for _, v := range violations {
		all := true
		for _, f := range fragments {
			all = all && strings.Contains(v+" ", f)
		}
		if all {
			return true
		}
	}
	return false
}

// publicSignatureViolations loads the non-test packages of the module at root and reports, for each
// exported identifier of each public package, every internal type it reaches. A public package is
// a non-main package of the module with no "internal" element in its import path; an internal
// type is one declared in a module package with such an element, which is exactly what Go forbids
// a caller outside the module to import. It also returns the number of public packages checked.
func publicSignatureViolations(t *testing.T, root string) ([]string, int) {
	t.Helper()
	modulePath := modulePathOf(t, root)
	loaded, err := packages.Load(&packages.Config{
		Dir:        root,
		BuildFlags: []string{"-tags=integration"},
		Mode:       packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes,
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

	inModule := func(path string) bool { return path == modulePath || strings.HasPrefix(path, modulePath+"/") }
	var violations []string
	public := 0
	for _, pkg := range loaded {
		if pkg.Types == nil || !inModule(pkg.PkgPath) || pkg.Name == "main" || hasInternalElement(pkg.PkgPath) {
			continue
		}
		public++
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if !obj.Exported() {
				continue
			}
			w := signatureWalker{inModule: inModule, seen: map[types.Type]bool{}, hits: map[string]string{}}
			w.walk(obj.Type(), name)
			for internalType, trail := range w.hits {
				violations = append(violations,
					fmt.Sprintf("%s.%s reaches internal type %s via %s", pkg.PkgPath, name, internalType, trail))
			}
		}
	}
	sort.Strings(violations)
	return violations, public
}

func hasInternalElement(path string) bool {
	for _, element := range strings.Split(path, "/") {
		if element == "internal" {
			return true
		}
	}
	return false
}

// signatureWalker follows what a caller outside the module reaches from one exported identifier.
// Named types are followed whether exported or not, because an exported function may return an
// unexported type whose exported methods and fields are still callable. Bodies, unexported fields
// and unexported methods are not followed: an outside caller cannot reach them.
type signatureWalker struct {
	inModule func(string) bool
	seen     map[types.Type]bool
	hits     map[string]string // internal type -> first trail that reached it
}

func (w *signatureWalker) hit(obj types.Object, trail string) {
	key := obj.Pkg().Path() + "." + obj.Name()
	if _, ok := w.hits[key]; !ok {
		w.hits[key] = trail
	}
}

func (w *signatureWalker) internal(obj types.Object) bool {
	return obj.Pkg() != nil && w.inModule(obj.Pkg().Path()) && hasInternalElement(obj.Pkg().Path())
}

func (w *signatureWalker) typeParams(list *types.TypeParamList, trail string) {
	for i := 0; i < list.Len(); i++ {
		w.walk(list.At(i).Constraint(), trail+"[constraint "+list.At(i).Obj().Name()+"]")
	}
}

func (w *signatureWalker) typeArgs(list *types.TypeList, trail string) {
	for i := 0; i < list.Len(); i++ {
		w.walk(list.At(i), trail+"[type argument]")
	}
}

func (w *signatureWalker) walk(typ types.Type, trail string) {
	switch t := typ.(type) {
	case nil:
	case *types.Alias:
		if w.internal(t.Obj()) {
			w.hit(t.Obj(), trail)
			return
		}
		w.typeArgs(t.TypeArgs(), trail)
		if w.seen[t] {
			return
		}
		w.seen[t] = true
		w.typeParams(t.TypeParams(), trail)
		w.walk(t.Rhs(), trail+" = "+t.Obj().Name())
	case *types.Named:
		w.typeArgs(t.TypeArgs(), trail)
		obj := t.Obj()
		if w.internal(obj) {
			w.hit(obj, trail)
			return
		}
		// Outside the module (or predeclared, like error): its authors answer for its surface.
		if obj.Pkg() == nil || !w.inModule(obj.Pkg().Path()) {
			return
		}
		origin := t.Origin()
		if w.seen[origin] {
			return
		}
		w.seen[origin] = true
		next := trail + " -> " + obj.Name()
		w.typeParams(origin.TypeParams(), next)
		w.walk(origin.Underlying(), next)
		for i := 0; i < origin.NumMethods(); i++ {
			if m := origin.Method(i); m.Exported() {
				w.walk(m.Type(), next+"."+m.Name())
			}
		}
	case *types.Pointer:
		w.walk(t.Elem(), trail)
	case *types.Slice:
		w.walk(t.Elem(), trail)
	case *types.Array:
		w.walk(t.Elem(), trail)
	case *types.Chan:
		w.walk(t.Elem(), trail)
	case *types.Map:
		w.walk(t.Key(), trail)
		w.walk(t.Elem(), trail)
	case *types.Signature:
		w.typeParams(t.TypeParams(), trail)
		for i := 0; i < t.Params().Len(); i++ {
			w.walk(t.Params().At(i).Type(), trail+"(param)")
		}
		for i := 0; i < t.Results().Len(); i++ {
			w.walk(t.Results().At(i).Type(), trail+"(result)")
		}
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if f := t.Field(i); f.Exported() || f.Embedded() {
				w.walk(f.Type(), trail+"."+f.Name())
			}
		}
	case *types.Interface:
		for i := 0; i < t.NumEmbeddeds(); i++ {
			w.walk(t.EmbeddedType(i), trail+"[embedded]")
		}
		for i := 0; i < t.NumMethods(); i++ {
			if m := t.Method(i); m.Exported() {
				w.walk(m.Type(), trail+"."+m.Name())
			}
		}
	case *types.Union:
		for i := 0; i < t.Len(); i++ {
			w.walk(t.Term(i).Type(), trail+"|")
		}
	case *types.TypeParam:
		if w.seen[t] {
			return
		}
		w.seen[t] = true
		w.walk(t.Constraint(), trail)
	case *types.Basic, *types.Tuple:
	default:
		panic(fmt.Sprintf("signatureWalker: unhandled type %T", typ))
	}
}
