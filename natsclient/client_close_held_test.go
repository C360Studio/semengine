package natsclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// T1, design "Tests that replace the probes". A publish the server is not
// reading holds nats.go's connection lock, which Drain and Close need; Close
// must close the socket when its context ends rather than wait for nats.go's
// write timeout (one minute by default).
func TestCloseEndsAHeldWrite(t *testing.T) {
	client, proxy := newHeldClient(t)
	conn := client.GetConnection()
	published := holdPublish(t, client, proxy)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	// The test's own wait. Before design D1, Close waited for the write.
	err := within(t, startClose(ctx, client), lifecycleBound, "Close")

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Error(t, within(t, published, lifecycleBound, "the held publish"), "the held publish succeeded")
	requireClosed(t, conn)
}

// requireClosed requires conn to reach CLOSED. It is awaited, not read once:
// when Close's drain had begun, nats.go's drain goroutine can set DRAINING_PUBS
// after natsclient's close, and closes the connection again only once a flush
// of up to 5 s has failed (nats.go:6378-6390). Seen in 2 of 160 runs of T1.
func requireClosed(t *testing.T, conn *nats.Conn) {
	t.Helper()
	wait, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()
	status, err := probe.Await(wait, func(context.Context) (nats.Status, error) {
		return conn.Status(), nil
	}, func(s nats.Status) bool { return s == nats.CLOSED })
	require.NoError(t, err, "the connection was not closed: status %v", status)
}

// T2, design "Tests that replace the probes". A Subscribe held behind a write
// the server is not reading must not hold natsclient's own lock while it waits
// for nats.go's, or Close cannot begin, and so cannot cut the socket (design D3;
// P21).
func TestCloseIsNotHeldByAHeldSubscribe(t *testing.T) {
	client, proxy := newHeldClient(t)
	conn := client.GetConnection()
	published := holdPublish(t, client, proxy)

	subscribed := make(chan error, 1)
	go func() {
		_, err := client.Subscribe(t.Context(), "natsclient.t2", func(context.Context, *nats.Msg) {})
		subscribed <- err
	}()
	awaitSubscribeAdmitted(t, client)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := within(t, startClose(ctx, client), lifecycleBound, "Close")

	require.ErrorIs(t, err, context.DeadlineExceeded)
	// Held until Close cuts the socket, the Subscribe fails after Close has
	// begun, whichever step fails it: the connection check, nats.go's subscribe
	// or the round trip.
	require.ErrorIs(t, within(t, subscribed, lifecycleBound, "the held Subscribe"), nats.ErrConnectionClosed)
	require.Error(t, within(t, published, lifecycleBound, "the held publish"), "the held publish succeeded")
	requireClosed(t, conn)
}

// T8, design "Tests that replace the probes". A reconnect dial holds nats.go's
// connection lock, as a held write does (P23); Close must end the dial through
// the dialer's Cancel rather than wait for it.
func TestCloseEndsAReconnectDial(t *testing.T) {
	client := newReconnectDialClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	// The test's own wait, under nats.go's 5 s default dial timeout, which
	// bounded Close before the cut ended the dial (P23: 4.69 s).
	err := within(t, startClose(ctx, client), 3*time.Second, "Close")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// Design D3: a Subscribe admitted before Close began, whose connection check
// then finds the connection not connected, returns nats.ErrConnectionClosed,
// not ErrNotConnected (transport-client, the Subscribe's error order). The
// reconnect dial holds nats.go's lock until Close cuts it, and no reconnect
// succeeds after the cut, so the check runs after Close has begun and fails.
func TestSubscribeEndedByCloseIsConnectionClosed(t *testing.T) {
	client := newReconnectDialClient(t)

	subscribed := make(chan error, 1)
	go func() {
		_, err := client.Subscribe(t.Context(), "natsclient.d3", func(context.Context, *nats.Msg) {})
		subscribed <- err
	}()
	awaitSubscribeAdmitted(t, client)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, within(t, startClose(ctx, client), lifecycleBound, "Close"), context.DeadlineExceeded)

	err := within(t, subscribed, lifecycleBound, "the held Subscribe")
	require.ErrorIs(t, err, nats.ErrConnectionClosed)
	require.NotErrorIs(t, err, ErrNotConnected)

	// The refused Subscribe is no longer owned work, so a later Close under a
	// live context sees the join end and returns nil (transport-client, "Close
	// under an ended context").
	live, cancelLive := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancelLive()
	require.NoError(t, within(t, startClose(live, client), lifecycleBound, "the second Close"))
}

// awaitSubscribeAdmitted waits until a Subscribe is counted as client-owned
// work. The count is read under TryRLock, so a Subscribe that keeps
// natsclient's lock while it waits for nats.go's fails this wait instead of
// holding it.
func awaitSubscribeAdmitted(t *testing.T, client *Client) {
	t.Helper()
	wait, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()
	_, err := probe.Await(wait, func(context.Context) (int, error) {
		if !client.mu.TryRLock() {
			return 0, errors.New("natsclient's lock is held")
		}
		defer client.mu.RUnlock()
		return client.running[workSubscription], nil
	}, func(n int) bool { return n > 0 })
	require.NoError(t, err, "the Subscribe was never admitted")
}

// newReconnectDialClient returns a client whose connection the server has
// dropped, once nats.go's reconnect dial has begun. The first dial reaches the
// server. Every later one waits for the dialer's Cancel, as a dial to an
// address that drops every packet would, or for the end of the test, so a run
// that fails does not hold nats.go's lock for the rest of the binary.
func newReconnectDialClient(t *testing.T) *Client {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	require.NoError(t, err)
	server.Start()
	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})
	require.True(t, server.ReadyForConnections(lifecycleBound), "embedded server not ready")

	client, err := NewClient(server.ClientURL(), WithHealthInterval(0), WithReconnectWait(10*time.Millisecond))
	require.NoError(t, err)
	entered := make(chan struct{}, 1)
	testEnded := make(chan struct{})
	var dials atomic.Int32
	client.netDial = func(d *net.Dialer, network, address string) (net.Conn, error) {
		if dials.Add(1) == 1 {
			return d.Dial(network, address)
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-d.Cancel:
			return nil, errors.New("dial cancelled")
		case <-testEnded:
			return nil, errors.New("test ended")
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()
	require.NoError(t, client.Connect(ctx))
	closeInCleanup(t, client)
	t.Cleanup(func() { close(testEnded) }) // registered after the client's Close, so it runs first

	cid, err := client.GetConnection().GetClientID()
	require.NoError(t, err)
	require.NoError(t, server.DisconnectClientByID(cid))
	within(t, entered, lifecycleBound, "the reconnect dial")
	return client
}

// heldSubject is the subject of the publish heldProxy holds.
const heldSubject = "natsclient.held"

// heldPayload is larger than the socket buffers between the client and the
// proxy, so a publish of it cannot finish while the proxy is not reading, and
// nats.go holds the connection's lock for the whole of it (nats.go:2346-2347).
const heldPayload = 16 << 20

// newHeldClient starts an embedded server whose max_payload admits
// heldPayload, and a client connected to it through a heldProxy. The proxy's
// cleanup runs before the client's Close: it closes both of its sockets, which
// ends a held write at once, so a test that finds Close held does not wait a
// minute for the write timeout.
func newHeldClient(t *testing.T) (*Client, *heldProxy) {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, MaxPayload: 32 << 20,
	})
	require.NoError(t, err)
	server.Start()
	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})
	require.True(t, server.ReadyForConnections(lifecycleBound), "embedded server not ready")

	proxy := startHeldProxy(t, server.Addr().String())
	client, err := NewClient(proxy.url, WithHealthInterval(0))
	require.NoError(t, err)
	closeInCleanup(t, client)
	t.Cleanup(proxy.close) // registered after the client's Close, so it runs first

	ctx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()
	require.NoError(t, client.Connect(ctx))
	return client, proxy
}

// holdPublish starts a publish of heldPayload on heldSubject and returns once
// the proxy holds it. Its result arrives on the returned channel.
func holdPublish(t *testing.T, client *Client, proxy *heldProxy) <-chan error {
	t.Helper()
	published := make(chan error, 1)
	go func() { published <- client.Publish(t.Context(), heldSubject, make([]byte, heldPayload)) }()
	wait := time.NewTimer(lifecycleBound)
	defer wait.Stop()
	select {
	case <-proxy.held:
		return published
	case err := <-published:
		t.Fatalf("the publish returned before the proxy held it: %v", err)
	case <-wait.C:
		t.Fatalf("the proxy saw no publish on %s within %v", heldSubject, lifecycleBound)
	}
	return nil
}

// startClose runs Close in its own goroutine.
func startClose(ctx context.Context, client *Client) <-chan error {
	result := make(chan error, 1)
	go func() { result <- client.Close(ctx) }()
	return result
}

// within returns the value ch delivers, and fails the test at once when none
// arrives within wait. It waits no longer: what holds the call is released by
// the test's cleanup.
func within[T any](t *testing.T, ch <-chan T, wait time.Duration, what string) T {
	t.Helper()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case v := <-ch:
		return v
	case <-timer.C:
		t.Fatalf("%s did not return within %v", what, wait)
	}
	var zero T
	return zero
}

// heldProxy forwards each connection it accepts to the server. Once a client
// has sent the first bytes of a publish on heldSubject, the proxy reads nothing
// more from that client, as a server that has stopped reading, and closes held.
type heldProxy struct {
	url       string
	ln        net.Listener
	server    string
	held      chan struct{}
	holdOnce  sync.Once
	accepting chan struct{} // closed when the accept loop has returned
	closeOnce sync.Once
	pumps     sync.WaitGroup

	mu     sync.Mutex
	closed bool
	conns  []net.Conn
}

// startHeldProxy listens on an ephemeral loopback port and forwards to server.
// Its cleanup is registered here too, so it is closed even when the test stops
// before registering its own; close runs once.
func startHeldProxy(t *testing.T, server string) *heldProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &heldProxy{
		url:       "nats://" + ln.Addr().String(),
		ln:        ln,
		server:    server,
		held:      make(chan struct{}),
		accepting: make(chan struct{}),
	}
	t.Cleanup(p.close)
	go p.accept()
	return p
}

func (p *heldProxy) accept() {
	defer close(p.accepting)
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		if !p.track(c) {
			return
		}
		p.pumps.Go(func() { p.forward(c) })
	}
}

// track records c for close, or closes it and reports false once close has
// begun.
func (p *heldProxy) track(c net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		_ = c.Close()
		return false
	}
	p.conns = append(p.conns, c)
	return true
}

// forward copies the server's bytes to c, and c's bytes to the server until c
// sends heldSubject. The sockets of a held client stay open until close.
func (p *heldProxy) forward(c net.Conn) {
	s, err := net.Dial("tcp", p.server)
	if err != nil {
		_ = c.Close()
		return
	}
	if !p.track(s) {
		_ = c.Close()
		return
	}
	p.pumps.Go(func() { _, _ = io.Copy(c, s) })
	marker := []byte(heldSubject)
	buf := make([]byte, 32<<10)
	var tail []byte // the end of the bytes read so far, for a marker split across reads
	for {
		n, err := c.Read(buf)
		if n > 0 {
			if _, werr := s.Write(buf[:n]); werr != nil {
				err = werr
			}
			window := append(tail, buf[:n]...)
			if bytes.Contains(window, marker) {
				p.holdOnce.Do(func() { close(p.held) })
				return
			}
			tail = window[max(0, len(window)-len(marker)+1):]
		}
		if err != nil {
			_ = s.Close()
			_ = c.Close()
			return
		}
	}
}

// close closes the listener and every socket the proxy holds, and waits for
// its goroutines.
func (p *heldProxy) close() {
	p.closeOnce.Do(func() {
		_ = p.ln.Close()
		<-p.accepting
		p.mu.Lock()
		p.closed = true
		conns := p.conns
		p.mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
		p.pumps.Wait()
	})
}
