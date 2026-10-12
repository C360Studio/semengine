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
)

const (
	prometheusPath = "github.com/prometheus/client_golang/prometheus"
	promautoPath   = "github.com/prometheus/client_golang/prometheus/promauto"
	// globalRegistrationRule is the requirement every violation names.
	globalRegistrationRule = "(metric-registry › No process-global registration)"
)

// prometheusGlobals are the prometheus package's names that reach the process-global registry.
var prometheusGlobals = map[string]bool{"DefaultRegisterer": true, "MustRegister": true, "Register": true}

// TestNoProcessGlobalRegistration holds the rule that a component registers its collectors only on
// the registry it is given: a series on the process-global registry outlives the component and is
// shared by every instance in the process.
//
// Requirement: metric-registry/No process-global registration
func TestNoProcessGlobalRegistration(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "process-global registration", globalRegistrationViolations(t, root, repoFiles(t, root)))
}

// Requirement: metric-registry/No process-global registration
func TestNoProcessGlobalRegistrationSensitivity(t *testing.T) {
	const header = "package x\n\nimport "
	clean := map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.26\n",
		// Registration on a given registry, promauto bound to one, and the names in a comment or a
		// string pass; a test file may use the global registry (it gathers to prove absence).
		"x/x.go": header + "(\n\t\"github.com/prometheus/client_golang/prometheus\"\n" +
			"\t\"github.com/prometheus/client_golang/prometheus/promauto\"\n)\n\n" +
			"// prometheus.MustRegister is not called here.\nconst s = \"prometheus.DefaultRegisterer\"\n\n" +
			"func f(reg *prometheus.Registry, c prometheus.Collector) {\n\treg.MustRegister(c)\n" +
			"\t_ = promauto.With(reg).NewCounter(prometheus.CounterOpts{Name: \"n\"})\n}\n",
		"x/x_test.go": header + "\"github.com/prometheus/client_golang/prometheus\"\n\n" +
			"var _ = prometheus.DefaultRegisterer\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", globalRegistrationViolations(t, root, files))

	for _, tc := range []struct {
		name, body, want string
	}{
		{"MustRegister", header + "\"github.com/prometheus/client_golang/prometheus\"\n\n" +
			"func f(c prometheus.Collector) {\n\tprometheus.MustRegister(c)\n}\n", "x/y.go:6"},
		{"Register", header + "\"github.com/prometheus/client_golang/prometheus\"\n\n" +
			"func f(c prometheus.Collector) error {\n\treturn prometheus.Register(c)\n}\n", "x/y.go:6"},
		{"DefaultRegisterer", header + "\"github.com/prometheus/client_golang/prometheus\"\n\n" +
			"var r = prometheus.DefaultRegisterer\n", "x/y.go:5"},
		{"renamed import", header + "prom \"github.com/prometheus/client_golang/prometheus\"\n\n" +
			"func f(c prom.Collector) {\n\tprom.MustRegister(c)\n}\n", "x/y.go:6"},
		{"promauto without a registry", header + "(\n\t\"github.com/prometheus/client_golang/prometheus\"\n" +
			"\t\"github.com/prometheus/client_golang/prometheus/promauto\"\n)\n\n" +
			"var c = promauto.NewCounter(prometheus.CounterOpts{Name: \"n\"})\n", "x/y.go:8"},
		{"dot import", header + ". \"github.com/prometheus/client_golang/prometheus\"\n\n" +
			"func f(c Collector) {\n\tMustRegister(c)\n}\n", "x/y.go:3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := map[string]string{}
			for k, v := range clean {
				tree[k] = v
			}
			tree["x/y.go"] = tc.body
			root, files := writeTree(t, tree)
			requireViolation(t, globalRegistrationViolations(t, root, files), tc.want, globalRegistrationRule)
		})
	}
	t.Run("unparsable file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/x.go": "package x\n\nfunc f() {\n"})
		requireViolation(t, globalRegistrationViolations(t, root, files), "x/x.go", "parse")
	})
	t.Run("no Go file", func(t *testing.T) {
		root, files := writeTree(t, map[string]string{"x/README.md": "prometheus.MustRegister\n"})
		requireViolation(t, globalRegistrationViolations(t, root, files), "scanned no Go file")
	})
}

// globalRegistrationViolations parses every non-test Go file and reports each use of a prometheus
// name that reaches the process-global registry, and each promauto constructor not bound to a
// registry with promauto.With, under whatever name the file imports the package. A dot import is
// reported because it hides the package name the check reads. A file that does not parse is a
// violation: a check that could not read a file has not passed it.
func globalRegistrationViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	scanned := 0
	fset := token.NewFileSet()
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(name)), nil, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: parse: %v", name, err))
			continue
		}
		local := map[string]string{} // local package name -> import path
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil || (path != prometheusPath && path != promautoPath) {
				continue
			}
			ident := filepath.Base(path)
			if spec.Name != nil {
				ident = spec.Name.Name
			}
			if ident == "." {
				violations = append(violations, fmt.Sprintf("%s:%d: dot import of %s hides which registry is used %s",
					name, fset.Position(spec.Pos()).Line, path, globalRegistrationRule))
				continue
			}
			local[ident] = path
		}
		if len(local) == 0 {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch local[pkg.Name] {
			case prometheusPath:
				if !prometheusGlobals[sel.Sel.Name] {
					return true
				}
			case promautoPath:
				if sel.Sel.Name == "With" {
					return true
				}
			default:
				return true
			}
			violations = append(violations, fmt.Sprintf("%s:%d: %s.%s registers on the process-global registry; "+
				"register on the metric.MetricsRegistry the component is given %s",
				name, fset.Position(sel.Pos()).Line, pkg.Name, sel.Sel.Name, globalRegistrationRule))
			return true
		})
	}
	if scanned == 0 {
		violations = append(violations, fmt.Sprintf("scanned no Go file among %d file(s)", len(files)))
	}
	return violations
}
