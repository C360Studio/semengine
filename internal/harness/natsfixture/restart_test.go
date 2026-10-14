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
// ready line to the container's log, the port read returns "2", and connect and JetStream answer.
func restartOnFakes(t *testing.T) *Fixture {
	t.Helper()
	f := startedWith(t)
	f.container, f.nc, f.url = struct{ testcontainers.Container }{}, &nats.Conn{}, "nats://fixture-host:1"
	boots := 1
	f.deps.drain = func(context.Context, *nats.Conn) error { return nil }
	f.deps.stopContainer = func(context.Context, testcontainers.Container) error { return nil }
	f.deps.startContainer = func(context.Context, testcontainers.Container) error { boots++; return nil }
	f.deps.logs = func(context.Context, testcontainers.Container) ([]byte, error) {
		return []byte(strings.Repeat(readyLog+"\n", boots)), nil
	}
	f.deps.mappedPort = func(context.Context, testcontainers.Container) (string, error) { return "2", nil }
	f.deps.inspect = func(context.Context, testcontainers.Container) (string, error) { return "state running", nil }
	f.deps.connect = func(context.Context, string) (*nats.Conn, error) { return &nats.Conn{}, nil }
	f.deps.jsReady = func(context.Context, jetstream.JetStream) error { return nil }
	return f
}

// A mapped-port failure names what an inspect of the container showed, or that the inspect
// failed, at Start and at Restart, so the next occurrence of #180 names its own cause. Each
// operation reads the port once and returns one *Error: neither tries again.
func TestMappedPortFailureCarriesTheInspect(t *testing.T) {
	shown := "state running, ports map[4222/tcp:[]]"
	refused := errors.New("inspect refused")
	ops := []struct {
		name string
		run  func(t *testing.T, inspect func(context.Context, testcontainers.Container) (string, error)) (*Fixture, error)
	}{
		{"Restart", func(t *testing.T, inspect func(context.Context, testcontainers.Container) (string, error)) (*Fixture, error) {
			f := restartOnFakes(t)
			f.deps.mappedPort = func(context.Context, testcontainers.Container) (string, error) { return "", errNotFound }
			f.deps.inspect = inspect
			return f, f.Restart(t.Context())
		}},
		{"Start", func(t *testing.T, inspect func(context.Context, testcontainers.Container) (string, error)) (*Fixture, error) {
			f := New(t)
			f.deps.start = func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
				return idContainer{}, nil
			}
			f.deps.host = func(context.Context, testcontainers.Container) (string, error) { return "fixture-host", nil }
			f.deps.mappedPort = func(context.Context, testcontainers.Container) (string, error) { return "", errNotFound }
			f.deps.logs = func(context.Context, testcontainers.Container) ([]byte, error) { return nil, nil }
			f.deps.terminate = func(context.Context, testcontainers.Container) error { return nil }
			f.deps.inspect = inspect
			return f, f.Start(t.Context())
		}},
	}
	inspects := []struct {
		name    string
		inspect func(context.Context, testcontainers.Container) (string, error)
		want    []string // text the error must carry beside the not-found error
	}{
		{"inspect shows", func(context.Context, testcontainers.Container) (string, error) { return shown, nil }, []string{shown}},
		{"inspect fails", func(context.Context, testcontainers.Container) (string, error) { return "", refused }, []string{"inspect", refused.Error()}},
	}
	for _, op := range ops {
		for _, in := range inspects {
			t.Run(op.name+"/"+in.name, func(t *testing.T) {
				f, err := op.run(t, in.inspect)
				var fe *Error
				if !errors.As(err, &fe) || fe.Phase != PhaseMappedPort || !errors.Is(err, errNotFound) {
					t.Fatalf("%s = %v, want one *Error at phase %s wrapping the not-found error", op.name, err, PhaseMappedPort)
				}
				for _, want := range in.want {
					if !strings.Contains(fe.Error(), want) {
						t.Errorf("%s error %q lacks %q", op.name, fe.Error(), want)
					}
				}
				if calls := f.callCounts(); calls["mappedPort"] != 1 || calls["inspect"] != 1 || calls["start"] > 1 || calls["startContainer"] > 1 {
					t.Errorf("calls %v: want one port read, one inspect and no second container start", calls)
				}
			})
		}
	}
}

// idContainer is a container with an id and nothing else: every other call goes through deps.
type idContainer struct{ testcontainers.Container }

func (idContainer) GetContainerID() string { return "c0ffee" }
