//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/types"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// verbsClientTimeout is the fixture client's request timeout, the value the pin's
// NewTestClient family used.
const verbsClientTimeout = 15 * time.Second

// openVerbsClient opens a natsclient.Client the way the pin's NewTestClient built it: no
// reconnects, no health monitor (natsfixture.Open bounds it and closes it on cleanup).
func openVerbsClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(verbsClientTimeout),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}

// recordingHandler keeps every log record the component writes, so a test can read what
// the component reports it did.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *recordingHandler) WithGroup(string) slog.Handler { return h }

// listedQuerySubjects is the test adapter's list of graph-ingest's query request
// subscriptions: the subjects attribute of the "query handlers registered" record the
// component writes once it has subscribed to them.
func (h *recordingHandler) listedQuerySubjects(t *testing.T) []string {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Message != "query handlers registered" {
			continue
		}
		var subjects []string
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "subjects" {
				subjects, _ = a.Value.Any().([]string)
				return false
			}
			return true
		})
		return subjects
	}
	t.Fatal(`graph-ingest wrote no "query handlers registered" record`)
	return nil
}

// startGraphIngestOnFixture starts graph-ingest with its default configuration on a
// fresh broker, its entity stream created first, and stops it on cleanup under a bounded
// context. It returns the fixture and the log records the component wrote.
func startGraphIngestOnFixture(t *testing.T) (*natsfixture.Fixture, *recordingHandler) {
	t.Helper()
	f := natsfixture.New(t)
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("natsfixture Start: %v", err)
	}
	client := natsfixture.Open(t, f, openVerbsClient)

	config := DefaultConfig()
	stream := f.Name("entity")
	config.Ports.Inputs[0].Config = component.JetStreamPort{StreamName: stream, Subjects: []string{"entity.>"}, DeliverPolicy: "all"}
	if _, err := f.CreateStream(t.Context(), stream, "entity.>"); err != nil {
		t.Fatalf("create entity stream: %v", err)
	}
	rawConfig, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	logs := &recordingHandler{}
	built, err := CreateGraphIngest(rawConfig, component.Dependencies{
		NATSClient:      client,
		PayloadRegistry: payloadregistry.New(),
		MetricsRegistry: metric.NewMetricsRegistry(),
		Platform:        types.PlatformMeta{Org: "acme", Platform: "ops"},
		Logger:          slog.New(logs),
	})
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	return f, logs
}

// hasResponder reports whether a request on subject reaches a subscriber: any reply,
// error replies included, means one answered; nats.ErrNoResponders means none is there.
func hasResponder(t *testing.T, nc *nats.Conn, subject string) bool {
	t.Helper()
	_, err := nc.Request(subject, []byte(`{}`), 10*time.Second)
	if errors.Is(err, nats.ErrNoResponders) {
		return false
	}
	if err != nil {
		t.Fatalf("request on %s: %v", subject, err)
	}
	return true
}

// TestGraphIngestProvisionsNoSuffixIndex: after Start, the ENTITY_SUFFIX_INDEX bucket the
// pin created does not exist, and a request on graph.ingest.query.suffix gets no
// responder (design D19). ENTITY_STATES exists and the entity verb answers, so Start did
// provision and subscribe.
func TestGraphIngestProvisionsNoSuffixIndex(t *testing.T) {
	f, _ := startGraphIngestOnFixture(t)
	js := f.JetStream()

	if _, err := js.KeyValue(t.Context(), graph.BucketEntityStates); err != nil {
		t.Fatalf("ENTITY_STATES after Start: %v", err)
	}
	_, err := js.KeyValue(t.Context(), "ENTITY_SUFFIX_INDEX")
	if !errors.Is(err, jetstream.ErrBucketNotFound) {
		t.Errorf("ENTITY_SUFFIX_INDEX after Start: err = %v, want jetstream.ErrBucketNotFound", err)
	}

	nc := js.Conn()
	if !hasResponder(t, nc, "graph.ingest.query.entity") {
		t.Fatal("graph.ingest.query.entity has no responder after Start")
	}
	if hasResponder(t, nc, "graph.ingest.query.suffix") {
		t.Error("graph.ingest.query.suffix has a responder after Start")
	}
}

// TestGraphIngestServesExactlyTheDeclaredVerbs: the subjects graph-ingest lists as its
// query request subscriptions equal the subjects of the verb table's entries for
// responder graph-ingest, and each listed subject has a responder (design D20;
// graph-transport-boundary, "Subscriptions match the table").
func TestGraphIngestServesExactlyTheDeclaredVerbs(t *testing.T) {
	f, logs := startGraphIngestOnFixture(t)

	var declared []string
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == "graph-ingest" {
			declared = append(declared, verb.Subject)
		}
	}
	if len(declared) == 0 {
		t.Fatal("the verb table declares no verb for graph-ingest")
	}
	listed := logs.listedQuerySubjects(t)
	slices.Sort(declared)
	slices.Sort(listed)
	if !slices.Equal(listed, declared) {
		t.Fatalf("graph-ingest lists query subscriptions %v, the table declares %v", listed, declared)
	}
	nc := f.JetStream().Conn()
	for _, subject := range listed {
		if !hasResponder(t, nc, subject) {
			t.Errorf("listed subject %s has no responder", subject)
		}
	}
}
