//go:build integration

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
)

// The message logger's KV query route against a real NATS server (design D4, D7 U-e). The pin
// drove these cases through an injected bucket provider (message_logger_http_kv_query_test.go:98-125,
// :149-205) and an unexported handler (message_logger_kv_acquisition_integration_test.go:18-145);
// here every request goes through the routes RegisterHTTPHandlers mounts, and the buckets are real.

// kvRouteBudget bounds each test's NATS calls and requests.
const kvRouteBudget = 10 * time.Second

// kvRouteResult is the KV query's JSON answer. Value stays raw, so a test compares it with the
// bytes it put.
type kvRouteResult struct {
	Bucket  string `json:"bucket"`
	Pattern string `json:"pattern"`
	Count   int    `json:"count"`
	Entries []struct {
		Key      string          `json:"key"`
		Value    json.RawMessage `json:"value"`
		Revision uint64          `json:"revision"`
	} `json:"entries"`
}

// kvRouteMux mounts ml's routes under /message-logger, as the service manager does.
func kvRouteMux(ml *MessageLogger) *http.ServeMux {
	mux := http.NewServeMux()
	ml.RegisterHTTPHandlers("/message-logger", mux)
	return mux
}

// serveKVRoute sends GET target through mux under ctx and returns the recorded answer.
func serveKVRoute(ctx context.Context, mux *http.ServeMux, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// decodeKVRoute decodes a 200 answer of the KV query.
func decodeKVRoute(t *testing.T, rec *httptest.ResponseRecorder) kvRouteResult {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var result kvRouteResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&result))
	return result
}

// entryKeys lists the keys of result's entries in the order the route answered them.
func entryKeys(result kvRouteResult) []string {
	keys := make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		keys = append(keys, entry.Key)
	}
	return keys
}

// TestKVQueryReadsExistingBucketsWithoutCreating: the query reads a framework catalog bucket and a
// product bucket that already exist, and answers the value put there (pin
// message_logger_kv_acquisition_integration_test.go:18-96).
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestKVQueryReadsExistingBucketsWithoutCreating(t *testing.T) {
	client := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), kvRouteBudget)
	defer cancel()

	frameworkKV, err := kvcatalog.EnsureCatalogBucket(ctx, client, graph.BucketGraphStatus)
	require.NoError(t, err)
	const productBucket = "MESSAGE_LOGGER_PRODUCT_FIXTURE"
	productKV, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: productBucket, History: 1})
	require.NoError(t, err)

	mux := kvRouteMux(newTestMessageLogger(t, client, `{"monitor_subjects":["*"]}`, nil))

	for _, fixture := range []struct {
		name   string
		bucket string
		kv     jetstream.KeyValue
		key    string
		value  string
	}{
		{"framework bucket", graph.BucketGraphStatus, frameworkKV, "message_logger.framework", `{"source":"framework"}`},
		{"product bucket", productBucket, productKV, "message_logger.product", `{"source":"product"}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			revision, err := fixture.kv.Put(ctx, fixture.key, []byte(fixture.value))
			require.NoError(t, err)

			result := decodeKVRoute(t, serveKVRoute(ctx, mux,
				"/message-logger/kv/"+fixture.bucket+"?pattern="+fixture.key))
			require.Equal(t, fixture.bucket, result.Bucket)
			require.Equal(t, fixture.key, result.Pattern)
			require.Equal(t, 1, result.Count)
			require.Len(t, result.Entries, 1)
			require.Equal(t, fixture.key, result.Entries[0].Key)
			require.JSONEq(t, fixture.value, string(result.Entries[0].Value))
			require.Equal(t, revision, result.Entries[0].Revision)
		})
	}

	buckets, err := client.ListKeyValueBuckets(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{graph.BucketGraphStatus, productBucket}, buckets,
		"reading existing buckets created another")
}

// TestKVQueryMissingBucketIs404AndCreatesNothing: a query for a bucket that does not exist answers
// 404, and neither the query nor a watch of that bucket creates it or any other (pin
// message_logger_http_kv_query_test.go:98-125 on a real server; acquisition test :126-144). The
// lookup is classified with errors.Is(err, jetstream.ErrBucketNotFound) (D4, S7).
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestKVQueryMissingBucketIs404AndCreatesNothing(t *testing.T) {
	client := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), kvRouteBudget)
	defer cancel()

	const missingBucket = "MESSAGE_LOGGER_MISSING_FIXTURE"
	before, err := client.ListKeyValueBuckets(ctx)
	require.NoError(t, err)
	require.NotContains(t, before, missingBucket, "precondition: the missing bucket must not exist")

	mux := kvRouteMux(newTestMessageLogger(t, client, `{"monitor_subjects":["*"]}`, nil))

	rec := serveKVRoute(ctx, mux, "/message-logger/kv/"+missingBucket)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	// The watch route looks the bucket up the same way; whatever it answers, it must not create it.
	serveKVRoute(ctx, mux, "/message-logger/kv/"+missingBucket+"/watch")
	require.NoError(t, ctx.Err(), "the watch of a missing bucket did not return within %s", kvRouteBudget)

	after, err := client.ListKeyValueBuckets(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, before, after, "a query or watch of a missing bucket created persistent state")
	_, err = client.GetKeyValueBucket(ctx, missingBucket)
	require.ErrorIs(t, err, jetstream.ErrBucketNotFound)
}

// TestKVQueryEmptyBucketReturnsNoEntries: a bucket that exists with no keys answers 200 with no
// entries, not an error; nats.go reports that listing as jetstream.ErrNoKeysFound (D4, S7).
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestKVQueryEmptyBucketReturnsNoEntries(t *testing.T) {
	client := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), kvRouteBudget)
	defer cancel()

	const emptyBucket = "MESSAGE_LOGGER_EMPTY_FIXTURE"
	_, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: emptyBucket, History: 1})
	require.NoError(t, err)

	mux := kvRouteMux(newTestMessageLogger(t, client, `{"monitor_subjects":["*"]}`, nil))

	rec := serveKVRoute(ctx, mux, "/message-logger/kv/"+emptyBucket)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var result struct {
		Bucket  string          `json:"bucket"`
		Count   int             `json:"count"`
		Entries json.RawMessage `json:"entries"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&result))
	require.Equal(t, emptyBucket, result.Bucket)
	require.Equal(t, 0, result.Count)
	require.JSONEq(t, `[]`, string(result.Entries), "an empty bucket answers an empty list, not null")
}

// TestKVQueryFailedRecordRoundTrips: a failed EMBEDDING_INDEX record put on the server comes back
// through the route with status=failed, its operator fields intact, and a generated record beside
// it is filtered out (pin message_logger_http_kv_query_test.go:149-185, which round-tripped a map
// through encoding/json without the route; #613's per-entity forensic view).
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestKVQueryFailedRecordRoundTrips(t *testing.T) {
	client := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), kvRouteBudget)
	defer cancel()

	kv, err := kvcatalog.EnsureCatalogBucket(ctx, client, graph.BucketEmbeddingIndex)
	require.NoError(t, err)

	const failedKey = "acme.ops.src.embed.record.e1"
	const failedValue = `{"entity_id":"acme.ops.src.embed.record.e1","status":"failed",` +
		`"reason":"connection_refused","error_msg":"generation failed: dial tcp: connection refused"}`
	failedRevision, err := kv.Put(ctx, failedKey, []byte(failedValue))
	require.NoError(t, err)
	_, err = kv.Put(ctx, "acme.ops.src.embed.record.e2",
		[]byte(`{"entity_id":"acme.ops.src.embed.record.e2","status":"generated"}`))
	require.NoError(t, err)

	mux := kvRouteMux(newTestMessageLogger(t, client, `{"monitor_subjects":["*"]}`, nil))

	result := decodeKVRoute(t, serveKVRoute(ctx, mux,
		"/message-logger/kv/"+graph.BucketEmbeddingIndex+"?status=failed"))
	require.Equal(t, 1, result.Count)
	require.Len(t, result.Entries, 1)
	require.Equal(t, failedKey, result.Entries[0].Key)
	require.JSONEq(t, failedValue, string(result.Entries[0].Value), "operator-reachable fields lost on the route")
	require.Equal(t, failedRevision, result.Entries[0].Revision)

	all := decodeKVRoute(t, serveKVRoute(ctx, mux, "/message-logger/kv/"+graph.BucketEmbeddingIndex))
	require.Equal(t, 2, all.Count, "without status the route answers every record")
}

// TestKVQueryLookupFailureIs500: a bucket lookup that fails for any reason other than a missing
// bucket answers 500, not 404 — the not-found answer is a classification, not a catch-all (pin
// message_logger_http_kv_query_test.go:187-205). The failure is real: the logger's client never
// connected.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestKVQueryLookupFailureIs500(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), kvRouteBudget)
	defer cancel()

	mux := kvRouteMux(newTestMessageLogger(t, newUnconnectedClient(t), `{"monitor_subjects":["*"]}`, nil))

	rec := serveKVRoute(ctx, mux, "/message-logger/kv/"+graph.BucketEntityStates)
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
}

// TestKVQuerySortsKeysBeforeLimit: the query answers keys in byte order and applies limit to that
// order, whatever order the keys were written in (D4, M23; AGENTS.md § 2). nats.go v1.54.0 sorts
// Keys itself (jetstream/kv.go:1420) without promising it, so the mutant that drops the logger's
// sort survives at this version; the test is the guard for a nats.go that stops sorting.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestKVQuerySortsKeysBeforeLimit(t *testing.T) {
	client := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), kvRouteBudget)
	defer cancel()

	const bucket = "MESSAGE_LOGGER_SORT_FIXTURE"
	kv, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: bucket, History: 1})
	require.NoError(t, err)
	// Written out of order, and k.a rewritten last, so neither write order nor revision order is
	// byte order.
	for _, key := range []string{"k.c", "k.a", "k.d", "k.b", "k.a"} {
		_, err := kv.Put(ctx, key, []byte(`{"key":"`+key+`"}`))
		require.NoError(t, err)
	}

	mux := kvRouteMux(newTestMessageLogger(t, client, `{"monitor_subjects":["*"]}`, nil))

	limited := decodeKVRoute(t, serveKVRoute(ctx, mux, "/message-logger/kv/"+bucket+"?limit=2"))
	require.Equal(t, []string{"k.a", "k.b"}, entryKeys(limited), "limit applied before the sort")
	require.Equal(t, 2, limited.Count)

	all := decodeKVRoute(t, serveKVRoute(ctx, mux, "/message-logger/kv/"+bucket))
	require.Equal(t, []string{"k.a", "k.b", "k.c", "k.d"}, entryKeys(all))
}

// TestKVQueryPreservesValueBytes: the query answers each stored JSON value byte for byte, with and
// without the status filter, so an integer past 2^53, a nested number and a float spelling come back
// as stored (#370). The oracle is the stored bytes; JSONEq would decode both sides into float64 and
// hide the rounding. A value that is not JSON comes back as a JSON string, as the watch sends it.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestKVQueryPreservesValueBytes(t *testing.T) {
	client := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), kvRouteBudget)
	defer cancel()

	const bucket = "MESSAGE_LOGGER_NUMBER_FIXTURE"
	kv, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: bucket})
	require.NoError(t, err)
	const failedValue = `{"sequence":9007199254740993,"nested":{"n":[18446744073709551615,1.0e2,-0.0]},"status":"failed"}`
	_, err = kv.Put(ctx, "a.failed", []byte(failedValue))
	require.NoError(t, err)
	_, err = kv.Put(ctx, "b.raw", []byte("not json 9007199254740993"))
	require.NoError(t, err)

	mux := kvRouteMux(newTestMessageLogger(t, client, `{"monitor_subjects":["*"]}`, nil))

	filtered := decodeKVRoute(t, serveKVRoute(ctx, mux, "/message-logger/kv/"+bucket+"?status=failed"))
	require.Len(t, filtered.Entries, 1)
	require.Equal(t, failedValue, string(filtered.Entries[0].Value), "the filtered route changed the stored value")

	all := decodeKVRoute(t, serveKVRoute(ctx, mux, "/message-logger/kv/"+bucket))
	require.Equal(t, []string{"a.failed", "b.raw"}, entryKeys(all))
	require.Equal(t, failedValue, string(all.Entries[0].Value), "the unfiltered route changed the stored value")
	require.Equal(t, `"not json 9007199254740993"`, string(all.Entries[1].Value), "a non-JSON value must come back as a JSON string")
}

// TestKVQueryPatternMatchesBeforeLimit: a middle-wildcard pattern matches only keys long enough to
// hold both literal parts (#371), and a key it does not match never counts toward limit. In byte
// order the keys are aaXaa, aaa, aaaa; aa*aa matches aaXaa and aaaa (empty wildcard) but not aaa,
// whose prefix and suffix would share a character.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestKVQueryPatternMatchesBeforeLimit(t *testing.T) {
	client := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), kvRouteBudget)
	defer cancel()

	const bucket = "MESSAGE_LOGGER_PATTERN_FIXTURE"
	kv, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: bucket})
	require.NoError(t, err)
	for _, key := range []string{"aaa", "aaaa", "aaXaa"} {
		_, err := kv.Put(ctx, key, []byte(`{}`))
		require.NoError(t, err)
	}

	mux := kvRouteMux(newTestMessageLogger(t, client, `{"monitor_subjects":["*"]}`, nil))

	limited := decodeKVRoute(t, serveKVRoute(ctx, mux, "/message-logger/kv/"+bucket+"?pattern=aa*aa&limit=2"))
	require.Equal(t, []string{"aaXaa", "aaaa"}, entryKeys(limited), "a nonmatching key consumed the limit")
}
