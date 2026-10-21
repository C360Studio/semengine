package service

import (
	"fmt"
	"sync"
)

// Registry manages service constructor registration
type Registry struct {
	constructors map[string]Constructor
	mu           sync.RWMutex
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry() *Registry {
	return &Registry{
		constructors: make(map[string]Constructor),
	}
}

// Register registers a service constructor
func (r *Registry) Register(name string, constructor Constructor) error {
	if name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if constructor == nil {
		return fmt.Errorf("constructor cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.constructors[name]; exists {
		return fmt.Errorf("service %s already registered", name)
	}

	r.constructors[name] = constructor
	return nil
}

// Constructor returns a constructor for the given service name
func (r *Registry) Constructor(name string) (Constructor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	constructor, exists := r.constructors[name]
	return constructor, exists
}
