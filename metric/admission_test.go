package metric

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/pkg/security"
	"github.com/stretchr/testify/require"
)

// pausingListener wraps each accepted connection in a pausingConn.
type pausingListener struct {
	net.Listener
	conn chan *pausingConn
}

func (l *pausingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	pc := &pausingConn{Conn: c, reached: make(chan struct{}), release: make(chan struct{}), served: make(chan struct{})}
	l.conn <- pc
	return pc, nil
}

// pausingConn holds net/http's connection goroutine at its second SetReadDeadline, which in
// Go 1.26 is connReader.startBackgroundRead: after conn.serve has read the request and checked
// for shutdown, before the handler runs. Its second Close is conn.serve's own close as that
// goroutine finishes; the first is the server's force-close while the goroutine is held.
type pausingConn struct {
	net.Conn
	deadlines, closes atomic.Int32
	reached, release  chan struct{}
	served            chan struct{}
	servedOnce        sync.Once
}

func (c *pausingConn) SetReadDeadline(t time.Time) error {
	err := c.Conn.SetReadDeadline(t)
	if c.deadlines.Add(1) == 2 {
		close(c.reached)
		<-c.release
	}
	return err
}

func (c *pausingConn) Close() error {
	err := c.Conn.Close()
	if c.closes.Add(1) >= 2 {
		c.servedOnce.Do(func() { close(c.served) })
	}
	return err
}

// TestServerRefusesRequestAfterStopReportedCompletion (Codex review of PR #48, comment
// 5959412053, finding 1): a request whose connection goroutine passed net/http's shutdown check
// but had not reached the handler when Stop reported completion must never start collection.
// Stop closes admission; the late request is refused without running the handler.
func TestServerRefusesRequestAfterStopReportedCompletion(t *testing.T) {
	registry := NewMetricsRegistry()
	collector := newExitingCollector()
	registry.PrometheusRegistry().MustRegister(collector)
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(collector.release) }) })

	raw := boundServerListener(t)
	listener := &pausingListener{Listener: raw, conn: make(chan *pausingConn, 1)}
	server := NewServer(9090, "/metrics", registry, security.Config{})
	require.NoError(t, server.StartWithListener(t.Context(), listener))

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := testServerGET(t, server.Address(), nil)
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	conn := <-listener.conn
	<-conn.reached

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, server.Stop(ended), context.Canceled)
	require.NoError(t, server.Stop(t.Context()), "no request was admitted when Stop ran")

	close(conn.release)
	select {
	case <-collector.entered:
		release.Do(func() { close(collector.release) })
		t.Fatal("a request reached collection after Stop had reported completion")
	case <-conn.served:
	}
	<-requestDone
	select {
	case <-collector.entered:
		t.Fatal("a request reached collection after Stop had reported completion")
	default:
	}
	require.NoError(t, server.Stop(t.Context()))
}

// TestAdmittedRequestsRefuseAfterClose: the refusal itself, at the wrapper. A request arriving
// after admission closed gets 503 and its handler never runs; admission closed with nothing
// running is observed as complete.
func TestAdmittedRequestsRefuseAfterClose(t *testing.T) {
	var requests admittedRequests
	ran := false
	handler := requests.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { ran = true }))
	requests.closeAdmission()
	recorder := &statusRecorder{header: http.Header{}}
	handler.ServeHTTP(recorder, &http.Request{})
	require.False(t, ran, "a request after admission closed must not run the handler")
	require.Equal(t, http.StatusServiceUnavailable, recorder.status)
	require.Zero(t, requests.pending())
	require.NoError(t, requests.wait(t.Context()))
}

type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header         { return r.header }
func (r *statusRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *statusRecorder) WriteHeader(status int)      { r.status = status }
