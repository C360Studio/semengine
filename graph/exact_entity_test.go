package graph

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/pkg/errs"
)

type exactEntityRequesterFunc func(context.Context, string, []byte, time.Duration) ([]byte, error)

func (fn exactEntityRequesterFunc) RequestClassified(
	ctx context.Context,
	subject string,
	data []byte,
	timeout time.Duration,
) ([]byte, error) {
	return fn(ctx, subject, data, timeout)
}

func TestExactEntityReaderClassifiesInvalidIDBeforeTransport(t *testing.T) {
	called := false
	requester := exactEntityRequesterFunc(func(context.Context, string, []byte, time.Duration) ([]byte, error) {
		called = true
		return nil, nil
	})
	_, err := newTestExactEntityReader(t, requester).ReadExactEntity(context.Background(), "not-six-parts")
	if called {
		t.Fatal("invalid entity ID reached transport")
	}
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorInvalid || classified.Code != ErrorCodeInvalidRequest {
		t.Fatalf("error = %v, want invalid/invalid_request", err)
	}
}

// ReadExactEntity is an exported boundary that returns an error, so a nil context is
// refused there, before any request (developer contract, "Context ownership"). The
// requester here would answer a valid read, so only the boundary's own check can refuse.
func TestExactEntityReaderRefusesNilContextBeforeTransport(t *testing.T) {
	const entityID = "acme.ops.robotics.gcs.drone.001"
	var nilCtx context.Context
	called := false
	requester := exactEntityRequesterFunc(func(context.Context, string, []byte, time.Duration) ([]byte, error) {
		called = true
		return []byte(`{"entity":{"id":"` + entityID + `","triples":[]},"kvRevision":17}`), nil
	})
	exact, err := newTestExactEntityReader(t, requester).ReadExactEntity(nilCtx, entityID)
	if err == nil || called || exact != nil {
		t.Fatalf("ReadExactEntity(nil) = (%+v, %v), requester called = %v; want a refusal before any request",
			exact, err, called)
	}
}

func TestExactEntityReaderReturnsValidatedEntityAndRevision(t *testing.T) {
	const entityID = "acme.ops.robotics.gcs.drone.001"
	requester := exactEntityRequesterFunc(func(_ context.Context, subject string, data []byte, _ time.Duration) ([]byte, error) {
		if subject != "graph.ingest.query.entity" {
			t.Fatalf("subject = %q", subject)
		}
		var request struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &request); err != nil || request.ID != entityID {
			t.Fatalf("request = %s, err = %v", data, err)
		}
		return []byte(`{"entity":{"id":"` + entityID + `","version":999,"triples":[]},"kvRevision":17}`), nil
	})

	reader := newTestExactEntityReader(t, requester)
	exact, err := reader.ReadExactEntity(context.Background(), entityID)
	if err != nil {
		t.Fatalf("ReadExactEntity: %v", err)
	}
	if exact.Entity == nil || exact.Entity.ID != entityID || exact.KVRevision != 17 {
		t.Fatalf("exact = %#v", exact)
	}
	// The reply's entity carries a stale "version" key (a value written by the pin);
	// the revision reported is the KV revision, and the key is ignored on decode.
}

func TestExactEntityReaderRejectsZeroRevisionAndMismatchedEntity(t *testing.T) {
	const entityID = "acme.ops.robotics.gcs.drone.001"
	tests := []string{
		`{"entity":{"id":"` + entityID + `","triples":[]},"kvRevision":0}`,
		`{"entity":{"id":"acme.ops.robotics.gcs.drone.002","triples":[]},"kvRevision":2}`,
	}
	for _, response := range tests {
		requester := exactEntityRequesterFunc(func(context.Context, string, []byte, time.Duration) ([]byte, error) {
			return []byte(response), nil
		})
		if exact, err := newTestExactEntityReader(t, requester).ReadExactEntity(context.Background(), entityID); err == nil {
			t.Fatalf("response %s returned %#v", response, exact)
		}
	}
}

func newTestExactEntityReader(t *testing.T, requester exactEntityRequesterFunc) ExactEntityReader {
	t.Helper()
	reader, err := NewExactEntityReader(requester, time.Second)
	if err != nil {
		t.Fatalf("NewExactEntityReader: %v", err)
	}
	return reader
}

// A negative timeout would give every request a context that has already
// expired, so each read would fail while the caller's context is live and count
// against the shared client's breaker (#151). The constructor refuses it. Zero
// stays valid and reaches the requester as zero, whose own default applies (D16).
func TestNewExactEntityReaderRefusesNegativeTimeoutAndPassesZeroThrough(t *testing.T) {
	const entityID = "acme.ops.robotics.gcs.drone.001"
	var timeouts []time.Duration
	requester := exactEntityRequesterFunc(func(_ context.Context, _ string, _ []byte, timeout time.Duration) ([]byte, error) {
		timeouts = append(timeouts, timeout)
		return []byte(`{"entity":{"id":"` + entityID + `","triples":[]},"kvRevision":17}`), nil
	})

	if reader, err := NewExactEntityReader(requester, -time.Nanosecond); err == nil || reader != nil {
		t.Fatalf("NewExactEntityReader(-1ns) = (%v, %v), want (nil, error)", reader, err)
	}

	reader, err := NewExactEntityReader(requester, 0)
	if err != nil {
		t.Fatalf("NewExactEntityReader(0): %v, want a reader: zero means the requester's default", err)
	}
	if _, err := reader.ReadExactEntity(context.Background(), entityID); err != nil {
		t.Fatalf("ReadExactEntity: %v", err)
	}
	if len(timeouts) != 1 || timeouts[0] != 0 {
		t.Fatalf("requester saw timeouts %v, want [0]: a zero timeout is passed through", timeouts)
	}
}
