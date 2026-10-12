package graphingest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// entityWriteSeamFile is the one file allowed to write graph-ingest's entity bucket (design D15).
const entityWriteSeamFile = "entity_writes.go"

// entityBucketWrites are the methods of natsclient.KVStore that change a stored entity.
var entityBucketWrites = map[string]bool{
	"Create":              true,
	"Update":              true,
	"UpdateJSON":          true,
	"UpdateWithRetry":     true,
	"UpdateWithRetryRead": true,
	"UpdateWithRetryRev":  true,
	"Put":                 true,
	"Delete":              true,
	"DeleteAtRevision":    true,
}

// TestEntityWritesHaveOneSeam holds #100's shape: every change to a stored entity goes through one
// write path, in entity_writes.go, where each write mode's rule is visible in one place.
func TestEntityWritesHaveOneSeam(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	files := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		body, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = string(body)
	}
	if violations := entityWriteViolations(files); len(violations) > 0 {
		t.Fatalf("entity bucket written outside %s:\n%s", entityWriteSeamFile, strings.Join(violations, "\n"))
	}
}

func TestEntityWritesHaveOneSeamSensitivity(t *testing.T) {
	const header = "package graphingest\n\n"
	// The seam writes the bucket; another file reads it, writes the guard bucket and the cache, and
	// names a write method in a comment and a string; a test file writes the bucket. All of it passes.
	clean := map[string]string{
		entityWriteSeamFile: header +
			"func (c *Component) createEntity(id string, b []byte) error {\n" +
			"\t_, err := c.entityBucket.Create(nil, id, b)\n\treturn err\n}\n",
		"other.go": header +
			"// c.entityBucket.Put is not called here.\nconst s = \"c.entityBucket.Delete\"\n\n" +
			"func (c *Component) other(id string, b []byte) {\n" +
			"\t_, _ = c.entityBucket.Get(nil, id)\n" +
			"\t_, _ = c.ingestGuardBucket.Put(nil, id, b)\n" +
			"\tc.entityCache.Delete(id)\n}\n",
		"other_test.go": header +
			"func plant(c *Component) {\n\t_ = c.entityBucket.Delete(nil, \"e\")\n}\n",
		"README.md": "c.entityBucket.Put(ctx, id, b)\n",
	}
	if violations := entityWriteViolations(clean); len(violations) > 0 {
		t.Fatalf("clean fixture reported:\n%s", strings.Join(violations, "\n"))
	}

	for _, tc := range []struct{ name, body, want string }{
		{"Put through the component", header +
			"func (c *Component) w(id string, b []byte) {\n\t_, _ = c.entityBucket.Put(nil, id, b)\n}\n",
			"planted.go:4: c.entityBucket.Put"},
		{"DeleteAtRevision through a nested selector", header +
			"func (a *adapter) w(id string) error {\n" +
			"\treturn a.component.entityBucket.DeleteAtRevision(nil, id, 1)\n}\n",
			"planted.go:4: a.component.entityBucket.DeleteAtRevision"},
		{"UpdateWithRetryRev inside a closure", header +
			"func (c *Component) w(id string) {\n\tgo func() {\n" +
			"\t\t_, _ = c.entityBucket.UpdateWithRetryRev(nil, id, nil)\n\t}()\n}\n",
			"planted.go:5: c.entityBucket.UpdateWithRetryRev"},
		{"UpdateWithRetryRead through the component", header +
			"func (c *Component) w(id string) {\n" +
			"\t_, _ = c.entityBucket.UpdateWithRetryRead(nil, id, nil)\n}\n",
			"planted.go:4: c.entityBucket.UpdateWithRetryRead"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := copyTree(clean)
			tree["planted.go"] = tc.body
			requireSeamViolation(t, entityWriteViolations(tree), tc.want)
		})
	}

	// Every write method is recognised, not only the ones the cases above plant.
	for method := range entityBucketWrites {
		t.Run("method "+method, func(t *testing.T) {
			tree := copyTree(clean)
			tree["planted.go"] = header + "func (c *Component) w() {\n\tc.entityBucket." + method + "()\n}\n"
			requireSeamViolation(t, entityWriteViolations(tree), "planted.go:4: c.entityBucket."+method)
		})
	}

	t.Run("seam with no write", func(t *testing.T) {
		// A renamed field leaves the seam with nothing the check recognises, and every write
		// elsewhere unrecognised too: the check must not pass by recognising nothing.
		tree := copyTree(clean)
		tree[entityWriteSeamFile] = header + "func (c *Component) createEntity() {}\n"
		requireSeamViolation(t, entityWriteViolations(tree), entityWriteSeamFile+": no entity bucket write")
	})
	t.Run("seam missing", func(t *testing.T) {
		tree := copyTree(clean)
		delete(tree, entityWriteSeamFile)
		requireSeamViolation(t, entityWriteViolations(tree), entityWriteSeamFile+": no entity bucket write")
	})
	t.Run("unparsable file", func(t *testing.T) {
		tree := copyTree(clean)
		tree["broken.go"] = header + "func f() {\n"
		requireSeamViolation(t, entityWriteViolations(tree), "broken.go: parse")
	})
}

// entityWriteViolations parses every non-test Go file in files (name to source) and reports each
// call of an entityBucketWrites method on the entity bucket outside entityWriteSeamFile, by file and
// line. A file that does not parse is a violation, and so is a seam with no write in it.
//
// The entity bucket is recognised by its field name: a receiver expression ending in the selector
// entityBucket (c.entityBucket, a.component.entityBucket). Type information would not tell it apart:
// the guard bucket (ingestGuardBucket) is the same type, *natsclient.KVStore, and it is the field
// that names the entity bucket. Matching the selector cannot see the bucket under another name (a
// local variable or a parameter holding it, as startEntityStateGuard's watch does); such an alias is
// review only. A renamed field is caught: the seam then holds no write the check recognises.
func entityWriteViolations(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var violations []string
	seamWrites := 0
	fset := token.NewFileSet()
	for _, name := range names {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, files[name], parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: parse: %v", name, err))
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !entityBucketWrites[method.Sel.Name] {
				return true
			}
			receiver, ok := method.X.(*ast.SelectorExpr)
			if !ok || receiver.Sel.Name != "entityBucket" {
				return true
			}
			if name == entityWriteSeamFile {
				seamWrites++
				return true
			}
			violations = append(violations, fmt.Sprintf("%s:%d: %s.%s writes the entity bucket outside %s",
				name, fset.Position(call.Pos()).Line, exprText(receiver), method.Sel.Name, entityWriteSeamFile))
			return true
		})
	}
	if seamWrites == 0 {
		violations = append(violations, entityWriteSeamFile+": no entity bucket write; the check recognises nothing")
	}
	return violations
}

// exprText renders a selector chain such as a.component.entityBucket for a violation message.
func exprText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprText(e.X) + "." + e.Sel.Name
	default:
		return "(expr)"
	}
}

func copyTree(tree map[string]string) map[string]string {
	out := make(map[string]string, len(tree))
	for k, v := range tree {
		out[k] = v
	}
	return out
}

// requireSeamViolation fails unless exactly one violation is reported and it contains want.
func requireSeamViolation(t *testing.T, violations []string, want string) {
	t.Helper()
	if len(violations) != 1 || !strings.Contains(violations[0], want) {
		t.Fatalf("want one violation containing %q, got %d:\n%s", want, len(violations), strings.Join(violations, "\n"))
	}
}
