package service

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// The pin's five TestManager_* tests read the private middleware slice and drove the private
// buildHTTPHandler on a recorder. Here they register middleware through UseHTTPMiddleware, start
// the manager, and request a service's route on the main listener, so the wiring from the
// registered list to the served handler is what they prove (ADR-030; design D6).

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

// lockedTrace is a trace that the server's handler goroutines and the test share.
type lockedTrace struct {
	mu      sync.Mutex
	entries []string
}

// middleware returns a middleware that appends enter:label and exit:label around the handler.
func (l *lockedTrace) middleware(label string) HTTPMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			l.add("enter:" + label)
			next.ServeHTTP(w, r)
			l.add("exit:" + label)
		})
	}
}

func (l *lockedTrace) add(entry string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *lockedTrace) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

// newProbeManager returns a manager composing the stand-in component manager and a service named
// probe whose route /probe/probe answers status with body "probe". net/http sends a response this
// small only once the handler chain has returned, so a middleware's exit is recorded before the
// client reads the answer.
func newProbeManager(t *testing.T, deps *Dependencies, status int) *Manager {
	t.Helper()
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance("probe", newRoutedService("probe", func(prefix string, mux *http.ServeMux) {
		mux.HandleFunc("GET "+prefix+"/probe", answer(status, "probe"))
	})))
	return manager
}

// TestManager_UseHTTPMiddleware_AppendsBeforeBoot: middleware registered by several calls before
// StartAll is appended in call order, the first outermost (pin middleware_test.go:128, which
// counted the private slice).
func TestManager_UseHTTPMiddleware_AppendsBeforeBoot(t *testing.T) {
	var trace lockedTrace
	manager := newProbeManager(t, createTestServiceDependencies(), http.StatusOK)
	manager.UseHTTPMiddleware(trace.middleware("a"), trace.middleware("b"))
	manager.UseHTTPMiddleware(trace.middleware("c"))
	startTestManager(t, manager)

	requireAnswer(t, observedManagerHTTPURL(t, manager)+"/probe/probe", http.StatusOK, "probe")
	require.Equal(t, []string{"enter:a", "enter:b", "enter:c", "exit:c", "exit:b", "exit:a"}, trace.snapshot())
}

// TestManager_UseHTTPMiddleware_IgnoredAfterServerStarts: middleware registered after the main
// listener started is not built and never runs, and the call logs a Warn (pin :143, which set the
// private server field to fake a start).
//
// spec: operator-surface / Products wrap every route through one middleware seam
func TestManager_UseHTTPMiddleware_IgnoredAfterServerStarts(t *testing.T) {
	deps := createTestServiceDependencies()
	logged := &recordingHandler{}
	deps.Logger = slog.New(logged)
	manager := newProbeManager(t, deps, http.StatusOK)
	startTestManager(t, manager)

	var built atomic.Bool
	manager.UseHTTPMiddleware(func(http.Handler) http.Handler {
		built.Store(true)
		return answer(http.StatusTeapot, "late middleware")
	})

	requireAnswer(t, observedManagerHTTPURL(t, manager)+"/probe/probe", http.StatusOK, "probe")
	require.False(t, built.Load(), "middleware registered after the start was built")
	require.True(t, slices.ContainsFunc(logged.warnings(), func(text string) bool {
		return strings.Contains(text, "called after HTTP server started")
	}), "no Warn for middleware registered after the start: %v", logged.warnings())
}

// TestManager_UseHTTPMiddleware_EmptyAndNilArgsAreNoOps: calls with no middleware change nothing,
// and nil entries are skipped (pin :177).
func TestManager_UseHTTPMiddleware_EmptyAndNilArgsAreNoOps(t *testing.T) {
	var trace lockedTrace
	manager := newProbeManager(t, createTestServiceDependencies(), http.StatusOK)
	manager.UseHTTPMiddleware()
	manager.UseHTTPMiddleware([]HTTPMiddleware{}...)
	manager.UseHTTPMiddleware(nil, trace.middleware("a"), nil)
	startTestManager(t, manager)

	requireAnswer(t, observedManagerHTTPURL(t, manager)+"/probe/probe", http.StatusOK, "probe")
	require.Equal(t, []string{"enter:a", "exit:a"}, trace.snapshot())
}

// TestManager_BuildHTTPHandler_AppliesRegisteredMiddleware: a request to a service's route on the
// main listener runs through the registered middleware, the first registered outermost: the
// wired-path guard that the served handler is the chained one (pin :190).
//
// spec: operator-surface / Products wrap every route through one middleware seam
func TestManager_BuildHTTPHandler_AppliesRegisteredMiddleware(t *testing.T) {
	var trace lockedTrace
	manager := newProbeManager(t, createTestServiceDependencies(), http.StatusOK)
	manager.UseHTTPMiddleware(trace.middleware("outer"), trace.middleware("inner"))
	startTestManager(t, manager)

	requireAnswer(t, observedManagerHTTPURL(t, manager)+"/probe/probe", http.StatusOK, "probe")
	require.Equal(t, []string{"enter:outer", "enter:inner", "exit:inner", "exit:outer"}, trace.snapshot())
}

// TestManager_BuildHTTPHandler_PassesThroughWhenNoMiddleware: with no middleware a service's route
// answers unchanged (pin :236).
func TestManager_BuildHTTPHandler_PassesThroughWhenNoMiddleware(t *testing.T) {
	manager := newProbeManager(t, createTestServiceDependencies(), http.StatusTeapot)
	startTestManager(t, manager)

	requireAnswer(t, observedManagerHTTPURL(t, manager)+"/probe/probe", http.StatusTeapot, "probe")
}
