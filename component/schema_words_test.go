package component

import (
	"reflect"
	"testing"
)

// oneWordNested is a nested object whose fields carry no schema tag, so their
// types are inferred from the Go types.
type oneWordNested struct {
	Count int       `json:"count"`
	Ratio float64   `json:"ratio"`
	On    bool      `json:"on"`
	Flags []bool    `json:"flags"`
	Grid  [][]int   `json:"grid"`
	Steps []float64 `json:"steps"`
}

type oneWordConfig struct {
	Count  int           `json:"count" schema:"type:int"`
	Ratio  float64       `json:"ratio" schema:"type:float"`
	On     bool          `json:"on" schema:"type:bool"`
	Counts []int         `json:"counts" schema:"type:array"`
	Ratios []float64     `json:"ratios" schema:"type:array"`
	Flags  []bool        `json:"flags" schema:"type:array"`
	Grid   [][]int       `json:"grid" schema:"type:array"`
	Nested oneWordNested `json:"nested" schema:"type:object"`
}

// TestGeneratedSchemaUsesOneWordPerType proves a generated schema spells each
// basic type one way: int, float and bool, in properties, in nested objects
// and in array items at every depth. The expected words are written here, not
// read from the generator or the tag parser.
func TestGeneratedSchemaUsesOneWordPerType(t *testing.T) {
	schema := GenerateConfigSchema(reflect.TypeOf(oneWordConfig{}))

	nested := schema.Properties["nested"].Properties
	checks := []struct {
		path string
		got  *PropertySchema
		want string
	}{
		{"count", ptrTo(schema.Properties["count"]), "int"},
		{"ratio", ptrTo(schema.Properties["ratio"]), "float"},
		{"on", ptrTo(schema.Properties["on"]), "bool"},
		{"counts[]", schema.Properties["counts"].Items, "int"},
		{"ratios[]", schema.Properties["ratios"].Items, "float"},
		{"flags[]", schema.Properties["flags"].Items, "bool"},
		{"grid[]", schema.Properties["grid"].Items, "array"},
		{"grid[][]", itemsOf(schema.Properties["grid"].Items), "int"},
		{"nested.count", ptrTo(nested["count"]), "int"},
		{"nested.ratio", ptrTo(nested["ratio"]), "float"},
		{"nested.on", ptrTo(nested["on"]), "bool"},
		{"nested.flags[]", nested["flags"].Items, "bool"},
		{"nested.grid[][]", itemsOf(nested["grid"].Items), "int"},
		{"nested.steps[]", nested["steps"].Items, "float"},
	}
	for _, c := range checks {
		if c.got == nil {
			t.Errorf("%s: no schema generated", c.path)
			continue
		}
		if c.got.Type != c.want {
			t.Errorf("%s: type %q, want %q", c.path, c.got.Type, c.want)
		}
	}
}

func ptrTo(p PropertySchema) *PropertySchema { return &p }

func itemsOf(p *PropertySchema) *PropertySchema {
	if p == nil {
		return nil
	}
	return p.Items
}
