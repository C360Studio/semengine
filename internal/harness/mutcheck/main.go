// Package main is the program behind `task mutate:check` (spec mutation-check). It runs the
// experiment of docs/testing.md, "Show that the test can fail", for one wrong change to one Go
// source file, and classifies the outcome by rule as detection, survivor, invalid or inconclusive.
//
//	go run ./internal/harness/mutcheck -pkg ./internal/x -test TestY -file internal/x/y.go \
//	  -mutant /tmp/y.go -expect y_test.go:42
//
// The wrong change reaches every build of a mutant run through GOFLAGS=-overlay=<file>, so the
// program writes nothing in the repository: the overlay, its copy of the wrong change and the
// logs are in a temporary directory outside it. It runs from the module's root, prints a report
// whose last line is the verdict, and exits zero only for detection.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, ".", os.Args[1:], os.Environ(), os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run is the program with its working directory, arguments, environment and output passed in.
// Every child process gets an environment built from environ. It returns the exit status: 0 for
// detection, 1 for any other verdict, 2 when it refused or was interrupted and gave no verdict.
func run(ctx context.Context, root string, args, environ []string, stdout, stderr io.Writer) int {
	in, err := parseInputs(args, stderr)
	if err == nil {
		var p prepared
		if p, err = prepare(ctx, root, in, environ); err == nil {
			return check(ctx, args, in, p, environ, stdout, stderr)
		}
	}
	if ctx.Err() != nil {
		return interrupted(stderr, "")
	}
	fmt.Fprintf(stderr, "mutcheck: refused, and no run was started: %v\n", err)
	return 2
}

func interrupted(stderr io.Writer, logs string) int {
	msg := "mutcheck: interrupted; the process group of the run in progress was stopped, and there is no verdict"
	if logs != "" {
		msg += "; the logs so far are in " + logs + " (on this machine only)"
	}
	fmt.Fprintln(stderr, msg)
	return 2
}

// check makes the runs of one check and prints its report.
func check(ctx context.Context, args []string, in inputs, p prepared, environ []string, stdout, stderr io.Writer) int {
	logs, err := os.MkdirTemp(p.tmpBase, "semengine-mutcheck-")
	if err != nil {
		fmt.Fprintf(stderr, "mutcheck: the check could not start: %v\n", err)
		return 2
	}
	r := report{args: args, in: in, p: p, logs: logs, bound: 2 * in.timeout}
	failed := func(err error) int {
		if ctx.Err() != nil {
			return interrupted(stderr, logs)
		}
		fmt.Fprintf(stderr, "mutcheck: the check stopped without a verdict: %v; logs: %s (on this machine only)\n", err, logs)
		return 2
	}
	overlay, err := writeOverlay(&r)
	if err != nil {
		return failed(err)
	}
	if r.diff, err = runGit(ctx, p.root, environ, "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--inter-hunk-context=0", "--no-color", "-U0", "--", p.target, r.mutantCopy); err != nil {
		return failed(err)
	}
	if r.hunks, err = parseHunks(r.diff); err != nil {
		return failed(err)
	}
	commit, err := runGit(ctx, p.root, environ, "rev-parse", "HEAD")
	if err != nil {
		return failed(err)
	}
	r.commit = strings.TrimSpace(string(commit))
	status, err := runGit(ctx, p.root, environ, "status", "--porcelain")
	if err != nil {
		return failed(err)
	}
	r.dirty = len(status) > 0
	treeBefore, err := fingerprint(ctx, p.root, environ)
	if err != nil {
		return failed(err)
	}

	mutantFlags := strings.TrimSpace(p.goflags + " " + overlay)
	profile := filepath.Join(logs, "reach.cover")
	coverpkg := "./" + path.Dir(p.target)
	if path.Dir(p.target) == "." {
		coverpkg = "."
	}
	do := func(ctx context.Context, kind runKind, index int) (runRecord, error) {
		rec := runRecord{kind: kind, index: index, goflags: p.goflags, args: testArgs(in)}
		switch kind {
		case mutantRun:
			rec.goflags = mutantFlags
		case reachRun:
			rec.args = append(rec.args, "-coverpkg="+coverpkg, "-coverprofile="+profile)
		}
		rec.args = append(rec.args, in.pkg)
		env := withEnv(environ, "GOFLAGS="+rec.goflags, fmt.Sprintf("RAPID_SEED=%d", in.seed), "RAPID_NOFAILFILE=true")
		label := fmt.Sprintf("%s-%d", kindName(kind), index)
		stream, end, wall, err := goTest(ctx, runSpec{label: label, args: rec.args, env: env, dir: p.root, logs: logs, bound: r.bound})
		if err != nil {
			return rec, err
		}
		rec.end, rec.wall = end, wall
		rec.obs = observe(stream, end, in.test, in.expect)
		rec.reading = read(rec.obs.facts, kind == mutantRun, r.bound)
		return rec, nil
	}
	c, err := sequence(ctx, in.runs, do)
	if err != nil {
		return failed(err)
	}
	c.seed, c.treeBefore = in.seed, treeBefore
	if c.treeAfter, err = fingerprint(ctx, p.root, environ); err != nil {
		if ctx.Err() != nil {
			return interrupted(stderr, logs)
		}
		c.treeErr = err.Error()
	}
	if after, err := os.ReadFile(filepath.Join(p.root, p.target)); err == nil {
		r.targetAfter = sha256Hex(after)
	} else {
		r.targetAfter = "(not read: " + err.Error() + ")"
	}
	if c.reach != nil && c.reach.reading.outcome == pass {
		data, err := os.ReadFile(profile)
		if err == nil {
			c.reachState, r.regions, err = reach(r.hunks, p.targetSrc, data, path.Base(p.target))
		}
		if err != nil {
			c.reachErr = err.Error()
		}
	}
	r.c = c
	r.verdict, r.reason = decide(c)
	writeReport(stdout, r)
	if r.verdict == detection {
		return 0
	}
	return 1
}

// writeOverlay copies the wrong change into the log directory and writes the overlay that maps
// the target to that copy. It returns the overlay's GOFLAGS field.
func writeOverlay(r *report) (string, error) {
	r.mutantCopy = filepath.Join(r.logs, "mutant-"+path.Base(r.p.target))
	if err := os.WriteFile(r.mutantCopy, r.p.mutantSrc, 0o644); err != nil {
		return "", err
	}
	target := filepath.Join(r.p.root, filepath.FromSlash(r.p.target))
	data, err := json.Marshal(map[string]map[string]string{"Replace": {target: r.mutantCopy}})
	if err != nil {
		return "", err
	}
	file := filepath.Join(r.logs, "overlay.json")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return "", err
	}
	return quoteFlag("-overlay=" + file)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
