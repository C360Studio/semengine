package service

import "fmt"

// RegisterAll registers the framework's built-in services with the registry:
// metrics, the component manager, the message logger, storage observability,
// the heartbeat, the metrics forwarder and the log forwarder, which forwards
// nothing yet: no log handler in this module publishes logs to NATS.
//
// It also registers the component manager's and the message logger's OpenAPI
// specifications, because /openapi.json takes its schemas only from that
// registry: without them, the flowgraph and validate responses' Graph and
// Result references, and the entries response's MessageLogEntry reference,
// dangle. Storage observability's specification names no schema, and none of
// the heartbeat, the metrics forwarder and the log forwarder serves a route,
// so none is registered for any of the four. A composition that registers
// NewComponentManager or NewMessageLoggerService by hand instead of calling
// RegisterAll registers the specification itself.
func RegisterAll(registry *Registry) error {
	services := map[string]Constructor{
		"metrics":                       NewMetrics,
		componentManagerName:            NewComponentManager,
		"message-logger":                NewMessageLoggerService,
		StorageObservabilityServiceName: NewStorageObservabilityService,
		heartbeatServiceName:            NewHeartbeatService,
		metricsForwarderServiceName:     NewMetricsForwarderService,
		logForwarderServiceName:         NewLogForwarderService,
	}

	for name, constructor := range services {
		if err := registry.Register(name, constructor); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}
	RegisterOpenAPISpec(componentManagerName, componentManagerOpenAPISpec())
	RegisterOpenAPISpec("message-logger", messageLoggerOpenAPISpec())
	return nil
}
