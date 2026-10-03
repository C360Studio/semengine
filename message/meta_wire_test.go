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
