package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadDomainExamples_ValidJSON tests loading a valid domain examples JSON file
func TestLoadDomainExamples_ValidJSON(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		validate func(t *testing.T, examples *DomainExamples)
	}{
		{
			name: "complete logistics example",
			json: `{
				"domain": "logistics",
				"version": "1.0",
				"examples": [
					{
						"query": "Where is shipment ABC-123?",
						"intent": "entity_lookup",
						"options": {"entity_pattern": "shipment"}
					},
					{
						"query": "Show delayed deliveries this week",
						"intent": "temporal_filter",
						"options": {"status_filter": "delayed"}
					}
				]
			}`,
			validate: func(t *testing.T, examples *DomainExamples) {
				assert.Equal(t, "logistics", examples.Domain)
				assert.Equal(t, "1.0", examples.Version)
				assert.Len(t, examples.Examples, 2)

				// Verify first example
				ex1 := examples.Examples[0]
				assert.Equal(t, "Where is shipment ABC-123?", ex1.Query)
				assert.Equal(t, "entity_lookup", ex1.Intent)
				assert.NotNil(t, ex1.Options)
				assert.Equal(t, "shipment", ex1.Options["entity_pattern"])

				// Verify second example
				ex2 := examples.Examples[1]
				assert.Equal(t, "Show delayed deliveries this week", ex2.Query)
				assert.Equal(t, "temporal_filter", ex2.Intent)
				assert.Equal(t, "delayed", ex2.Options["status_filter"])
			},
		},
		{
			name: "example without options field",
			json: `{
				"domain": "iot",
				"version": "2.0",
				"examples": [
					{
						"query": "Find packages similar to order-456",
						"intent": "similarity"
					}
				]
			}`,
			validate: func(t *testing.T, examples *DomainExamples) {
				assert.Equal(t, "iot", examples.Domain)
				assert.Equal(t, "2.0", examples.Version)
				require.Len(t, examples.Examples, 1)

				ex := examples.Examples[0]
				assert.Equal(t, "Find packages similar to order-456", ex.Query)
				assert.Equal(t, "similarity", ex.Intent)
				// Options should be nil or empty when not provided
				assert.Nil(t, ex.Options)
			},
		},
		{
			name: "empty examples array",
			json: `{
				"domain": "test",
				"version": "1.0",
				"examples": []
			}`,
			validate: func(t *testing.T, examples *DomainExamples) {
				assert.Equal(t, "test", examples.Domain)
				assert.Equal(t, "1.0", examples.Version)
				assert.Empty(t, examples.Examples)
			},
		},
		{
			name: "complex options with nested structures",
			json: `{
				"domain": "robotics",
				"version": "1.0",
				"examples": [
					{
						"query": "Find drones within 10km of location",
						"intent": "spatial_filter",
						"options": {
							"entity_pattern": "drone",
							"radius_km": 10,
							"include_offline": false,
							"filters": ["active", "deployed"]
						}
					}
				]
			}`,
			validate: func(t *testing.T, examples *DomainExamples) {
				require.Len(t, examples.Examples, 1)
				ex := examples.Examples[0]

				assert.Equal(t, "drone", ex.Options["entity_pattern"])
				assert.Equal(t, float64(10), ex.Options["radius_km"]) // JSON numbers are float64
				assert.Equal(t, false, ex.Options["include_offline"])

				// Verify array in options
				filters, ok := ex.Options["filters"].([]interface{})
				require.True(t, ok)
				assert.Len(t, filters, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file with JSON
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "examples.json")
			err := os.WriteFile(filePath, []byte(tt.json), 0644)
			require.NoError(t, err)

			// Load examples
			examples, err := LoadDomainExamples(filePath)
			require.NoError(t, err)
			require.NotNil(t, examples)

			// Validate results
			tt.validate(t, examples)
		})
	}
}

// TestLoadDomainExamples_InvalidJSON tests error handling for malformed JSON
func TestLoadDomainExamples_InvalidJSON(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		errContains string
	}{
		{
			name:        "malformed JSON",
			json:        `{"domain": "test", "version": "1.0", "examples": [}`,
			errContains: "invalid character",
		},
		{
			name:        "missing required domain field",
			json:        `{"version": "1.0", "examples": []}`,
			errContains: "", // Should still parse, just have empty domain
		},
		{
			name:        "wrong type for examples field",
			json:        `{"domain": "test", "version": "1.0", "examples": "not-an-array"}`,
			errContains: "cannot unmarshal",
		},
		{
			name:        "empty JSON object",
			json:        `{}`,
			errContains: "", // Should parse with empty values
		},
		{
			name:        "not valid JSON at all",
			json:        `this is not json`,
			errContains: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "examples.json")
			err := os.WriteFile(filePath, []byte(tt.json), 0644)
			require.NoError(t, err)

			examples, err := LoadDomainExamples(filePath)

			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, examples)
			} else {
				// Some invalid cases should still parse
				require.NoError(t, err)
			}
		})
	}
}

// TestLoadDomainExamples_FileErrors tests file system error handling
func TestLoadDomainExamples_FileErrors(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		examples, err := LoadDomainExamples("/path/that/does/not/exist/examples.json")
		require.Error(t, err)
		assert.Nil(t, examples)
		assert.Contains(t, err.Error(), "no such file")
	})

	t.Run("empty file path", func(t *testing.T) {
		examples, err := LoadDomainExamples("")
		require.Error(t, err)
		assert.Nil(t, examples)
	})

	t.Run("directory instead of file", func(t *testing.T) {
		tmpDir := t.TempDir()
		examples, err := LoadDomainExamples(tmpDir)
		require.Error(t, err)
		assert.Nil(t, examples)
	})

	t.Run("empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "empty.json")
		err := os.WriteFile(filePath, []byte(""), 0644)
		require.NoError(t, err)

		examples, err := LoadDomainExamples(filePath)
		require.Error(t, err)
		assert.Nil(t, examples)
	})
}

// TestExample_VectorField tests that Vector field is properly initialized
func TestExample_VectorField(t *testing.T) {
	// The Vector field should not be populated from JSON, it's runtime-only
	json := `{
		"domain": "test",
		"version": "1.0",
		"examples": [
			{
				"query": "test query",
				"intent": "test_intent",
				"options": {"key": "value"}
			}
		]
	}`

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.json")
	err := os.WriteFile(filePath, []byte(json), 0644)
	require.NoError(t, err)

	examples, err := LoadDomainExamples(filePath)
	require.NoError(t, err)
	require.Len(t, examples.Examples, 1)

	ex := examples.Examples[0]
	// Vector should be nil/empty when loaded from JSON
	assert.Nil(t, ex.Vector)
}

// TestExample_JSONRoundTrip tests marshaling and unmarshaling
func TestExample_JSONRoundTrip(t *testing.T) {
	original := Example{
		Query:  "test query",
		Intent: "entity_lookup",
		Options: map[string]any{
			"pattern": "test",
			"limit":   10,
			"enabled": true,
		},
		Vector: []float32{0.1, 0.2, 0.3}, // Runtime field
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var restored Example
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	// Verify core fields match
	assert.Equal(t, original.Query, restored.Query)
	assert.Equal(t, original.Intent, restored.Intent)

	// Options should match
	assert.Equal(t, "test", restored.Options["pattern"])
	assert.Equal(t, float64(10), restored.Options["limit"]) // JSON numbers are float64
	assert.Equal(t, true, restored.Options["enabled"])

	// Vector should NOT be preserved through JSON (json:"-" tag)
	// It's a runtime-only field, so restored.Vector should be nil
	assert.Nil(t, restored.Vector)
}

// TestExample_EmptyQueryString tests handling of empty query strings
func TestExample_EmptyQueryString(t *testing.T) {
	json := `{
		"domain": "test",
		"version": "1.0",
		"examples": [
			{
				"query": "",
				"intent": "test_intent"
			}
		]
	}`

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.json")
	err := os.WriteFile(filePath, []byte(json), 0644)
	require.NoError(t, err)

	examples, err := LoadDomainExamples(filePath)
	require.NoError(t, err)
	require.Len(t, examples.Examples, 1)

	// Empty query should be loaded as-is
	assert.Equal(t, "", examples.Examples[0].Query)
	assert.Equal(t, "test_intent", examples.Examples[0].Intent)
}

// TestExample_SpecialCharacters tests handling of special characters in queries
func TestExample_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "unicode characters",
			query: "Where is shipment 运输-ABC-123?",
		},
		{
			name:  "emojis",
			query: "Show 🚚 deliveries with 📦 status",
		},
		{
			name:  "quotes and escapes",
			query: `Find items with "special" characters and \backslash`,
		},
		{
			name:  "newlines and tabs",
			query: "Multi\nline\tquery",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domainExamples := DomainExamples{
				Domain:  "test",
				Version: "1.0",
				Examples: []Example{
					{
						Query:  tt.query,
						Intent: "test",
					},
				},
			}

			// Marshal to JSON
			data, err := json.Marshal(domainExamples)
			require.NoError(t, err)

			// Unmarshal back
			var restored DomainExamples
			err = json.Unmarshal(data, &restored)
			require.NoError(t, err)

			// Query should be preserved exactly
			require.Len(t, restored.Examples, 1)
			assert.Equal(t, tt.query, restored.Examples[0].Query)
		})
	}
}

// TestLoadDomainExamples_LargeFile tests handling of files with many examples
func TestLoadDomainExamples_LargeFile(t *testing.T) {
	// Create a domain file with 1000 examples
	domainExamples := DomainExamples{
		Domain:   "large_test",
		Version:  "1.0",
		Examples: make([]Example, 1000),
	}

	for i := range domainExamples.Examples {
		domainExamples.Examples[i] = Example{
			Query:  "Test query number " + string(rune(i)),
			Intent: "test_intent",
			Options: map[string]any{
				"index": i,
			},
		}
	}

	// Marshal to JSON
	data, err := json.Marshal(domainExamples)
	require.NoError(t, err)

	// Write to file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large.json")
	err = os.WriteFile(filePath, data, 0644)
	require.NoError(t, err)

	// Load and verify
	loaded, err := LoadDomainExamples(filePath)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Len(t, loaded.Examples, 1000)
	assert.Equal(t, "large_test", loaded.Domain)
}

// TestExample_OptionsTypes tests that Options can hold various Go types
func TestExample_OptionsTypes(t *testing.T) {
	example := Example{
		Query:  "test",
		Intent: "test",
		Options: map[string]any{
			"string": "value",
			"int":    42,
			"float":  3.14,
			"bool":   true,
			"null":   nil,
			"array":  []string{"a", "b", "c"},
			"map":    map[string]string{"key": "value"},
		},
	}

	// Should be able to access all types
	assert.Equal(t, "value", example.Options["string"])
	assert.Equal(t, 42, example.Options["int"])
	assert.Equal(t, 3.14, example.Options["float"])
	assert.Equal(t, true, example.Options["bool"])
	assert.Nil(t, example.Options["null"])
	assert.IsType(t, []string{}, example.Options["array"])
	assert.IsType(t, map[string]string{}, example.Options["map"])
}
