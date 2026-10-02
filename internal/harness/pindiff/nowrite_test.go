package main

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

// harness-boundaries › "Carried entries match the pin" and "Pin difference command": the program
// writes nothing inside the repository. Each run happens with every directory of the tree made
// read-only, so a write the program would clean up itself (a fetch directory removed on return)
// still fails the run, and the tree's listing with content hashes is identical afterwards.
func TestWritesNothingInTree(t *testing.T) {
	remote, shas := pinRepo(t, checkPin)
	ledger := ledgerOf(row{"pkg/a", shas[0], "internal/a", "carry"})
	violation := maps.Clone(checkTree)
	violation["internal/a/a.go"] += "// edited\n"
	unreachable := filepath.Join(t.TempDir(), "no-such-remote")

	for _, tc := range []struct {
		name   string
		files  map[string]string
		remote string
		args   []string
		code   int
	}{
		{"check, passing tree", checkTree, remote, []string{"check"}, 0},
		{"check, planted violation", violation, remote, []string{"check"}, 1},
		{"check, pin unreadable", checkTree, unreachable, []string{"check"}, 2},
		{"diff, passing tree", checkTree, remote, []string{"diff"}, 0},
		{"diff, planted violation", violation, remote, []string{"diff", "pkg/a"}, 0},
		{"diff, pin unreadable", checkTree, unreachable, []string{"diff"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := tree(t, ledger, tc.files)
			before := snapshot(t, root)
			readOnly(t, root)
			got := runIn(t, root, remoteEnv(tc.remote), tc.args...)
			if got.code != tc.code {
				t.Fatalf("got:\n%s\nwant exit %d", got, tc.code)
			}
			after := snapshot(t, root)
			if !maps.Equal(before, after) {
				t.Fatalf("tree changed:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

// snapshot maps every path under root to a hash of its content, or "dir" for a directory.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			files[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// readOnly removes write permission from every directory under root until the test ends.
func readOnly(t *testing.T, root string) {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if err := os.Chmod(d, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, d := range dirs {
			_ = os.Chmod(d, 0o755)
		}
	})
}
