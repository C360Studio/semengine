//go:build integration

package natsfixture

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

// A mapped-port failure names what an inspect of the container showed, or why the inspect failed,
// at Start and at Restart, so the next occurrence of #180 names its own cause (nats-fixture › "A
// missing mapped port fails Start", "A missing mapped port fails Restart"). The read is the
// fixture's one test seam; the inspect is Docker's. To fail the inspect, the read cancels the
// caller's context before it fails: the inspect runs under that context. Each operation reads the
// port once and returns one error: neither tries again.
func TestMappedPortFailureCarriesTheInspect(t *testing.T) {
	ops := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, read func(context.Context, testcontainers.Container) (string, error)) error
	}{
		{"Start", func(t *testing.T, ctx context.Context, read func(context.Context, testcontainers.Container) (string, error)) error {
			f := New(t)
			f.readPort = read
			err := f.Start(ctx)
			if got := processContainers(t); len(got) != 0 {
				t.Errorf("containers of this process after the failed Start: %v, want none", got)
			}
			return err
		}},
		{"Restart", func(t *testing.T, ctx context.Context, read func(context.Context, testcontainers.Container) (string, error)) error {
			f := startFixture(t)
			id := f.containerID
			f.readPort = read
			rctx, cancel := context.WithTimeout(ctx, stopBound)
			defer cancel()
			err := f.Restart(rctx)
			if got := processContainers(t); !slices.Equal(got, []string{id}) {
				t.Errorf("containers of this process after the failed Restart: %v, want only %s", got, id)
			}
			if conn(f) != nil || f.URL() != "" || f.JetStream() != nil {
				t.Errorf("after the failed Restart: connection %v, URL %q, JetStream %v; want no connect and no binding", conn(f), f.URL(), f.JetStream())
			}
			if err := f.Restart(t.Context()); !errors.Is(err, errNotStarted) {
				t.Errorf("Restart after a failed Restart = %v, want the not-started refusal", err)
			}
			return err
		}},
	}
	for _, op := range ops {
		t.Run(op.name+"/inspect shows", func(t *testing.T) {
			reads := 0
			err := op.run(t, t.Context(), func(context.Context, testcontainers.Container) (string, error) {
				reads++
				return "", errNotFound
			})
			checkMappedPortError(t, err, reads, "(inspect: state running, ports ")
		})
		t.Run(op.name+"/inspect fails", func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reads := 0
			err := op.run(t, ctx, func(context.Context, testcontainers.Container) (string, error) {
				reads++
				cancel()
				return "", errNotFound
			})
			checkMappedPortError(t, err, reads, "(inspect failed: ")
			if !errors.Is(err, context.Canceled) {
				t.Errorf("errors.Is(%v, context.Canceled) = false: the cancelled caller is not told", err)
			}
		})
	}
}

func checkMappedPortError(t *testing.T, err error, reads int, inspect string) {
	t.Helper()
	if !errors.Is(err, errNotFound) {
		t.Fatalf("error = %v, want the not-found error", err)
	}
	for _, want := range []string{"phase mapped-port", inspect} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if reads != 1 {
		t.Errorf("%d port reads, want one: nothing is tried again", reads)
	}
}
