package main

import (
	"maps"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// checkPin is SemStreams as the check tests see it: a carried package with a non-test file, a
// test file and a testdata file, and a package the ledger excludes.
var checkPin = map[string]string{
	"pkg/a/a.go": "package a\n\n" +
		"import \"github.com/c360studio/semstreams/pkg/old\"\n\n" +
		"// Name is the module path, held in a string.\n" +
		"var Name = \"github.com/c360studio/semstreams/pkg/a\"\n\n" +
		"var _ = old.X\n",
	"pkg/a/a_test.go":       "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
	"pkg/a/testdata/in.txt": "input\n",
	"pkg/old/o.go":          "package old\n\nvar X int\n",
	// A directory with nothing the comparison covers: a README and a sub-package only.
	"pkg/docs/README.md": "# docs\n",
	"pkg/docs/sub/s.go":  "package sub\n",
}

// checkTree is the tree that carries pkg/a unchanged: the module path rewritten in the import and
// left alone in the string.
var checkTree = map[string]string{
	"internal/a/a.go": "package a\n\n" +
		"import \"github.com/c360studio/semengine/pkg/old\"\n\n" +
		"// Name is the module path, held in a string.\n" +
		"var Name = \"github.com/c360studio/semstreams/pkg/a\"\n\n" +
		"var _ = old.X\n",
	"internal/a/a_test.go":       "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
	"internal/a/testdata/in.txt": "input\n",
	"internal/oldcopy/o.go":      "package old\n\nvar X int\n",
}

// harness-boundaries › "Carried entries match the pin": each planted violation exits 1 and names
// the entry, the file and which of the four kinds it is, with the closing lines.
func TestCheckSensitivity(t *testing.T) {
	remote, shas := pinRepo(t, checkPin)
	ledger := func(dest string) string {
		return ledgerOf(
			row{"pkg/a", shas[0], dest, "carry"},
			row{"pkg/old", shas[0], "internal/oldcopy (nothing taken from the source)", "defer-exclude"},
		)
	}
	edit := func(name, from, to string) func(map[string]string) {
		return func(files map[string]string) { files[name] = strings.Replace(files[name], from, to, 1) }
	}

	clean := runIn(t, tree(t, ledger("internal/a"), checkTree), remoteEnv(remote), "check")
	if clean.code != 0 {
		t.Fatalf("clean fixture:\n%s", clean)
	}

	for _, tc := range []struct {
		name   string
		dest   string // the destination; empty for internal/a
		plant  func(files map[string]string)
		wants  []string
		reason bool // a not-compared case: the file is the reason
	}{
		{"one-line edit in a non-test file", "", edit("internal/a/a.go", "held in a string", "held in a constant"),
			[]string{"internal/a/a.go", "differs from the pin"}, false},
		{"one-line edit in a test file", "", edit("internal/a/a_test.go", "TestA", "TestARepaired"),
			[]string{"internal/a/a_test.go", "differs from the pin"}, false},
		{"one-line edit in a testdata file", "", edit("internal/a/testdata/in.txt", "input", "output"),
			[]string{"internal/a/testdata/in.txt", "differs from the pin"}, false},
		{"file removed", "", func(files map[string]string) { delete(files, "internal/a/a_test.go") },
			[]string{"pin/pkg/a/a_test.go", "only at the pin"}, false},
		{"file added", "", func(files map[string]string) { files["internal/a/extra.go"] = "package a\n" },
			[]string{"internal/a/extra.go", "only in the tree"}, false},
		{"string literal holding the module path changed", "",
			edit("internal/a/a.go", "\"github.com/c360studio/semstreams/pkg/a\"", "\"github.com/c360studio/semengine/pkg/a\""),
			[]string{"internal/a/a.go", "differs from the pin"}, false},
		{"import redirected to a defer-exclude destination", "",
			edit("internal/a/a.go", "semengine/pkg/old", "semengine/internal/oldcopy"),
			[]string{"internal/a/a.go", "differs from the pin"}, false},
		{"destination that does not exist", "internal/missing", nil,
			[]string{"not compared", `"internal/missing" does not exist`}, true},
		{"destination that starts with /", "<abs>", nil,
			[]string{"not compared", "not a path inside this repository"}, true},
		{"destination with a .. segment", "internal/../internal/a", nil,
			[]string{"not compared", `"internal/../internal/a" is not a path inside this repository`}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := maps.Clone(checkTree)
			if tc.plant != nil {
				tc.plant(files)
			}
			root := tree(t, "", files)
			dest := tc.dest
			switch dest {
			case "":
				dest = "internal/a"
			case "<abs>":
				// An absolute path to the very directory that holds the carried files.
				dest = filepath.ToSlash(filepath.Join(root, "internal", "a"))
			}
			writeFiles(t, root, map[string]string{"docs/admission-ledger.yaml": ledger(dest)})

			got := runIn(t, root, remoteEnv(remote), "check")
			if got.code != 1 {
				t.Fatalf("got:\n%s\nwant exit 1", got)
			}
			requireLine(t, got.stderr, append([]string{"carry entry pkg/a:"}, tc.wants...)...)
			requireLine(t, got.stderr, "task ledger:diff -- pkg/a", "prints the lines")
			requireLine(t, got.stderr, "differs from the pin is adapt", "docs/provenance.md rule 5")
		})
	}

	// A carry entry that compared nothing does not pass.
	t.Run("no covered file at the pin or in the tree", func(t *testing.T) {
		files := maps.Clone(checkTree)
		files["internal/docs/README.md"] = "# docs\n"
		root := tree(t, ledgerOf(row{"pkg/docs", shas[0], "internal/docs", "carry"}), files)
		got := runIn(t, root, remoteEnv(remote), "check")
		if got.code != 1 {
			t.Fatalf("got:\n%s\nwant exit 1", got)
		}
		requireLine(t, got.stderr, "carry entry pkg/docs:", "not compared", "no covered file at the pin or in the tree")
	})
}

// Only carry entries are checked: a carried entry passes, an adapt entry is never failed, and with
// no carry entry SemStreams is not contacted.
func TestCheckScope(t *testing.T) {
	remote, shas := pinRepo(t, checkPin)

	t.Run("a carried entry passes", func(t *testing.T) {
		root := tree(t, ledgerOf(row{"pkg/a", shas[0], "internal/a", "carry"}), checkTree)
		got := runIn(t, root, remoteEnv(remote), "check")
		if got.code != 0 {
			t.Fatalf("got:\n%s\nwant exit 0", got)
		}
		requireLine(t, got.stderr, "pkg/a (carry): 3 files, 0 differ, 0 only at the pin, 0 only in the tree")
	})

	t.Run("an adapt entry is never failed", func(t *testing.T) {
		files := maps.Clone(checkTree)
		files["internal/old/o.go"] = "package old // adapted\n"
		// A file entry: an adapt directory entry for pkg/old would move pkg/old, and pkg/a imports it.
		root := tree(t, ledgerOf(
			row{"pkg/a", shas[0], "internal/a", "carry"},
			row{"pkg/old/o.go", shas[0], "internal/old/o.go (adapted)", "adapt"},
			row{"pkg/never", shas[0], "internal/never", "adapt"},
		), files)
		got := runIn(t, root, remoteEnv(remote), "check")
		if got.code != 0 || strings.Contains(got.stderr, "pkg/old") || strings.Contains(got.stderr, "pkg/never") {
			t.Fatalf("got:\n%s\nwant exit 0 and no line about an adapt entry", got)
		}
	})

	t.Run("no carry entry, unreachable remote", func(t *testing.T) {
		root := tree(t, ledgerOf(row{"pkg/old", shas[0], "internal/old", "adapt"}), checkTree)
		got := runIn(t, root, remoteEnv(filepath.Join(t.TempDir(), "no-such-remote")), "check")
		if got.code != 0 {
			t.Fatalf("got:\n%s\nwant exit 0", got)
		}
		requireLine(t, got.stderr, "no carry entry", "not fetched")
	})
}

// harness-boundaries › "Carried entries match the pin": when the pin cannot be read the program
// exits 2, says so, and says no entry was checked. The fetch bound is one for the whole run.
func TestCheckPinUnreadable(t *testing.T) {
	remote, shas := pinRepo(t, checkPin)
	carry := ledgerOf(row{"pkg/a", shas[0], "internal/a", "carry"})

	t.Run("remote refuses", func(t *testing.T) {
		got := runIn(t, tree(t, carry, checkTree), remoteEnv(filepath.Join(t.TempDir(), "no-such-remote")), "check")
		requirePinUnreadable(t, got, shas[0])
	})

	t.Run("remote lacks the commit", func(t *testing.T) {
		missing := strings.Repeat("0", 40)
		got := runIn(t, tree(t, ledgerOf(row{"pkg/a", missing, "internal/a", "carry"}), checkTree), remoteEnv(remote), "check")
		requirePinUnreadable(t, got, missing)
	})

	t.Run("remote never answers", func(t *testing.T) {
		first, second := strings.Repeat("1", 40), strings.Repeat("2", 40)
		root := tree(t, ledgerOf(
			row{"pkg/a", first, "internal/a", "carry"},
			row{"pkg/b", second, "internal/a", "carry"},
		), checkTree)
		env := map[string]string{"SEMENGINE_PIN_REMOTE": silentRemote(t), "SEMENGINE_PIN_FETCH_BOUND": "300ms"}
		got := runIn(t, root, env, "check")
		requirePinUnreadable(t, got, first)
		requireLine(t, got.stderr, first, "did not finish within the fetch bound of 300ms")
		// One bound for the run: the second fetch never starts, it does not get a bound of its own.
		requireLine(t, got.stderr, second, "not fetched: the fetch bound of 300ms for the whole run was spent")
	})
}

func requirePinUnreadable(t *testing.T, got outcome, sha string) {
	t.Helper()
	if got.code != 2 {
		t.Fatalf("got:\n%s\nwant exit 2", got)
	}
	requireLine(t, got.stderr, "the pin could not be read", sha)
	requireLine(t, got.stderr, "no entry was checked")
}

// silentRemote is an HTTP remote that accepts connections and never answers. Its connections are
// closed when the test ends, which also ends any git transport helper still waiting on one.
func silentRemote(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-accepted
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return "http://" + ln.Addr().String() + "/semstreams.git"
}
