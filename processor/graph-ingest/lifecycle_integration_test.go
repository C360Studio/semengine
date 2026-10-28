//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
	"github.com/nats-io/nats.go/jetstream"
)

// The pin ran these tests on one NATS client shared through TestMain
// (natsclient.NewSharedTestClient), terminated with no deadline. natsfixture.Open
// takes the test (design D3), so each test here opens a broker of its own, which its
// cleanup stops under a bounded context. The pin's TestMain also registered four test
// predicates for merge_entity_integration_test.go, which a later stage ports, and
// checked the owner-child request first; TestGraphIngestOwnerChildFixture checks that
// request itself.

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle(t *testing.T) *Component {
	t.Helper()
	config := DefaultConfig()
	deps := component.Dependencies{
		NATSClient:      newFixtureClient(t, entityStream),
		PayloadRegistry: newTestPayloadRegistry(t),
		Platform:        types.PlatformMeta{Org: testDeploymentOrg, Platform: testDeploymentPlatform},
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	comp, err := CreateGraphIngest(configJSON, deps)
	if err != nil {
		t.Fatalf("create component: %v", err)
	}

	return comp.(*Component)
}

func TestGraphIngest_OneShotLifecycleAgainstNATS(t *testing.T) {
	comp := createTestComponentForLifecycle(t)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := comp.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("repeated completed Stop: %v", err)
	}
	if err := comp.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start after terminal Stop error = %v, want ErrAlreadyStarted", err)
	}
}

// TestGraphIngestLifecycleSuite runs the lifecycle suite on graph-ingest through its
// adapter, suiteOwner. Each healthy owner is a fresh component on a client of its own,
// connected to one fixture broker that holds the ENTITY stream. The must-fail owner's
// client dials a listener that closes every connection before the handshake, so Start
// fails on its connection, before the component binds anything, and the cleanup Start
// runs succeeds: this is the
// "Cleanup succeeds after a failed start" scenario of lifecycle-suite's "Failed start
// whose own cleanup fails". The branch where that cleanup fails is
// TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop's. graph-ingest is
// one-shot (design D13: a Start after Stop is refused), so no restart is promised.
func TestGraphIngestLifecycleSuite(t *testing.T) {
	url := shared.Reset(t)
	setup := natsfixture.Open(t, url, openFixtureClient)
	js, err := setup.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	if _, err := natsfixture.CreateStream(t.Context(), js, jetstream.FileStorage, entityStream.name, entityStream.subjects...); err != nil {
		t.Fatalf("CreateStream %s: %v", entityStream.name, err)
	}
	configJSON, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	newOwner := func(client *natsclient.Client) lifecycletest.Owner {
		created, err := CreateGraphIngest(configJSON, testDependencies(t, client))
		if err != nil {
			t.Fatalf("CreateGraphIngest: %v", err)
		}
		comp := created.(*Component)
		if err := comp.Initialize(); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		return newSuiteOwner(comp)
	}
	refused := refusingBrokerURL(t)
	factory := func() lifecycletest.Owner {
		return newOwner(natsfixture.Open(t, url, openFixtureClient))
	}
	mustFail := func() lifecycletest.Owner {
		client, err := natsclient.NewClient(refused,
			natsclient.WithMaxReconnects(0),
			natsclient.WithHealthInterval(0),
		)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		return newOwner(client)
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles". The suite judges only what is
	// left after a Stop, so an adapter that omitted a kind would pass it; this names every
	// kind a started component holds.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := factory()
		startCtx, cancelStart := context.WithCancel(t.Context())
		defer cancelStart()
		if err := owner.Start(startCtx); err != nil {
			t.Fatalf("Start: %v", err)
		}
		started := owner.Observe()
		want := []string{
			"runtime cancel", "ingest submit cancel", "ingest pool cancel", "ingest pool lanes",
			"consumers", "request subscriptions", "status loop", "status publisher", "entity cache",
			"readiness bound consumers",
		}
		if !slices.Equal(started.Unresolved, want) {
			t.Errorf("held while started = %v, want %v", started.Unresolved, want)
		}
		binds := started.Calls["consumer bind"]
		if binds == 0 || started.Calls["consumer drain"] != 0 {
			t.Errorf("calls while started = %v, want consumer binds and no drain", started.Calls)
		}
		stopCtx, cancelStop := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancelStop()
		if err := owner.Stop(stopCtx); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		stopped := owner.Observe()
		if len(stopped.Unresolved) != 0 {
			t.Errorf("held after a nil Stop = %v, want nothing", stopped.Unresolved)
		}
		if stopped.Calls["consumer drain"] != binds || stopped.Calls["consumer stop"] != 0 {
			t.Errorf("calls after Stop = %v, want one drain per bound consumer and no force stop", stopped.Calls)
		}
	})
}

// TestGraphIngest_StartBoundedByItsContext: the Start context bounds startup, including
// the consumer setup's wait for its input stream (ADR-113; design D6, T-G3). No input
// stream exists, so that wait, which retries for about 50 s under any other context,
// can end only by the Start context's 1 s deadline. Start must return that deadline
// well inside 10 s and hold nothing, and Stop must then return nil.
func TestGraphIngest_StartBoundedByItsContext(t *testing.T) {
	configJSON, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	created, err := CreateGraphIngest(configJSON, testDependencies(t, newFixtureClient(t)))
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	comp := created.(*Component)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	owner := newSuiteOwner(comp)

	startCtx, cancelStart := context.WithTimeout(t.Context(), time.Second)
	defer cancelStart()
	startResult := make(chan error, 1)
	go func() { startResult <- owner.Start(startCtx) }()
	// failureBound is the real-clock bound past which Start is reported as not bounded
	// by its context; the deadline it must honour is 1 s.
	const failureBound = 10 * time.Second
	bound := time.NewTimer(failureBound)
	defer bound.Stop()
	var startErr error
	select {
	case startErr = <-startResult:
	case <-bound.C:
		t.Errorf("Start had not returned %s after its 1 s deadline", failureBound)
		// Start's own wait is bounded by waitForStream's retry budget (about 50 s, as mutant M7
		// showed); join it so nothing it logs outlives the test.
		startErr = <-startResult
	}
	if !errors.Is(startErr, context.DeadlineExceeded) {
		t.Fatalf("Start error = %v, want it to wrap context.DeadlineExceeded", startErr)
	}
	if held := owner.Observe().Unresolved; len(held) != 0 {
		t.Errorf("held after a Start whose context ended = %v, want nothing", held)
	}
	stopCtx, cancelStop := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancelStop()
	if err := owner.Stop(stopCtx); err != nil {
		t.Fatalf("Stop after a failed Start: %v", err)
	}
}

// cancelOnRecordHandler is the component's logger handler: it cancels when it handles the
// record whose message is msg.
type cancelOnRecordHandler struct {
	msg    string
	cancel context.CancelFunc
}

func (h *cancelOnRecordHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (h *cancelOnRecordHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Message == h.msg {
		h.cancel()
	}
	return nil
}

func (h *cancelOnRecordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *cancelOnRecordHandler) WithGroup(string) slog.Handler      { return h }

// TestGraphIngest_StartFailsWhenItsContextEndsAfterHandlerSetup: a Start context that ends
// after the last request registration succeeded, while setup is still running, fails
// Start, which holds nothing, and a later Stop returns nil (ADR-113; design
// natsclient-handler-root D2, T6). The component's logger, given through deps.Logger,
// cancels the Start context when it handles "mutation handlers registered", which
// setupMutationHandlers logs after its last registration and before it returns. It passes
// on the base too: it guards start's final check of its context (mutant M7).
func TestGraphIngest_StartFailsWhenItsContextEndsAfterHandlerSetup(t *testing.T) {
	startCtx, cancelStart := context.WithCancel(t.Context())
	defer cancelStart()
	deps := testDependencies(t, newFixtureClient(t, entityStream))
	deps.Logger = slog.New(&cancelOnRecordHandler{msg: "mutation handlers registered", cancel: cancelStart})
	configJSON, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	created, err := CreateGraphIngest(configJSON, deps)
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	comp := created.(*Component)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	owner := newSuiteOwner(comp)

	startErr := owner.Start(startCtx)
	if !errors.Is(startErr, context.Canceled) {
		t.Fatalf("Start error = %v, want it to wrap context.Canceled", startErr)
	}
	if held := owner.Observe().Unresolved; len(held) != 0 {
		t.Errorf("held after a Start whose context ended = %v, want nothing", held)
	}
	stopCtx, cancelStop := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancelStop()
	if err := owner.Stop(stopCtx); err != nil {
		t.Fatalf("Stop after a failed Start: %v", err)
	}
}

// refusingBrokerURL returns the NATS URL of a loopback listener that stays bound for the
// test's life and closes every connection it accepts before sending the server's INFO, so
// a client's Connect to it fails on the handshake. A port that was bound and released can
// be handed to another process, whose broker would accept the dial (#89). The listener is
// closed, and its accept loop joined, on cleanup.
func refusingBrokerURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	accepting := make(chan struct{})
	go func() {
		defer close(accepting)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // the listener is closed
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-accepting
	})
	return "nats://" + listener.Addr().String()
}

// relayDialTimeout bounds the relay's dial to the shared broker, which is already
// answering; the relay's cleanup joins a dial in flight, so it waits at most this long.
const relayDialTimeout = 5 * time.Second

// brokerRelay forwards loopback connections to the package's shared broker until the
// test cuts it, so a client's connection drops and every reconnect is refused while the
// broker keeps running (design D2). It exists because natsfixture.Restart stops and
// starts the container in one call, so it cannot hold the broker down while Start runs,
// and an embedded server brought back would reuse a port it released, which another
// process may have bound in between (#89). Like refusingBrokerURL, its listener stays
// bound for the test's life. Its cleanup closes the listener and every connection and
// joins its goroutines.
type brokerRelay struct {
	listener net.Listener
	upstream string // the shared broker's host:port
	wg       sync.WaitGroup

	mu     sync.Mutex
	cut    bool
	closed bool
	conns  map[net.Conn]struct{}
}

// newBrokerRelay starts a relay to brokerURL, the address shared.Reset returned. Call it
// after the reset and before natsfixture.Open, so cleanup closes the client before the
// relay and the relay before the reset's connection check.
func newBrokerRelay(t *testing.T, brokerURL string) *brokerRelay {
	t.Helper()
	upstream, ok := strings.CutPrefix(brokerURL, "nats://")
	if !ok {
		t.Fatalf("broker URL %q is not a nats:// URL", brokerURL)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	r := &brokerRelay{listener: listener, upstream: upstream, conns: map[net.Conn]struct{}{}}
	r.wg.Go(r.accept)
	t.Cleanup(r.close)
	return r
}

// URL is the relay's NATS URL.
func (r *brokerRelay) URL() string { return "nats://" + r.listener.Addr().String() }

// Cut closes every relayed connection, and each connection accepted until Restore is
// closed at once, before the broker's INFO reaches the client.
func (r *brokerRelay) Cut() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cut = true
	r.closeAllLocked()
}

// Restore forwards connections to the broker again.
func (r *brokerRelay) Restore() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cut = false
}

func (r *brokerRelay) close() {
	r.mu.Lock()
	r.closed = true
	r.closeAllLocked()
	r.mu.Unlock()
	_ = r.listener.Close()
	r.wg.Wait()
}

func (r *brokerRelay) closeAllLocked() {
	for conn := range r.conns {
		_ = conn.Close()
	}
	clear(r.conns)
}

// track records conn as relayed, or reports false when the relay is cut or closed.
func (r *brokerRelay) track(conn net.Conn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cut || r.closed {
		return false
	}
	r.conns[conn] = struct{}{}
	return true
}

func (r *brokerRelay) drop(conn net.Conn) {
	r.mu.Lock()
	delete(r.conns, conn)
	r.mu.Unlock()
	_ = conn.Close()
}

func (r *brokerRelay) accept() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			return // the listener is closed
		}
		if !r.track(conn) {
			_ = conn.Close()
			continue
		}
		r.wg.Go(func() { r.forward(conn) })
	}
}

// forward copies bytes both ways between client and a new broker connection until either
// side ends, then closes both.
func (r *brokerRelay) forward(client net.Conn) {
	server, err := net.DialTimeout("tcp", r.upstream, relayDialTimeout)
	if err != nil {
		r.drop(client)
		return
	}
	if !r.track(server) {
		_ = server.Close()
		r.drop(client)
		return
	}
	r.wg.Go(func() {
		_, _ = io.Copy(server, client)
		r.drop(server)
		r.drop(client)
	})
	_, _ = io.Copy(client, server)
	r.drop(client)
	r.drop(server)
}

// reconnectWait is the relayed client's wait between reconnect attempts: short, so the
// client is back soon after Restore.
const reconnectWait = 50 * time.Millisecond

// statusBound is the real-clock failure bound of a wait for the relayed client's status.
// nats.go's reconnect pacing is not promised (ADR-112 section 5).
const statusBound = 10 * time.Second

// openReconnectingClient opens a natsclient.Client with natsclient's reconnect default
// (unlimited reconnects), a short reconnect wait and no health monitor, so only nats.go's
// own disconnect and reconnect move its status (design D2). natsfixture.Open bounds it
// and closes it on cleanup.
func openReconnectingClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithReconnectWait(reconnectWait),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}

// newRelayedComponent builds an initialized graph-ingest whose client reaches the shared
// broker through a relay, with the ENTITY stream already created.
func newRelayedComponent(t *testing.T) (*suiteOwner, *natsclient.Client, *brokerRelay) {
	t.Helper()
	brokerURL := shared.Reset(t)
	relay := newBrokerRelay(t, brokerURL)
	client := natsfixture.Open(t, relay.URL(), openReconnectingClient)
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	if _, err := natsfixture.CreateStream(t.Context(), js, jetstream.FileStorage, entityStream.name, entityStream.subjects...); err != nil {
		t.Fatalf("CreateStream %s: %v", entityStream.name, err)
	}
	configJSON, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	created, err := CreateGraphIngest(configJSON, testDependencies(t, client))
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	comp := created.(*Component)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return newSuiteOwner(comp), client, relay
}

// awaitStatus waits, under statusBound, until client reports want.
func awaitStatus(ctx context.Context, client *natsclient.Client, want natsclient.ConnectionStatus) error {
	ctx, cancel := context.WithTimeout(ctx, statusBound)
	defer cancel()
	_, err := probe.Await(ctx,
		func(context.Context) (natsclient.ConnectionStatus, error) { return client.Status(), nil },
		func(s natsclient.ConnectionStatus) bool { return s == want })
	return err
}

// cutUntilReconnecting cuts the relay and waits until the client reports Reconnecting:
// observed, not predicted, since nats.go does not promise when it notices the cut
// (design D4). It registers nothing; the caller restores.
func cutUntilReconnecting(t *testing.T, relay *brokerRelay, client *natsclient.Client) {
	t.Helper()
	relay.Cut()
	if err := awaitStatus(t.Context(), client, natsclient.StatusReconnecting); err != nil {
		t.Fatalf("client never reported Reconnecting after the cut: %v", err)
	}
}

// restoreUntilConnected restores the relay and waits until the client reports Connected
// again. Every test that cuts defers it: Close drains, and a drain on a reconnecting
// connection fails at the pinned nats.go (design D2), so the client's cleanup would fail
// a correct implementation if the client were still reconnecting.
func restoreUntilConnected(t *testing.T, relay *brokerRelay, client *natsclient.Client) {
	t.Helper()
	relay.Restore()
	if err := awaitStatus(t.Context(), client, natsclient.StatusConnected); err != nil {
		t.Errorf("client never reported Connected after the relay was restored: %v", err)
	}
}

// TestGraphIngest_StartWaitsForAReconnectingClient: Start given a client that nats.go is
// reconnecting waits for the connection instead of failing (#291; design D1, D2 T1).
// The relay is cut and the client observed Reconnecting before Start runs. Start must
// not return within a 1 s observation window, during which no connection is possible;
// the window only lets Start reach its connect step before the restore, and no correct
// Start can return inside it. After the restore Start must return nil within 10 s, Stop
// must return nil and nothing is held.
func TestGraphIngest_StartWaitsForAReconnectingClient(t *testing.T) {
	owner, client, relay := newRelayedComponent(t)
	defer restoreUntilConnected(t, relay, client) // registered first: a failed cut still restores
	cutUntilReconnecting(t, relay, client)

	startCtx, cancelStart := context.WithCancel(t.Context())
	defer cancelStart()
	startResult := make(chan error, 1)
	go func() { startResult <- owner.Start(startCtx) }()

	// observationWindow is listed in design D2 (docs/testing.md, "Porting a test from
	// SemStreams"): its expiry is the expected outcome, never a failure.
	const observationWindow = time.Second
	window := time.NewTimer(observationWindow)
	select {
	case err := <-startResult:
		window.Stop()
		t.Fatalf("Start returned %v within %s while the client was reconnecting, want it to wait for the connection", err, observationWindow)
	case <-window.C:
	}

	relay.Restore()
	// failureBound is the real-clock bound past which Start is reported as not having
	// returned once the connection is possible again.
	const failureBound = 10 * time.Second
	bound := time.NewTimer(failureBound)
	defer bound.Stop()
	var startErr error
	select {
	case startErr = <-startResult:
	case <-bound.C:
		t.Errorf("Start had not returned %s after the relay was restored", failureBound)
		// End Start's context and join it, so nothing it runs outlives the test.
		cancelStart()
		startErr = <-startResult
	}
	if startErr != nil {
		t.Fatalf("Start after the client reconnected: %v", startErr)
	}

	stopCtx, cancelStop := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancelStop()
	if err := owner.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if held := owner.Observe().Unresolved; len(held) != 0 {
		t.Errorf("held after a nil Stop = %v, want nothing", held)
	}
}

// TestGraphIngest_StartContextEndsWhileWaitingForReconnect: a Start context that ends
// while Start waits for a reconnecting client fails Start with that context's error, and
// Start holds nothing (ADR-113; design D2 T2). The relay stays cut, so no connection is
// possible before Start's 1 s deadline. Start must return an error matching
// context.DeadlineExceeded within 10 s, hold nothing, and Stop must then return nil.
func TestGraphIngest_StartContextEndsWhileWaitingForReconnect(t *testing.T) {
	owner, client, relay := newRelayedComponent(t)
	defer restoreUntilConnected(t, relay, client) // registered first: a failed cut still restores
	cutUntilReconnecting(t, relay, client)

	startCtx, cancelStart := context.WithTimeout(t.Context(), time.Second)
	defer cancelStart()
	startResult := make(chan error, 1)
	go func() { startResult <- owner.Start(startCtx) }()

	// failureBound is the real-clock bound past which Start is reported as not bounded by
	// its context; the deadline it must honour is 1 s.
	const failureBound = 10 * time.Second
	bound := time.NewTimer(failureBound)
	defer bound.Stop()
	var startErr error
	select {
	case startErr = <-startResult:
	case <-bound.C:
		t.Errorf("Start had not returned %s after its 1 s deadline", failureBound)
		// Let Start reach the broker and join it, so nothing it runs outlives the test.
		relay.Restore()
		startErr = <-startResult
	}
	if !errors.Is(startErr, context.DeadlineExceeded) {
		t.Errorf("Start error = %v, want it to wrap context.DeadlineExceeded", startErr)
	}
	if held := owner.Observe().Unresolved; len(held) != 0 {
		t.Errorf("held after a Start whose context ended = %v, want nothing", held)
	}
	stopCtx, cancelStop := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancelStop()
	if err := owner.Stop(stopCtx); err != nil {
		t.Fatalf("Stop after a failed Start: %v", err)
	}
}
