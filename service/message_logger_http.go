package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/internal/flowgraph"
)

// Compile-time check that MessageLogger implements HTTPHandler
var _ HTTPHandler = (*MessageLogger)(nil)

// Route limits: /entries serves 100 entries unless asked for up to 10000, and /kv/{bucket} 100
// entries unless asked for up to 1000. A limit that is missing, not a whole number, zero or
// negative gives the default; one above the maximum gives the maximum.
const (
	entriesDefaultLimit = 100
	entriesMaxLimit     = 10000
	kvQueryDefaultLimit = 100
	kvQueryMaxLimit     = 1000
)

// RegisterHTTPHandlers mounts the logger's routes under prefix. Every route answers GET only; the
// mux answers any other method with 405.
func (ml *MessageLogger) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	mux.HandleFunc("GET "+prefix+"entries", ml.handleGetEntries)
	mux.HandleFunc("GET "+prefix+"stats", ml.handleGetStats)
	mux.HandleFunc("GET "+prefix+"subjects", ml.handleGetSubjects)
	mux.HandleFunc("GET "+prefix+"trace/{traceID}", ml.handleGetTrace)
	mux.HandleFunc("GET "+prefix+"kv/{bucket}", ml.handleKVQuery)
	mux.HandleFunc("GET "+prefix+"kv/{bucket}/watch", ml.handleKVWatch)

	ml.logger.Info("MessageLogger HTTP handlers registered", "prefix", prefix)
}

// OpenAPISpec returns the OpenAPI specification for MessageLogger endpoints
func (ml *MessageLogger) OpenAPISpec() *OpenAPISpec {
	return messageLoggerOpenAPISpec()
}

// messageLoggerOpenAPISpec returns the OpenAPI specification for MessageLogger endpoints.
// RegisterAll registers it; importing the package registers nothing.
func messageLoggerOpenAPISpec() *OpenAPISpec {
	return &OpenAPISpec{
		Tags: []TagSpec{
			{
				Name:        "MessageLogger",
				Description: "Message observation and debugging endpoints",
			},
		},
		Paths: map[string]PathSpec{
			"/entries": {
				GET: &OperationSpec{
					Summary:     "Get recent message entries",
					Description: "Returns the most recent logged messages from the circular buffer, newest first",
					Tags:        []string{"MessageLogger"},
					Parameters: []ParameterSpec{
						{
							Name:        "limit",
							In:          "query",
							Description: "Maximum number of entries to return (default: 100, max: 10000)",
							Required:    false,
							Schema:      Schema{Type: "integer"},
						},
						{
							Name: "subject",
							In:   "query",
							Description: "NATS subject pattern ('*' one token, '>' the remaining tokens); the filter " +
								"applies before the limit, and a pattern NATS cannot parse matches nothing",
							Required: false,
							Schema:   Schema{Type: "string"},
						},
					},
					Responses: map[string]ResponseSpec{
						"200": {
							Description: "List of message entries",
							ContentType: "application/json",
							SchemaRef:   "#/components/schemas/MessageLogEntry",
							IsArray:     true,
						},
					},
				},
			},
			"/stats": {
				GET: &OperationSpec{
					Summary:     "Get message statistics",
					Description: "Returns statistics about processed messages",
					Tags:        []string{"MessageLogger"},
					Responses: map[string]ResponseSpec{
						"200": {
							Description: "Message statistics",
							ContentType: "application/json",
						},
					},
				},
			},
			"/subjects": {
				GET: &OperationSpec{
					Summary:     "Get monitored subjects",
					Description: "Returns the NATS subjects the logger is subscribed to",
					Tags:        []string{"MessageLogger"},
					Responses: map[string]ResponseSpec{
						"200": {
							Description: "List of monitored subjects",
							ContentType: "application/json",
						},
					},
				},
			},
			"/trace/{traceID}": {
				GET: &OperationSpec{
					Summary:     "Get entries by trace ID",
					Description: "Returns the entries still held for a W3C trace ID, oldest first",
					Tags:        []string{"MessageLogger"},
					Parameters: []ParameterSpec{
						{
							Name:        "traceID",
							In:          "path",
							Description: "W3C trace ID (32 hex characters)",
							Required:    true,
							Schema:      Schema{Type: "string"},
						},
					},
					Responses: map[string]ResponseSpec{
						"200": {
							Description: "Trace entries found",
							ContentType: "application/json",
						},
						"400": {
							Description: "Invalid trace ID format",
						},
					},
				},
			},
			"/kv/{bucket}": {
				GET: &OperationSpec{
					Summary:     "Query KV bucket",
					Description: "Reads the entries of an existing NATS KV bucket, keys in sorted order; never creates the bucket",
					Tags:        []string{"MessageLogger"},
					Parameters: []ParameterSpec{
						{
							Name:        "bucket",
							In:          "path",
							Description: "KV bucket name",
							Required:    true,
							Schema:      Schema{Type: "string"},
						},
						{
							Name:        "pattern",
							In:          "query",
							Description: "Key glob: '*' alone, or one '*' as prefix, suffix, both ends or in the middle (e.g., 'entity*'). Default: '*' (all keys)",
							Required:    false,
							Schema:      Schema{Type: "string"},
						},
						{
							Name:        "limit",
							In:          "query",
							Description: "Maximum number of entries to return (default: 100, max: 1000)",
							Required:    false,
							Schema:      Schema{Type: "integer"},
						},
						{
							Name:        "status",
							In:          "query",
							Description: "Opt-in filter: keep only records whose top-level JSON 'status' equals this (e.g. 'failed' over EMBEDDING_INDEX for per-entity failure forensics). Empty (default) returns all records.",
							Required:    false,
							Schema:      Schema{Type: "string"},
						},
					},
					Responses: map[string]ResponseSpec{
						"200": {
							Description: "KV bucket entries",
							ContentType: "application/json",
						},
						"400": {
							Description: "Invalid bucket name",
						},
						"404": {
							Description: "Bucket not found",
						},
						"500": {
							Description: "Bucket lookup, key listing or key read failed",
						},
					},
				},
			},
			"/kv/{bucket}/watch": {
				GET: &OperationSpec{
					Summary: "Watch KV bucket changes",
					Description: "Streams a bucket's current values, then its changes, as Server-Sent Events. A client " +
						"that falls behind gets an 'error' event and the stream ends.",
					Tags: []string{"MessageLogger"},
					Parameters: []ParameterSpec{
						{
							Name:        "bucket",
							In:          "path",
							Description: "KV bucket name (e.g., ENTITY_STATES, INCOMING_INDEX)",
							Required:    true,
							Schema:      Schema{Type: "string"},
						},
						{
							Name:        "pattern",
							In:          "query",
							Description: "NATS KV key filter (e.g., 'entity.*' or 'entity.>'). Default: '*' (all keys)",
							Required:    false,
							Schema:      Schema{Type: "string"},
						},
					},
					Responses: map[string]ResponseSpec{
						"200": {
							Description: "SSE stream of KV changes. Events: 'connected' (initial), 'kv_change' (values and updates), 'error' (failures)",
							ContentType: "text/event-stream",
						},
						"400": {
							Description: "Invalid bucket name or pattern",
						},
						"404": {
							Description: "Bucket not found",
						},
						"500": {
							Description: "Bucket lookup failed",
						},
						"503": {
							Description: "The logger is stopping or stopped",
						},
					},
				},
			},
		},
		// MessageLogEntry is the only typed response - stats returns map[string]any
		ResponseTypes: []reflect.Type{
			reflect.TypeOf(MessageLogEntry{}),
		},
	}
}

// parseLimit reads a limit query value: missing, not a whole number, zero or negative gives def;
// above maxLimit gives maxLimit.
func parseLimit(value string, def, maxLimit int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return def
	}
	return min(parsed, maxLimit)
}

// writeJSON answers 200 with value as JSON. A value that cannot be encoded answers 500.
func (ml *MessageLogger) writeJSON(w http.ResponseWriter, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		ml.logger.Error("Failed to encode response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(data, '\n'))
}

// handleGetEntries serves the newest entries whose subject the `subject` filter matches, at most
// `limit` of them: the filter picks before the limit counts.
func (ml *MessageLogger) handleGetEntries(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := parseLimit(query.Get("limit"), entriesDefaultLimit, entriesMaxLimit)
	filter := query.Get("subject")

	entries := ml.GetLogEntries(0)
	if filter != "" {
		entries = slices.DeleteFunc(entries, func(entry MessageLogEntry) bool {
			return !flowgraph.SubjectCovers(filter, entry.Subject)
		})
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	ml.writeJSON(w, entries)
}

// handleGetTrace returns all message entries for a specific trace ID
func (ml *MessageLogger) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceID")
	if len(traceID) != 32 || !isHexString(traceID) {
		http.Error(w, "Invalid trace ID format: must be 32 hex characters", http.StatusBadRequest)
		return
	}

	entries := ml.GetEntriesByTrace(traceID)
	ml.writeJSON(w, map[string]any{
		"trace_id": traceID,
		"count":    len(entries),
		"entries":  entries,
	})
}

// isHexString checks if a string contains only hex characters
func isHexString(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// handleGetStats returns message statistics
func (ml *MessageLogger) handleGetStats(w http.ResponseWriter, _ *http.Request) {
	ml.writeJSON(w, ml.GetStatistics())
}

// handleGetSubjects returns the subjects the logger is subscribed to; overlap handling is in the
// statistics.
func (ml *MessageLogger) handleGetSubjects(w http.ResponseWriter, _ *http.Request) {
	subjects, _ := ml.subjectInspection()
	ml.writeJSON(w, subjects)
}

// isValidBucketName is the one bucket-name check both KV routes apply before any lookup: a name
// that is empty, a path step (`.`, `..`), holds a path separator (`/`, `\`) or a NATS wildcard
// (`*`, `>`) is refused.
func isValidBucketName(bucket string) bool {
	return bucket != "" && bucket != "." && bucket != ".." && !strings.ContainsAny(bucket, `/\*>`)
}

// handleKVQuery reads an existing KV bucket's entries (D4). It never creates the bucket.
func (ml *MessageLogger) handleKVQuery(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	if !isValidBucketName(bucket) {
		http.Error(w, "Invalid bucket name", http.StatusBadRequest)
		return
	}

	query := r.URL.Query()
	pattern := query.Get("pattern")
	if pattern == "" {
		pattern = "*"
	}
	limit := parseLimit(query.Get("limit"), kvQueryDefaultLimit, kvQueryMaxLimit)
	// Opt-in status filter (#613): when set, keep only records whose top-level JSON "status"
	// field equals it, e.g. ?status=failed over EMBEDDING_INDEX enumerates the durable failed
	// embeddings. Empty (the default) keeps every record.
	statusFilter := query.Get("status")

	entries, err := ml.queryKVBucket(r.Context(), bucket, pattern, limit, statusFilter)
	if err != nil {
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			http.Error(w, fmt.Sprintf("Bucket not found: %s", bucket), http.StatusNotFound)
			return
		}
		ml.logger.Error("Failed to query KV bucket", "bucket", bucket, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ml.writeJSON(w, map[string]any{
		"bucket":  bucket,
		"pattern": pattern,
		"count":   len(entries),
		"entries": entries,
	})
}

// queryKVBucket looks bucket up, never creating it, and returns up to limit of its entries whose
// key matches pattern and whose value passes statusFilter, keys in sorted order. Creating on read
// would let a reader win the cold-boot race against the bucket's owner and impose its own
// settings, and would turn a typo'd name into a permanent bucket. An empty bucket gives no
// entries. A key deleted or purged between the listing and its read is left out; any other read
// failure is returned, naming the key.
func (ml *MessageLogger) queryKVBucket(
	ctx context.Context,
	bucket, pattern string,
	limit int,
	statusFilter string,
) ([]map[string]any, error) {
	kv, err := ml.natsClient.GetKeyValueBucket(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("get KV bucket %s: %w", bucket, err)
	}

	entries := make([]map[string]any, 0)
	keys, err := kv.Keys(ctx, jetstream.IgnoreDeletes())
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list keys of %s: %w", bucket, err)
	}
	// nats.go promises the keys deduplicated, not ordered, so sort before the limit.
	slices.Sort(keys)

	for _, key := range keys {
		if len(entries) >= limit {
			break
		}
		if !matchesPattern(key, pattern) {
			continue
		}

		entry, err := kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read key %q of %s: %w", key, bucket, err)
		}

		var value any
		if err := json.Unmarshal(entry.Value(), &value); err != nil {
			value = string(entry.Value())
		}
		if !recordMatchesStatus(value, statusFilter) {
			continue
		}

		entries = append(entries, map[string]any{
			"key":      key,
			"value":    value,
			"revision": entry.Revision(),
			"created":  entry.Created(),
		})
	}
	return entries, nil
}

// recordMatchesStatus reports whether a decoded KV value passes the opt-in status
// filter (#613). An EMPTY filter matches everything (the filter is off, so the endpoint
// is unchanged). A non-empty filter matches only a JSON object whose top-level "status"
// string equals it; a non-object value (raw string, array, number) never matches, so a
// bucket without status-shaped records returns nothing under a filter rather than leaking
// unfiltered rows.
func recordMatchesStatus(value any, statusFilter string) bool {
	if statusFilter == "" {
		return true
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	s, _ := obj["status"].(string)
	return s == statusFilter
}

// matchesPattern reports whether key matches the KV query's key glob: `*` or empty matches every
// key; otherwise one `*` as a prefix, a suffix, at both ends, or in the middle; no `*` is an exact
// match. It is the query's grammar only; the watch's `pattern` is a NATS filter.
func matchesPattern(str, pattern string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}

	if strings.Contains(pattern, "*") {
		switch {
		case strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*"):
			return strings.Contains(str, strings.Trim(pattern, "*"))
		case strings.HasPrefix(pattern, "*"):
			return strings.HasSuffix(str, strings.TrimPrefix(pattern, "*"))
		case strings.HasSuffix(pattern, "*"):
			return strings.HasPrefix(str, strings.TrimSuffix(pattern, "*"))
		}
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(str, parts[0]) && strings.HasSuffix(str, parts[1])
		}
	}

	return str == pattern
}
