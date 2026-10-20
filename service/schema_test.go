package service

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// schemaSample has one field per struct-tag case SchemaFromType reads.
type schemaSample struct {
	Name     string    `json:"name" description:"the name"`
	Count    int       `json:"count,omitempty"`
	Optional *string   `json:"optional"`
	Skipped  string    `json:"-"`
	Untagged bool      // no json tag: named by its Go name
	When     time.Time `json:"when"`
	hidden   string    // unexported: skipped
}

// TestSchemaFromType: one case per kind schema.go handles (design D6). The expected schemas are
// written from JSON Schema draft-07: integer, number, boolean and string types, "date-time" and
// "byte" formats, anyOf with null for a pointer, and an object's properties and required list.
func TestSchemaFromType(t *testing.T) {
	for _, tt := range []struct {
		name string
		typ  reflect.Type
		want map[string]any
	}{
		{"string", reflect.TypeFor[string](), map[string]any{"type": "string"}},
		{"int", reflect.TypeFor[int64](), map[string]any{"type": "integer"}},
		{"uint", reflect.TypeFor[uint8](), map[string]any{"type": "integer", "minimum": 0}},
		{"float", reflect.TypeFor[float32](), map[string]any{"type": "number"}},
		{"bool", reflect.TypeFor[bool](), map[string]any{"type": "boolean"}},
		{"time", reflect.TypeFor[time.Time](), map[string]any{"type": "string", "format": "date-time"}},
		{"bytes", reflect.TypeFor[[]byte](), map[string]any{"type": "string", "format": "byte"}},
		{"slice", reflect.TypeFor[[]int](), map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}},
		{"map", reflect.TypeFor[map[string]bool](), map[string]any{
			"type": "object", "additionalProperties": map[string]any{"type": "boolean"},
		}},
		{"pointer", reflect.TypeFor[*float64](), map[string]any{
			"anyOf": []any{map[string]any{"type": "number"}, map[string]any{"type": "null"}},
		}},
		{"interface", reflect.TypeFor[any](), map[string]any{}},
		{"unknown kind", reflect.TypeFor[chan int](), map[string]any{"type": "string"}},
		{"struct", reflect.TypeFor[schemaSample](), map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string", "description": "the name"},
				"count": map[string]any{"type": "integer"},
				"optional": map[string]any{
					"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}},
				},
				"Untagged": map[string]any{"type": "boolean"},
				"when":     map[string]any{"type": "string", "format": "date-time"},
			},
			"required": []string{"name", "Untagged", "when"},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SchemaFromType(tt.typ))
		})
	}
}

func TestTypeNameFromReflect(t *testing.T) {
	require.Equal(t, "schemaSample", TypeNameFromReflect(reflect.TypeFor[schemaSample]()))
	require.Equal(t, "schemaSample", TypeNameFromReflect(reflect.TypeFor[*schemaSample]()))
	require.Equal(t, "int", TypeNameFromReflect(reflect.TypeFor[int]()))
	require.Equal(t, "[]int", TypeNameFromReflect(reflect.TypeFor[[]int]()), "an unnamed type is named by its text")
}
