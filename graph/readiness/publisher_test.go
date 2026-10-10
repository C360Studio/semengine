package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
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

// TestEnsureBucket_OneShapeIsTheCatalogRow: both producers (graph-index and
// graph-embedding) route through EnsureBucket, which delegates to the ONE
// catalog descriptor — so they structurally cannot ask for different shapes of
// the same bucket. The test pins the descriptor both will acquire under.
func TestEnsureBucket_OneShapeIsTheCatalogRow(t *testing.T) {
	spec, ok := kvcatalog.SpecFor(BucketGraphStatus)
	if !ok {
		t.Fatalf("catalog must declare %s", BucketGraphStatus)
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

func TestEnsureBucket_NilClientErrors(t *testing.T) {
	if _, err := EnsureBucket(t.Context(), nil); err == nil {
		t.Fatal("want error for nil client, got nil")
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
			pub := NewPublisher(writer, tt.key)

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

			// The production consumer decode path, not a local json.Unmarshal.
			h := newHarness(t)
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
	}
}

func TestPublisher_PutFailureIsReturnedNotSwallowed(t *testing.T) {
	// The tick loop must be able to count and log the failure, so it has to see it.
	boom := errors.New("no responders")
	pub := NewPublisher(&fakeWriter{err: boom}, KeyGraphIndex)

	err := pub.Publish(t.Context(), graph.IndexStatusResponse{State: graph.IndexStateBuilding})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error does not wrap the Put cause: %v", err)
	}
}

func TestNewPublisher_RejectsIncompleteWiring(t *testing.T) {
	tests := []struct {
		name   string
		writer StatusWriter
		key    string
	}{
		{name: "nil writer", writer: nil, key: KeyGraphIndex},
		{name: "empty key", writer: &fakeWriter{}, key: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := NewPublisher(tt.writer, tt.key)
			if pub != nil {
				t.Fatalf("want nil publisher for incomplete wiring, got %+v", pub)
			}
			// A nil publisher must stay a safe no-op: the tick loop guards on it, and
			// a panic there would take down the component the status describes.
			if err := pub.Publish(t.Context(), graph.IndexStatusResponse{}); err != nil {
				t.Errorf("nil publisher Publish = %v, want nil", err)
			}
		})
	}
}

// TestPublisherKey: Key reports the key the publisher was built for, and "" for the nil
// publisher NewPublisher returns on incomplete wiring. graph-ingest's publish-failure log
// reads it (processor/graph-ingest/readiness.go:341).
func TestPublisherKey(t *testing.T) {
	tests := []struct {
		name string
		pub  *Publisher
		want string
	}{
		{name: "nil publisher", pub: NewPublisher(nil, KeyGraphIngest), want: ""},
		{name: "set", pub: NewPublisher(&fakeWriter{}, KeyGraphIngest), want: KeyGraphIngest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pub.Key(); got != tt.want {
				t.Errorf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
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
			pub := NewPublisher(writer, KeyGraphIndex)

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
