package service

// KV watch route for the message logger.
//
// GET /message-logger/kv/{bucket}/watch streams a NATS KV bucket's values and changes as
// Server-Sent Events (SSE). The optional pattern query parameter is a NATS KV filter; empty or
// "*" watches every key.
//
// Response format (SSE):
//
//	event: connected
//	id: 1
//	data: {"bucket":"ENTITY_STATES","pattern":"*","message":"Watching for changes"}
//
//	retry: 5000
//
//	event: kv_change
//	id: 2
//	data: {"bucket":"ENTITY_STATES","key":"acme.iot...","operation":"put","value":{...},"revision":5,"timestamp":"..."}
//
// The stream first sends every matching key's current value, then a kv_change event whose
// operation is "initial_sync_complete", then each later change. Event ids count the events of
// one stream; the logger does not read a Last-Event-ID header, so a client that reconnects gets
// a new initial replay. The stream ends when the client leaves, when the logger stops, or when
// the client falls behind (see handleKVWatch).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/natsclient"
)

// kvWatchBufferSize is how many events a watch stream holds between its KV watcher and a client
// that has not read them yet. The first event that does not fit ends the stream.
const kvWatchBufferSize = 100

// kvWatchFellBehind is the error event's text when a stream ends because its client fell behind.
const kvWatchFellBehind = "watch fell behind: events were dropped"

// KVWatchEvent is the data of a kv_change event on the KV watch stream.
type KVWatchEvent struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
	// Operation is the operation NATS records for the entry: "put", "delete" or "purge"; or
	// "initial_sync_complete" once every key's current value has been sent.
	Operation string          `json:"operation"`
	Value     json.RawMessage `json:"value,omitempty"`
	Revision  uint64          `json:"revision"`
	Timestamp time.Time       `json:"timestamp"`
}

// KVWatchConnectedEvent is the data of the connected event that opens the KV watch stream.
type KVWatchConnectedEvent struct {
	Bucket  string `json:"bucket"`
	Pattern string `json:"pattern"`
	Message string `json:"message"`
}

// isValidWatchPattern reports whether pattern is a watch filter the route accepts: empty or "*"
// watches every key; any other pattern must be a NATS KV wildcard filter.
func isValidWatchPattern(pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	return natsclient.ValidateKVWildcardFilter(pattern) == nil
}

// detectKVOperation names the operation NATS records for an entry.
func detectKVOperation(op jetstream.KeyValueOp) string {
	switch op {
	case jetstream.KeyValuePut:
		return "put"
	case jetstream.KeyValueDelete:
		return "delete"
	case jetstream.KeyValuePurge:
		return "purge"
	default:
		return "unknown"
	}
}

// handleKVWatch handles GET /message-logger/kv/{bucket}/watch.
//
// It answers 503 once the logger's Stop has begun, 400 for a malformed bucket name or pattern,
// 404 when the bucket does not exist and 500 when the lookup fails otherwise; none of these
// opens a stream or creates a bucket. Otherwise it streams the bucket until the client leaves,
// the logger stops, or the client falls behind: when an event does not fit in the stream's
// buffer, the stream stops its KV watcher, sends the events already buffered, then one error
// event, and ends, so a client never reads initial_sync_complete or a change after a gap. The
// KV watcher and its reader end before the handler returns.
func (ml *MessageLogger) handleKVWatch(w http.ResponseWriter, r *http.Request) {
	stream, ok := ml.openWatchStream()
	if !ok {
		http.Error(w, "message logger is stopped", http.StatusServiceUnavailable)
		return
	}
	defer ml.closeWatchStream(stream)

	bucket := r.PathValue("bucket")
	if !isValidBucketName(bucket) {
		http.Error(w, fmt.Sprintf("invalid bucket name %q: must not be empty, '.', '..', or contain '/', '\\', '*' or '>'", bucket),
			http.StatusBadRequest)
		return
	}
	pattern := r.URL.Query().Get("pattern")
	if !isValidWatchPattern(pattern) {
		http.Error(w, fmt.Sprintf("invalid watch pattern %q: must be empty, '*', or a NATS KV key filter", pattern),
			http.StatusBadRequest)
		return
	}
	if pattern == "" {
		pattern = "*"
	}

	kv, err := ml.natsClient.GetKeyValueBucket(r.Context(), bucket)
	if err != nil {
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			http.Error(w, fmt.Sprintf("bucket %q not found", bucket), http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("look up bucket %q: %v", bucket, err), http.StatusInternalServerError)
		return
	}

	// watchCtx ends the watcher's reader when the handler returns, whatever ended the stream.
	watchCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	var watcher jetstream.KeyWatcher
	if pattern == "*" {
		watcher, err = kv.WatchAll(watchCtx)
	} else {
		watcher, err = kv.Watch(watchCtx, pattern)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("watch bucket %q: %v", bucket, err), http.StatusInternalServerError)
		return
	}

	// The reader owns the watcher and stops it before it closes readerDone. Deferred after
	// closeWatchStream, this join runs first: Stop learns the stream ended only once the watcher
	// has stopped and the reader has returned.
	events := make(chan *KVWatchEvent, kvWatchBufferSize)
	fellBehind := make(chan struct{})
	readerDone := make(chan struct{})
	go ml.readKVWatch(watchCtx, watcher, bucket, events, fellBehind, readerDone)
	defer func() {
		cancel()
		<-readerDone
	}()

	ml.streamKVWatch(r.Context(), w, stream, bucket, pattern, events, fellBehind)
}

// readKVWatch forwards the watcher's entries to events until ctx ends, the watcher closes, or
// an entry does not fit in events; in that last case it closes fellBehind. It never waits on
// events. On return it stops the watcher, closes events, then closes done.
func (ml *MessageLogger) readKVWatch(
	ctx context.Context,
	watcher jetstream.KeyWatcher,
	bucket string,
	events chan<- *KVWatchEvent,
	fellBehind, done chan<- struct{},
) {
	defer close(done)
	defer close(events)
	defer func() {
		if err := watcher.Stop(); err != nil {
			ml.logger.Warn("Failed to stop KV watcher", "error", err, "bucket", bucket)
		}
	}()

	for {
		var entry jetstream.KeyValueEntry
		var ok bool
		select {
		case <-ctx.Done():
			return
		case entry, ok = <-watcher.Updates():
			if !ok {
				return
			}
		}

		event := kvWatchEvent(bucket, entry)
		select {
		case events <- event:
		default:
			// The client fell behind. Dropping this event and going on would leave a gap the
			// client cannot see, so the stream ends after the events already buffered.
			ml.logger.Warn("KV watch buffer full, dropping event",
				"bucket", bucket, "key", event.Key, "operation", event.Operation, "revision", event.Revision)
			close(fellBehind)
			return
		}
	}
}

// kvWatchEvent builds the kv_change event for a watcher entry; a nil entry is the watcher's
// signal that every key's current value has been delivered.
func kvWatchEvent(bucket string, entry jetstream.KeyValueEntry) *KVWatchEvent {
	if entry == nil {
		return &KVWatchEvent{Bucket: bucket, Operation: "initial_sync_complete"}
	}
	event := &KVWatchEvent{
		Bucket:    bucket,
		Key:       entry.Key(),
		Operation: detectKVOperation(entry.Operation()),
		Revision:  entry.Revision(),
		Timestamp: entry.Created(),
	}
	if entry.Operation() == jetstream.KeyValuePut {
		if json.Valid(entry.Value()) {
			event.Value = entry.Value()
		} else {
			// A value that is not JSON travels as a JSON string.
			event.Value, _ = json.Marshal(string(entry.Value()))
		}
	}
	return event
}

// streamKVWatch writes the stream's events to the client until the client leaves (ctx), the
// logger stops (stream.stop), or events closes; it then returns.
func (ml *MessageLogger) streamKVWatch(
	ctx context.Context,
	w http.ResponseWriter,
	stream *watchStream,
	bucket, pattern string,
	events <-chan *KVWatchEvent,
	fellBehind <-chan struct{},
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
	flushSSE(w)

	eventID := uint64(1)
	ml.writeKVWatchEvent(w, "connected", eventID, KVWatchConnectedEvent{
		Bucket:  bucket,
		Pattern: pattern,
		Message: "Watching for changes",
	})
	fmt.Fprint(w, "retry: 5000\n\n")
	flushSSE(w)

	for {
		select {
		case <-ctx.Done():
			ml.logger.Info("Client disconnected from KV watch stream", "bucket", bucket, "pattern", pattern)
			return
		case <-stream.stop:
			return
		case event, ok := <-events:
			if !ok {
				message := "watcher closed unexpectedly"
				select {
				case <-fellBehind:
					message = kvWatchFellBehind
				default:
				}
				ml.writeKVWatchEvent(w, "error", 0, map[string]string{"error": message})
				return
			}
			eventID++
			ml.writeKVWatchEvent(w, "kv_change", eventID, event)
		}
	}
}

// writeKVWatchEvent writes one SSE event, with an id line when id is not 0, and flushes it.
func (ml *MessageLogger) writeKVWatchEvent(w http.ResponseWriter, event string, id uint64, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		ml.logger.Error("Failed to marshal SSE event", "event", event, "error", err)
		return
	}
	if id != 0 {
		fmt.Fprintf(w, "event: %s\nid: %d\ndata: %s\n\n", event, id, jsonData)
	} else {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
	}
	flushSSE(w)
}

// flushSSE sends what has been written to the client when w can flush.
func flushSSE(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
