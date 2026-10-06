package contract

import (
	"fmt"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
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
	loaded := loadModuleTypes(t, root)

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

// loadModuleTypes loads and type-checks the non-test packages of the module at root, integration
// files included, and fails the test on any load or type error.
func loadModuleTypes(t *testing.T, root string) []*packages.Package {
	t.Helper()
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
	return loaded
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

// The deployment-authority rule (#72 ruling A, comment 5969505488; check C1 of that ruling, placed
// here as the failing-first proof for PR #48's surface audit): a deployment's org and platform
// become an identity's first two positions only through the entity-ID family, so no production
// package exports a second spelling of that authority. FederationMeta and its family, GlobalID and
// vocabulary.EntityIRI were such spellings at the pin; an exported name containing one of these
// words, in any non-test package of the module, internal and main packages included, fails.
var authorityNames = regexp.MustCompile(`Federation|GlobalID|EntityIRI`)

func TestNoDeploymentAuthorityNames(t *testing.T) {
	violations, checked := authorityNameViolations(t, repoRoot(t))
	requireNoViolations(t, "deployment-authority name", violations)
	t.Logf("checked %d packages", checked)
}

// authorityFixture plants a matching exported name at each place an exported name can be declared,
// in a public, an internal and a main package, and the same words where the rule does not apply.
var authorityFixture = map[string]string{
	"go.mod": "module example.com/fixture\n\ngo 1.26\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ./dep\n",
	// Aliases (Codex F11, PR #48 comment 5970321028): an alias's own name, and the members of the
	// type it stands for, are checked; members of a type this module declares are reported once, at
	// that declaration.
	"pub/alias.go": `package pub

import "example.com/dep"

// Codex's control: a defined struct, reported as before.
type Direct struct{ FederationOrigin string }

type Metadata = struct{ FederationOrigin string }

type AliasMeta = interface{ GlobalID() string }

type PtrMeta = *struct{ EntityIRI string }

// The alias name is checked; Message's members were reported at Message.
type FederationAlias = Message

type MessageAlias = *Message

// A type declared outside the module: its members are reported at the alias.
type Record = dep.Record

// No forbidden name: nothing is reported.
type Plain = struct{ Name string }

// An interface that embeds one declared outside the module: GlobalID is reported at the alias.
type Exposed = interface{ dep.Identity }

// A module-owned embedded interface: GlobalID is reported once, at Inner.
type Inner interface{ GlobalID() string }

type Outer interface{ Inner }

type OuterAlias = interface{ Inner }

type Wraps interface{ dep.Identity }
`,
	"dep/go.mod": "module example.com/dep\n\ngo 1.26\n",
	"dep/dep.go": "package dep\n\ntype Record struct{ GlobalID string }\n\nfunc (Record) FederationOrigin() string { return \"\" }\n\n" +
		"type Identity interface{ GlobalID() string }\n",
	"pub/pub.go": `package pub

type FederationMeta interface{ Platform() string }

func BuildGlobalID() string { return "" }

var DefaultFederation = 1

const EntityIRIBase = "x"

type Message struct {
	FederationOrigin string
	clean            string
}

func (Message) WithFederation() Message { return Message{} }

type hidden struct{}

func (hidden) GlobalID() string { return "" }

type Meta interface{ EntityIRI() string }

// Not exported, or not the word: none is reported.
type federationMeta struct{}

func entityIRI() string { return "" }

type Federated struct{ globalID string }

func (Federated) entityIRI() string { return "" }
`,
	"pub/pub_test.go": `package pub

func FederationTestHelper() {}
`,
	// The match is by declared name and case-sensitive: an exported name with the word in lower
	// case only, a comment and a string literal are not reported (PR #73, task 3.1).
	"pub/words.go": `package pub

// BuildGlobalID, FederationMeta and EntityIRI in a comment are not names.
const Label = "BuildGlobalID FederationMeta EntityIRI"

func Confederation() string { return Label }
`,
	"internal/inner/inner.go": "package inner\n\nfunc NewFederationMeta() {}\n",
	"cmd/tool/main.go":        "package main\n\nfunc EntityIRI() {}\n\nfunc main() {}\n",
}

func TestNoDeploymentAuthorityNamesSensitivity(t *testing.T) {
	root, _ := writeTree(t, authorityFixture)
	violations, checked := authorityNameViolations(t, root)
	// pub, internal/inner and cmd/tool; dep is a second module.
	if checked != 3 {
		t.Errorf("want 3 fixture packages checked, got %d", checked)
	}
	empty, _ := writeTree(t, map[string]string{"go.mod": "module example.com/empty\n\ngo 1.26\n"})
	got, n := authorityNameViolations(t, empty)
	if n != 0 || len(got) != 1 || got[0] != "checked no package of example.com/empty" {
		t.Errorf("a module with no package: want one \"checked no package\" violation, got %d packages and %q", n, got)
	}
	t.Logf("violations:\n  %s", strings.Join(violations, "\n  "))
	// The suffix is written from the spec heading, not taken from authoritySuffix, so a wrong
	// constant fails here.
	at := func(position, qualified string) string {
		return position + ": example.com/fixture/" + qualified + " spells the deployment authority outside the " +
			"entity-ID family (harness-boundaries › No second spelling of deployment authority)"
	}
	want := []string{
		at("cmd/tool/main.go:3", "cmd/tool.EntityIRI"),
		at("internal/inner/inner.go:3", "internal/inner.NewFederationMeta"),
		at("pub/pub.go:3", "pub.FederationMeta"),
		at("pub/pub.go:5", "pub.BuildGlobalID"),
		at("pub/pub.go:7", "pub.DefaultFederation"),
		at("pub/pub.go:9", "pub.EntityIRIBase"),
		at("pub/pub.go:12", "pub.Message.FederationOrigin"),
		at("pub/pub.go:16", "pub.Message.WithFederation"),
		at("pub/pub.go:20", "pub.hidden.GlobalID"),
		at("pub/pub.go:22", "pub.Meta.EntityIRI"),
		at("pub/alias.go:6", "pub.Direct.FederationOrigin"),
		at("pub/alias.go:8", "pub.Metadata.FederationOrigin"),
		at("pub/alias.go:10", "pub.AliasMeta.GlobalID"),
		at("pub/alias.go:12", "pub.PtrMeta.EntityIRI"),
		at("pub/alias.go:15", "pub.FederationAlias"),
		// dep.Record's members are declared outside the module, so the alias's line is reported.
		at("pub/alias.go:20", "pub.Record.FederationOrigin"),
		at("pub/alias.go:20", "pub.Record.GlobalID"),
		// Inherited interface methods (Codex F3, PR #73 comment 6018394100): one declared outside the
		// module is reported at the embedder; one the module declares, once, at its declaration.
		at("pub/alias.go:26", "pub.Exposed.GlobalID"),
		at("pub/alias.go:29", "pub.Inner.GlobalID"),
		at("pub/alias.go:35", "pub.Wraps.GlobalID"),
	}
	sort.Strings(want)
	if strings.Join(violations, "\n") != strings.Join(want, "\n") {
		t.Errorf("violations:\n  %s\nwant exactly:\n  %s", strings.Join(violations, "\n  "), strings.Join(want, "\n  "))
	}
}

const authoritySuffix = " spells the deployment authority outside the entity-ID family " +
	"(harness-boundaries › No second spelling of deployment authority)"

// authorityNameViolations reports every exported name matching authorityNames in the non-test
// packages of the module at root: package-level identifiers, and the exported methods, struct
// fields and interface methods (embedded ones included) of package-level types, exported or not
// (an exported method of an unexported type is still callable through an exported function that
// returns it). An alias is a
// package-level type too: its own name is checked, and so are the members of the type it stands
// for, unless that type is one this module declares, whose members are reported once, at its own
// declaration. Each line names the file, line, qualified identifier and rule; the count is the
// number of module packages checked.
func authorityNameViolations(t *testing.T, root string) ([]string, int) {
	t.Helper()
	modulePath := modulePathOf(t, root)
	inModule := func(pkg *types.Package) bool {
		return pkg != nil && (pkg.Path() == modulePath || strings.HasPrefix(pkg.Path(), modulePath+"/"))
	}
	var violations []string
	checked := 0
	for _, pkg := range loadModuleTypes(t, root) {
		if pkg.Types == nil || !inModule(pkg.Types) {
			continue
		}
		checked++
		// report names obj, a candidate, at pos: obj's own position when the module declares it,
		// else the position of the alias that exposes it.
		report := func(pos token.Pos, qualified string, obj types.Object) {
			if !obj.Exported() || !authorityNames.MatchString(obj.Name()) {
				return
			}
			if inModule(obj.Pkg()) {
				pos = obj.Pos()
			}
			position := pkg.Fset.Position(pos)
			path, err := filepath.Rel(root, position.Filename)
			if err != nil {
				path = position.Filename
			}
			violations = append(violations,
				fmt.Sprintf("%s:%d: %s%s", filepath.ToSlash(path), position.Line, qualified, authoritySuffix))
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			qualified := pkg.PkgPath + "." + name
			report(obj.Pos(), qualified, obj)
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			typ := tn.Type()
			if tn.IsAlias() {
				typ = types.Unalias(typ)
				if ptr, isPtr := typ.(*types.Pointer); isPtr {
					typ = types.Unalias(ptr.Elem())
				}
				if named, isNamed := typ.(*types.Named); isNamed && inModule(named.Obj().Pkg()) {
					continue // reported at the declaration of the type the alias stands for
				}
			}
			if named, isNamed := typ.(*types.Named); isNamed {
				for i := 0; i < named.NumMethods(); i++ {
					report(obj.Pos(), qualified+"."+named.Method(i).Name(), named.Method(i))
				}
			}
			switch u := typ.Underlying().(type) {
			case *types.Struct:
				for i := 0; i < u.NumFields(); i++ {
					report(obj.Pos(), qualified+"."+u.Field(i).Name(), u.Field(i))
				}
			case *types.Interface:
				// The whole method set, embedded interfaces included: a method declared in the
				// module is reported at its own declaration, so here only when it is this
				// interface's own; one inherited from outside the module only here.
				explicit := map[*types.Func]bool{}
				for i := 0; i < u.NumExplicitMethods(); i++ {
					explicit[u.ExplicitMethod(i)] = true
				}
				for i := 0; i < u.NumMethods(); i++ {
					if m := u.Method(i); explicit[m] || !inModule(m.Pkg()) {
						report(obj.Pos(), qualified+"."+m.Name(), m)
					}
				}
			}
		}
	}
	// A load that found no package of the module would pass vacuously; it fails instead.
	if checked == 0 {
		violations = append(violations, "checked no package of "+modulePath)
	}
	sort.Strings(violations)
	return violations, checked
}
