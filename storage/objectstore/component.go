package objectstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage"
	"github.com/c360studio/semengine/types"
)

// instanceName is the one StorageInstance the Component stamps into every reference and offers
// its Store under, and the instance of its store-provide port. The factory receives no instance
// name, and consumers resolve references by this literal; a second Component in one process is
// refused at Start by the store registry (design D7, owner ruling 3).
const instanceName = "objectstore"

// storedPort is the one output the Component publishes on: a StoredMessage per stored body.
const storedPort = "stored"

// componentConfig is the Component's configuration. It is decoded strictly: any other key is
// refused, naming it.
type componentConfig struct {
	// BucketName is the NATS ObjectStore bucket. Empty means "MESSAGES".
	BucketName string `json:"bucket_name" schema:"type:string,description:NATS ObjectStore bucket name,default:MESSAGES,category:basic"`

	// Ports are the input and output ports. Absent means the default write input and stored
	// output; present means exactly the ports written.
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration for inputs and outputs,category:basic"`
}

// schema is the configuration schema Register declares.
var schema = component.GenerateConfigSchema(reflect.TypeOf(componentConfig{}))

// defaultPorts returns the ports of a configuration with no ports key: the write input on core
// NATS and the stored output carrying storage.stored.v1.
func defaultPorts() *component.PortConfig {
	return &component.PortConfig{
		Inputs: []component.PortDefinition{{
			Name:        "write",
			Config:      component.NATSPort{Subject: "storage.objectstore.write"},
			Description: "Messages whose ContentStorable body is stored",
		}},
		Outputs: []component.PortDefinition{{
			Name: storedPort,
			Config: component.NATSPort{
				Subject:   "storage.objectstore.stored",
				Interface: &component.InterfaceContract{Type: "storage.stored.v1"},
			},
			Description: "A StoredMessage carrying each stored body's reference",
		}},
	}
}

// Component stores the bodies of the messages on its inputs in a NATS ObjectStore bucket and
// publishes a StoredMessage carrying each body's reference on its stored output (ADR-063).
//
// The write lane comes with task 3.4; until then each input counts and settles a delivery
// without storing it.
type Component struct {
	config   componentConfig
	inputs   []component.Port
	outputs  []component.Port
	bindings []inputBinding

	natsClient      *natsclient.Client
	metricsRegistry *metric.MetricsRegistry
	logger          *slog.Logger

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle

	// store is the Store start opens, offered by ProvidedStores, which may be called while Start
	// or Stop runs.
	store atomic.Pointer[Store]

	// The handles start records for release, which clears them only after a cleanup that
	// succeeded. Both run under the lifecycle guard, one at a time, so no lock is needed; no other
	// goroutine reads them: the input check closes its own checkDone.
	cancel        context.CancelFunc
	subscriptions []coreSubscription
	consumers     []consumerBinding
	checkDone     chan struct{}

	// inputFault is Health's stuck state for the JetStream inputs, written by each input check
	// that runs to its end (checkInputs); nil when that check found every input bound.
	inputFault atomic.Pointer[string]
}

// coreSubscription is a bound core-NATS input: natsclient's Subscription.
type coreSubscription interface {
	Drain(context.Context) error
}

// consumerBinding is a bound JetStream input's consume loop, and whether cleanup has issued its
// Drain, so a cleanup that runs again waits on the same drain instead of issuing another.
type consumerBinding struct {
	handle      jetstream.ConsumeContext
	drainIssued bool
}

var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
	_ component.StoreProvider      = (*Component)(nil)
)

// Register registers the objectstore factory with registry.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        "objectstore",
		Type:        types.ComponentTypeStorage,
		Protocol:    "objectstore",
		Domain:      "storage",
		Description: "Stores message bodies in a NATS ObjectStore bucket and publishes their references",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     newComponent,
		Ports:       declarePorts,
	})
}

// declarePorts is the component.PortDeclarer for objectstore: the ports newComponent builds for
// rawConfig. The instance name is not used: the store-provide port always names instanceName.
func declarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(inputs, outputs), nil
}

// newComponent is the objectstore factory. It validates the configuration, resolves the ports
// and plans how Start binds each input; it opens no connection.
func newComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	bindings, err := planInputBindings(inputs)
	if err != nil {
		return nil, errs.WrapInvalid(err, "objectstore", "newComponent", "plan input bindings")
	}
	return &Component{
		config:          config,
		inputs:          inputs,
		outputs:         outputs,
		bindings:        bindings,
		natsClient:      deps.NATSClient,
		metricsRegistry: deps.MetricsRegistry,
		logger:          deps.GetLogger(),
	}, nil
}

// resolveConfig decodes rawConfig strictly, applies the defaults and resolves the ports. It is the
// one derivation declarePorts and newComponent share.
func resolveConfig(rawConfig json.RawMessage) (componentConfig, []component.Port, []component.Port, error) {
	var config componentConfig
	if len(rawConfig) > 0 {
		// A port definition decodes through its own UnmarshalJSON, which this setting does not
		// reach; it refuses unknown keys itself.
		decoder := json.NewDecoder(bytes.NewReader(rawConfig))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			return componentConfig{}, nil, nil, errs.WrapInvalid(err, "objectstore", "newComponent", "config unmarshal")
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return componentConfig{}, nil, nil, errs.WrapInvalid(errors.New("data after the configuration object"),
				"objectstore", "newComponent", "config unmarshal")
		}
	}
	if config.BucketName == "" {
		config.BucketName = defaultBucketName
	}
	if config.Ports == nil {
		config.Ports = defaultPorts()
	}
	inputs, outputs, err := resolvePorts(*config.Ports)
	if err != nil {
		return componentConfig{}, nil, nil, errs.WrapInvalid(err, "objectstore", "newComponent", "resolve ports")
	}
	return config, inputs, outputs, nil
}

// resolvePorts resolves the configured ports and appends the store-provide output. Each port is
// core NATS or JetStream on one subject, and names are unique. The only output is stored, and an
// input needs it: a body stored with no reference published is addressable by nobody (design D3).
func resolvePorts(ports component.PortConfig) ([]component.Port, []component.Port, error) {
	seen := make(map[string]bool, len(ports.Inputs)+len(ports.Outputs))
	resolve := func(definitions []component.PortDefinition, direction component.Direction) ([]component.Port, error) {
		resolved := make([]component.Port, 0, len(definitions))
		for _, definition := range definitions {
			if direction == component.DirectionInput && definition.Name == "api" {
				return nil, errors.New(`input "api" was removed; read through the registered Store`)
			}
			if direction == component.DirectionOutput && definition.Name != storedPort {
				return nil, fmt.Errorf("output %q is not supported: the only output is %q", definition.Name, storedPort)
			}
			port, err := definition.Resolve(direction)
			if err != nil {
				return nil, err
			}
			if seen[port.Name] {
				return nil, fmt.Errorf("duplicate port name %q", port.Name)
			}
			seen[port.Name] = true
			facts, err := port.Facts()
			if err != nil {
				return nil, err
			}
			if kind := facts.Kind(); kind != component.PortKindNATS && kind != component.PortKindJetStream {
				return nil, fmt.Errorf("port %q kind %q is not supported", port.Name, kind)
			}
			if subjects := facts.NATSSubjects(); len(subjects) != 1 {
				return nil, fmt.Errorf("port %q declares %d subjects, want one", port.Name, len(subjects))
			}
			resolved = append(resolved, port)
		}
		return resolved, nil
	}
	inputs, err := resolve(ports.Inputs, component.DirectionInput)
	if err != nil {
		return nil, nil, err
	}
	outputs, err := resolve(ports.Outputs, component.DirectionOutput)
	if err != nil {
		return nil, nil, err
	}
	if len(inputs) > 0 && len(outputs) == 0 {
		return nil, nil, fmt.Errorf("inputs need the %q output, which publishes each stored body's reference", storedPort)
	}
	provider, err := component.PortDefinition{
		Name:        "store-provide",
		Description: "Owns the store addressable as StorageInstance=" + instanceName,
		Config:      component.StoreProvidePort{Instance: instanceName},
	}.Resolve(component.DirectionOutput)
	if err != nil {
		return nil, nil, err
	}
	return inputs, append(outputs, provider), nil
}

// InputPorts returns the input ports, fixed at construction.
func (c *Component) InputPorts() []component.Port {
	return slices.Clone(c.inputs)
}

// OutputPorts returns the output ports, fixed at construction: stored, if configured, and
// store-provide.
func (c *Component) OutputPorts() []component.Port {
	return slices.Clone(c.outputs)
}

// Health reports the lifecycle shell's verdict over the Component's one stuck state: a JetStream
// input whose consume loop ended while running, which only a process restart clears, or whose
// consumer the latest input check could not find (#216's ruling; checkInputs). ErrorCount is
// reported and never lowers Healthy.
func (c *Component) Health() component.HealthStatus {
	var fault string
	if f := c.inputFault.Load(); f != nil {
		fault = *f
	}
	return c.lifecycle.Health(fault)
}

// DataFlow reports the lifecycle shell's counts.
func (c *Component) DataFlow() component.FlowMetrics {
	return c.lifecycle.DataFlow()
}

// ProvidedStores offers the running Component's Store under the instance name it stamps into
// every reference, for the component manager to register (ADR-063). It returns nil before Start
// and after Stop.
func (c *Component) ProvidedStores() map[string]storage.StreamableStore {
	store := c.store.Load()
	if store == nil {
		return nil
	}
	return map[string]storage.StreamableStore{store.InstanceName(): store}
}

// Initialize marks the Component initialized (no I/O). The factory validated the configuration.
func (c *Component) Initialize() error {
	return c.lifecycle.Initialize()
}

// Start opens the Store and binds every input, through the lifecycle shell: a nil or ended
// context and a second Start are refused before anything is acquired, and a Start that fails is
// rolled back through release; a rollback that fails leaves release pending for the next Stop. A
// Component with no NATS client is refused before the shell is consulted.
//
// ctx bounds Start only: opening the bucket, waiting for each JetStream input's stream and binding
// the inputs run under it, and a ctx that ends first fails Start. The inputs' handlers run under
// contexts natsclient owns, and the input check under a run context start creates and release
// ends, so ending ctx after Start returned stops nothing (ADR-113).
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "objectstore", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.release)
}

// start acquires everything Start owns, under the guard. Each handle is recorded as it is
// acquired, so release can return whatever a failed start left behind.
func (c *Component) start(ctx context.Context) error {
	// The run context: Start's values without its cancellation, ended only by release.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c.cancel = cancel

	store, err := NewStoreWithConfigAndMetrics(ctx, c.natsClient, Config{
		BucketName:   c.config.BucketName,
		InstanceName: instanceName,
		Logger:       c.logger,
	}, c.metricsRegistry)
	if err != nil {
		// The Store's class is kept: a bucket whose retention it cannot strip is fatal.
		return errs.Wrap(err, "objectstore", "Start", "open bucket "+c.config.BucketName)
	}
	c.store.Store(store)

	var watched []watchedInput
	for _, binding := range c.bindings {
		if binding.kind == component.PortKindNATS {
			subscription, err := c.natsClient.Subscribe(ctx, binding.subject, c.handleCore)
			if err != nil {
				return errs.WrapTransient(err, "objectstore", "Start",
					fmt.Sprintf("subscribe input %s to %s", binding.port.Name, binding.subject))
			}
			c.subscriptions = append(c.subscriptions, subscription)
			continue
		}
		handle, err := c.bindJetStream(ctx, binding)
		if err != nil {
			return errs.Wrap(err, "objectstore", "Start", "bind input "+binding.port.Name)
		}
		c.consumers = append(c.consumers, consumerBinding{handle: handle})
		watched = append(watched, watchedInput{
			port: binding.port.Name, stream: binding.stream, consumer: binding.consumer, closed: handle.Closed(),
		})
	}
	// A Start context that ends after the last binding succeeded fails Start here (ADR-113: if
	// ctx ends before Start returns, Start returns its error and holds nothing); the guard's
	// rollback then runs release.
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "objectstore", "Start", "context ended during startup")
	}

	// The last step: nothing between here and the guard's commit can fail. A Component with no
	// JetStream input has nothing the check observes and runs none.
	if len(watched) > 0 {
		checkDone := make(chan struct{})
		c.checkDone = checkDone
		go func() {
			defer close(checkDone)
			c.watchInputs(runCtx, watched)
		}()
	}
	return nil
}

// bindJetStream waits for binding's stream and binds its durable consumer. The consumer takes
// the port's consumer settings and delivers from the start of the stream unless the port says
// otherwise, so a body published before the Component started is still stored.
func (c *Component) bindJetStream(ctx context.Context, binding inputBinding) (jetstream.ConsumeContext, error) {
	if err := c.waitForStream(ctx, binding.stream); err != nil {
		return nil, fmt.Errorf("stream %s not available: %w", binding.stream, err)
	}
	declared, err := component.GetConsumerConfig(binding.port)
	if err != nil {
		return nil, errs.WrapInvalid(err, "objectstore", "bindJetStream", "resolve consumer config")
	}
	facts, err := binding.port.Facts()
	if err != nil {
		return nil, errs.WrapInvalid(err, "objectstore", "bindJetStream", "project input port facts")
	}
	if stream, _ := facts.Stream(); stream.DeliverPolicy() == "" {
		declared.DeliverPolicy = "all"
	}
	handle, err := c.natsClient.ConsumeStreamWithConfig(ctx,
		natsclient.PortConsumerContext{Component: instanceName, Port: binding.port.Name},
		natsclient.StreamConsumerConfig{
			StreamName:    binding.stream,
			ConsumerName:  binding.consumer,
			FilterSubject: binding.subject,
			DeliverPolicy: declared.DeliverPolicy,
			AckPolicy:     declared.AckPolicy,
			MaxDeliver:    declared.MaxDeliver,
			MaxAckPending: declared.MaxAckPending,
			AutoCreate:    false,
		},
		c.handleJetStream)
	if err != nil {
		return nil, errs.WrapTransient(err, "objectstore", "bindJetStream", "consume stream "+binding.stream)
	}
	return handle, nil
}

// waitForStream waits for a JetStream stream to be available, retrying with a backoff until ctx
// ends or 30 lookups have failed.
func (c *Component) waitForStream(ctx context.Context, streamName string) error {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return errs.WrapTransient(err, "objectstore", "waitForStream", "get JetStream context")
	}
	const maxRetries = 30
	retryInterval := 100 * time.Millisecond
	for i := range maxRetries {
		if _, err := js.Stream(ctx, streamName); err == nil {
			return nil
		}
		if i == maxRetries-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
			retryInterval = min(retryInterval*2, 2*time.Second)
		}
	}
	return errs.WrapTransient(errs.ErrStorageUnavailable, "objectstore", "waitForStream",
		fmt.Sprintf("stream %s not available after %d retries", streamName, maxRetries))
}

// handleCore receives a core-NATS input message. Task 3.4 brings the write lane; until then the
// message is counted and dropped.
func (c *Component) handleCore(_ context.Context, msg *nats.Msg) {
	c.lifecycle.CountInput(len(msg.Data))
}

// handleJetStream receives a JetStream input delivery. Task 3.4 brings the write lane and its
// settlement; until then the delivery is counted and acknowledged.
func (c *Component) handleJetStream(_ context.Context, msg jetstream.Msg) {
	c.lifecycle.CountInput(len(msg.Data()))
	if err := msg.Ack(); err != nil {
		c.logger.Error("objectstore: ack failed", slog.String("subject", msg.Subject()), slog.Any("error", err))
	}
}

// Stop stops the Component through the shell, which runs release: Stop before Start releases
// nothing; a Stop whose release fails keeps it pending, and the next Stop runs it again; after a
// release that succeeded, Stop returns nil and runs nothing.
func (c *Component) Stop(ctx context.Context) error {
	return c.lifecycle.Stop(ctx)
}

// release is the cleanup the shell runs, for a Stop or for a failed Start's rollback. The handles
// stay set when cleanup fails, so the next Stop releases them again; only a cleanup that
// succeeded clears them.
func (c *Component) release(ctx context.Context) error {
	if err := c.cleanup(ctx); err != nil {
		return err
	}
	c.clearLifecycleHandles()
	return nil
}

// cleanup drains every bound input under ctx, then ends the run context and waits for the input
// check. A JetStream consumer's Drain is issued once; a later cleanup waits on the same drain. The
// Store has nothing to close.
func (c *Component) cleanup(ctx context.Context) error {
	var cleanupErr error
	for index := range c.consumers {
		binding := &c.consumers[index]
		if !binding.drainIssued {
			binding.handle.Drain()
			binding.drainIssued = true
		}
	}
	for _, subscription := range c.subscriptions {
		cleanupErr = errors.Join(cleanupErr, subscription.Drain(ctx))
	}
	for _, binding := range c.consumers {
		select {
		case <-binding.handle.Closed():
		case <-ctx.Done():
			cleanupErr = errors.Join(cleanupErr, ctx.Err())
		}
	}
	if c.cancel != nil {
		c.cancel()
	}
	if c.checkDone != nil {
		select {
		case <-c.checkDone:
		case <-ctx.Done():
			cleanupErr = errors.Join(cleanupErr, ctx.Err())
		}
	}
	return errors.Join(cleanupErr, ctx.Err())
}

// clearLifecycleHandles forgets the handles once a cleanup has released them. checkDone stays:
// cleanup closed it by waiting for the check, and an open one would show a check still running.
func (c *Component) clearLifecycleHandles() {
	c.cancel = nil
	c.subscriptions = nil
	c.consumers = nil
	c.store.Store(nil)
	c.inputFault.Store(nil)
}
