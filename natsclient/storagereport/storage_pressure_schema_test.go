package storagereport

import (
	"reflect"
	"testing"

	"github.com/c360studio/semengine/component"
)

// TestStoragePressureThresholdsSchemaTypesHeadroomFloat pins the schema of
// the headroom fields: each is a free fraction (0..1), so each is in the
// schema and typed "float", the component-registration word for
// floating-point values.
func TestStoragePressureThresholdsSchemaTypesHeadroomFloat(t *testing.T) {
	schema := component.GenerateConfigSchema(reflect.TypeOf(StoragePressureThresholds{}))
	for _, name := range []string{"warning_headroom", "high_headroom", "critical_headroom"} {
		property, ok := schema.Properties[name]
		if !ok {
			t.Errorf("schema has no property %q", name)
			continue
		}
		if property.Type != "float" {
			t.Errorf("%s type = %q, want \"float\"", name, property.Type)
		}
	}
}
