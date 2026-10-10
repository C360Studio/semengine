package contract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/graphmutation"
)

// reservedSubjectsRule is the requirement every violation names.
const reservedSubjectsRule = "(graph-transport-boundary › Reserved request subjects have one declaration)"

// reservedSubjectHomes are the two files that declare the reserved subjects (design D20): graph's
// verb table and the mutation protocol. They may spell them; no other non-test Go file may.
var reservedSubjectHomes = map[string]bool{
	"graph/query_verbs.go":               true,
	"internal/graphmutation/protocol.go": true,
}

// reservedSubjects returns what no literal outside the homes may contain: each subject in the verb
// table and the mutation family's prefix, graph.mutation. Both are read from their declarations, so
// a verb added to the table is held from the commit that adds it.
func reservedSubjects() []string {
	var reserved []string
	for _, verb := range graph.QueryVerbs() {
		reserved = append(reserved, verb.Subject)
	}
	return append(reserved, strings.TrimSuffix(graphmutation.SubjectFamily, ">"))
}

// TestReservedSubjectsDeclaredOnce holds the rule that the request subjects graph-ingest serves and
// the graph mutation family are spelled only where they are declared: a second spelling can drift
// from the declaration, and no compiler notices.
//
// Requirement: graph-transport-boundary/Reserved request subjects have one declaration
func TestReservedSubjectsDeclaredOnce(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "reserved subject literals", reservedSubjectViolations(t, root, repoFiles(t, root)))
}

// Requirement: graph-transport-boundary/Reserved request subjects have one declaration
func TestReservedSubjectsDeclaredOnceSensitivity(t *testing.T) {
	clean := map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.26\n",
		// The two homes spell their subjects; a comment and a test file may name them; a literal that
		// only resembles one (the mutation interface name, with no dot after "mutation") is not one.
		"graph/query_verbs.go": "package graph\n\nvar subjects = []string{\"graph.ingest.query.entity\", " +
			"\"graph.ingest.query.batch\", \"graph.ingest.query.prefix\"}\n",
		"internal/graphmutation/protocol.go": "package graphmutation\n\nconst SubjectFamily = \"graph.mutation.>\"\n",
		"graph/exact_entity.go": "package graph\n\n// The reader requests graph.ingest.query.entity and never graph.mutation.>.\n" +
			"const interfaceType = \"semengine.graph.mutation\"\n",
		"graph/exact_entity_test.go": "package graph\n\nconst want = \"graph.ingest.query.entity\"\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", reservedSubjectViolations(t, root, files))

	for _, tc := range []struct {
		name, file, body, want string
	}{
		{"entity subject", "graph/y.go", "package graph\n\nconst s = \"graph.ingest.query.entity\"\n", "graph/y.go:3"},
		{"batch subject as a raw string", "graph/y.go", "package graph\n\nconst s = `graph.ingest.query.batch`\n",
			"graph/y.go:3"},
		{"prefix subject inside a longer literal", "graph/y.go",
			"package graph\n\nvar s = \"no reply on graph.ingest.query.prefix\"\n", "graph/y.go:3"},
		{"mutation prefix", "processor/x/x.go",
			"package x\n\nfunc subject(op string) string {\n\treturn \"graph.mutation.\" + op\n}\n", "processor/x/x.go:4"},
		{"mutation family", "processor/x/x.go", "package x\n\nconst family = \"graph.mutation.>\"\n",
			"processor/x/x.go:3"},
		// A home is exempt by its path, not by its file name.
		{"a home's file name elsewhere", "processor/x/query_verbs.go",
			"package x\n\nconst s = \"graph.ingest.query.entity\"\n", "processor/x/query_verbs.go:3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := map[string]string{}
			for k, v := range clean {
				tree[k] = v
			}
			tree[tc.file] = tc.body
			root, files := writeTree(t, tree)
			requireViolation(t, reservedSubjectViolations(t, root, files), tc.want, reservedSubjectsRule)
		})
	}
	t.Run("unparsable file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/x.go": "package x\n\nfunc f() {\n"})
		requireViolation(t, reservedSubjectViolations(t, root, files), "x/x.go", "parse")
	})
	t.Run("no Go file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/README.md": "graph.ingest.query.entity\n"})
		requireViolation(t, reservedSubjectViolations(t, root, files), "scanned no Go file")
	})
}

// reservedSubjectViolations parses every non-test Go file other than the two homes and reports each
// string literal that contains a reserved subject or the mutation prefix. A comment is not a
// literal, so a subject named in one is not reported. A file that does not parse is a violation: a
// check that could not read a file has not passed it.
func reservedSubjectViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	reserved := reservedSubjects()
	var violations []string
	scanned := 0
	fset := token.NewFileSet()
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || reservedSubjectHomes[name] {
			continue
		}
		scanned++
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(name)), nil, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: parse: %v", name, err))
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			at := fmt.Sprintf("%s:%d", name, fset.Position(lit.Pos()).Line)
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				violations = append(violations, fmt.Sprintf("%s: unreadable literal %s: %v %s",
					at, lit.Value, err, reservedSubjectsRule))
				return true
			}
			for _, subject := range reserved {
				if strings.Contains(value, subject) {
					violations = append(violations, fmt.Sprintf("%s: %s spells %s; take it from graph.QueryVerbs "+
						"or internal/graphmutation %s", at, lit.Value, subject, reservedSubjectsRule))
					break
				}
			}
			return true
		})
	}
	if scanned == 0 {
		violations = append(violations, fmt.Sprintf("scanned no Go file among %d file(s)", len(files)))
	}
	return violations
}
