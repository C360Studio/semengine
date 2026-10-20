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

// GetAllOpenAPISpecs returns a copy of the registration map. The *OpenAPISpec
// values are the registered pointers themselves, so a change through one is seen
// by every later caller. The service manager's /openapi.json handler reads it
// (PR 2 of the port).
func GetAllOpenAPISpecs() map[string]*OpenAPISpec {
	openAPIRegistryMu.RLock()
	defer openAPIRegistryMu.RUnlock()

	// Copy the map so a caller cannot add or remove registrations.
	result := make(map[string]*OpenAPISpec, len(openAPIRegistry))
	for k, v := range openAPIRegistry {
		result[k] = v
	}
	return result
}
