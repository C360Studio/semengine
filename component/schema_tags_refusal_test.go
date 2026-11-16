package component

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The types below each carry one schema tag ParseSchemaTag refuses
// ("type:number" is not a schema word; floating-point values are "float").
// Each sits where a different generator reads it: the top level, a nested
// object, and the item struct of an array.

type refusedTopLevel struct {
	Name  string  `json:"name" schema:"type:string,description:Name"`
	Ratio float64 `json:"ratio" schema:"type:number,description:Ratio"`
}

type refusedInner struct {
	Ratio float64 `json:"ratio" schema:"type:number,description:Ratio"`
}

type refusedNested struct {
	Inner refusedInner `json:"inner" schema:"type:object,description:Inner block"`
}

type refusedItem struct {
	Ratio float64 `json:"ratio" schema:"type:number,description:Ratio"`
}

type refusedArrayItem struct {
	Items []refusedItem `json:"items" schema:"type:array,description:Items"`
}

// TestGenerateConfigSchemaPanicsOnRefusedTag: a schema tag ParseSchemaTag
// refuses is a programming error, so schema generation panics with a message
// naming the struct type, the field and the tag, instead of leaving the field
// out of the schema without a word.
func TestGenerateConfigSchemaPanicsOnRefusedTag(t *testing.T) {
	const tag = "type:number,description:Ratio"
	tests := []struct {
		name       string
		configType reflect.Type
		structName string // the struct that declares the refused field
	}{
		{"top level", reflect.TypeOf(refusedTopLevel{}), "refusedTopLevel"},
		{"nested object", reflect.TypeOf(refusedNested{}), "refusedInner"},
		{"array item struct", reflect.TypeOf(refusedArrayItem{}), "refusedItem"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, panicked := recoverMessage(func() { GenerateConfigSchema(tt.configType) })
			if !panicked {
				t.Fatalf("GenerateConfigSchema(%s) returned without panicking; want a panic naming field Ratio and tag %q", tt.configType, tag)
			}
			for _, want := range []string{tt.structName, "Ratio", tag} {
				if !strings.Contains(msg, want) {
					t.Errorf("panic message %q does not name %q", msg, want)
				}
			}
		})
	}
}

// recoverMessage runs f and reports the text of its panic, if it panicked.
func recoverMessage(f func()) (msg string, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			msg, panicked = fmt.Sprint(r), true
		}
	}()
	f()
	return "", false
}
