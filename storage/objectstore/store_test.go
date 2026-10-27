package objectstore

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	dto "github.com/prometheus/client_model/go"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// nilCtx is a nil context.Context, passed where a caller forgot one.
var nilCtx context.Context

// refusesNilContext reports whether err is the invalid-input refusal of a nil context, not some
// later failure.
func refusesNilContext(err error) bool {
	return errs.IsInvalid(err) && strings.Contains(err.Error(), "nil context")
}

func TestNewStoreRefusesNilContext(t *testing.T) {
	// A nil client too: the context is refused first, before anything reads the client.
	if _, err := NewStoreWithConfig(nilCtx, nil, Config{}); !refusesNilContext(err) {
		t.Errorf("NewStoreWithConfig(nil ctx) error = %v, want the invalid-input nil-context refusal", err)
	}
	if _, err := NewStoreWithConfigAndMetrics(nilCtx, nil, Config{}, metric.NewMetricsRegistry()); !refusesNilContext(err) {
		t.Errorf("NewStoreWithConfigAndMetrics(nil ctx) error = %v, want the invalid-input nil-context refusal", err)
	}
}

func TestNewStoreRefusesNilClientAndClientWithoutJetStream(t *testing.T) {
	if _, err := NewStoreWithConfig(context.Background(), nil, Config{}); !errs.IsInvalid(err) {
		t.Errorf("nil client: error = %v, want an invalid-input error", err)
	}
	// A client that never connected has no JetStream context.
	client, err := natsclient.NewClient("nats://unused")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Close(ctx)
	})
	if _, err := NewStoreWithConfig(context.Background(), client, Config{}); !errs.IsInvalid(err) {
		t.Errorf("client without JetStream: error = %v, want an invalid-input error", err)
	}
}

// TestStoreMethodsRefuseNilContext calls each method on a Store whose bucket handle answers every
// call with success and counts it: a method that reached the bucket before checking its context
// returns no error.
func TestStoreMethodsRefuseNilContext(t *testing.T) {
	objects := &okObjects{}
	s := &Store{objects: objects}
	calls := map[string]func() error{
		"Put":    func() error { return s.Put(nilCtx, "k", []byte("v")) },
		"Get":    func() error { _, err := s.Get(nilCtx, "k"); return err },
		"Open":   func() error { _, err := s.Open(nilCtx, "k"); return err },
		"List":   func() error { _, err := s.List(nilCtx, ""); return err },
		"Delete": func() error { return s.Delete(nilCtx, "k") },
		"StoreContent": func() error {
			_, err := s.StoreContent(nilCtx, &content{entityID: validEntityID, raw: map[string]string{"body": "b"},
				fields: map[string]string{message.ContentRoleBody: "body"}})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !refusesNilContext(err) {
				t.Errorf("%s(nil ctx) error = %v, want the invalid-input nil-context refusal", name, err)
			}
			if objects.calls != 0 {
				t.Errorf("%s(nil ctx) reached the bucket %d times", name, objects.calls)
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	got := Config{}.withDefaults()
	if got.BucketName != "MESSAGES" || got.InstanceName != "MESSAGES" {
		t.Errorf("zero Config defaults to bucket %q, instance %q; want MESSAGES, MESSAGES", got.BucketName, got.InstanceName)
	}
	got = Config{BucketName: "CONTENT"}.withDefaults()
	if got.InstanceName != "CONTENT" {
		t.Errorf("instance defaults to %q, want the bucket name CONTENT", got.InstanceName)
	}
	got = Config{BucketName: "CONTENT", InstanceName: "docs"}.withDefaults()
	if got.BucketName != "CONTENT" || got.InstanceName != "docs" {
		t.Errorf("set values changed to bucket %q, instance %q", got.BucketName, got.InstanceName)
	}
}

// TestStoreContentRefusesInvalidEntityIDBeforeWriting uses a Store with no bucket handle: a write
// would panic. The payload counts reads of its content, which must not happen either.
func TestStoreContentRefusesInvalidEntityIDBeforeWriting(t *testing.T) {
	for _, id := range []string{"a.b.c.d.e", "a.b.c.d.e.-bad", "a.a.a.a.a." + strings.Repeat("x", 247)} {
		cs := &content{entityID: id, raw: map[string]string{"body": "b"}, fields: map[string]string{"body": "body"}}
		ref, err := (&Store{}).StoreContent(context.Background(), cs)
		if !errs.IsInvalid(err) || ref != nil {
			t.Errorf("StoreContent(%q) = %v, %v; want nil and an invalid-input error", id, ref, err)
		}
		if cs.reads != 0 {
			t.Errorf("StoreContent(%q) read the content %d times before refusing the entity ID", id, cs.reads)
		}
	}
}

// TestStoreContentRefusesNonBodyRoleAliasedToBody names the body's own field under a second,
// non-body role: that role carries non-empty content, so the owner ruling (PR #432 comment
// 6447702631) refuses it as it would a separate field (#433).
func TestStoreContentRefusesNonBodyRoleAliasedToBody(t *testing.T) {
	objects := &okObjects{}
	s := &Store{objects: objects}
	cs := &content{
		entityID: validEntityID,
		raw:      map[string]string{"text": "body bytes"},
		fields:   map[string]string{message.ContentRoleBody: "text", message.ContentRoleAbstract: "text"},
	}
	ref, err := s.StoreContent(context.Background(), cs)
	if !errs.IsInvalid(err) || ref != nil || objects.calls != 0 {
		t.Fatalf("StoreContent = %+v, %v with %d writes; want nil, an invalid-input error and no write",
			ref, err, objects.calls)
	}
	for _, name := range []string{`"text"`, `role "abstract"`} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

// TestRefusalsCountAsOperationErrors drives each operation's refusals through the public call and
// reads the gathered counter: every returned error counts once, refused input included (D7, #434).
func TestRefusalsCountAsOperationErrors(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	m, err := newStoreMetrics(registry, "CONTENT")
	if err != nil {
		t.Fatalf("newStoreMetrics: %v", err)
	}
	objects := &okObjects{}
	s := &Store{objects: objects, metrics: m}
	ctx := context.Background()
	body := func(id string, raw map[string]string) *content {
		return &content{entityID: id, raw: raw,
			fields: map[string]string{message.ContentRoleBody: "body", message.ContentRoleAbstract: "description"}}
	}
	calls := []struct {
		operation string
		call      func() error
	}{
		{"put", func() error { return s.Put(nilCtx, "k", []byte("v")) }},
		{"get", func() error { _, err := s.Get(nilCtx, "k"); return err }},
		{"open", func() error { _, err := s.Open(nilCtx, "k"); return err }},
		{"list", func() error { _, err := s.List(nilCtx, ""); return err }},
		{"delete", func() error { return s.Delete(nilCtx, "k") }},
		{"store_content", func() error {
			_, err := s.StoreContent(nilCtx, body(validEntityID, map[string]string{"body": "b"}))
			return err
		}},
		{"store_content", func() error { _, err := s.StoreContent(ctx, nil); return err }},
		{"store_content", func() error {
			_, err := s.StoreContent(ctx, body("a.b.c.d.e", map[string]string{"body": "b"}))
			return err
		}},
		{"store_content", func() error {
			_, err := s.StoreContent(ctx, body(validEntityID, map[string]string{"body": "b", "description": "extra"}))
			return err
		}},
	}
	want := map[string]float64{}
	for _, c := range calls {
		if err := c.call(); !errs.IsInvalid(err) {
			t.Fatalf("%s refusal error = %v, want an invalid-input error", c.operation, err)
		}
		want[c.operation]++
	}
	if objects.calls != 0 {
		t.Fatalf("refusals reached the bucket %d times, want none", objects.calls)
	}
	family := gather(t, registry)["semengine_objectstore_operation_errors_total"]
	for operation, n := range want {
		got := counterValue(family, map[string]string{"bucket": "CONTENT", "operation": operation})
		if got != n {
			t.Errorf("operation_errors_total{operation=%q} = %v after %v returned errors, want %v", operation, got, n, n)
		}
	}
}

// TestStoreContentKeysAreUniquePerCall asks for many keys for one entity as fast as it can; the
// clock may return one reading for all of them, so uniqueness must not come from it.
func TestStoreContentKeysAreUniquePerCall(t *testing.T) {
	const n = 10_000
	seen := make(map[string]bool, n)
	for range n {
		key := contentKey(validEntityID)
		if seen[key] {
			t.Fatalf("contentKey repeated %q", key)
		}
		seen[key] = true
		if !strings.HasPrefix(key, "content/") || !strings.Contains(key, "/"+validEntityID+"_") {
			t.Fatalf("contentKey = %q, want content/<yyyy>/<mm>/<dd>/<hh>/%s_<nonce>", key, validEntityID)
		}
	}
}

func TestStoreMetricsRegisterOnInjectedRegistry(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	m, err := newStoreMetrics(registry, "CONTENT")
	if err != nil {
		t.Fatalf("newStoreMetrics: %v", err)
	}
	m.recordWriteOp("put")
	failed := errors.New("failed")
	m.recordError("put", &failed)

	families := gather(t, registry)
	write := families["semengine_objectstore_write_operations_total"]
	if got := counterValue(write, map[string]string{"bucket": "CONTENT", "operation": "put"}); got != 1 {
		t.Errorf("write_operations_total{bucket=CONTENT,operation=put} = %v, want 1", got)
	}
	errsFamily := families["semengine_objectstore_operation_errors_total"]
	if got := counterValue(errsFamily, map[string]string{"bucket": "CONTENT", "operation": "put"}); got != 1 {
		t.Errorf("operation_errors_total{bucket=CONTENT,operation=put} = %v, want 1", got)
	}
	for name := range families {
		if strings.HasPrefix(name, "semengine_objectstore_") &&
			(strings.HasSuffix(name, "object_count") || strings.HasSuffix(name, "storage_bytes") || strings.Contains(name, "cache")) {
			t.Errorf("registry gathers %s, a series the design drops (D6, D7)", name)
		}
	}
}

func TestNilRegistryRecordsNothing(t *testing.T) {
	m, err := newStoreMetrics(nil, "CONTENT")
	if err != nil || m != nil {
		t.Fatalf("newStoreMetrics(nil) = %v, %v; want nil, nil", m, err)
	}
	// Each recorder is safe on the nil value a Store without a registry holds.
	m.recordReadOp("get")
	m.recordWriteOp("put")
	m.recordDeleteOp()
	m.recordListOp()
	m.recordReadLatency("get", time.Now())
	m.recordWriteLatency("put", time.Now())
	m.recordDeleteLatency(time.Now())
	m.recordListLatency(time.Now())
	failed := errors.New("failed")
	m.recordError("get", &failed)
}

// TestRetentionGuardFailsClosed is the pin's TestReconcileNoLifecycleRetention fatal case: a
// stream update that does not take (real NATS always takes it, so a fake stands in for the
// denial) leaves the retention binding, and the guard fails with a fatal retention error.
func TestRetentionGuardFailsClosed(t *testing.T) {
	js := &deniedUpdateJS{info: jetstream.StreamInfo{Config: jetstream.StreamConfig{
		Name: natsclient.ObjectStoreStreamPrefix + "CONTENT", MaxAge: 24 * time.Hour, MaxBytes: -1,
	}}}
	err := reconcileNoLifecycleRetention(context.Background(), js, "CONTENT",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !errors.Is(err, natsclient.ErrGraphBucketRetention) || !errs.IsFatal(err) {
		t.Fatalf("guard error = %v, want a fatal error matching ErrGraphBucketRetention", err)
	}
	if !js.updateTried {
		t.Error("the guard did not try to strip the retention before failing")
	}
}

// okObjects is a bucket handle whose calls all succeed with nothing; calls counts them. Only the
// calls the Store makes are modelled; any other call panics on the nil embed.
type okObjects struct {
	jetstream.ObjectStore
	calls int
}

func (o *okObjects) PutBytes(context.Context, string, []byte) (*jetstream.ObjectInfo, error) {
	o.calls++
	return &jetstream.ObjectInfo{}, nil
}

func (o *okObjects) GetBytes(context.Context, string, ...jetstream.GetObjectOpt) ([]byte, error) {
	o.calls++
	return nil, nil
}

func (o *okObjects) Get(context.Context, string, ...jetstream.GetObjectOpt) (jetstream.ObjectResult, error) {
	o.calls++
	return nil, nil
}

func (o *okObjects) Delete(context.Context, string) error {
	o.calls++
	return nil
}

func (o *okObjects) List(context.Context, ...jetstream.ListObjectsOpt) ([]*jetstream.ObjectInfo, error) {
	o.calls++
	return nil, nil
}

// deniedUpdateJS answers Stream with a stream whose info carries a binding retention, and refuses
// every UpdateStream. Only those two calls are modelled; any other call panics on the nil embed.
type deniedUpdateJS struct {
	jetstream.JetStream
	info        jetstream.StreamInfo
	updateTried bool
}

func (f *deniedUpdateJS) Stream(context.Context, string) (jetstream.Stream, error) {
	return &fixedInfoStream{info: f.info}, nil
}

func (f *deniedUpdateJS) UpdateStream(context.Context, jetstream.StreamConfig) (jetstream.Stream, error) {
	f.updateTried = true
	return nil, errors.New("update denied")
}

type fixedInfoStream struct {
	jetstream.Stream
	info jetstream.StreamInfo
}

func (s *fixedInfoStream) Info(context.Context, ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error) {
	info := s.info
	return &info, nil
}

const validEntityID = "acme.ops.docs.library.document.manual-1"

// content is a message.ContentStorable whose raw content and role map the test sets; reads counts
// the calls that read either.
type content struct {
	entityID string
	raw      map[string]string
	fields   map[string]string
	reads    int
}

func (c *content) EntityID() string                      { return c.entityID }
func (c *content) Triples() []message.Triple             { return nil }
func (c *content) StorageRef() *message.StorageReference { return nil }
func (c *content) RawContent() map[string]string         { c.reads++; return c.raw }
func (c *content) ContentFields() map[string]string      { c.reads++; return c.fields }
func gather(t *testing.T, r *metric.MetricsRegistry) map[string]*dto.MetricFamily {
	t.Helper()
	families, err := r.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	byName := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		byName[f.GetName()] = f
	}
	return byName
}

// counterValue is the value of the counter in family whose labels are exactly want, or 0.
func counterValue(family *dto.MetricFamily, want map[string]string) float64 {
	for _, m := range family.GetMetric() {
		if len(m.GetLabel()) != len(want) {
			continue
		}
		match := true
		for _, l := range m.GetLabel() {
			if want[l.GetName()] != l.GetValue() {
				match = false
			}
		}
		if match {
			return m.GetCounter().GetValue()
		}
	}
	return 0
}
