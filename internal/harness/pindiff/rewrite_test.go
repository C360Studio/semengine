package main

import (
	"testing"
)

// harness-boundaries › "Comparison with the pin": the module path is rewritten in import paths
// and comments, never in a string literal; a package path ends before the first character that
// is not a letter, a digit, `/`, `_`, `-` or `.`, less a trailing `.` or `/`. Every expected
// text is written out here by hand.
func TestRewriteModulePath(t *testing.T) {
	moved := map[string]string{"pkg/tlsutil": "internal/tlsutil"}
	for _, tc := range []struct{ name, pin, want string }{
		{"import and comment, not string", "// Package a uses github.com/c360studio/semstreams/pkg/errs.\n" +
			"package a\n\n" +
			"import \"github.com/c360studio/semstreams/pkg/errs\"\n\n" +
			"// Wrap returns an error from github.com/c360studio/semstreams/pkg/errs.\n" +
			"func Wrap() error { return errs.New(\"github.com/c360studio/semstreams/pkg/errs\") }\n\n" +
			"const raw = `github.com/c360studio/semstreams`\n",
			"// Package a uses github.com/c360studio/semengine/pkg/errs.\n" +
				"package a\n\n" +
				"import \"github.com/c360studio/semengine/pkg/errs\"\n\n" +
				"// Wrap returns an error from github.com/c360studio/semengine/pkg/errs.\n" +
				"func Wrap() error { return errs.New(\"github.com/c360studio/semstreams/pkg/errs\") }\n\n" +
				"const raw = `github.com/c360studio/semstreams`\n"},
		{"block comment and named import", "package a\n\n" +
			"import (\n\tx \"github.com/c360studio/semstreams/pkg/errs\"\n)\n\n" +
			"/* see github.com/c360studio/semstreams */\nvar _ = x.New\n",
			"package a\n\n" +
				"import (\n\tx \"github.com/c360studio/semengine/pkg/errs\"\n)\n\n" +
				"/* see github.com/c360studio/semengine */\nvar _ = x.New\n"},
		{"doc link ends at ]", "// Package b links [github.com/c360studio/semstreams/pkg/tlsutil].\n" +
			"//\n" +
			"// [github.com/c360studio/semstreams/pkg/tlsutil]: https://pkg.go.dev/github.com/c360studio/semstreams/pkg/tlsutil\n" +
			"package b\n",
			"// Package b links [github.com/c360studio/semengine/internal/tlsutil].\n" +
				"//\n" +
				"// [github.com/c360studio/semengine/internal/tlsutil]: https://pkg.go.dev/github.com/c360studio/semengine/internal/tlsutil\n" +
				"package b\n"},
		{"trailing dot and slash are not the path", "package c\n\n" +
			"// See github.com/c360studio/semstreams/pkg/tlsutil.\n" +
			"// Or github.com/c360studio/semstreams/pkg/tlsutil/ and github.com/c360studio/semstreams.\n" +
			"var X int\n",
			"package c\n\n" +
				"// See github.com/c360studio/semengine/internal/tlsutil.\n" +
				"// Or github.com/c360studio/semengine/internal/tlsutil/ and github.com/c360studio/semengine.\n" +
				"var X int\n"},
		{"a package path that is not exactly a moved one", "package d\n\n" +
			"// github.com/c360studio/semstreams/pkg/tlsutilx and github.com/c360studio/semstreams/pkg/tlsutil/sub\n" +
			"var X int\n",
			"package d\n\n" +
				"// github.com/c360studio/semengine/pkg/tlsutilx and github.com/c360studio/semengine/pkg/tlsutil/sub\n" +
				"var X int\n"},
		{"another module that starts with the same text", "package e\n\n// github.com/c360studio/semstreams-ui/x\nvar X int\n",
			"package e\n\n// github.com/c360studio/semstreams-ui/x\nvar X int\n"},
		{"gofmt applied", "package f\nvar   X   int\n", "package f\n\nvar X int\n"},
		{"unparseable is compared as it stands", "package g\n// github.com/c360studio/semstreams\nfunc {\n",
			"package g\n// github.com/c360studio/semstreams\nfunc {\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(rewrite([]byte(tc.pin), moved)); got != tc.want {
				t.Fatalf("rewrite:\n--- got\n%s--- want\n%s", got, tc.want)
			}
		})
	}

	// The tree side gets the same rule: a .go file that is not a whole Go file stands as it is,
	// though gofmt's library would format it as a list of declarations.
	t.Run("tree side, not a whole Go file", func(t *testing.T) {
		if got := string(gofmt([]byte("var  X  int\n"))); got != "var  X  int\n" {
			t.Fatalf("gofmt: got %q, want it as it stands", got)
		}
	})
}

// A moved package is a carry or adapt directory entry at the same source_sha whose destination
// path is a directory in the tree and differs; re-sorted imports are equal. Run through the
// program, so the moved set comes from the ledger as the program reads it.
func TestRewriteMovedPackage(t *testing.T) {
	t.Run("re-sorted imports are equal", func(t *testing.T) {
		pin := "package m\n\nimport (\n" +
			"\t\"github.com/c360studio/semstreams/pkg/errs\"\n" +
			"\t\"github.com/c360studio/semstreams/pkg/tlsutil\"\n" +
			")\n"
		want := "package m\n\nimport (\n" +
			"\t\"github.com/c360studio/semengine/internal/tlsutil\"\n" +
			"\t\"github.com/c360studio/semengine/pkg/errs\"\n" +
			")\n"
		if got := string(rewrite([]byte(pin), map[string]string{"pkg/tlsutil": "internal/tlsutil"})); got != want {
			t.Fatalf("rewrite:\n--- got\n%s--- want\n%s", got, want)
		}
	})

	t.Run("moved set from the ledger", func(t *testing.T) {
		pkg := func(name string) string { return "package " + name + "\n\nvar X int\n" }
		imports := func(path, use string) string {
			return "package message\n\nimport \"github.com/c360studio/" + path + "\"\n\nvar _ = " + use + "\n"
		}
		base := map[string]string{
			"internal/semantictest/s.go": pkg("semantictest"),
			"pkg/tlsutil/t.go":           pkg("tlsutil"),
			"pkg/errs/e.go":              pkg("errs"),
			"pkg/old/o.go":               pkg("old"),
			"pkg/gone/g.go":              pkg("gone"),
			"pkg/other/o.go":             pkg("other"),
			"message/m.go": "package message\n\nimport (\n" +
				"\t\"github.com/c360studio/semstreams/internal/semantictest\"\n" +
				"\t\"github.com/c360studio/semstreams/pkg/errs\"\n" +
				"\t\"github.com/c360studio/semstreams/pkg/tlsutil\"\n" +
				")\n\nvar _, _, _ = semantictest.X, errs.X, tlsutil.X\n",
			"message/old.go":   imports("semstreams/pkg/old", "old.X"),
			"message/other.go": imports("semstreams/pkg/other", "other.X"),
			"message/gone.go":  imports("semstreams/pkg/gone", "gone.X"),
		}
		later := map[string]string{"pkg/later.go": "package pkg\n"}
		for k, v := range base {
			later[k] = v
		}
		remote, shas := pinRepo(t, base, later)
		root := tree(t, ledgerOf(
			row{"message", shas[0], "message", "carry"},
			row{"internal/semantictest", shas[0], "internal/harness/semantictest", "carry"},
			row{"pkg/tlsutil", shas[0], "internal/tlsutil (made internal by Q17)", "adapt"},
			row{"pkg/old", shas[0], "internal/oldcopy (nothing taken from the source)", "defer-exclude"},
			row{"pkg/gone", shas[0], "internal/gone", "adapt"},
			row{"pkg/other", shas[1], "internal/other", "adapt"},
		), map[string]string{
			"internal/harness/semantictest/s.go": pkg("semantictest"),
			"internal/tlsutil/t.go":              pkg("tlsutil"),
			"internal/oldcopy/o.go":              pkg("old"),
			"internal/other/o.go":                pkg("other"),
			"message/m.go": "package message\n\nimport (\n" +
				"\t\"github.com/c360studio/semengine/internal/harness/semantictest\"\n" +
				"\t\"github.com/c360studio/semengine/internal/tlsutil\"\n" +
				"\t\"github.com/c360studio/semengine/pkg/errs\"\n" +
				")\n\nvar _, _, _ = semantictest.X, errs.X, tlsutil.X\n",
			"message/old.go":   imports("semengine/internal/oldcopy", "old.X"),
			"message/other.go": imports("semengine/internal/other", "other.X"),
			"message/gone.go":  imports("semengine/pkg/gone", "gone.X"),
		})

		got := runIn(t, root, remoteEnv(remote), "diff", "message")
		wantStdout := "--- pin/message/old.go\n" +
			"+++ message/old.go\n" +
			"@@ -1,5 +1,5 @@\n" +
			" package message\n" +
			" \n" +
			"-import \"github.com/c360studio/semengine/pkg/old\"\n" +
			"+import \"github.com/c360studio/semengine/internal/oldcopy\"\n" +
			" \n" +
			" var _ = old.X\n" +
			"--- pin/message/other.go\n" +
			"+++ message/other.go\n" +
			"@@ -1,5 +1,5 @@\n" +
			" package message\n" +
			" \n" +
			"-import \"github.com/c360studio/semengine/pkg/other\"\n" +
			"+import \"github.com/c360studio/semengine/internal/other\"\n" +
			" \n" +
			" var _ = other.X\n"
		wantStderr := "message (carry): 4 files, 2 differ, 0 only at the pin, 0 only in the tree\n"
		if got.code != 0 || got.stdout != wantStdout || got.stderr != wantStderr {
			t.Fatalf("got:\n%s\nwant exit 0\n--- stdout\n%s--- stderr\n%s", got, wantStdout, wantStderr)
		}
	})
}
