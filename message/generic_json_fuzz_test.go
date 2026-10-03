package message_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// shapeGen builds a value for GenericJSONPayload.Data from fuzz bytes. Each node is chosen by one
// byte: every ruled kind (#9 comment 5972117486) is a choice, nested map[string]any and []any to a
// depth of four, strings and keys of raw bytes (so invalid UTF-8 arises), floats from raw bits (so
// NaN and the infinities arise), and three kinds the ruling refuses. The generator records what
// it built, so the oracle reads the facts from construction, never from the message package.
type shapeGen struct {
	in      []byte
	badUTF8 bool // a string or key it built is not valid UTF-8
	foreign bool // it built a value of a kind the ruling refuses
	nonJSON bool // it built a NaN or an infinity, which JSON cannot write
}

func (g *shapeGen) next() byte {
	if len(g.in) == 0 {
		return 0
	}
	b := g.in[0]
	g.in = g.in[1:]
	return b
}

func (g *shapeGen) bits() uint64 {
	var buf [8]byte
	for i := range buf {
		buf[i] = g.next()
	}
	return binary.LittleEndian.Uint64(buf[:])
}

func (g *shapeGen) text() string {
	n := int(g.next() % 6)
	b := make([]byte, n)
	for i := range b {
		b[i] = g.next()
	}
	if !utf8.Valid(b) {
		g.badUTF8 = true
	}
	return string(b)
}

const shapeKinds = 23

func (g *shapeGen) value(depth int) any {
	kind := int(g.next()) % shapeKinds
	if depth >= 4 && kind < 2 {
		kind = 2
	}
	switch kind {
	case 0:
		m := map[string]any{}
		for n := int(g.next() % 4); n > 0; n-- {
			k := g.text()
			if _, dup := m[k]; dup {
				continue // a second value would replace the first, whose facts are recorded
			}
			m[k] = g.value(depth + 1)
		}
		return m
	case 1:
		l := []any{}
		for n := int(g.next() % 4); n > 0; n-- {
			l = append(l, g.value(depth+1))
		}
		return l
	case 2:
		return g.text()
	case 3:
		return int(g.bits())
	case 4:
		return int8(g.bits())
	case 5:
		return int16(g.bits())
	case 6:
		return int32(g.bits())
	case 7:
		return int64(g.bits())
	case 8:
		return uint(g.bits())
	case 9:
		return uint8(g.bits())
	case 10:
		return uint16(g.bits())
	case 11:
		return uint32(g.bits())
	case 12:
		return g.bits()
	case 13:
		return uintptr(g.bits())
	case 14:
		f := math.Float32frombits(uint32(g.bits()))
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			g.nonJSON = true
		}
		return f
	case 15:
		f := math.Float64frombits(g.bits())
		if math.IsNaN(f) || math.IsInf(f, 0) {
			g.nonJSON = true
		}
		return f
	case 16:
		return json.Number(strconv.FormatInt(int64(g.bits()), 10))
	case 17:
		return g.next()%2 == 0
	case 18:
		return nil
	case 19:
		g.foreign = true
		return []string{g.text()}
	case 20:
		g.foreign = true
		return time.Unix(int64(g.bits()%1e10), 0)
	case 21:
		g.foreign = true
		return struct{ S string }{g.text()}
	default:
		g.foreign = true
		return map[string]string{g.text(): g.text()}
	}
}

// sameShape reports whether decoded (read with json.Decoder.UseNumber, so every number is its
// literal) is the generated value v. Integers compare as their exact decimal literal, a float64
// as the float64 its literal parses to, a float32 as the float32, and a json.Number as its text.
func sameShape(v, decoded any) bool {
	switch v := v.(type) {
	case nil:
		return decoded == nil
	case map[string]any:
		d, ok := decoded.(map[string]any)
		if !ok || len(d) != len(v) {
			return false
		}
		for k, e := range v {
			if de, ok := d[k]; !ok || !sameShape(e, de) {
				return false
			}
		}
		return true
	case []any:
		d, ok := decoded.([]any)
		if !ok || len(d) != len(v) {
			return false
		}
		for i := range v {
			if !sameShape(v[i], d[i]) {
				return false
			}
		}
		return true
	case string, bool:
		return decoded == v
	}
	n, ok := decoded.(json.Number)
	if !ok {
		return false
	}
	switch v := v.(type) {
	case int, int8, int16, int32, int64:
		return string(n) == strconv.FormatInt(reflect.ValueOf(v).Int(), 10)
	case uint, uint8, uint16, uint32, uint64, uintptr:
		return string(n) == strconv.FormatUint(reflect.ValueOf(v).Uint(), 10)
	case float32:
		f, err := strconv.ParseFloat(string(n), 32)
		return err == nil && float32(f) == v
	case float64:
		f, err := strconv.ParseFloat(string(n), 64)
		return err == nil && f == v
	case json.Number:
		return n == v
	}
	return false
}

// asFloats converts every json.Number in a UseNumber decode to the float64 its literal parses to,
// which is what json.Unmarshal gives a GenericJSONPayload.
func asFloats(t *testing.T, v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = asFloats(t, e)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = asFloats(t, e)
		}
		return out
	case json.Number:
		f, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			t.Fatalf("literal %q: %v", v, err)
		}
		return f
	}
	return v
}

// FuzzGenericJSONShapes: encoding a core.json.v1 payload whose Data is a generated tree of every
// ruled kind either refuses it or writes it so that it decodes to the same tree (#9 comments
// 5970334875, 5972117486 and 5972208367). The expected outcome comes from what the generator
// built: a refused kind or invalid UTF-8 anywhere gives an invalid-data error; otherwise a NaN or
// an infinity gives an error (JSON cannot write it); otherwise the payload, alone and in its
// envelope, encodes, and its bytes decode back to the generated tree. Numbers are compared at
// the literal: a json.Decoder with UseNumber reads each number's text, which must be the exact
// decimal of a generated integer, parse to the same float64 or float32, or equal a generated
// json.Number. The decode through NewDecoder must then equal that tree with every number as the
// float64 its literal parses to, as json.Unmarshal stores it.
//
// Seeds reach each kind (one seed per kind byte), invalid UTF-8 in a key and in a value at
// depth, each refused kind at depth, a NaN, and an empty map and list.
func FuzzGenericJSONShapes(f *testing.F) {
	for k := byte(0); k < shapeKinds; k++ {
		f.Add([]byte{0, 1, 1, 'k', k, 3, 'v', 0xe2, 0x82, 1, 2, 3, 4, 5, 6, 7, 8})
	}
	for _, seed := range [][]byte{
		{0, 1, 2, 'k', 0xff, 2, 1, 'v'},                     // invalid UTF-8 key
		{0, 1, 1, 'a', 1, 1, 0, 1, 1, 'b', 2, 2, 'x', 0xff}, // invalid value at depth
		{0, 1, 1, 'a', 1, 1, 19, 1, 'x'},                    // []string in a list
		{0, 1, 1, 'a', 0, 1, 1, 'b', 20, 1, 2, 3, 4},        // time.Time in a nested map
		{0, 1, 1, 'n', 15, 0, 0, 0, 0, 0, 0, 0xf8, 0x7f},    // NaN
		{0, 2, 1, 'm', 0, 0, 1, 'l', 1, 0},                  // empty map and list
		{0, 1, 3, 0xef, 0xbf, 0xbd, 2, 3, 0xef, 0xbf, 0xbd}, // U+FFFD, valid
	} {
		f.Add(seed)
	}
	dec := message.NewDecoder(fuzzRegistry(f))
	f.Fuzz(func(t *testing.T, in []byte) {
		g := &shapeGen{in: in}
		top := map[string]any{"v": g.value(0)}
		payload := message.NewGenericJSON(top)
		raw, err := payload.MarshalJSON()
		envelope, envErr := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "gw"))
		switch {
		case g.foreign || g.badUTF8:
			if !errs.IsInvalid(err) || !errs.IsInvalid(envErr) {
				t.Fatalf("Data %#v: MarshalJSON %v, envelope %v; want invalid-data errors", top, err, envErr)
			}
			return
		case g.nonJSON:
			if err == nil || envErr == nil {
				t.Fatalf("Data %#v holds a NaN or an infinity: MarshalJSON %s, envelope %s; want errors", top, raw, envelope)
			}
			return
		case err != nil || envErr != nil:
			t.Fatalf("Data %#v: MarshalJSON %v, envelope %v; want accepted", top, err, envErr)
		}

		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var exact struct {
			Data any `json:"data"`
		}
		if err := d.Decode(&exact); err != nil {
			t.Fatalf("standard library decode of %s: %v", raw, err)
		}
		if !sameShape(top, exact.Data) {
			t.Fatalf("Data %#v encoded as %s, which decodes to %#v", top, raw, exact.Data)
		}
		got, err := dec.Decode(envelope)
		if err != nil {
			t.Fatalf("Decode(%s): %v", envelope, err)
		}
		if want := asFloats(t, exact.Data); !reflect.DeepEqual(got.Payload().(*message.GenericJSONPayload).Data, want) {
			t.Fatalf("decoded %#v, want %#v", got.Payload().(*message.GenericJSONPayload).Data, want)
		}
	})
}
