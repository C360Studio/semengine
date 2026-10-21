package service

import "fmt"

// RegisterAll registers the framework's built-in services with the registry:
// metrics and the component manager. The other services are offered as each
// one's owner is ported (setup-04a-03-boot-service-manager design D1).
//
// It also registers the component manager's OpenAPI specification, because
// /openapi.json takes its schemas only from that registry: without it, the
// flowgraph and validate responses' Graph and Result references dangle. A
// composition that registers NewComponentManager by hand instead of calling
// RegisterAll registers the specification itself.
func RegisterAll(registry *Registry) error {
	services := map[string]Constructor{
		"metrics":            NewMetrics,
		componentManagerName: NewComponentManager,
	}

	for name, constructor := range services {
		if err := registry.Register(name, constructor); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}
	RegisterOpenAPISpec(componentManagerName, componentManagerOpenAPISpec())
	return nil
}
