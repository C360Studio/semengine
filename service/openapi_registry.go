// Package service provides the OpenAPI registry for service specifications
package service

import "sync"

// openAPIRegistry holds all registered service OpenAPI specifications
var (
	openAPIRegistry   = make(map[string]*OpenAPISpec)
	openAPIRegistryMu sync.RWMutex
)

// RegisterOpenAPISpec registers an OpenAPI specification for a service. Call
// it from the composition root, never from an init function: importing a
// package registers nothing.
func RegisterOpenAPISpec(name string, spec *OpenAPISpec) {
	openAPIRegistryMu.Lock()
	defer openAPIRegistryMu.Unlock()
	openAPIRegistry[name] = spec
}

// GetAllOpenAPISpecs returns all registered OpenAPI specifications.
// Used by the openapi-generator tool to collect specs from all services.
func GetAllOpenAPISpecs() map[string]*OpenAPISpec {
	openAPIRegistryMu.RLock()
	defer openAPIRegistryMu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*OpenAPISpec, len(openAPIRegistry))
	for k, v := range openAPIRegistry {
		result[k] = v
	}
	return result
}
