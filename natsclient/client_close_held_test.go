package natsclient

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
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
	require.True(t, conn.IsClosed(), "the connection is still open after Close returned")
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
