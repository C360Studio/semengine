//go:build integration

package objectstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
)

// document is a registered ContentStorable test payload: its body is the "body" role and its
// title the "title" role, so a document with a title carries content other than the body. Its
// Validate accepts any entity ID, so a document whose ID is invalid still decodes and reaches
// StoreContent.
type document struct {
	ID    string `json:"id"`
	Body  string `json:"body"`
	Title string `json:"title,omitempty"`
}

func (d *document) Schema() message.Type {
	return message.Type{Domain: "objectstoretest", Category: "document", Version: "v1"}
}
func (d *document) Validate() error  { return nil }
func (d *document) EntityID() string { return d.ID }
func (d *document) Triples() []message.Triple {
	return []message.Triple{{Subject: d.ID, Predicate: "objectstoretest.document.kind", Object: "note"}}
}
func (d *document) StorageRef() *message.StorageReference { return nil }
func (d *document) RawContent() map[string]string {
	return map[string]string{"body": d.Body, "title": d.Title}
}
func (d *document) ContentFields() map[string]string {
	return map[string]string{message.ContentRoleBody: "body", "title": "title"}
}
func (d *document) MarshalJSON() ([]byte, error) {
	type alias document
	return json.Marshal((*alias)(d))
}
func (d *document) UnmarshalJSON(data []byte) error {
	type alias document
	return json.Unmarshal(data, (*alias)(d))
}

// note is a registered payload that is not ContentStorable.
type note struct {
	Text string `json:"text"`
}

func (n *note) Schema() message.Type {
	return message.Type{Domain: "objectstoretest", Category: "note", Version: "v1"}
}
func (n *note) Validate() error { return nil }
func (n *note) MarshalJSON() ([]byte, error) {
	type alias note
	return json.Marshal((*alias)(n))
}
func (n *note) UnmarshalJSON(data []byte) error {
	type alias note
	return json.Unmarshal(data, (*alias)(n))
}

var (
	_ message.ContentStorable = (*document)(nil)
	_ message.Payload         = (*note)(nil)
)

// testRegistry is the payload registry the Component's tests give it: StoredMessage and the two
// test payloads.
func testRegistry() (*payloadregistry.Registry, error) {
	reg := payloadregistry.New()
	return reg, errors.Join(
		RegisterPayloads(reg),
		reg.Register(&payloadregistry.Registration{
			Factory: func() any { return &document{} }, Domain: "objectstoretest", Category: "document", Version: "v1",
			Description: "ContentStorable test document",
		}),
		reg.Register(&payloadregistry.Registration{
			Factory: func() any { return &note{} }, Domain: "objectstoretest", Category: "note", Version: "v1",
			Description: "test payload that is not ContentStorable",
		}),
	)
}

// wire is payload wrapped in a BaseMessage, as a producer publishes it.
func wire(t *testing.T, payload message.Payload) []byte {
	t.Helper()
	data, err := message.NewBaseMessage(payload.Schema(), payload, "objectstore-test").MarshalJSON()
	if err != nil {
		t.Fatalf("marshal %T: %v", payload, err)
	}
	return data
}

// logBuffer collects the Component's log lines; the handlers write it from their own goroutines.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startLogged builds and starts a Component from raw whose log goes to the returned buffer, and
// returns the Stop the caller defers.
func startLogged(ctx context.Context, t *testing.T, client *natsclient.Client, raw string) (*Component, *logBuffer, func()) {
	t.Helper()
	reg, err := testRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	logs := &logBuffer{}
	built, err := newComponent(json.RawMessage(raw), component.Dependencies{
		NATSClient: client, PayloadRegistry: reg, Logger: slog.New(slog.NewTextHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return c, logs, func() {
		stopCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		if err := c.Stop(stopCtx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}
}

// listBucket lists the objects in bucket through JetStream itself, not through the Store.
func listBucket(ctx context.Context, js jetstream.JetStream, bucket string) ([]string, error) {
	objects, err := js.ObjectStore(ctx, bucket)
	if err != nil {
		return nil, err
	}
	infos, err := objects.List(ctx)
	if err != nil && !errors.Is(err, jetstream.ErrNoObjectsFound) {
		return nil, err
	}
	keys := []string{}
	for _, info := range infos {
		keys = append(keys, info.Name)
	}
	slices.Sort(keys)
	return keys, nil
}

// bucketKeys is listBucket, failing the test if the list fails.
func bucketKeys(ctx context.Context, t *testing.T, js jetstream.JetStream, bucket string) []string {
	t.Helper()
	keys, err := listBucket(ctx, js, bucket)
	if err != nil {
		t.Fatalf("list %s: %v", bucket, err)
	}
	return keys
}

// awaitKeys waits, up to bound, for bucket to hold exactly want objects, and returns their keys.
func awaitKeys(ctx context.Context, t *testing.T, js jetstream.JetStream, bucket string, want int, bound time.Duration) []string {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	keys, err := probe.Await(waitCtx, func(ctx context.Context) ([]string, error) {
		return listBucket(ctx, js, bucket)
	}, func(keys []string) bool { return len(keys) == want })
	if err != nil {
		t.Fatalf("bucket %s must hold %d objects; holds %v: %v", bucket, want, keys, err)
	}
	return keys
}

// awaitErrorCount waits, up to 30 seconds, for the Component's ErrorCount to reach want. A
// handler counts an error after it settled the delivery and logged it, so the wait orders reads of
// the log and of the bucket after both.
func awaitErrorCount(ctx context.Context, t *testing.T, c *Component, want int) {
	t.Helper()
	if _, err := awaitHealth(ctx, t, c, func(h component.HealthStatus) bool { return h.ErrorCount >= want }); err != nil {
		t.Fatalf("ErrorCount must reach %d: %v (Health %+v)", want, err, c.Health())
	}
	if got := c.Health().ErrorCount; got != want {
		t.Fatalf("ErrorCount = %d, want %d", got, want)
	}
}

// decodeStored decodes data as a StoredMessage through the production decoder.
func decodeStored(t *testing.T, data []byte) *StoredMessage {
	t.Helper()
	reg, err := testRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	msg, err := message.NewDecoder(reg).Decode(data)
	if err != nil {
		t.Fatalf("decode the stored output: %v", err)
	}
	stored, ok := msg.Payload().(*StoredMessage)
	if !ok {
		t.Fatalf("stored output payload = %T, want *StoredMessage", msg.Payload())
	}
	return stored
}

// TestWriteLaneStoresBodyAndEmitsReference: a document on the core-NATS write input is stored
// through StoreContent, and the StoredMessage on the stored output carries its entity, its
// triples and a reference, stamped objectstore, that opens to the body through the Store the
// Component provides (object-store spec, "A document on the write input"; design D3).
func TestWriteLaneStoresBodyAndEmitsReference(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
	stored, err := client.GetConnection().SubscribeSync("storage.objectstore.stored")
	if err != nil {
		t.Fatalf("subscribe to the stored output: %v", err)
	}
	if err := client.GetConnection().Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	c, _, stop := startLogged(ctx, t, client, coreConfig)
	defer stop()

	doc := &document{ID: "acme.test.objectstore.content.document.doc1", Body: "the body text"}
	if err := client.Publish(ctx, "objectstore.test.core", wire(t, doc)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	reply, err := stored.NextMsg(15 * time.Second)
	if err != nil {
		t.Fatalf("no StoredMessage on the stored output: %v", err)
	}
	got := decodeStored(t, reply.Data)
	if got.EntityID() != doc.ID {
		t.Errorf("EntityID = %q, want %q", got.EntityID(), doc.ID)
	}
	if !reflect.DeepEqual(got.Triples(), doc.Triples()) {
		t.Errorf("Triples = %+v, want %+v", got.Triples(), doc.Triples())
	}
	if got.MessageType() != "objectstoretest.document.v1" {
		t.Errorf("MessageType = %q, want objectstoretest.document.v1", got.MessageType())
	}
	ref := got.StorageRef()
	if ref == nil || ref.StorageInstance != "objectstore" || ref.ContentType != "text/plain; charset=utf-8" ||
		ref.Size != int64(len(doc.Body)) {
		t.Fatalf("StorageRef = %+v, want instance objectstore, text/plain; charset=utf-8, size %d", ref, len(doc.Body))
	}
	body, err := c.ProvidedStores()[ref.StorageInstance].Get(ctx, ref.Key)
	if err != nil || string(body) != doc.Body {
		t.Fatalf("the reference opens to %q, %v; want the body %q", body, err, doc.Body)
	}
	if health := c.Health(); health.ErrorCount != 0 {
		t.Fatalf("ErrorCount = %d after a stored document, want 0", health.ErrorCount)
	}
}

// TestStartContextEndKeepsBothLanesWriting: after Start returned, its context is cancelled, and a
// document is then published on a core-NATS input and on a JetStream input; both bodies are
// stored (object-store spec, "Both lanes after the Start context ended"; design D5). At the pin
// both lanes wrote under a child of the Start context and stopped storing here (inventory probe 2).
func TestStartContextEndKeepsBothLanesWriting(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
	js := clientJetStream(t, client)
	createInputStream(ctx, t, js, jetstream.FileStorage)
	const bucket = "START_CONTEXT_END"
	startCtx, endStart := context.WithCancel(ctx)
	c, _, stop := startLogged(startCtx, t, client, `{"bucket_name": "`+bucket+`", "ports": {"inputs": [`+
		`{"name": "write", "config": {"kind": "nats", "subject": "objectstore.test.core"}}, `+
		`{"name": "`+inputPort+`", "config": {"kind": "jetstream", "stream_name": "`+inputStream+
		`", "subjects": ["`+inputSubject+`"]}}], "outputs": [`+storedOutput+`]}}`)
	defer stop()
	endStart()

	core := &document{ID: "acme.test.objectstore.content.document.core", Body: "stored from core NATS"}
	stream := &document{ID: "acme.test.objectstore.content.document.stream", Body: "stored from JetStream"}
	if err := client.Publish(ctx, "objectstore.test.core", wire(t, core)); err != nil {
		t.Fatalf("publish core: %v", err)
	}
	if err := client.PublishToStream(ctx, inputSubject, wire(t, stream)); err != nil {
		t.Fatalf("publish JetStream: %v", err)
	}
	keys := awaitKeys(ctx, t, js, bucket, 2, 20*time.Second)
	var bodies []string
	for _, key := range keys {
		body, err := c.ProvidedStores()[instanceName].Get(ctx, key)
		if err != nil {
			t.Fatalf("Get %s: %v", key, err)
		}
		bodies = append(bodies, string(body))
	}
	slices.Sort(bodies)
	if want := []string{stream.Body, core.Body}; !slices.Equal(bodies, want) { // sorted
		t.Fatalf("stored bodies = %q, want %q", bodies, want)
	}
}

// TestWriteLaneRefusals: each message the lane refuses (bytes that do not decode, a payload that
// is not ContentStorable, a document with content other than its body, a document with an
// invalid entity ID), on a core-NATS input and on a JetStream input, stores nothing, is logged at
// error level naming why, and raises ErrorCount by one; on JetStream the delivery is terminated,
// which the server announces with its MSG_TERMINATED advisory, and never redelivered (object-store
// spec, "Each refusal on each lane"; design D4, owner ruling 2).
func TestWriteLaneRefusals(t *testing.T) {
	cases := []struct {
		name, logs string
		data       func(*testing.T) []byte
	}{
		{"not decodable", "decode message", func(*testing.T) []byte { return []byte("not a message") }},
		{"not ContentStorable", "objectstoretest.note.v1 is not message.ContentStorable", func(t *testing.T) []byte {
			return wire(t, &note{Text: "no body here"})
		}},
		{"non-body content", `content other than the body is not stored`, func(t *testing.T) []byte {
			return wire(t, &document{ID: "acme.test.objectstore.content.document.titled", Body: "body", Title: "a title"})
		}},
		{"invalid entity ID", "validate entity ID", func(t *testing.T) []byte {
			return wire(t, &document{ID: "not-an-entity-id", Body: "body"})
		}},
	}
	for _, lane := range []struct {
		name, raw, bucket string
		jetStream         bool
	}{
		{"core", coreConfig, "COMPONENT_CORE", false},
		{"jetstream", jetStreamConfig, "COMPONENT_JS", true},
	} {
		t.Run(lane.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
			js := clientJetStream(t, client)
			if lane.jetStream {
				createInputStream(ctx, t, js, jetstream.FileStorage)
			}
			terminated, err := client.GetConnection().SubscribeSync(
				"$JS.EVENT.ADVISORY.CONSUMER.MSG_TERMINATED." + inputStream + "." + inputConsumer)
			if err != nil {
				t.Fatalf("subscribe to the termination advisory: %v", err)
			}
			if err := client.GetConnection().Flush(); err != nil {
				t.Fatalf("flush: %v", err)
			}
			c, logs, stop := startLogged(ctx, t, client, lane.raw)
			defer stop()

			for index, tc := range cases {
				logged := len(logs.String())
				subject := "objectstore.test.core"
				publish := client.Publish
				if lane.jetStream {
					subject, publish = inputSubject, client.PublishToStream
				}
				if err := publish(ctx, subject, tc.data(t)); err != nil {
					t.Fatalf("%s: publish: %v", tc.name, err)
				}
				awaitErrorCount(ctx, t, c, index+1)
				if line := logs.String()[logged:]; !strings.Contains(line, "level=ERROR") || !strings.Contains(line, tc.logs) {
					t.Errorf("%s: log = %q, want an error line containing %q", tc.name, line, tc.logs)
				}
				if keys := bucketKeys(ctx, t, js, lane.bucket); len(keys) != 0 {
					t.Errorf("%s: bucket holds %v, want nothing stored", tc.name, keys)
				}
				if lane.jetStream {
					if _, err := terminated.NextMsg(10 * time.Second); err != nil {
						t.Errorf("%s: no MSG_TERMINATED advisory: %v", tc.name, err)
					}
				}
			}
			if !lane.jetStream {
				return
			}
			consumer, err := js.Consumer(ctx, inputStream, inputConsumer)
			if err != nil {
				t.Fatalf("Consumer: %v", err)
			}
			info, err := consumer.Info(ctx)
			if err != nil {
				t.Fatalf("consumer Info: %v", err)
			}
			if info.NumAckPending != 0 || info.NumRedelivered != 0 || info.Delivered.Consumer != uint64(len(cases)) {
				t.Fatalf("consumer: %d pending, %d redelivered, %d delivered; want 0, 0, %d (each delivery terminated once)",
					info.NumAckPending, info.NumRedelivered, info.Delivered.Consumer, len(cases))
			}
		})
	}
}

// TestStoredEmitFailureNaksThenRedeliveryStores is the one cross-delivery history (design D4,
// D9): the stored output is a JetStream stream the test deletes, so the first delivery's body is
// stored but its reference cannot be published, and the delivery is NAKed, counted and logged.
// With the stream back, the redelivery (after the 30-second transient delay) stores the body again
// under a new key and publishes its reference: one acknowledged delivery, two objects, exactly one
// referenced (object-store spec, "The reference cannot be published").
func TestStoredEmitFailureNaksThenRedeliveryStores(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
	js := clientJetStream(t, client)
	createInputStream(ctx, t, js, jetstream.FileStorage)
	const (
		bucket        = "EMIT_FAILURE"
		storedStream  = "OBJECTSTORE_STORED"
		storedSubject = "objectstore.test.stored"
	)
	if _, err := natsfixture.CreateStream(ctx, js, jetstream.FileStorage, storedStream, storedSubject); err != nil {
		t.Fatalf("CreateStream %s: %v", storedStream, err)
	}
	c, logs, stop := startLogged(ctx, t, client, `{"bucket_name": "`+bucket+`", "ports": {"inputs": [`+
		`{"name": "`+inputPort+`", "config": {"kind": "jetstream", "stream_name": "`+inputStream+
		`", "subjects": ["`+inputSubject+`"]}}], "outputs": [`+
		`{"name": "stored", "config": {"kind": "jetstream", "stream_name": "`+storedStream+
		`", "subjects": ["`+storedSubject+`"]}}]}}`)
	defer stop()
	if err := js.DeleteStream(ctx, storedStream); err != nil {
		t.Fatalf("DeleteStream %s: %v", storedStream, err)
	}

	doc := &document{ID: "acme.test.objectstore.content.document.retried", Body: "stored twice, referenced once"}
	if err := client.PublishToStream(ctx, inputSubject, wire(t, doc)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	awaitErrorCount(ctx, t, c, 1)
	if line := logs.String(); !strings.Contains(line, "level=ERROR") || !strings.Contains(line, "publish StoredMessage") {
		t.Fatalf("log = %q, want an error line naming the failed publish", line)
	}
	if keys := bucketKeys(ctx, t, js, bucket); len(keys) != 1 {
		t.Fatalf("after the failed publish the bucket holds %v, want the one stored body", keys)
	}

	output, err := natsfixture.CreateStream(ctx, js, jetstream.FileStorage, storedStream, storedSubject)
	if err != nil {
		t.Fatalf("CreateStream %s again: %v", storedStream, err)
	}
	// The redelivery waits out the 30-second transient NAK delay.
	waitCtx, cancelWait := context.WithTimeout(ctx, 60*time.Second)
	defer cancelWait()
	if _, err := probe.Await(waitCtx, func(ctx context.Context) (uint64, error) {
		info, err := output.Info(ctx)
		if err != nil {
			return 0, err
		}
		return info.State.Msgs, nil
	}, func(msgs uint64) bool { return msgs > 0 }); err != nil {
		t.Fatalf("no StoredMessage reached %s after the redelivery: %v", storedStream, err)
	}
	raw, err := output.GetMsg(ctx, 1)
	if err != nil {
		t.Fatalf("GetMsg: %v", err)
	}
	ref := decodeStored(t, raw.Data).StorageRef()

	consumer, err := js.Consumer(ctx, inputStream, inputConsumer)
	if err != nil {
		t.Fatalf("Consumer: %v", err)
	}
	info, err := probe.Await(waitCtx, func(ctx context.Context) (*jetstream.ConsumerInfo, error) {
		return consumer.Info(ctx)
	}, func(info *jetstream.ConsumerInfo) bool { return info.NumAckPending == 0 })
	if err != nil {
		t.Fatalf("the redelivery was never acknowledged: %v", err)
	}
	if info.Delivered.Consumer != 2 || info.AckFloor.Consumer != 2 || info.AckFloor.Stream != 1 {
		t.Fatalf("consumer delivered %d, ack floor %d (stream %d); want two deliveries of one message, the second acked",
			info.Delivered.Consumer, info.AckFloor.Consumer, info.AckFloor.Stream)
	}
	keys := bucketKeys(ctx, t, js, bucket)
	if len(keys) != 2 || !slices.Contains(keys, ref.Key) {
		t.Fatalf("bucket holds %v, want two objects, one of them the referenced %s", keys, ref.Key)
	}
	if final, err := output.Info(ctx); err != nil || final.State.Msgs != 1 {
		t.Fatalf("%s holds %v StoredMessages (%v), want exactly one", storedStream, final, err)
	}
	if got := c.Health().ErrorCount; got != 1 {
		t.Fatalf("ErrorCount = %d, want the one failed publish", got)
	}
}
