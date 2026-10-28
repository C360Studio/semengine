package objectstore

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// testGraphable is the graph.Graphable a StoredMessage is built from.
type testGraphable struct {
	entityID string
	triples  []message.Triple
}

func (g *testGraphable) EntityID() string          { return g.entityID }
func (g *testGraphable) Triples() []message.Triple { return g.triples }

// TestStoredMessageRoundTripsThroughDecoder: RegisterPayloads registers StoredMessage as
// storage.stored.v1, and a StoredMessage wrapped in a BaseMessage, marshalled and decoded by the
// production decoder comes back with its entity ID, triples, reference, message type and storage
// time unchanged (object-store spec, "StoredMessage is a registered payload").
func TestStoredMessageRoundTripsThroughDecoder(t *testing.T) {
	const entityID = "acme.ops.docs.library.document.readme"
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	triples := []message.Triple{
		{Subject: entityID, Predicate: "docs.document.title", Object: "Read me", Source: "test", Timestamp: at, Confidence: 1},
		{Subject: entityID, Predicate: "docs.document.size", Object: float64(42), Source: "test", Timestamp: at, Confidence: 0.5},
	}
	ref := &message.StorageReference{StorageInstance: "objectstore", Key: "content/readme", ContentType: "text/plain", Size: 7}

	stored := NewStoredMessage(&testGraphable{entityID: entityID, triples: triples}, ref, "docs.document.v1")
	if got := stored.Schema().Key(); got != "storage.stored.v1" {
		t.Fatalf("Schema().Key() = %q, want storage.stored.v1", got)
	}
	data, err := json.Marshal(message.NewBaseMessage(stored.Schema(), stored, "objectstore"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded, err := message.NewDecoder(payloadfixture.NewWithSubset(t, RegisterPayloads)).Decode(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.Payload().(*StoredMessage)
	if !ok {
		t.Fatalf("decoded payload is %T, want *StoredMessage", decoded.Payload())
	}
	if got.EntityID() != entityID {
		t.Errorf("EntityID = %q, want %q", got.EntityID(), entityID)
	}
	if !reflect.DeepEqual(got.Triples(), triples) {
		t.Errorf("Triples = %+v, want %+v", got.Triples(), triples)
	}
	if !reflect.DeepEqual(got.StorageRef(), ref) {
		t.Errorf("StorageRef = %+v, want %+v", got.StorageRef(), ref)
	}
	if got.MessageType() != "docs.document.v1" {
		t.Errorf("MessageType = %q, want docs.document.v1", got.MessageType())
	}
	if !got.StoredAt().Equal(stored.StoredAt()) || got.StoredAt().IsZero() {
		t.Errorf("StoredAt = %v, want %v", got.StoredAt(), stored.StoredAt())
	}
}

// TestStoredMessageValidate: Validate accepts a canonical entity ID up to 256 bytes with a
// reference, refuses an empty, non-canonical or 257-byte ID with the entity-ID error code, and
// refuses a missing reference; the production marshal refuses the same message, keeping the code
// (pin stored_message_contract_test.go).
func TestStoredMessageValidate(t *testing.T) {
	boundaryID := "a.b.c.d.e." + strings.Repeat("x", semtypes.MaxEntityIDBytes-10)
	if len(boundaryID) != semtypes.MaxEntityIDBytes {
		t.Fatalf("boundary ID is %d bytes, want %d", len(boundaryID), semtypes.MaxEntityIDBytes)
	}
	ref := &message.StorageReference{StorageInstance: "objectstore", Key: "k"}

	for _, tc := range []struct {
		name     string
		entityID string
		ref      *message.StorageReference
		wantCode string // entity-ID error code wanted, or "" for another refusal
		wantErr  string // text wanted in a non-entity-ID refusal
	}{
		{name: "canonical ID", entityID: "acme.ops.plan.fanout.unit.1", ref: ref},
		{name: "256-byte ID", entityID: boundaryID, ref: ref},
		{name: "empty ID", entityID: "", ref: ref, wantCode: semtypes.ErrorCodeEntityIDInvalid},
		{name: "non-canonical ID", entityID: "legacy-id", ref: ref, wantCode: semtypes.ErrorCodeEntityIDInvalid},
		{name: "257-byte ID", entityID: boundaryID + "x", ref: ref, wantCode: semtypes.ErrorCodeEntityIDInvalid},
		{name: "no reference", entityID: "acme.ops.plan.fanout.unit.1", wantErr: "storage_ref is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := NewStoredMessage(&testGraphable{entityID: tc.entityID}, tc.ref, "test.payload.v1")
			err := stored.Validate()
			_, marshalErr := message.NewBaseMessage(stored.Schema(), stored, "objectstore").MarshalJSON()
			if tc.wantCode == "" && tc.wantErr == "" {
				if err != nil || marshalErr != nil {
					t.Fatalf("Validate = %v, MarshalJSON = %v, want both nil", err, marshalErr)
				}
				return
			}
			if err == nil || marshalErr == nil {
				t.Fatalf("Validate = %v, MarshalJSON = %v, want both to refuse", err, marshalErr)
			}
			if tc.wantErr != "" {
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Validate = %v, want it to say %q", err, tc.wantErr)
				}
				return
			}
			for _, got := range []error{err, marshalErr} {
				var classified *errs.ClassifiedError
				if !errors.As(got, &classified) || classified.Code != tc.wantCode {
					t.Errorf("error %v carries no code %q", got, tc.wantCode)
				}
			}
		})
	}
}
