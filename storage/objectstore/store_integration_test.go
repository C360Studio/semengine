//go:build integration

package objectstore_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	dto "github.com/prometheus/client_model/go"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage"
	"github.com/c360studio/semengine/storage/objectstore"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

// fixtureClient resets the package's shared server and returns a Client connected to it, closed on
// t.Cleanup before the reset's connection check runs.
func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	return natsfixture.Open(t, shared.Reset(t), openClient)
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(15*time.Second),
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

func newStore(t *testing.T, client *natsclient.Client, cfg objectstore.Config) *objectstore.Store {
	t.Helper()
	s, err := objectstore.NewStoreWithConfig(context.Background(), client, cfg)
	if err != nil {
		t.Fatalf("NewStoreWithConfig(%+v): %v", cfg, err)
	}
	return s
}

func mustPut(t *testing.T, s *objectstore.Store, key, value string) {
	t.Helper()
	if err := s.Put(context.Background(), key, []byte(value)); err != nil {
		t.Fatalf("Put(%q): %v", key, err)
	}
}

func mustList(t *testing.T, s *objectstore.Store, prefix string) []string {
	t.Helper()
	keys, err := s.List(context.Background(), prefix)
	if err != nil {
		t.Fatalf("List(%q): %v", prefix, err)
	}
	return keys
}

// backingStream reads the bucket's backing stream config straight from JetStream.
func backingStream(t *testing.T, client *natsclient.Client, bucket string) jetstream.StreamConfig {
	t.Helper()
	ctx := context.Background()
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	stream, err := js.Stream(ctx, natsclient.ObjectStoreStreamPrefix+bucket)
	if err != nil {
		t.Fatalf("Stream(OBJ_%s): %v", bucket, err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("Info(OBJ_%s): %v", bucket, err)
	}
	return info.Config
}

func TestPutReplacesObjectAtKey(t *testing.T) {
	s := newStore(t, fixtureClient(t), objectstore.Config{BucketName: "REPLACE"})
	mustPut(t, s, "doc/1", "first")
	mustPut(t, s, "doc/1", "second")
	got, err := s.Get(context.Background(), "doc/1")
	if err != nil || string(got) != "second" {
		t.Fatalf("Get after two puts = %q, %v; want \"second\"", got, err)
	}
	if keys := mustList(t, s, ""); !slices.Equal(keys, []string{"doc/1"}) {
		t.Errorf("List = %q, want one object [doc/1]", keys)
	}
}

func TestGetAndOpenReportMissingAsNotFound(t *testing.T) {
	s := newStore(t, fixtureClient(t), objectstore.Config{BucketName: "MISSING"})
	ctx := context.Background()

	_, getErr := s.Get(ctx, "content/never/written")
	r, openErr := s.Open(ctx, "content/never/written")
	if r != nil {
		_ = r.Close()
		t.Error("Open of a missing key returned a reader")
	}
	for name, err := range map[string]error{"Get": getErr, "Open": openErr} {
		if !errors.Is(err, storage.ErrObjectNotFound) {
			t.Errorf("%s of a missing key: error %v does not match storage.ErrObjectNotFound", name, err)
		}
		if errs.IsTransient(err) {
			t.Errorf("%s of a missing key: error %v is classified transient", name, err)
		}
	}
}

func TestDeleteMissingKeySucceeds(t *testing.T) {
	s := newStore(t, fixtureClient(t), objectstore.Config{BucketName: "DELETE"})
	ctx := context.Background()
	if err := s.Delete(ctx, "never/written"); err != nil {
		t.Errorf("Delete of a never-written key = %v, want nil", err)
	}
	// Regression check, observed in the probe, not promised: a second delete of a deleted key.
	mustPut(t, s, "once", "v")
	if err := s.Delete(ctx, "once"); err != nil {
		t.Fatalf("Delete of a written key: %v", err)
	}
	if err := s.Delete(ctx, "once"); err != nil {
		t.Errorf("Delete of an already-deleted key = %v, want nil", err)
	}
	if _, err := s.Get(ctx, "once"); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("Get after Delete: error %v does not match storage.ErrObjectNotFound", err)
	}
}

func TestListSortedAndEmpty(t *testing.T) {
	s := newStore(t, fixtureClient(t), objectstore.Config{BucketName: "LIST"})
	if keys := mustList(t, s, ""); keys == nil || len(keys) != 0 {
		t.Errorf("List on an empty bucket = %#v, want an empty, non-nil slice", keys)
	}
	for _, k := range []string{"c/2", "a/1", "b/9"} {
		mustPut(t, s, k, k)
	}
	if keys := mustList(t, s, ""); !slices.Equal(keys, []string{"a/1", "b/9", "c/2"}) {
		t.Errorf("List(\"\") = %q, want [a/1 b/9 c/2]", keys)
	}
	if keys := mustList(t, s, "b/"); !slices.Equal(keys, []string{"b/9"}) {
		t.Errorf("List(\"b/\") = %q, want [b/9]", keys)
	}
	if keys := mustList(t, s, "z/"); keys == nil || len(keys) != 0 {
		t.Errorf("List with no match = %#v, want an empty, non-nil slice", keys)
	}
}

// doc is a message.ContentStorable whose content and role map each test sets.
type doc struct {
	raw    map[string]string
	fields map[string]string
}

func (d doc) EntityID() string                      { return "acme.ops.docs.library.document.manual-1" }
func (d doc) Triples() []message.Triple             { return nil }
func (d doc) StorageRef() *message.StorageReference { return nil }
func (d doc) RawContent() map[string]string         { return d.raw }
func (d doc) ContentFields() map[string]string      { return d.fields }

// media is a doc that also carries binary fields.
type media struct {
	doc
	binary map[string]message.BinaryContent
}

func (m media) BinaryFields() map[string]message.BinaryContent { return m.binary }

var bodyRoles = map[string]string{message.ContentRoleBody: "body", message.ContentRoleAbstract: "description",
	message.ContentRoleTitle: "title"}

func TestStoreContentRefusesExtras(t *testing.T) {
	s := newStore(t, fixtureClient(t), objectstore.Config{BucketName: "EXTRAS"})
	cases := map[string]struct {
		cs    message.ContentStorable
		field string
	}{
		"body and description": {doc{raw: map[string]string{"body": "text", "description": "summary"}, fields: bodyRoles},
			"description"},
		"description without body": {doc{raw: map[string]string{"description": "summary"}, fields: bodyRoles},
			"description"},
		"title only": {doc{raw: map[string]string{"title": "Manual"}, fields: bodyRoles}, "title"},
		"field no role names": {doc{raw: map[string]string{"body": "text", "summary": "s"}, fields: bodyRoles},
			"summary"},
		"one binary field": {media{doc: doc{raw: map[string]string{"body": "text"}, fields: bodyRoles},
			binary: map[string]message.BinaryContent{"video": {ContentType: "video/mp4", Data: []byte{1}}}}, "video"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ref, err := s.StoreContent(context.Background(), tc.cs)
			if ref != nil || !errs.IsInvalid(err) {
				t.Fatalf("StoreContent = %+v, %v; want nil and an invalid-input error", ref, err)
			}
			if !strings.Contains(err.Error(), strconv.Quote(tc.field)) {
				t.Errorf("error %q does not name the refused field %q", err, tc.field)
			}
			if keys := mustList(t, s, ""); len(keys) != 0 {
				t.Errorf("bucket holds %q after a refusal, want nothing written", keys)
			}
		})
	}
	// An empty binary field is not content: nothing is lost, so the body is stored.
	t.Run("empty binary field beside a body is stored", func(t *testing.T) {
		cs := media{doc: doc{raw: map[string]string{"body": "text"}, fields: bodyRoles},
			binary: map[string]message.BinaryContent{"video": {ContentType: "video/mp4"}}}
		ref, err := s.StoreContent(context.Background(), cs)
		if err != nil {
			t.Fatalf("StoreContent: %v", err)
		}
		if got, err := s.Get(context.Background(), ref.Key); err != nil || string(got) != "text" {
			t.Errorf("Get(reference) = %q, %v; want the body", got, err)
		}
	})
}

func TestStoreContentReferenceOpensToBody(t *testing.T) {
	s := newStore(t, fixtureClient(t), objectstore.Config{BucketName: "CONTENT", InstanceName: "docs"})
	ctx := context.Background()
	const body = "Full document text, not JSON: {\"a\": 1}"
	// An empty non-body field is not content: nothing is refused or lost.
	ref, err := s.StoreContent(ctx, doc{raw: map[string]string{"body": body, "title": ""}, fields: bodyRoles})
	if err != nil {
		t.Fatalf("StoreContent: %v", err)
	}

	registry := storeregistry.New()
	if err := registry.Register(s.InstanceName(), s); err != nil {
		t.Fatalf("Register: %v", err)
	}
	store, ok := registry.Streamable(ref.StorageInstance)
	if !ok {
		t.Fatalf("reference instance %q does not resolve; the store is registered as %q", ref.StorageInstance, "docs")
	}
	r, err := store.Open(ctx, ref.Key)
	if err != nil {
		t.Fatalf("Open(%q): %v", ref.Key, err)
	}
	got, err := io.ReadAll(r)
	if cerr := r.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != body {
		t.Errorf("reference opens to %q, want the raw body %q", got, body)
	}
	if ref.ContentType != "text/plain; charset=utf-8" || ref.Size != int64(len(body)) {
		t.Errorf("reference ContentType %q Size %d, want text/plain; charset=utf-8 and %d", ref.ContentType, ref.Size, len(body))
	}
	if !strings.HasPrefix(ref.Key, "content/") || !strings.Contains(ref.Key, "/"+doc{}.EntityID()+"_") {
		t.Errorf("reference key %q, want content/…/<entity>_<nonce>", ref.Key)
	}
}

func TestStoreContentWithNoContentStoresEmptyObject(t *testing.T) {
	s := newStore(t, fixtureClient(t), objectstore.Config{BucketName: "EMPTY_CONTENT"})
	ref, err := s.StoreContent(context.Background(), doc{fields: bodyRoles})
	if err != nil {
		t.Fatalf("StoreContent: %v", err)
	}
	if ref.StorageInstance != "EMPTY_CONTENT" || ref.Size != 0 {
		t.Errorf("reference instance %q size %d, want the bucket name and 0", ref.StorageInstance, ref.Size)
	}
	got, err := s.Get(context.Background(), ref.Key)
	if err != nil || len(got) != 0 {
		t.Errorf("Get(reference) = %q, %v; want an empty object", got, err)
	}
}

func TestRetentionGuardStripsAndAsserts(t *testing.T) {
	client := fixtureClient(t)
	ctx := context.Background()
	const bucket = "LEGACY"
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	legacy, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: bucket, TTL: 24 * time.Hour, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatalf("create legacy bucket: %v", err)
	}
	if _, err := legacy.PutBytes(ctx, "evidence/key", []byte("verbatim body")); err != nil {
		t.Fatalf("PutBytes: %v", err)
	}
	if cfg := backingStream(t, client, bucket); cfg.MaxAge != 24*time.Hour || cfg.MaxBytes != 1<<20 {
		t.Fatalf("precondition: backing stream MaxAge %v MaxBytes %d", cfg.MaxAge, cfg.MaxBytes)
	}

	var logs bytes.Buffer
	s := newStore(t, client, objectstore.Config{BucketName: bucket, Logger: slog.New(slog.NewTextHandler(&logs, nil))})

	if cfg := backingStream(t, client, bucket); cfg.MaxAge != 0 || cfg.MaxBytes > 0 {
		t.Errorf("after construction backing stream MaxAge %v MaxBytes %d, want 0 and <= 0", cfg.MaxAge, cfg.MaxBytes)
	}
	line := logs.String()
	for _, want := range []string{"level=WARN", "bucket=" + bucket, "removed_max_age=24h0m0s", "removed_max_bytes=1048576"} {
		if !strings.Contains(line, want) {
			t.Errorf("warning %q does not carry %q", line, want)
		}
	}
	if got, err := s.Get(ctx, "evidence/key"); err != nil || string(got) != "verbatim body" {
		t.Errorf("Get after the strip = %q, %v; want the stored object kept", got, err)
	}
}

func TestFreshBucketHasNoLifecycleRetention(t *testing.T) {
	client := fixtureClient(t)
	newStore(t, client, objectstore.Config{BucketName: "FRESH"})
	if cfg := backingStream(t, client, "FRESH"); cfg.MaxAge != 0 || cfg.MaxBytes > 0 {
		t.Errorf("fresh backing stream MaxAge %v MaxBytes %d, want 0 and <= 0", cfg.MaxAge, cfg.MaxBytes)
	}
	// Constructing again opens the existing bucket.
	newStore(t, client, objectstore.Config{BucketName: "FRESH"})
}

func TestGetReadsNATSEveryTime(t *testing.T) {
	client := fixtureClient(t)
	first := newStore(t, client, objectstore.Config{BucketName: "NOCACHE"})
	second := newStore(t, client, objectstore.Config{BucketName: "NOCACHE"})
	mustPut(t, first, "k", "v")
	if got, err := first.Get(context.Background(), "k"); err != nil || string(got) != "v" {
		t.Fatalf("first Get = %q, %v", got, err)
	}
	if err := second.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("Delete through the second store: %v", err)
	}
	if _, err := first.Get(context.Background(), "k"); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("Get after another store's delete: error %v, want storage.ErrObjectNotFound", err)
	}
}

func TestStoreConcurrentUseUnderRace(t *testing.T) {
	s := newStore(t, fixtureClient(t), objectstore.Config{BucketName: "CONCURRENT"})
	const workers, rounds = 8, 10
	ctx := context.Background()
	var wg sync.WaitGroup
	failures := make(chan error, workers*rounds)
	for w := range workers {
		wg.Go(func() {
			for i := range rounds {
				key := fmt.Sprintf("w%d/%d", w, i)
				if err := s.Put(ctx, key, []byte(key)); err != nil {
					failures <- err
					return
				}
				if got, err := s.Get(ctx, key); err != nil || string(got) != key {
					failures <- fmt.Errorf("Get(%q) = %q, %v", key, got, err)
					return
				}
				if _, err := s.List(ctx, fmt.Sprintf("w%d/", w)); err != nil {
					failures <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if keys := mustList(t, s, ""); len(keys) != workers*rounds {
		t.Errorf("List after the workers = %d keys, want %d", len(keys), workers*rounds)
	}
}

func TestTwoStoresOnOneBucketShareCollectors(t *testing.T) {
	client := fixtureClient(t)
	registry := metric.NewMetricsRegistry()
	cfg := objectstore.Config{BucketName: "METRICS"}
	ctx := context.Background()
	first, err := objectstore.NewStoreWithConfigAndMetrics(ctx, client, cfg, registry)
	if err != nil {
		t.Fatalf("first store: %v", err)
	}
	second, err := objectstore.NewStoreWithConfigAndMetrics(ctx, client, cfg, registry)
	if err != nil {
		t.Fatalf("second store on the same bucket and registry: %v", err)
	}
	mustPut(t, first, "a", "1")
	mustPut(t, second, "b", "2")
	if got := writeCount(t, registry, "METRICS"); got != 2 {
		t.Errorf("write_operations_total{bucket=METRICS,operation=put} = %v, want 2 (one per store)", got)
	}
}

func TestTwoBucketsOnOneRegistryBothConstruct(t *testing.T) {
	client := fixtureClient(t)
	registry := metric.NewMetricsRegistry()
	ctx := context.Background()
	one, err := objectstore.NewStoreWithConfigAndMetrics(ctx, client, objectstore.Config{BucketName: "ONE"}, registry)
	if err != nil {
		t.Fatalf("store on ONE: %v", err)
	}
	two, err := objectstore.NewStoreWithConfigAndMetrics(ctx, client, objectstore.Config{BucketName: "TWO"}, registry)
	if err != nil {
		t.Fatalf("store on TWO with the same registry: %v", err)
	}
	mustPut(t, one, "a", "1")
	mustPut(t, two, "a", "1")
	mustPut(t, two, "b", "2")
	if got := writeCount(t, registry, "ONE"); got != 1 {
		t.Errorf("writes labelled ONE = %v, want 1", got)
	}
	if got := writeCount(t, registry, "TWO"); got != 2 {
		t.Errorf("writes labelled TWO = %v, want 2", got)
	}
}

// writeCount reads write_operations_total{bucket,operation="put"} from the registry.
func writeCount(t *testing.T, registry *metric.MetricsRegistry, bucket string) float64 {
	t.Helper()
	families, err := registry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != "semengine_objectstore_write_operations_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			if labels(m) == "bucket="+bucket+",operation=put" {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func labels(m *dto.Metric) string {
	var parts []string
	for _, l := range m.GetLabel() {
		parts = append(parts, l.GetName()+"="+l.GetValue())
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// historyChecks bounds TestStoreHistoryMatchesModel's generated runs (design D9) unless the
// command line or RAPID_CHECKS asks for another count.
const historyChecks = 50

// TestStoreHistoryMatchesModel runs generated operation histories on two Stores over one bucket
// and compares every result with a plain map, the model: Put sets, Delete removes, Get of an
// absent key is not found, List is the sorted keys under the prefix. Each run uses a new bucket.
func TestStoreHistoryMatchesModel(t *testing.T) {
	boundRapidChecks(t, historyChecks)
	client := fixtureClient(t)
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	keys := []string{"a/1", "a/2", "b/1", "b/10"}
	values := []string{"x", "y"}
	prefixes := []string{"", "a/", "b/1", "b/10", "c/"}
	run := 0
	// ran counts each assertion kind that executed, logged so a run shows which ones it exercised.
	ran := map[string]int{}

	rapid.Check(t, func(rt *rapid.T) {
		run++
		ctx := context.Background()
		bucket := fmt.Sprintf("HISTORY_%d", run)
		stores := [2]*objectstore.Store{}
		for i := range stores {
			s, err := objectstore.NewStoreWithConfig(ctx, client, objectstore.Config{BucketName: bucket})
			if err != nil {
				rt.Fatalf("NewStoreWithConfig: %v", err)
			}
			stores[i] = s
		}
		defer func() {
			if err := js.DeleteObjectStore(ctx, bucket); err != nil {
				rt.Errorf("DeleteObjectStore(%s): %v", bucket, err)
			}
		}()

		model := map[string]string{}
		n := rapid.IntRange(1, 30).Draw(rt, "operations")
		for range n {
			s := stores[rapid.IntRange(0, 1).Draw(rt, "store")]
			switch rapid.SampledFrom([]string{"put", "delete", "get", "list"}).Draw(rt, "op") {
			case "put":
				k, v := rapid.SampledFrom(keys).Draw(rt, "key"), rapid.SampledFrom(values).Draw(rt, "value")
				if err := s.Put(ctx, k, []byte(v)); err != nil {
					rt.Fatalf("Put(%q): %v", k, err)
				}
				model[k] = v
				ran["put"]++
			case "delete":
				k := rapid.SampledFrom(keys).Draw(rt, "key")
				if err := s.Delete(ctx, k); err != nil {
					rt.Fatalf("Delete(%q): %v", k, err)
				}
				if _, present := model[k]; !present {
					ran["delete absent"]++
				}
				delete(model, k)
			case "get":
				k := rapid.SampledFrom(keys).Draw(rt, "key")
				got, err := s.Get(ctx, k)
				want, present := model[k]
				switch {
				case present && (err != nil || string(got) != want):
					rt.Fatalf("Get(%q) = %q, %v; model has %q", k, got, err, want)
				case !present && !errors.Is(err, storage.ErrObjectNotFound):
					rt.Fatalf("Get(%q) = %q, %v; model has no %q, want storage.ErrObjectNotFound", k, got, err, k)
				}
				ran[map[bool]string{true: "get present", false: "get absent"}[present]]++
			case "list":
				p := rapid.SampledFrom(prefixes).Draw(rt, "prefix")
				got, err := s.List(ctx, p)
				if err != nil {
					rt.Fatalf("List(%q): %v", p, err)
				}
				want := []string{}
				for k := range model {
					if strings.HasPrefix(k, p) {
						want = append(want, k)
					}
				}
				sort.Strings(want)
				if got == nil || !slices.Equal(got, want) {
					rt.Fatalf("List(%q) = %#v; model gives %#v", p, got, want)
				}
				ran[map[bool]string{true: "list empty", false: "list non-empty"}[len(want) == 0]]++
			}
		}
	})
	t.Logf("history runs completed: %d; assertions run: %v", run, ran)
}

// boundRapidChecks sets -rapid.checks to n for this test unless the command line or RAPID_CHECKS
// already chose a count, and restores it after.
func boundRapidChecks(t *testing.T, n int) {
	t.Helper()
	chosen := os.Getenv("RAPID_CHECKS") != ""
	flag.Visit(func(f *flag.Flag) { chosen = chosen || f.Name == "rapid.checks" })
	f := flag.Lookup("rapid.checks")
	if chosen || f == nil {
		return
	}
	old := f.Value.String()
	if err := f.Value.Set(strconv.Itoa(n)); err != nil {
		t.Fatalf("set rapid.checks: %v", err)
	}
	t.Cleanup(func() { _ = f.Value.Set(old) })
}
