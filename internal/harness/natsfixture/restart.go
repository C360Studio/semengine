package natsfixture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	neturl "net/url"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
)

var errNotStarted = errors.New("natsfixture: Restart needs a fixture whose Start succeeded and whose Stop has not begun")

// Restart stops and starts the fixture's own container and reconnects to it, under ctx. The
// container's writable layer survives, so a file-backed stream keeps its messages and a
// memory-backed one does not.
//
// Before the container stops, the fixture's connection is drained, so the JetStream context and
// any stream, bucket or consumer handle taken from it before the restart are dead. Streams and
// buckets stay on the server; Stop's removal of the container removes them.
//
// URL and JetStream return the new binding once Restart returns nil, and stay valid until the next
// Restart. The host port is the one Start gave Docker, and URL has stayed unchanged in every run
// so far (TestRestartURLDials; Docker does not promise it), but every connection to the container
// is lost. A caller stops what it built on the URL before
// Restart and starts it again after.
//
// Restart refuses, with no Docker call, before a successful Start and once Stop has begun. A
// failure in a phase returns an error naming it; nothing is tried again and no replacement
// container is created. After a failure the fixture can no longer be restarted, and Stop removes
// the container.
//
// The caller must bound ctx. Readiness after the start is one more ready line in the container's
// log than before it; if Docker log rotation drops earlier lines the count can fall, and the wait
// then runs until ctx ends and fails at the start-container phase.
func (f *Fixture) Restart(ctx context.Context) error {
	if ctx == nil {
		return errors.New("natsfixture: Restart with a nil context")
	}
	c, id, err := f.restartable()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.restart(ctx, c, id)
}

// restartable reports the container to restart, or why there is none.
func (f *Fixture) restartable() (testcontainers.Container, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.container == nil || f.nc == nil || f.js == nil {
		return nil, "", errNotStarted
	}
	return f.container, f.containerID, nil
}

func (f *Fixture) restart(ctx context.Context, c testcontainers.Container, id string) error {
	f.mu.Lock()
	nc, oldURL := f.nc, f.url
	f.mu.Unlock()

	if err := drainAndObserveClosed(ctx, nc); err != nil {
		return fmt.Errorf("natsfixture: restart: drain connection: %w", err)
	}
	f.mu.Lock()
	f.nc, f.js, f.url = nil, nil, ""
	f.mu.Unlock()

	u, err := neturl.Parse(oldURL)
	if err != nil {
		return fmt.Errorf("natsfixture: restart: host from %q: %w", oldURL, err)
	}
	host := u.Hostname()

	// run times one phase and turns a cancelled parent into that phase's failure even when the
	// operation itself returned success, as Start's phases do.
	run := func(phase string, op func() error) error {
		err := op()
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			return &phaseError{phase: phase, containerID: id, parentErr: ctx.Err(), cause: err}
		}
		return nil
	}
	if err := run("stop-container", func() error {
		// A nil timeout leaves the stop grace period to Docker (the container's StopTimeout, else
		// the engine default); ctx bounds the call.
		return c.Stop(ctx, nil)
	}); err != nil {
		return err
	}
	if err := run("start-container", func() error {
		// The container's log keeps every boot's lines, so the ready log of the last boot is
		// already there: readiness is one more ready line than before the start.
		before, err := readyLines(ctx, c)
		if err != nil {
			return err
		}
		if err := c.Start(ctx); err != nil {
			return err
		}
		// If Docker log rotation drops old lines the count can fall; the wait then runs until ctx ends (loud, not silent).
		_, err = probe.Await(ctx, func(ctx context.Context) (int, error) { return readyLines(ctx, c) },
			func(n int) bool { return n > before })
		return err
	}); err != nil {
		return err
	}
	// The host port is the one Start gave Docker; it is read back rather than assumed, so a lost
	// binding fails here with the inspect output instead of at connect.
	var port string
	if err := run("mapped-port", func() error {
		var err error
		port, err = f.mappedPort(ctx, c)
		return err
	}); err != nil {
		return err
	}
	url := fmt.Sprintf("nats://%s:%s", host, port)
	if err := run("connect", func() error {
		nc, err := connect(ctx, url)
		if nc != nil {
			f.mu.Lock()
			f.nc = nc
			f.mu.Unlock()
		}
		return err
	}); err != nil {
		return err
	}
	return run("jetstream-ready", func() error {
		f.mu.Lock()
		nc := f.nc
		f.mu.Unlock()
		js, err := jetstream.New(nc)
		if err != nil {
			return err
		}
		err = jetStreamReady(ctx, js)
		if err == nil {
			f.mu.Lock()
			f.js, f.url = js, url
			f.mu.Unlock()
		}
		return err
	})
}

// readyLines counts the ready lines in the container's log.
func readyLines(ctx context.Context, c testcontainers.Container) (int, error) {
	logs, err := containerLogs(ctx, c)
	if err != nil {
		return 0, err
	}
	return bytes.Count(logs, []byte(readyLog)), nil
}
