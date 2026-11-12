package websocket

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/httpadmission"
	"github.com/c360studio/semengine/internal/tlsutil"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/security"
)

// key names the websocket output once: its registration, its logger and its metrics.
const key = "websocket"

// stopping is the answer to an upgrade once Stop has begun.
const stopping = "websocket output is stopping"

var _ component.LifecycleComponent = (*output)(nil)

// output is the websocket output component: an HTTP server whose upgrade path streams frames to
// every connected client.
type output struct {
	settings   settings
	natsClient *natsclient.Client
	logger     *slog.Logger
	// listener is the output's host:port, the label its metrics and warnings carry.
	listener string
	// tls is the deployment's server TLS configuration (Dependencies.Security), loaded at Start.
	tls      security.ServerTLSConfig
	metrics  *outputMetrics
	drops    *drops
	upgrader websocket.Upgrader

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle

	// requests admits upgrade requests; an upgraded client's handler runs until the client ends,
	// so cleanup's wait on requests is the join of every client.
	requests httpadmission.Requests

	// clientsMu guards clients and closing. closing is set by cleanup; a client attaching after it
	// is closed at once, so no client outlives Stop.
	clientsMu sync.Mutex
	clients   map[*client]struct{}
	closing   bool

	// The handles start records for cleanup, which clears them once it has released them all.
	// served closes once the server's Serve has returned.
	listen net.Listener
	sub    *natsclient.Subscription
	server *http.Server
	served chan struct{}

	// Applied state only. runCancel ends the one run context the view supervisor runs under
	// (ADR-113), supervised is closed once the supervisor has returned, and supervisorErr is what it
	// returned. view is the view upgrades attach to, nil while there is none; applied is its
	// readiness, which Health reports.
	runCancel     context.CancelFunc
	supervised    chan struct{}
	supervisorErr error
	view          atomic.Pointer[stateView]
	applied       atomic.Int32
}

// declarePorts is the component.PortDeclarer for the websocket output: the ports the factory
// resolves for rawConfig, computed without dependencies.
func declarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	s, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom([]component.Port{s.input}, []component.Port{s.output}), nil
}

// newSettings is the factory's derivation: the resolved configuration, refusing a nil NATS client.
func newSettings(rawConfig json.RawMessage, deps component.Dependencies) (settings, error) {
	if deps.NATSClient == nil {
		return settings{}, errs.WrapInvalid(errs.ErrInvalidConfig, key, "factory", "NATS client required")
	}
	return resolveConfig(rawConfig)
}

// create is the factory function for websocket outputs. It opens no connection and binds no port.
func create(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	s, err := newSettings(rawConfig, deps)
	if err != nil {
		return nil, err
	}
	network := s.output.Config.(component.NetworkPort) // resolveOutput accepts only a network port
	listener := net.JoinHostPort(network.Host, strconv.Itoa(network.Port))
	logger := deps.GetLoggerWithComponent(key)
	metrics, err := newOutputMetrics(deps.MetricsRegistry, listener)
	if err != nil {
		return nil, errs.Wrap(err, key, "factory", "register metrics")
	}
	return &output{
		settings:   s,
		natsClient: deps.NATSClient,
		logger:     logger,
		listener:   listener,
		tls:        deps.Security.TLS.Server,
		metrics:    metrics,
		drops:      newDrops(listener, logger, metrics),
		// Every origin is accepted: the output has no authentication.
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		clients:  map[*client]struct{}{},
	}, nil
}

// Register registers the websocket output factory with the component registry.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        key,
		Type:        "output",
		Protocol:    "websocket",
		Domain:      "network",
		Description: "WebSocket server streaming applied ENTITY_STATES or a core NATS subject to clients",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     create,
		Ports:       declarePorts,
	})
}

// InputPorts returns the one input port, fixed at construction.
func (c *output) InputPorts() []component.Port { return []component.Port{c.settings.input} }

// OutputPorts returns the one http network port the server listens on.
func (c *output) OutputPorts() []component.Port { return []component.Port{c.settings.output} }

// Health reports the shell's verdict (component.Lifecycle.Health), over the applied-state view's
// stuck state: catching up, or its watch lost and reopening.
func (c *output) Health() component.HealthStatus {
	if c.settings.subject != "" {
		return c.lifecycle.Health()
	}
	return c.lifecycle.Health(c.appliedStuck())
}

// DataFlow reports input messages: one per subject message received, or per applied-state write
// the view decodes, with its length.
func (c *output) DataFlow() component.FlowMetrics { return c.lifecycle.DataFlow() }

// Initialize marks the component initialized (no I/O); the factory validated the configuration.
func (c *output) Initialize() error { return c.lifecycle.Initialize() }

// Start loads TLS when the deployment enables it, binds the listener, subscribes to the input
// subject or starts the applied-state view supervisor, and serves upgrades, over TLS when it was
// loaded. The lifecycle
// state is the shell's (component.Lifecycle): Start before Initialize, a nil or ended context and a
// second Start are refused, and a Start that fails is rolled back through cleanup, which releases
// the port. A component with no NATS client is refused first.
//
// ctx bounds the bind and the subscribe and is not kept: ending it after Start returned stops
// nothing (ADR-113). The view supervisor runs under the one run context Start creates and Stop
// ends; the server, each client's reader and writer, and the subscription's handler end only
// through Stop.
func (c *output) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, key, "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.cleanup)
}

// start loads TLS, binds, subscribes, then serves, recording each handle as it is acquired so
// cleanup releases a partial start.
func (c *output) start(ctx context.Context) error {
	// LoadServerTLSConfig returns nil when server TLS is disabled, applies mTLS when it is enabled,
	// and refuses what this module does not build, such as an "acme" mode, naming the key
	// (TestACMEModeRefusedAtStart). Loading first means a refusal binds nothing.
	tlsConfig, err := tlsutil.LoadServerTLSConfig(c.tls)
	if err != nil {
		return errs.Wrap(err, key, "Start", "load TLS")
	}
	var config net.ListenConfig
	listen, err := config.Listen(ctx, "tcp", c.listener)
	if err != nil {
		return errs.WrapFatal(err, key, "Start", "bind "+c.listener)
	}
	c.listen = listen
	if c.settings.subject != "" {
		sub, err := c.natsClient.Subscribe(ctx, c.settings.subject, c.receive)
		if err != nil {
			return errs.Wrap(err, key, "Start", "subscribe to "+c.settings.subject)
		}
		c.sub = sub
	} else {
		c.applied.Store(appliedCatchingUp)
		// The one run context: Start's values without its cancellation, ended only by cleanup.
		runCtx, runCancel := context.WithCancel(context.WithoutCancel(ctx))
		supervised := make(chan struct{})
		c.runCancel, c.supervised = runCancel, supervised
		go func() {
			defer close(supervised)
			c.supervisorErr = c.superviseView(runCtx)
		}()
	}
	// A Start context that ended during setup fails Start here, before anything is served
	// (ADR-113).
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, key, "Start", "context ended during startup")
	}

	mux := http.NewServeMux()
	mux.HandleFunc(c.settings.path, c.serveUpgrade)
	c.server = &http.Server{Handler: c.requests.Wrap(mux, stopping)}
	if tlsConfig != nil {
		// A TLS listener rather than ServeTLS, which net/http documents as enabling HTTP/2: the
		// upgrade is an HTTP/1.1 request, and over this listener the server speaks HTTP/1.1 only.
		listen = tls.NewListener(listen, tlsConfig)
	}
	served := make(chan struct{})
	c.served = served
	go func(server *http.Server) {
		defer close(served)
		if err := server.Serve(listen); !errors.Is(err, http.ErrServerClosed) {
			c.lifecycle.CountError()
			c.metrics.errors.WithLabelValues(c.listener, errorServe).Inc()
			c.logger.Error("websocket server stopped serving; Stop releases the port",
				slog.String("listener", c.listener), slog.Any("error", err))
		}
	}(c.server)
	return nil
}

// Stop stops the component through the shell, which runs cleanup: Stop before Start does nothing,
// and a Stop whose context ends first returns that context's error and keeps what is left for the
// next Stop.
func (c *output) Stop(ctx context.Context) error { return c.lifecycle.Stop(ctx) }

// cleanup releases what start acquired, for a Stop or a failed Start's rollback, tolerating a
// partial start. It refuses later upgrades with 503, stops the server so the port is released,
// drains the subscription so nothing more is sent, sends every client close 1001, then waits until
// the server and every client handler have returned. Only then does it end the run context and wait
// for the view supervisor, so a client is closed by the stop, never by the view ending under it. A
// ctx that ends first returns its error with the handles kept, and the next Stop resumes. A view
// that could not stop its watch is reported by the Stop that joined its supervisor only, as
// graph-query does: there is nothing left to retry.
func (c *output) cleanup(ctx context.Context) error {
	c.requests.CloseAdmission()
	if c.server != nil {
		// Close stops Serve and closes the connections not upgraded. net/http documents that it
		// does not close a hijacked connection: an upgraded client is its handler's, which
		// closeClients asks to close (TestStartContextEndingStopsNothing sees its close 1001).
		if err := c.server.Close(); err != nil {
			return err
		}
	}
	if c.sub != nil {
		if err := c.sub.Drain(ctx); err != nil {
			return err
		}
	}
	c.closeClients()
	if c.served != nil {
		select {
		case <-c.served:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if c.listen != nil {
		// Serve closes the listener it was given; closing it here as well releases the port after
		// a partial start, which never served.
		if err := c.listen.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
	}
	if err := c.requests.Wait(ctx); err != nil {
		return err
	}
	if c.runCancel != nil {
		c.runCancel()
	}
	if c.supervised != nil {
		select {
		case <-c.supervised:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// Read after supervised closed.
	viewErr := c.supervisorErr
	c.listen, c.sub, c.server, c.served = nil, nil, nil, nil
	c.runCancel, c.supervised, c.supervisorErr = nil, nil, nil
	if viewErr != nil {
		return errs.Wrap(viewErr, key, "Stop", "stop the applied-state view")
	}
	return nil
}

// closeClients fences later attaches and asks every attached client to close with 1001.
func (c *output) closeClients() {
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()
	c.closing = true
	for cl := range c.clients {
		cl.stop()
	}
}

// attach adds cl to the clients frames fan out to, or reports false once cleanup has begun.
func (c *output) attach(cl *client) bool {
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()
	if c.closing {
		return false
	}
	c.clients[cl] = struct{}{}
	c.metrics.connected.Set(float64(len(c.clients)))
	return true
}

// detach counts cl's disconnection under the reason its connection ended, then removes it from
// the clients, if it was attached.
func (c *output) detach(cl *client) {
	c.metrics.disconnections.WithLabelValues(c.listener, cl.endReason).Inc()
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()
	delete(c.clients, cl)
	c.metrics.connected.Set(float64(len(c.clients)))
}

// receive is the subscription's handler: a message that is valid JSON goes to every client as a
// data frame; any other is dropped, counted and logged.
func (c *output) receive(_ context.Context, msg *nats.Msg) {
	c.lifecycle.CountInput(len(msg.Data))
	c.metrics.received.Inc()
	if !json.Valid(msg.Data) {
		c.drops.record(reasonNotJSON, 1,
			"subject message is not JSON and was not sent; later messages still flow",
			slog.String("subject", msg.Subject))
		return
	}
	frame := dataFrame(msg.Subject, msg.Data)
	full := 0
	c.clientsMu.Lock()
	for cl := range c.clients {
		if cl.enqueue(frame) {
			full++
		}
	}
	c.clientsMu.Unlock()
	if full > 0 {
		c.drops.record(reasonClientQueueFull, full,
			"a client is not reading and its oldest queued frame was dropped; other clients and later frames are unaffected")
	}
}
