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

var errNotStarted = errors.New("natsfixture: Restart needs a fixture whose Start succeeded")

// Restart stops and starts the fixture's own container and reconnects to it, under ctx. The
// container's writable layer survives, so a file-backed stream keeps its messages and a
// memory-backed one does not.
//
// Before the container stops, every consumer Consume created is ended as Stop ends it: delivery
// stops, Restart waits for its running handlers, then cancels their context. Restart does not
// re-create consumers; a test that needs one afterwards calls Consume again. The fixture's
// connection is then drained, so the JetStream context and any stream, bucket or consumer handle
// taken from it before the restart are dead. Streams and buckets stay owned, and Stop removes them
// or observes them absent.
//
// URL and JetStream return the new binding once Restart returns nil, and stay valid until the next
// Restart; Docker may map a different host port. A caller stops what it built on the old URL before
// Restart and starts it on the new one after.
//
// Restart refuses, with no Docker call, before a successful Start and once Stop has begun. A
// failure in a phase returns an *Error naming it; no replacement container is created, the fixture
// can no longer be restarted, and Stop removes what it still owns. Do not call Restart from a
// Consume handler: it waits for that handler to return.
//
// The caller must bound ctx. Readiness after the start is one more ready line in the container's
// log than before it; if Docker log rotation drops earlier lines the count can fall, and the wait
// then runs until ctx ends and fails at the start-container phase.
func (f *Fixture) Restart(ctx context.Context) error {
	if ctx == nil {
		return errors.New("natsfixture: Restart with a nil context")
	}
	if _, _, err := f.restartable(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := f.acquire(ctx, "restart"); err != nil {
		return err
	}
	defer f.release()
	c, id, err := f.restartable() // Stop may have begun and ended while this waited
	if err != nil {
		return err
	}
	f.count("restart")
	err = f.restart(ctx, c, id)
	f.writeEvidence()
	return err
}

// restartable reports the container to restart, or why there is none.
func (f *Fixture) restartable() (testcontainers.Container, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopping {
		return nil, "", errStopping
	}
	if f.container == nil || f.nc == nil || f.js == nil {
		return nil, "", errNotStarted
	}
	return f.container, f.containerID, nil
}

func (f *Fixture) restart(ctx context.Context, c testcontainers.Container, id string) error {
	f.mu.Lock()
	consumers := append([]*consumer(nil), f.consumers...)
	nc, oldURL := f.nc, f.url
	f.mu.Unlock()

	// Consumers are joined while the connection is still up, so a handler's ack still lands. They
	// stay owned: the durable consumer may outlive the restart on a file-backed stream, and Stop
	// deletes it, or observes it absent, over the new connection.
	for _, cs := range consumers {
		if err := f.stopConsumer(ctx, nil, cs, false); err != nil {
			return fmt.Errorf("natsfixture: restart: end consumer %s/%s: %w", cs.stream, cs.name, err)
		}
	}
	err := f.deps.drain(ctx, nc)
	f.count("drain")
	if err != nil {
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
	run := func(p Phase, op func() error) error {
		err := op()
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			return &Error{Attempt: 1, Phase: p, ContainerID: id, ParentErr: ctx.Err(), Cause: err}
		}
		return nil
	}
	if err := run(PhaseStopContainer, func() error {
		err := f.deps.stopContainer(ctx, c)
		f.count("stopContainer")
		return err
	}); err != nil {
		return err
	}
	if err := run(PhaseStartContainer, func() error {
		// The container's log keeps every boot's lines, so the ready log of the last boot is
		// already there: readiness is one more ready line than before the start.
		before, err := f.readyLines(ctx, c)
		if err != nil {
			return err
		}
		err = f.deps.startContainer(ctx, c)
		f.count("startContainer")
		if err != nil {
			return err
		}
		// If Docker log rotation drops old lines the count can fall; the wait then runs until ctx ends (loud, not silent).
		_, err = probe.Await(ctx, func(ctx context.Context) (int, error) { return f.readyLines(ctx, c) },
			func(n int) bool { return n > before })
		return err
	}); err != nil {
		return err
	}
	var port string
	if err := run(PhaseMappedPort, func() error {
		var err error
		port, err = f.deps.mappedPort(ctx, c)
		f.count("mappedPort")
		return err
	}); err != nil {
		return err
	}
	url := fmt.Sprintf("nats://%s:%s", host, port)
	if err := run(PhaseConnect, func() error {
		nc, err := f.deps.connect(ctx, url)
		f.count("connect")
		if nc != nil {
			f.mu.Lock()
			f.nc = nc
			f.mu.Unlock()
		}
		return err
	}); err != nil {
		return err
	}
	return run(PhaseJetStream, func() error {
		f.mu.Lock()
		nc := f.nc
		f.mu.Unlock()
		js, err := jetstream.New(nc)
		if err != nil {
			return err
		}
		err = f.deps.jsReady(ctx, js)
		f.count("jsReady")
		if err == nil {
			f.mu.Lock()
			f.js, f.url, f.rec.MappedPort = js, url, port
			f.mu.Unlock()
		}
		return err
	})
}

// readyLines counts the ready lines in the container's log.
func (f *Fixture) readyLines(ctx context.Context, c testcontainers.Container) (int, error) {
	logs, err := f.deps.logs(ctx, c)
	f.count("logs")
	if err != nil {
		return 0, err
	}
	return bytes.Count(logs, []byte(readyLog)), nil
}
