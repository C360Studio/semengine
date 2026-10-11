package natsclient

import (
	"context"
	"net"
	"sync"
	"time"
)

// socketDialer is the dialer nats.go uses for the connection of one Connect
// (nats.SetCustomDialer). It keeps the socket it dialled last, so cut can close
// it: that ends nats.go's wait for the server's answer at once, where nats.go
// would otherwise wait for its connect timeout (T4; P18, P25).
type socketDialer struct {
	timeout time.Duration
	dial    func(*net.Dialer, string, string) (net.Conn, error)
	done    chan struct{} // closed by cut: ends a dial in progress and refuses later ones

	mu   sync.Mutex // orders a dial's result against cut
	conn net.Conn
}

func newSocketDialer(timeout time.Duration, dial func(*net.Dialer, string, string) (net.Conn, error)) *socketDialer {
	return &socketDialer{timeout: timeout, dial: dial, done: make(chan struct{})}
}

// Dial implements nats.CustomDialer. That interface carries no context, and a
// callback may not invent one, so cut ends a dial in progress by closing the
// net.Dialer's Cancel channel (T4). The net package still honours that
// deprecated field (P23, P25; design D1).
func (d *socketDialer) Dial(network, address string) (net.Conn, error) {
	if d.isCut() {
		return nil, net.ErrClosed
	}
	c, err := d.dial(&net.Dialer{Timeout: d.timeout, Cancel: d.done}, network, address)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.isCut() {
		_ = c.Close()
		return nil, net.ErrClosed
	}
	d.conn = c
	return c, nil
}

// cutWhenDone arms a cut for when ctx ends. The stop it returns, called once,
// disarms it, or waits for a cut that has begun, so ctx touches nothing once
// stop has returned (T5).
func (d *socketDialer) cutWhenDone(ctx context.Context) (stop func()) {
	cut := make(chan struct{})
	stopCut := context.AfterFunc(ctx, func() { d.cut(); close(cut) })
	return func() {
		if !stopCut() {
			<-cut
		}
	}
}

// cut closes the socket, ends a dial in progress, and refuses every later dial.
func (d *socketDialer) cut() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.isCut() {
		return
	}
	close(d.done)
	if d.conn != nil {
		_ = d.conn.Close()
	}
}

func (d *socketDialer) isCut() bool {
	select {
	case <-d.done:
		return true
	default:
		return false
	}
}
