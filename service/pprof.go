package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/c360studio/semengine/pkg/errs"
)

// PProfServer is the profiling server MaybeStartPProf started. Shutdown stops
// it. A nil *PProfServer is a server that was never started; its Shutdown
// returns nil.
type PProfServer struct {
	server *http.Server
	// done is closed when Serve's goroutine returns; serveErr is set before.
	done     chan struct{}
	serveErr error
}

// MaybeStartPProf serves http.DefaultServeMux on :port when debug && port > 0,
// and returns nil otherwise. It binds :port before it returns. A bind failure is
// logged at Warn on logger (slog.Default() when nil) and returns nil; profiling
// is a debug aid, never a boot gate.
//
// SemStreams ADR-058 rollout step 4: pprof is deliberately NOT a
// service.Service. Wrapping it in a StartAll-managed Service would move the
// start from the composition root (before NATS) to StartAll (after NATS and all
// wiring), losing the ability to profile a wedged or slow boot, the canonical
// pprof use case. So it is a plain helper the composition root calls early. It
// is not fire-and-forget: the composition root calls Shutdown on its exit path,
// after its services have stopped (background-work spec).
//
// The pprof handlers are registered on http.DefaultServeMux by the CALLER's
// `import _ "net/http/pprof"` blank import; this helper serves that mux (nil
// handler). The blank import stays in the caller's main, not here, so importing
// package service does not arm pprof everywhere.
func MaybeStartPProf(debug bool, port int, logger *slog.Logger) *PProfServer {
	if !debug || port <= 0 {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Warn("pprof server not started: bind failed", "port", port, "error", err)
		return nil
	}
	logger.Info("pprof server listening", "addr", listener.Addr().String())
	return startPProf(&http.Server{}, listener)
}

// startPProf serves server on listener on a goroutine Shutdown joins.
func startPProf(server *http.Server, listener net.Listener) *PProfServer {
	s := &PProfServer{server: server, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			s.serveErr = err
		}
	}()
	return s
}

// Shutdown stops the server accepting and returns nil only after every
// in-flight request has finished and the serving goroutine has exited. If ctx
// ends first it returns ctx's error, and a later Shutdown waits again. A serve
// error other than http.ErrServerClosed is returned once the goroutine has
// exited. A nil ctx is refused, on a nil server too; a nil server's Shutdown
// otherwise returns nil.
func (s *PProfServer) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "PProfServer", "Shutdown", "nil context")
	}
	if s == nil {
		return nil
	}
	if err := s.server.Shutdown(ctx); err != nil {
		return err
	}
	select {
	case <-s.done:
		return s.serveErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
