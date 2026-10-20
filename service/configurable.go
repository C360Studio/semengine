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
	// Convert our extended PropertySchema to component.PropertySchema. The
	// outer Category is the one callers set; it shadows the embedded field, so
	// it is copied in when set rather than dropped.
	componentProps := make(map[string]component.PropertySchema)
	for key, prop := range properties {
		p := prop.PropertySchema
		if prop.Category != "" {
			p.Category = prop.Category
		}
		componentProps[key] = p
	}

	return ConfigSchema{
		ConfigSchema: component.ConfigSchema{
			Properties: componentProps,
			Required:   required,
		},
	}
}
