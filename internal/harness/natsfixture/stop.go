package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Stop finalises the fixture under the caller's context, in order: wait for in-flight async
// publishes; for each consumer stop delivery, join its running handlers, then cancel their context
// and delete it; record each stream's message count, then delete streams and buckets; drain and
// close the connection; terminate the container. Each resource leaves the fixture's ownership only
// once observed absent, so a nil return means everything is gone. Once Stop begins, the fixture
// refuses to create anything more.
//
// When the connection is already closed (the broker went away), the handler joins and cancels
// still happen; only the server-side deletes are skipped, and the consumers, streams, and buckets
// are released with the container that held them.
//
// If ctx ends first, Stop returns an error wrapping ctx.Err() and keeps every unresolved handle; a
// later Stop retries them. The fixture never substitutes its own timeout for ctx. Stop on a fixture
// with nothing owned (never started, a start that rolled back cleanly, or already stopped) returns
// nil without any call.
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
	sr := stopRecord{}
	err := f.stop(ctx, &sr)
	sr.Result = "ok"
	if err != nil {
		sr.Result = err.Error()
	}
	f.mu.Lock()
	f.rec.Stops = append(f.rec.Stops, sr)
	f.mu.Unlock()
	f.writeEvidence()
	return err
}

func (f *Fixture) stop(ctx context.Context, sr *stopRecord) error {
	step := func(name string, op func() error) error {
		started := time.Now()
		err := op()
		sr.Phases = append(sr.Phases, phaseRecord{Phase: name, ElapsedMS: since(started), Error: errString(err)})
		if err != nil {
			return fmt.Errorf("natsfixture: stop %s: %w", name, err)
		}
		return nil
	}
	f.mu.Lock()
	js, nc := f.js, f.nc
	consumers := append([]*consumer(nil), f.consumers...)
	streams := append([]string(nil), f.streams...)
	buckets := append([]string(nil), f.buckets...)
	f.mu.Unlock()

	// A connection that is already closed (the broker died) cannot delete anything; the
	// container's removal is then what removes the NATS resources, and they are released with it.
	// Handlers run in this process either way, so every consumer is still stopped and joined.
	natsUsable := js != nil && nc != nil && !nc.IsClosed()
	if natsUsable {
		if err := step("publish-complete", func() error {
			select {
			case <-js.PublishAsyncComplete():
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}); err != nil {
			return err
		}
	}
	for _, c := range consumers {
		if err := step("consumer "+c.stream+"/"+c.name, func() error { return f.stopConsumer(ctx, js, c, natsUsable) }); err != nil {
			return err
		}
	}
	if natsUsable {
		for _, name := range streams {
			if err := step("stream "+name, func() error { return f.deleteStream(ctx, js, name) }); err != nil {
				return err
			}
		}
		for _, b := range buckets {
			if err := step("bucket "+b, func() error { return f.deleteBucket(ctx, js, b) }); err != nil {
				return err
			}
		}
	}
	if nc != nil {
		if err := step("connection", func() error {
			err := f.deps.drain(ctx, nc)
			f.count("drain")
			if err != nil {
				return err
			}
			f.mu.Lock()
			f.nc, f.js = nil, nil
			f.mu.Unlock()
			return nil
		}); err != nil {
			return err
		}
	}
	f.mu.Lock()
	c, id := f.container, f.containerID
	f.mu.Unlock()
	if c != nil {
		if err := step("container", func() error {
			err := f.deps.terminate(ctx, c)
			f.count("terminate")
			if err != nil {
				return err
			}
			return f.observeAbsent(ctx, id)
		}); err != nil {
			return err
		}
		f.mu.Lock()
		f.container, f.containerID = nil, ""
		if !natsUsable {
			// Released with the container that held them.
			f.consumers, f.streams, f.buckets = nil, nil, nil
		}
		f.mu.Unlock()
	}
	// Nothing may be created once Stop begins, so this holds by construction; it is checked
	// anyway, because a nil return is the caller's only evidence that nothing is left.
	if rem := f.remaining(); len(rem) > 0 {
		return fmt.Errorf("natsfixture: stop finished its steps but still owns %v", rem)
	}
	return nil
}

// stopConsumer joins a consumer's handlers before ending their context and, when the broker can
// still be reached, deleting it. Without a usable connection the consumer stays owned until the
// container that holds it is observed absent.
func (f *Fixture) stopConsumer(ctx context.Context, js jetstream.JetStream, c *consumer, natsUsable bool) error {
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
	if !natsUsable {
		return nil
	}
	s, err := js.Stream(ctx, c.stream)
	switch {
	case errors.Is(err, jetstream.ErrStreamNotFound):
		// The stream is gone, so its consumers are.
	case err != nil:
		return err
	default:
		if err := s.DeleteConsumer(ctx, c.name); err != nil && !errors.Is(err, jetstream.ErrConsumerNotFound) {
			return err
		}
		if _, err := s.Consumer(ctx, c.name); !errors.Is(err, jetstream.ErrConsumerNotFound) {
			return absenceError("consumer "+c.name, err)
		}
	}
	f.mu.Lock()
	for i, owned := range f.consumers {
		if owned == c {
			f.consumers = append(f.consumers[:i], f.consumers[i+1:]...)
			break
		}
	}
	f.mu.Unlock()
	return nil
}

func (f *Fixture) deleteStream(ctx context.Context, js jetstream.JetStream, name string) error {
	// The message count is recorded before deletion, so evidence shows what the stream held when
	// finalisation reached it (graceful-finalisation proof, S1-4).
	if s, err := js.Stream(ctx, name); err == nil {
		if info, err := s.Info(ctx); err == nil {
			f.mu.Lock()
			if f.rec.StreamMsgs == nil {
				f.rec.StreamMsgs = map[string]uint64{}
			}
			f.rec.StreamMsgs[name] = info.State.Msgs
			f.mu.Unlock()
		}
	}
	if err := js.DeleteStream(ctx, name); err != nil && !errors.Is(err, jetstream.ErrStreamNotFound) {
		return err
	}
	if _, err := js.Stream(ctx, name); !errors.Is(err, jetstream.ErrStreamNotFound) {
		return absenceError("stream "+name, err)
	}
	f.mu.Lock()
	f.streams = removeName(f.streams, name)
	f.mu.Unlock()
	return nil
}

func (f *Fixture) deleteBucket(ctx context.Context, js jetstream.JetStream, bucket string) error {
	if err := js.DeleteKeyValue(ctx, bucket); err != nil && !errors.Is(err, jetstream.ErrBucketNotFound) {
		return err
	}
	if _, err := js.KeyValue(ctx, bucket); !errors.Is(err, jetstream.ErrBucketNotFound) {
		return absenceError("bucket "+bucket, err)
	}
	f.mu.Lock()
	f.buckets = removeName(f.buckets, bucket)
	f.mu.Unlock()
	return nil
}

// observeAbsent asks the daemon whether the container is gone.
func (f *Fixture) observeAbsent(ctx context.Context, id string) error {
	gone, err := f.deps.absent(ctx, id)
	f.count("absent")
	if err != nil {
		return err
	}
	if !gone {
		return fmt.Errorf("container %s still present after terminate", shortID(id))
	}
	return nil
}

// absenceError reports a resource not observed absent after its deletion.
func absenceError(what string, err error) error {
	if err == nil {
		return fmt.Errorf("%s still present after delete", what)
	}
	return fmt.Errorf("%s: absence not observed: %w", what, err)
}

func removeName(list []string, name string) []string {
	out := list[:0]
	for _, n := range list {
		if n != name {
			out = append(out, n)
		}
	}
	return out
}

// remaining lists what the fixture still owns, in Stop's order.
func (f *Fixture) remaining() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.remainingLocked()
}

func (f *Fixture) remainingLocked() []string {
	out := []string{}
	for _, c := range f.consumers {
		out = append(out, "consumer "+c.stream+"/"+c.name)
	}
	for _, s := range f.streams {
		out = append(out, "stream "+s)
	}
	for _, b := range f.buckets {
		out = append(out, "bucket "+b)
	}
	if f.nc != nil {
		out = append(out, "connection")
	}
	if f.container != nil {
		out = append(out, "container "+shortID(f.containerID))
	}
	return out
}
