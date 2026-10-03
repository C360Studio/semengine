// Package message provides the GenericJSON payload for StreamKit.
package message

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"unicode/utf8"

	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
)

func buildGenericJSONPayload(fields map[string]any) (any, error) {
	msg := &GenericJSONPayload{}

	if v, ok := fields["data"].(map[string]any); ok {
		msg.Data = v
	}

	if err := msg.Validate(); err != nil {
		return nil, errs.Wrap(err, "GenericJSONPayload", "buildGenericJSONPayload", "validation failed")
	}

	return msg, nil
}

// RegisterPayloads registers the GenericJSON payload type (core.json.v1)
// with the supplied registry. Called from payloadbuiltins.Register at
// process bootstrap.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Domain:      "core",
		Category:    "json",
		Version:     "v1",
		Description: "Generic JSON payload for testing, prototyping, and basic data processing",
		Factory: func() any {
			return &GenericJSONPayload{}
		},
		Builder: buildGenericJSONPayload,
		Example: map[string]any{
			"data": map[string]any{
				"sensor_id":   "temp-001",
				"temperature": 23.5,
				"unit":        "celsius",
			},
		},
	})
}

// GenericJSONPayload provides a simple, explicitly flexible payload type
// for testing, prototyping, and basic data processing flows.
//
// This is an intentional, well-known type (core.json.v1) designed for:
//   - Rapid prototyping of flows
//   - Integration testing
//   - Basic JSON data processing (filter, map, transform)
//   - Simple ETL pipelines
//
// Components that work with GenericJSON (JSONFilter, JSONMap) explicitly
// declare they require "core.json.v1" type, providing type safety while
// maintaining flexibility for arbitrary JSON structures.
//
// Example usage:
//
//	payload := &GenericJSONPayload{
//	    Data: map[string]any{
//	        "sensor_id": "temp-001",
//	        "temperature": 23.5,
//	        "unit": "celsius",
//	    },
//	}
type GenericJSONPayload struct {
	// Data contains the JSON payload as a map.
	// This supports arbitrary JSON structures while remaining type-safe
	// at the component level (components declare they work with core.json.v1).
	Data map[string]any `json:"data"`
}

// NewGenericJSON creates a new GenericJSON payload with the given data.
func NewGenericJSON(data map[string]any) *GenericJSONPayload {
	return &GenericJSONPayload{
		Data: data,
	}
}

// Schema returns the payload type identifier for GenericJSON.
// Always returns core.json.v1 as this is the well-known type for
// generic JSON processing in StreamKit.
func (g *GenericJSONPayload) Schema() Type {
	return Type{
		Domain:   "core",
		Category: "json",
		Version:  "v1",
	}
}

// Validate performs basic validation on the GenericJSON payload.
// Ensures the data map is not nil.
func (g *GenericJSONPayload) Validate() error {
	if g.Data == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "GenericJSONPayload", "Validate", "data cannot be nil")
	}
	return nil
}

// MarshalJSON serializes the GenericJSON payload to JSON format.
// The output format matches the input structure with a "data" wrapper.
//
// Data must be JSON-shaped: map[string]any, []any, string, a Go number (int,
// int8 to int64, uint, uint8 to uint64, uintptr, float32, float64),
// json.Number, bool or nil, nested to any depth. Any other value (a struct,
// time.Time, a pointer, a named type, a typed map or slice such as
// map[string]string or []string) is refused with an invalid-data error that
// names its type and its path, such as data.a[2].b; so is a string or map key
// that is not valid UTF-8, and a map or list that contains itself.
func (g *GenericJSONPayload) MarshalJSON() ([]byte, error) {
	// Only the ruled kinds are accepted (#9 comments 5972117486 and 5972208367), so encoding/json
	// runs no custom method and writes every string as it is; invalid UTF-8 is refused rather than
	// written as U+FFFD (#9 comment 5970334875).
	if err := (shapeCheck{onPath: map[shapeKey]bool{}}).check(g.Data, "data"); err != nil {
		return nil, errs.WrapInvalid(err, "GenericJSONPayload", "MarshalJSON", "data")
	}
	// Use alias to avoid infinite recursion
	type Alias GenericJSONPayload
	return json.Marshal((*Alias)(g))
}

// shapeKey names a map or list on the path being checked, by address and length, so a map or
// list that contains itself is refused instead of checked without end.
type shapeKey struct {
	ptr uintptr
	n   int
}

type shapeCheck struct{ onPath map[shapeKey]bool }

func (c shapeCheck) check(v any, at string) error {
	switch v := v.(type) {
	case nil, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64:
		return nil
	case string:
		return validString(v, at)
	case json.Number:
		return validString(string(v), at)
	case map[string]any:
		return c.object(v, at)
	case []any:
		return c.list(v, at)
	default:
		return fmt.Errorf("%s: %T is not JSON-shaped (want map[string]any, []any, string, a Go number, "+
			"json.Number, bool or nil)", at, v)
	}
}

func validString(s, at string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s is not valid UTF-8", at)
	}
	return nil
}

func (c shapeCheck) enter(v any, n int, at string) (func(), error) {
	key := shapeKey{ptr: reflect.ValueOf(v).Pointer(), n: n}
	if c.onPath[key] {
		return nil, fmt.Errorf("%s: %T contains itself", at, v)
	}
	c.onPath[key] = true
	return func() { delete(c.onPath, key) }, nil
}

func (c shapeCheck) object(m map[string]any, at string) error {
	if len(m) == 0 {
		return nil
	}
	leave, err := c.enter(m, 0, at)
	if err != nil {
		return err
	}
	defer leave()
	// Sorted, as encoding/json writes them, so the refusal names the same key every time.
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if !utf8.ValidString(k) {
			return fmt.Errorf("%s: key %q is not valid UTF-8", at, k)
		}
		if err := c.check(m[k], member(at, k)); err != nil {
			return err
		}
	}
	return nil
}

func (c shapeCheck) list(l []any, at string) error {
	if len(l) == 0 {
		return nil
	}
	leave, err := c.enter(l, len(l), at)
	if err != nil {
		return err
	}
	defer leave()
	for i, e := range l {
		if err := c.check(e, fmt.Sprintf("%s[%d]", at, i)); err != nil {
			return err
		}
	}
	return nil
}

// member writes the path to key k of the map at at: data.k for a plain name, data["a b"] otherwise.
func member(at, k string) string {
	plain := k != ""
	for _, r := range k {
		if !(r == '_' || r == '-' || '0' <= r && r <= '9' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z') {
			plain = false
			break
		}
	}
	if plain {
		return at + "." + k
	}
	return fmt.Sprintf("%s[%q]", at, k)
}

// UnmarshalJSON deserializes JSON data into the GenericJSON payload.
func (g *GenericJSONPayload) UnmarshalJSON(data []byte) error {
	// Use alias to avoid infinite recursion
	type Alias GenericJSONPayload
	return json.Unmarshal(data, (*Alias)(g))
}

// RuleFields implements RuleReadable: the whole data map IS this payload's
// declared projection.
//
// core.json.v1 exists to carry arbitrary caller-supplied JSON with no schema
// of its own, so there is no narrower honest answer — the payload's author is
// the caller, and the caller already chose every key. It is also the behavior
// every rule written before RuleReadable existed depends on, which is why
// exposing Data verbatim keeps them all valid.
func (g *GenericJSONPayload) RuleFields() map[string]any {
	return g.Data
}
