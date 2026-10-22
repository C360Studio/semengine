//go:build integration

package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/natsclient"
)

// The KV watch route against a real bucket (design D2's streams, D4, owner rulings Q1(b) and
// Q1(e); D7 U-f). Each test builds the logger through U-a's helper over the shared server, starts
// it, and requests the watch through the logger's own mux registration.

// watchLoggerConfig monitors one explicit subject, so the logger starts without registry
// declarations; the watch reads no configuration.
const watchLoggerConfig = `{"monitor_subjects":["watch.logger"]}`

// watchDropWarn is the Warn record the watch writes when its buffer is full (pin
// message_logger_kv_watch.go:294; design D4, "the Warn line stays").
const watchDropWarn = "KV watch buffer full, dropping event"

// watchBound bounds every wait on a watch response; a stream still open at the bound fails the
// test, never hangs it.
const watchBound = 10 * time.Second

// sseEvent is one Server-Sent Event as written on the wire: its event name and its data line.
type sseEvent struct {
	name string
	data string
}

// watchEvent is a kv_change event's data, decoded independently of the logger's KVWatchEvent.
type watchEvent struct {
	Bucket    string          `json:"bucket"`
	Key       string          `json:"key"`
	Operation string          `json:"operation"`
	Value     json.RawMessage `json:"value"`
	Revision  uint64          `json:"revision"`
}

// readSSEEvent reads the next event that carries a name or data; a block with neither, such as
// the "retry:" directive, is passed over. It returns io.EOF when the stream ends between events.
func readSSEEvent(r *bufio.Reader) (sseEvent, error) {
	var event sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" && event == (sseEvent{}) {
				return event, io.EOF
			}
			// Not wrapped: only a clean end between events is io.EOF.
			return event, fmt.Errorf("SSE stream ended mid-event (%q): %v", line, err)
		}
		line = strings.TrimSuffix(line, "\n")
		switch {
		case line == "":
			if event != (sseEvent{}) {
				return event, nil
			}
		case strings.HasPrefix(line, "event: "):
			event.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			event.data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// readAllSSEEvents reads every event in a complete stream body.
func readAllSSEEvents(t *testing.T, body []byte) []sseEvent {
	t.Helper()
	r := bufio.NewReader(bytes.NewReader(body))
	var events []sseEvent
	for {
		event, err := readSSEEvent(r)
		if errors.Is(err, io.EOF) {
			return events
		}
		require.NoError(t, err, "body: %q", body)
		events = append(events, event)
	}
}

// startWatchLogger builds and starts a logger over client, composing logger (nil for the base
// service's default), and registers its routes on a fresh mux. The logger is stopped at cleanup
// under a bounded context.
func startWatchLogger(t *testing.T, client *natsclient.Client, logger *slog.Logger) (*MessageLogger, *http.ServeMux) {
	t.Helper()
	ml := newTestMessageLogger(t, client, watchLoggerConfig, logger)
	require.NoError(t, ml.Start(t.Context()))
	t.Cleanup(func() {
		assert.NoError(t, ml.Stop(stopContext(t)), "cleanup Stop")
	})
	mux := http.NewServeMux()
	ml.RegisterHTTPHandlers("/message-logger", mux)
	return ml, mux
}

// serveWatchLogger starts a logger as startWatchLogger does and serves its mux over HTTP.
func serveWatchLogger(t *testing.T, client *natsclient.Client) (*MessageLogger, *httptest.Server) {
	t.Helper()
	ml, mux := startWatchLogger(t, client, nil)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return ml, srv
}

// watchURL returns the watch route's URL for bucket, with pattern as its query when not empty.
func watchURL(base, bucket, pattern string) string {
	u := base + "/message-logger/kv/" + bucket + "/watch"
	if pattern != "" {
		u += "?" + url.Values{"pattern": {pattern}}.Encode()
	}
	return u
}

// openWatch requests the watch route under a watchBound deadline and returns the response; its
// body and request are released at cleanup.
func openWatch(t *testing.T, base, bucket, pattern string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), watchBound)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, watchURL(base, bucket, pattern), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		_ = resp.Body.Close()
	})
	return resp
}

// nextWatchEvent reads the next named event and, for a kv_change, decodes its data.
func nextWatchEvent(t *testing.T, r *bufio.Reader) (sseEvent, watchEvent) {
	t.Helper()
	event, err := readSSEEvent(r)
	require.NoError(t, err)
	var data watchEvent
	if event.name == "kv_change" {
		require.NoError(t, json.Unmarshal([]byte(event.data), &data), "kv_change data %q", event.data)
	}
	return event, data
}

// requireConnected reads the stream's first event and requires it to be connected for bucket.
func requireConnected(t *testing.T, r *bufio.Reader, bucket string) {
	t.Helper()
	event, _ := nextWatchEvent(t, r)
	require.Equal(t, "connected", event.name, "first event %+v", event)
	var connected struct {
		Bucket string `json:"bucket"`
	}
	require.NoError(t, json.Unmarshal([]byte(event.data), &connected))
	require.Equal(t, bucket, connected.Bucket)
}

// TestKVWatchStreamsInitialValuesThenChanges: the watch sends connected, then every key's current
// value, then initial_sync_complete, then each later change, and names each entry's operation as
// NATS records it (Q1(b)). The second key's first write is the bucket's revision 2, which the pin
// called "update"; a non-JSON value travels as a JSON string. Initial values are matched by key,
// not by order.
//
// spec: operator-surface / The message logger's KV watch streams a bucket's values and changes
func TestKVWatchStreamsInitialValuesThenChanges(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()
	const bucket = "WATCH_EVENTS"
	kv, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: bucket})
	require.NoError(t, err)
	firstRev, err := kv.Put(ctx, "first", []byte(`{"n":1}`))
	require.NoError(t, err)
	secondRev, err := kv.Put(ctx, "second", []byte("not json"))
	require.NoError(t, err)

	_, srv := serveWatchLogger(t, client)
	resp := openWatch(t, srv.URL, bucket, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	r := bufio.NewReader(resp.Body)
	requireConnected(t, r, bucket)

	initial := map[string]watchEvent{}
	for range 2 {
		event, data := nextWatchEvent(t, r)
		require.Equal(t, "kv_change", event.name, "event %+v", event)
		initial[data.Key] = data
	}
	require.Contains(t, initial, "first")
	require.Contains(t, initial, "second")
	assert.Equal(t, watchEvent{Bucket: bucket, Key: "first", Operation: "put", Value: json.RawMessage(`{"n":1}`), Revision: firstRev}, initial["first"])
	assert.Equal(t, "put", initial["second"].Operation, "a key's first write at bucket revision %d", secondRev)
	assert.Equal(t, secondRev, initial["second"].Revision)
	assert.JSONEq(t, `"not json"`, string(initial["second"].Value))

	event, data := nextWatchEvent(t, r)
	require.Equal(t, "kv_change", event.name)
	require.Equal(t, "initial_sync_complete", data.Operation, "event %+v", event)

	updateRev, err := kv.Put(ctx, "first", []byte(`{"n":2}`))
	require.NoError(t, err)
	event, data = nextWatchEvent(t, r)
	require.Equal(t, "kv_change", event.name)
	assert.Equal(t, watchEvent{Bucket: bucket, Key: "first", Operation: "put", Value: json.RawMessage(`{"n":2}`), Revision: updateRev}, data)

	require.NoError(t, kv.Delete(ctx, "second"))
	event, data = nextWatchEvent(t, r)
	require.Equal(t, "kv_change", event.name)
	assert.Equal(t, "second", data.Key)
	assert.Equal(t, "delete", data.Operation)

	require.NoError(t, kv.Purge(ctx, "first"))
	event, data = nextWatchEvent(t, r)
	require.Equal(t, "kv_change", event.name)
	assert.Equal(t, "first", data.Key)
	assert.Equal(t, "purge", data.Operation)
}

// TestKVWatchRejectsInvalidFilter: a pattern outside the NATS KV filter grammar answers 400 naming
// it and opens nothing; the pin accepted a space and refused a final '>' (design D4, M17). A final
// '>' is accepted and reaches the watcher: only the matching key is sent.
//
// spec: operator-surface / The message logger's KV watch streams a bucket's values and changes
func TestKVWatchRejectsInvalidFilter(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()
	const bucket = "WATCH_FILTER"
	kv, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: bucket})
	require.NoError(t, err)
	_, err = kv.Put(ctx, "a.x", []byte(`1`))
	require.NoError(t, err)
	_, err = kv.Put(ctx, "b.y", []byte(`2`))
	require.NoError(t, err)

	ml, srv := serveWatchLogger(t, client)
	owner := &messageLoggerOwner{ml: ml}

	for _, pattern := range []string{"a b", "a..b", "a.>.b", "a.b*", `a\b`} {
		t.Run(pattern, func(t *testing.T) {
			resp := openWatch(t, srv.URL, bucket, pattern)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body %q", body)
			assert.True(t, strings.Contains(string(body), pattern) || strings.Contains(string(body), strconv.Quote(pattern)),
				"the 400 does not name %q: %q", pattern, body)
			assert.NotContains(t, owner.Observe().Unresolved, "watch-streams=1")
		})
	}

	resp := openWatch(t, srv.URL, bucket, "a.>")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	r := bufio.NewReader(resp.Body)
	requireConnected(t, r, bucket)
	event, data := nextWatchEvent(t, r)
	require.Equal(t, "kv_change", event.name)
	assert.Equal(t, "a.x", data.Key)
	event, data = nextWatchEvent(t, r)
	require.Equal(t, "kv_change", event.name)
	assert.Equal(t, "initial_sync_complete", data.Operation, "b.y is outside a.> and must not be sent: %+v", data)
}

// TestKVWatchMissingBucketIs404 (owner ruling Q1(e)): watching a bucket that does not exist
// answers 404 before any stream begins, and creates no bucket.
//
// spec: operator-surface / The message logger's KV watch streams a bucket's values and changes
func TestKVWatchMissingBucketIs404(t *testing.T) {
	client := fixtureClient(t)
	_, srv := serveWatchLogger(t, client)

	resp := openWatch(t, srv.URL, "WATCH_MISSING", "")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "body %q", body)
	assert.NotEqual(t, "text/event-stream", resp.Header.Get("Content-Type"))

	_, err = client.GetKeyValueBucket(t.Context(), "WATCH_MISSING")
	assert.ErrorIs(t, err, jetstream.ErrBucketNotFound, "the watch created the bucket")
}

// TestKVWatchAfterStopIs503 (design D2, "Reads during and after Stop"): a watch requested after
// Stop answers 503 and opens no stream.
//
// spec: operator-surface / The message logger's KV watch streams a bucket's values and changes
func TestKVWatchAfterStopIs503(t *testing.T) {
	client := fixtureClient(t)
	const bucket = "WATCH_AFTER_STOP"
	_, err := client.CreateKeyValueBucket(t.Context(), jetstream.KeyValueConfig{Bucket: bucket})
	require.NoError(t, err)
	ml, srv := serveWatchLogger(t, client)
	require.NoError(t, ml.Stop(stopContext(t)))

	resp := openWatch(t, srv.URL, bucket, "")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "body %q", body)
	assert.Empty(t, (&messageLoggerOwner{ml: ml}).Observe().Unresolved)
}

// TestMessageLoggerStopEndsOpenWatchStreams (design D2, Stop step 3): with one watch open, Stop
// under a 10 s bound returns nil, the stream's body ends before the request's own 10 s deadline,
// and the logger holds no open stream. Under a Stop that leaves streams open, either Stop returns
// its context's error or the body read fails at the request's deadline.
//
// spec: service-lifecycle / The message logger's Start bounds Start and Stop is the only stop
func TestMessageLoggerStopEndsOpenWatchStreams(t *testing.T) {
	client := fixtureClient(t)
	const bucket = "WATCH_STOP"
	_, err := client.CreateKeyValueBucket(t.Context(), jetstream.KeyValueConfig{Bucket: bucket})
	require.NoError(t, err)
	ml, srv := serveWatchLogger(t, client)
	owner := &messageLoggerOwner{ml: ml}

	resp := openWatch(t, srv.URL, bucket, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	r := bufio.NewReader(resp.Body)
	requireConnected(t, r, bucket)
	event, data := nextWatchEvent(t, r)
	require.Equal(t, "initial_sync_complete", data.Operation, "event %+v", event)
	require.Contains(t, owner.Observe().Unresolved, "watch-streams=1")

	require.NoError(t, ml.Stop(stopContext(t)))
	_, err = io.ReadAll(r)
	require.NoError(t, err, "the watch stream was still open after Stop returned")
	assert.Empty(t, owner.Observe().Unresolved)
}

// blockingResponseWriter is a client that does not read: an http.ResponseWriter and
// http.Flusher whose Write blocks until the test releases it, then records what is written. It
// sits outside the logger, so the logger carries no hook for it (design D4, M26).
type blockingResponseWriter struct {
	header      http.Header
	release     chan struct{}
	releaseOnce sync.Once

	mu   sync.Mutex
	body bytes.Buffer
}

func newBlockingResponseWriter(t *testing.T) *blockingResponseWriter {
	w := &blockingResponseWriter{header: http.Header{}, release: make(chan struct{})}
	t.Cleanup(w.releaseWrites)
	return w
}

func (w *blockingResponseWriter) Header() http.Header { return w.header }
func (w *blockingResponseWriter) WriteHeader(int)     {}
func (w *blockingResponseWriter) Flush()              {}

func (w *blockingResponseWriter) Write(p []byte) (int, error) {
	<-w.release
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(p)
}

func (w *blockingResponseWriter) releaseWrites() { w.releaseOnce.Do(func() { close(w.release) }) }

func (w *blockingResponseWriter) written() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.body.Bytes())
}

// TestKVWatchFallingBehindEndsStream (design D4, M26): a watch whose client falls behind ends,
// rather than dropping events and then claiming the initial values are complete. 150 keys are put
// before the watch opens; the stream is held at its first write, so its 100-event buffer fills
// and a later event does not fit. After the drop's Warn, the writer is released: the events
// already buffered are sent, then one error event, and the response ends. Under the pin's
// drop-and-continue the stream sends initial_sync_complete, which fails the assertion, not the
// bound.
//
// spec: operator-surface / A KV watch that falls behind ends
func TestKVWatchFallingBehindEndsStream(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()
	const bucket = "WATCH_BEHIND"
	const keys = 150
	kv, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: bucket})
	require.NoError(t, err)
	for i := range keys {
		_, err := kv.Put(ctx, fmt.Sprintf("k%03d", i), []byte(fmt.Sprintf(`{"i":%d}`, i)))
		require.NoError(t, err)
	}

	handler := newHoldingHandler(t, "")
	_, mux := startWatchLogger(t, client, slog.New(handler))
	dropped := handler.arrived(watchDropWarn)

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req := httptest.NewRequestWithContext(reqCtx, http.MethodGet, watchURL("", bucket, ""), nil)
	w := newBlockingResponseWriter(t)
	served := make(chan struct{})
	go func() {
		defer close(served)
		mux.ServeHTTP(w, req)
	}()

	waitForSignal(t, dropped, "the watch's buffer-full Warn")
	w.releaseWrites()
	timer := time.NewTimer(watchBound)
	defer timer.Stop()
	select {
	case <-served:
	case <-timer.C:
		cancel()
		t.Fatalf("the watch stream did not end within %s after it fell behind; body %q", watchBound, w.written())
	}

	body := w.written()
	events := readAllSSEEvents(t, body)
	require.NotEmpty(t, events)
	assert.Equal(t, "connected", events[0].name)
	last := events[len(events)-1]
	require.Equal(t, "error", last.name, "last event %+v", last)
	assert.JSONEq(t, `{"error":"watch fell behind: events were dropped"}`, last.data)
	assert.NotContains(t, string(body), "initial_sync_complete")

	changes := 0
	for _, event := range events {
		if event.name == "kv_change" {
			changes++
		}
	}
	assert.Less(t, changes, keys, "a dropped event's gap must end the stream")
	// A drop means the 100-event buffer was full; those buffered events are still sent, and at most
	// one more was in the stream's hands (design D4).
	assert.GreaterOrEqual(t, changes, 100, "events buffered before the drop were not sent")
	assert.LessOrEqual(t, changes, 101)
}
