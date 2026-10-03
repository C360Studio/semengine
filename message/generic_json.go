// Package message provides the GenericJSON payload for StreamKit.
package message

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
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
// A string, map key or string value anywhere in Data that is not valid UTF-8
// is refused with an invalid-data error rather than written with its invalid
// bytes replaced.
func (g *GenericJSONPayload) MarshalJSON() ([]byte, error) {
	// encoding/json writes each invalid byte of a string as U+FFFD, so the data on the wire would
	// differ from Data with no error; refuse it (owner ruling, PR #48 comment 5970334875).
	if problem, bad := invalidUTF8(reflect.ValueOf(g.Data), "data"); bad {
		return nil, errs.WrapInvalid(errors.New(problem),
			"GenericJSONPayload", "MarshalJSON", "data")
	}
	// Use alias to avoid infinite recursion
	type Alias GenericJSONPayload
	data, err := json.Marshal((*Alias)(g))
	if err != nil {
		return nil, err
	}
	// A json.Marshaler's output is copied as it is, invalid bytes included, which the walk above
	// does not see.
	if !utf8.Valid(data) {
		return nil, errs.WrapInvalid(fmt.Errorf("data encodes to text that is not valid UTF-8"),
			"GenericJSONPayload", "MarshalJSON", "data")
	}
	return data, nil
}

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
)

// invalidUTF8 describes the first string encoding/json would write from v that is not valid
// UTF-8: a string, a map key, or the text of a TextMarshaler, at any depth. A json.Marshaler is
// not descended into (its output is checked whole), nor are []byte (written as base64),
// unexported struct fields and fields tagged "-" (not written). An embedded struct with no JSON
// name is walked field by field, as encoding/json flattens it into its parent.
func invalidUTF8(v reflect.Value, at string) (string, bool) {
	return utf8Walker{onPath: map[walkKey]bool{}}.walk(v, at)
}

// walkKey names a pointer, map or slice on the path being walked, by address, type and length,
// so a cycle stops the walk (json.Marshal then reports it) instead of recursing without end.
type walkKey struct {
	ptr uintptr
	typ reflect.Type
	n   int
}

type utf8Walker struct{ onPath map[walkKey]bool }

func (w utf8Walker) walk(v reflect.Value, at string) (string, bool) {
	if !v.IsValid() {
		return "", false
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return "", false
		}
	}
	if _, ok := marshaler(v, jsonMarshalerType); ok {
		return "", false
	}
	if m, ok := marshaler(v, textMarshalerType); ok {
		text, err := m.Interface().(encoding.TextMarshaler).MarshalText()
		// An error is json.Marshal's to report.
		return at + " is not valid UTF-8", err == nil && !utf8.Valid(text)
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice:
		key := walkKey{ptr: v.Pointer(), typ: v.Type()}
		if v.Kind() == reflect.Slice {
			key.n = v.Len()
		}
		if w.onPath[key] {
			return "", false
		}
		w.onPath[key] = true
		defer delete(w.onPath, key)
	}
	switch v.Kind() {
	case reflect.String:
		return at + " is not valid UTF-8", !utf8.ValidString(v.String())
	case reflect.Pointer, reflect.Interface:
		return w.walk(v.Elem(), at)
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			key := fmt.Sprintf("%s[%q]", at, fmt.Sprint(iter.Key()))
			if path, bad := w.walk(iter.Key(), key+" (key)"); bad {
				return path, true
			}
			if path, bad := w.walk(iter.Value(), key); bad {
				return path, true
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return "", false
		}
		for i := 0; i < v.Len(); i++ {
			if path, bad := w.walk(v.Index(i), fmt.Sprintf("%s[%d]", at, i)); bad {
				return path, true
			}
		}
	case reflect.Struct:
		return w.fields(v, at)
	}
	return "", false
}

// marshaler returns the value whose method encoding/json calls when v implements iface, or when
// v is addressable and its pointer does.
func marshaler(v reflect.Value, iface reflect.Type) (reflect.Value, bool) {
	if v.Type().Implements(iface) {
		return v, true
	}
	if v.Kind() != reflect.Pointer && v.CanAddr() && reflect.PointerTo(v.Type()).Implements(iface) {
		return v.Addr(), true
	}
	return reflect.Value{}, false
}

// fields walks the fields encoding/json writes from struct v, by its rules: a field tagged "-" is
// not written; an embedded struct (or pointer to one) with no JSON name is flattened, its own
// Marshaler methods not called and its fields written in its parent's place; an embedded struct
// with a JSON name is written as a field even when unexported; any other unexported field is not
// written.
func (w utf8Walker) fields(v reflect.Value, at string) (string, bool) {
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		value := v.Field(i)
		if field.Anonymous {
			embedded := field.Type
			if embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() != reflect.Struct {
				if !field.IsExported() {
					continue
				}
			} else if strings.Split(tag, ",")[0] == "" {
				if value.Kind() == reflect.Pointer {
					if value.IsNil() {
						continue
					}
					value = value.Elem()
				}
				if path, bad := w.fields(value, at+"."+field.Name); bad {
					return path, true
				}
				continue
			}
		} else if !field.IsExported() {
			continue
		}
		if path, bad := w.walk(value, at+"."+field.Name); bad {
			return path, true
		}
	}
	return "", false
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
