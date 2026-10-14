package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	neturl "net/url"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
)

const (
	clientPort = "4222/tcp"
	// dialTimeout bounds one TCP dial to a container already past its ready log; the caller's
	// context still bounds Start as a whole and is checked when the dial returns.
	dialTimeout = 5 * time.Second
	// maxLogBytes caps the container log a failed Start writes to the test log.
	maxLogBytes = 256 << 10
)

// readMappedPort reads the host port Docker mapped to the container's client port. It is the
// fixture's one test seam (Fixture.readPort): a test replaces it to fail the mapped-port phase
// and see the inspect in the error (#180).
func readMappedPort(ctx context.Context, c testcontainers.Container) (string, error) {
	p, err := c.MappedPort(ctx, clientPort)
	return p.Port(), err
}

// inspect describes the container's state and published ports, for a failed port read.
func inspect(ctx context.Context, c testcontainers.Container) (string, error) {
	info, err := c.Inspect(ctx)
	if err != nil {
		return "", err
	}
	var state, ports any = "unknown", "unknown"
	if info.State != nil {
		state = info.State.Status
	}
	if info.NetworkSettings != nil {
		ports = info.NetworkSettings.Ports
	}
	return fmt.Sprintf("state %v, ports %v", state, ports), nil
}

// containerLogs reads the container's log, up to maxLogBytes.
func containerLogs(ctx context.Context, c testcontainers.Container) ([]byte, error) {
	rc, err := c.Logs(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(io.LimitReader(rc, maxLogBytes))
}

// jetStreamReady returns once JetStream answers an account request.
func jetStreamReady(ctx context.Context, js jetstream.JetStream) error {
	_, err := js.AccountInfo(ctx)
	return err
}

// connect dials the broker under the caller's context and leaves nothing running when it returns.
// nats.Connect takes no context, so connect dials the TCP connection itself under ctx and hands it
// to nats.Connect; when ctx ends during the handshake that connection is closed, which ends the
// handshake, and connect returns ctx's error. dialTimeout still bounds a handshake the broker never
// answers. No reconnects: a test broker that goes away is a failure to report, not to hide.
func connect(ctx context.Context, url string) (*nats.Conn, error) {
	u, err := neturl.Parse(url)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: dialTimeout}
	raw, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, err
	}
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = raw.Close()
		close(closed)
	})
	nc, err := nats.Connect(url, nats.Timeout(dialTimeout), nats.MaxReconnects(0), nats.SetCustomDialer(dialled{raw}))
	if !stop() {
		<-closed // the close already began; join it
	}
	if cerr := ctx.Err(); cerr != nil {
		if nc != nil {
			nc.Close()
		}
		_ = raw.Close()
		return nil, cerr
	}
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	return nc, nil
}

// dialled hands nats.Connect the connection connect already dialled. With MaxReconnects(0)
// nats.go dials once, so the one connection is all it is ever asked for.
type dialled struct{ conn net.Conn }

func (d dialled) Dial(string, string) (net.Conn, error) { return d.conn, nil }

// drainAndObserveClosed drains the connection and returns only once it is observed closed, or
// with the caller's context error while it is still draining.
func drainAndObserveClosed(ctx context.Context, nc *nats.Conn) error {
	if !nc.IsClosed() && !nc.IsDraining() {
		if err := nc.Drain(); err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
			return err
		}
	}
	_, err := probe.Await(ctx, func(context.Context) (bool, error) { return nc.IsClosed(), nil },
		func(closed bool) bool { return closed })
	return err
}
