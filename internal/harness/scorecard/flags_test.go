package main

import (
	"io"
	"strings"
	"testing"
)

const goodCommit = "0123456789abcdef0123456789abcdef01234567"

func goodArgs(extra ...string) []string {
	return append([]string{"-url", "http://127.0.0.1:18081/v1", "-model", "m", "-commit", goodCommit, "-tree", "clean"}, extra...)
}

func TestOptionsAccepted(t *testing.T) {
	env := map[string]string{"KEY": "secret"}
	opts, err := parseOptions(goodArgs("-query-prefix", "Q: ", "-api-key-env", "KEY", "-provider", "semembed"),
		func(k string) string { return env[k] }, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := options{url: "http://127.0.0.1:18081/v1", model: "m", queryPrefix: "Q: ", apiKey: "secret",
		apiKeyEnv: "KEY", provider: "semembed", commit: goodCommit, tree: "clean"}
	if opts != want {
		t.Errorf("options = %+v, want %+v", opts, want)
	}
}

// Each refusal names what is wrong, before anything is started.
func TestOptionsRefused(t *testing.T) {
	noEnv := func(string) string { return "" }
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no url", []string{"-model", "m", "-commit", goodCommit, "-tree", "clean"}, "-url is required"},
		{"no model", []string{"-url", "http://h/v1", "-commit", goodCommit, "-tree", "clean"}, "-model is required"},
		{"url not http", goodArgs("-url", "ftp://h/v1"), "httpembedder"},
		{"url without host", goodArgs("-url", "http:///v1"), "httpembedder"},
		{"unknown flag", goodArgs("-floor", "0.5"), "flag provided but not defined: -floor"},
		{"positional argument", goodArgs("extra"), `unexpected argument "extra"`},
		{"key variable unset", goodArgs("-api-key-env", "NOPE"), "-api-key-env NOPE: the variable is empty or unset"},
		{"no commit", []string{"-url", "http://h/v1", "-model", "m", "-tree", "clean"}, "-commit"},
		{"short commit", goodArgs("-commit", "0123abc"), "-commit"},
		{"bad tree", goodArgs("-tree", "maybe"), `-tree "maybe"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOptions(tc.args, noEnv, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("parseOptions(%q) = %v, want an error holding %q", tc.args, err, tc.want)
			}
		})
	}
}

// The program's TB runs cleanups last-registered first, and an Errorf marks the run failed.
func TestProgramTB(t *testing.T) {
	var out strings.Builder
	tb := &programTB{out: &out}
	var order []int
	tb.Cleanup(func() { order = append(order, 1) })
	tb.Cleanup(func() { order = append(order, 2); tb.Errorf("stop %s: %v", "x", "gone") })
	if tb.failed {
		t.Fatal("failed before any Errorf")
	}
	tb.runCleanups()
	if len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Errorf("cleanups ran in order %v, want [2 1]", order)
	}
	if !tb.failed || !strings.Contains(out.String(), "stop x: gone") {
		t.Errorf("after Errorf: failed %v, output %q", tb.failed, out.String())
	}
}
