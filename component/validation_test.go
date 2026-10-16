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

// TestValidateFactoryConfigRefusesDataAfterTheFirstValue proves the
// configuration check reads the whole input: anything but whitespace after the
// first JSON value is refused as invalid, and CreateComponent refuses such a
// configuration before its factory runs.
func TestValidateFactoryConfigRefusesDataAfterTheFirstValue(t *testing.T) {
	tests := []struct {
		name   string
		config string
		refuse bool
	}{
		{name: "an object pasted twice", config: `{} {}`, refuse: true},
		{name: "a bare word after the object", config: `{} x`, refuse: true},
		{name: "a second object with fields", config: `{} {"s":"x"}`, refuse: true},
		{name: "a closing brace after the object", config: `{} }`, refuse: true},
		{name: "a newline after the object", config: "{}\n"},
		{name: "spaces, tabs and newlines around the object", config: " \t{}\r\n "},
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

	t.Run("CreateComponent never calls the factory", func(t *testing.T) {
		registry := NewRegistry()
		called := false
		factory := func(json.RawMessage, Dependencies) (Discoverable, error) {
			called = true
			return &declarationTestComponent{}, nil
		}
		requireNoError(t, registry.RegisterWithConfig(RegistrationConfig{
			Name: "twice", Type: "processor", Factory: factory, Ports: noPorts,
		}))
		_, err := registry.CreateComponent(
			"worker", declarationTestConfig("twice", `{} {"s":"x"}`), declarationTestDeps(), nil)
		requireInvalid(t, err)
		if called {
			t.Fatal("factory ran on a configuration with data after the first value")
		}
	})
}

type unknownKeyTarget struct {
	Port int `json:"port"`
}

// TestSafeUnmarshalRefusesANilTarget proves SafeUnmarshal checks its target
// before anything else: a nil target, or a nil pointer, is refused as invalid
// whether the configuration is valid or empty, and nothing panics.
func TestSafeUnmarshalRefusesANilTarget(t *testing.T) {
	var nilPointer *unknownKeyTarget
	tests := []struct {
		name   string
		config json.RawMessage
		target any
	}{
		{name: "valid configuration, nil target", config: json.RawMessage(`{"port":7}`), target: nil},
		{name: "empty configuration, nil target", config: nil, target: nil},
		{name: "valid configuration, nil pointer", config: json.RawMessage(`{"port":7}`), target: nilPointer},
		{name: "empty configuration, nil pointer", config: nil, target: nilPointer},
		{name: "empty configuration, not a pointer", config: nil, target: unknownKeyTarget{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("SafeUnmarshal panicked: %v", r)
				}
			}()
			requireInvalid(t, SafeUnmarshal(tt.config, tt.target))
		})
	}
}

// TestSafeUnmarshalRefusesAnUnknownKey proves SafeUnmarshal decodes strictly:
// a key the target struct does not define is refused as invalid with an error
// naming the key, and so is data after the first JSON value.
func TestSafeUnmarshalRefusesAnUnknownKey(t *testing.T) {
	t.Run("a misspelled key", func(t *testing.T) {
		var target unknownKeyTarget
		err := SafeUnmarshal(json.RawMessage(`{"prot":7}`), &target)
		requireInvalid(t, err)
		if !strings.Contains(err.Error(), `"prot"`) {
			t.Fatalf("error %q does not name the key \"prot\"", err)
		}
	})

	t.Run("data after the first value", func(t *testing.T) {
		var target unknownKeyTarget
		requireInvalid(t, SafeUnmarshal(json.RawMessage(`{"port":7} {"port":8}`), &target))
	})
}
