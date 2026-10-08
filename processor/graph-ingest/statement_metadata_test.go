package graphingest

// Statement metadata on the stream lane (design D15, #98; task 4.8): a Graphable payload's
// statement without a Source or Timestamp takes the message envelope's, a message whose
// envelope cannot supply a needed value is refused as poison, and a message carrying a
// reserved source is refused as poison. Each test drives the consume closure Start wires
// for the entity_stream port, with a real keyed ingest pool behind it.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// streamLaneMsg is a jetstream.Msg carrying one published message. Its disposition (ack,
// nak or term) is sent on settled, which is how a test waits for the pool to finish it.
type streamLaneMsg struct {
	data    []byte
	seq     uint64
	settled chan string
}

func newStreamLaneMsg(data []byte, seq uint64) *streamLaneMsg {
	return &streamLaneMsg{data: data, seq: seq, settled: make(chan string, 4)}
}

func (m *streamLaneMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{
		Stream:    "ENTITY",
		Sequence:  jetstream.SequencePair{Stream: m.seq, Consumer: m.seq},
		Timestamp: time.Now(),
	}, nil
}
func (m *streamLaneMsg) Data() []byte                     { return m.data }
func (*streamLaneMsg) Headers() nats.Header               { return nil }
func (*streamLaneMsg) Subject() string                    { return "entity.statement-metadata" }
func (*streamLaneMsg) Reply() string                      { return "" }
func (m *streamLaneMsg) Ack() error                       { m.settled <- "ack"; return nil }
func (m *streamLaneMsg) DoubleAck(context.Context) error  { m.settled <- "ack"; return nil }
func (m *streamLaneMsg) Nak() error                       { m.settled <- "nak"; return nil }
func (m *streamLaneMsg) NakWithDelay(time.Duration) error { m.settled <- "nak"; return nil }
func (*streamLaneMsg) InProgress() error                  { return nil }
func (m *streamLaneMsg) Term() error                      { m.settled <- "term"; return nil }
func (m *streamLaneMsg) TermWithReason(string) error      { m.settled <- "term"; return nil }

// startStreamLane wires the component's JetStream inputs as Start does (setupSubscriptions,
// over a real keyed ingest pool and the mock entity bucket) and returns the consume closure
// the entity_stream consumer would call for each message.
func startStreamLane(t *testing.T) (*Component, func(context.Context, jetstream.Msg)) {
	t.Helper()
	c, _ := createTestComponentWithMockKVBucket(t, withAuthority("c360", "test"))
	registerMergeTestPayload(t, c)
	if err := c.buildIngestPool(t.Context()); err != nil {
		t.Fatalf("buildIngestPool: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.ingestPool.Shutdown(ctx); err != nil {
			t.Errorf("ingest pool Shutdown: %v", err)
		}
	})

	var handler func(context.Context, jetstream.Msg)
	c.waitForStreamInput = func(context.Context, string) error { return nil }
	c.consumeStream = func(
		_ context.Context,
		_ natsclient.PortConsumerContext,
		cfg natsclient.StreamConsumerConfig,
		h func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		if cfg.StreamName == "ENTITY" {
			handler = h
		}
		return nil, nil
	}
	if err := c.setupSubscriptions(t.Context(), t.Context()); err != nil {
		t.Fatalf("setupSubscriptions: %v", err)
	}
	if handler == nil {
		t.Fatal("setupSubscriptions bound no consumer on the ENTITY stream")
	}
	return c, handler
}

// streamLaneData encodes a Graphable payload in an envelope from source, as a producer
// publishes it.
func streamLaneData(t *testing.T, payload *mergeTestGraphable, source string, opts ...message.Option) []byte {
	t.Helper()
	data, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, source, opts...))
	if err != nil {
		t.Fatalf("encode stream message: %v", err)
	}
	return data
}

// deliverStreamLane hands one message to the consume closure and returns its disposition.
func deliverStreamLane(t *testing.T, handler func(context.Context, jetstream.Msg), data []byte, seq uint64) string {
	t.Helper()
	msg := newStreamLaneMsg(data, seq)
	handler(t.Context(), msg)
	select {
	case disposition := <-msg.settled:
		return disposition
	case <-time.After(10 * time.Second):
		t.Fatalf("message %d was neither acked, naked nor terminated within 10s", seq)
		return ""
	}
}

// storedRevision returns the entity's stored bytes and revision, or ok false when the
// bucket holds no entry for it.
func storedRevision(t *testing.T, c *Component, entityID string) (value []byte, revision uint64, ok bool) {
	t.Helper()
	entry, err := c.entityBucket.Get(context.Background(), entityID)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return nil, 0, false
		}
		t.Fatalf("read entity %s: %v", entityID, err)
	}
	return entry.Value, entry.Revision, true
}

// storedStatement returns the stored statement of entity with predicate, failing the test
// when there is not exactly one.
func storedStatement(t *testing.T, entity *graph.EntityState, predicate string) message.Triple {
	t.Helper()
	var found []message.Triple
	for _, triple := range entity.Triples {
		if triple.Predicate == predicate {
			found = append(found, triple)
		}
	}
	if len(found) != 1 {
		t.Fatalf("stored statements with predicate %s: %d, want 1: %v", predicate, len(found), entity.Triples)
	}
	return found[0]
}

// assertInvalidRequest checks that err is an invalid_request-class error whose text names
// every one of names.
func assertInvalidRequest(t *testing.T, err error, names ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("decodeEntity accepted the message; want an invalid_request refusal")
	}
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorInvalid || classified.Code != graph.ErrorCodeInvalidRequest {
		t.Fatalf("refusal is not invalid_request-class: %v", err)
	}
	for _, name := range names {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("refusal %q does not name %q", err.Error(), name)
		}
	}
}

func TestGraphableLaneStampsFromEnvelope(t *testing.T) {
	c, handler := startStreamLane(t)
	const entityID = "c360.test.stamp.lane.entity.001"
	// The wire carries the envelope's creation time in milliseconds.
	created := time.UnixMilli(1_790_000_000_000)
	own := time.UnixMilli(1_780_000_000_000)
	payload := &mergeTestGraphable{entityID: entityID, triples: []message.Triple{
		{Subject: entityID, Predicate: "stamp.state.bare", Object: "a", Confidence: 1},
		{Subject: entityID, Predicate: "stamp.state.sourced", Object: "b", Source: "relay-origin", Confidence: 1},
		{Subject: entityID, Predicate: "stamp.state.timed", Object: "c", Timestamp: own, Confidence: 1},
		{Subject: entityID, Predicate: "stamp.state.own", Object: "d", Source: "relay-origin", Timestamp: own, Confidence: 1},
	}}

	if got := deliverStreamLane(t, handler, streamLaneData(t, payload, "stamp-producer", message.WithTime(created)), 1); got != "ack" {
		t.Fatalf("disposition = %s, want ack", got)
	}

	stored := storedEntity(t, c, entityID)
	for _, want := range []struct {
		predicate string
		source    string
		at        time.Time
	}{
		{"stamp.state.bare", "stamp-producer", created},
		{"stamp.state.sourced", "relay-origin", created},
		{"stamp.state.timed", "stamp-producer", own},
		{"stamp.state.own", "relay-origin", own},
	} {
		got := storedStatement(t, stored, want.predicate)
		if got.Source != want.source || !got.Timestamp.Equal(want.at) {
			t.Errorf("%s stored with source %q at %s, want %q at %s",
				want.predicate, got.Source, got.Timestamp, want.source, want.at)
		}
	}
}

func TestGraphableLaneRefusesWithoutEnvelopeMetadata(t *testing.T) {
	at := time.UnixMilli(1_790_000_000_000)
	tests := []struct {
		name    string
		source  string
		opts    []message.Option
		triples func(entityID string) []message.Triple
		field   string
		reason  string
	}{
		{
			name:   "no envelope source",
			source: "",
			triples: func(entityID string) []message.Triple {
				return []message.Triple{
					{Subject: entityID, Predicate: "stamp.state.first", Object: "a", Source: "own-source", Confidence: 1},
					{Subject: entityID, Predicate: "stamp.state.second", Object: "b", Confidence: 1},
				}
			},
			field:  "source",
			reason: "missing_source",
		},
		{
			name:   "no envelope creation time",
			source: "stamp-producer",
			opts:   []message.Option{message.WithTime(time.Time{})},
			triples: func(entityID string) []message.Triple {
				return []message.Triple{
					{Subject: entityID, Predicate: "stamp.state.first", Object: "a", Timestamp: at, Confidence: 1},
					{Subject: entityID, Predicate: "stamp.state.second", Object: "b", Confidence: 1},
				}
			},
			field:  "timestamp",
			reason: "missing_timestamp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, handler := startStreamLane(t)
			const entityID = "c360.test.stamp.refuse.entity.001"
			data := streamLaneData(t, &mergeTestGraphable{entityID: entityID, triples: tt.triples(entityID)}, tt.source, tt.opts...)

			var logs bytes.Buffer
			c.logger = slog.New(slog.NewTextHandler(&logs, nil))
			counter := c.predicateContractRejections.WithLabelValues("graphable", tt.reason)
			before := testutil.ToFloat64(counter)

			if got := deliverStreamLane(t, handler, data, 1); got != "term" {
				t.Errorf("disposition = %s, want term", got)
			}
			if got := testutil.ToFloat64(counter); got != before+1 {
				t.Errorf("predicate_contract_rejections_total{lane=graphable,reason=%s} = %v, want %v", tt.reason, got, before+1)
			}
			logLine := logs.String()
			for _, want := range []string{"level=WARN", "lane=graphable", "field=" + tt.field, "reason=" + tt.reason, "triple_index=1"} {
				if !strings.Contains(logLine, want) {
					t.Errorf("rejection log %q lacks %q", logLine, want)
				}
			}
			if _, _, ok := storedRevision(t, c, entityID); ok {
				t.Errorf("entity %s stored from a refused message", entityID)
			}

			_, err := c.decodeEntity("entity.statement-metadata", data)
			assertInvalidRequest(t, err, "triple[1]", tt.field)
		})
	}

	// A statement that carries its own value needs nothing from the envelope: with every
	// statement sourced, an envelope without a source is admitted.
	t.Run("statements carry their own", func(t *testing.T) {
		c, handler := startStreamLane(t)
		const entityID = "c360.test.stamp.admit.entity.001"
		payload := &mergeTestGraphable{entityID: entityID, triples: []message.Triple{
			{Subject: entityID, Predicate: "stamp.state.first", Object: "a", Source: "own-source", Timestamp: at, Confidence: 1},
		}}
		if got := deliverStreamLane(t, handler, streamLaneData(t, payload, ""), 1); got != "ack" {
			t.Fatalf("disposition = %s, want ack", got)
		}
		got := storedStatement(t, storedEntity(t, c, entityID), "stamp.state.first")
		if got.Source != "own-source" || !got.Timestamp.Equal(at) {
			t.Errorf("stored with source %q at %s, want own-source at %s", got.Source, got.Timestamp, at)
		}
	})
}

func TestStreamLaneRefusesReservedSource(t *testing.T) {
	tests := []struct {
		name     string
		envelope string
		reserved message.Triple
		source   string
	}{
		{
			name:     "statement names graph-ingest-hierarchy",
			envelope: "stamp-producer",
			reserved: message.Triple{Predicate: "stamp.state.reserved", Object: "x", Source: graph.SourceHierarchy, Confidence: 1},
			source:   graph.SourceHierarchy,
		},
		{
			name:     "envelope source is semengine-lifecycle",
			envelope: graph.SourceLifecycle,
			reserved: message.Triple{Predicate: "stamp.state.reserved", Object: "x", Confidence: 1},
			source:   graph.SourceLifecycle,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, handler := startStreamLane(t)
			const entityID = "c360.test.stamp.reserved.entity.001"
			created := time.UnixMilli(1_790_000_000_000)

			// The entity exists before the refused message arrives.
			seed := &mergeTestGraphable{entityID: entityID, triples: []message.Triple{
				{Subject: entityID, Predicate: "stamp.state.kept", Object: "before", Confidence: 1},
			}}
			if got := deliverStreamLane(t, handler, streamLaneData(t, seed, "stamp-producer", message.WithTime(created)), 1); got != "ack" {
				t.Fatalf("seed disposition = %s, want ack", got)
			}
			beforeValue, beforeRevision, ok := storedRevision(t, c, entityID)
			if !ok {
				t.Fatalf("seed message stored no entity %s", entityID)
			}

			reserved := tt.reserved
			reserved.Subject = entityID
			// Statement 0 names its own source, so only statement 1 can carry a reserved one.
			payload := &mergeTestGraphable{entityID: entityID, triples: []message.Triple{
				{Subject: entityID, Predicate: "stamp.state.kept", Object: "after", Source: "stamp-producer", Confidence: 1},
				reserved,
			}}
			data := streamLaneData(t, payload, tt.envelope, message.WithTime(created.Add(time.Second)))

			counter := c.predicateContractRejections.WithLabelValues("graphable", "reserved_source")
			before := testutil.ToFloat64(counter)
			if got := deliverStreamLane(t, handler, data, 2); got != "term" {
				t.Errorf("disposition = %s, want term", got)
			}
			if got := testutil.ToFloat64(counter); got != before+1 {
				t.Errorf("predicate_contract_rejections_total{lane=graphable,reason=reserved_source} = %v, want %v", got, before+1)
			}
			afterValue, afterRevision, _ := storedRevision(t, c, entityID)
			if afterRevision != beforeRevision || !bytes.Equal(afterValue, beforeValue) {
				t.Errorf("entity changed by a refused message: revision %d -> %d", beforeRevision, afterRevision)
			}

			_, err := c.decodeEntity("entity.statement-metadata", data)
			assertInvalidRequest(t, err, "triple[1]", tt.source)
		})
	}
}
