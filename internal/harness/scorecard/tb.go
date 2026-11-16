package main

import (
	"fmt"
	"io"
)

// programTB is the natsfixture.TB of a program: it keeps the cleanups to run when the program
// ends, last registered first, and an Errorf marks the run failed. Only the main goroutine
// calls it.
type programTB struct {
	out      io.Writer
	cleanups []func()
	failed   bool
}

func (p *programTB) Helper() {}

func (p *programTB) Errorf(format string, args ...any) {
	p.failed = true
	fmt.Fprintf(p.out, "scorecard: "+format+"\n", args...)
}

func (p *programTB) Logf(format string, args ...any) {
	fmt.Fprintf(p.out, "scorecard: "+format+"\n", args...)
}

func (p *programTB) Cleanup(f func()) { p.cleanups = append(p.cleanups, f) }

func (p *programTB) runCleanups() {
	for i := len(p.cleanups) - 1; i >= 0; i-- {
		p.cleanups[i]()
	}
	p.cleanups = nil
}
