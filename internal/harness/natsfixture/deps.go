package natsfixture

import (
	"context"
	"errors"
	"io"
	"net"
	neturl "net/url"
	"strings"
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
	// maxLogBytes caps the container log copied into evidence on failure.
	maxLogBytes = 256 << 10
)

// deps is every call the fixture makes to Docker or NATS. Tests replace entries to fail or block
// a phase; every call is counted, so a test can prove a path made none.
type deps struct {
	start        func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error)
	host         func(context.Context, testcontainers.Container) (string, error)
	mappedPort   func(context.Context, testcontainers.Container) (string, error)
	connect      func(context.Context, string) (*nats.Conn, error)
	jsReady      func(context.Context, jetstream.JetStream) error
	createStream func(context.Context, jetstream.JetStream, jetstream.StreamConfig) (jetstream.Stream, error)
	createKV     func(context.Context, jetstream.JetStream, jetstream.KeyValueConfig) (jetstream.KeyValue, error)
	terminate    func(context.Context, testcontainers.Container) error
	// stopContainer and startContainer stop and start the same container, for Restart.
	stopContainer  func(context.Context, testcontainers.Container) error
	startContainer func(context.Context, testcontainers.Container) error
	drain          func(context.Context, *nats.Conn) error
	logs           func(context.Context, testcontainers.Container) ([]byte, error)
	absent         func(context.Context, string) (bool, error)
}

func defaultDeps() deps {
	return deps{
		start: func(ctx context.Context, req testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
			return testcontainers.GenericContainer(ctx, req)
		},
		host: func(ctx context.Context, c testcontainers.Container) (string, error) { return c.Host(ctx) },
		mappedPort: func(ctx context.Context, c testcontainers.Container) (string, error) {
			p, err := c.MappedPort(ctx, clientPort)
			return p.Port(), err
		},
		connect: connect,
		jsReady: func(ctx context.Context, js jetstream.JetStream) error {
			_, err := js.AccountInfo(ctx)
			return err
		},
		createStream: func(ctx context.Context, js jetstream.JetStream, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
			return js.CreateStream(ctx, cfg)
		},
		createKV: func(ctx context.Context, js jetstream.JetStream, cfg jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
			return js.CreateKeyValue(ctx, cfg)
		},
		terminate: func(ctx context.Context, c testcontainers.Container) error { return c.Terminate(ctx) },
		// A nil timeout leaves the stop grace period to Docker (the container's StopTimeout, else
		// the engine default); ctx bounds the call.
		stopContainer:  func(ctx context.Context, c testcontainers.Container) error { return c.Stop(ctx, nil) },
		startContainer: func(ctx context.Context, c testcontainers.Container) error { return c.Start(ctx) },
		drain:          drainAndObserveClosed,
		logs: func(ctx context.Context, c testcontainers.Container) ([]byte, error) {
			rc, err := c.Logs(ctx)
			if err != nil {
				return nil, err
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(io.LimitReader(rc, maxLogBytes))
		},
		absent: containerAbsent,
	}
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

// containerAbsent asks the daemon directly: after Terminate, testcontainers closes the container's
// own client, so absence is observed through a fresh one, reached through testcontainers' exported
// types so the Docker client packages stay indirect dependencies.
func containerAbsent(ctx context.Context, id string) (bool, error) {
	cli, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		return false, err
	}
	provider := &testcontainers.DockerProvider{}
	provider.SetClient(cli)
	defer func() { _ = provider.Close() }()
	probe := &testcontainers.DockerContainer{ID: id}
	probe.SetProvider(provider)
	_, err = probe.Inspect(ctx)
	if err == nil {
		return false, nil
	}
	// The daemon's not-found error is recognisable only through containerd's errdefs sentinel,
	// which would make another module a direct dependency. Its message has been stable since
	// Docker 1.x; if it ever changes, this fails closed: Stop reports the error instead of claiming
	// the container absent.
	if strings.Contains(err.Error(), "No such container") {
		return true, nil
	}
	return false, err
}
