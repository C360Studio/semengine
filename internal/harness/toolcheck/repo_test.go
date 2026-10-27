package toolcheck

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the toolcheck package")
		}
		dir = parent
	}
}

func writeTree(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var names []string
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return root, names
}

func requireViolation(t *testing.T, violations []string, fragments ...string) {
	t.Helper()
	for _, v := range violations {
		all := true
		for _, f := range fragments {
			if !strings.Contains(v, f) {
				all = false
				break
			}
		}
		if all {
			return
		}
	}
	t.Fatalf("no violation mentions %q; got %d violation(s):\n  %s",
		fragments, len(violations), strings.Join(violations, "\n  "))
}

func requireNoViolations(t *testing.T, what string, violations []string) {
	t.Helper()
	if len(violations) > 0 {
		t.Fatalf("%s violations:\n  %s", what, strings.Join(violations, "\n  "))
	}
}

func fakeBin(t *testing.T, tools map[string]string, extra ...string) []string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range tools {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") {
			kv = "PATH=" + dir + ":" + strings.TrimPrefix(kv, "PATH=")
		}
		env = append(env, kv)
	}
	return append(env, extra...)
}

func copyScript(t *testing.T, name string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", name))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", name), src, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}
