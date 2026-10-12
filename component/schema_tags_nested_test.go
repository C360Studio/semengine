package component

import (
	"reflect"
	"sort"
	"testing"
)

// The types below are the inputs of the nested-schema tests. Each field's
// comment says what the generated property must be; the expected schemas in
// the tests are written from those comments, not from the generator.

type nestedSchemaHost struct {
	Host    string `json:"host" schema:"required,type:string,description:Host name"` // string, required
	Retries int    `json:"retries" description:"Retry count"`                        // inferred int
}

type nestedSchemaLimits struct {
	Burst   int               `json:"burst" schema:"required,type:int,description:Burst size,min:1,max:64,default:8,category:advanced"`
	Mode    string            `json:"mode" schema:"type:enum,enum:fast|safe,default:safe"` // description falls back to the name
	Broken  string            `json:"broken" schema:"type:nonsense"`                       // invalid tag: skipped
	Ignored string            `json:"-" schema:"type:string"`                              // json "-": skipped
	NoJSON  string            `schema:"type:string"`                                       // no json tag: skipped
	Unnamed string            `json:",omitempty" schema:"type:string"`                     // empty json name: skipped
	Inner   *nestedSchemaHost `json:"inner" schema:"type:object,description:Inner block"`  // pointer to struct: expanded
	Tags    []string          `json:"tags" schema:"type:array,description:Tags"`           // items string
	Label   string            `json:"label" description:"Display label"`                   // inferred string
	Count   uint16            `json:"count"`                                               // inferred int, name as description
	Ratio   float32           `json:"ratio" description:"Ratio"`                           // inferred float
	On      *bool             `json:"on" description:"Switch"`                             // pointer: inferred bool
	Matrix  [][]int           `json:"matrix" description:"Matrix"`                         // array of array of integer
	Lookup  map[string]int    `json:"lookup"`                                              // inferred object
	Child   nestedSchemaHost  `json:"child" description:"Child"`                           // inferred object, expanded
	Any     any               `json:"any"`                                                 // other kinds fall back to string
}

type nestedSchemaRoute struct {
	Path    string           `json:"path" schema:"required,type:string,description:Route path"`
	Methods []string         `json:"methods" schema:"type:array,description:Methods"` // items string
	Weight  float64          `json:"weight" description:"Weight"`                     // inferred float
	Flags   []bool           `json:"flags"`                                           // items boolean
	Meta    map[string]int   `json:"meta"`                                            // inferred object
	Extra   []map[string]int `json:"extra"`                                           // items object
	Opaque  []any            `json:"opaque"`                                          // items string
}

type nestedSchemaConfig struct {
	Limits  nestedSchemaLimits   `json:"limits" schema:"type:object,description:Rate limits"`
	Routes  []*nestedSchemaRoute `json:"routes" schema:"type:array,description:Routes"`
	Weights *[]float64           `json:"weights" schema:"type:array,description:Weights"`
	Cache   struct{}             `json:"cache" schema:"type:cache,description:Cache settings"`
}

// wantCacheFields is the cache field metadata written from the tags of
// internal/cache.Config: every field editable, with its type, the strategy's
// four values and the size's minimum of one.
func wantCacheFields() map[string]CacheFieldInfo {
	one := 1
	return map[string]CacheFieldInfo{
		"enabled":          {Type: "bool", Editable: true},
		"strategy":         {Type: "enum", Editable: true, Enum: []string{"simple", "lru", "ttl", "hybrid"}},
		"max_size":         {Type: "int", Editable: true, Min: &one},
		"ttl":              {Type: "string", Editable: true},
		"cleanup_interval": {Type: "string", Editable: true},
	}
}

// TestGenerateConfigSchemaExpandsNestedStructsAndArrays proves the schema a
// component publishes describes what is inside its object and array fields: a
// field tagged type:object carries its struct's properties and required list,
// at every depth and through pointers; a field tagged type:array carries the
// schema of its elements, a struct element expanded the same way; a nested
// field without a schema tag gets a type inferred from its Go type and the
// description from its description tag; a nested field with no json name or an
// invalid schema tag is left out; and a field tagged type:cache carries the
// cache's field metadata.
func TestGenerateConfigSchemaExpandsNestedStructsAndArrays(t *testing.T) {
	one, sixtyFour := 1, 64
	host := map[string]PropertySchema{
		"host":    {Type: "string", Description: "Host name"},
		"retries": {Type: "int", Description: "Retry count"},
	}
	want := map[string]PropertySchema{
		"limits": {
			Type: "object", Description: "Rate limits", Required: []string{"burst"},
			Properties: map[string]PropertySchema{
				"burst": {
					Type: "int", Description: "Burst size", Category: "advanced", Default: 8,
					Minimum: &one, Maximum: &sixtyFour,
				},
				"mode": {Type: "enum", Description: "mode", Default: "safe", Enum: []string{"fast", "safe"}},
				"inner": {
					Type: "object", Description: "Inner block", Properties: host, Required: []string{"host"},
				},
				"tags":   {Type: "array", Description: "Tags", Items: &PropertySchema{Type: "string"}},
				"label":  {Type: "string", Description: "Display label"},
				"count":  {Type: "int", Description: "count"},
				"ratio":  {Type: "float", Description: "Ratio"},
				"on":     {Type: "bool", Description: "Switch"},
				"matrix": {Type: "array", Description: "Matrix", Items: &PropertySchema{Type: "array", Items: &PropertySchema{Type: "integer"}}},
				"lookup": {Type: "object", Description: "lookup"},
				"child":  {Type: "object", Description: "Child", Properties: host, Required: []string{"host"}},
				"any":    {Type: "string", Description: "any"},
			},
		},
		"routes": {
			Type: "array", Description: "Routes",
			Items: &PropertySchema{
				Type: "object", Required: []string{"path"},
				Properties: map[string]PropertySchema{
					"path":    {Type: "string", Description: "Route path"},
					"methods": {Type: "array", Description: "Methods", Items: &PropertySchema{Type: "string"}},
					"weight":  {Type: "float", Description: "Weight"},
					"flags":   {Type: "array", Description: "flags", Items: &PropertySchema{Type: "boolean"}},
					"meta":    {Type: "object", Description: "meta"},
					"extra":   {Type: "array", Description: "extra", Items: &PropertySchema{Type: "object"}},
					"opaque":  {Type: "array", Description: "opaque", Items: &PropertySchema{Type: "string"}},
				},
			},
		},
		"weights": {Type: "array", Description: "Weights", Items: &PropertySchema{Type: "number"}},
		"cache":   {Type: "cache", Description: "Cache settings", CacheFields: wantCacheFields()},
	}

	got := GenerateConfigSchema(reflect.TypeOf(nestedSchemaConfig{}))

	if gotKeys, wantKeys := propertyNames(got.Properties), propertyNames(want); !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("top-level properties = %v, want %v", gotKeys, wantKeys)
	}
	if len(got.Required) != 0 {
		t.Errorf("top-level required = %v, want none", got.Required)
	}
	for name, wantProperty := range want {
		gotProperty := got.Properties[name]
		if gotKeys, wantKeys := propertyNames(gotProperty.Properties), propertyNames(wantProperty.Properties); !reflect.DeepEqual(gotKeys, wantKeys) {
			t.Errorf("%s: nested properties = %v, want %v", name, gotKeys, wantKeys)
		}
		for nested, wantNested := range wantProperty.Properties {
			if gotNested := gotProperty.Properties[nested]; !reflect.DeepEqual(gotNested, wantNested) {
				t.Errorf("%s.%s =\n%#v\nwant\n%#v", name, nested, gotNested, wantNested)
			}
		}
		if !reflect.DeepEqual(gotProperty, wantProperty) {
			t.Errorf("%s =\n%#v\nwant\n%#v", name, gotProperty, wantProperty)
		}
	}
}

// TestGenerateCacheFieldSchemaDescribesEveryCacheField proves the metadata a
// type:cache field publishes names each field of the cache configuration by
// its json name, marks it editable, and carries its type, the strategy's
// allowed values and the size's minimum, as the cache configuration's tags
// declare them.
func TestGenerateCacheFieldSchemaDescribesEveryCacheField(t *testing.T) {
	got := GenerateCacheFieldSchema()
	want := wantCacheFields()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GenerateCacheFieldSchema() =\n%#v\nwant\n%#v", got, want)
	}
}

func propertyNames(properties map[string]PropertySchema) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
