package contract

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// consumerPolicyRule is the requirement every allowlist violation names.
const consumerPolicyRule = "(harness-boundaries › Consumer creation goes through the policy-applying entry points)"

// policyEntryPoints are natsclient's policy-applying consumer entry points, and the port reader
// (component.GetConsumerConfig) whose result feeds them.
var policyEntryPoints = map[string]bool{
	"ConsumeInternalStreamWithConfig": true,
	"ConsumeStreamWithConfig":         true,
	"ConsumeStreamWithConfigContexts": true,
	"GetConsumerConfig":               true,
}

// directConsumerCreations are the jetstream calls that create a consumer without the policy.
var directConsumerCreations = map[string]bool{
	"CreateOrUpdateConsumer": true,
	"CreateConsumer":         true,
	"OrderedConsumer":        true,
}

// policyEntryPointAllowlist names the production files that may reach a policy entry point:
// graph-ingest's JetStream input ports, its one port consumer and its one GetConsumerConfig reader,
// and the objectstore Component's JetStream inputs, bound the same way. A new caller is a
// deliberate edit here, reviewed as such; how often a listed file calls is not checked (#422).
var policyEntryPointAllowlist = map[string]bool{
	"processor/graph-ingest/consume.go": true,
	"storage/objectstore/component.go":  true,
}

// directConsumerCreationAllowlist names the production files that may create a consumer
// directly: natsclient's own policy-checked creations behind the entry points.
var directConsumerCreationAllowlist = map[string]bool{
	"natsclient/stream.go": true,
}

type productionGoFile struct {
	rel  string
	fset *token.FileSet
	file *ast.File
}

// productionGoFiles parses every listed .go file that is not a test file. A file that does not
// parse fails the test, and so does a listing with no production Go file: a guard that read
// nothing has not passed anything.
func productionGoFiles(t *testing.T, root string, files []string) []productionGoFile {
	t.Helper()
	fset := token.NewFileSet()
	var parsed []productionGoFile
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(name)), nil, 0)
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		parsed = append(parsed, productionGoFile{rel: name, fset: fset, file: file})
	}
	if len(parsed) == 0 {
		t.Fatalf("no production Go file among %d listed file(s)", len(files))
	}
	return parsed
}

// natsclientFiles keeps the files under natsclient/, the tree the natsclient surface guards
// read when they lived in that package.
func natsclientFiles(t *testing.T, files []productionGoFile) []productionGoFile {
	t.Helper()
	var kept []productionGoFile
	for _, f := range files {
		if strings.HasPrefix(f.rel, "natsclient/") {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 {
		t.Fatal("no production Go file under natsclient/")
	}
	return kept
}

// TestConsumerPolicyProductionCallsiteCensus holds the rule that only the allowlisted production
// files reach natsclient's policy-applying entry points, and that a file reading a port's
// consumer config never consumes through the internal path.
//
// Requirement: harness-boundaries/Consumer creation goes through the policy-applying entry points
func TestConsumerPolicyProductionCallsiteCensus(t *testing.T) {
	root := repoRoot(t)
	files := productionGoFiles(t, root, repoFiles(t, root))
	requireNoViolations(t, "consumer-policy entry point", consumerPolicyCallsiteViolations(files))
}

// TestConsumerPolicyProductionCallsiteCensusRejectsPlantedCaller plants one caller per entry point
// outside the allowlist and requires the guard to name it; a listed file calling more than once
// passes.
//
// Requirement: harness-boundaries/Consumer creation goes through the policy-applying entry points
func TestConsumerPolicyProductionCallsiteCensusRejectsPlantedCaller(t *testing.T) {
	const listed = "package graphingest\nimport nc \"github.com/c360studio/semengine/natsclient\"\n" +
		"func f(c *nc.Client, p interface{ GetConsumerConfig() any }) {\n" +
		"\t_ = p.GetConsumerConfig()\n\t_ = p.GetConsumerConfig()\n" +
		"\t_, _ = c.ConsumeStreamWithConfig(nil, nil, nc.StreamConsumerConfig{}, nil)\n" +
		"\tconsume := c.ConsumeStreamWithConfig\n\t_ = consume\n}\n"
	root, files := writeTree(t, map[string]string{"processor/graph-ingest/consume.go": listed})
	requireNoViolations(t, "listed file calling twice",
		consumerPolicyCallsiteViolations(productionGoFiles(t, root, files)))

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "internal consumer",
			source: "package planted\nimport nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"func f(c *nc.Client) { _, _ = c.ConsumeInternalStreamWithConfig(nil, nc.StreamConsumerConfig{}, nil) }\n",
			want: "ConsumeInternalStreamWithConfig",
		},
		{
			name: "canonical port consumer",
			source: "package planted\nimport nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"func f(c *nc.Client) { _, _ = c.ConsumeStreamWithConfig(nil, nil, nc.StreamConsumerConfig{}, nil) }\n",
			want: "ConsumeStreamWithConfig",
		},
		{
			name: "split-context port consumer",
			source: "package planted\nimport nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"func f(c *nc.Client) { _, _ = c.ConsumeStreamWithConfigContexts(nil, nil, nil, nc.StreamConsumerConfig{}, nil) }\n",
			want: "ConsumeStreamWithConfigContexts",
		},
		{
			name:   "port consumer config reader",
			source: "package planted\nfunc f(p interface{ GetConsumerConfig() any }) { _ = p.GetConsumerConfig() }\n",
			want:   "GetConsumerConfig",
		},
		{
			name: "method value",
			source: "package planted\nimport nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"func f(c *nc.Client) { consume := c.ConsumeStreamWithConfig; _ = consume }\n",
			want: "ConsumeStreamWithConfig",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, files := writeTree(t, map[string]string{
				"processor/graph-ingest/consume.go": listed,
				"planted/planted.go":                tt.source,
			})
			violations := consumerPolicyCallsiteViolations(productionGoFiles(t, root, files))
			requireViolation(t, violations, "planted/planted.go:", tt.want, consumerPolicyRule)
			t.Logf("planted caller rejected: %s", strings.Join(violations, "\n"))
		})
	}

	t.Run("port-backed file on the internal path", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{
			"processor/graph-ingest/consume.go": "package graphingest\n" +
				"import nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"func f(c *nc.Client, p interface{ GetConsumerConfig() any }) {\n\t_ = p.GetConsumerConfig()\n" +
				"\t_, _ = c.ConsumeInternalStreamWithConfig(nil, nc.StreamConsumerConfig{}, nil)\n}\n",
		})
		requireViolation(t, consumerPolicyCallsiteViolations(productionGoFiles(t, root, files)),
			"processor/graph-ingest/consume.go", "port-backed", consumerPolicyRule)
	})

	// The lists stay apart: the direct-creation file is not on the entry-point list.
	t.Run("entry point in the direct-creation file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{
			"natsclient/stream.go": "package natsclient\n" +
				"func f(c *Client) { _, _ = c.ConsumeStreamWithConfig(nil, nil, StreamConsumerConfig{}, nil) }\n",
		})
		requireViolation(t, consumerPolicyCallsiteViolations(productionGoFiles(t, root, files)),
			"natsclient/stream.go:2: ConsumeStreamWithConfig", consumerPolicyRule)
	})
}

// consumerPolicyCallsiteViolations reports each reference to a policy entry point (a call or a
// method value) in a file outside policyEntryPointAllowlist, and each file that both reads a
// port's consumer config and consumes through ConsumeInternalStreamWithConfig.
func consumerPolicyCallsiteViolations(files []productionGoFile) []string {
	var violations []string
	for _, parsed := range files {
		usesPortConfig := false
		usesInternal := false
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !policyEntryPoints[selector.Sel.Name] {
				return true
			}
			switch selector.Sel.Name {
			case "GetConsumerConfig":
				usesPortConfig = true
			case "ConsumeInternalStreamWithConfig":
				usesInternal = true
			}
			if !policyEntryPointAllowlist[parsed.rel] {
				violations = append(violations, fmt.Sprintf("%s:%d: %s outside the policy entry-point allowlist %s",
					parsed.rel, parsed.fset.Position(selector.Pos()).Line, selector.Sel.Name, consumerPolicyRule))
			}
			return true
		})
		if usesPortConfig && usesInternal {
			violations = append(violations, fmt.Sprintf("%s: port-backed file reads GetConsumerConfig and consumes "+
				"through ConsumeInternalStreamWithConfig; use ConsumeStreamWithConfig %s", parsed.rel, consumerPolicyRule))
		}
	}
	return violations
}

// TestConsumerPolicyDirectCreationCallCensus holds the rule that only the allowlisted production
// files create a JetStream consumer directly, outside the policy entry points.
//
// Requirement: harness-boundaries/Consumer creation goes through the policy-applying entry points
func TestConsumerPolicyDirectCreationCallCensus(t *testing.T) {
	root := repoRoot(t)
	files := productionGoFiles(t, root, repoFiles(t, root))
	requireNoViolations(t, "direct consumer creation", directConsumerCreationViolations(files))
}

// TestConsumerPolicyDirectCreationCallCensusRejectsPlantedCaller plants one direct creation per
// method outside the allowlist and requires the guard to name it; the listed file passes.
//
// Requirement: harness-boundaries/Consumer creation goes through the policy-applying entry points
func TestConsumerPolicyDirectCreationCallCensusRejectsPlantedCaller(t *testing.T) {
	const listed = "package natsclient\nimport \"github.com/nats-io/nats.go/jetstream\"\n" +
		"func f(s jetstream.Stream) {\n\t_, _ = s.CreateOrUpdateConsumer(nil, jetstream.ConsumerConfig{})\n" +
		"\t_, _ = s.CreateOrUpdateConsumer(nil, jetstream.ConsumerConfig{})\n}\n"
	root, files := writeTree(t, map[string]string{"natsclient/stream.go": listed})
	requireNoViolations(t, "listed file creating twice",
		directConsumerCreationViolations(productionGoFiles(t, root, files)))

	for _, method := range []string{"CreateOrUpdateConsumer", "CreateConsumer", "OrderedConsumer"} {
		t.Run(method, func(t *testing.T) {
			source := "package planted\nimport \"github.com/nats-io/nats.go/jetstream\"\n" +
				"func f(s jetstream.Stream) { _, _ = s." + method + "(nil, jetstream.ConsumerConfig{}) }\n"
			if method == "OrderedConsumer" {
				source = "package planted\nimport \"github.com/nats-io/nats.go/jetstream\"\n" +
					"func f(s jetstream.Stream) { _, _ = s.OrderedConsumer(nil, jetstream.OrderedConsumerConfig{}) }\n"
			}
			root, files := writeTree(t, map[string]string{
				"natsclient/stream.go": listed,
				"planted/planted.go":   source,
			})
			violations := directConsumerCreationViolations(productionGoFiles(t, root, files))
			requireViolation(t, violations, "planted/planted.go:3: "+method, consumerPolicyRule)
			t.Logf("planted caller rejected: %s", strings.Join(violations, "\n"))
		})
	}

	// The lists stay apart: the entry-point file is not on the direct-creation list.
	t.Run("direct creation in the entry-point file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{
			"processor/graph-ingest/consume.go": "package graphingest\nimport \"github.com/nats-io/nats.go/jetstream\"\n" +
				"func f(s jetstream.Stream) { _, _ = s.CreateOrUpdateConsumer(nil, jetstream.ConsumerConfig{}) }\n",
		})
		requireViolation(t, directConsumerCreationViolations(productionGoFiles(t, root, files)),
			"processor/graph-ingest/consume.go:3: CreateOrUpdateConsumer", consumerPolicyRule)
	})
}

// directConsumerCreationViolations reports each reference to a direct consumer creation (a call
// or a method value) in a file outside directConsumerCreationAllowlist.
func directConsumerCreationViolations(files []productionGoFile) []string {
	var violations []string
	for _, parsed := range files {
		if directConsumerCreationAllowlist[parsed.rel] {
			continue
		}
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && directConsumerCreations[selector.Sel.Name] {
				violations = append(violations, fmt.Sprintf("%s:%d: %s creates a consumer outside the direct-creation allowlist %s",
					parsed.rel, parsed.fset.Position(selector.Pos()).Line, selector.Sel.Name, consumerPolicyRule))
			}
			return true
		})
	}
	return violations
}

type legacyHeartbeatReferenceScan struct {
	directCalls map[string]int
	violations  []string
}

func newDurableHandlerRetirementViolations(files []productionGoFile) []string {
	var violations []string
	for _, parsed := range files {
		declarationNames := map[*ast.Ident]struct{}{}
		if parsed.file.Name.Name == "natsclient" {
			for _, declaration := range parsed.file.Decls {
				switch declaration := declaration.(type) {
				case *ast.FuncDecl:
					if declaration.Name.Name == "NewDurableHandler" {
						declarationNames[declaration.Name] = struct{}{}
						violations = append(violations, parsed.rel+": function or receiver method")
					}
				case *ast.GenDecl:
					for _, spec := range declaration.Specs {
						switch spec := spec.(type) {
						case *ast.ValueSpec:
							for _, name := range spec.Names {
								if name.Name == "NewDurableHandler" {
									declarationNames[name] = struct{}{}
									violations = append(violations, parsed.rel+": variable or constant alias")
								}
							}
						case *ast.TypeSpec:
							if spec.Name.Name == "NewDurableHandler" {
								declarationNames[spec.Name] = struct{}{}
								violations = append(violations, parsed.rel+": type alias")
							}
						}
					}
				}
			}
		}

		aliases, dotImport := natsclientImportBindings(parsed.file)
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			switch reference := node.(type) {
			case *ast.SelectorExpr:
				if reference.Sel.Name != "NewDurableHandler" {
					return true
				}
				if qualifier, qualified := reference.X.(*ast.Ident); qualified {
					if _, imported := aliases[qualifier.Name]; imported {
						violations = append(violations, parsed.rel+": qualified call or symbol reference")
					}
				}
				return false
			case *ast.Ident:
				if reference.Name != "NewDurableHandler" ||
					(parsed.file.Name.Name != "natsclient" && !dotImport) {
					return true
				}
				if _, declaration := declarationNames[reference]; !declaration {
					violations = append(violations, parsed.rel+": identifier call or symbol reference")
				}
			}
			return true
		})
	}
	return violations
}

func scanLegacyHeartbeatReferences(files []productionGoFile) legacyHeartbeatReferenceScan {
	result := legacyHeartbeatReferenceScan{directCalls: map[string]int{}}
	for _, parsed := range files {
		declarationNames := map[*ast.Ident]struct{}{}
		if parsed.file.Name.Name == "natsclient" {
			for _, declaration := range parsed.file.Decls {
				switch declaration := declaration.(type) {
				case *ast.FuncDecl:
					if declaration.Name.Name != "ConsumeWithHeartbeat" {
						continue
					}
					// Any declaration is a violation now. Until #1249 the
					// scan exempted natsclient/heartbeat.go, because the
					// symbol still had to exist for its last caller; that
					// exemption is what would let the helper come back.
					declarationNames[declaration.Name] = struct{}{}
					result.violations = append(result.violations,
						parsed.rel+": function or receiver method")
				case *ast.GenDecl:
					for _, spec := range declaration.Specs {
						switch spec := spec.(type) {
						case *ast.ValueSpec:
							for _, name := range spec.Names {
								if name.Name == "ConsumeWithHeartbeat" {
									declarationNames[name] = struct{}{}
									result.violations = append(result.violations,
										parsed.rel+": variable or constant alias")
								}
							}
						case *ast.TypeSpec:
							if spec.Name.Name == "ConsumeWithHeartbeat" {
								declarationNames[spec.Name] = struct{}{}
								result.violations = append(result.violations, parsed.rel+": type alias")
							}
						}
					}
				}
			}
		}

		aliases, dotImport := natsclientImportBindings(parsed.file)
		relevantSelector := func(selector *ast.SelectorExpr) bool {
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok || selector.Sel.Name != "ConsumeWithHeartbeat" {
				return false
			}
			_, ok = aliases[qualifier.Name]
			return ok
		}
		relevantIdent := func(identifier *ast.Ident) bool {
			return identifier.Name == "ConsumeWithHeartbeat" &&
				(parsed.file.Name.Name == "natsclient" || dotImport)
		}

		directReferences := map[ast.Node]struct{}{}
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch called := call.Fun.(type) {
			case *ast.SelectorExpr:
				if relevantSelector(called) {
					directReferences[called] = struct{}{}
				}
			case *ast.Ident:
				if relevantIdent(called) {
					directReferences[called] = struct{}{}
				}
			}
			return true
		})

		ast.Inspect(parsed.file, func(node ast.Node) bool {
			switch reference := node.(type) {
			case *ast.SelectorExpr:
				if reference.Sel.Name != "ConsumeWithHeartbeat" {
					return true
				}
				if relevantSelector(reference) {
					if _, direct := directReferences[reference]; direct {
						result.directCalls[parsed.rel]++
					} else {
						result.violations = append(result.violations, parsed.rel+": indirect package reference")
					}
				}
				return false
			case *ast.Ident:
				if !relevantIdent(reference) {
					return true
				}
				if _, declaration := declarationNames[reference]; declaration {
					return true
				}
				if _, direct := directReferences[reference]; direct {
					result.directCalls[parsed.rel]++
				} else {
					result.violations = append(result.violations, parsed.rel+": indirect identifier reference")
				}
			}
			return true
		})
	}
	return result
}

func natsclientImportBindings(file *ast.File) (map[string]struct{}, bool) {
	aliases := map[string]struct{}{}
	dotImport := false
	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil || importPath != "github.com/c360studio/semengine/natsclient" {
			continue
		}
		if imported.Name == nil {
			aliases["natsclient"] = struct{}{}
			continue
		}
		switch imported.Name.Name {
		case ".":
			dotImport = true
		case "_":
		default:
			aliases[imported.Name.Name] = struct{}{}
		}
	}
	return aliases, dotImport
}

// TestConsumerPolicyExportedClientAPICensus pins the signatures of natsclient's exported Client
// consumer methods.
func TestConsumerPolicyExportedClientAPICensus(t *testing.T) {
	root := repoRoot(t)
	files := natsclientFiles(t, productionGoFiles(t, root, repoFiles(t, root)))
	requireNoViolations(t, "exported Client consumer API", exportedClientConsumerAPIViolations(t, files))
}

func exportedClientConsumerAPIViolations(t *testing.T, files []productionGoFile) []string {
	t.Helper()
	got := map[string]string{}
	for _, parsed := range files {
		for _, declaration := range parsed.file.Decls {
			method, ok := declaration.(*ast.FuncDecl)
			if !ok || method.Recv == nil || !method.Name.IsExported() || !receiverIsClientPointer(method.Recv) {
				continue
			}
			if !strings.HasPrefix(method.Name.Name, "Consume") && method.Name.Name != "ObserveDirectPortConsumerPolicy" {
				continue
			}
			got[method.Name.Name] = compactNode(t, method.Type)
		}
	}

	want := map[string]string{
		"ConsumeInternalStreamWithConfig": "func(ctx context.Context, cfg StreamConsumerConfig, handler func(ctx context.Context, msg jetstream.Msg)) (jetstream.ConsumeContext, error)",
		"ConsumeStreamWithConfig":         "func(ctx context.Context, owner PortConsumerContext, cfg StreamConsumerConfig, handler func(ctx context.Context, msg jetstream.Msg)) (jetstream.ConsumeContext, error)",
		"ConsumeStreamWithConfigContexts": "func(setupCtx context.Context, handlerCtx context.Context, owner PortConsumerContext, cfg StreamConsumerConfig, handler func(ctx context.Context, msg jetstream.Msg)) (jetstream.ConsumeContext, error)",
		"ObserveDirectPortConsumerPolicy": "func(ctx context.Context, owner PortConsumerContext, finalConfig jetstream.ConsumerConfig, consumer jetstream.Consumer) (func(), error)",
	}
	if !reflect.DeepEqual(got, want) {
		return []string{fmt.Sprintf("exported Client consumer API census = %#v, want %#v", got, want)}
	}
	return nil
}

func TestNewDurableHandlerHasNoDeclarationOrProductionCalls(t *testing.T) {
	root := repoRoot(t)
	files := productionGoFiles(t, root, repoFiles(t, root))
	if violations := newDurableHandlerRetirementViolations(files); len(violations) != 0 {
		t.Fatalf("retired NewDurableHandler surface remains: %v", violations)
	}
}

func TestNewDurableHandlerRetirementRejectsAliasAndReceiverBypasses(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "exported variable alias",
			source: "package natsclient\nvar NewDurableHandler = func() {}\n",
		},
		{
			name: "receiver method",
			source: "package natsclient\n" +
				"type Client struct{}\n" +
				"func (*Client) NewDurableHandler() {}\n",
		},
		{
			name: "external qualified call",
			source: "package fixture\n" +
				"import nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"func callRetired() { nc.NewDurableHandler() }\n",
		},
		{
			name: "external default-import symbol",
			source: "package fixture\n" +
				"import \"github.com/c360studio/semengine/natsclient\"\n" +
				"var retired = natsclient.NewDurableHandler\n",
		},
		{
			name: "external symbol taking",
			source: "package fixture\n" +
				"import nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"var retired = nc.NewDurableHandler\n",
		},
		{
			name: "external dot-import call",
			source: "package fixture\n" +
				"import . \"github.com/c360studio/semengine/natsclient\"\n" +
				"func callRetired() { NewDurableHandler() }\n",
		},
		{
			name:   "type alias",
			source: "package natsclient\ntype NewDurableHandler = func()\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, files := writeTree(t, map[string]string{"bypass.go": tt.source})
			if violations := newDurableHandlerRetirementViolations(productionGoFiles(t, root, files)); len(violations) == 0 {
				t.Fatal("retirement guard accepted bypass")
			}
		})
	}
}

func TestNewDurableHandlerRetirementIgnoresUnrelatedSelector(t *testing.T) {
	source := "package fixture\n" +
		"import other \"example.com/other\"\n" +
		"func callOther() { other.NewDurableHandler() }\n"
	root, files := writeTree(t, map[string]string{"unrelated.go": source})
	if violations := newDurableHandlerRetirementViolations(productionGoFiles(t, root, files)); len(violations) != 0 {
		t.Fatalf("unrelated selector classified as retired builder: %v", violations)
	}
}

// TestConsumeWithHeartbeatHasNoDeclarationOrProductionCalls is the inverted
// ratchet (#1249/#759). Until this layer the guard pinned the EXACT remaining
// caller set and asserted that the declaration still existed, because the
// symbol had to survive for its last caller; AgentRun migrated to the typed
// path and the helper was deleted without alias, so the guard now asserts
// absence everywhere — no declaration, no alias, no reference, in any package.
//
// It is the same shape as the NewDurableHandler retirement above, and for the
// same reason: a retired helper comes back as a convenience wrapper, an
// exported variable, or a receiver method long before anyone re-adds the
// original function, and each of those is caught here rather than in review.
func TestConsumeWithHeartbeatHasNoDeclarationOrProductionCalls(t *testing.T) {
	root := repoRoot(t)
	scan := scanLegacyHeartbeatReferences(productionGoFiles(t, root, repoFiles(t, root)))
	if len(scan.violations) != 0 {
		t.Fatalf("retired ConsumeWithHeartbeat surface remains: %v", scan.violations)
	}
	if len(scan.directCalls) != 0 {
		t.Fatalf("ConsumeWithHeartbeat callers = %#v, want none: the helper no longer exists", scan.directCalls)
	}
}

func TestLegacyHeartbeatGuardRejectsTakingOrAliasingSymbol(t *testing.T) {
	source := "package fixture\n" +
		"import nc \"github.com/c360studio/semengine/natsclient\"\n" +
		"var ExportedLegacyHeartbeat = nc.ConsumeWithHeartbeat\n" +
		"func callIndirect() { legacy := nc.ConsumeWithHeartbeat; _ = legacy(nil, nil, 0, nil) }\n"
	root, files := writeTree(t, map[string]string{"indirect.go": source})
	scan := scanLegacyHeartbeatReferences(productionGoFiles(t, root, files))
	if len(scan.violations) != 2 {
		t.Fatalf("legacy indirect-reference violations = %v, want two", scan.violations)
	}
	if len(scan.directCalls) != 0 {
		t.Fatalf("legacy direct calls = %#v, want none", scan.directCalls)
	}
}

func TestLegacyHeartbeatGuardRejectsAlternateExportedSurface(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "function declaration",
			source: "package natsclient\nfunc ConsumeWithHeartbeat() {}\n",
		},
		{
			name:   "variable alias",
			source: "package natsclient\nvar ConsumeWithHeartbeat = func() {}\n",
		},
		{
			name: "receiver method",
			source: "package natsclient\n" +
				"type Client struct{}\n" +
				"func (*Client) ConsumeWithHeartbeat() {}\n",
		},
		{
			name:   "type alias",
			source: "package natsclient\ntype ConsumeWithHeartbeat = func()\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, files := writeTree(t, map[string]string{"alternate.go": tt.source})
			scan := scanLegacyHeartbeatReferences(productionGoFiles(t, root, files))
			if len(scan.violations) == 0 {
				t.Fatal("legacy surface guard accepted alternate export")
			}
		})
	}
}

func TestLegacyHeartbeatGuardCountsDotImportAsDirectCall(t *testing.T) {
	source := "package fixture\n" +
		"import . \"github.com/c360studio/semengine/natsclient\"\n" +
		"func callLegacy() { _ = ConsumeWithHeartbeat(nil, nil, 0, nil) }\n"
	root, files := writeTree(t, map[string]string{"dot.go": source})
	scan := scanLegacyHeartbeatReferences(productionGoFiles(t, root, files))
	if len(scan.violations) != 0 {
		t.Fatalf("dot-import direct call reported as indirect: %v", scan.violations)
	}
	want := map[string]int{"dot.go": 1}
	if !reflect.DeepEqual(scan.directCalls, want) {
		t.Fatalf("dot-import direct calls = %#v, want %#v", scan.directCalls, want)
	}
}

func TestLegacyHeartbeatGuardIgnoresUnrelatedSelector(t *testing.T) {
	source := "package fixture\n" +
		"import other \"example.com/other\"\n" +
		"func callOther() { other.ConsumeWithHeartbeat() }\n"
	root, files := writeTree(t, map[string]string{"unrelated.go": source})
	scan := scanLegacyHeartbeatReferences(productionGoFiles(t, root, files))
	if len(scan.violations) != 0 || len(scan.directCalls) != 0 {
		t.Fatalf("unrelated selector classified as legacy: violations=%v direct=%#v", scan.violations, scan.directCalls)
	}
}

func TestNoDeliveryDispositionExportedSurface(t *testing.T) {
	root := repoRoot(t)
	files := natsclientFiles(t, productionGoFiles(t, root, repoFiles(t, root)))
	requireNoViolations(t, "delivery disposition surface", deliveryDispositionViolations(files))
}

func deliveryDispositionViolations(files []productionGoFile) []string {
	var violations []string
	for _, parsed := range files {
		for _, declaration := range parsed.file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if strings.Contains(declaration.Name.Name, "DeliveryDisposition") {
					violations = append(violations, fmt.Sprintf("%s: forbidden disposition constructor/function exported: %s",
						parsed.rel, declaration.Name.Name))
				}
			case *ast.GenDecl:
				for _, spec := range declaration.Specs {
					if typed, ok := spec.(*ast.TypeSpec); ok && strings.Contains(typed.Name.Name, "DeliveryDisposition") {
						violations = append(violations, fmt.Sprintf("%s: forbidden disposition type exported: %s",
							parsed.rel, typed.Name.Name))
					}
				}
			}
		}
	}
	return violations
}

func TestClientHasNoChildLifecycleSurfaceOrCatalog(t *testing.T) {
	root := repoRoot(t)
	files := natsclientFiles(t, productionGoFiles(t, root, repoFiles(t, root)))
	requireNoViolations(t, "Client child lifecycle surface", childLifecycleSurfaceViolations(files))
}

func childLifecycleSurfaceViolations(files []productionGoFile) []string {
	forbiddenMethods := map[string]struct{}{
		"ConsumeDurable": {}, "StopConsumer": {}, "StopAndDeleteConsumer": {},
		"StopAllConsumers": {}, "OutstandingWork": {},
	}
	forbiddenFields := map[string]struct{}{
		"consumers": {}, "consumersMu": {}, "subs": {},
	}
	var violations []string
	for _, parsed := range files {
		for _, declaration := range parsed.file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if ok && fn.Recv != nil && receiverIsClientPointer(fn.Recv) {
				if _, forbidden := forbiddenMethods[fn.Name.Name]; forbidden {
					violations = append(violations, fmt.Sprintf("%s: forbidden Client lifecycle method remains: %s",
						parsed.rel, fn.Name.Name))
				}
			}
			gen, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != "Client" {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range structType.Fields.List {
					for _, name := range field.Names {
						if _, forbidden := forbiddenFields[name.Name]; forbidden {
							violations = append(violations, fmt.Sprintf("%s: forbidden Client child catalog remains: %s",
								parsed.rel, name.Name))
						}
					}
				}
			}
		}
	}
	return violations
}

// TestNatsclientSurfaceGuardsRejectPlantedSurface plants, under natsclient/, each surface the three
// natsclient surface guards forbid, and requires each guard to name it.
func TestNatsclientSurfaceGuardsRejectPlantedSurface(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"natsclient/client.go": "package natsclient\n" +
			"type Client struct{ consumers map[string]int }\n" +
			"func (*Client) ConsumeAnything() {}\n" +
			"func (*Client) StopConsumer() {}\n" +
			"type DeliveryDispositionKind int\n" +
			"func NewDeliveryDisposition() {}\n",
	})
	parsed := natsclientFiles(t, productionGoFiles(t, root, files))
	requireViolation(t, exportedClientConsumerAPIViolations(t, parsed), "ConsumeAnything")
	requireViolation(t, childLifecycleSurfaceViolations(parsed), "natsclient/client.go", "StopConsumer")
	requireViolation(t, childLifecycleSurfaceViolations(parsed), "natsclient/client.go", "consumers")
	requireViolation(t, deliveryDispositionViolations(parsed), "natsclient/client.go", "DeliveryDispositionKind")
	requireViolation(t, deliveryDispositionViolations(parsed), "natsclient/client.go", "NewDeliveryDisposition")
}

func receiverIsClientPointer(receivers *ast.FieldList) bool {
	if receivers == nil || len(receivers.List) != 1 {
		return false
	}
	pointer, ok := receivers.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	name, ok := pointer.X.(*ast.Ident)
	return ok && name.Name == "Client"
}

func compactNode(t *testing.T, node ast.Node) string {
	t.Helper()
	var output bytes.Buffer
	if err := format.Node(&output, token.NewFileSet(), node); err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(output.String()), " ")
}
