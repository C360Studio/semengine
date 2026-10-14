package natsfixture

import (
	"context"
	"errors"
	"fmt"
)

// Stop closes the fixture's connection and removes its container, under the caller's context.
// Removing the container removes every stream, bucket and consumer in it, so Stop deletes none of
// them first and needs no answer from the server. Before closing the connection it stops each
// consumer Consume created and waits for its running handlers, then cancels their context. Once
// Stop begins, the fixture refuses to create anything more.
//
// Stop returns nil only when Docker reported the container removed. If ctx ends first, or the
// removal fails, Stop returns the error and keeps what it has not released; a later Stop retries.
// The fixture never substitutes its own timeout for ctx. Stop on a fixture with nothing held
// (never started, a start that rolled back cleanly, or already stopped) returns nil without any
// call.
func (f *Fixture) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("natsfixture: Stop with a nil context")
	}
	if err := f.acquire(ctx, "stop"); err != nil {
		return fmt.Errorf("natsfixture: stop not begun: waiting for a Start or Stop in progress: %w", err)
	}
	defer f.release()
	if len(f.remaining()) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("natsfixture: stop not begun: %w", err)
	}
	f.beginStop()

	f.mu.Lock()
	consumers := append([]*consumer(nil), f.consumers...)
	f.mu.Unlock()
	for _, c := range consumers {
		if err := f.stopConsumer(ctx, c); err != nil {
			return fmt.Errorf("natsfixture: stop consumer %s/%s: %w", c.stream, c.name, err)
		}
	}

	f.mu.Lock()
	nc, c, id := f.nc, f.container, f.containerID
	f.nc, f.js = nil, nil
	f.mu.Unlock()
	if nc != nil {
		nc.Close()
	}
	if c == nil {
		return nil
	}
	err := f.deps.terminate(ctx, c)
	f.count("terminate")
	if err != nil {
		return fmt.Errorf("natsfixture: stop container %s: %w", shortID(id), err)
	}
	f.mu.Lock()
	f.container, f.containerID = nil, ""
	f.mu.Unlock()
	return nil
}

// stopConsumer stops a consumer's delivery, waits for its running handlers to return, and only
// then cancels their context. The consumer itself goes with the container.
func (f *Fixture) stopConsumer(ctx context.Context, c *consumer) error {
	c.stopDelivery()
	select {
	case <-c.idle:
	case <-ctx.Done():
		return ctx.Err()
	}
	c.mu.Lock()
	cc, cancel := c.cc, c.cancel
	c.mu.Unlock()
	if cc != nil {
		select {
		case <-cc.Closed():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if cancel != nil {
		cancel()
	}
	f.mu.Lock()
	for i, held := range f.consumers {
		if held == c {
			f.consumers = append(f.consumers[:i], f.consumers[i+1:]...)
			break
		}
	}
	f.mu.Unlock()
	return nil
}

// remaining lists what the fixture still holds, in Stop's order.
func (f *Fixture) remaining() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for _, c := range f.consumers {
		out = append(out, "consumer "+c.stream+"/"+c.name)
	}
	if f.nc != nil {
		out = append(out, "connection")
	}
	if f.container != nil {
		out = append(out, "container "+shortID(f.containerID))
	}
	return out
}
