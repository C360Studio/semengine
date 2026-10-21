package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The pin's four TestManager_* middleware tests drive a Manager and move with the manager's
// tests (task 2.1, part 3).

// recordingMiddleware appends a label to the trace slice on entry
// and exit. Lets tests assert outermost-first ordering by reading
// the trace after the handler runs.
func recordingMiddleware(label string, trace *[]string) HTTPMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*trace = append(*trace, "enter:"+label)
			next.ServeHTTP(w, r)
			*trace = append(*trace, "exit:"+label)
		})
	}
}

// shortCircuitMiddleware writes a status and never calls next. Used
// to verify that an auth-style middleware can foreclose the inner
// handler.
func shortCircuitMiddleware(status int, body string) HTTPMiddleware {
	return func(_ http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		})
	}
}

func TestChainMiddleware_OrderIsOutermostFirst(t *testing.T) {
	t.Parallel()

	var trace []string
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		trace = append(trace, "handler")
		w.WriteHeader(http.StatusOK)
	})

	chain := chainMiddleware(inner, []HTTPMiddleware{
		recordingMiddleware("A", &trace),
		recordingMiddleware("B", &trace),
		recordingMiddleware("C", &trace),
	})

	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	want := []string{
		"enter:A", "enter:B", "enter:C",
		"handler",
		"exit:C", "exit:B", "exit:A",
	}
	if got := fmt.Sprint(trace); got != fmt.Sprint(want) {
		t.Fatalf("middleware order wrong:\n got: %v\nwant: %v", trace, want)
	}
}

func TestChainMiddleware_EmptyAndNilSlicePassThrough(t *testing.T) {
	t.Parallel()

	cases := map[string][]HTTPMiddleware{
		"nil slice":   nil,
		"empty slice": {},
		"all nils":    {nil, nil},
	}

	for name, mws := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var ran bool
			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				ran = true
				w.WriteHeader(http.StatusTeapot)
			})

			chain := chainMiddleware(inner, mws)

			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

			if !ran {
				t.Fatal("inner handler did not run")
			}
			if rec.Code != http.StatusTeapot {
				t.Fatalf("status: got %d, want %d", rec.Code, http.StatusTeapot)
			}
		})
	}
}

func TestChainMiddleware_ShortCircuitForeclosesInnerHandler(t *testing.T) {
	t.Parallel()

	var ran bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	})

	// Outer auth-style middleware that 401s before delegating.
	chain := chainMiddleware(inner, []HTTPMiddleware{
		shortCircuitMiddleware(http.StatusUnauthorized, "no token"),
	})

	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if ran {
		t.Fatal("inner handler ran despite short-circuit middleware")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", rec.Code)
	}
	if got := rec.Body.String(); !strings.Contains(got, "no token") {
		t.Fatalf("body: got %q, want substring 'no token'", got)
	}
}

func TestChainMiddleware_NilEntriesAreSkipped(t *testing.T) {
	t.Parallel()

	var trace []string
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		trace = append(trace, "handler")
		w.WriteHeader(http.StatusOK)
	})

	chain := chainMiddleware(inner, []HTTPMiddleware{
		recordingMiddleware("A", &trace),
		nil,
		recordingMiddleware("B", &trace),
	})

	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	want := []string{"enter:A", "enter:B", "handler", "exit:B", "exit:A"}
	if got := fmt.Sprint(trace); got != fmt.Sprint(want) {
		t.Fatalf("nil entries should be skipped:\n got: %v\nwant: %v", trace, want)
	}
}
