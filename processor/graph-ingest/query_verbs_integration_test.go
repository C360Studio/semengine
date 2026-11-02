//go:build integration

package graphingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/types"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

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

// startGraphIngestOnFixture starts graph-ingest with its default configuration on the
// package's shared broker reset for t, its entity stream created first, and stops it on
// cleanup under a bounded context. It returns the connected client and the log records
// the component wrote.
func startGraphIngestOnFixture(t *testing.T) (*natsclient.Client, *recordingHandler) {
	t.Helper()
	client := newFixtureClient(t, entityStream)

	config := DefaultConfig()
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
	return client, logs
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
	client, _ := startGraphIngestOnFixture(t)
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}

	if _, err := js.KeyValue(t.Context(), graph.BucketEntityStates); err != nil {
		t.Fatalf("ENTITY_STATES after Start: %v", err)
	}
	_, err = js.KeyValue(t.Context(), "ENTITY_SUFFIX_INDEX")
	if !errors.Is(err, jetstream.ErrBucketNotFound) {
		t.Errorf("ENTITY_SUFFIX_INDEX after Start: err = %v, want jetstream.ErrBucketNotFound", err)
	}

	nc := client.GetConnection()
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
	client, logs := startGraphIngestOnFixture(t)

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
	nc := client.GetConnection()
	for _, subject := range listed {
		if !hasResponder(t, nc, subject) {
			t.Errorf("listed subject %s has no responder", subject)
		}
	}
}

// replyMatchesEntry decodes raw as a caller holding verb's table entry does: the envelope when the
// entry is Enveloped, then the entry's Reply, each refusing an unknown field. Re-encoding the decoded
// body must give the same JSON, so a Reply with fields the wire does not carry fails too.
func replyMatchesEntry(verb graph.QueryVerb, raw []byte) error {
	decodeStrict := func(data []byte, into any) error {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		return decoder.Decode(into)
	}
	body := raw
	if verb.Enveloped {
		var reply graph.QueryResponse[json.RawMessage]
		if err := decodeStrict(raw, &reply); err != nil {
			return fmt.Errorf("as an envelope: %w", err)
		}
		body = reply.Data
	}
	decoded := reflect.New(verb.Reply)
	if err := decodeStrict(body, decoded.Interface()); err != nil {
		return fmt.Errorf("as %s: %w", verb.Reply, err)
	}
	again, err := json.Marshal(decoded.Interface())
	if err != nil {
		return err
	}
	var sent, resent any
	if err := errors.Join(json.Unmarshal(body, &sent), json.Unmarshal(again, &resent)); err != nil {
		return err
	}
	if !reflect.DeepEqual(sent, resent) {
		return fmt.Errorf("as %s re-encodes to %s, the reply body is %s", verb.Reply, again, body)
	}
	return nil
}

// TestRepliesMatchTheirTableEntries: every row the verb table declares for graph-ingest, not a list
// kept here, gets a real reply with its optional keys present (batch's missing, prefix's next_cursor),
// and that reply has the row's shape and type.
func TestRepliesMatchTheirTableEntries(t *testing.T) {
	ctx := t.Context()
	c, nc, owner := startPrefixTestComponent(ctx, t, withAuthority("acme", "ops"))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		owner.finish(stopCtx, t)
	})
	const one, two, absent = "acme.ops.dom.sys.type.one", "acme.ops.dom.sys.type.two", "acme.ops.dom.sys.type.absent"
	seedPrefixEntity(t, ctx, c, one)
	seedPrefixEntity(t, ctx, c, two)

	requests := map[string]any{
		"entity": map[string]string{"id": one},
		"batch":  map[string][]string{"ids": {one, absent}},
		"prefix": map[string]any{"prefix": "acme", "limit": 1},
	}
	walked := 0
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphIngest {
			continue
		}
		walked++
		request, ok := requests[verb.Name]
		if !ok {
			t.Errorf("the table declares %s and this test has no request for it", verb.Name)
			continue
		}
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := nc.RequestClassified(ctx, verb.Subject, data, wireRequestTimeout)
		if err != nil {
			t.Errorf("%s: %v", verb.Name, err)
			continue
		}
		if err := replyMatchesEntry(verb, raw); err != nil {
			t.Errorf("%s reply does not match its table entry: %v; reply %s", verb.Name, err, raw)
		}
	}
	if walked == 0 {
		t.Fatal("the table declares no graph-ingest verb")
	}
}
