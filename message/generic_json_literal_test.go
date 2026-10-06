package message_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// Each number in a decoded core.json.v1 payload is a json.Number holding the literal as written,
// including literals float64 cannot hold (Codex F41). The expected values are written here by
// hand, not computed by any decoder, so this table is the independent check the fuzz targets'
// differential oracle is not.
func TestGenericJSONKeepsNumberLiteral(t *testing.T) {
	reg := payloadfixture.NewWithSubset(t, message.RegisterPayloads)
	for _, literal := range []string{
		"1e400",
		"-1e400",
		"1e-400",
		"-0",
		"123456789012345678901234567890",
		"1E+999999",
	} {
		t.Run(literal, func(t *testing.T) {
			want := json.Number(literal)
			body := `{"data":{"n":` + literal + `}}`

			var p message.GenericJSONPayload
			if err := p.UnmarshalJSON([]byte(body)); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", body, err)
			}
			if got := p.Data["n"]; got != want {
				t.Fatalf("UnmarshalJSON(%s): n = %#v (%T), want %#v", body, got, got, want)
			}

			envelope := `{"id":"m","type":{"domain":"core","category":"json","version":"v1"},"payload":` + body + `}`
			msg, err := message.NewDecoder(reg).Decode([]byte(envelope))
			if err != nil {
				t.Fatalf("Decode(%s): %v", envelope, err)
			}
			if got := msg.Payload().(*message.GenericJSONPayload).Data["n"]; got != want {
				t.Fatalf("Decode(%s): n = %#v (%T), want %#v", envelope, got, got, want)
			}
		})
	}
}

// What GenericJSONPayload.UnmarshalJSON accepts and refuses, stated input by input as the
// message-codec spec states it: a body that is an object or null is accepted, and data that is
// null or absent leaves Data nil; malformed JSON, non-whitespace bytes after the value, a body
// that is neither an object nor null, and a data that is neither an object nor null are refused
// with an invalid-data error.
func TestGenericJSONUnmarshalAcceptsAndRefuses(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want map[string]any // when accepted
	}{
		{in: `{"data":null}`},
		{in: `{}`},
		{in: `null`},
		{in: `{"other":1}`},
		{in: "{\"data\":{}} \n\t\r", want: map[string]any{}},
		{in: `{"data":{"k":"v"}}`, want: map[string]any{"k": "v"}},
	} {
		var p message.GenericJSONPayload
		if err := p.UnmarshalJSON([]byte(tc.in)); err != nil {
			t.Fatalf("UnmarshalJSON(%q) = %v, want accepted", tc.in, err)
		}
		if !reflect.DeepEqual(p.Data, tc.want) {
			t.Fatalf("UnmarshalJSON(%q): Data %#v, want %#v", tc.in, p.Data, tc.want)
		}
	}
	for _, in := range []string{
		`{"data":{"k":1}`, // malformed
		`{"data":{}}x`,    // non-whitespace after the value
		`{"data":{}} {}`,
		`[]`, // a body that is neither an object nor null
		`1`,
		`"data"`,
		`{"data":[1]}`, // a data that is neither an object nor null
		`{"data":1}`,
		`{"data":"x"}`,
		`{"data":true}`,
	} {
		var p message.GenericJSONPayload
		if err := p.UnmarshalJSON([]byte(in)); !errs.IsInvalid(err) {
			t.Fatalf("UnmarshalJSON(%q) = %v, want an invalid-data error", in, err)
		}
	}
}
