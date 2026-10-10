// Package component defines SemEngine component factories, lifecycle
// interfaces, dependencies, configuration schemas, ports, and immutable
// registry declarations.
//
// Applications register component factories explicitly during process
// composition. Factories are side-effect free: they construct a Discoverable,
// while I/O begins in Start and all owned work joins Stop.
//
// Component creation is a boot operation of the component manager
// (service.ComponentManager). It constructs every enabled component from its
// constructor-captured configuration, captures the component's declared ports
// and resources, and admits an immutable declaration to Registry. The manager
// retains the concrete handle as the sole lifecycle owner. Registry does not
// refuse a direct call; such a call bypasses the manager's bookkeeping.
//
// Registry read surfaces return defensive values and never expose a live
// component handle. Once boot admission is complete, Registry is sealed; later
// component admission attempts fail until another process boot creates a
// fresh Registry.
//
// Discoverable components describe metadata, health, flow metrics, schemas,
// and typed ports. Port declarations support core NATS subjects, JetStream,
// KV watches and writes, request/reply, network listeners, APIs, and related
// framework patterns. Registry.Declare produces the same declaration values
// offline, for static diagram validation, without constructing a component.
package component
