package natsfixture

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
)

// errNotFound is the error testcontainers' MappedPort returns when Docker Desktop has left the
// container's port list empty (#180).
var errNotFound = errors.New(`port "4222/tcp" not found`)

// restartOnFakes returns a started-looking fixture whose Restart runs on fakes: each start adds a
// ready line to the container's log, connect and JetStream answer, and the port read fails for its
// first fails reads, then returns "2".
func restartOnFakes(t *testing.T, fails int) *Fixture {
	t.Helper()
	f := startedWith(t)
	f.container, f.nc, f.url = struct{ testcontainers.Container }{}, &nats.Conn{}, "nats://fixture-host:1"
	boots, reads := 1, 0
	f.deps.drain = func(context.Context, *nats.Conn) error { return nil }
	f.deps.stopContainer = func(context.Context, testcontainers.Container) error { return nil }
	f.deps.startContainer = func(context.Context, testcontainers.Container) error { boots++; return nil }
	f.deps.logs = func(context.Context, testcontainers.Container) ([]byte, error) {
		return []byte(strings.Repeat(readyLog+"\n", boots)), nil
	}
	f.deps.mappedPort = func(context.Context, testcontainers.Container) (string, error) {
		if reads++; reads <= fails {
			return "", errNotFound
		}
		return "2", nil
	}
	f.deps.inspect = func(context.Context, testcontainers.Container) (string, error) { return "state running", nil }
	f.deps.connect = func(context.Context, string) (*nats.Conn, error) { return &nats.Conn{}, nil }
	f.deps.jsReady = func(context.Context, jetstream.JetStream) error { return nil }
	return f
}

// fixtureErrors lists the *Error values err carries: err itself, or each one errors.Join joined.
func fixtureErrors(err error) []*Error {
	if fe, ok := err.(*Error); ok {
		return []*Error{fe}
	}
	var out []*Error
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range j.Unwrap() {
			out = append(out, fixtureErrors(e)...)
		}
	}
	return out
}

// A mapped-port failure after a restart, with the caller's context still live, earns one more
// stop, start and port read on the same container (maxAttempts says why); a second failure, or a
// cancelled context, is returned, and no container is replaced (nats-fixture › "Mapped-port failure
// after a restart").
func TestRestartTriesOnceMoreAfterAMappedPortFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fails  int  // port reads that fail before one succeeds
		cancel bool // the caller's context ends as the first read fails
		// bounces is the stop-container, start-container and mapped-port call count each.
		bounces int
		// attempts are the *Error attempts Restart returns, in order; none means it returns nil.
		attempts []int
	}{
		{name: "second read succeeds", fails: 1, bounces: 2},
		{name: "second read fails", fails: 3, bounces: 2, attempts: []int{1, 2}},
		{name: "cancelled context", fails: 1, cancel: true, bounces: 1, attempts: []int{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := restartOnFakes(t, tc.fails)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancel {
				read := f.deps.mappedPort
				f.deps.mappedPort = func(ctx context.Context, c testcontainers.Container) (string, error) {
					cancel()
					return read(ctx, c)
				}
			}

			err := f.Restart(ctx)
			fes := fixtureErrors(err)
			if len(tc.attempts) == 0 {
				if err != nil {
					t.Fatalf("Restart = %v, want nil after one more stop, start and port read", err)
				}
				if f.URL() != "nats://fixture-host:2" || f.JetStream() == nil {
					t.Fatalf("after Restart: URL %q, JetStream %v; want the second read's binding", f.URL(), f.JetStream())
				}
			} else if len(fes) != len(tc.attempts) {
				t.Fatalf("Restart = %v, want %d *Error(s)", err, len(tc.attempts))
			}
			for i, fe := range fes {
				if fe.Attempt != tc.attempts[i] || fe.Phase != PhaseMappedPort || !errors.Is(fe, errNotFound) || fe.ContainerID != "c0ffee" {
					t.Fatalf("Restart error %d = %v, want attempt %d at phase %s wrapping the not-found error", i, fe, tc.attempts[i], PhaseMappedPort)
				}
				if live := fe.ParentErr == nil; live == tc.cancel {
					t.Fatalf("Restart error %d parent context %v, want cancelled %t", i, fe.ParentErr, tc.cancel)
				}
			}
			calls := f.callCounts()
			for _, dep := range []string{"stopContainer", "startContainer", "mappedPort"} {
				if calls[dep] != tc.bounces {
					t.Errorf("%s called %d times, want %d", dep, calls[dep], tc.bounces)
				}
			}
			if calls["start"] != 0 || calls["terminate"] != 0 || calls["restart"] != 1 {
				t.Errorf("calls %v: Restart replaced or removed the container", calls)
			}
		})
	}
}

// A mapped-port failure names what an inspect of the container showed, or that the inspect
// failed, at Start and at Restart, so the next occurrence of #180 names its own cause.
func TestMappedPortFailureCarriesTheInspect(t *testing.T) {
	shown := "state running, ports map[4222/tcp:[]]"
	refused := errors.New("inspect refused")
	// inspectOnce shows the container the first time and fails every time after.
	inspectOnce := func(f *Fixture) {
		inspects := 0
		f.deps.inspect = func(context.Context, testcontainers.Container) (string, error) {
			if inspects++; inspects == 1 {
				return shown, nil
			}
			return "", refused
		}
	}
	check := func(t *testing.T, op string, err error) {
		t.Helper()
		fes := fixtureErrors(err)
		if len(fes) != 2 {
			t.Fatalf("%s = %v, want two mapped-port *Errors", op, err)
		}
		if msg := fes[0].Error(); !strings.Contains(msg, errNotFound.Error()) || !strings.Contains(msg, shown) {
			t.Errorf("%s first error %q does not name what the inspect showed (%q)", op, msg, shown)
		}
		if msg := fes[1].Error(); !strings.Contains(msg, "inspect") || !strings.Contains(msg, refused.Error()) {
			t.Errorf("%s second error %q does not say the inspect failed with %q", op, msg, refused)
		}
	}

	t.Run("Restart", func(t *testing.T) {
		f := restartOnFakes(t, 2)
		inspectOnce(f)
		check(t, "Restart", f.Restart(t.Context()))
	})
	t.Run("Start", func(t *testing.T) {
		plantLock(t, "tok", "tok")
		f := New(t)
		f.deps.start = func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
			return idContainer{}, nil
		}
		f.deps.host = func(context.Context, testcontainers.Container) (string, error) { return "fixture-host", nil }
		f.deps.mappedPort = func(context.Context, testcontainers.Container) (string, error) { return "", errNotFound }
		f.deps.logs = func(context.Context, testcontainers.Container) ([]byte, error) { return nil, nil }
		f.deps.terminate = func(context.Context, testcontainers.Container) error { return nil }
		f.deps.absent = func(context.Context, string) (bool, error) { return true, nil }
		inspectOnce(f)
		check(t, "Start", f.Start(t.Context()))
	})
}

// idContainer is a container with an id and nothing else: every other call goes through deps.
type idContainer struct{ testcontainers.Container }

func (idContainer) GetContainerID() string { return "c0ffee" }
