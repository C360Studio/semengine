package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/flowgraph"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
)

// The message logger's construction, subject and in-memory tests (design D7, U-b). Every logger
// is built through NewMessageLoggerService, by U-a's helper over a client that never connects, and
// none of them is started except to watch Start refuse a malformed subject. The pin's tests that
// built the logger through the dropped NewMessageLogger, read ConfigSchema or the default config
// (pin message_logger_test.go:12-96, :222-266's DefaultMessageLoggerConfig) are adapted to the
// constructor's JSON or dropped with that surface (D1).

// TestMessageLoggerConstructionRefusals: NewMessageLoggerService refuses each input D1 names, with
// an invalid-class error naming it, and builds nothing; the same input with the one fact changed
// builds. Strict decoding refuses an unknown key and a key in another letter case, naming the key.
func TestMessageLoggerConstructionRefusals(t *testing.T) {
	client := newUnconnectedClient(t)
	deps := func() *Dependencies {
		return &Dependencies{
			NATSClient:        client,
			ComponentRegistry: component.NewRegistry(),
			PayloadRegistry:   payloadregistry.New(),
		}
	}

	for _, tt := range []struct {
		name   string
		config string
		deps   func() *Dependencies
		want   string
	}{
		{
			name: "nil NATS client", config: `{"monitor_subjects":["a.b"]}`, want: "NATS client",
			deps: func() *Dependencies { d := deps(); d.NATSClient = nil; return d },
		},
		{
			name: "nil payload registry", config: `{"monitor_subjects":["a.b"]}`, want: "payload registry",
			deps: func() *Dependencies { d := deps(); d.PayloadRegistry = nil; return d },
		},
		{
			name: "auto-discover without a component registry", config: `{"monitor_subjects":["*"]}`,
			want: "component registry",
			deps: func() *Dependencies { d := deps(); d.ComponentRegistry = nil; return d },
		},
		{
			// An empty monitor_subjects means ["*"] (D1's one home for defaults), so it needs the
			// component registry too.
			name: "default subjects without a component registry", config: `{}`, want: "component registry",
			deps: func() *Dependencies { d := deps(); d.ComponentRegistry = nil; return d },
		},
		{name: "negative max_entries", config: `{"max_entries":-1}`, deps: deps, want: "max_entries"},
		{name: "max_entries above 100000", config: `{"max_entries":100001}`, deps: deps, want: "max_entries"},
		{name: "negative sample_rate", config: `{"sample_rate":-1}`, deps: deps, want: "sample_rate"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewMessageLoggerService(json.RawMessage(tt.config), tt.deps())
			require.ErrorContains(t, err, tt.want)
			require.True(t, errs.IsInvalid(err), "refusal %v is not invalid-class", err)
			require.Nil(t, svc)
		})
	}

	t.Run("unknown key", func(t *testing.T) {
		svc, err := NewMessageLoggerService(json.RawMessage(`{"enabled":true}`), deps())
		require.ErrorContains(t, err, `unknown field "enabled"`)
		require.Nil(t, svc)
	})

	t.Run("key in another letter case", func(t *testing.T) {
		svc, err := NewMessageLoggerService(json.RawMessage(`{"Max_Entries":10}`), deps())
		require.ErrorContains(t, err, "config key Max_Entries must be written max_entries")
		require.Nil(t, svc)
	})

	// The bounds themselves build, and the defaults are the ones D1 states.
	for _, config := range []string{
		`{"max_entries":0}`, `{"max_entries":100000}`, `{"sample_rate":0}`, `{"sample_rate":1}`, `{}`,
	} {
		t.Run("accepts "+config, func(t *testing.T) {
			svc, err := NewMessageLoggerService(json.RawMessage(config), deps())
			require.NoError(t, err)
			require.IsType(t, &MessageLogger{}, svc)
		})
	}
	t.Run("defaults", func(t *testing.T) {
		stats := newTestMessageLogger(t, client, `{}`, nil).GetStatistics()
		require.EqualValues(t, 10000, stats["max_entries"])
		require.EqualValues(t, 1, stats["sample_rate"])
	})
}

// TestMessageLoggerSampling: with sample_rate n every n-th message reaching the subscription
// callback is stored and the rest only counted; 0 (the default) and 1 store every message (pin
// message_logger_test.go:98-148 and :268-279, through the constructor and handleMessage instead
// of a struct literal and shouldSample).
func TestMessageLoggerSampling(t *testing.T) {
	for _, tt := range []struct {
		config     string
		calls      int
		wantStored int
	}{
		{config: `{"monitor_subjects":["test.>"]}`, calls: 10, wantStored: 10},
		{config: `{"monitor_subjects":["test.>"],"sample_rate":1}`, calls: 10, wantStored: 10},
		{config: `{"monitor_subjects":["test.>"],"sample_rate":2}`, calls: 10, wantStored: 5},
		{config: `{"monitor_subjects":["test.>"],"sample_rate":10}`, calls: 100, wantStored: 10},
	} {
		t.Run(tt.config, func(t *testing.T) {
			ml := newTestMessageLogger(t, newUnconnectedClient(t), tt.config, nil)
			for i := range tt.calls {
				ml.handleMessage(t.Context(), "test.sampled", fmt.Appendf(nil, `{"n":%d}`, i))
			}

			require.Len(t, ml.GetLogEntries(0), tt.wantStored)
			stats := ml.GetStatistics()
			require.EqualValues(t, tt.calls, stats["total_messages"])
			require.EqualValues(t, tt.wantStored, stats["sampled_messages"])
			require.Contains(t, stats, "sample_rate")
		})
	}
}

// TestContainsWildcard: only the whole subject "*" asks for auto-discovery; NATS wildcards inside a
// subject do not (pin message_logger_test.go:150-189).
func TestContainsWildcard(t *testing.T) {
	tests := []struct {
		name     string
		subjects []string
		want     bool
	}{
		{name: "empty_list", subjects: []string{}, want: false},
		{name: "no_wildcard", subjects: []string{"raw.udp.messages", "processed.>"}, want: false},
		{name: "only_wildcard", subjects: []string{"*"}, want: true},
		{name: "wildcard_with_others", subjects: []string{"*", "debug.>"}, want: true},
		{name: "nats_wildcard_not_auto_discover", subjects: []string{"raw.>", "*.messages"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, containsWildcard(tt.subjects))
		})
	}
}

func TestUniqueStringsPreservesFirstOccurrence(t *testing.T) {
	got := uniqueStrings([]string{"graph.mutation.>", "raw.>", "graph.mutation.>", "raw.>"})
	require.Equal(t, []string{"graph.mutation.>", "raw.>"}, got)
}

// TestResolveLoggerSubjectsOmitsCoveredSubjects: a desired subject another desired subject covers
// is not subscribed and is reported as an overlap; two subjects that overlap without either
// covering the other are both subscribed (design D3, M10). The pin's version (:198-220) omitted
// only three named agentic pairs, so "unrelated.*" was subscribed beside "unrelated.>".
func TestResolveLoggerSubjectsOmitsCoveredSubjects(t *testing.T) {
	desired := map[string]struct{}{
		"agent.toolcall.proposed.>": {}, "agent.toolcall.proposed.*": {},
		"unrelated.>": {}, "unrelated.*": {},
		"a.b.*": {}, "*.b.c": {}, // share a.b.c, neither covers the other
		"x.y": {},
	}

	subjects, overlaps := resolveLoggerSubjects(desired)
	require.Equal(t, []string{"*.b.c", "a.b.*", "agent.toolcall.proposed.>", "unrelated.>", "x.y"}, subjects)
	require.ElementsMatch(t, []subjectOverlap{
		{Broader: "agent.toolcall.proposed.>", Covered: "agent.toolcall.proposed.*", Resolution: "covered subscription omitted"},
		{Broader: "unrelated.>", Covered: "unrelated.*", Resolution: "covered subscription omitted"},
	}, overlaps)
}

// TestPropCoveredSubjectsSubscribedOnce checks D3's law on every generated subject set: every
// desired subject is covered by at least one subscribed subject; no subscribed subject covers
// another; the subscribed subjects are a sorted, duplicate-free subset of the desired ones; and
// each desired subject left out is reported once, as covered by another desired subject that
// covers it. The oracle is flowgraph.SubjectCovers, tested on its own
// (internal/flowgraph/subject_cover_test.go); the reference result is not recomputed.
//
// Generator: sets of zero to six subjects, each one to three tokens from {a, b, *}, the last
// possibly >, so every subject is valid under D3's grammar by construction (the reduction's input
// is a validated set) and drawn from a small alphabet so covers are common. A slice is drawn and
// folded into the set, so a repeated subject is part of the input. What makes each assertion run:
// the cases below are counted when a set reaches them, and the test fails at the end unless each
// count is above zero, so a generator that stops reaching one fails rather than passing quietly.
// The run's seed and completed checks are recorded in the notes when the test first runs (task
// 2.2).
func TestPropCoveredSubjectsSubscribedOnce(t *testing.T) {
	reached := map[string]int{}
	cases := []string{
		"a subject omitted under a > subject",
		"a subject omitted under a * subject with no >",
		"a chain of three covers",
		"two subscribed subjects overlap without a cover",
		"a repeated subject",
		"nothing omitted from two or more subjects",
	}

	rapid.Check(t, func(rt *rapid.T) {
		drawn := rapid.SliceOfN(rapid.Custom(drawValidSubject), 0, 6).Draw(rt, "subjects")
		desired := make(map[string]struct{}, len(drawn))
		for _, s := range drawn {
			desired[s] = struct{}{}
		}
		input := make(map[string]struct{}, len(desired))
		for s := range desired {
			input[s] = struct{}{}
		}

		subjects, overlaps := resolveLoggerSubjects(desired)

		// Sorted, duplicate-free subset of the input.
		if !slices.IsSorted(subjects) {
			rt.Fatalf("subscribed %q is not sorted", subjects)
		}
		if len(slices.Compact(slices.Clone(subjects))) != len(subjects) {
			rt.Fatalf("subscribed %q holds a duplicate", subjects)
		}
		subscribed := make(map[string]bool, len(subjects))
		for _, s := range subjects {
			if _, ok := input[s]; !ok {
				rt.Fatalf("subscribed %q was never desired (input %q)", s, drawn)
			}
			subscribed[s] = true
		}

		// Every desired subject is covered by a subscribed one.
		for s := range input {
			if !slices.ContainsFunc(subjects, func(sub string) bool { return flowgraph.SubjectCovers(sub, s) }) {
				rt.Fatalf("desired %q is covered by no subscribed subject in %q", s, subjects)
			}
		}

		// No subscribed subject covers another.
		for _, a := range subjects {
			for _, b := range subjects {
				if a != b && flowgraph.SubjectCovers(a, b) {
					rt.Fatalf("subscribed %q covers subscribed %q", a, b)
				}
			}
		}

		// Each omission is reported once, by a desired subject that covers it.
		reported := map[string]int{}
		for _, o := range overlaps {
			reported[o.Covered]++
			if o.Resolution != "covered subscription omitted" {
				rt.Fatalf("overlap %+v has resolution %q", o, o.Resolution)
			}
			if _, ok := input[o.Broader]; !ok || o.Broader == o.Covered || !flowgraph.SubjectCovers(o.Broader, o.Covered) {
				rt.Fatalf("overlap %+v: broader is not another desired subject covering the omitted one", o)
			}
		}
		for s := range input {
			want := 0
			if !subscribed[s] {
				want = 1
			}
			if reported[s] != want {
				rt.Fatalf("desired %q reported as omitted %d times, want %d (subscribed %q)", s, reported[s], want, subjects)
			}
		}

		// Count the cases the assertions above ran on.
		for _, o := range overlaps {
			if strings.HasSuffix(o.Broader, ">") {
				reached["a subject omitted under a > subject"]++
			} else {
				reached["a subject omitted under a * subject with no >"]++
			}
		}
		for a := range input {
			for b := range input {
				for c := range input {
					if a != b && b != c && flowgraph.SubjectCovers(a, b) && flowgraph.SubjectCovers(b, c) {
						reached["a chain of three covers"]++
					}
				}
			}
		}
		for _, a := range subjects {
			for _, b := range subjects {
				if a < b && flowgraph.SubjectsOverlap(a, b) {
					reached["two subscribed subjects overlap without a cover"]++
				}
			}
		}
		if len(drawn) > len(input) {
			reached["a repeated subject"]++
		}
		if len(overlaps) == 0 && len(subjects) >= 2 {
			reached["nothing omitted from two or more subjects"]++
		}
	})

	if t.Failed() {
		return
	}
	for _, c := range cases {
		t.Logf("case %q reached %d times", c, reached[c])
	}
	for _, c := range cases {
		if reached[c] == 0 {
			t.Errorf("no generated set reached %q, so its assertions never ran", c)
		}
	}
}

// drawValidSubject draws one to three tokens from {a, b, *}, the last possibly >: valid under D3's
// grammar by construction.
func drawValidSubject(rt *rapid.T) string {
	n := rapid.IntRange(1, 3).Draw(rt, "tokens")
	tokens := make([]string, n)
	for i := range n - 1 {
		tokens[i] = rapid.SampledFrom([]string{"a", "b", "*"}).Draw(rt, "token")
	}
	tokens[n-1] = rapid.SampledFrom([]string{"a", "b", "*", ">"}).Draw(rt, "last token")
	return strings.Join(tokens, ".")
}

// requireMessageLoggerHoldsNothing: the base service is not running and no subscription or
// subscribe retry is held (the fields U-a's suite adapter reads, under the same lock).
func requireMessageLoggerHoldsNothing(t *testing.T, ml *MessageLogger) {
	t.Helper()
	require.Equal(t, StatusStopped, ml.Status())
	ml.lifecycleMu.Lock()
	defer ml.lifecycleMu.Unlock()
	require.Empty(t, ml.subscriptions)
	require.Nil(t, ml.retryDone)
}

// stopMessageLoggerAtCleanup stops ml under a bounded context when the test ends, so a Start that
// wrongly held something does not leak it past the test.
func stopMessageLoggerAtCleanup(t *testing.T, ml *MessageLogger) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		if err := ml.Stop(ctx); err != nil {
			t.Logf("cleanup Stop: %v", err)
		}
	})
}

// TestMessageLoggerRefusesMalformedSubject: Start checks every subject before it starts the base
// service or subscribes, and refuses the first malformed one with an invalid-class error naming
// the subject (quoted, so the empty subject is visible) and its source, monitor_subjects; the
// logger holds nothing afterwards (design D3, M11, #368). The client never connects, so without
// the check the subscribe would fail with natsclient's ErrNotConnected and be retried, and Start
// would return nil. A valid subject listed first shows the refusal does not wait for a subscribe.
func TestMessageLoggerRefusesMalformedSubject(t *testing.T) {
	for _, subject := range []string{"a..b", "a.foo bar", "a.>.b", "a*", ""} {
		t.Run(fmt.Sprintf("%q", subject), func(t *testing.T) {
			config, err := json.Marshal(map[string][]string{"monitor_subjects": {"valid.first", subject}})
			require.NoError(t, err)
			ml := newTestMessageLogger(t, newUnconnectedClient(t), string(config), nil)
			stopMessageLoggerAtCleanup(t, ml)

			err = ml.Start(t.Context())
			require.Error(t, err)
			require.True(t, errs.IsInvalid(err), "refusal %v is not invalid-class", err)
			require.ErrorContains(t, err, fmt.Sprintf("%q", subject))
			require.ErrorContains(t, err, "monitor_subjects")
			requireMessageLoggerHoldsNothing(t, ml)
		})
	}
}

// TestMessageLoggerRefusesMalformedSubjectCoveredByBroader: a malformed subject is refused even
// when a broader subject in the set would otherwise lead the cover reduction to omit it, because
// validation runs before the reduction (design D3: with ["a.>", "a.foo bar"] the pin's order
// removed the second before anything could refuse it).
func TestMessageLoggerRefusesMalformedSubjectCoveredByBroader(t *testing.T) {
	ml := newTestMessageLogger(t, newUnconnectedClient(t), `{"monitor_subjects":["a.>","a.foo bar"]}`, nil)
	stopMessageLoggerAtCleanup(t, ml)

	err := ml.Start(t.Context())
	require.Error(t, err)
	require.True(t, errs.IsInvalid(err), "refusal %v is not invalid-class", err)
	require.ErrorContains(t, err, `"a.foo bar"`)
	require.ErrorContains(t, err, "monitor_subjects")
	requireMessageLoggerHoldsNothing(t, ml)
}

// The trace and ring tests store entries through storeEntry with sequences the test chooses
// (design D7: "trace tests use storeEntry only"; D4, M13: the trace index is dropped and
// GetEntriesByTrace scans the buffer).

// TestMessageLoggerEntriesByTrace: a trace query returns the entries of that trace still held,
// oldest first, whatever order they were stored in; an unknown trace returns none (pin
// message_logger_test.go:268-347).
func TestMessageLoggerEntriesByTrace(t *testing.T) {
	ml := newTestMessageLogger(t, newUnconnectedClient(t), `{"monitor_subjects":["test.>"]}`, nil)
	traceA := "aaaabbbbccccdddd1111222233334444"
	traceB := "55556666777788889999aaaabbbbcccc"

	// Stored out of order, as two callbacks that took sequences in order can reach the buffer
	// in reverse.
	for _, e := range []MessageLogEntry{
		{Sequence: 5, Subject: "test.subject", TraceID: traceA, Summary: "a5"},
		{Sequence: 2, Subject: "test.subject", TraceID: traceB, Summary: "b2"},
		{Sequence: 1, Subject: "test.subject", TraceID: traceA, Summary: "a1"},
		{Sequence: 4, Subject: "test.subject", TraceID: traceB, Summary: "b4"},
		{Sequence: 3, Subject: "test.subject", TraceID: traceA, Summary: "a3"},
		{Sequence: 6, Subject: "test.subject", Summary: "no trace"},
	} {
		ml.storeEntry(e)
	}

	require.Equal(t, []string{"a1", "a3", "a5"}, entrySummaries(ml.GetEntriesByTrace(traceA)))
	require.Equal(t, []string{"b2", "b4"}, entrySummaries(ml.GetEntriesByTrace(traceB)))
	require.Empty(t, ml.GetEntriesByTrace("00000000000000000000000000000000"))
}

// TestMessageLoggerTraceOmitsOverwrittenEntry: once the ring wraps over a trace's entry, the trace
// query no longer returns it (pin message_logger_test.go:379-415).
func TestMessageLoggerTraceOmitsOverwrittenEntry(t *testing.T) {
	ml := newTestMessageLogger(t, newUnconnectedClient(t), `{"monitor_subjects":["test.>"],"max_entries":10}`, nil)
	trace := "aaaabbbbccccdddd1111222233334444"

	ml.storeEntry(MessageLogEntry{Sequence: 1, TraceID: trace, Summary: "old"})
	for seq := uint64(2); seq <= 11; seq++ { // ten more entries: sequence 11 takes sequence 1's slot
		ml.storeEntry(MessageLogEntry{Sequence: seq, Summary: "filler"})
	}
	ml.storeEntry(MessageLogEntry{Sequence: 12, TraceID: trace, Summary: "new"})

	require.Equal(t, []string{"new"}, entrySummaries(ml.GetEntriesByTrace(trace)))
}

// TestMessageLoggerOlderEntryCannotOverwriteNewer: an entry stored late never replaces a newer one
// already in its slot (the ring's law, pin message_logger.go:1062-1075; test :349-363).
func TestMessageLoggerOlderEntryCannotOverwriteNewer(t *testing.T) {
	ml := newTestMessageLogger(t, newUnconnectedClient(t), `{"monitor_subjects":["test.>"],"max_entries":1}`, nil)

	ml.storeEntry(MessageLogEntry{Sequence: 2, Summary: "newer"})
	ml.storeEntry(MessageLogEntry{Sequence: 1, Summary: "delayed older"})

	entries := ml.GetLogEntries(0)
	require.Len(t, entries, 1)
	assert.Equal(t, uint64(2), entries[0].Sequence)
	assert.Equal(t, "newer", entries[0].Summary)
}

// TestMessageLoggerRecentEntriesAreTheStoredOnes: the entries read are the ones stored, not a
// range of sequences counted back from the newest; a sequence taken by a callback that has not
// reached the buffer leaves no empty entry (pin message_logger_test.go:365-377).
func TestMessageLoggerRecentEntriesAreTheStoredOnes(t *testing.T) {
	ml := newTestMessageLogger(t, newUnconnectedClient(t), `{"monitor_subjects":["test.>"],"max_entries":2}`, nil)

	ml.storeEntry(MessageLogEntry{Sequence: 2, Summary: "visible"}) // sequence 1 never stored

	entries := ml.GetLogEntries(0)
	require.Len(t, entries, 1)
	assert.Equal(t, uint64(2), entries[0].Sequence)
}

func entrySummaries(entries []MessageLogEntry) []string {
	summaries := make([]string, 0, len(entries))
	for _, e := range entries {
		summaries = append(summaries, e.Summary)
	}
	return summaries
}
