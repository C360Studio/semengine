package natsclient

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type productionGoFile struct {
	rel  string
	file *ast.File
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

// TestConsumerPolicyProductionCallsiteCensus pins every production caller of the consumer-policy
// entry points in SemEngine's tree. The expected maps hold what this repository has, measured when
// natsclient was ported (task 3.7), not the pin's SemStreams callers: each later port that adds a
// caller updates them (admission ledger row natsclient, known_risks).
func TestConsumerPolicyProductionCallsiteCensus(t *testing.T) {
	files := parseProductionGoFiles(t, filepath.Clean(".."))
	if violations := consumerPolicyCallsiteCensusViolations(files); len(violations) != 0 {
		t.Fatalf("consumer-policy call-site census:\n%s", strings.Join(violations, "\n"))
	}
}

// TestConsumerPolicyProductionCallsiteCensusRejectsPlantedCaller adds one planted caller per entry
// point, written in a t.TempDir() tree, to the measured tree and requires the census to name it.
func TestConsumerPolicyProductionCallsiteCensusRejectsPlantedCaller(t *testing.T) {
	tree := parseProductionGoFiles(t, filepath.Clean(".."))
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "internal consumer",
			source: "package planted\nimport nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"func f(c *nc.Client) { _, _ = c.ConsumeInternalStreamWithConfig(nil, nc.StreamConsumerConfig{}, nil) }\n",
			want: "internal consumer census",
		},
		{
			name: "canonical port consumer",
			source: "package planted\nimport nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"func f(c *nc.Client) { _, _ = c.ConsumeStreamWithConfig(nil, nil, nc.StreamConsumerConfig{}, nil) }\n",
			want: "canonical port consumer census",
		},
		{
			name: "split-context port consumer",
			source: "package planted\nimport nc \"github.com/c360studio/semengine/natsclient\"\n" +
				"func f(c *nc.Client) { _, _ = c.ConsumeStreamWithConfigContexts(nil, nil, nil, nc.StreamConsumerConfig{}, nil) }\n",
			want: "split-context canonical port census",
		},
		{
			name:   "port consumer config reader",
			source: "package planted\nfunc f(p interface{ GetConsumerConfig() any }) { _ = p.GetConsumerConfig() }\n",
			want:   "GetConsumerConfig production files",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "planted.go"), []byte(tt.source), 0o600); err != nil {
				t.Fatal(err)
			}
			planted := parseProductionGoFiles(t, root)
			violations := consumerPolicyCallsiteCensusViolations(append(append([]productionGoFile{}, tree...), planted...))
			joined := strings.Join(violations, "\n")
			if !strings.Contains(joined, tt.want) || !strings.Contains(joined, "planted.go") {
				t.Fatalf("census accepted a planted caller: violations = %q, want %q naming planted.go", joined, tt.want)
			}
			t.Logf("planted caller rejected: %s", joined)
		})
	}
}

func consumerPolicyCallsiteCensusViolations(files []productionGoFile) []string {
	internalCallers := map[string]int{}
	portCallers := map[string]int{}
	contextsPortCallers := map[string]int{}
	portConfigCallers := map[string]struct{}{}
	portBackedInternalCallers := map[string]struct{}{}
	for _, parsed := range files {
		usesPortConfig := false
		usesInternal := false
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "GetConsumerConfig":
				usesPortConfig = true
			case "ConsumeInternalStreamWithConfig":
				usesInternal = true
				internalCallers[parsed.rel]++
			case "ConsumeStreamWithConfig":
				portCallers[parsed.rel]++
			case "ConsumeStreamWithConfigContexts":
				contextsPortCallers[parsed.rel]++
			}
			return true
		})
		if usesPortConfig {
			portConfigCallers[parsed.rel] = struct{}{}
		}
		if usesPortConfig && usesInternal {
			portBackedInternalCallers[parsed.rel] = struct{}{}
		}
	}

	// SemEngine's tree at task 3.7: no production package outside natsclient consumes a stream
	// yet, and natsclient calls none of the four through a selector.
	var violations []string
	wantInternal := map[string]int{}
	if !reflect.DeepEqual(internalCallers, wantInternal) {
		violations = append(violations, fmt.Sprintf("internal consumer census = %#v, want %#v", internalCallers, wantInternal))
	}
	wantPort := map[string]int{}
	if !reflect.DeepEqual(portCallers, wantPort) {
		violations = append(violations, fmt.Sprintf("canonical port consumer census = %#v, want %#v", portCallers, wantPort))
	}
	wantContextsPort := map[string]int{}
	if !reflect.DeepEqual(contextsPortCallers, wantContextsPort) {
		violations = append(violations, fmt.Sprintf("split-context canonical port census = %#v, want %#v", contextsPortCallers, wantContextsPort))
	}
	if len(portConfigCallers) != 0 {
		violations = append(violations, fmt.Sprintf("GetConsumerConfig production files = %d, want 0: %#v", len(portConfigCallers), portConfigCallers))
	}
	if len(portBackedInternalCallers) != 0 {
		violations = append(violations, fmt.Sprintf("port-backed files use internal consumer path: %#v", portBackedInternalCallers))
	}
	return violations
}

func TestParseProductionGoFilesIgnoresClaudeWorktrees(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "kept.go"), []byte("package fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(root, ".claude", "worktrees", "agent-test")
	if err := os.MkdirAll(worktree, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "contamination.go"), []byte("package contamination\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	files := parseProductionGoFiles(t, root)
	if len(files) != 1 || files[0].rel != "kept.go" {
		t.Fatalf("production scan files = %#v, want only kept.go", files)
	}
}

func TestConsumerPolicyExportedClientAPICensus(t *testing.T) {
	files := parseProductionGoFiles(t, ".")
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
		t.Fatalf("exported Client consumer API census = %#v, want %#v", got, want)
	}
}

func TestNewDurableHandlerHasNoDeclarationOrProductionCalls(t *testing.T) {
	files := parseProductionGoFiles(t, filepath.Clean(".."))
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
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "bypass.go"), []byte(tt.source), 0o600); err != nil {
				t.Fatal(err)
			}
			if violations := newDurableHandlerRetirementViolations(parseProductionGoFiles(t, root)); len(violations) == 0 {
				t.Fatal("retirement guard accepted bypass")
			}
		})
	}
}

func TestNewDurableHandlerRetirementIgnoresUnrelatedSelector(t *testing.T) {
	root := t.TempDir()
	source := "package fixture\n" +
		"import other \"example.com/other\"\n" +
		"func callOther() { other.NewDurableHandler() }\n"
	if err := os.WriteFile(filepath.Join(root, "unrelated.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if violations := newDurableHandlerRetirementViolations(parseProductionGoFiles(t, root)); len(violations) != 0 {
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
	scan := scanLegacyHeartbeatReferences(parseProductionGoFiles(t, filepath.Clean("..")))
	if len(scan.violations) != 0 {
		t.Fatalf("retired ConsumeWithHeartbeat surface remains: %v", scan.violations)
	}
	if len(scan.directCalls) != 0 {
		t.Fatalf("ConsumeWithHeartbeat callers = %#v, want none: the helper no longer exists", scan.directCalls)
	}
}

func TestLegacyHeartbeatGuardRejectsTakingOrAliasingSymbol(t *testing.T) {
	root := t.TempDir()
	source := "package fixture\n" +
		"import nc \"github.com/c360studio/semengine/natsclient\"\n" +
		"var ExportedLegacyHeartbeat = nc.ConsumeWithHeartbeat\n" +
		"func callIndirect() { legacy := nc.ConsumeWithHeartbeat; _ = legacy(nil, nil, 0, nil) }\n"
	if err := os.WriteFile(filepath.Join(root, "indirect.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	scan := scanLegacyHeartbeatReferences(parseProductionGoFiles(t, root))
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
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "alternate.go"), []byte(tt.source), 0o600); err != nil {
				t.Fatal(err)
			}
			scan := scanLegacyHeartbeatReferences(parseProductionGoFiles(t, root))
			if len(scan.violations) == 0 {
				t.Fatal("legacy surface guard accepted alternate export")
			}
		})
	}
}

func TestLegacyHeartbeatGuardCountsDotImportAsDirectCall(t *testing.T) {
	root := t.TempDir()
	source := "package fixture\n" +
		"import . \"github.com/c360studio/semengine/natsclient\"\n" +
		"func callLegacy() { _ = ConsumeWithHeartbeat(nil, nil, 0, nil) }\n"
	if err := os.WriteFile(filepath.Join(root, "dot.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	scan := scanLegacyHeartbeatReferences(parseProductionGoFiles(t, root))
	if len(scan.violations) != 0 {
		t.Fatalf("dot-import direct call reported as indirect: %v", scan.violations)
	}
	want := map[string]int{"dot.go": 1}
	if !reflect.DeepEqual(scan.directCalls, want) {
		t.Fatalf("dot-import direct calls = %#v, want %#v", scan.directCalls, want)
	}
}

func TestLegacyHeartbeatGuardIgnoresUnrelatedSelector(t *testing.T) {
	root := t.TempDir()
	source := "package fixture\n" +
		"import other \"example.com/other\"\n" +
		"func callOther() { other.ConsumeWithHeartbeat() }\n"
	if err := os.WriteFile(filepath.Join(root, "unrelated.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	scan := scanLegacyHeartbeatReferences(parseProductionGoFiles(t, root))
	if len(scan.violations) != 0 || len(scan.directCalls) != 0 {
		t.Fatalf("unrelated selector classified as legacy: violations=%v direct=%#v", scan.violations, scan.directCalls)
	}
}

func TestNoDeliveryDispositionExportedSurface(t *testing.T) {
	files := parseProductionGoFiles(t, ".")
	for _, parsed := range files {
		for _, declaration := range parsed.file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if strings.Contains(declaration.Name.Name, "DeliveryDisposition") {
					t.Fatalf("forbidden disposition constructor/function exported: %s", declaration.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range declaration.Specs {
					if typed, ok := spec.(*ast.TypeSpec); ok && strings.Contains(typed.Name.Name, "DeliveryDisposition") {
						t.Fatalf("forbidden disposition type exported: %s", typed.Name.Name)
					}
				}
			}
		}
	}
}

func TestClientHasNoChildLifecycleSurfaceOrCatalog(t *testing.T) {
	files := parseProductionGoFiles(t, ".")
	forbiddenMethods := map[string]struct{}{
		"ConsumeDurable": {}, "StopConsumer": {}, "StopAndDeleteConsumer": {},
		"StopAllConsumers": {}, "OutstandingWork": {},
	}
	forbiddenFields := map[string]struct{}{
		"consumers": {}, "consumersMu": {}, "subs": {},
	}
	for _, parsed := range files {
		for _, declaration := range parsed.file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if ok && fn.Recv != nil && receiverIsClientPointer(fn.Recv) {
				if _, forbidden := forbiddenMethods[fn.Name.Name]; forbidden {
					t.Fatalf("forbidden Client lifecycle method remains: %s", fn.Name.Name)
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
							t.Fatalf("forbidden Client child catalog remains: %s", name.Name)
						}
					}
				}
			}
		}
	}
}

// TestConsumerPolicyDirectCreationCallCensus pins every production call that creates a consumer
// directly, outside the policy entry points, in SemEngine's tree. Measured at task 3.7; each later
// port that adds such a call updates the map (admission ledger row natsclient, known_risks).
func TestConsumerPolicyDirectCreationCallCensus(t *testing.T) {
	files := parseProductionGoFiles(t, filepath.Clean(".."))
	if violations := directConsumerCreationCensusViolations(files); len(violations) != 0 {
		t.Fatalf("%s", strings.Join(violations, "\n"))
	}
}

// TestConsumerPolicyDirectCreationCallCensusRejectsPlantedCaller adds one planted direct creation
// per method, written in a t.TempDir() tree, to the measured tree and requires the census to name it.
func TestConsumerPolicyDirectCreationCallCensusRejectsPlantedCaller(t *testing.T) {
	tree := parseProductionGoFiles(t, filepath.Clean(".."))
	for _, method := range []string{"CreateOrUpdateConsumer", "CreateConsumer", "OrderedConsumer"} {
		t.Run(method, func(t *testing.T) {
			root := t.TempDir()
			source := "package planted\nimport \"github.com/nats-io/nats.go/jetstream\"\n" +
				"func f(s jetstream.Stream) { _, _ = s." + method + "(nil, jetstream.ConsumerConfig{}) }\n"
			if method == "OrderedConsumer" {
				source = "package planted\nimport \"github.com/nats-io/nats.go/jetstream\"\n" +
					"func f(s jetstream.Stream) { _, _ = s.OrderedConsumer(nil, jetstream.OrderedConsumerConfig{}) }\n"
			}
			if err := os.WriteFile(filepath.Join(root, "planted.go"), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			planted := parseProductionGoFiles(t, root)
			violations := directConsumerCreationCensusViolations(append(append([]productionGoFile{}, tree...), planted...))
			joined := strings.Join(violations, "\n")
			if !strings.Contains(joined, "planted.go:"+method+"/args=2") {
				t.Fatalf("direct creation census accepted a planted %s: violations = %q", method, joined)
			}
			t.Logf("planted caller rejected: %s", joined)
		})
	}
}

func directConsumerCreationCensusViolations(files []productionGoFile) []string {
	got := map[string]int{}
	for _, parsed := range files {
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "CreateOrUpdateConsumer", "CreateConsumer", "OrderedConsumer":
				key := parsed.rel + ":" + selector.Sel.Name + "/args=" + strconv.Itoa(len(call.Args))
				got[key]++
			}
			return true
		})
	}

	// SemEngine's tree at task 3.7: natsclient's two policy-checked creations, and the test
	// fixture's probe consumer (internal/harness/natsfixture/fixture.go, a non-test file).
	want := map[string]int{
		"internal/harness/natsfixture/fixture.go:CreateConsumer/args=2": 1,
		"natsclient/stream.go:CreateOrUpdateConsumer/args=2":            2,
	}
	if !reflect.DeepEqual(got, want) {
		return []string{fmt.Sprintf("direct consumer creation census = %#v, want %#v", got, want)}
	}
	return nil
}

func parseProductionGoFiles(t *testing.T, root string) []productionGoFile {
	t.Helper()
	files := []productionGoFile{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".claude" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, productionGoFile{rel: filepath.ToSlash(rel), file: file})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
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
