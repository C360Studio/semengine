package component

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
)

// requireInvalid fails unless err carries the invalid class.
func requireInvalid(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("accepted, want refused as invalid")
	}
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorInvalid {
		t.Fatalf("error %v is not classified invalid", err)
	}
}

// nestedArrays is a JSON value holding the number 1 under depth arrays.
func nestedArrays(depth int) string {
	return strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth)
}

// arrayOfZeros is a JSON array of n zeros.
func arrayOfZeros(n int) string {
	return "[" + strings.TrimSuffix(strings.Repeat("0,", n), ",") + "]"
}

// TestValidateFactoryConfigRefusesHostileConfigPastEachLimit proves the gate
// every CreateComponent passes a configuration through before the factory runs
// (ValidateFactoryConfig, and validateValue under it): a configuration at each
// limit the validator states is accepted, one past it is refused as invalid,
// and a refused value is found wherever it is nested. The limits: MaxJSONSize
// bytes, a value under at most ten containers, 1000 elements in an array,
// MaxStringLength bytes in a string or a key, no control character but tab,
// newline and carriage return, and a number a float64 can hold. Each refused
// case differs from an accepted one in that one respect only.
func TestValidateFactoryConfigRefusesHostileConfigPastEachLimit(t *testing.T) {
	long := strings.Repeat("a", MaxStringLength)
	tests := []struct {
		name   string
		config string
		refuse bool
	}{
		{name: "empty", config: ""},
		{name: "empty object", config: `{}`},
		{name: "MaxJSONSize bytes", config: `{}` + strings.Repeat(" ", MaxJSONSize-2)},
		{name: "one byte over MaxJSONSize", config: `{}` + strings.Repeat(" ", MaxJSONSize-1), refuse: true},
		{name: "malformed", config: `{"a":`, refuse: true},
		{name: "value under ten arrays", config: nestedArrays(10)},
		{name: "value under eleven arrays", config: nestedArrays(11), refuse: true},
		{name: "1000 elements", config: arrayOfZeros(1000)},
		{name: "1001 elements", config: arrayOfZeros(1001), refuse: true},
		{name: "string of MaxStringLength bytes", config: `{"s":"` + long + `"}`},
		{name: "string one byte longer", config: `{"s":"` + long + `a"}`, refuse: true},
		{name: "key of MaxStringLength bytes", config: `{"` + long + `":1}`},
		{name: "key one byte longer", config: `{"` + long + `a":1}`, refuse: true},
		{name: "tab, newline and carriage return", config: `{"s":"a\tb\nc\rd"}`},
		{name: "null byte in a string", config: `{"s":"a\u0000b"}`, refuse: true},
		{name: "control character in a string", config: `{"s":"a\u0007b"}`, refuse: true},
		{name: "control character in a key", config: `{"a\u001bb":1}`, refuse: true},
		{name: "integer beyond int64", config: `{"n":9223372036854775808}`},
		{name: "fraction and large exponent", config: `{"n":[1.5,-2e300]}`},
		{name: "number beyond float64", config: `{"n":1e400}`, refuse: true},
		{name: "booleans and null", config: `{"b":true,"c":false,"z":null}`},
		{name: "control character deep in arrays and objects", config: `{"a":{"b":[1,{"c":"\u0001"}]}}`, refuse: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFactoryConfig(json.RawMessage(tt.config))
			if !tt.refuse {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			requireInvalid(t, err)
		})
	}
}

type safeUnmarshalTarget struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

var errSafeUnmarshalNoPort = errors.New("port is required")

type validatingTarget struct {
	Port int `json:"port"`
}

func (v *validatingTarget) Validate() error {
	if v.Port == 0 {
		return errSafeUnmarshalNoPort
	}
	return nil
}

// TestSafeUnmarshalValidatesBeforeDecoding proves SafeUnmarshal's order: a
// configuration ValidateFactoryConfig refuses is refused as invalid before any
// field of the target is written; an empty configuration leaves the target's
// defaults; a target that is not a pointer, or JSON that does not fit it, is
// refused as invalid; and a target that validates itself has its Validate run
// on the decoded value, its refusal returned.
func TestSafeUnmarshalValidatesBeforeDecoding(t *testing.T) {
	defaults := safeUnmarshalTarget{Name: "default", Port: 9}

	t.Run("decodes a valid configuration", func(t *testing.T) {
		target := defaults
		if err := SafeUnmarshal(json.RawMessage(`{"name":"edge","port":7}`), &target); err != nil {
			t.Fatal(err)
		}
		if want := (safeUnmarshalTarget{Name: "edge", Port: 7}); target != want {
			t.Fatalf("target = %+v, want %+v", target, want)
		}
	})

	t.Run("empty configuration keeps the defaults", func(t *testing.T) {
		target := defaults
		if err := SafeUnmarshal(nil, &target); err != nil {
			t.Fatal(err)
		}
		if target != defaults {
			t.Fatalf("target = %+v, want the defaults %+v", target, defaults)
		}
	})

	t.Run("hostile configuration is refused before decoding", func(t *testing.T) {
		target := defaults
		err := SafeUnmarshal(json.RawMessage(`{"name":"edge","port":7,"note":"\u0007"}`), &target)
		requireInvalid(t, err)
		if target != defaults {
			t.Fatalf("target = %+v after a refused configuration, want the defaults %+v", target, defaults)
		}
	})

	t.Run("target that is not a pointer", func(t *testing.T) {
		requireInvalid(t, SafeUnmarshal(json.RawMessage(`{"port":7}`), defaults))
	})

	t.Run("configuration that does not fit the target", func(t *testing.T) {
		target := defaults
		requireInvalid(t, SafeUnmarshal(json.RawMessage(`{"port":"seven"}`), &target))
	})

	t.Run("target's own validation refuses", func(t *testing.T) {
		var target validatingTarget
		err := SafeUnmarshal(json.RawMessage(`{"port":0}`), &target)
		if !errors.Is(err, errSafeUnmarshalNoPort) {
			t.Fatalf("error = %v, want the target's own refusal", err)
		}
	})

	t.Run("target's own validation accepts", func(t *testing.T) {
		var target validatingTarget
		if err := SafeUnmarshal(json.RawMessage(`{"port":3}`), &target); err != nil {
			t.Fatal(err)
		}
		if target.Port != 3 {
			t.Fatalf("port = %d, want 3", target.Port)
		}
	})
}
