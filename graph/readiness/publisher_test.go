package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// fakeWriter captures Put calls. It is the narrow producer-side method set, so the
// test needs neither a live NATS nor the other 19 jetstream.KeyValue methods.
type fakeWriter struct {
	key   string
	value []byte
	calls int
	err   error
}

func (w *fakeWriter) Put(_ context.Context, key string, value []byte) (uint64, error) {
	w.calls++
	w.key = key
	// Copy: the caller owns the marshal buffer and a later tick may reuse it.
	w.value = append([]byte(nil), value...)
	if w.err != nil {
		return 0, w.err
	}
	return uint64(w.calls), nil
}

// TestGraphStatusBucketShapeIsTheCatalogRow: every producer acquires GRAPH_STATUS with
// kvcatalog.EnsureCatalogBucket, which reads the ONE catalog descriptor — so producers
// structurally cannot ask for different shapes of the same bucket. The test pins the
// descriptor they acquire under.
func TestGraphStatusBucketShapeIsTheCatalogRow(t *testing.T) {
	spec, ok := kvcatalog.SpecFor(graph.BucketGraphStatus)
	if !ok {
		t.Fatalf("catalog must declare %s", graph.BucketGraphStatus)
	}
	if spec.History != 3 {
		t.Errorf("catalog History = %d, want 3 (enough replay to see recent transitions)", spec.History)
	}
	// The readiness bucket is NOT the live graph, but ADR-068 discipline holds:
	// no TTL, no size-based eviction. Freshness is judged consumer-side.
	if spec.Retention.Kind != natsclient.RetentionNoLifecycle {
		t.Errorf("catalog retention = %q, want no-lifecycle (freshness is consumer-side, ADR-083 D2)",
			spec.Retention.Kind)
	}
}

// TestPublisher_ValueIsPlainEnvelopeJSON pins the wire contract from BOTH ends: the
// producer writes a bare graph.IndexStatusResponse (a KV value, NOT a payload-registry
// BaseMessage publish) and the production consumer — the real Watcher — decodes it
// back to an equal envelope. A registry envelope here would round-trip through neither.
func TestPublisher_ValueIsPlainEnvelopeJSON(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		status graph.IndexStatusResponse
	}{
		{
			name: "ready",
			key:  KeyGraphIndex,
			status: graph.IndexStatusResponse{
				Ready: true, State: graph.IndexStateReady,
				IndexedRevision: 500, TargetRevision: 500,
			},
		},
		{
			name: "building with staleness",
			key:  KeyGraphEmbedding,
			status: graph.IndexStatusResponse{
				State:           graph.IndexStateBuilding,
				IndexedRevision: 480, TargetRevision: 500, Lag: 20, StalenessMs: 1200,
			},
		},
		{
			name: "reset required carries code and reason",
			key:  KeyGraphIndex,
			status: graph.IndexStatusResponse{
				State:  graph.IndexStateResetRequired,
				Code:   graph.ErrorCodeGraphStateResetRequired,
				Reason: string(graph.GraphStateReasonNoncanonicalPredicate),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := &fakeWriter{}
			pub := newTestPublisher(t, writer, tt.key)

			if err := pub.Publish(t.Context(), tt.status); err != nil {
				t.Fatalf("Publish: %v", err)
			}
			if writer.key != tt.key {
				t.Errorf("key = %q, want %q", writer.key, tt.key)
			}

			// No payload-registry envelope: the value is the struct itself, so a
			// BaseMessage discriminator must not appear.
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(writer.value, &raw); err != nil {
				t.Fatalf("value is not a JSON object: %v", err)
			}
			for _, envelopeKey := range []string{"type", "payload", "message_type"} {
				if _, found := raw[envelopeKey]; found {
					t.Errorf("value carries payload-registry envelope key %q: %s", envelopeKey, writer.value)
				}
			}

			// The production consumer decode path, not a local json.Unmarshal. The
			// watcher harness runs in a synctest bubble (watcher_test.go).
			synctest.Test(t, func(t *testing.T) {
				h := newHarness(t, DefaultHeartbeat)
				h.deliver(fakeEntry{key: tt.key, value: writer.value, rev: 1, op: jetstream.KeyValuePut})

				got := h.watcher.Read()
				if !got.Known {
					t.Fatal("watcher did not accept the published value")
				}
				// PublishedAt is the publisher's stamp (TestPublishStampsPublishedAt);
				// every other field is the caller's and round-trips unchanged.
				want := tt.status
				want.PublishedAt = got.Status.PublishedAt
				if !reflect.DeepEqual(got.Status, want) {
					t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got.Status, want)
				}
			})
		})
	}
}

func TestPublisher_PutFailureIsReturnedNotSwallowed(t *testing.T) {
	// The tick loop must be able to count and log the failure, so it has to see it.
	boom := errors.New("no responders")
	pub := newTestPublisher(t, &fakeWriter{err: boom}, KeyGraphIndex)

	err := pub.Publish(t.Context(), graph.IndexStatusResponse{State: graph.IndexStateBuilding})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error does not wrap the Put cause: %v", err)
	}
}

// TestNewPublisherRefusesIncompleteWiring: a producer wired with no bucket or no key is
// refused at construction, naming the cause, so its Start fails instead of publishing
// nothing while consumers read the key as absent.
func TestNewPublisherRefusesIncompleteWiring(t *testing.T) {
	tests := []struct {
		name     string
		writer   StatusWriter
		key      string
		wantText string
	}{
		{name: "nil bucket", writer: nil, key: KeyGraphIndex, wantText: "bucket"},
		{name: "empty key", writer: &fakeWriter{}, key: "", wantText: "key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub, err := NewPublisher(tt.writer, tt.key)
			if err == nil {
				t.Fatalf("NewPublisher(%v, %q) = %+v, nil; want an error", tt.writer, tt.key, pub)
			}
			if pub != nil {
				t.Errorf("NewPublisher returned a publisher beside its error: %+v", pub)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("error %q does not name the missing %s", err, tt.wantText)
			}
		})
	}

	// The control: complete wiring builds a publisher for that key.
	pub, err := NewPublisher(&fakeWriter{}, KeyGraphIngest)
	if err != nil {
		t.Fatalf("NewPublisher with a bucket and a key: %v", err)
	}
	if pub.Key() != KeyGraphIngest {
		t.Errorf("Key() = %q, want %q", pub.Key(), KeyGraphIngest)
	}
}

// newTestPublisher builds a publisher the production way and fails the test on a
// refusal.
func newTestPublisher(t *testing.T, writer StatusWriter, key string) *Publisher {
	t.Helper()
	pub, err := NewPublisher(writer, key)
	if err != nil {
		t.Fatalf("NewPublisher(%q): %v", key, err)
	}
	return pub
}

// TestPublishStampsPublishedAt: every write's stored value carries published_at, the
// wall-clock time of the write in UTC, RFC 3339 with nanoseconds, whatever the caller
// passed (design D16; graph-transport-boundary, "The readiness envelope carries its
// publish time and no legacy fields").
func TestPublishStampsPublishedAt(t *testing.T) {
	tests := []struct {
		name   string
		passed string
	}{
		{name: "caller left it empty", passed: ""},
		{name: "caller's value is replaced", passed: "2001-02-03T04:05:06Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := &fakeWriter{}
			pub := newTestPublisher(t, writer, KeyGraphIndex)

			before := time.Now()
			status := graph.IndexStatusResponse{State: graph.IndexStateBuilding, PublishedAt: tt.passed}
			if err := pub.Publish(t.Context(), status); err != nil {
				t.Fatalf("Publish: %v", err)
			}
			after := time.Now()

			var stored struct {
				PublishedAt *string `json:"published_at"`
			}
			if err := json.Unmarshal(writer.value, &stored); err != nil {
				t.Fatalf("stored value is not JSON: %v", err)
			}
			if stored.PublishedAt == nil {
				t.Fatalf("stored value has no published_at: %s", writer.value)
			}
			at, err := time.Parse(time.RFC3339Nano, *stored.PublishedAt)
			if err != nil {
				t.Fatalf("published_at = %q, not RFC 3339: %v", *stored.PublishedAt, err)
			}
			if want := at.UTC().Format(time.RFC3339Nano); *stored.PublishedAt != want {
				t.Errorf("published_at = %q, want the UTC form %q", *stored.PublishedAt, want)
			}
			if at.Before(before) || at.After(after) {
				t.Errorf("published_at = %s, want it between %s and %s, the clock read around Publish",
					at.Format(time.RFC3339Nano), before.UTC().Format(time.RFC3339Nano), after.UTC().Format(time.RFC3339Nano))
			}
		})
	}
}

// TestPublishRefusesNilContext: a nil context is refused with an error before any write,
// never a panic in the tick loop (readiness-nil-context D1).
func TestPublishRefusesNilContext(t *testing.T) {
	var nilCtx context.Context
	tests := []struct {
		name      string
		ctx       context.Context
		wantErr   bool
		wantCalls int
	}{
		{name: "nil context", ctx: nilCtx, wantErr: true, wantCalls: 0},
		{name: "valid context", ctx: t.Context(), wantErr: false, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := &fakeWriter{}
			pub := newTestPublisher(t, writer, KeyGraphIndex)

			err := pub.Publish(tt.ctx, graph.IndexStatusResponse{State: graph.IndexStateBuilding})
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "nil context") {
					t.Errorf("Publish = %v, want an error naming nil context", err)
				}
			} else if err != nil {
				t.Errorf("Publish = %v, want nil", err)
			}
			if writer.calls != tt.wantCalls {
				t.Errorf("Put calls = %d, want %d", writer.calls, tt.wantCalls)
			}
		})
	}
}
