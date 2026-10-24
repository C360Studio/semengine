package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/natsclient"
)

// The message logger's HTTP routes (design D4, Q1; D7 unit U-d). Every test builds the logger the
// production way (U-a's helper) over a client that never connects, stores entries through
// handleMessage, the subscription callback's body, and reads them through the routes
// RegisterHTTPHandlers mounts on a ServeMux, as the service manager mounts them. No test needs NATS:
// the KV routes are exercised only up to their refusals; their reads are U-e's and U-f's.

// routeTestTraceID is a W3C trace ID written by hand, the oracle for the trace route.
const routeTestTraceID = "0123456789abcdef0123456789abcdef"

// routeLogger is a message logger and the mux its routes are mounted on.
type routeLogger struct {
	ml  *MessageLogger
	mux *http.ServeMux
}

// newRouteLogger builds a logger from config over a never-connected client, with an empty
// component registry, and mounts its routes under /message-logger.
func newRouteLogger(t *testing.T, config string) routeLogger {
	t.Helper()
	ml := newTestMessageLogger(t, newUnconnectedClient(t), config, slog.New(slog.DiscardHandler))
	mux := http.NewServeMux()
	ml.RegisterHTTPHandlers("/message-logger", mux)
	return routeLogger{ml: ml, mux: mux}
}

// store hands data on subject to the logger as its subscription callback would.
func (rl routeLogger) store(ctx context.Context, subject string, data []byte) {
	rl.ml.handleMessage(ctx, subject, data)
}

// serve sends one request through the mux and returns the recorded response. The request carries a
// 10 s deadline, so a handler that wrongly opens a stream ends the test instead of hanging it.
func (rl routeLogger) serve(t *testing.T, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	rec := httptest.NewRecorder()
	rl.mux.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, method, target, nil))
	return rec
}

// entryView is the part of a served MessageLogEntry these tests read, decoded from the wire.
type entryView struct {
	Subject string          `json:"subject"`
	TraceID string          `json:"trace_id"`
	RawData json.RawMessage `json:"raw_data"`
}

// getEntries answers GET target, requires 200 and a JSON body, and decodes the entries.
func (rl routeLogger) getEntries(t *testing.T, target string) []entryView {
	t.Helper()
	rec := rl.serve(t, http.MethodGet, target)
	require.Equal(t, http.StatusOK, rec.Code, "GET %s: %s", target, rec.Body.String())
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var entries []entryView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &entries), "GET %s: body %q", target, rec.Body.String())
	return entries
}

// payloadNumbers decodes each entry's {"n":N} payload, in served order.
func payloadNumbers(t *testing.T, entries []entryView) []int {
	t.Helper()
	numbers := make([]int, 0, len(entries))
	for _, entry := range entries {
		var payload struct {
			N int `json:"n"`
		}
		require.NoError(t, json.Unmarshal(entry.RawData, &payload), "raw_data %s", entry.RawData)
		numbers = append(numbers, payload.N)
	}
	return numbers
}

func subjectsOf(entries []entryView) []string {
	subjects := make([]string, 0, len(entries))
	for _, entry := range entries {
		subjects = append(subjects, entry.Subject)
	}
	return subjects
}

// TestEntriesLimit: /entries serves the newest entries first, at most limit of them; a limit that
// is missing, zero, negative or not a whole number serves 100, and one above 10000 serves 10000
// (pin message_logger_http.go:251-258). The buffer holds 20000 so the clamp is visible: 10001
// entries are stored and a limit of 20000 must still serve 10000.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestEntriesLimit(t *testing.T) {
	rl := newRouteLogger(t, `{"monitor_subjects":["*"],"max_entries":20000}`)
	const stored = 10001
	for n := range stored {
		rl.store(t.Context(), "test.limit", fmt.Appendf(nil, `{"n":%d}`, n))
	}

	t.Run("NewestFirst", func(t *testing.T) {
		entries := rl.getEntries(t, "/message-logger/entries?limit=5")
		require.Equal(t, []int{10000, 9999, 9998, 9997, 9996}, payloadNumbers(t, entries))
	})
	for _, limit := range []string{"", "0", "-3", "abc", "1.5"} {
		t.Run("Default/"+limit, func(t *testing.T) {
			require.Len(t, rl.getEntries(t, "/message-logger/entries?limit="+limit), 100)
		})
	}
	t.Run("Default/absent", func(t *testing.T) {
		require.Len(t, rl.getEntries(t, "/message-logger/entries"), 100)
	})
	t.Run("AtMaximum", func(t *testing.T) {
		require.Len(t, rl.getEntries(t, "/message-logger/entries?limit=10000"), 10000)
	})
	t.Run("AboveMaximumIsClamped", func(t *testing.T) {
		require.Len(t, rl.getEntries(t, "/message-logger/entries?limit=20000"), 10000)
	})
}

// TestEntriesFilterBeforeLimit: the subject filter picks the entries before the limit counts them
// (Q1(c)). Three matching entries are stored before twenty newer ones that do not match; a filtered
// request with limit 3 serves the three. At the pin the limit took the three newest entries first,
// all non-matching, and the filter then served none.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestEntriesFilterBeforeLimit(t *testing.T) {
	rl := newRouteLogger(t, `{"monitor_subjects":["*"]}`)
	for n := range 3 {
		rl.store(t.Context(), "agent.task.created", fmt.Appendf(nil, `{"n":%d}`, n))
	}
	for n := 3; n < 23; n++ {
		rl.store(t.Context(), "noise.tick", fmt.Appendf(nil, `{"n":%d}`, n))
	}

	entries := rl.getEntries(t, "/message-logger/entries?subject=agent.%3E&limit=3")
	require.Equal(t, []int{2, 1, 0}, payloadNumbers(t, entries))

	entries = rl.getEntries(t, "/message-logger/entries?subject=noise.tick&limit=3")
	require.Equal(t, []int{22, 21, 20}, payloadNumbers(t, entries), "the limit still applies after the filter")
}

// TestEntriesSubjectFilterUsesNATSMatching: the subject filter is a NATS subject pattern, matched
// with the cover rule the logger's subscriptions use (Q1(c), D3): `*` matches exactly one token and
// `>` one or more trailing tokens, so `subject=*` serves only one-token subjects and `agent.*` only
// two-token ones (the ruling's stated cost). A filter NATS cannot parse matches nothing; it is not
// refused. The expected sets are written by hand from the NATS rule.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestEntriesSubjectFilterUsesNATSMatching(t *testing.T) {
	rl := newRouteLogger(t, `{"monitor_subjects":["*"]}`)
	all := []string{"plain", "agent.task", "agent.task.created", "user.response.x", "user.response.x.y"}
	for _, subject := range all {
		rl.store(t.Context(), subject, []byte(`{}`))
	}

	cases := []struct {
		filter string
		want   []string
	}{
		{filter: "", want: all},
		{filter: ">", want: all},
		{filter: "*", want: []string{"plain"}},
		{filter: "agent.*", want: []string{"agent.task"}},
		{filter: "agent.>", want: []string{"agent.task", "agent.task.created"}},
		{filter: "user.response.*", want: []string{"user.response.x"}},
		{filter: "*.response.*.y", want: []string{"user.response.x.y"}},
		{filter: "agent.task", want: []string{"agent.task"}},
		{filter: "agent", want: []string{}},
		// Malformed patterns, each of which the pin's text match would have matched something with.
		{filter: "agent*", want: []string{}},
		{filter: "agent..task", want: []string{}},
		{filter: "agent.>.created", want: []string{}},
		{filter: "*task*", want: []string{}},
		{filter: "agent.task created", want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			rec := rl.serve(t, http.MethodGet, "/message-logger/entries?subject="+url.QueryEscape(tc.filter))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var entries []entryView
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &entries))
			require.ElementsMatch(t, tc.want, subjectsOf(entries))
			if len(tc.want) == 0 {
				require.JSONEq(t, `[]`, rec.Body.String(), "an empty match is served as an empty list, not null")
			}
		})
	}
}

// TestEntriesServeNonJSONPayload: a message whose bytes are not JSON is stored as a JSON string of
// those bytes (Q1(a)), so /entries and /trace still serve valid JSON with every entry, and the
// string reads back as the bytes sent; a JSON payload is still served as itself. At the pin one
// such message made both routes fail for as long as it was held. The trace ID rides in the
// callback's context as natsclient.Client.Subscribe puts it there, read from the message's
// headers.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestEntriesServeNonJSONPayload(t *testing.T) {
	rl := newRouteLogger(t, `{"monitor_subjects":["*"]}`)
	traced := natsclient.ContextWithTrace(t.Context(), &natsclient.TraceContext{
		TraceID: routeTestTraceID, SpanID: "0123456789abcdef", Sampled: true,
	})
	nonJSON := []string{"plain text, not JSON", `{"cut":`, "<xml/>"}
	for _, text := range nonJSON {
		rl.store(traced, "raw.text", []byte(text))
	}
	rl.store(traced, "json.object", []byte(`{"ok":true}`))

	check := func(t *testing.T, entries []entryView) {
		t.Helper()
		var gotText []string
		for _, entry := range entries {
			if entry.Subject == "json.object" {
				require.JSONEq(t, `{"ok":true}`, string(entry.RawData))
				continue
			}
			var text string
			require.NoError(t, json.Unmarshal(entry.RawData, &text), "raw_data %s is not a JSON string", entry.RawData)
			gotText = append(gotText, text)
		}
		require.ElementsMatch(t, nonJSON, gotText)
	}

	t.Run("Entries", func(t *testing.T) {
		entries := rl.getEntries(t, "/message-logger/entries")
		require.Len(t, entries, 4)
		check(t, entries)
	})
	t.Run("Trace", func(t *testing.T) {
		rec := rl.serve(t, http.MethodGet, "/message-logger/trace/"+routeTestTraceID)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		var trace struct {
			TraceID string      `json:"trace_id"`
			Count   int         `json:"count"`
			Entries []entryView `json:"entries"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &trace), rec.Body.String())
		require.Equal(t, routeTestTraceID, trace.TraceID)
		require.Equal(t, 4, trace.Count)
		require.Equal(t, []string{"raw.text", "raw.text", "raw.text", "json.object"}, subjectsOf(trace.Entries),
			"the trace serves its entries oldest first")
		for _, entry := range trace.Entries {
			require.Equal(t, routeTestTraceID, entry.TraceID)
		}
		check(t, trace.Entries)
	})
}

// TestStatsCountsOnlyTypedEnvelopesValid: /stats counts a message valid only when it decodes as a
// typed envelope (Q1(d)); a JSON payload that is no envelope and a payload that is not JSON both
// count invalid. The logger's payload registry here is empty, so nothing decodes as an envelope.
// At the pin valid_messages counted every message and invalid_messages stayed 0. The typed case is
// U-g's (message_logger_decode_test.go).
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestStatsCountsOnlyTypedEnvelopesValid(t *testing.T) {
	rl := newRouteLogger(t, `{"monitor_subjects":["*"]}`)
	rl.store(t.Context(), "raw.json", []byte(`{"k":1}`))
	rl.store(t.Context(), "raw.text", []byte("not json"))

	rec := rl.serve(t, http.MethodGet, "/message-logger/stats")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var stats struct {
		Total   *int `json:"total_messages"`
		Sampled *int `json:"sampled_messages"`
		Valid   *int `json:"valid_messages"`
		Invalid *int `json:"invalid_messages"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats), rec.Body.String())
	require.NotNil(t, stats.Total, "total_messages missing: %s", rec.Body.String())
	require.NotNil(t, stats.Sampled, "sampled_messages missing: %s", rec.Body.String())
	require.NotNil(t, stats.Valid, "valid_messages missing: %s", rec.Body.String())
	require.NotNil(t, stats.Invalid, "invalid_messages missing: %s", rec.Body.String())
	assert.Equal(t, 2, *stats.Total)
	assert.Equal(t, 2, *stats.Sampled)
	assert.Equal(t, 0, *stats.Valid)
	assert.Equal(t, 2, *stats.Invalid)
}

// TestMessageLoggerCountsAndLogsADroppedMessage is design D3: a message the subscription callback
// is handed with a context that has ended is not stored and not counted in total_messages; /stats
// reports it in dropped_messages, and one Warn record names its subject and the context's error.
// natsclient ends a handler's context once its Close has begun (T2, T3); which messages arrive that
// late is not promised, so the test hands the callback's body an ended context directly.
//
// spec: service-lifecycle / The message logger's Start bounds Start and Stop is the only stop
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestMessageLoggerCountsAndLogsADroppedMessage(t *testing.T) {
	records := &recordingHandler{} // pprof_test.go's: it keeps each record's level and attributes
	ml := newTestMessageLogger(t, newUnconnectedClient(t), `{"monitor_subjects":["*"]}`, slog.New(records))
	rl := routeLogger{ml: ml, mux: http.NewServeMux()}
	ml.RegisterHTTPHandlers("/message-logger", rl.mux)

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	rl.store(ended, "logger.dropped", []byte(`{"n":1}`))

	rec := rl.serve(t, http.MethodGet, "/message-logger/stats")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var stats struct {
		Total   *int `json:"total_messages"`
		Dropped *int `json:"dropped_messages"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats), rec.Body.String())
	require.NotNil(t, stats.Dropped, "dropped_messages missing: %s", rec.Body.String())
	require.NotNil(t, stats.Total, "total_messages missing: %s", rec.Body.String())
	assert.Equal(t, 1, *stats.Dropped)
	assert.Equal(t, 0, *stats.Total)
	assert.Empty(t, ml.GetLogEntries(0), "a message whose context has ended is not stored")

	records.mu.Lock()
	var dropped []slog.Record
	for _, r := range records.records {
		if r.Message == "Message dropped: its handler context has ended" {
			dropped = append(dropped, r)
		}
	}
	records.mu.Unlock()
	require.Len(t, dropped, 1, "one Warn record for the dropped message")
	assert.Equal(t, slog.LevelWarn, dropped[0].Level)
	attrs := map[string]slog.Value{}
	dropped[0].Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	require.Contains(t, attrs, "subject")
	assert.Equal(t, "logger.dropped", attrs["subject"].String())
	require.Contains(t, attrs, "error")
	err, ok := attrs["error"].Any().(error)
	require.True(t, ok, "the error attribute is an error, got %T", attrs["error"].Any())
	assert.True(t, errors.Is(err, context.Canceled), "the error attribute is the context's error, got %v", err)
}

// TestTraceRouteRefusesMalformedID: /trace/{traceID} answers 400 unless the ID is 32 hex digits,
// either case, as at the pin (message_logger_http.go:287-299); a well-formed ID nobody used answers
// 200 with no entries.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestTraceRouteRefusesMalformedID(t *testing.T) {
	rl := newRouteLogger(t, `{"monitor_subjects":["*"]}`)
	for _, id := range []string{
		"abc",
		strings.Repeat("a", 31),
		strings.Repeat("a", 33),
		strings.Repeat("a", 31) + "g",
		strings.Repeat("a", 31) + "%20",
	} {
		t.Run(id, func(t *testing.T) {
			rec := rl.serve(t, http.MethodGet, "/message-logger/trace/"+id)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
	for _, id := range []string{routeTestTraceID, strings.ToUpper(routeTestTraceID)} {
		t.Run(id, func(t *testing.T) {
			rec := rl.serve(t, http.MethodGet, "/message-logger/trace/"+id)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var trace struct {
				TraceID string      `json:"trace_id"`
				Count   int         `json:"count"`
				Entries []entryView `json:"entries"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &trace), rec.Body.String())
			require.Equal(t, id, trace.TraceID)
			require.Zero(t, trace.Count)
			require.Empty(t, trace.Entries)
		})
	}
}

// TestMessageLoggerRoutesRefuseOtherMethods: every route answers GET and HEAD (ServeMux serves HEAD
// on a "GET " pattern); any other method gets 405 (D4).
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestMessageLoggerRoutesRefuseOtherMethods(t *testing.T) {
	rl := newRouteLogger(t, `{"monitor_subjects":["*"]}`)
	paths := []string{
		"/message-logger/entries",
		"/message-logger/stats",
		"/message-logger/subjects",
		"/message-logger/trace/" + routeTestTraceID,
		"/message-logger/kv/KV_TEST",
		"/message-logger/kv/KV_TEST/watch",
	}
	for _, path := range paths {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			t.Run(method+" "+path, func(t *testing.T) {
				rec := rl.serve(t, method, path)
				require.Equal(t, http.StatusMethodNotAllowed, rec.Code, rec.Body.String())
			})
		}
	}
	for _, path := range []string{"/message-logger/entries", "/message-logger/stats"} {
		t.Run(http.MethodHead+" "+path, func(t *testing.T) {
			rec := rl.serve(t, http.MethodHead, path)
			require.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// TestKVRoutesRejectMalformedBucket: both KV routes refuse a malformed bucket name before any bucket
// lookup (D4, M16): `.`, `..`, `\`, `*` and `>` answer 400 from the one validator. An empty name or
// one holding `/` never reaches a handler, because no route matches a path with an empty or split
// segment, so the mux answers 404 (measured with go1.26.4's ServeMux: `/kv/%2F` and `/kv/` both 404);
// either way no lookup runs. A lookup on this never-connected client fails with 500, which is what a
// well-formed name gets: the control that shows the test is not passing on a route that refuses
// everything.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestKVRoutesRejectMalformedBucket(t *testing.T) {
	rl := newRouteLogger(t, `{"monitor_subjects":["*"]}`)
	routes := map[string]func(bucket string) string{
		"query": func(bucket string) string { return "/message-logger/kv/" + bucket },
		"watch": func(bucket string) string { return "/message-logger/kv/" + bucket + "/watch" },
	}
	for name, route := range routes {
		for _, bucket := range []string{"%2E", "%2E%2E", "%5C", "*", "%3E", "A%5CB", "A*", "A%3EB"} {
			t.Run(name+"/"+bucket, func(t *testing.T) {
				rec := rl.serve(t, http.MethodGet, route(bucket))
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			})
		}
		for _, bucket := range []string{"", "%2F", "A%2FB"} {
			t.Run(name+"/unrouted/"+bucket, func(t *testing.T) {
				rec := rl.serve(t, http.MethodGet, route(bucket))
				// An empty name makes the path unclean (kv//watch), which ServeMux redirects (307)
				// before matching any pattern; no handler runs, so no lookup either.
				require.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound, http.StatusTemporaryRedirect}, rec.Code,
					"a bucket name holding / must be refused before a lookup: %s", rec.Body.String())
			})
		}
		t.Run(name+"/wellFormedIsLookedUp", func(t *testing.T) {
			rec := rl.serve(t, http.MethodGet, route("KV_TEST"))
			require.Equal(t, http.StatusInternalServerError, rec.Code,
				"a well-formed name is looked up, and the lookup fails on a never-connected client: %s", rec.Body.String())
		})
	}
}

// TestRecordMatchesStatus pins the KV query's opt-in status filter, carried from the pin
// (message_logger_http_kv_query_test.go:126-147): an empty filter matches every value, a non-empty
// one only a JSON object whose top-level "status" string equals it, and a non-object never.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestRecordMatchesStatus(t *testing.T) {
	failed := []byte(`{"entity_id":"acme.ops.src.embed.record.e1","status":"failed","reason":"connection_refused","error_msg":"dial: connection refused"}`)
	generated := []byte(`{"entity_id":"acme.ops.src.embed.record.e2","status":"generated"}`)
	numeric := []byte(`{"status":3}`)
	raw := []byte("raw string value")

	assert.True(t, recordMatchesStatus(failed, ""), "an empty filter must match every record")
	assert.True(t, recordMatchesStatus(generated, ""), "an empty filter must match every record")
	assert.True(t, recordMatchesStatus(raw, ""), "an empty filter must match a non-JSON value too")
	assert.True(t, recordMatchesStatus(failed, "failed"), "a failed record must pass status=failed")
	assert.False(t, recordMatchesStatus(generated, "failed"), "a generated record must not pass status=failed")
	assert.False(t, recordMatchesStatus(numeric, "3"), "a non-string status must not match")
	assert.False(t, recordMatchesStatus(raw, "failed"), "a non-JSON value must never match a status filter")
	assert.False(t, recordMatchesStatus([]byte(`"failed"`), "failed"), "a JSON string must never match a status filter")
	assert.False(t, recordMatchesStatus([]byte(`["failed"]`), "failed"), "an array must never match a status filter")
	assert.False(t, recordMatchesStatus([]byte(`{"Status":"failed"}`), "failed"), "the field name is matched exactly")
	assert.False(t, recordMatchesStatus([]byte(`{"status":null}`), "failed"), "a null status must not match")
}

// TestMatchesPatternMiddleWildcard: a middle `*` matches only a key at least as long as its two
// literal parts together, so the parts never share a character (#371).
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestMatchesPatternMiddleWildcard(t *testing.T) {
	for key, want := range map[string]bool{
		"aaa":   false, // prefix and suffix would overlap
		"aaaa":  true,  // empty wildcard
		"aaXaa": true,
		"aXaa":  false,
	} {
		assert.Equal(t, want, matchesPattern(key, "aa*aa"), "matchesPattern(%q, %q)", key, "aa*aa")
	}
}

// TestMessageLoggerRoutesAfterStop: after Stop, /entries, /stats and /trace still answer from memory,
// /subjects answers [], and a watch answers 503 and opens nothing (D2, "Reads during and after
// Stop"). The 503 is checked on a well-formed bucket, which before Stop is looked up (500 on this
// never-connected client, TestKVRoutesRejectMalformedBucket), so a 503 shows the watch refused
// before any lookup.
//
// spec: operator-surface / The message logger's routes are served under /message-logger
func TestMessageLoggerRoutesAfterStop(t *testing.T) {
	rl := newRouteLogger(t, `{"monitor_subjects":["*"]}`)
	traced := natsclient.ContextWithTrace(t.Context(), &natsclient.TraceContext{
		TraceID: routeTestTraceID, SpanID: "0123456789abcdef", Sampled: true,
	})
	rl.store(traced, "test.after.stop", []byte(`{"n":0}`))
	rl.store(traced, "test.after.stop", []byte(`{"n":1}`))

	stop := func() error {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		return rl.ml.Stop(ctx)
	}
	startCtx, cancelStart := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelStart()
	require.NoError(t, rl.ml.Start(startCtx))
	t.Cleanup(func() { assert.NoError(t, stop()) })
	require.NoError(t, stop())

	require.Equal(t, []int{1, 0}, payloadNumbers(t, rl.getEntries(t, "/message-logger/entries")))

	rec := rl.serve(t, http.MethodGet, "/message-logger/stats")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var stats struct {
		Total int `json:"total_messages"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats), rec.Body.String())
	require.Equal(t, 2, stats.Total)

	rec = rl.serve(t, http.MethodGet, "/message-logger/trace/"+routeTestTraceID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var trace struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &trace), rec.Body.String())
	require.Equal(t, 2, trace.Count)

	rec = rl.serve(t, http.MethodGet, "/message-logger/subjects")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.JSONEq(t, `[]`, rec.Body.String())

	rec = rl.serve(t, http.MethodGet, "/message-logger/kv/KV_TEST/watch")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
}
