package service

import (
	"github.com/c360studio/semengine/component"
)

// ConfigSchema describes the configuration parameters for a service.
// We embed the component ConfigSchema for consistency across the system.
type ConfigSchema struct {
	component.ConfigSchema

	// ServiceSpecific can hold any service-specific schema extensions
	ServiceSpecific map[string]any `json:"service_specific,omitempty"`
}

// PropertySchema extends component.PropertySchema with service-specific fields
type PropertySchema struct {
	component.PropertySchema

	// Category groups related properties for UI organization
	Category string `json:"category,omitempty"`
}

// NewConfigSchema creates a service ConfigSchema with extended property schemas
func NewConfigSchema(properties map[string]PropertySchema, required []string) ConfigSchema {
	// Convert our extended PropertySchema to component.PropertySchema
	componentProps := make(map[string]component.PropertySchema)
	for key, prop := range properties {
		componentProps[key] = prop.PropertySchema
	}

	return ConfigSchema{
		ConfigSchema: component.ConfigSchema{
			Properties: componentProps,
			Required:   required,
		},
	}
}
