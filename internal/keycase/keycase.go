// Package keycase refuses a configuration key that differs from the name of the
// struct field it decodes into only in letter case.
//
// encoding/json matches such a key to the field anyway, so a document holding
// both spellings decodes whichever comes last, and a merge with defaults drops or
// applies it depending on what the defaults hold (#325). config checks the
// configuration file with it, and service checks every strictly decoded service
// payload (setup-04a-03-boot-service-manager design D5).
package keycase

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// Check walks value, a document decoded into any, beside t, the type it will be
// decoded into, and returns an error naming the first key (in sorted order at each
// level) that matches a field name only in another letter case, with both
// spellings: "config key Port must be written port". path prefixes the names; ""
// names the document's top level.
//
// The walk follows struct fields, pointers and map values. Map keys are names a
// deployment chooses and are not checked. A value that is not an object, a key no
// field declares in any case, a slice or array, an embedded struct's promoted
// fields, and a type that decodes itself are left to the decoder.
func Check(value any, t reflect.Type, path string) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	obj, ok := value.(map[string]any)
	if !ok || (t.Kind() != reflect.Map && t.Kind() != reflect.Struct) {
		return nil
	}
	fields := map[string]reflect.Type{}
	if t.Kind() == reflect.Struct {
		for i := range t.NumField() {
			f := t.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if !f.IsExported() || name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fields[name] = f.Type
		}
	}
	for _, key := range slices.Sorted(maps.Keys(obj)) {
		next := key
		if path != "" {
			next = path + "." + key
		}
		elem, declared := fields[key]
		if t.Kind() == reflect.Map {
			elem, declared = t.Elem(), true
		}
		if declared {
			if err := Check(obj[key], elem, next); err != nil {
				return err
			}
			continue
		}
		for name := range fields {
			if strings.EqualFold(key, name) {
				return fmt.Errorf("config key %s must be written %s", next, strings.TrimSuffix(next, key)+name)
			}
		}
	}
	return nil
}
