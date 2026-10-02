// Package main is the program behind `task ledger:check` and `task ledger:diff`. It compares
// admission-ledger entries with SemStreams at each entry's source_sha (harness-boundaries ›
// "Comparison with the pin"), fetching the pin into a temporary directory outside the repository.
//
//	pindiff check                   fail when a carry entry differs from the pin
//	pindiff diff [<source_path>...] print the differences of the named entries, or of every
//	                                carry and adapt entry
//
// It reads docs/admission-ledger.yaml and the tree from its working directory. SEMENGINE_PIN_REMOTE
// replaces the SemStreams remote and SEMENGINE_PIN_FETCH_BOUND (a Go duration) the bound shared by
// all the fetches of one run; tests set both to run against a local repository.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	envRemote     = "SEMENGINE_PIN_REMOTE"
	envBound      = "SEMENGINE_PIN_FETCH_BOUND"
	defaultRemote = "https://github.com/c360studio/semstreams.git"
	defaultBound  = 2 * time.Minute
	usage         = "usage: pindiff check | pindiff diff [<source_path>...]"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, ".", os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

type config struct {
	remote string
	bound  time.Duration
}

func configFrom(getenv func(string) string) (config, error) {
	cfg := config{remote: defaultRemote, bound: defaultBound}
	if r := getenv(envRemote); r != "" {
		cfg.remote = r
	}
	if b := getenv(envBound); b != "" {
		d, err := time.ParseDuration(b)
		if err != nil || d <= 0 {
			return cfg, fmt.Errorf("%s=%q is not a positive duration", envBound, b)
		}
		cfg.bound = d
	}
	return cfg, nil
}

// run is the program with its working directory, arguments, environment and output passed in. It
// returns the exit status: 0 when it ran (check: and every carry entry matches), 1 when check
// finds a carry entry that does not match, 2 when it could not do what was asked.
func run(ctx context.Context, root string, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "check" && args[0] != "diff") || (args[0] == "check" && len(args) > 1) {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	prefix := "ledger:" + args[0] + ": "
	cfg, err := configFrom(getenv)
	if err != nil {
		fmt.Fprintln(stderr, prefix+err.Error())
		return 2
	}
	entries, err := readLedger(root)
	if err != nil {
		fmt.Fprintf(stderr, "%s%s could not be read: %v\n", prefix, ledgerPath, err)
		return 2
	}
	if args[0] == "check" {
		return runCheck(ctx, root, cfg, entries, stderr)
	}
	return runDiff(ctx, root, cfg, entries, args[1:], stdout, stderr)
}

// runCheck compares every carry entry with the pin (harness-boundaries › "Carried entries match
// the pin"). Other dispositions are not compared. All its output goes to standard error.
func runCheck(ctx context.Context, root string, cfg config, entries []entry, stderr io.Writer) int {
	var carried []entry
	for _, e := range entries {
		if e.Disposition == "carry" {
			carried = append(carried, e)
		}
	}
	if len(carried) == 0 {
		fmt.Fprintln(stderr, "ledger:check: no carry entry; the pin was not fetched")
		return 0
	}
	results, problems := compareAll(ctx, root, cfg, entries, carried)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(stderr, "ledger:check: the pin could not be read: "+p)
		}
		fmt.Fprintln(stderr, "ledger:check: no entry was checked")
		return 2
	}
	var failed []string
	for _, r := range results {
		fmt.Fprintln(stderr, summary(r))
		label := "carry entry " + r.entry.SourcePath + ": "
		failures := 0
		if r.notCompared != "" {
			fmt.Fprintln(stderr, label+"not compared: "+r.notCompared)
			failures++
		}
		for _, f := range r.files {
			switch f.outcome {
			case fileDiffers:
				fmt.Fprintln(stderr, label+f.treePath+": differs from the pin")
			case fileOnlyAtPin:
				fmt.Fprintln(stderr, label+"pin/"+f.pinPath+": only at the pin")
			case fileOnlyInTree:
				fmt.Fprintln(stderr, label+f.treePath+": only in the tree")
			default:
				continue
			}
			failures++
		}
		if failures > 0 {
			failed = append(failed, r.entry.SourcePath)
		}
	}
	if len(failed) == 0 {
		fmt.Fprintf(stderr, "ledger:check: %d carry entries match the pin\n", len(carried))
		return 0
	}
	fmt.Fprintf(stderr, "ledger:check: %d of %d carry entries do not match the pin\n", len(failed), len(carried))
	for _, p := range failed {
		fmt.Fprintf(stderr, "ledger:check: `task ledger:diff -- %s` prints the lines\n", p)
	}
	fmt.Fprintln(stderr, "ledger:check: a package that differs from the pin is adapt, not carry (docs/provenance.md rule 5)")
	return 1
}

func runDiff(ctx context.Context, root string, cfg config, entries []entry, names []string, stdout, stderr io.Writer) int {
	var selected []entry
	if len(names) == 0 {
		for _, e := range entries {
			if e.Disposition == "carry" || e.Disposition == "adapt" {
				selected = append(selected, e)
			}
		}
	}
	unknown := 0
	for _, name := range names {
		i := indexOf(entries, name)
		if i < 0 {
			fmt.Fprintf(stderr, "ledger:diff: no ledger entry has source_path %q\n", name)
			unknown++
			continue
		}
		selected = append(selected, entries[i])
	}
	if unknown > 0 {
		return 2
	}
	if len(selected) == 0 {
		fmt.Fprintln(stderr, "ledger:diff: no carry or adapt entry; nothing was compared")
		return 0
	}
	results, problems := compareAll(ctx, root, cfg, entries, selected)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(stderr, "ledger:diff: the pin could not be read: "+p)
		}
		fmt.Fprintln(stderr, "ledger:diff: no entry was compared")
		return 2
	}
	notCompared := 0
	for _, r := range results {
		for _, f := range r.files {
			switch f.outcome {
			case fileDiffers:
				fmt.Fprint(stdout, unifiedDiff("pin/"+f.pinPath, f.treePath, f.pinText, f.treeText))
			case fileOnlyAtPin:
				fmt.Fprintln(stdout, "only at the pin: pin/"+f.pinPath)
			case fileOnlyInTree:
				fmt.Fprintln(stdout, "only in the tree: "+f.treePath)
			}
		}
		fmt.Fprintln(stderr, summary(r))
		if r.notCompared != "" {
			notCompared++
		}
	}
	if len(names) > 0 && notCompared > 0 {
		fmt.Fprintf(stderr, "ledger:diff: %d named entries were not compared\n", notCompared)
		return 2
	}
	return 0
}

func indexOf(entries []entry, sourcePath string) int {
	for i, e := range entries {
		if e.SourcePath == sourcePath {
			return i
		}
	}
	return -1
}

// summary is an entry's line on standard error.
func summary(r entryResult) string {
	if r.notCompared != "" {
		return fmt.Sprintf("%s (%s): not compared: %s", r.entry.SourcePath, r.entry.Disposition, r.notCompared)
	}
	both, differ, onlyPin, onlyTree := r.counts()
	return fmt.Sprintf("%s (%s): %d files, %d differ, %d only at the pin, %d only in the tree",
		r.entry.SourcePath, r.entry.Disposition, both, differ, onlyPin, onlyTree)
}

// compareAll fetches the pins the selected entries need and compares each entry. Problems mean
// the pin could not be read, and then no result is returned.
func compareAll(ctx context.Context, root string, cfg config, entries, selected []entry) ([]entryResult, []string) {
	var shas []string
	for _, e := range selected {
		if !contains(shas, e.SourceSHA) {
			shas = append(shas, e.SourceSHA)
		}
	}
	tmp, err := os.MkdirTemp("", "semengine-pin-")
	if err != nil {
		return nil, []string{err.Error()}
	}
	defer os.RemoveAll(tmp)
	store, problems := fetchPins(ctx, cfg.remote, cfg.bound, tmp, shas)
	if len(problems) > 0 {
		return nil, problems
	}
	movedBySHA := map[string]map[string]string{}
	var results []entryResult
	for _, e := range selected {
		moved, ok := movedBySHA[e.SourceSHA]
		if !ok {
			isDir := func(p string) (bool, error) {
				o, err := store.stat(ctx, e.SourceSHA, p)
				return o.kind == "tree", err
			}
			if moved, err = movedPackages(root, entries, e.SourceSHA, isDir); err != nil {
				return nil, []string{err.Error()}
			}
			movedBySHA[e.SourceSHA] = moved
		}
		r, err := compareEntry(ctx, root, store, e, moved)
		if err != nil {
			return nil, []string{err.Error()}
		}
		results = append(results, r)
	}
	return results, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
