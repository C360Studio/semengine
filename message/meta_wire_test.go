package message_test

import (
	"encoding/json"
	"go/types"
	"sort"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// TestMetaCarriesOnlyWhatTheWireCarries: BaseMessage.MarshalJSON writes created_at, received_at
// and source and nothing else, so a Meta that holds more loses it on the first Decode (PR #48,
// Codex review 5969256728: a federation UID present before encoding, absent after). Owner ruling
// #72 comment 5969293525 removes the FederationMeta family; this test keeps the package from
// offering such a Meta again: every exported type in message that implements Meta has exactly
// Meta's exported methods, and no exported interface extends Meta.
func TestMetaCarriesOnlyWhatTheWireCarries(t *testing.T) {
	loaded, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedTypes}, ".")
	if err != nil {
		t.Fatalf("load message: %v", err)
	}
	if len(loaded) != 1 || len(loaded[0].Errors) > 0 || loaded[0].Types == nil {
		t.Fatalf("load message: %d packages, errors %v", len(loaded), packageErrors(loaded))
	}
	scope := loaded[0].Types.Scope()
	metaObj := scope.Lookup("Meta")
	if metaObj == nil {
		t.Fatal("message.Meta not found")
	}
	meta, ok := metaObj.Type().Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("message.Meta is %T, not an interface", metaObj.Type().Underlying())
	}
	wire := exportedMethods(metaObj.Type())

	checked := 0
	for _, name := range scope.Names() {
		obj, isType := scope.Lookup(name).(*types.TypeName)
		if !isType || !obj.Exported() || obj == metaObj {
			continue
		}
		typ := obj.Type()
		if _, isIface := typ.Underlying().(*types.Interface); !isIface {
			typ = types.NewPointer(typ)
		}
		if !types.Implements(typ, meta) {
			continue
		}
		checked++
		if extra := difference(exportedMethods(typ), wire); len(extra) > 0 {
			t.Errorf("message.%s implements Meta and adds %v, which BaseMessage.MarshalJSON does not write", name, extra)
		}
	}
	if checked == 0 {
		t.Fatal("no exported type implements message.Meta; DefaultMeta should")
	}
}

// TestBaseMessageMetaSurvivesTheWire: a message built by NewBaseMessage with each option the
// package offers decodes with the Meta it was built with.
func TestBaseMessageMetaSurvivesTheWire(t *testing.T) {
	created := time.UnixMilli(1727900000000)
	received := time.UnixMilli(1727900000250)
	dec := message.NewDecoder(payloadfixture.NewWithSubset(t, message.RegisterPayloads))
	for name, opt := range map[string]message.Option{
		"none":     func(*message.BaseMessage) {},
		"WithTime": message.WithTime(created),
		"WithMeta": message.WithMeta(message.NewDefaultMetaWithReceivedAt(created, received, "sensor-gw")),
	} {
		t.Run(name, func(t *testing.T) {
			payload := message.NewGenericJSON(map[string]any{"k": "v"})
			built := message.NewBaseMessage(payload.Schema(), payload, "sensor-gw", opt)
			data, err := json.Marshal(built)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got, err := dec.Decode(data)
			if err != nil {
				t.Fatalf("decode %s: %v", data, err)
			}
			b, g := built.Meta(), got.Meta()
			if g.Source() != b.Source() || g.CreatedAt().UnixMilli() != b.CreatedAt().UnixMilli() ||
				g.ReceivedAt().UnixMilli() != b.ReceivedAt().UnixMilli() {
				t.Fatalf("meta after the wire: source %q created %d received %d; built %q %d %d",
					g.Source(), g.CreatedAt().UnixMilli(), g.ReceivedAt().UnixMilli(),
					b.Source(), b.CreatedAt().UnixMilli(), b.ReceivedAt().UnixMilli())
			}
		})
	}
}

// TestBaseMessageTimestampsAreMilliseconds: MarshalJSON writes created_at and received_at as
// integer Unix milliseconds, and Decode reads them back as exactly that (owner ruling 7, #9 comment
// 5969522395). At the pin decode went through timestamp.Parse, which reads a number up to 10^12 as
// seconds, so any instant before 2001-09-09 came back wrong (1999-01-01 as year 30969). The
// expected instants are written out with time.Date, and the wire is read with the standard
// library, so neither side of the check uses the package's conversion. The epoch is 0 ms, which
// DefaultMeta already holds as "no time" (the zero time.Time) before encoding; it must come back
// as the same.
func TestBaseMessageTimestampsAreMilliseconds(t *testing.T) {
	dec := message.NewDecoder(payloadfixture.NewWithSubset(t, message.RegisterPayloads))
	for name, at := range map[string]time.Time{
		"1999-01-01": time.Date(1999, 1, 1, 0, 0, 0, 123_000_000, time.UTC),
		"1960-01-01": time.Date(1960, 1, 1, 0, 0, 0, 456_000_000, time.UTC),
		"epoch":      time.Unix(0, 0),
		"2024":       time.Date(2024, 6, 1, 12, 34, 56, 789_000_000, time.UTC),
	} {
		t.Run(name, func(t *testing.T) {
			received := at.Add(250 * time.Millisecond)
			payload := message.NewGenericJSON(map[string]any{"k": "v"})
			built := message.NewBaseMessage(payload.Schema(), payload, "sensor-gw",
				message.WithMeta(message.NewDefaultMetaWithReceivedAt(at, received, "sensor-gw")))
			data, err := json.Marshal(built)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var wire struct {
				Meta struct {
					CreatedAt  int64 `json:"created_at"`
					ReceivedAt int64 `json:"received_at"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatalf("read the wire %s: %v", data, err)
			}
			if wire.Meta.CreatedAt != at.UnixMilli() || wire.Meta.ReceivedAt != received.UnixMilli() {
				t.Fatalf("wire %s: want created_at %d, received_at %d", data, at.UnixMilli(), received.UnixMilli())
			}
			got, err := dec.Decode(data)
			if err != nil {
				t.Fatalf("decode %s: %v", data, err)
			}
			wantCreated := at
			if at.UnixMilli() == 0 {
				wantCreated = time.Time{}
			}
			if c := got.Meta().CreatedAt(); !c.Equal(wantCreated) || !c.Equal(built.Meta().CreatedAt()) {
				t.Errorf("created_at after the wire: %v, want %v (built %v)", c, wantCreated, built.Meta().CreatedAt())
			}
			if r := got.Meta().ReceivedAt(); !r.Equal(received) || !r.Equal(built.Meta().ReceivedAt()) {
				t.Errorf("received_at after the wire: %v, want %v (built %v)", r, received, built.Meta().ReceivedAt())
			}
		})
	}
}

// TestBaseMessageRefusesTimestampsThatAreNotMilliseconds: a timestamp is an integer number of
// milliseconds, or absent or null for none. Anything else is refused rather than read as some
// other unit or dropped to the zero time (owner ruling 7, #9 comment 5969522395).
func TestBaseMessageRefusesTimestampsThatAreNotMilliseconds(t *testing.T) {
	dec := message.NewDecoder(payloadfixture.NewWithSubset(t, message.RegisterPayloads))
	envelope := func(meta string) []byte {
		return []byte(`{"id":"m","type":{"domain":"core","category":"json","version":"v1"},` +
			`"payload":{"data":{}},"meta":{` + meta + `}}`)
	}
	for _, meta := range []string{
		`"created_at":"2024-06-01T12:34:56Z"`,
		`"created_at":"1717245296789"`,
		`"created_at":1717245296789.5`,
		`"created_at":1.717245296789e12`,
		`"created_at":true`,
		`"created_at":{}`,
		`"received_at":"1717245296789"`,
		`"received_at":99999999999999999999`,
	} {
		if msg, err := dec.Decode(envelope(meta)); err == nil {
			t.Errorf("Decode accepted meta {%s}: created %v, received %v", meta,
				msg.Meta().CreatedAt(), msg.Meta().ReceivedAt())
		}
	}
	for _, meta := range []string{``, `"created_at":null,"received_at":null`, `"created_at":0`} {
		msg, err := dec.Decode(envelope(meta))
		if err != nil {
			t.Errorf("Decode refused meta {%s}: %v", meta, err)
			continue
		}
		if !msg.Meta().CreatedAt().IsZero() || !msg.Meta().ReceivedAt().IsZero() {
			t.Errorf("meta {%s}: created %v, received %v, want the zero time for both", meta,
				msg.Meta().CreatedAt(), msg.Meta().ReceivedAt())
		}
	}
}

// TestBaseMessageRefusesSourceThatIsNotUTF8: JSON text is UTF-8, and encoding/json documents that
// Marshal replaces each invalid byte of a string with U+FFFD, so a source that is not valid UTF-8
// would reach the wire as a different source with no error. Encoding refuses it instead (owner
// ruling 2, #9 comment 5969776736), whether the source came through NewBaseMessage or a Meta
// given with WithMeta; a valid source, U+FFFD itself included, encodes and decodes unchanged.
func TestBaseMessageRefusesSourceThatIsNotUTF8(t *testing.T) {
	coreJSON := message.Type{Domain: "core", Category: "json", Version: "v1"}
	payload := func() message.Payload { return message.NewGenericJSON(map[string]any{"k": "v"}) }
	for _, source := range []string{"\xff", "gw-\xfe", "\xe2\x82", "ok\xc0\xafok"} {
		for name, msg := range map[string]*message.BaseMessage{
			"NewBaseMessage": message.NewBaseMessage(coreJSON, payload(), source),
			"WithMeta": message.NewBaseMessage(coreJSON, payload(), "valid",
				message.WithMeta(message.NewDefaultMeta(time.UnixMilli(1), source))),
		} {
			encoded, err := json.Marshal(msg)
			if err == nil {
				t.Errorf("%s: marshal of source %q succeeded: %s", name, source, encoded)
				continue
			}
			if !errs.IsInvalid(err) {
				t.Errorf("%s: marshal of source %q: %v, want an invalid-data error", name, source, err)
			}
		}
	}

	dec := message.NewDecoder(payloadfixture.NewWithSubset(t, message.RegisterPayloads))
	for _, source := range []string{"", "sensor-gw", "温度-センサー", "�", "a b"} {
		encoded, err := json.Marshal(message.NewBaseMessage(coreJSON, payload(), source))
		if err != nil {
			t.Errorf("marshal of source %q: %v", source, err)
			continue
		}
		got, err := dec.Decode(encoded)
		if err != nil {
			t.Errorf("decode of %s: %v", encoded, err)
			continue
		}
		if got.Meta().Source() != source {
			t.Errorf("source after the wire %q, want %q", got.Meta().Source(), source)
		}
	}
}

func exportedMethods(typ types.Type) []string {
	set := types.NewMethodSet(typ)
	var names []string
	for i := 0; i < set.Len(); i++ {
		if fn := set.At(i).Obj(); fn.Exported() {
			names = append(names, fn.Name())
		}
	}
	sort.Strings(names)
	return names
}

func difference(have, want []string) []string {
	allowed := make(map[string]bool, len(want))
	for _, w := range want {
		allowed[w] = true
	}
	var extra []string
	for _, h := range have {
		if !allowed[h] {
			extra = append(extra, h)
		}
	}
	return extra
}

func packageErrors(loaded []*packages.Package) []string {
	var errs []string
	for _, p := range loaded {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
	}
	return errs
}

// TestBaseMessageRefusesTypeThatIsNotUTF8: a type component that is not valid UTF-8 would reach the
// wire as U+FFFD with no error, so encoding the envelope refuses it (owner ruling, #9 comment
// 5970334875). BaseMessage.Validate checks only that each component is non-empty, so the refusal
// is at MarshalJSON, as the source's is. A valid component, U+FFFD itself included, reaches the
// wire unchanged.
func TestBaseMessageRefusesTypeThatIsNotUTF8(t *testing.T) {
	payload := func() message.Payload { return message.NewGenericJSON(map[string]any{"k": "v"}) }
	for _, mt := range []message.Type{
		{Domain: "\xff", Category: "json", Version: "v1"},
		{Domain: "core", Category: "js\xfeon", Version: "v1"},
		{Domain: "core", Category: "json", Version: "v\xe2\x82"},
	} {
		encoded, err := json.Marshal(message.NewBaseMessage(mt, payload(), "gw"))
		if err == nil {
			t.Errorf("marshal of type %q succeeded: %s", mt.String(), encoded)
			continue
		}
		if !errs.IsInvalid(err) {
			t.Errorf("marshal of type %q: %v, want an invalid-data error", mt.String(), err)
		}
	}

	mt := message.Type{Domain: "\uFFFD", Category: "温度", Version: "v1"}
	encoded, err := json.Marshal(message.NewBaseMessage(mt, payload(), "gw"))
	if err != nil {
		t.Fatalf("marshal of type %q: %v", mt.String(), err)
	}
	var envelope struct {
		Type struct{ Domain, Category, Version string } `json:"type"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Type.Domain != mt.Domain || envelope.Type.Category != mt.Category || envelope.Type.Version != mt.Version {
		t.Fatalf("type on the wire %+v, want %+v", envelope.Type, mt)
	}
}
