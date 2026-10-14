package natsfixture

import (
	"context"
	"errors"
	"fmt"
)

// Stop closes the fixture's connection and removes its container, under the caller's context.
// Removing the container removes every stream, bucket and consumer in it, so Stop deletes none of
// them first and needs no answer from the server. Once Stop begins the fixture holds no
// connection, so it creates nothing more and Restart refuses.
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
	if len(f.remaining()) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("natsfixture: stop not begun: %w", err)
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

// remaining lists what the fixture still holds, in Stop's order.
func (f *Fixture) remaining() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	if f.nc != nil {
		out = append(out, "connection")
	}
	if f.container != nil {
		out = append(out, "container "+shortID(f.containerID))
	}
	return out
}
