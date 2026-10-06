package message_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/payloadregistry"
)

func TestReviewerGenericJSONPreservesIntegerValue(t *testing.T) {
	reg := payloadregistry.New()
	if err := message.RegisterPayloads(reg); err != nil {
		t.Fatal(err)
	}
	for _, number := range []any{int64(9007199254740993), uint64(18446744073709551615), json.Number("9007199254740993")} {
		payload := message.NewGenericJSON(map[string]any{"value": number})
		msg := message.NewBaseMessage(payload.Schema(), payload, "review-probe")
		if err := msg.Validate(); err != nil {
			t.Fatal(err)
		}
		before, err := payload.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := message.NewDecoder(reg).Decode(wire)
		if err != nil {
			t.Fatal(err)
		}
		after, err := decoded.Payload().MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Errorf("accepted %T numeric value changed: before=%s after=%s", number, before, after)
		}
	}
}
