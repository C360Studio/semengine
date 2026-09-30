// Package natsfixture gives one test one disposable, admitted, real NATS server in a container
// (openspec change setup-02-isolated-harness, capability nats-fixture). The pattern is adapted from
// SemStreams natsclient/test_client.go at 5457b345 (ledger row L1) over nats.go and
// testcontainers-go directly, without its eleven caller knobs (ledger row L2).
//
// Start succeeds only after JetStream answers; Stop succeeds only after it has observed every
// owned consumer, stream, bucket, the connection, and the container absent. Test-only: contract
// test T-B1 refuses any production import.
package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// maxAttempts allows one replacement container, and only after a mapped-port failure with a
	// live parent and a clean rollback: the one transient SemStreams observed under Docker API
	// pressure (gh#736). Every other failure is reported, not retried.
	maxAttempts = 2
	// cleanupBudget is the fresh finite authority New's t.Cleanup gives Stop. The test's own
	// context is already cancelled when cleanups run.
	cleanupBudget = 60 * time.Second
	// Stream and bucket bounds: every resource the fixture creates declares them, so a runaway
	// test fails on a ceiling instead of filling the broker (SemStreams test_client.go:911-927).
	resourceMaxAge   = time.Hour
	resourceMaxBytes = 64 << 20
)

// Fixture is one test's NATS server and the resources it creates on it. It holds no
// context.Context: callback contexts live in the goroutines nats.go runs, and the fixture keeps
// only their cancel functions.
type Fixture struct {
	testName string
	errorf   func(format string, args ...any)
	deps     deps

	op sync.Mutex // serialises Start and Stop
	mu sync.Mutex // guards everything below; never held across a Docker or NATS call

	used        bool
	adm         admission
	container   testcontainers.Container
	containerID string
	url         string
	nc          *nats.Conn
	js          jetstream.JetStream
	consumers   []*consumer
	streams     []string
	buckets     []string
	calls       map[string]int
	rec         record
}

// New binds a fixture to its test and registers a Stop, under fresh bounded authority, as the
// test's cleanup. It makes no Docker call.
func New(t testing.TB) *Fixture {
	t.Helper()
	f := &Fixture{testName: t.Name(), errorf: t.Errorf, deps: defaultDeps(), calls: map[string]int{}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupBudget)
		defer cancel()
		if err := f.Stop(ctx); err != nil {
			t.Errorf("natsfixture: cleanup Stop: %v; still held: %v", err, f.remaining())
		}
	})
	return f
}

// Start admits the test, starts one NATS container with JetStream, connects, and returns once
// JetStream answers. Every operation runs under ctx. A failure returns an *Error per attempt,
// after removing whatever the attempt created. A second Start returns ErrAlreadyUsed.
func (f *Fixture) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("natsfixture: Start with a nil context")
	}
	// Refusals before any action consume nothing: the fixture can still be started.
	if err := ctx.Err(); err != nil {
		return err
	}
	f.op.Lock()
	defer f.op.Unlock()
	f.mu.Lock()
	used := f.used
	f.mu.Unlock()
	if used {
		return ErrAlreadyUsed
	}
	adm, err := admit()
	if err != nil {
		return err
	}
	name := f.Name("fixture")
	f.mu.Lock()
	f.used, f.adm = true, adm
	f.rec = record{Test: f.testName, Name: name, Image: adm.image, Owned: []string{}, Remaining: []string{}}
	f.mu.Unlock()
	// The session id must be on disk before any container exists, or the runner's leak check
	// could not see what this process leaves behind.
	sid, err := recordSession(adm.evidenceDir)
	f.mu.Lock()
	f.rec.SessionID = sid
	f.mu.Unlock()
	if err != nil {
		return fmt.Errorf("natsfixture: record testcontainers session before starting: %w", err)
	}

	var failures []error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := f.attempt(ctx, attempt)
		if err == nil {
			f.writeEvidence()
			return nil
		}
		failures = append(failures, err)
		var fe *Error
		if !errors.As(err, &fe) || fe.Phase != PhaseMappedPort || fe.ParentErr != nil || fe.Cleanup != nil {
			break
		}
	}
	f.writeEvidence()
	if len(failures) == 1 {
		return failures[0]
	}
	return errors.Join(failures...)
}

// attempt runs the Start phases once. On failure it rolls back what it created and returns *Error.
func (f *Fixture) attempt(ctx context.Context, n int) error {
	ar := attemptRecord{Attempt: n}
	defer func() {
		f.mu.Lock()
		f.rec.Attempts = append(f.rec.Attempts, ar)
		f.mu.Unlock()
	}()
	phase := func(p Phase, started time.Time, err error) {
		ar.Phases = append(ar.Phases, phaseRecord{Phase: string(p), ElapsedMS: since(started), Error: errString(err)})
	}
	fail := func(p Phase, cause error) error {
		f.mu.Lock()
		id := f.containerID
		f.mu.Unlock()
		fe := &Error{Attempt: n, Phase: p, ContainerID: id, ParentErr: ctx.Err(), Cause: cause}
		ar.Logs, fe.Cleanup = f.rollbackAttempt(ctx, n)
		ar.Container, ar.Error, ar.ParentErr, ar.Cleanup = id, errString(cause), errString(fe.ParentErr), errString(fe.Cleanup)
		return fe
	}
	// run times one phase and turns a cancelled parent into that phase's failure even when the
	// operation itself returned success: work finished after cancellation is not accepted.
	run := func(p Phase, op func() error) error {
		started := time.Now()
		err := op()
		if err == nil {
			err = ctx.Err()
		}
		phase(p, started, err)
		if err != nil {
			return fail(p, err)
		}
		return nil
	}

	f.mu.Lock()
	image := f.adm.image
	f.mu.Unlock()
	if err := run(PhaseImage, func() error {
		if !digestImage.MatchString(image) {
			return fmt.Errorf("SEMENGINE_NATS_IMAGE %q is not a digest reference", image)
		}
		return nil
	}); err != nil {
		return err
	}

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			Name:         f.Name("nats"),
			ExposedPorts: []string{clientPort},
			// The image's entrypoint prefixes nats-server to flag arguments. JetStream stores in
			// the container's own filesystem: the fixture creates no volume.
			Cmd:        []string{"--port", "4222", "--js"},
			WaitingFor: wait.ForLog("Server is ready"),
		},
		Started: true,
	}
	var c testcontainers.Container
	if err := run(PhaseStart, func() error {
		var err error
		c, err = f.deps.start(ctx, req)
		f.count("start")
		// GenericContainer can return a live container together with its error (generic.go:89,94);
		// it is owned from this moment either way.
		if c != nil {
			f.mu.Lock()
			f.container, f.containerID = c, c.GetContainerID()
			f.mu.Unlock()
		}
		return err
	}); err != nil {
		return err
	}

	var host, port string
	if err := run(PhaseHost, func() error {
		var err error
		host, err = f.deps.host(ctx, c)
		f.count("host")
		return err
	}); err != nil {
		return err
	}
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
	if err := run(PhaseJetStream, func() error {
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
			f.js = js
			f.mu.Unlock()
		}
		return err
	}); err != nil {
		return err
	}

	f.mu.Lock()
	f.url = url
	f.rec.Container, f.rec.MappedPort = f.containerID, port
	f.mu.Unlock()
	return nil
}

// rollbackAttempt removes what a failed attempt created, each operation under its own fresh
// bounded context, and records the container's log first. It returns the log file name and a nil
// error only when the container has been observed absent; otherwise the handles stay owned for a
// later Stop.
func (f *Fixture) rollbackAttempt(ctx context.Context, n int) (string, error) {
	f.mu.Lock()
	nc, c, id, dir, name := f.nc, f.container, f.containerID, f.adm.evidenceDir, f.rec.Name
	f.nc = nil
	f.mu.Unlock()
	if nc != nil {
		nc.Close() // nothing of the test's is in flight on a connection Start never returned
	}
	if c == nil {
		return "", nil
	}
	logsFile := ""
	logErr := rollback(ctx, func(rctx context.Context) error {
		logs, err := f.deps.logs(rctx, c)
		f.count("logs")
		if len(logs) > 0 {
			var werr error
			logsFile, werr = writeLogs(dir, name, n, logs)
			err = errors.Join(err, werr)
		}
		return err
	})
	if logErr != nil {
		f.report("natsfixture: container log for attempt %d not captured: %v", n, logErr)
	}
	err := rollback(ctx, func(rctx context.Context) error {
		err := f.deps.terminate(rctx, c)
		f.count("terminate")
		return err
	})
	if err == nil {
		err = rollback(ctx, func(rctx context.Context) error { return f.observeAbsent(rctx, id) })
	}
	if err != nil {
		return logsFile, fmt.Errorf("container %s: %w", shortID(id), err)
	}
	f.mu.Lock()
	f.container, f.containerID = nil, ""
	f.mu.Unlock()
	return logsFile, nil
}

// URL is the broker's client URL on a Docker-assigned host port.
func (f *Fixture) URL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.url
}

// Conn is the fixture's own connection; Stop drains and closes it. Tests needing a second
// connection dial URL and close it themselves.
func (f *Fixture) Conn() *nats.Conn {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nc
}

// JetStream is the JetStream context on Conn. Stop waits for its in-flight async publishes.
func (f *Fixture) JetStream() jetstream.JetStream {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.js
}

func (f *Fixture) started() (jetstream.JetStream, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.js == nil {
		return nil, "", errors.New("natsfixture: fixture not started")
	}
	return f.js, f.containerID, nil
}

// CreateStream creates a stream the fixture owns, with the fixture's MaxAge, MaxBytes, and
// DiscardOld bounds. Stop deletes it and observes it absent.
func (f *Fixture) CreateStream(ctx context.Context, name string, subjects ...string) (jetstream.Stream, error) {
	if ctx == nil {
		return nil, errors.New("natsfixture: CreateStream with a nil context")
	}
	js, id, err := f.started()
	if err != nil {
		return nil, err
	}
	// Owned before the call: a create that times out may still have happened, and Stop treats a
	// stream that turns out not to exist as absent.
	f.own(&f.streams, name, "stream "+name)
	s, err := f.deps.createStream(ctx, js, jetstream.StreamConfig{
		Name: name, Subjects: subjects, MaxAge: resourceMaxAge, MaxBytes: resourceMaxBytes, Discard: jetstream.DiscardOld,
	})
	f.count("createStream")
	if err != nil {
		return nil, &Error{Attempt: 1, Phase: PhaseCreateStream, ContainerID: id, ParentErr: ctx.Err(), Cause: err}
	}
	return s, nil
}

// CreateKeyValue creates a bucket the fixture owns, bounded like a stream (TTL as MaxAge).
func (f *Fixture) CreateKeyValue(ctx context.Context, bucket string) (jetstream.KeyValue, error) {
	if ctx == nil {
		return nil, errors.New("natsfixture: CreateKeyValue with a nil context")
	}
	js, id, err := f.started()
	if err != nil {
		return nil, err
	}
	f.own(&f.buckets, bucket, "bucket "+bucket)
	kv, err := f.deps.createKV(ctx, js, jetstream.KeyValueConfig{Bucket: bucket, TTL: resourceMaxAge, MaxBytes: resourceMaxBytes})
	f.count("createKV")
	if err != nil {
		return nil, &Error{Attempt: 1, Phase: PhaseCreateKV, ContainerID: id, ParentErr: ctx.Err(), Cause: err}
	}
	return kv, nil
}

// Consume creates a durable consumer named Name(base) on stream and delivers each message to
// handler with a context derived from ctx. Stop stops delivery, waits for every running handler to
// return, and only then cancels that context and deletes the consumer: a callback's context stays
// live until its handler has been joined.
func (f *Fixture) Consume(ctx context.Context, stream, base string, handler func(context.Context, jetstream.Msg)) (jetstream.Consumer, error) {
	if ctx == nil || handler == nil {
		return nil, errors.New("natsfixture: Consume needs a context and a handler")
	}
	js, id, err := f.started()
	if err != nil {
		return nil, err
	}
	c := &consumer{stream: stream, name: f.Name(base), idle: make(chan struct{})}
	f.mu.Lock()
	f.consumers = append(f.consumers, c)
	f.rec.Owned = append(f.rec.Owned, "consumer "+stream+"/"+c.name)
	f.mu.Unlock()
	fail := func(err error) error {
		return &Error{Attempt: 1, Phase: PhaseConsume, ContainerID: id, ParentErr: ctx.Err(), Cause: err}
	}
	s, err := js.Stream(ctx, stream)
	if err != nil {
		return nil, fail(err)
	}
	jc, err := s.CreateConsumer(ctx, jetstream.ConsumerConfig{Durable: c.name, AckPolicy: jetstream.AckExplicitPolicy})
	if err != nil {
		return nil, fail(err)
	}
	cbCtx, cancel := context.WithCancel(ctx)
	cc, err := jc.Consume(c.wrap(cbCtx, handler))
	if err != nil {
		cancel()
		return nil, fail(err)
	}
	c.mu.Lock()
	c.cc, c.cancel = cc, cancel
	c.mu.Unlock()
	return jc, nil
}

// consumer is one owned consumer and the join state of its handlers.
type consumer struct {
	stream, name string

	mu       sync.Mutex
	cc       jetstream.ConsumeContext
	cancel   context.CancelFunc
	inflight int
	stopping bool
	idle     chan struct{} // closed once stopping and no handler is running
	idleOnce sync.Once
}

// wrap admits a handler call only until Stop begins and counts it until it returns.
func (c *consumer) wrap(ctx context.Context, handler func(context.Context, jetstream.Msg)) jetstream.MessageHandler {
	return func(msg jetstream.Msg) {
		c.mu.Lock()
		if c.stopping {
			c.mu.Unlock()
			return // delivered after Stop began: left unacked, deleted with the consumer
		}
		c.inflight++
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			c.inflight--
			if c.stopping && c.inflight == 0 {
				c.idleOnce.Do(func() { close(c.idle) })
			}
			c.mu.Unlock()
		}()
		handler(ctx, msg)
	}
}

// stopDelivery stops new deliveries; idle closes when the last running handler returns.
func (c *consumer) stopDelivery() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopping {
		return
	}
	c.stopping = true
	if c.cc != nil {
		c.cc.Stop()
	}
	if c.inflight == 0 {
		c.idleOnce.Do(func() { close(c.idle) })
	}
}

// own records a stream or bucket name before it is created.
func (f *Fixture) own(list *[]string, name, label string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Contains(*list, name) {
		*list = append(*list, name)
		f.rec.Owned = append(f.rec.Owned, label)
	}
}

func (f *Fixture) count(dep string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[dep]++
}

// callCounts is a copy of the per-dependency call counts.
func (f *Fixture) callCounts() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]int, len(f.calls))
	for k, v := range f.calls {
		out[k] = v
	}
	return out
}

func (f *Fixture) totalCalls() int {
	n := 0
	for _, v := range f.callCounts() {
		n += v
	}
	return n
}

// report surfaces a degraded evidence path as a test error: evidence that silently fails to
// land is the failure the evidence directory exists to prevent.
func (f *Fixture) report(format string, args ...any) {
	if f.errorf != nil {
		f.errorf(format, args...)
	}
}

func (f *Fixture) writeEvidence() {
	f.mu.Lock()
	f.rec.Remaining = f.remainingLocked()
	r, dir := f.rec, f.adm.evidenceDir
	f.mu.Unlock()
	if dir == "" {
		return
	}
	if err := writeRecord(dir, r.Name, r); err != nil {
		f.report("natsfixture: evidence record not written: %v", err)
	}
}
