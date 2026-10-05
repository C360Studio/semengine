package message_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// The owner's ruling on #9 (comment 5972117486, with comment 5972208367 for time.Time):
// GenericJSONPayload.Data holds JSON-shaped values only: map[string]any, []any, string, a Go
// number, json.Number, bool and nil. Encoding refuses any other value with an error naming its
// type and path, and any string or key that is not valid UTF-8 (#9 comment 5970334875).

// requireRefused asserts that the payload, alone and in its envelope, is refused with an
// invalid-data error whose text holds each of want.
func requireRefused(t *testing.T, data map[string]any, want ...string) {
	t.Helper()
	payload := message.NewGenericJSON(data)
	encoded, err := payload.MarshalJSON()
	if err == nil || !errs.IsInvalid(err) {
		t.Fatalf("payload MarshalJSON = %s, %v; want an invalid-data error", encoded, err)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q does not name %q", err, w)
		}
	}
	if encoded, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "gw")); err == nil || !errs.IsInvalid(err) {
		t.Errorf("envelope marshal = %s, %v; want an invalid-data error", encoded, err)
	}
}

type label string

type celsius float64

type fields map[string]any

// Codex's F15 probes (PR #48 comment 5970889953): a named key type with JSON and text methods,
// a byte-element text method, and a text method whose result changes between calls.
type jsonKey string

func (jsonKey) MarshalJSON() ([]byte, error) { return []byte(`"fixed"`), nil }

type textKey string

func (textKey) MarshalText() ([]byte, error) { return []byte("fixed"), nil }

type byteText byte

func (byteText) MarshalText() ([]byte, error) { return []byte{0xff}, nil }

type changingText struct{ calls int }

func (v *changingText) MarshalText() ([]byte, error) {
	v.calls++
	if v.calls == 1 {
		return []byte("valid"), nil
	}
	return []byte{0xff}, nil
}

// Codex's F16 probes: a shadowed embedded field and ambiguous promoted fields.
type inner struct{ Name string }
type selected struct {
	inner
	Name string
}
type left struct{ Name string }
type right struct{ Name string }
type ambiguous struct {
	left
	right
}

// embeddedCycle is Codex's self-embedding probe; refusing it by kind means no walk into it.
type embeddedCycle struct{ *embeddedCycle }

func TestGenericJSONRefusesValuesThatAreNotJSONShaped(t *testing.T) {
	str := "ok"
	at := []any{1, "x", map[string]any{"b": nil}}
	for name, tc := range map[string]struct {
		value any
		typ   string
	}{
		"time.Time":           {time.Unix(0, 0).UTC(), "time.Time"},
		"struct":              {struct{ Name string }{"ok"}, "struct { Name string }"},
		"pointer to string":   {&str, "*string"},
		"named string":        {label("ok"), "message_test.label"},
		"named number":        {celsius(21.5), "message_test.celsius"},
		"named map":           {fields{"k": "v"}, "message_test.fields"},
		"typed map":           {map[string]string{"k": "v"}, "map[string]string"},
		"typed map of any":    {map[any]any{"k": "v"}, "map[interface {}]interface {}"},
		"typed slice":         {[]string{"ok"}, "[]string"},
		"bytes":               {[]byte("ok"), "[]uint8"},
		"array":               {[1]any{"ok"}, "[1]interface {}"},
		"raw JSON":            {json.RawMessage(`"ok"`), "json.RawMessage"},
		"pointer to map":      {&map[string]any{}, "*map[string]interface {}"},
		"complex":             {complex(1, 2), "complex128"},
		"func":                {func() {}, "func()"},
		"channel":             {make(chan int), "chan int"},
		"error":               {errs.ErrInvalidData, "*errors.errorString"},
		"F15 JSON-method key": {map[jsonKey]int{jsonKey("\xff"): 1}, "map[message_test.jsonKey]int"},
		"F15 text-method key": {map[textKey]int{textKey("\xff"): 1}, "map[message_test.textKey]int"},
		"F15 byte text":       {[]byteText{1}, "[]message_test.byteText"},
		"F16 shadowed field":  {selected{inner: inner{"\xff"}, Name: "valid"}, "message_test.selected"},
		"F16 ambiguous":       {ambiguous{left: left{"\xff"}, right: right{"valid"}}, "message_test.ambiguous"},
	} {
		t.Run(name, func(t *testing.T) {
			at[2].(map[string]any)["b"] = tc.value
			requireRefused(t, map[string]any{"a": at}, "data.a[2].b", tc.typ)
		})
	}

	t.Run("F15 changing text method is never called", func(t *testing.T) {
		v := &changingText{}
		requireRefused(t, map[string]any{"value": v}, "data.value", "*message_test.changingText")
		if v.calls != 0 {
			t.Errorf("MarshalText called %d times; a refused value runs no method", v.calls)
		}
	})
}

// A cycle through map[string]any or []any is refused with an invalid-data error. The cases run in
// a child process under a stack cap and a deadline, so a walk that recurses without end fails this
// test instead of killing the test binary; the deadline is ten seconds for a check that takes
// microseconds, so only a hang reaches it.
func TestGenericJSONRefusesCycles(t *testing.T) {
	const child = "SEMENGINE_GENERIC_JSON_CYCLE_CHILD"
	if os.Getenv(child) == "1" {
		debug.SetMaxStack(1 << 20)
		self := map[string]any{"ok": 1}
		self["self"] = self
		list := []any{"ok", nil}
		list[1] = list
		through := map[string]any{}
		through["l"] = []any{map[string]any{"back": through}}
		embedded := &embeddedCycle{}
		embedded.embeddedCycle = embedded
		for name, data := range map[string]map[string]any{
			"map refers to itself":         self,
			"list holds itself":            {"l": list},
			"map through a list":           through,
			"F16 self-embedding (by kind)": {"e": embedded},
		} {
			t.Run(name, func(t *testing.T) {
				_, err := message.NewGenericJSON(data).MarshalJSON()
				if err == nil || !errs.IsInvalid(err) {
					t.Fatalf("MarshalJSON: %v, want an invalid-data error", err)
				}
			})
		}
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGenericJSONRefusesCycles$", "-test.v")
	cmd.Env = append(os.Environ(), child+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 30 {
			lines = lines[:30]
		}
		t.Fatalf("child: %v (deadline: %v)\n%s", err, ctx.Err(), strings.Join(lines, "\n"))
	}
	if !strings.Contains(string(out), "--- PASS: TestGenericJSONRefusesCycles/map_refers_to_itself") {
		t.Fatalf("child ran no cycle case:\n%s", out)
	}
}

// Every ruled kind encodes, alone and in its envelope, to the standard library's encoding of the
// same value, and decodes through NewDecoder.
func TestGenericJSONAcceptsJSONShapedValues(t *testing.T) {
	data := map[string]any{
		"map":      map[string]any{"k": "v", "�": "温度 �"},
		"list":     []any{"a", 1.5, nil, true, map[string]any{}},
		"string":   "s",
		"int":      int(-1),
		"int8":     int8(-8),
		"int16":    int16(-16),
		"int32":    int32(-32),
		"int64":    int64(-1 << 62),
		"uint":     uint(1),
		"uint8":    uint8(8),
		"uint16":   uint16(16),
		"uint32":   uint32(32),
		"uint64":   uint64(1<<64 - 1),
		"uintptr":  uintptr(7),
		"float32":  float32(0.1),
		"float64":  0.1,
		"number":   json.Number("12345678901234567890"),
		"true":     true,
		"false":    false,
		"nil":      nil,
		"nil map":  map[string]any(nil),
		"nil list": []any(nil),
		"empty":    []any{},
		"shared":   []any{map[string]any{"x": 1}, map[string]any{"x": 1}},
	}
	want, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		t.Fatal(err)
	}
	payload := message.NewGenericJSON(data)
	got, err := payload.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("MarshalJSON = %s, want %s", got, want)
	}
	encoded, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "gw"))
	if err != nil {
		t.Fatalf("envelope marshal: %v", err)
	}
	decoded, err := message.NewDecoder(payloadfixture.NewWithSubset(t, message.RegisterPayloads)).Decode(encoded)
	if err != nil {
		t.Fatalf("Decode(%s): %v", encoded, err)
	}
	var oracle map[string]any
	if err := unmarshalNumbers(got, &oracle); err != nil {
		t.Fatal(err)
	}
	if gotData := decoded.Payload().(*message.GenericJSONPayload).Data; !reflect.DeepEqual(gotData, oracle["data"]) {
		t.Fatalf("decoded %#v, want %#v", gotData, oracle["data"])
	}
}

// A string or key that is not valid UTF-8 is refused at any depth, with its path.
func TestGenericJSONRefusesInvalidUTF8AtDepth(t *testing.T) {
	for name, tc := range map[string]struct {
		data map[string]any
		want []string
	}{
		"top-level value": {map[string]any{"k": "\xff"}, []string{"data.k", "not valid UTF-8"}},
		"top-level key":   {map[string]any{"k\xfe": 1}, []string{"data", `key "k\xfe"`, "not valid UTF-8"}},
		"value at depth":  {map[string]any{"a": []any{map[string]any{"b": []any{nil, "x\xe2\x82"}}}}, []string{"data.a[0].b[1]", "not valid UTF-8"}},
		"key at depth":    {map[string]any{"a": []any{map[string]any{"\xc0\xaf": true}}}, []string{"data.a[0]", `key "\xc0\xaf"`}},
		"json.Number":     {map[string]any{"n": json.Number("1\xff")}, []string{"data.n", "not valid UTF-8"}},
		"quoted path key": {map[string]any{"a b": map[string]any{"c": "\xff"}}, []string{`data["a b"].c`}},
	} {
		t.Run(name, func(t *testing.T) { requireRefused(t, tc.data, tc.want...) })
	}
}

// UnmarshalJSON refuses malformed input and a well-formed value of the wrong shape with the same
// invalid-data class (early check E7), and leaves no partial data behind for malformed input.
func TestGenericJSONUnmarshalRefusalsAreInvalid(t *testing.T) {
	for name, in := range map[string]string{
		"malformed":        `{"data":{"k":1}`,
		"trailing data":    `{"data":{}} 1`,
		"data is a list":   `{"data":[1,2]}`,
		"data is text":     `{"data":"text"}`,
		"top is a list":    `[]`,
		"data is a number": `{"data":3}`,
	} {
		t.Run(name, func(t *testing.T) {
			var p message.GenericJSONPayload
			if err := p.UnmarshalJSON([]byte(in)); !errs.IsInvalid(err) {
				t.Fatalf("UnmarshalJSON(%s) = %v, want an invalid-data error", in, err)
			}
		})
	}
}
