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
	"io"
	"os"
	"os/signal"
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
	return 2
}
