// Package websocket is the websocket output component: a server that streams applied ENTITY_STATES,
// or one core NATS subject, to WebSocket clients. Register is its only export.
package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/httpadmission"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// key names the websocket output once: its registration, its logger and its metrics.
const key = "websocket"

// stopping is the answer to an upgrade once Stop has begun.
const stopping = "websocket output is stopping"

// errNotBuilt refuses an applied-state configuration, which passed every check, because the input
// that would feed it does not exist yet.
//
// shortcut: the kv-watch input of ENTITY_STATES is refused, so no composition admits an instance
// that never streams; task 3.3 of alpha4-output-websocket replaces this with the applied-state view.
var errNotBuilt = errors.New("websocket output: the applied-state input is not built yet (alpha4-output-websocket task 3.3)")

// errTLSNotBuilt refuses a deployment that enables TLS, so the output never serves plain HTTP where
// TLS was asked for.
//
// shortcut: TLS is refused until task 3.2 of alpha4-output-websocket loads it at Start.
var errTLSNotBuilt = errors.New("websocket output: TLS is not built yet (alpha4-output-websocket task 3.2)")

var _ component.LifecycleComponent = (*output)(nil)

// output is the websocket output component: an HTTP server whose upgrade path streams frames to
// every connected client (design D2, D3).
type output struct {
	settings   settings
	natsClient *natsclient.Client
	logger     *slog.Logger
	// listener is the output's host:port, the label its metrics and warnings carry.
	listener string
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
	if s.subject == "" {
		return nil, errs.Wrap(errNotBuilt, key, "factory", "build")
	}
	if deps.Security.TLS.Server.Enabled {
		return nil, errs.Wrap(errTLSNotBuilt, key, "factory", "build")
	}
	network := s.output.Config.(component.NetworkPort) // resolveOutput accepts only a network port
	listener := net.JoinHostPort(network.Host, strconv.Itoa(network.Port))
	logger := deps.GetLoggerWithComponent(key)
	dropped, err := newDroppedCounter(deps.MetricsRegistry)
	if err != nil {
		return nil, errs.Wrap(err, key, "factory", "register metrics")
	}
	return &output{
		settings:   s,
		natsClient: deps.NATSClient,
		logger:     logger,
		listener:   listener,
		drops:      newDrops(listener, logger, dropped),
		// Every origin is accepted, as the pin did: the output has no authentication (#84).
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

// Health reports the shell's verdict (component.Lifecycle.Health).
func (c *output) Health() component.HealthStatus { return c.lifecycle.Health() }

// DataFlow reports input messages: one per subject message received, with its length.
func (c *output) DataFlow() component.FlowMetrics { return c.lifecycle.DataFlow() }

// Initialize marks the component initialized (no I/O); the factory validated the configuration.
func (c *output) Initialize() error { return c.lifecycle.Initialize() }

// Start binds the listener, subscribes to the input subject and serves upgrades. The lifecycle
// state is the shell's (component.Lifecycle): Start before Initialize, a nil or ended context and a
// second Start are refused, and a Start that fails is rolled back through cleanup, which releases
// the port. A component with no NATS client is refused first.
//
// ctx bounds the bind and the subscribe and is not kept: ending it after Start returned stops
// nothing (ADR-113). Nothing that continues after Start takes a context: the server, each client's
// reader and writer, and the subscription's handler end only through Stop.
func (c *output) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, key, "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.cleanup)
}

// start binds, subscribes, then serves, recording each handle as it is acquired so cleanup
// releases a partial start.
func (c *output) start(ctx context.Context) error {
	var config net.ListenConfig
	listen, err := config.Listen(ctx, "tcp", c.listener)
	if err != nil {
		return errs.WrapFatal(err, key, "Start", "bind "+c.listener)
	}
	c.listen = listen
	sub, err := c.natsClient.Subscribe(ctx, c.settings.subject, c.receive)
	if err != nil {
		return errs.Wrap(err, key, "Start", "subscribe to "+c.settings.subject)
	}
	c.sub = sub
	// A Start context that ended during setup fails Start here, before anything is served
	// (ADR-113).
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, key, "Start", "context ended during startup")
	}

	mux := http.NewServeMux()
	mux.HandleFunc(c.settings.path, c.serveUpgrade)
	c.server = &http.Server{Handler: c.requests.Wrap(mux, stopping)}
	served := make(chan struct{})
	c.served = served
	go func(server *http.Server) {
		defer close(served)
		if err := server.Serve(listen); !errors.Is(err, http.ErrServerClosed) {
			c.lifecycle.CountError()
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
// the server and every client handler have returned. A ctx that ends first returns its error with
// the handles kept, and the next Stop resumes.
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
	c.listen, c.sub, c.server, c.served = nil, nil, nil, nil
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
	return true
}

func (c *output) detach(cl *client) {
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()
	delete(c.clients, cl)
}

// receive is the subscription's handler: a message that is valid JSON goes to every client as a
// data frame; any other is dropped, counted and logged (design D2, "Declared drops").
func (c *output) receive(_ context.Context, msg *nats.Msg) {
	c.lifecycle.CountInput(len(msg.Data))
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
