package message_test

import (
	"encoding/json"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
)

// countPayload is a consumer-style payload with a typed field, so the fuzz registry holds a type
// whose payload can fail to fit (a string where an integer belongs), which core.json.v1 cannot.
type countPayload struct {
	Count int64 `json:"count"`
}

var countType = message.Type{Domain: "test", Category: "count", Version: "v1"}

func (p *countPayload) Schema() message.Type { return countType }

func (p *countPayload) Validate() error { return nil }

func (p *countPayload) MarshalJSON() ([]byte, error) {
	type alias countPayload
	return json.Marshal((*alias)(p))
}

func (p *countPayload) UnmarshalJSON(data []byte) error {
	type alias countPayload
	return json.Unmarshal(data, (*alias)(p))
}

// oracleEnvelope is the test's own copy of the documented wire envelope. The oracle decodes
// input with the standard library into it; nothing here calls the message package's decoding.
// Meta values stay raw so the oracle reads each timestamp's literal itself.
type oracleEnvelope struct {
	ID   string `json:"id"`
	Type struct {
		Domain   string `json:"domain"`
		Category string `json:"category"`
		Version  string `json:"version"`
	} `json:"type"`
	Payload json.RawMessage            `json:"payload"`
	Meta    map[string]json.RawMessage `json:"meta"`
}

type oracleGenericJSON struct {
	Data map[string]any `json:"data"`
}

type oracleCount struct {
	Count int64 `json:"count"`
}

// fuzzRegistry holds the three kinds of registration the decoder can meet: the built-in
// core.json.v1, a typed consumer payload, and a schema-less stub that is not a message.Payload.
func fuzzRegistry(tb testing.TB) *payloadregistry.Registry {
	tb.Helper()
	reg := payloadfixture.NewWithSubset(tb, message.RegisterPayloads)
	if err := reg.Register(&payloadregistry.Registration{
		Domain: countType.Domain, Category: countType.Category, Version: countType.Version,
		Description: "fuzz payload with a typed field",
		Factory:     func() any { return &countPayload{} },
	}); err != nil {
		tb.Fatalf("register test.count.v1: %v", err)
	}
	payloadfixture.RegisterTestType(tb, reg, message.Type{Domain: "test", Category: "stub", Version: "v1"})
	return reg
}

// FuzzDecoderDecode: Decoder.Decode decodes outside bytes (task 3.6b, Codex F8). The rules come
// from the decoder's documented contract and owner ruling 7 (#9 comment 5969522395), and are
// checked against the standard library's decode of the same bytes into oracleEnvelope (design D7,
// "Generated checks for message"):
//
//   - it never panics;
//   - it accepts exactly when the envelope decodes, its type is core.json.v1 or test.count.v1
//     (test.stub.v1 is registered but is not a Payload; every other type is unregistered), the
//     payload decodes into that type's fields, and each of meta.created_at and meta.received_at
//     is absent, null, or an integer literal in int64's range. Both directions are asserted, so an
//     always-refusing decoder fails on the accepted seeds and an over-accepting one on the refused;
//   - an accepted message keeps the ID, the type, the payload's fields, meta.source (empty unless
//     a string) and each timestamp as exactly that many milliseconds (0, absent or null: the zero
//     time);
//   - an accepted message that validates marshals, and decoding that gives the same message in
//     full: ID, type, payload, source and both timestamps.
func FuzzDecoderDecode(f *testing.F) {
	for _, seed := range []string{
		// accepted
		`{"id":"m-1","type":{"domain":"core","category":"json","version":"v1"},` +
			`"payload":{"data":{"sensor_id":"temp-001","temperature":23.5,"tags":["a",null,true]}},` +
			`"meta":{"created_at":1727900000000,"received_at":1727900000123,"source":"sensor-gw"}}`,
		`{"id":"m-2","type":{"domain":"test","category":"count","version":"v1"},"payload":{"count":42},` +
			`"meta":{"created_at":915148800123,"received_at":-315619199544,"source":"counter"}}`,
		`{"id":"m-2a","type":{"domain":"test","category":"count","version":"v1"},"payload":{"count":-1},` +
			`"meta":{"created_at":0,"received_at":999999999999,"source":""}}`,
		`{"id":"m-2b","type":{"domain":"test","category":"count","version":"v1"},"payload":{"count":1},` +
			`"meta":{"created_at":-9223372036854775808,"received_at":9223372036854775807}}`,
		`{"type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{}}}`,
		`{"id":"m-3","type":{"domain":"core","category":"json","version":"v1"},"payload":null,"meta":null}`,
		`{"id":"m-4","type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{"k":1}},` +
			`"meta":{"source":7,"created_at":null,"extra":[1],"Created_At":"x"}}`,
		`{"ID":"m-5","Type":{"Domain":"test","Category":"count","Version":"v1"},"Payload":{"Count":-3},"unknown":1}`,
		// refused
		`{"id":"m-6","type":{"domain":"nope","category":"json","version":"v1"},"payload":{"data":{}}}`,
		`{"id":"m-7","type":{"domain":"test","category":"stub","version":"v1"},"payload":{}}`,
		`{"id":"m-8","type":{"domain":"test","category":"count","version":"v1"},"payload":{"count":"many"}}`,
		`{"id":"m-9","type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":[1,2]}}`,
		`{"id":"m-10","type":{"domain":"test","category":"count","version":"v1"}}`,
		`{"id":11,"type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{}}}`,
		`{"id":"m-12","type":"core.json.v1","payload":{"data":{}}}`,
		`{"id":"m-13","type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{}},"meta":[]}`,
		`{"id":"m-14","type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{}}`,
		`{"id":"m-15","type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{}},` +
			`"meta":{"created_at":"2024-10-02T00:00:00Z"}}`,
		`{"id":"m-16","type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{}},` +
			`"meta":{"received_at":1727900000000.5}}`,
		`{"id":"m-17","type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{}},` +
			`"meta":{"created_at":1.7279e12}}`,
		`{"id":"m-18","type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{}},` +
			`"meta":{"created_at":9223372036854775808}}`,
		`{"id":"m-19","type":{"domain":"core","category":"json","version":"v1"},"payload":{"data":{}},` +
			`"meta":{"received_at":"1727900000000"}}`,
		`null`,
		`[]`,
		``,
	} {
		f.Add([]byte(seed))
	}
	dec := message.NewDecoder(fuzzRegistry(f))
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := dec.Decode(data)

		want, accept := oracleDecode(data)
		if accept != (err == nil) {
			t.Fatalf("Decode(%q): err = %v, oracle accepts = %t", data, err, accept)
		}
		if err != nil {
			if got != nil {
				t.Fatalf("Decode(%q) refused with %v but returned a message", data, err)
			}
			return
		}
		checkPreserved(t, data, got, want)

		if got.Validate() != nil {
			return
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("marshal of the message decoded from %q: %v", data, err)
		}
		again, err := dec.Decode(encoded)
		if err != nil {
			t.Fatalf("second Decode of %s (from %q): %v", encoded, data, err)
		}
		requireSameMessage(t, encoded, got, again)
	})
}

// FuzzDecoderRoundTrip: decode(marshal(m)) == m in full for a message built through the package's
// own constructors, with every int64 timestamp reachable by construction (owner ruling 7: the wire
// is milliseconds both ways); marshal refuses a source that is not valid UTF-8 (owner ruling 2,
// #9 comment 5969776736). Seeds sit on each boundary the old heuristic and the representation
// have: 0 (the zero time), ±1, 10^12 - 1 and 10^12 (the old seconds/milliseconds switch),
// pre-1970, and both ends of int64; of the last two sources, U+FFFD itself is valid UTF-8 and
// \xff\xfe is not.
func FuzzDecoderRoundTrip(f *testing.F) {
	for _, seed := range []struct {
		source            string
		created, received int64
		count             int64
	}{
		{"sensor-gw", 1727900000000, 1727900000250, 42},
		{"", 0, 0, 0},
		{"a", 1, -1, -1},
		{"b", 999_999_999_999, 1_000_000_000_000, 7},
		{"c", 915_148_800_123, -315_619_199_544, 1 << 53},
		{"d", math.MinInt64, math.MaxInt64, math.MinInt64},
		{"e", -62_135_596_800_000, 253_402_300_799_999, math.MaxInt64},
		{"\uFFFD 温度", 4, 5, 6},
		{"\xff\xfe source", 1, 2, 3},
	} {
		f.Add(seed.source, seed.created, seed.received, seed.count)
	}
	dec := message.NewDecoder(fuzzRegistry(f))
	f.Fuzz(func(t *testing.T, source string, created, received, count int64) {
		built := message.NewBaseMessage(countType, &countPayload{Count: count}, source,
			message.WithMeta(message.NewDefaultMetaWithReceivedAt(
				time.UnixMilli(created), time.UnixMilli(received), source)))
		// The instants the message was built with, at millisecond precision; 0 ms is the zero time.
		for _, ts := range []struct {
			name string
			got  time.Time
			ms   int64
		}{
			{"created_at", built.Meta().CreatedAt(), created},
			{"received_at", built.Meta().ReceivedAt(), received},
		} {
			if !ts.got.Equal(millisInstant(ts.ms)) {
				t.Fatalf("built %s %v, want %d ms", ts.name, ts.got, ts.ms)
			}
		}
		encoded, err := json.Marshal(built)
		// JSON text is UTF-8, so a source that is not is refused, never written altered (owner
		// ruling 2, #9 comment 5969776736). The oracle is the standard library's utf8 package.
		if !utf8.ValidString(source) {
			if err == nil {
				t.Fatalf("marshal of source %q succeeded: %s", source, encoded)
			}
			if !errs.IsInvalid(err) {
				t.Fatalf("marshal of source %q: %v, want an invalid-data error", source, err)
			}
			return
		}
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		got, err := dec.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(%s): %v", encoded, err)
		}
		requireSameMessage(t, encoded, built, got)
	})
}

// anyTypePayload is a payload whose type is chosen per input, so FuzzDecoderStrings can register
// and decode a message of any type that Type.Validate accepts.
type anyTypePayload struct {
	typ   message.Type
	Count int64 `json:"count"`
}

func (p *anyTypePayload) Schema() message.Type { return p.typ }

func (p *anyTypePayload) Validate() error { return nil }

func (p *anyTypePayload) MarshalJSON() ([]byte, error) {
	type alias anyTypePayload
	return json.Marshal((*alias)(p))
}

func (p *anyTypePayload) UnmarshalJSON(data []byte) error {
	type alias anyTypePayload
	return json.Unmarshal(data, (*alias)(p))
}

// FuzzDecoderStrings: every string message encodes either reaches the wire unchanged or is
// refused, never rewritten (owner ruling, PR #48 comment 5970334875, extending ruling 2 of #9
// comment 5969776736). The expected outcomes come from the standard library's utf8 and strings
// packages and the documented grammar, never from the message package:
//
//   - Type.Validate accepts a type exactly when each component is non-empty, holds no "." and is
//     valid UTF-8;
//   - marshalling a message of that type is refused, with an invalid-data error, exactly when a
//     component is empty or not valid UTF-8 (BaseMessage.Validate does not check the "."). An
//     encoded type that Validate accepts is registered and decoded, giving the same message in
//     full; one it refuses for its "." is read back with the standard library, unchanged;
//   - a core.json.v1 payload holding the generated key and value at the top level, in a nested
//     map and in a list is refused, alone and in its envelope, exactly when the key or the value
//     is not valid UTF-8; otherwise decoding the envelope gives the same message in full.
//
// Seeds hold an invalid byte in each of the five positions, U+FFFD itself (valid), a "." and an
// empty component.
func FuzzDecoderStrings(f *testing.F) {
	for _, seed := range [][5]string{
		{"core", "json", "v1", "k", "v"},
		{"\uFFFD", "温度", "v1", "\uFFFD", "温度 \uFFFD"},
		{"\xff", "json", "v1", "k", "v"},
		{"core", "js\xfeon", "v1", "k", "v"},
		{"core", "json", "v\xe2\x82", "k", "v"},
		{"core", "json", "v1", "k\xc0\xaf", "v"},
		{"core", "json", "v1", "k", "\xff\xfe"},
		{"a.b", "json", "v1", "list", "nested"},
		{"", "json", "v1", "", ""},
		{"d", "c", "", "nested", "\x00"},
	} {
		f.Add(seed[0], seed[1], seed[2], seed[3], seed[4])
	}
	genericDecoder := message.NewDecoder(fuzzRegistry(f))
	f.Fuzz(func(t *testing.T, domain, category, version, key, value string) {
		mt := message.Type{Domain: domain, Category: category, Version: version}
		components := []string{domain, category, version}
		wantValid, wantMarshal := true, true
		for _, c := range components {
			if c == "" || !utf8.ValidString(c) {
				wantValid, wantMarshal = false, false
			}
			if strings.Contains(c, ".") {
				wantValid = false
			}
		}
		if err := mt.Validate(); (err == nil) != wantValid {
			t.Fatalf("Type%+q.Validate() = %v, want accepted = %t", components, err, wantValid)
		}

		built := message.NewBaseMessage(mt, &anyTypePayload{typ: mt, Count: 7}, "gw")
		encoded, err := json.Marshal(built)
		if (err == nil) != wantMarshal {
			t.Fatalf("marshal of type %+q: %s, %v; want accepted = %t", components, encoded, err, wantMarshal)
		}
		switch {
		case err != nil:
			if !errs.IsInvalid(err) {
				t.Fatalf("marshal of type %+q: %v, want an invalid-data error", components, err)
			}
		case wantValid:
			reg := payloadregistry.New()
			if err := reg.Register(&payloadregistry.Registration{
				Domain: domain, Category: category, Version: version, Description: "fuzz type",
				Factory: func() any { return &anyTypePayload{typ: mt} },
			}); err != nil {
				t.Fatalf("register %+q: %v", components, err)
			}
			got, err := message.NewDecoder(reg).Decode(encoded)
			if err != nil {
				t.Fatalf("Decode(%s): %v", encoded, err)
			}
			requireSameMessage(t, encoded, built, got)
		default:
			var env oracleEnvelope
			if err := json.Unmarshal(encoded, &env); err != nil {
				t.Fatalf("standard library decode of %s: %v", encoded, err)
			}
			if got := (message.Type{Domain: env.Type.Domain, Category: env.Type.Category, Version: env.Type.Version}); got != mt {
				t.Fatalf("type on the wire %+v, want %+v", got, mt)
			}
		}

		data := map[string]any{
			"list":   []any{value, nil, true},
			"nested": map[string]any{key: value},
		}
		data[key] = value
		wantRefused := !utf8.ValidString(key) || !utf8.ValidString(value)
		payload := message.NewGenericJSON(data)
		if raw, err := payload.MarshalJSON(); (err != nil) != wantRefused || (err != nil && !errs.IsInvalid(err)) {
			t.Fatalf("payload MarshalJSON with key %q value %q: %s, %v; want refused = %t (invalid data)",
				key, value, raw, err, wantRefused)
		}
		envelope := message.NewBaseMessage(payload.Schema(), payload, "gw")
		encoded, err = json.Marshal(envelope)
		if (err != nil) != wantRefused || (err != nil && !errs.IsInvalid(err)) {
			t.Fatalf("envelope marshal with key %q value %q: %s, %v; want refused = %t (invalid data)",
				key, value, encoded, err, wantRefused)
		}
		if err != nil {
			return
		}
		got, err := genericDecoder.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(%s): %v", encoded, err)
		}
		requireSameMessage(t, encoded, envelope, got)
	})
}

func millisInstant(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// requireSameMessage asserts decode(marshal(m)) == m in full.
func requireSameMessage(t *testing.T, encoded []byte, first, then *message.BaseMessage) {
	t.Helper()
	if then.ID() != first.ID() || then.Type() != first.Type() ||
		then.Meta().Source() != first.Meta().Source() ||
		!then.Meta().CreatedAt().Equal(first.Meta().CreatedAt()) ||
		!then.Meta().ReceivedAt().Equal(first.Meta().ReceivedAt()) ||
		!reflect.DeepEqual(then.Payload(), first.Payload()) {
		t.Fatalf("round trip through %s:\n  id %q type %v source %q created %v received %v payload %#v\n"+
			"then\n  id %q type %v source %q created %v received %v payload %#v", encoded,
			first.ID(), first.Type(), first.Meta().Source(), first.Meta().CreatedAt(), first.Meta().ReceivedAt(), first.Payload(),
			then.ID(), then.Type(), then.Meta().Source(), then.Meta().CreatedAt(), then.Meta().ReceivedAt(), then.Payload())
	}
}

// oracleExpect is what the standard library says an accepted input holds.
type oracleExpect struct {
	env                 oracleEnvelope
	generic             *oracleGenericJSON
	count               *oracleCount
	source              string
	createdMs, received int64
}

// integerLiteral is RFC 8259's number grammar with no fraction and no exponent.
var integerLiteral = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// oracleMillis reads a raw meta value as the wire's integer milliseconds: absent or null is 0.
func oracleMillis(raw json.RawMessage, present bool) (int64, bool) {
	if !present || len(raw) == 0 || string(raw) == "null" {
		return 0, true
	}
	if !integerLiteral.Match(raw) {
		return 0, false
	}
	n, ok := new(big.Int).SetString(string(raw), 10)
	if !ok || !n.IsInt64() {
		return 0, false
	}
	return n.Int64(), true
}

func oracleDecode(data []byte) (oracleExpect, bool) {
	var want oracleExpect
	if json.Unmarshal(data, &want.env) != nil {
		return want, false
	}
	var ok bool
	raw, present := want.env.Meta["created_at"]
	if want.createdMs, ok = oracleMillis(raw, present); !ok {
		return want, false
	}
	raw, present = want.env.Meta["received_at"]
	if want.received, ok = oracleMillis(raw, present); !ok {
		return want, false
	}
	if raw, present := want.env.Meta["source"]; present {
		var source any
		if json.Unmarshal(raw, &source) == nil {
			want.source, _ = source.(string)
		}
	}
	switch t := want.env.Type; {
	case t.Domain == "core" && t.Category == "json" && t.Version == "v1":
		want.generic = &oracleGenericJSON{}
		return want, json.Unmarshal(want.env.Payload, want.generic) == nil
	case t.Domain == "test" && t.Category == "count" && t.Version == "v1":
		want.count = &oracleCount{}
		return want, json.Unmarshal(want.env.Payload, want.count) == nil
	default:
		return want, false
	}
}

func checkPreserved(t *testing.T, data []byte, got *message.BaseMessage, want oracleExpect) {
	t.Helper()
	if got.ID() != want.env.ID {
		t.Fatalf("Decode(%q): ID %q, want %q", data, got.ID(), want.env.ID)
	}
	wantType := message.Type{Domain: want.env.Type.Domain, Category: want.env.Type.Category, Version: want.env.Type.Version}
	if got.Type() != wantType {
		t.Fatalf("Decode(%q): type %+v, want %+v", data, got.Type(), wantType)
	}
	switch p := got.Payload().(type) {
	case *message.GenericJSONPayload:
		if want.generic == nil || !reflect.DeepEqual(p.Data, want.generic.Data) {
			t.Fatalf("Decode(%q): core.json.v1 data %#v, want %#v", data, p.Data, want.generic)
		}
	case *countPayload:
		if want.count == nil || p.Count != want.count.Count {
			t.Fatalf("Decode(%q): count %d, want %+v", data, p.Count, want.count)
		}
	default:
		t.Fatalf("Decode(%q): payload %T for type %+v", data, got.Payload(), wantType)
	}
	if got.Meta().Source() != want.source {
		t.Fatalf("Decode(%q): source %q, want %q", data, got.Meta().Source(), want.source)
	}
	if c := got.Meta().CreatedAt(); !c.Equal(millisInstant(want.createdMs)) {
		t.Fatalf("Decode(%q): created_at %v, want %d ms", data, c, want.createdMs)
	}
	if r := got.Meta().ReceivedAt(); !r.Equal(millisInstant(want.received)) {
		t.Fatalf("Decode(%q): received_at %v, want %d ms", data, r, want.received)
	}
}

// FuzzGenericJSONPayloadUnmarshalJSON: GenericJSONPayload.UnmarshalJSON decodes outside bytes
// (task 3.6b, Codex F8). It never panics, it refuses exactly what the standard library refuses
// for {"data": object}, and it keeps exactly the data the standard library decodes.
func FuzzGenericJSONPayloadUnmarshalJSON(f *testing.F) {
	for _, seed := range []string{
		// accepted
		`{"data":{"sensor_id":"temp-001","temperature":23.5,"nested":{"a":[1,"b",null]}}}`,
		`{"data":{}}`,
		`{"Data":{"k":"v"}}`,
		`{"data":null}`,
		`{}`,
		`{"other":1}`,
		`null`,
		// refused
		`{"data":[1,2]}`,
		`{"data":"text"}`,
		`{"data":{"k":1}`,
		`[]`,
		`"data"`,
		``,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var got message.GenericJSONPayload
		err := got.UnmarshalJSON(data)

		var want oracleGenericJSON
		wantErr := json.Unmarshal(data, &want)
		if (err == nil) != (wantErr == nil) {
			t.Fatalf("UnmarshalJSON(%q): err = %v, standard library err = %v", data, err, wantErr)
		}
		if err == nil && !reflect.DeepEqual(got.Data, want.Data) {
			t.Fatalf("UnmarshalJSON(%q): data %#v, want %#v", data, got.Data, want.Data)
		}
	})
}
