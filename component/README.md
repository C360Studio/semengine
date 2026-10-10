# Component Package

The component model every SemEngine component builds on: factory registration, immutable port declarations, typed
ports, configuration schemas, and the lifecycle interface.

```go
import "github.com/c360studio/semengine/component"
```

## Overview

A **component** is one self-describing unit of a SemEngine process. There are five kinds: inputs (data sources),
processors (data transformers), outputs (data sinks), storage (persistence), and gateways (query surfaces). Each one
reports its metadata, its ports, its configuration schema, its health, and its data flow through the `Discoverable`
interface.

The terms this README uses:

- **Factory**: a function that builds a component from its raw JSON configuration and a `Dependencies` value. It
  decodes the configuration and does no I/O; the component's I/O starts in `Start`.
- **Port declarer**: a function, registered beside the factory, that derives the component's ports from the same raw
  configuration without building anything. It lets a tool read a component's ports offline.
- **Declaration**: the immutable port shape of one component instance (its name, factory, ports, and the resources it
  holds exclusively). The `Registry` keeps declarations, never the live component.
- **Boot admission**: building each configured instance once at process start, checking that the ports the component
  reports equal the ports its declarer derives, and recording its declaration.
- **Sealing**: closing boot admission. After `SealComposition`, the `Registry` refuses to create or admit another
  component until the process starts again with a new `Registry`.
- **Component manager**: `service.ComponentManager`, which owns every live component and its lifecycle. It is not yet
  in SemEngine; it arrives with setup change 3.

## Registration

A component package exports a `Register(registry)` function instead of registering itself in `init()`. The host
creates a `Registry`, calls each package's `Register`, and hands the registry to the component manager. The manager
calls `CreateComponent` for each enabled instance and then `SealComposition`.

Explicit registration keeps package imports free of side effects: a test builds a registry holding only the components
it needs, and the host decides what is registered and in which order. The cost is that a new component package must
be added to the host's list of `Register` calls.

## Implementing a component

```go
package mycomponent

import (
    "encoding/json"

    "github.com/c360studio/semengine/component"
)

// Config is this component's configuration; its factory decodes it.
type Config struct {
    Subject string `json:"subject"`
}

var schema = component.ConfigSchema{
    Properties: map[string]component.PropertySchema{
        "subject": {Type: "string", Description: "NATS subject the input publishes on"},
    },
    Required: []string{"subject"},
}

// outputPorts derives the ports from the configuration. The port declarer and
// the component both call it, so the declared and the reported ports agree.
func outputPorts(cfg Config) []component.Port {
    return []component.Port{{
        Name:      "output",
        Direction: component.DirectionOutput,
        Required:  true,
        Config:    component.NATSPort{Subject: cfg.Subject},
    }}
}

// declarePorts is the port declarer: no dependencies, no I/O.
func declarePorts(raw json.RawMessage, _ string) (component.PortConfig, error) {
    var cfg Config
    if err := json.Unmarshal(raw, &cfg); err != nil {
        return component.PortConfig{}, err
    }
    return component.PortConfigFrom(nil, outputPorts(cfg)), nil
}

type Input struct {
    cfg  Config
    deps component.Dependencies
}

func (c *Input) Meta() component.Metadata {
    return component.Metadata{Name: "my-input", Type: "input", Description: "Example input", Version: "1.0.0"}
}
func (c *Input) InputPorts() []component.Port         { return nil }
func (c *Input) OutputPorts() []component.Port        { return outputPorts(c.cfg) }
func (c *Input) ConfigSchema() component.ConfigSchema { return schema }
func (c *Input) Health() component.HealthStatus       { return component.HealthStatus{Healthy: true} }
func (c *Input) DataFlow() component.FlowMetrics      { return component.FlowMetrics{} }

// create is the factory.
func create(raw json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
    var cfg Config
    if err := json.Unmarshal(raw, &cfg); err != nil {
        return nil, err
    }
    return &Input{cfg: cfg, deps: deps}, nil
}

// Register adds this package's factory to a registry. The host calls it.
func Register(registry *component.Registry) error {
    return registry.RegisterWithConfig(component.RegistrationConfig{
        Name:        "my-input",
        Factory:     create,
        Ports:       declarePorts,
        Schema:      schema,
        Type:        "input",
        Protocol:    "nats",
        Domain:      "example",
        Description: "Example input",
        Version:     "1.0.0",
    })
}
```

A component that does work also implements `LifecycleComponent`: `Initialize() error`, `Start(ctx) error` and
`Stop(ctx) error` beside `Discoverable`. `Start` and `Stop` refuse a nil context, and `Stop` joins the work `Start`
began. `AsLifecycleComponent` reports whether a `Discoverable` implements it.

## Core types

### Dependencies

What the host passes to every factory:

```go
type Dependencies struct {
    NATSClient      *natsclient.Client        // required: CreateComponent refuses a nil client
    MetricsRegistry *metric.MetricsRegistry   // may be nil
    Logger          *slog.Logger              // may be nil; GetLogger falls back to slog.Default()
    Platform        PlatformMeta              // the deployment's organization and platform
    Security        security.Config           // platform-wide security configuration
    ModelRegistry   model.RegistryReader      // the model registry chosen at boot; may be nil
    PayloadRegistry *payloadregistry.Registry // may be nil; a component that decodes BaseMessage needs it
    StoreRegistry   *storeregistry.Registry   // the stores storage components provide, filled by the manager
}
```

`Dependencies` carries no lifecycle manager and no tool registry. A component that needs the lifecycle manager
(`pkg/lifecycle`) receives it from the host when it is registered or constructed. A contract test
(`internal/harness/contract/componentdeps_test.go`) fails if `component` comes to import, directly or through another
package, an agentic package, `graph` or a package under it, `internal/graphmutation`, `pkg/lifecycle`, or
`pkg/projection` other than `pkg/projection/contract`.

### Ports

A `Port` has a name, a direction, and a typed configuration (`Config`) that says what the port binds to:

```go
// Core NATS publish or subscribe
component.NATSPort{Subject: "data.output"}

// JetStream stream and its subjects
component.JetStreamPort{StreamName: "EVENTS", Subjects: []string{"events.>"}}

// KV bucket watch; no keys means every key
component.KVWatchPort{Bucket: "CONFIG", Keys: []string{"app.*"}}

// KV bucket write, with the payload type it writes
component.KVWritePort{
    Bucket:    "ENTITY_STATES",
    Interface: &component.InterfaceContract{Type: "graph.EntityState", Version: "v1"},
}

// Network listener
component.NetworkPort{Protocol: "udp", Host: "0.0.0.0", Port: 14550}
```

The other port types are `NATSRequestPort` (request and reply), `KVReadPort`, `HTTPClientPort`, `FilePort`,
`TimerPort`, `StoreReadPort` and `StoreProvidePort`, each in a `port_*.go` file.

## Registry

- `NewRegistry()` returns an empty registry.
- `RegisterWithConfig(RegistrationConfig)` registers a factory and its static metadata. It refuses an empty name, a nil
  factory, an empty type, a nil port declarer (`Ports`), and a name already registered.
- `ListFactories()` returns a copy of each registration's metadata, without its factory function; it returns no live
  component.
- `Declare(instanceName, config)` runs a factory's port declarer for one configured instance and returns its
  `Declaration`, without building the component.
- `CreateComponent(instanceName, config, deps, prepare)`, `SealComposition()` and `Snapshots()` exist for the
  component manager. A consumer creates and composes components through that manager; a direct call is not refused,
  but it bypasses the manager's bookkeeping. `CreateComponent` refuses a disabled instance, a nil
  `Dependencies.NATSClient`, a port declaration that differs from the ports the built component reports, an instance
  name already admitted, an exclusive resource another instance holds, and any call after `SealComposition`. `prepare`
  may be nil.

### Errors

Every refusal from `RegisterWithConfig` is a `pkg/errs` error of the invalid class:

```go
err := registry.RegisterWithConfig(registration)
if errs.IsInvalid(err) {
    // A required field is missing, or the name is already registered.
}
```

`CreateComponent` wraps a factory's own error with `errs.Wrap`, so its class is the one the factory chose.

### Concurrency

Every `Registry` method is safe for concurrent use; one read-write mutex guards the factories, the declarations and the
sealed flag. A factory and a port declarer run without that mutex held. Sealing stops `CreateComponent`; it does not
stop `RegisterWithConfig`.

## Testing

- Build a registry per test with `NewRegistry()` and register only the components the test needs; no registry is
  process-global.
- A test in this module that needs NATS opens a server through `internal/harness/natsfixture` (`natsfixture.Open`).
- Test a component through its `Discoverable` and `LifecycleComponent` methods, not its private fields.

## Related packages

- [types](../types): `ComponentConfig` and `ComponentType`
- [natsclient](../natsclient): the NATS client in `Dependencies`
- [metric](../metric): the metrics registry in `Dependencies`
- [payloadregistry](../payloadregistry): payload decoding for components that read `BaseMessage`
- [storage/storeregistry](../storage/storeregistry): the stores components share

`service` (the component manager) and `componentregistry` (a host's list of `Register` calls) are not yet in SemEngine.
