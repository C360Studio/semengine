package websocket

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/c360studio/semengine/pkg/graphview"
)

const (
	// clientQueueFrames is how many frames wait for one client's writer; a frame queued while it is
	// full drops the oldest (design D2, the stalled-client row).
	clientQueueFrames = 256
	// writeWait bounds each write to a client, as the pin's (websocket.go:1743 at 8b99efe9): a
	// client whose write is blocked that long is closed.
	writeWait = 10 * time.Second
	// closeWait bounds the close frame Stop sends: a client that cannot take it that soon is closed
	// without it.
	closeWait = time.Second
)

// client is one upgraded connection. Its handler's goroutine is the connection's only reader and
// its writer goroutine the only writer: every frame, ping and close frame goes through the writer.
type client struct {
	conn *websocket.Conn

	// Applied state only: snapshot is the frames sent first, and sub the changes after them. The
	// writer takes a batch from sub only once it has written the one before, so while a client is
	// behind, its later changes merge per key in the view (design D2).
	snapshot frames
	sub      *stateSubscription

	// mu guards frames, oldest first, at most clientQueueFrames. ready holds one signal that
	// frames were queued.
	mu     sync.Mutex
	frames [][]byte
	ready  chan struct{}

	// closing is closed by Stop: the writer sends close 1001 and ends. readerDone is closed when
	// the reader has ended: the writer ends.
	closing    chan struct{}
	closeOnce  sync.Once
	readerDone chan struct{}

	// endReason is why the connection ended, its client_disconnections_total reason: the first
	// recorded by end, read once the reader and the writer have both returned.
	endOnce   sync.Once
	endReason string
}

func newClient(conn *websocket.Conn) *client {
	return &client{
		conn:       conn,
		ready:      make(chan struct{}, 1),
		closing:    make(chan struct{}),
		readerDone: make(chan struct{}),
	}
}

// enqueue queues frame for the writer and reports whether the queue was full, in which case its
// oldest frame was dropped to make room.
func (cl *client) enqueue(frame []byte) (droppedOldest bool) {
	cl.mu.Lock()
	if len(cl.frames) == clientQueueFrames {
		cl.frames[0] = nil
		cl.frames = cl.frames[1:]
		droppedOldest = true
	}
	cl.frames = append(cl.frames, frame)
	cl.mu.Unlock()
	select {
	case cl.ready <- struct{}{}:
	default: // a signal is already waiting; the writer drains every queued frame on it
	}
	return droppedOldest
}

// next removes and returns the oldest queued frame, or reports false when none is queued.
func (cl *client) next() ([]byte, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if len(cl.frames) == 0 {
		return nil, false
	}
	frame := cl.frames[0]
	cl.frames[0] = nil
	cl.frames = cl.frames[1:]
	return frame, true
}

// stop asks the writer to send close 1001 and end. It never blocks.
func (cl *client) stop() { cl.closeOnce.Do(func() { close(cl.closing) }) }

// end records reason as why the connection ended unless a reason was recorded first. The reader
// and the writer each record theirs before they close the connection, so the side whose failure
// ended it is the one recorded, not the side that saw the closed connection after.
func (cl *client) end(reason string) { cl.endOnce.Do(func() { cl.endReason = reason }) }

// serveUpgrade upgrades one admitted request and serves the client until it ends, so the
// request's admission lasts as long as the client: cleanup's wait on requests joins it.
func (c *output) serveUpgrade(w http.ResponseWriter, r *http.Request) {
	var snapshot frames
	var sub *stateSubscription
	if c.settings.subject == "" {
		// Attach before the upgrade, so a view that is not caught up is answered with 503 naming why
		// (design D2, D3). The subscription ends with the request, or when this handler returns.
		state, attached, notReady := c.subscribeState(r.Context())
		if attached == nil {
			http.Error(w, notReady, http.StatusServiceUnavailable)
			return
		}
		defer attached.Unsubscribe()
		snapshot, sub = snapshotFrames(state), attached
	}
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has answered the request with an HTTP error.
		c.metrics.errors.WithLabelValues(c.listener, errorUpgrade).Inc()
		return
	}
	c.metrics.connections.Inc()
	cl := newClient(conn)
	cl.snapshot, cl.sub = snapshot, sub
	if !c.attach(cl) {
		cl.stop() // Stop has begun: the writer sends close 1001 at once
	}
	defer c.detach(cl)
	written := make(chan struct{})
	go func() {
		defer close(written)
		c.write(cl)
	}()
	c.read(cl)
	close(cl.readerDone)
	// Closing the connection ends a write the writer is blocked in, so the join below waits for
	// no write deadline.
	_ = conn.Close()
	<-written
}

// read discards every frame the client sends until a read fails. The read deadline starts at twice
// ping_interval and each pong extends it by as much, so a client that answers pings stays connected
// however long it sends nothing, and one that answers none is closed (design D2, S18). Probe P1 on
// PR #592: at gorilla/websocket v1.5.3, a pong that does not extend the deadline leaves the read to
// time out with pongs received; TestQuietClientStaysConnected fails without the extension.
func (c *output) read(cl *client) {
	deadline := 2 * c.settings.pingInterval
	// Setting a deadline fails only on a closed connection, whose next read fails and ends the loop.
	_ = cl.conn.SetReadDeadline(time.Now().Add(deadline))
	cl.conn.SetPongHandler(func(string) error {
		return cl.conn.SetReadDeadline(time.Now().Add(deadline))
	})
	for {
		_, frame, err := cl.conn.NextReader()
		if err == nil {
			_, err = io.Copy(io.Discard, frame)
		}
		if err != nil {
			cl.end(readEndReason(err))
			return
		}
	}
}

// readEndReason is the disconnection reason for the error that ended a client's reads: the read
// deadline passing (TestSilentClientIsClosed drives it), or anything else.
func readEndReason(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return disconnectReadTimeout
	}
	return disconnectClientClosed
}

// write is the client's only writer: an applied-state client's snapshot, then queued frames or
// changes in order, a ping every ping_interval, between frames too, and close 1001 when Stop asks or
// 1013 when the view's watch is lost. It ends at the first failed write, when the reader has ended,
// or after the close frame, and closes the connection so the reader ends too.
func (c *output) write(cl *client) {
	// A second Close, after serveUpgrade's, has nothing left to release.
	defer func() { _ = cl.conn.Close() }()
	ping := time.NewTicker(c.settings.pingInterval)
	defer ping.Stop()
	if cl.snapshot != nil && !c.drain(cl, ping, cl.snapshot) {
		return
	}
	var changes <-chan []graphview.Delta[storedState] // nil for a subject client: never ready
	if cl.sub != nil {
		changes = cl.sub.Deltas()
	}
	for {
		select {
		case <-cl.readerDone:
			return
		case <-cl.closing:
			c.sendClose(cl)
			return
		case <-ping.C:
			if !c.ping(cl) {
				return
			}
		case <-cl.ready:
			if !c.drain(cl, ping, cl.next) {
				return
			}
		case batch, ok := <-changes:
			if !ok {
				c.sendEnded(cl)
				return
			}
			if !c.drain(cl, ping, batchFrames(batch)) {
				return
			}
		}
	}
}

// drain writes every frame next gives, reporting false when the connection ended instead. A ping due
// between two frames goes before the next, or a client kept busy past its read deadline would be
// closed while reading (TestBusyClientKeepsPings), and a stop is taken between frames too.
func (c *output) drain(cl *client, ping *time.Ticker, next frames) bool {
	for {
		select {
		case <-cl.closing:
			c.sendClose(cl)
			return false
		case <-ping.C:
			if !c.ping(cl) {
				return false
			}
		default:
		}
		frame, ok := next()
		if !ok {
			return true
		}
		if c.send(cl, websocket.TextMessage, frame) != nil {
			cl.end(disconnectWriteFailed)
			return false
		}
		c.metrics.sent.Inc()
		c.metrics.bytesSent.Add(float64(len(frame)))
		c.metrics.sizes.Observe(float64(len(frame)))
	}
}

// ping sends a ping, reporting false when the write failed and the connection ends.
func (c *output) ping(cl *client) bool {
	if c.send(cl, websocket.PingMessage, nil) != nil {
		cl.end(disconnectWriteFailed)
		return false
	}
	return true
}

func (c *output) send(cl *client, messageType int, data []byte) error {
	if err := cl.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return cl.conn.WriteMessage(messageType, data)
}

// sendClose records the stop as why the connection ends, then sends close 1001. The connection is
// closed next whether or not the frame was sent.
func (c *output) sendClose(cl *client) {
	cl.end(disconnectStopped)
	writeClose(cl, websocket.CloseGoingAway, "websocket output stopping")
}

// sendEnded closes an applied-state client whose subscription ended: with 1013 when the view's watch
// was lost, so the client reconnects for a fresh snapshot once a new view has caught up (design D2),
// and as a stop otherwise. Stop ends the view only after every client has returned, so a client
// whose subscription ended for any other reason is one whose request has ended.
func (c *output) sendEnded(cl *client) {
	if !errors.Is(cl.sub.Err(), graphview.ErrWatcherLost) {
		c.sendClose(cl)
		return
	}
	cl.end(disconnectWatchLost)
	writeClose(cl, websocket.CloseTryAgainLater, "applied-state watch lost; reconnect for a fresh snapshot")
}

// writeClose sends a close frame with code and text under closeWait. The connection is closed next
// whether or not the frame was sent.
func writeClose(cl *client, code int, text string) {
	if cl.conn.SetWriteDeadline(time.Now().Add(closeWait)) != nil {
		return
	}
	_ = cl.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, text))
}

// dataFrame is the data frame for a subject message: its subject, the receive time in Unix
// milliseconds, and its bytes unchanged as the payload. The bytes are placed by hand, not through
// a json.RawMessage field, so the payload is exactly what was published: TestSubjectFanOut
// publishes bytes a re-encode would change.
func dataFrame(subject string, payload []byte) []byte {
	quoted, _ := json.Marshal(subject) // a string always marshals
	frame := make([]byte, 0, len(payload)+len(quoted)+64)
	frame = append(frame, `{"type":"data","subject":`...)
	frame = append(frame, quoted...)
	frame = append(frame, `,"timestamp":`...)
	frame = strconv.AppendInt(frame, time.Now().UnixMilli(), 10)
	frame = append(frame, `,"payload":`...)
	frame = append(frame, payload...)
	return append(frame, '}')
}
