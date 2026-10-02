package metric

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/c360studio/semengine/internal/tlsutil"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/security"
)

// Server represents the metrics HTTP server
type Server struct {
	port      int
	path      string
	server    *http.Server
	listener  net.Listener
	serveDone chan error
	requests  *admittedRequests
	// opHook, when set before Start, is told each external operation the Server
	// performs (a bind, the serve goroutine, a shutdown or close); a test seam,
	// nil in production.
	opHook   func(op string)
	registry *MetricsRegistry
	security security.Config
	mu       sync.Mutex // serializes server lifecycle fields
	used     bool
	stopping bool
}

// NewServer creates a new metrics server with the provided registry
func NewServer(port int, path string, registry *MetricsRegistry, securityCfg security.Config) *Server {
	if path == "" {
		path = "/metrics"
	}
	if port == 0 {
		port = 9090
	}

	return &Server{
		port:     port,
		path:     path,
		registry: registry,
		security: securityCfg,
	}
}

// Start starts the one-shot metrics HTTP server. It binds synchronously and
// returns only after this Server owns its listener. ctx is the exact base
// context for served requests. Restart requires a freshly constructed Server.
func (s *Server) Start(ctx context.Context) error {
	return s.start(ctx, nil, false, "Start")
}

// StartWithListener starts the one-shot metrics HTTP server on a caller-bound
// raw TCP listener. A successful return transfers listener ownership to Server;
// every returned error leaves it with the caller. Server applies configured TLS
// itself, so callers must not pass a TLS-wrapped listener.
func (s *Server) StartWithListener(ctx context.Context, listener net.Listener) error {
	return s.start(ctx, listener, true, "StartWithListener")
}

func (s *Server) start(ctx context.Context, supplied net.Listener, provided bool, operation string) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Server", operation, "nil context")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "Server", operation, "context already ended")
	}
	if provided {
		if supplied == nil {
			return errs.WrapInvalid(errs.ErrInvalidData, "Server", operation, "nil listener")
		}
		addr, ok := supplied.Addr().(*net.TCPAddr)
		if !ok || addr.Port <= 0 {
			return errs.WrapInvalid(errs.ErrInvalidData, "Server", operation, "listener has no assigned TCP endpoint")
		}
		if socket, ok := supplied.(syscall.Conn); ok {
			raw, err := socket.SyscallConn()
			if err == nil {
				err = raw.Control(func(uintptr) {})
			}
			if err != nil {
				return errs.WrapInvalid(err, "Server", operation, "listener is closed")
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.used {
		return errs.WrapInvalid(
			errs.ErrAlreadyStarted,
			"Server", operation, "server instance already used")
	}
	s.used = true

	// Validate that we have a registry
	if s.registry == nil {
		return errs.WrapFatal(
			fmt.Errorf("nil registry"),
			"Server", operation, "metrics registry not provided")
	}
	// Client certificates are only verified over TLS; serving plain HTTP would drop the
	// configured client authentication without a word, so the policy is refused instead.
	if !s.security.TLS.Server.Enabled && s.security.TLS.Server.MTLS.Enabled {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Server", operation,
			"mTLS is enabled but server TLS is not")
	}

	mux := http.NewServeMux()

	// Create Prometheus HTTP handler
	handler := promhttp.HandlerFor(
		s.registry.PrometheusRegistry(),
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	)

	// Register the handler
	mux.Handle(s.path, handler)

	// Add a health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Add a root handler with information
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<html>
<head><title>SemStreams Metrics</title></head>
<body>
<h1>SemStreams Metrics Server</h1>
<p><a href="%s">Metrics</a></p>
<p><a href="/health">Health</a></p>
</body>
</html>`, s.path)
	})

	// Create the server
	s.requests = &admittedRequests{}
	s.server = &http.Server{
		Addr:        fmt.Sprintf(":%d", s.port),
		Handler:     s.requests.wrap(mux),
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	// Configure TLS if enabled at platform level
	if s.security.TLS.Server.Enabled {
		tlsConfig, err := tlsutil.LoadServerTLSConfigWithMTLS(s.security.TLS.Server, s.security.TLS.Server.MTLS)
		if err != nil {
			s.server = nil
			return errs.WrapFatal(err, "Server", operation, "load TLS config")
		}
		s.server.TLSConfig = tlsConfig
	}

	httpServer := s.server
	listener := supplied
	if !provided {
		var err error
		s.recordOp("listen")
		listener, err = net.Listen("tcp", httpServer.Addr)
		if err != nil {
			s.server = nil
			return errs.WrapFatal(err, "Server", operation,
				fmt.Sprintf("failed to start server on port %d", s.port))
		}
	}
	if s.security.TLS.Server.Enabled {
		listener = tls.NewListener(listener, httpServer.TLSConfig)
	}

	serveDone := make(chan error, 1)
	s.listener = listener
	s.serveDone = serveDone
	s.recordOp("serve")
	go func() {
		serveDone <- httpServer.Serve(listener)
		close(serveDone)
	}()

	return nil
}

// Stop attempts graceful shutdown within ctx. If that budget ends or graceful
// shutdown fails, Stop force-closes the server and listener and then waits for
// the exact serving goroutine, which returns once its listener is closed. Stop
// returns ctx's error whenever ctx has ended, even if shutdown completed. A
// request the server admitted, with the metrics collection it runs, is the
// server's work until its handler returns: Stop returns nil only after every
// such request has finished, and when ctx ends first it returns ctx's error and
// a later Stop waits again. A completed repeat is a nil no-op. Concurrent Stop
// is unsupported and returns a typed transient error.
func (s *Server) Stop(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Server", "Stop", "nil context")
	}
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return errs.WrapTransient(errors.New("metrics server stop already in progress"),
			"Server", "Stop", "concurrent Stop is unsupported")
	}
	if s.server == nil {
		s.used = true
		requests := s.requests
		if requests == nil || requests.pending() == 0 {
			s.mu.Unlock()
			return nil
		}
		// An earlier Stop's context ended while admitted requests still ran.
		s.stopping = true
		s.mu.Unlock()
		err := requests.wait(ctx)
		s.mu.Lock()
		s.stopping = false
		s.mu.Unlock()
		return err
	}

	httpServer := s.server
	listener := s.listener
	serveDone := s.serveDone
	requests := s.requests
	s.stopping = true
	s.mu.Unlock()

	var stopErr error
	s.recordOp("shutdown")
	shutdownErr := httpServer.Shutdown(ctx)
	if shutdownErr != nil {
		stopErr = errors.Join(stopErr, errs.WrapTransient(shutdownErr, "Server", "Stop",
			"gracefully shut down metrics server"))
	}
	completed := false
	if shutdownErr == nil {
		select {
		case err := <-serveDone:
			completed = true
			stopErr = errors.Join(stopErr, classifyServeError(err))
		case <-ctx.Done():
		}
		// Shutdown returns nil on an idle server even when ctx has ended, and when both
		// cases above are ready the select picks one at random; the caller's ended
		// context is reported here, never left to that pick.
		if err := ctx.Err(); err != nil {
			stopErr = errors.Join(stopErr, err)
		}
	}

	if shutdownErr != nil || !completed {
		s.recordOp("close server")
		if err := httpServer.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			stopErr = errors.Join(stopErr, errs.WrapTransient(err, "Server", "Stop",
				"force close metrics HTTP server"))
		}
		if listener != nil {
			s.recordOp("close listener")
			if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				stopErr = errors.Join(stopErr, errs.WrapTransient(err, "Server", "Stop",
					"force close metrics listener"))
			}
		}

		// Serve returns once its listener is closed, which happened just above, so the
		// join needs no timer of its own.
		stopErr = errors.Join(stopErr, classifyServeError(<-serveDone))
	}

	// Serve has returned, but a force-closed connection does not stop a handler that is
	// still collecting; Stop waits for admitted requests within ctx.
	if requests != nil {
		if err := requests.wait(ctx); err != nil && !errors.Is(stopErr, err) {
			stopErr = errors.Join(stopErr, err)
		}
	}

	s.mu.Lock()
	s.server = nil
	s.listener = nil
	s.serveDone = nil
	s.stopping = false
	s.mu.Unlock()
	return stopErr
}

func classifyServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return errs.WrapTransient(err, "Server", "Stop", "metrics server exited with an error")
}

// Address reports the accepted listener's endpoint while Server owns it.
// Before startup and after completed Stop, it reports the configured endpoint.
// The address does not assert that Serve is ready to accept requests.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	host := "localhost"
	port := s.port
	if s.listener != nil {
		if addr, ok := s.listener.Addr().(*net.TCPAddr); ok {
			port = addr.Port
			if addr.IP != nil && !addr.IP.IsUnspecified() {
				host = addr.IP.String()
				if addr.Zone != "" {
					host += "%" + addr.Zone
				}
			}
		}
	}
	scheme := "http"
	if s.security.TLS.Server.Enabled {
		scheme = "https"
	}
	return (&url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: s.path}).String()
}

func (s *Server) recordOp(op string) {
	if s.opHook != nil {
		s.opHook(op)
	}
}

// admittedRequests counts the requests a Server's handler has admitted and not yet
// returned from, so Stop can wait for them after a forced close.
type admittedRequests struct {
	mu     sync.Mutex
	active int
	idle   chan struct{} // closed when active returns to zero; nil while none run
}

func (r *admittedRequests) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.begin()
		defer r.end()
		next.ServeHTTP(w, req)
	})
}

func (r *admittedRequests) begin() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == 0 {
		r.idle = make(chan struct{})
	}
	r.active++
}

func (r *admittedRequests) end() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
	if r.active == 0 {
		close(r.idle)
		r.idle = nil
	}
}

func (r *admittedRequests) pending() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

// wait returns nil once no admitted request is running, or ctx's error if ctx
// ends first.
func (r *admittedRequests) wait(ctx context.Context) error {
	for {
		r.mu.Lock()
		idle := r.idle
		r.mu.Unlock()
		if idle == nil {
			return nil
		}
		select {
		case <-idle:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
