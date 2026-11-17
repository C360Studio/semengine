// Package main is the program behind `task scorecard:tier1`: it measures tier 1 search on a real
// embedding provider. There is no other committed way to: task verify must reach no provider,
// and a test may not skip when none is there.
//
//	task -x scorecard:tier1 -- -url http://127.0.0.1:18081/v1 -model Snowflake/snowflake-arctic-embed-s \
//	  -query-prefix 'Represent this sentence for searching relevant passages: ' -provider 'semembed <commit>'
//
// It starts NATS from the one image pin, composes graph-ingest, graph-index, graph-lexical,
// graph-embedding over the provider (section bodies only) and graph-query at tier 1, writes the
// word scorecard's 2,000 sections, and asks its 500 questions through globalSearch, the semantic
// verb and searchGraph. It prints R@1, R@10 and MRR for each, with the model, prefix, commit and
// timings, then the verdict against tier 1's floor (floor.go). It exits 0 when the merged row holds
// the floor, 1 when it does not, and 2 on any other failure. Task runs it built, not through go run,
// which reports every failing status as 1; Task's -x passes the status on, and without -x Task
// reports any failure as 201.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run is the program with its arguments, environment and output passed in; it returns the exit
// status.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, getenv, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "scorecard: refused, and nothing was started: %v\n", err)
		return 2
	}
	tb := &programTB{out: stderr}
	r, err := measure(ctx, opts, tb, stdout)
	code := 2
	if err == nil {
		code = report(r, stdout)
	} else {
		fmt.Fprintf(stderr, "scorecard: no verdict: %v\n", err)
	}
	tb.runCleanups()
	if tb.failed {
		fmt.Fprintln(stderr, "scorecard: a cleanup failed, so the run exits 2")
		return 2
	}
	return code
}

// report prints the three rows and the verdict, and returns 0 when the floor holds, else 1.
func report(r rows, out io.Writer) int {
	for _, row := range []*tally{r.words, r.vectors, r.merged} {
		fmt.Fprintln(out, row)
	}
	fmt.Fprintf(out, "floor: merged R@10 at least %.1f%%, MRR at least %.3f, R@10 not below either single row\n",
		100*floorRecallAt10, floorMRR)
	short := verdict(r, floorRecallAt10, floorMRR)
	if len(short) == 0 {
		fmt.Fprintln(out, "verdict: holds tier 1's floor")
		return 0
	}
	for _, s := range short {
		fmt.Fprintln(out, "verdict: below the floor:", s)
	}
	return 1
}
