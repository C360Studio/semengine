package graphgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/httpadmission"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// key names graph-gateway once: its registration and its logger.
const key = "graph-gateway"

// stopping is the answer to a request before Start and once Stop begins (design D6).
const stopping = "graph-gateway is stopping"

// Component implements graph-gateway. It holds no NATS state and runs no goroutine of its own:
// each admitted HTTP request makes its one NATS request on the handler's goroutine (design D5).
type Component struct {
	config  Config
	outputs []component.Port

	natsClient *natsclient.Client
	resolvers  map[string]resolver

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle

	// requests admits HTTP requests; cleanup closes it and waits for those admitted.
	requests httpadmission.Requests
}

// DeclarePorts is the component.PortDeclarer for graph-gateway: the ports CreateGraphGateway will
// report for rawConfig, computed without dependencies.
func DeclarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(nil, outputs), nil
}

// CreateGraphGateway is the factory function for creating graph-gateway components. It opens no
// connection. Each root field's subject comes from graph.QueryVerbs; a root field with no declared
// verb is refused here.
func CreateGraphGateway(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphGateway", "factory", "NATSClient required")
	}
	config, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	c := &Component{config: config, outputs: outputs, natsClient: deps.NATSClient}
	resolvers, err := c.rootResolvers()
	if err != nil {
		return nil, errs.WrapFatal(err, "CreateGraphGateway", "factory", "resolve root fields")
	}
	c.resolvers = resolvers
	return c, nil
}

// Register registers the graph-gateway factory with the component registry.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        key,
		Type:        "gateway",
		Protocol:    "http",
		Domain:      "graph",
		Description: "Read-only GraphQL gateway over the graph.query.* verbs",
		Version:     "1.0.0",
		Schema:      configSchema,
		Factory:     CreateGraphGateway,
		Ports:       DeclarePorts,
	})
}

// InputPorts returns no ports: graph-gateway has none.
func (c *Component) InputPorts() []component.Port {
	return nil
}

// OutputPorts returns a new slice of the output ports. The ports share their Interface pointer with
// the component; no code writes through it.
func (c *Component) OutputPorts() []component.Port {
	return slices.Clone(c.outputs)
}

// Health reports the shell's verdict (component.Lifecycle.Health).
func (c *Component) Health() component.HealthStatus {
	return c.lifecycle.Health()
}

// DataFlow reports the shell's flow metrics (component.Lifecycle.DataFlow).
func (c *Component) DataFlow() component.FlowMetrics {
	return c.lifecycle.DataFlow()
}

// Initialize marks the component initialized (no I/O); the factory validated the configuration.
func (c *Component) Initialize() error {
	return c.lifecycle.Initialize()
}

// Start makes the gateway answer requests. It acquires nothing: the lifecycle state is the shell's
// (component.Lifecycle), so Start before Initialize, a nil or ended context and a second Start are
// refused. A component with no NATS client is refused first.
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, func(context.Context) error { return nil }, c.cleanup)
}

// Stop stops the component through the shell, which runs cleanup: Stop before Start does nothing,
// and a Stop whose context ends first returns that context's error and a later Stop waits again.
func (c *Component) Stop(ctx context.Context) error {
	return c.lifecycle.Stop(ctx)
}

// cleanup refuses every later request, then waits until no admitted request runs, or returns ctx's
// error if it ends first.
func (c *Component) cleanup(ctx context.Context) error {
	c.requests.CloseAdmission()
	return c.requests.Wait(ctx)
}

// RegisterHTTPHandlers mounts the GraphQL route at <prefix>/graphql. The service manager calls it
// with /<instance> (service/service_manager_http.go).
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	mux.Handle(strings.TrimSuffix(prefix, "/")+"/graphql", c.requests.Wrap(http.HandlerFunc(c.serveGraphQL), stopping))
}

// serveGraphQL answers one admitted request with the HTTP status design D6 assigns it.
func (c *Component) serveGraphQL(w http.ResponseWriter, r *http.Request) {
	// No stuck state is ever passed in, so Healthy is false exactly while the shell is not running:
	// before Start, and from the moment cleanup begins, after which admission is closed too.
	if !c.lifecycle.Health().Healthy {
		w.Header().Set("Connection", "close")
		http.Error(w, stopping, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeResponse(w, http.StatusMethodNotAllowed, response{Errors: refuse("method not allowed: %s", r.Method)})
		return
	}
	c.lifecycle.CountInput(max(0, int(r.ContentLength)))
	var req request
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		writeResponse(w, http.StatusBadRequest, response{Errors: refuse("request body: %v", err)})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeResponse(w, http.StatusBadRequest, response{Errors: refuse("request body: data after the request object")})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), c.timeout())
	defer cancel()
	resp, refused := execute(ctx, c.resolvers, req)
	if refused {
		writeResponse(w, http.StatusBadRequest, resp)
		return
	}
	status := c.classify(resp.Errors)
	if status == http.StatusBadRequest {
		resp.Data = nil
	}
	writeResponse(w, status, resp)
}

func (c *Component) timeout() time.Duration {
	return time.Duration(c.config.QueryTimeoutMs) * time.Millisecond
}

// classify fills each resolver error's extensions with its class and code (ADR-060) and returns the
// HTTP status of the answer: 400 for an argument the schema cannot refuse, 504 for a request past
// query_timeout_ms, 500 for any other request that got no answer, else 200.
func (c *Component) classify(list gqlerror.List) int {
	status := http.StatusOK
	for _, e := range list {
		var refusal *errs.ClassifiedError
		if errors.As(e.Err, &refusal) {
			e.Extensions = map[string]any{"class": refusal.Class.String()}
			if refusal.Code != "" {
				e.Extensions["code"] = refusal.Code
			}
		}
		var argument *argumentError
		var missing *noAnswer
		switch {
		case errors.As(e.Err, &argument):
			status = http.StatusBadRequest
		case errors.As(e.Err, &missing) && errors.Is(e.Err, context.DeadlineExceeded):
			c.lifecycle.CountError()
			e.Message = "request timeout"
			status = http.StatusGatewayTimeout
		case errors.As(e.Err, &missing):
			c.lifecycle.CountError()
			status = http.StatusInternalServerError
		}
	}
	return status
}

func writeResponse(w http.ResponseWriter, status int, resp response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status is sent; an encoding or write failure here has no one left to tell.
	_ = json.NewEncoder(w).Encode(resp)
}
