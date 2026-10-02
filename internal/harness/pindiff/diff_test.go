package main

import (
	"fmt"
	"maps"
	"strings"
	"testing"
)

// harness-boundaries › "Comparison with the pin": a directory entry covers the .go files directly
// in it, test files included, and every file under its testdata directory; README.md and
// sub-directories are not covered; a note after the destination path is not part of it; a file
// against a directory is not compared.
func TestCoveredFiles(t *testing.T) {
	pin := map[string]string{
		"pkg/a/a.go":               "package a\n",
		"pkg/a/a_test.go":          "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
		"pkg/a/README.md":          "# a\n",
		"pkg/a/sub/s.go":           "package sub\n",
		"pkg/a/testdata/in.txt":    "input\n",
		"pkg/a/testdata/deep/x.go": "not go at all\n",
		"scripts/x.sh":             "echo x\n",
		"pkg/d/d.go":               "package d\n",
		"pkg/docs/README.md":       "# docs\n",
	}
	remote, shas := pinRepo(t, pin)
	ledger := ledgerOf(
		row{"pkg/a", shas[0], "internal/a (moved here by change x)", "carry"},
		row{"scripts/x.sh", shas[0], "scripts/x.sh", "adapt"},
		row{"pkg/d", shas[0], "pkg/d.go", "adapt"},
		row{"pkg/docs", shas[0], "internal/docs", "carry"},
		row{"pkg/nope", shas[0], "internal/a", "adapt"},
	)
	clean := map[string]string{
		"internal/a/a.go":               "package a\n",
		"internal/a/a_test.go":          "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
		"internal/a/README.md":          "# a, rewritten for SemEngine\n",
		"internal/a/notes.txt":          "not covered\n",
		"internal/a/sub/s.go":           "package sub // changed\n",
		"internal/a/sub/t.go":           "package sub\n",
		"internal/a/testdata/in.txt":    "input\n",
		"internal/a/testdata/deep/x.go": "not go at all\n",
		"scripts/x.sh/inside":           "a directory where the pin has a file\n",
		"pkg/d.go":                      "package d\n",
		"internal/docs/README.md":       "# docs\n",
	}
	with := func(changes map[string]string) map[string]string {
		files := maps.Clone(clean)
		maps.Copy(files, changes)
		return files
	}

	for _, tc := range []struct {
		name                     string
		files                    map[string]string
		arg                      string
		code                     int
		wantStdout, wantStderrLn string
	}{
		{"README and sub-directory out, testdata in", clean, "pkg/a", 0, "",
			"pkg/a (carry): 4 files, 0 differ, 0 only at the pin, 0 only in the tree"},
		{"a testdata file differs", with(map[string]string{"internal/a/testdata/in.txt": "changed\n"}), "pkg/a", 0,
			"--- pin/pkg/a/testdata/in.txt\n+++ internal/a/testdata/in.txt\n@@ -1 +1 @@\n-input\n+changed\n",
			"pkg/a (carry): 4 files, 1 differ, 0 only at the pin, 0 only in the tree"},
		{"a .go file under testdata is not rewritten or formatted", with(map[string]string{"internal/a/testdata/deep/x.go": "not  go at all\n"}), "pkg/a", 0,
			"--- pin/pkg/a/testdata/deep/x.go\n+++ internal/a/testdata/deep/x.go\n@@ -1 +1 @@\n-not go at all\n+not  go at all\n",
			"pkg/a (carry): 4 files, 1 differ, 0 only at the pin, 0 only in the tree"},
		{"a testdata file only in the tree", with(map[string]string{"internal/a/testdata/new.txt": "new\n"}), "pkg/a", 0,
			"only in the tree: internal/a/testdata/new.txt\n",
			"pkg/a (carry): 4 files, 0 differ, 0 only at the pin, 1 only in the tree"},
		{"file at the pin, directory in the tree", clean, "scripts/x.sh", 2, "",
			"scripts/x.sh (adapt): not compared: source_path is a file at the pin and the destination path is a directory"},
		{"directory at the pin, file in the tree", clean, "pkg/d", 2, "",
			"pkg/d (adapt): not compared: source_path is a directory at the pin and the destination path is a file"},
		{"source_path that does not exist at source_sha", clean, "pkg/nope", 2, "",
			"pkg/nope (adapt): not compared: source_path does not exist at " + shas[0]},
		{"no covered file at the pin or in the tree", clean, "pkg/docs", 2, "",
			"pkg/docs (carry): not compared: no covered file at the pin or in the tree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runIn(t, tree(t, ledger, tc.files), remoteEnv(remote), "diff", tc.arg)
			if got.code != tc.code || got.stdout != tc.wantStdout || !strings.Contains(got.stderr, tc.wantStderrLn+"\n") {
				t.Fatalf("got:\n%s\nwant exit %d, stdout %q, a stderr line %q", got, tc.code, tc.wantStdout, tc.wantStderrLn)
			}
		})
	}
}

// harness-boundaries › "Pin difference command": standard output holds a unified diff per
// differing file and a line per one-sided file, nothing for an equal file; standard error holds
// one line per entry.
func TestDiffOutput(t *testing.T) {
	a := "package a\n\n// One returns one.\nfunc One() int { return 1 }\n\n// Two returns two.\nfunc Two() int { return 2 }\n"
	remote, shas := pinRepo(t, map[string]string{
		"pkg/a/a.go":     a,
		"pkg/a/b.go":     "package a\n",
		"pkg/a/keep.go":  "package a\n",
		"pkg/e/e.go":     "package e\n",
		"scripts/run.sh": "echo one\necho two\n",
	})
	root := tree(t, ledgerOf(
		row{"pkg/a", shas[0], "internal/a", "adapt"},
		row{"pkg/e", shas[0], "internal/e", "carry"},
		row{"scripts/run.sh", shas[0], "scripts/run.sh (adapted)", "adapt"},
		row{"pkg/x", shas[0], "none", "defer-exclude"},
	), map[string]string{
		"internal/a/a.go":    strings.Replace(a, "return 2", "return 3", 1),
		"internal/a/keep.go": "package a\n",
		"internal/a/c.go":    "package a\n",
		"internal/e/e.go":    "package e\n",
		"scripts/run.sh":     "echo one\necho 2",
	})

	t.Run("every carry and adapt entry, in ledger order", func(t *testing.T) {
		got := runIn(t, root, remoteEnv(remote), "diff")
		wantStdout := "--- pin/pkg/a/a.go\n" +
			"+++ internal/a/a.go\n" +
			"@@ -4,4 +4,4 @@\n" +
			" func One() int { return 1 }\n" +
			" \n" +
			" // Two returns two.\n" +
			"-func Two() int { return 2 }\n" +
			"+func Two() int { return 3 }\n" +
			"only at the pin: pin/pkg/a/b.go\n" +
			"only in the tree: internal/a/c.go\n" +
			"--- pin/scripts/run.sh\n" +
			"+++ scripts/run.sh\n" +
			"@@ -1,2 +1,2 @@\n" +
			" echo one\n" +
			"-echo two\n" +
			"+echo 2\n" +
			"\\ No newline at end of file\n"
		wantStderr := "pkg/a (adapt): 2 files, 1 differ, 1 only at the pin, 1 only in the tree\n" +
			"pkg/e (carry): 1 files, 0 differ, 0 only at the pin, 0 only in the tree\n" +
			"scripts/run.sh (adapt): 1 files, 1 differ, 0 only at the pin, 0 only in the tree\n"
		if got.code != 0 || got.stdout != wantStdout || got.stderr != wantStderr {
			t.Fatalf("got:\n%s\nwant exit 0\n--- stdout\n%s--- stderr\n%s", got, wantStdout, wantStderr)
		}
	})

	t.Run("an unchanged entry prints nothing", func(t *testing.T) {
		got := runIn(t, root, remoteEnv(remote), "diff", "pkg/e")
		if got.code != 0 || got.stdout != "" || got.stderr != "pkg/e (carry): 1 files, 0 differ, 0 only at the pin, 0 only in the tree\n" {
			t.Fatalf("got:\n%s", got)
		}
	})
}

// Past the matching table's size the changed middle is printed as one removal and one addition;
// the program says so on standard error, naming the file, and the diff still applies.
func TestDiffLargeFileFallback(t *testing.T) {
	var pin, tree0 strings.Builder
	for i := 0; i < 2100; i++ {
		fmt.Fprintf(&pin, "pin %d\n", i)
		fmt.Fprintf(&tree0, "tree %d\n", i)
	}
	remote, shas := pinRepo(t, map[string]string{"docs/big.txt": pin.String()})
	root := tree(t, ledgerOf(row{"docs/big.txt", shas[0], "docs/big.txt", "adapt"}), map[string]string{"docs/big.txt": tree0.String()})
	got := runIn(t, root, remoteEnv(remote), "diff")
	if got.code != 0 {
		t.Fatalf("got exit %d\n%s", got.code, got.stderr)
	}
	requireLine(t, got.stderr, "docs/big.txt", "printed as one removal and one addition")
	applied, err := applyUnified(pin.String(), got.stdout)
	if err != nil || applied != tree0.String() {
		t.Fatalf("the printed diff does not turn the pin into the tree: %v", err)
	}
}

// harness-boundaries › "Pin difference command": exit 0 when the program ran, differences or not;
// exit 2 for an argument that names no entry, a named entry not compared, or a ledger that cannot
// be read.
func TestDiffExitCodes(t *testing.T) {
	remote, shas := pinRepo(t, map[string]string{"pkg/a/a.go": "package a\n", "pkg/b/b.go": "package b\n", "scripts/x.sh": "echo x\n"})
	ledger := ledgerOf(
		row{"pkg/a", shas[0], "internal/a", "carry"},
		row{"pkg/b", shas[0], "internal/missing", "adapt"},
		row{"scripts/x.sh", shas[0], "internal/a", "adapt"},
	)
	root := tree(t, ledger, map[string]string{"internal/a/a.go": "package a // changed\n"})

	for _, tc := range []struct {
		name   string
		root   string
		args   []string
		code   int
		stderr []string
	}{
		{"a differing entry", root, []string{"diff", "pkg/a"}, 0, []string{"pkg/a (carry): 1 files, 1 differ"}},
		{"no argument, an adapt entry not compared", root, []string{"diff"}, 0,
			[]string{"pkg/b (adapt): not compared:", "internal/missing", "does not exist"}},
		{"no argument, a file at the pin and a directory in the tree", root, []string{"diff"}, 0,
			[]string{"scripts/x.sh (adapt): not compared: source_path is a file at the pin and the destination path is a directory"}},
		{"an argument that names no entry", root, []string{"diff", "pkg/a", "pkg/nope"}, 2, []string{"pkg/nope", "no ledger entry"}},
		{"a named entry not compared", root, []string{"diff", "pkg/b"}, 2, []string{"pkg/b (adapt): not compared:"}},
		{"no ledger", t.TempDir(), []string{"diff"}, 2, []string{"docs/admission-ledger.yaml", "could not be read"}},
		{"no mode", root, nil, 2, []string{"usage"}},
		{"unknown mode", root, []string{"compare"}, 2, []string{"usage"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runIn(t, tc.root, remoteEnv(remote), tc.args...)
			if got.code != tc.code {
				t.Fatalf("got:\n%s\nwant exit %d", got, tc.code)
			}
			requireLine(t, got.stderr, tc.stderr...)
		})
	}
}
