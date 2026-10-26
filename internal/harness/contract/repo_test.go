package contract

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
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
			t.Fatal("no go.mod above the contract package")
		}
		dir = parent
	}
}

func repoFiles(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var files []string
	seen := map[string]bool{}
	for _, f := range bytes.Split(out, []byte{0}) {
		name := string(f)
		if name == "" || seen[name] {
			continue
		}
		// A tracked file deleted in the working tree is not content to check.
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			continue
		}
		seen[name] = true
		files = append(files, filepath.ToSlash(name))
	}
	sort.Strings(files)
	return files
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

func readLines(t *testing.T, root, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", name, err)
	}
	return lines
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
