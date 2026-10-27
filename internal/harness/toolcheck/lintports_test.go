package toolcheck

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runLintTestPorts(root string) ([]byte, error) {
	cmd := exec.Command("bash", "scripts/lint-test-ports.sh")
	cmd.Dir = root
	return cmd.CombinedOutput()
}

// TestLintTestPortsHonoursNoMarker (harness-boundaries › "Marked fixed port", "Guidance keeps the
// listener"): a fixed port on a line carrying the old inline marker still fails the guard, the
// line is named, and the guidance says to bind port 0 and hand the listener on, naming no file.
// The planted line is assembled at run time so this file does not trip the guard itself.
func TestLintTestPortsHonoursNoMarker(t *testing.T) {
	root := copyScript(t, "lint-test-ports.sh")
	planted := "func f() { net." + "Listen(\"tcp\", \"127.0.0.1:18082\") } // gh#220:allow-fixed-port"
	if err := os.MkdirAll(filepath.Join(root, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "x", "x_test.go"), []byte("package x\n\n"+planted+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runLintTestPorts(root)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("a marked fixed port: err=%v, want exit 1\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "x/x_test.go:3:") {
		t.Errorf("output does not name the marked line x/x_test.go:3:\n%s", text)
	}
	var guidance []string
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "x_test.go:3:") {
			guidance = append(guidance, line)
		}
	}
	g := strings.Join(guidance, "\n")
	for _, want := range []string{"port 0", "listener"} {
		if !strings.Contains(g, want) {
			t.Errorf("guidance does not mention %q:\n%s", want, g)
		}
	}
	if strings.Contains(g, ".go") {
		t.Errorf("guidance names a Go file:\n%s", g)
	}
}

// TestLintTestPortsSkipsIgnoredFiles (design D6): the guard reads no git-ignored path, such as a
// main checkout's .worktrees/ copies, yet still refuses the same fixed port at a path git does not
// ignore. The planted line is assembled at run time so this file does not trip the guard itself.
func TestLintTestPortsSkipsIgnoredFiles(t *testing.T) {
	planted := "package x\n\nfunc f() { net." + "Listen(\"tcp\", \"127.0.0.1:18082\") }\n"
	for _, tc := range []struct {
		dir      string
		wantExit int
	}{
		{dir: "ign", wantExit: 0},
		{dir: "kept", wantExit: 1},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			root := copyScript(t, "lint-test-ports.sh")
			if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ign/\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, tc.dir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, tc.dir, "x_test.go"), []byte(planted), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := runLintTestPorts(root)
			got := 0
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				got = exit.ExitCode()
			} else if err != nil {
				t.Fatalf("running the guard: %v\n%s", err, out)
			}
			if got != tc.wantExit {
				t.Fatalf("fixed port in %s/x_test.go with .gitignore naming ign/: exit %d, want %d\n%s",
					tc.dir, got, tc.wantExit, out)
			}
		})
	}
}
