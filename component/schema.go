// Package component provides schema validation and helper functions
package component

import "fmt"

// ValidationError represents a validation error for a specific configuration field.
// It provides structured error information that can be displayed to users
// and mapped to specific form fields in the UI.
//
// Error codes are standardized across frontend and backend:
//   - "required": Field is required but missing
//   - "min": Numeric value below minimum threshold
//   - "max": Numeric value above maximum threshold
//   - "enum": Value not in allowed enum values
//   - "type": Value doesn't match expected type (string, int, bool, etc.)
//   - "pattern": String doesn't match required pattern (future use)
type ValidationError struct {
	Field   string `json:"field"`   // Name of the field that failed validation
	Message string `json:"message"` // Human-readable error message
	Code    string `json:"code"`    // Machine-readable error code (see above)
}

// ValidateConfig validates a configuration map against a ConfigSchema.
// It checks required fields, type constraints, min/max bounds, and enum values.
//
// The validation is lenient - unknown fields are allowed to support backward
// compatibility and future schema evolution. Only explicitly defined properties
// are validated against their schema constraints.
//
// Returns a slice of ValidationError containing all validation failures found.
// An empty slice indicates the configuration is valid.
//
// Example usage:
//
//	schema := component.ConfigSchema{
//	    Properties: map[string]component.PropertySchema{
//	        "port": {
//	            Type:     "int",
//	            Minimum:  ptrInt(1),
//	            Maximum:  ptrInt(65535),
//	            Category: "basic",
//	        },
//	    },
//	    Required: []string{"port"},
//	}
//
//	config := map[string]any{"port": 99999}
//	errors := component.ValidateConfig(config, schema)
//	if len(errors) > 0 {
//	    // Handle validation errors
//	    fmt.Printf("Validation failed: %s\n", errors[0].Message)
//	}
func ValidateConfig(config map[string]any, schema ConfigSchema) []ValidationError {
	var errors []ValidationError

	// Check required fields
	for _, requiredField := range schema.Required {
		if _, exists := config[requiredField]; !exists {
			errors = append(errors, ValidationError{
				Field:   requiredField,
				Message: fmt.Sprintf("Field %q is required", requiredField),
				Code:    "required",
			})
		}
	}

	// Validate each field in config
	for fieldName, value := range config {
		propSchema, exists := schema.Properties[fieldName]
		if !exists {
			// Unknown fields are allowed (lenient validation)
			continue
		}

		// Type validation
		if err := validateType(fieldName, value, propSchema); err != nil {
			errors = append(errors, *err)
			continue // Skip further validation if type is wrong
		}

		// Enum validation
		if len(propSchema.Enum) > 0 {
			if err := validateEnum(fieldName, value, propSchema.Enum); err != nil {
				errors = append(errors, *err)
			}
		}

		// Min/Max validation for numeric types
		if propSchema.Type == "int" || propSchema.Type == "float" {
			if propSchema.Minimum != nil {
				if err := validateMin(fieldName, value, *propSchema.Minimum); err != nil {
					errors = append(errors, *err)
				}
			}
			if propSchema.Maximum != nil {
				if err := validateMax(fieldName, value, *propSchema.Maximum); err != nil {
					errors = append(errors, *err)
				}
			}
		}

		// Recursive validation for nested objects
		if propSchema.Type == "object" && len(propSchema.Properties) > 0 {
			if nestedConfig, ok := value.(map[string]any); ok {
				nestedSchema := ConfigSchema{
					Properties: propSchema.Properties,
					Required:   propSchema.Required,
				}
				nestedErrors := ValidateConfig(nestedConfig, nestedSchema)
				for _, err := range nestedErrors {
					errors = append(errors, ValidationError{
						Field:   fieldName + "." + err.Field,
						Message: err.Message,
						Code:    err.Code,
					})
				}
			}
		}
	}

	return errors
}

// validateType checks if the value matches the expected type
func validateType(fieldName string, value any, propSchema PropertySchema) *ValidationError {
	switch propSchema.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("Field %q must be a string", fieldName),
				Code:    "type",
			}
		}
	case "int":
		// Accept both int and float64 (JSON numbers)
		switch value.(type) {
		case int, int32, int64, float64:
			// Valid
		default:
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("Field %q must be an integer", fieldName),
				Code:    "type",
			}
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("Field %q must be a boolean", fieldName),
				Code:    "type",
			}
		}
	case "float":
		// Accept int, float32, float64
		switch value.(type) {
		case int, int32, int64, float32, float64:
			// Valid
		default:
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("Field %q must be a number", fieldName),
				Code:    "type",
			}
		}
	}
	return nil
}

// validateEnum checks if the value is in the allowed enum values
func validateEnum(fieldName string, value any, enumValues []string) *ValidationError {
	strValue, ok := value.(string)
	if !ok {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("Field %q must be a string for enum validation", fieldName),
			Code:    "type",
		}
	}

	for _, allowed := range enumValues {
		if strValue == allowed {
			return nil // Valid
		}
	}

	return &ValidationError{
		Field:   fieldName,
		Message: fmt.Sprintf("Field %q must be one of: %v", fieldName, enumValues),
		Code:    "enum",
	}
}

// validateMin checks if numeric value meets minimum
func validateMin(fieldName string, value any, minV int) *ValidationError {
	var numValue float64
	switch v := value.(type) {
	case int:
		numValue = float64(v)
	case int32:
		numValue = float64(v)
	case int64:
		numValue = float64(v)
	case float32:
		numValue = float64(v)
	case float64:
		numValue = v
	default:
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("Field %q must be numeric for min validation", fieldName),
			Code:    "type",
		}
	}

	if numValue < float64(minV) {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("Field %q must be >= %d", fieldName, minV),
			Code:    "min",
		}
	}
	return nil
}

// validateMax checks if numeric value meets maximum
func validateMax(fieldName string, value any, maxV int) *ValidationError {
	var numValue float64
	switch v := value.(type) {
	case int:
		numValue = float64(v)
	case int32:
		numValue = float64(v)
	case int64:
		numValue = float64(v)
	case float32:
		numValue = float64(v)
	case float64:
		numValue = v
	default:
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("Field %q must be numeric for max validation", fieldName),
			Code:    "type",
		}
	}

	if numValue > float64(maxV) {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("Field %q must be <= %d", fieldName, maxV),
			Code:    "max",
		}
	}
	return nil
}
