package service

import "fmt"

// RegisterAll registers the framework's built-in services with the registry.
// It offers only metrics until each other service's owner is ported
// (setup-04a-03-boot-service-manager design D1).
func RegisterAll(registry *Registry) error {
	services := map[string]Constructor{
		"metrics": NewMetrics,
	}

	for name, constructor := range services {
		if err := registry.Register(name, constructor); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}
	return nil
}
