package service

import (
	"reflect"
	"regexp"
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
}

// schemaNamePattern is the component-name grammar OpenAPI 3.0 allows.
var schemaNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// TestTypeNameFromReflectNamesEachShapeApart: a pointer is named by its element and a named type
// keeps its bare name, while a slice, an array, a map and an anonymous struct get names that differ
// from their element's and from one another, each a valid OpenAPI component name (#349; design D6).
// The pin named []X and map[string]X "X", so a collection's schema overwrote its element's.
//
// spec: operator-surface / /openapi.json documents the mounted routes
func TestTypeNameFromReflectNamesEachShapeApart(t *testing.T) {
	require.Equal(t, "schemaSample", TypeNameFromReflect(reflect.TypeFor[schemaSample]()))
	require.Equal(t, "schemaSample", TypeNameFromReflect(reflect.TypeFor[*schemaSample]()))

	shapes := map[string]reflect.Type{
		"slice":            reflect.TypeFor[[]schemaSample](),
		"array":            reflect.TypeFor[[2]schemaSample](),
		"map":              reflect.TypeFor[map[string]schemaSample](),
		"anonymous struct": reflect.TypeFor[struct{ Name string }](),
	}
	seen := map[string]string{"schemaSample": "the element"}
	for shape, typ := range shapes {
		name := TypeNameFromReflect(typ)
		require.Regexp(t, schemaNamePattern, name, "%s (%s)", shape, typ)
		if other, taken := seen[name]; taken {
			t.Errorf("%s (%s) is named %q, as %s is", shape, typ, name, other)
		}
		seen[name] = shape
	}
}
