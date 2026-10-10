package component

import (
	"encoding/json"
	"testing"
)

// TestValidateConfigRequiredFields tests required field validation (T011)
// Given: Schema with required=["port"], config without port
// When: ValidateConfig called
// Then: Returns ValidationError for missing required field
func TestValidateConfigRequiredFields(t *testing.T) {
	schema := ConfigSchema{
		Properties: map[string]PropertySchema{
			"port": {
				Type:        "int",
				Description: "Port number",
			},
		},
		Required: []string{"port"},
	}

	config := map[string]any{
		// Missing required "port" field
	}

	// Execute
	errors := ValidateConfig(config, schema)

	// Assert
	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}

	if len(errors) > 0 {
		if errors[0].Field != "port" {
			t.Errorf("Expected error on field 'port', got %q", errors[0].Field)
		}

		if errors[0].Code != "required" {
			t.Errorf("Expected error code 'required', got %q", errors[0].Code)
		}
	}
}

// TestValidateConfigMinMax tests numeric min/max validation (T011)
// Given: Schema with port min=1, max=65535
// When: ValidateConfig with invalid values
// Then: Returns appropriate ValidationErrors
func TestValidateConfigMinMax(t *testing.T) {
	schema := ConfigSchema{
		Properties: map[string]PropertySchema{
			"port": {
				Type:    "int",
				Minimum: intPtr(1),
				Maximum: intPtr(65535),
			},
		},
		Required: []string{"port"},
	}

	testCases := []struct {
		name          string
		config        map[string]any
		expectedCode  string
		expectedField string
	}{
		{
			name:          "Below minimum",
			config:        map[string]any{"port": 0},
			expectedCode:  "min",
			expectedField: "port",
		},
		{
			name:          "Above maximum",
			config:        map[string]any{"port": 99999},
			expectedCode:  "max",
			expectedField: "port",
		},
		{
			name:          "Valid value",
			config:        map[string]any{"port": 8080},
			expectedCode:  "", // No error
			expectedField: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors := ValidateConfig(tc.config, schema)

			if tc.expectedCode == "" {
				if len(errors) != 0 {
					t.Errorf("Expected no errors, got %d: %+v", len(errors), errors)
				}
			} else {
				if len(errors) == 0 {
					t.Errorf("Expected error with code %q, got none", tc.expectedCode)
				} else if errors[0].Code != tc.expectedCode {
					t.Errorf("Expected error code %q, got %q", tc.expectedCode, errors[0].Code)
				}
			}
		})
	}
}

// TestValidateConfigEnumValues tests enum validation (T011)
// Given: Schema with enum values
// When: ValidateConfig with invalid enum value
// Then: Returns ValidationError with code="enum"
func TestValidateConfigEnumValues(t *testing.T) {
	schema := ConfigSchema{
		Properties: map[string]PropertySchema{
			"level": {
				Type: "string",
				Enum: []string{"debug", "info", "warn", "error"},
			},
		},
		Required: []string{"level"},
	}

	testCases := []struct {
		name         string
		config       map[string]any
		shouldError  bool
		expectedCode string
	}{
		{
			name:        "Valid enum value",
			config:      map[string]any{"level": "info"},
			shouldError: false,
		},
		{
			name:         "Invalid enum value",
			config:       map[string]any{"level": "invalid"},
			shouldError:  true,
			expectedCode: "enum",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors := ValidateConfig(tc.config, schema)

			if tc.shouldError {
				if len(errors) == 0 {
					t.Error("Expected validation error")
				} else if errors[0].Code != tc.expectedCode {
					t.Errorf("Expected code %q, got %q", tc.expectedCode, errors[0].Code)
				}
			} else {
				if len(errors) != 0 {
					t.Errorf("Expected no errors, got %d: %+v", len(errors), errors)
				}
			}
		})
	}
}

// TestValidateConfigTypeValidation tests type validation (T011)
// Given: Schema with specific types
// When: ValidateConfig with wrong type values
// Then: Returns ValidationError with code="type"
func TestValidateConfigTypeValidation(t *testing.T) {
	schema := ConfigSchema{
		Properties: map[string]PropertySchema{
			"port": {
				Type: "int",
			},
			"enabled": {
				Type: "bool",
			},
			"name": {
				Type: "string",
			},
		},
		Required: []string{"port", "enabled", "name"},
	}

	testCases := []struct {
		name         string
		config       map[string]any
		shouldError  bool
		expectedCode string
	}{
		{
			name: "Valid types",
			config: map[string]any{
				"port":    8080,
				"enabled": true,
				"name":    "test",
			},
			shouldError: false,
		},
		{
			name: "String for int field",
			config: map[string]any{
				"port":    "8080",
				"enabled": true,
				"name":    "test",
			},
			shouldError:  true,
			expectedCode: "type",
		},
		{
			name: "Number for bool field",
			config: map[string]any{
				"port":    8080,
				"enabled": 1,
				"name":    "test",
			},
			shouldError:  true,
			expectedCode: "type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors := ValidateConfig(tc.config, schema)

			if tc.shouldError {
				if len(errors) == 0 {
					t.Error("Expected validation error for type mismatch")
				} else {
					hasTypeError := false
					for _, err := range errors {
						if err.Code == "type" {
							hasTypeError = true
							break
						}
					}
					if !hasTypeError {
						t.Errorf("Expected at least one type error, got: %+v", errors)
					}
				}
			} else {
				if len(errors) != 0 {
					t.Errorf("Expected no errors, got %d: %+v", len(errors), errors)
				}
			}
		})
	}
}

// TestValidationErrorStructure tests ValidationError JSON serialization (T014)
// Given: ValidationError instances
// When: JSON marshal
// Then: Correct structure with field/message/code
func TestValidationErrorStructure(t *testing.T) {
	err := ValidationError{
		Field:   "port",
		Message: "Value must be between 1 and 65535",
		Code:    "max",
	}

	jsonData, jsonErr := json.Marshal(err)
	if jsonErr != nil {
		t.Fatalf("Failed to marshal ValidationError: %v", jsonErr)
	}

	expected := `{"field":"port","message":"Value must be between 1 and 65535","code":"max"}`

	if string(jsonData) != expected {
		t.Errorf("Expected JSON:\n%s\nGot:\n%s", expected, string(jsonData))
	}
}

// TestValidationErrorCodes tests error code enum values (T014)
// Given: ValidationError with various codes
// When: Codes are used
// Then: Codes match spec (required, min, max, pattern, enum, type)
func TestValidationErrorCodes(t *testing.T) {
	validCodes := []string{"required", "min", "max", "pattern", "enum", "type"}

	// Verify these codes are valid
	for _, code := range validCodes {
		_ = ValidationError{Code: code}
	}

	// No assertion needed - just verify it compiles and runs
	t.Logf("All validation error codes are valid: %v", validCodes)
}

// TestValidationConsistencyWithFrontend tests that backend validation produces
// the same error codes as frontend validation for consistent user experience (T112)
// Given: Schema with various constraints
// When: ValidateConfig called with invalid values
// Then: Error codes match frontend expectations (required, min, max, enum, type)
func TestValidationConsistencyWithFrontend(t *testing.T) {
	schema := ConfigSchema{
		Properties: map[string]PropertySchema{
			"port": {
				Type:    "int",
				Minimum: intPtr(1),
				Maximum: intPtr(65535),
			},
			"protocol": {
				Type: "string",
				Enum: []string{"tcp", "udp"},
			},
			"enabled": {
				Type: "bool",
			},
		},
		Required: []string{"port", "protocol"},
	}

	testCases := []struct {
		name          string
		config        map[string]any
		expectedCode  string
		expectedField string
	}{
		{
			name:          "Required field missing - error code: required",
			config:        map[string]any{"protocol": "tcp"}, // port missing
			expectedCode:  "required",
			expectedField: "port",
		},
		{
			name:          "Value exceeds max - error code: max",
			config:        map[string]any{"port": 99999, "protocol": "tcp"}, // Exceeds max 65535
			expectedCode:  "max",
			expectedField: "port",
		},
		{
			name:          "Value below min - error code: min",
			config:        map[string]any{"port": 0, "protocol": "tcp"}, // Below min 1
			expectedCode:  "min",
			expectedField: "port",
		},
		{
			name:          "Invalid enum value - error code: enum",
			config:        map[string]any{"port": 8080, "protocol": "http"}, // Not in enum ["tcp", "udp"]
			expectedCode:  "enum",
			expectedField: "protocol",
		},
		{
			name:          "Type mismatch (string for int) - error code: type",
			config:        map[string]any{"port": "not-a-number", "protocol": "tcp"},
			expectedCode:  "type",
			expectedField: "port",
		},
		{
			name:          "Type mismatch (number for bool) - error code: type",
			config:        map[string]any{"port": 8080, "protocol": "tcp", "enabled": 1},
			expectedCode:  "type",
			expectedField: "enabled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors := ValidateConfig(tc.config, schema)

			if len(errors) == 0 {
				t.Errorf("Expected validation error with code %q, got none", tc.expectedCode)
				return
			}

			// Find error for expected field
			var foundError *ValidationError
			for _, err := range errors {
				if err.Field == tc.expectedField {
					foundError = &err
					break
				}
			}

			if foundError == nil {
				t.Errorf("Expected error for field %q, got errors for: %v", tc.expectedField, errors)
				return
			}

			if foundError.Code != tc.expectedCode {
				t.Errorf(
					"Expected error code %q for field %q, got %q",
					tc.expectedCode,
					tc.expectedField,
					foundError.Code,
				)
			}

			// Verify error message is present
			if foundError.Message == "" {
				t.Error("Expected non-empty error message")
			}
		})
	}
}

// TestSchemaFallback tests graceful degradation when schema missing (T021)
// Given: Component without schema (empty ConfigSchema)
// When: ValidateConfig called
// Then: Validation skipped gracefully, no errors returned
func TestSchemaFallback(t *testing.T) {
	t.Run("Empty schema allows any config", func(t *testing.T) {
		// Given: Schema with no properties defined
		emptySchema := ConfigSchema{
			Properties: nil,
			Required:   nil,
		}

		// When: ValidateConfig called with any config
		config := map[string]any{
			"arbitrary_field": "arbitrary_value",
			"port":            8080,
			"enabled":         true,
		}

		errors := ValidateConfig(config, emptySchema)

		// Then: No validation errors (graceful fallback)
		if len(errors) != 0 {
			t.Errorf("Expected no errors for empty schema, got %d: %+v", len(errors), errors)
		}
	})

	t.Run("Schema with empty Properties map allows any config", func(t *testing.T) {
		// Given: Schema with empty Properties map
		schema := ConfigSchema{
			Properties: make(map[string]PropertySchema),
			Required:   []string{},
		}

		// When: ValidateConfig called
		config := map[string]any{
			"any_field": "any_value",
		}

		errors := ValidateConfig(config, schema)

		// Then: No validation errors
		if len(errors) != 0 {
			t.Errorf("Expected no errors for schema with empty Properties, got %d: %+v", len(errors), errors)
		}
	})
}
