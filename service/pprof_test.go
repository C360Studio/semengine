package service

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof" // populate DefaultServeMux so the index test serves /debug/pprof
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/pkg/errs"
)

// recordingHandler is a slog.Handler that keeps every record, so a test can read what was logged.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

// warnings returns the text of each Warn record: its message and every attribute value.
func (h *recordingHandler) warnings() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, r := range h.records {
		if r.Level != slog.LevelWarn {
			continue
		}
		text := r.Message
		r.Attrs(func(a slog.Attr) bool {
			text += " " + a.Key + "=" + a.Value.String()
			return true
		})
		out = append(out, text)
	}
	return out
}

// loopback returns a listener on an ephemeral loopback port.
func loopback(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return listener
}

// shutdownInCleanup shuts server down when the test ends, under a fresh bounded context.
func shutdownInCleanup(t *testing.T, server *PProfServer) {
	t.Helper()
	t.Cleanup(func() { require.NoError(t, server.Shutdown(stopContext(t))) })
}

// TestMaybeStartPProfDisabledStartsNothing: with debug off or a port that is not positive,
// MaybeStartPProf starts nothing and logs nothing, and the nil server's Shutdown returns nil
// (design D3).
//
// spec: service-lifecycle / The profiling server stops on Shutdown
func TestMaybeStartPProfDisabledStartsNothing(t *testing.T) {
	for _, tt := range []struct {
		debug bool
		port  int
	}{{false, 1}, {true, 0}, {true, -1}} {
		rec := &recordingHandler{}
		server := MaybeStartPProf(tt.debug, tt.port, slog.New(rec))
		require.Nil(t, server, "debug=%v port=%d", tt.debug, tt.port)
		require.NoError(t, server.Shutdown(stopContext(t)))
		require.Empty(t, rec.records, "a disabled pprof server logged")
	}
}

// TestMaybeStartPProfLogsABindFailure: a port another listener holds fails the bind, which
// MaybeStartPProf logs at Warn naming the port, and nothing is started (design D3, probe H1).
//
// spec: service-lifecycle / The profiling server stops on Shutdown
func TestMaybeStartPProfLogsABindFailure(t *testing.T) {
	// Every address: blocks MaybeStartPProf's ":port" bind (pprof.go).
	held, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Close() })
	port := held.Addr().(*net.TCPAddr).Port

	rec := &recordingHandler{}
	server := MaybeStartPProf(true, port, slog.New(rec))
	if server != nil {
		shutdownInCleanup(t, server)
	}
	require.Nil(t, server, "MaybeStartPProf returned a server for a port it could not bind")

	warnings := rec.warnings()
	require.Len(t, warnings, 1, "want one Warn for the failed bind")
	require.Contains(t, warnings[0], strconv.Itoa(port), "the Warn does not name the port")
}

// TestPProfShutdownRefusesANilContext: Shutdown refuses a nil context before anything else, on a
// started server and on a nil one, and the started server still serves afterwards (design D3).
//
// spec: service-lifecycle / The profiling server stops on Shutdown
func TestPProfShutdownRefusesANilContext(t *testing.T) {
	var nilCtx context.Context

	var none *PProfServer
	require.True(t, errs.IsInvalid(none.Shutdown(nilCtx)), "a nil server's Shutdown(nil) was not refused as invalid")

	listener := loopback(t)
	server := startPProf(&http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})}, listener)
	shutdownInCleanup(t, server)

	require.True(t, errs.IsInvalid(server.Shutdown(nilCtx)), "Shutdown(nil) was not refused as invalid")

	client := &http.Client{Transport: &http.Transport{}}
	t.Cleanup(client.CloseIdleConnections)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String()+"/", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err, "the server stopped serving after a refused Shutdown")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "ok", string(body))
}

// TestPProfShutdownWaitsForAnInFlightRequest: a Shutdown given an ended context while a request is
// blocked in its handler returns that context's error; once the handler returns, a later Shutdown
// returns nil (design D3; probes H2 to H4).
//
// spec: service-lifecycle / The profiling server stops on Shutdown
func TestPProfShutdownWaitsForAnInFlightRequest(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	listener := loopback(t)
	server := startPProf(&http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, "done")
	})}, listener)
	// On a failure before the release, the handler is released and the server shut down here.
	releaseHandler := sync.OnceFunc(func() { close(release) })
	shutdownInCleanup(t, server)
	t.Cleanup(releaseHandler)

	client := &http.Client{Transport: &http.Transport{}}
	t.Cleanup(client.CloseIdleConnections)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String()+"/block", nil)
	require.NoError(t, err)
	type reply struct {
		body string
		err  error
	}
	replied := make(chan reply, 1)
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			replied <- reply{err: err}
			return
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		replied <- reply{body: string(body), err: err}
	}()
	<-entered

	endedCtx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, server.Shutdown(endedCtx), context.Canceled,
		"Shutdown with an ended context returned while a request was in flight")

	releaseHandler()
	got := <-replied
	require.NoError(t, got.err)
	require.Equal(t, "done", got.body, "the in-flight request was cut off")
	require.NoError(t, server.Shutdown(stopContext(t)))
}

// TestPProfServesTheIndexAndLeavesNothingRunning serves the pprof index through startPProf inside
// synctest.Test (background-work, "Stopped inside a bubble"; design D3, probe H5). The bubble
// returns only once every goroutine it started has exited, so a Shutdown that left the serving
// goroutine running fails the test.
//
// spec: service-lifecycle / The profiling server stops on Shutdown
func TestPProfServesTheIndexAndLeavesNothingRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := loopback(t)
		server := startPProf(&http.Server{}, listener)

		transport := &http.Transport{}
		client := &http.Client{Transport: transport}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			"http://"+listener.Addr().String()+"/debug/pprof/", nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode, "/debug/pprof/ must serve the pprof index")
		require.True(t, strings.Contains(string(body), "goroutine"), "the body is not the pprof index")
		transport.CloseIdleConnections()

		require.NoError(t, server.Shutdown(t.Context()))
	})
}
