package keycase_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/service"
	"github.com/c360studio/semengine/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPayloadTypesReachOnlyShapesTheLetterCaseCheckWalks guards the walk's limits
// (setup-04a-03-boot-service-manager design D5; moved from config's
// TestConfigReachesOnlyShapesTheLetterCaseWalkerHandles): keycase.Check does not
// handle field promotion from embedded structs, keys inside slices or arrays, or a
// type that decodes or encodes itself. If a payload type it checks ever reaches one,
// this fails, so the walk is extended before the rule silently stops covering it.
func TestPayloadTypesReachOnlyShapesTheLetterCaseCheckWalks(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[config.Config](),
		reflect.TypeFor[service.ManagerConfig](),
		reflect.TypeFor[service.MetricsConfig](),
	} {
		assert.Empty(t, letterCaseWalkerGaps(typ), typ.String())
	}
}

// TestLetterCaseWalkerGapsDetectsEachShape shows the guard above can fail: each
// shape the walker does not handle is reported.
func TestLetterCaseWalkerGapsDetectsEachShape(t *testing.T) {
	type inner struct {
		A string `json:"a"`
	}
	shapes := []struct {
		name  string
		shape reflect.Type
		want  string
	}{
		{"embedded struct", reflect.TypeFor[struct{ inner }](), "embeds"},
		{"slice of structs", reflect.TypeFor[struct {
			S []inner `json:"s"`
		}](), "inside a slice or array"},
		{"array of maps", reflect.TypeFor[struct {
			A [1]map[string]string `json:"a"`
		}](), "inside a slice or array"},
		{"slice of struct pointers", reflect.TypeFor[struct {
			S []*inner `json:"s"`
		}](), "inside a slice or array"},
		{"own json.Unmarshaler behind a pointer", reflect.TypeFor[struct {
			U *gapUnmarshaler `json:"u"`
		}](), "json.Unmarshaler"},
		{"own json.Marshaler", reflect.TypeFor[struct {
			M gapMarshaler `json:"m"`
		}](), "json.Marshaler"},
	}
	for _, tc := range shapes {
		t.Run(tc.name, func(t *testing.T) {
			gaps := letterCaseWalkerGaps(tc.shape)
			require.Len(t, gaps, 1)
			assert.Contains(t, gaps[0], tc.want)
		})
	}
}

type gapUnmarshaler struct{}

func (*gapUnmarshaler) UnmarshalJSON([]byte) error { return nil }

type gapMarshaler struct{}

func (gapMarshaler) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// letterCaseWalkerGaps lists every type reachable from t that the walker does
// not handle. json.RawMessage is a payload its owner decodes, and
// types.ServiceConfig decodes into its own tagged fields, so both are allowed.
func letterCaseWalkerGaps(t reflect.Type) []string {
	unmarshaler := reflect.TypeFor[json.Unmarshaler]()
	marshaler := reflect.TypeFor[json.Marshaler]()
	rawMessage := reflect.TypeFor[json.RawMessage]()
	serviceConfig := reflect.TypeFor[types.ServiceConfig]()
	var gaps []string
	seen := map[reflect.Type]bool{}
	var visit func(reflect.Type)
	visit = func(t reflect.Type) {
		if seen[t] {
			return
		}
		seen[t] = true
		if t != rawMessage && t.Kind() != reflect.Pointer {
			if reflect.PointerTo(t).Implements(unmarshaler) && t != serviceConfig {
				gaps = append(gaps, t.String()+" has its own json.Unmarshaler")
			}
			if reflect.PointerTo(t).Implements(marshaler) {
				gaps = append(gaps, t.String()+" has its own json.Marshaler")
			}
		}
		switch t.Kind() {
		case reflect.Pointer, reflect.Map:
			visit(t.Elem())
		case reflect.Slice, reflect.Array:
			elem := t.Elem()
			for elem.Kind() == reflect.Pointer {
				elem = elem.Elem()
			}
			if elem.Kind() == reflect.Struct || elem.Kind() == reflect.Map {
				gaps = append(gaps, t.String()+" holds keys inside a slice or array")
			}
		case reflect.Struct:
			for i := range t.NumField() {
				f := t.Field(i)
				if f.Anonymous {
					gaps = append(gaps, t.String()+" embeds "+f.Type.String())
				}
				if f.IsExported() && f.Tag.Get("json") != "-" {
					visit(f.Type)
				}
			}
		}
	}
	visit(t)
	return gaps
}
