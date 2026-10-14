// Package natsfixture gives one test one disposable, real NATS server in a container
// (openspec change setup-02-isolated-harness, capability nats-fixture). The pattern is adapted from
// SemStreams natsclient/test_client.go at 5457b345 (ledger row L1) over nats.go and
// testcontainers-go directly, without its eleven caller knobs (ledger row L2).
//
// Start succeeds only after JetStream answers; Stop closes the connection and removes the
// container, which removes everything in it. Test-only: contract test T-B1 refuses any production
// import.
package natsfixture

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// maxAttempts allows Start one replacement container, and only after a mapped-port failure
	// with a live parent and a clean rollback (retryable): the one transient SemStreams observed
	// under Docker API pressure (gh#736). Restart's one more stop and start of the same container
	// is decided the same way. Both were also kept for Docker Desktop on macOS picking a host port
	// a Mac process held (#180), which the start phase now prevents by giving Docker the port
	// (freePort, #209); whether they stay is the test-infra audit's question (#205). Every other
	// failure is reported, not retried.
	maxAttempts = 2
	// cleanupBudget is the fresh finite authority New's t.Cleanup gives Stop. The test's own
	// context is already cancelled when cleanups run.
	cleanupBudget = 60 * time.Second
	// Stream and bucket bounds: every resource the fixture creates declares them, so a runaway
	// test fails on a ceiling instead of filling the broker (SemStreams test_client.go:911-927).
	resourceMaxAge   = time.Hour
	resourceMaxBytes = 64 << 20
	// readyLog is the line nats-server logs once per boot when it accepts clients.
	readyLog = "Server is ready"
	// maxPayloadConfig is where WithMaxPayload's config file lands in the container.
	maxPayloadConfig = "/etc/nats/semengine-fixture.conf"
)

// imagePin is the pin file, the one spelling of the NATS image (harness-boundaries › "One image
// pin"); pinnedImage reads it.
//
//go:embed nats-image
var imagePin string

// digestImage is a NATS image named by its content digest, with or without a tag.
var digestImage = regexp.MustCompile(`^nats(:[A-Za-z0-9][A-Za-z0-9._-]*)?@sha256:[0-9a-f]{64}$`)

// pinnedImage returns the one image line of a pin file, skipping blank lines and # comments, and
// refuses a file without exactly one, or one that is not a digest reference: a tag alone can be
// re-pointed under a running repository.
func pinnedImage(file string) (string, error) {
	var images []string
	for line := range strings.SplitSeq(file, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			images = append(images, line)
		}
	}
	if len(images) != 1 {
		return "", fmt.Errorf("the image pin holds %d image lines, want exactly one", len(images))
	}
	if !digestImage.MatchString(images[0]) {
		return "", fmt.Errorf("the image pin %q is not a digest reference (nats[:tag]@sha256:<64 hex>)", images[0])
	}
	return images[0], nil
}

// Fixture is one test's NATS server and the resources it creates on it. It holds no
// context.Context: callback contexts live in the goroutines nats.go runs, and the fixture keeps
// only their cancel functions.
type Fixture struct {
	errorf func(format string, args ...any)
	logf   func(format string, args ...any) // the test log, for a failed attempt's container log
	deps   deps

	// op is a one-slot semaphore serialising Start, Stop, and resource creation. It is a channel,
	// not a mutex, so a caller waiting for it still answers to its own context.
	op chan struct{}
	mu sync.Mutex // guards everything below; never held across a Docker or NATS call

	used          bool
	stopping      bool          // set when Stop begins on a started fixture; creation is refused from then on
	stopBegun     chan struct{} // closed with stopping set, so a creation queued for the slot is refused too
	container     testcontainers.Container
	containerID   string
	url           string
	nc            *nats.Conn
	js            jetstream.JetStream
	consumers     []*consumer
	calls         map[string]int
	slotWaits     map[string]int // operations that found the slot taken, by kind; read by tests
	maxPayload    int64          // broker max_payload in bytes, from WithMaxPayload
	maxPayloadSet bool           // unset leaves the broker default
}

// Option configures a fixture's broker; New applies it before any Docker call.
type Option func(*Fixture)

// WithMaxPayload starts the broker with max_payload set to n bytes, so a test can reach a payload
// bound without building a megabyte message. Without it the broker's own default applies. Start
// refuses an n below 1 before any Docker call.
func WithMaxPayload(n int64) Option {
	return func(f *Fixture) { f.maxPayload, f.maxPayloadSet = n, true }
}

// New binds a fixture to its test and registers a Stop, under fresh bounded authority, as the
// test's cleanup. It makes no Docker call.
func New(t testing.TB, opts ...Option) *Fixture {
	t.Helper()
	f := &Fixture{errorf: t.Errorf, logf: t.Logf, deps: defaultDeps(t.Logf), calls: map[string]int{}}
	for _, opt := range opts {
		opt(f)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupBudget)
		defer cancel()
		if err := f.Stop(ctx); err != nil {
			t.Errorf("natsfixture: cleanup Stop: %v; still held: %v", err, f.remaining())
		}
	})
	return f
}

// Start starts one NATS container with JetStream from the pinned image, connects, and returns once
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
	if f.maxPayloadSet && f.maxPayload < 1 {
		return fmt.Errorf("natsfixture: max payload %d bytes: the broker needs at least 1", f.maxPayload)
	}
	if err := f.acquire(ctx, "start"); err != nil {
		return err
	}
	defer f.release()
	f.mu.Lock()
	used := f.used
	f.mu.Unlock()
	if used {
		return ErrAlreadyUsed
	}
	f.mu.Lock()
	f.used = true
	f.mu.Unlock()

	var failures []error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := f.attempt(ctx, attempt)
		if err == nil {
			return nil
		}
		failures = append(failures, err)
		if !retryable(err) {
			break
		}
	}
	if len(failures) == 1 {
		return failures[0]
	}
	return errors.Join(failures...)
}

// acquire takes the operation slot for an operation of kind who, or returns ctx's error if ctx
// ends first. A creation or a restart also gives up when Stop begins: it may be queued behind Stop,
// and Stop may be waiting to join the very handler that called it.
func (f *Fixture) acquire(ctx context.Context, who string) error {
	f.mu.Lock()
	if f.op == nil {
		f.op = make(chan struct{}, 1)
	}
	if f.stopBegun == nil {
		f.stopBegun = make(chan struct{})
	}
	op := f.op
	var stopBegun <-chan struct{} // nil, never ready, except for creations and restarts
	if who == "create" || who == "restart" {
		stopBegun = f.stopBegun
	}
	f.mu.Unlock()
	select {
	case op <- struct{}{}:
		return nil
	default:
	}
	f.mu.Lock()
	if f.slotWaits == nil {
		f.slotWaits = map[string]int{}
	}
	f.slotWaits[who]++
	f.mu.Unlock()
	select {
	case op <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-stopBegun:
		return errStopping
	}
}

// beginStop marks the fixture stopping and wakes every creation queued for the slot. The caller
// holds the slot.
func (f *Fixture) beginStop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopping {
		return
	}
	f.stopping = true
	if f.stopBegun == nil {
		f.stopBegun = make(chan struct{})
	}
	close(f.stopBegun)
}

func (f *Fixture) release() { <-f.op }

// attempt runs the Start phases once. On failure it rolls back what it created and returns *Error.
func (f *Fixture) attempt(ctx context.Context, n int) error {
	fail := func(p Phase, cause error) error {
		f.mu.Lock()
		id := f.containerID
		f.mu.Unlock()
		fe := &Error{Attempt: n, Phase: p, ContainerID: id, ParentErr: ctx.Err(), Cause: cause}
		fe.Cleanup = f.rollbackAttempt(ctx, n)
		return fe
	}
	// run runs one phase and turns a cancelled parent into that phase's failure even when the
	// operation itself returned success: work finished after cancellation is not accepted.
	run := func(p Phase, op func() error) error {
		err := op()
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			return fail(p, err)
		}
		return nil
	}

	var image string
	if err := run(PhaseImage, func() error {
		var err error
		image, err = pinnedImage(imagePin)
		return err
	}); err != nil {
		return err
	}

	// The image's entrypoint prefixes nats-server to flag arguments. JetStream stores in the
	// container's own filesystem: the fixture creates no volume.
	cmd := []string{"--port", "4222", "--js"}
	var files []testcontainers.ContainerFile
	if f.maxPayloadSet {
		// nats-server has no command-line flag for max_payload, so it goes in a config file
		// copied in before the container starts; the flags above still apply over it. The file
		// is in the container's writable layer, so a Restart keeps the limit.
		cmd = append(cmd, "--config", maxPayloadConfig)
		files = append(files, testcontainers.ContainerFile{
			Reader:            strings.NewReader(fmt.Sprintf("max_payload: %d\n", f.maxPayload)),
			ContainerFilePath: maxPayloadConfig,
			FileMode:          0o644,
		})
	}
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			Labels:       map[string]string{testLabel: labelValue()},
			ExposedPorts: []string{clientPort},
			Cmd:          cmd,
			Files:        files,
			WaitingFor:   wait.ForLog(readyLog),
		},
		Started: true,
	}
	var c testcontainers.Container
	if err := run(PhaseStart, func() error {
		hostPort, err := freePort()
		if err != nil {
			return err
		}
		req.HostConfigModifier = func(hc *container.HostConfig) {
			hc.PortBindings = network.PortMap{network.MustParsePort(clientPort): {{HostPort: hostPort}}}
		}
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
		port, err = f.mappedPort(ctx, c)
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
	f.mu.Unlock()
	return nil
}

// freePort asks the operating system for a free TCP port on all addresses and releases it, so the
// start phase can give Docker that exact host port. Left to pick, Docker Desktop for Mac takes the
// next port of a counter of its own, which does not see the Mac's listeners, and a container whose
// port a Mac process holds runs with an empty port list (#209; the probe on PR #210, comment
// 6151736554; TestStartPastHeldPorts). The macOS kernel hands ports out in sequence (the same
// probe), so a port-0 listener does not get this port back until the range wraps; a process that
// binds it by number before Docker does fails the start phase with "address already in use" (the
// same probe). Restart keeps the binding (TestRestartURLDials).
func freePort() (string, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	return port, l.Close()
}

// retryable reports whether a failure earns the one more try Start and Restart allow: the
// mapped-port phase failed with the caller's context live and, for Start, a clean rollback.
func retryable(err error) bool {
	var fe *Error
	return errors.As(err, &fe) && fe.Phase == PhaseMappedPort && fe.ParentErr == nil && fe.Cleanup == nil
}

// mappedPort reads the host port Docker mapped to the container's client port. A failed read also
// says what an inspect of the container showed, or why the inspect failed, so an empty port list
// is named in the error (#180, #209).
func (f *Fixture) mappedPort(ctx context.Context, c testcontainers.Container) (string, error) {
	port, err := f.deps.mappedPort(ctx, c)
	f.count("mappedPort")
	if err == nil {
		return port, nil
	}
	shown, ierr := f.deps.inspect(ctx, c)
	f.count("inspect")
	if ierr != nil {
		return "", fmt.Errorf("%w (inspect failed: %v)", err, ierr)
	}
	return "", fmt.Errorf("%w (inspect: %s)", err, shown)
}

// rollbackAttempt removes what a failed attempt created, each operation under its own fresh
// bounded context, and writes the container's log to the test log first. It returns nil only when
// Docker reported the container removed; otherwise the container stays held for a later Stop.
func (f *Fixture) rollbackAttempt(ctx context.Context, n int) error {
	f.mu.Lock()
	nc, c, id := f.nc, f.container, f.containerID
	f.nc = nil
	f.mu.Unlock()
	if nc != nil {
		nc.Close() // nothing of the test's is in flight on a connection Start never returned
	}
	if c == nil {
		return nil
	}
	logErr := rollback(ctx, func(rctx context.Context) error {
		logs, err := f.deps.logs(rctx, c)
		f.count("logs")
		if len(logs) > 0 && f.logf != nil {
			f.logf("natsfixture: attempt %d, container %s log:\n%s", n, shortID(id), logs)
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
	if err != nil {
		return fmt.Errorf("container %s: %w", shortID(id), err)
	}
	f.mu.Lock()
	f.container, f.containerID = nil, ""
	f.mu.Unlock()
	return nil
}

// URL is the broker's client URL, on the host port the start phase picked (freePort); Restart keeps
// it.
func (f *Fixture) URL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.url
}

// JetStream is the JetStream context on the fixture's own connection, which Stop closes; there is
// deliberately no accessor for that connection. Tests needing a connection of their own dial URL
// and close it themselves. Stop does not wait for in-flight async publishes.
func (f *Fixture) JetStream() jetstream.JetStream {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.js
}

func (f *Fixture) started() (jetstream.JetStream, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopping {
		return nil, "", errStopping
	}
	if f.js == nil {
		return nil, "", errors.New("natsfixture: fixture not started")
	}
	return f.js, f.containerID, nil
}

var errStopping = errors.New("natsfixture: Stop has begun; the fixture creates nothing more")

// beginCreate admits one resource creation: refused at once once Stop has begun, otherwise it
// holds the operation slot so Stop cannot snapshot the consumers while a creation is in flight. A
// handler that creates while Stop waits to join it is refused, not deadlocked. The caller releases
// the slot.
func (f *Fixture) beginCreate(ctx context.Context) (jetstream.JetStream, string, error) {
	if _, _, err := f.started(); err != nil {
		return nil, "", err
	}
	if err := f.acquire(ctx, "create"); err != nil {
		return nil, "", err
	}
	js, id, err := f.started() // Stop may have begun and ended while this waited
	if err != nil {
		f.release()
		return nil, "", err
	}
	return js, id, nil
}

// CreateStream creates a file-backed stream with the fixture's MaxAge, MaxBytes, and DiscardOld
// bounds. Its messages survive Restart; Stop's removal of the container removes it.
func (f *Fixture) CreateStream(ctx context.Context, name string, subjects ...string) (jetstream.Stream, error) {
	if ctx == nil {
		return nil, errors.New("natsfixture: CreateStream with a nil context")
	}
	return f.createStream(ctx, jetstream.FileStorage, name, subjects)
}

// CreateMemoryStream creates a memory-backed stream with the same bounds as CreateStream. Its
// messages do not survive Restart; Stop's removal of the container removes it.
func (f *Fixture) CreateMemoryStream(ctx context.Context, name string, subjects ...string) (jetstream.Stream, error) {
	if ctx == nil {
		return nil, errors.New("natsfixture: CreateMemoryStream with a nil context")
	}
	return f.createStream(ctx, jetstream.MemoryStorage, name, subjects)
}

func (f *Fixture) createStream(ctx context.Context, storage jetstream.StorageType, name string, subjects []string) (jetstream.Stream, error) {
	js, id, err := f.beginCreate(ctx)
	if err != nil {
		return nil, err
	}
	defer f.release()
	s, err := f.deps.createStream(ctx, js, jetstream.StreamConfig{
		Name: name, Subjects: subjects, Storage: storage,
		MaxAge: resourceMaxAge, MaxBytes: resourceMaxBytes, Discard: jetstream.DiscardOld,
	})
	f.count("createStream")
	if err != nil {
		return nil, &Error{Attempt: 1, Phase: PhaseCreateStream, ContainerID: id, ParentErr: ctx.Err(), Cause: err}
	}
	return s, nil
}

// CreateKeyValue creates a bucket bounded like a stream (TTL as MaxAge).
func (f *Fixture) CreateKeyValue(ctx context.Context, bucket string) (jetstream.KeyValue, error) {
	if ctx == nil {
		return nil, errors.New("natsfixture: CreateKeyValue with a nil context")
	}
	js, id, err := f.beginCreate(ctx)
	if err != nil {
		return nil, err
	}
	defer f.release()
	kv, err := f.deps.createKV(ctx, js, jetstream.KeyValueConfig{Bucket: bucket, TTL: resourceMaxAge, MaxBytes: resourceMaxBytes})
	f.count("createKV")
	if err != nil {
		return nil, &Error{Attempt: 1, Phase: PhaseCreateKV, ContainerID: id, ParentErr: ctx.Err(), Cause: err}
	}
	return kv, nil
}

// Consume creates a durable consumer named name on stream and delivers each message to
// handler with a context derived from ctx. Stop stops delivery, waits for every running handler to
// return, and only then cancels that context and deletes the consumer: a callback's context stays
// live until its handler has been joined.
func (f *Fixture) Consume(ctx context.Context, stream, name string, handler func(context.Context, jetstream.Msg)) (jetstream.Consumer, error) {
	if ctx == nil || handler == nil {
		return nil, errors.New("natsfixture: Consume needs a context and a handler")
	}
	js, id, err := f.beginCreate(ctx)
	if err != nil {
		return nil, err
	}
	defer f.release()
	c := &consumer{stream: stream, name: name, idle: make(chan struct{})}
	f.mu.Lock()
	f.consumers = append(f.consumers, c)
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

// report surfaces a failed attempt's container log that could not be read as a test error: the
// log is how a developer learns why the container failed.
func (f *Fixture) report(format string, args ...any) {
	if f.errorf != nil {
		f.errorf(format, args...)
	}
}
