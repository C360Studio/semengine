package objectstore

import (
	"encoding/json"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// RegisterPayloads registers StoredMessage as storage.stored.v1 with reg. Importing this package
// registers nothing: the consumer's composition root calls RegisterPayloads on the registry it
// gives its decoders, and a test on the registry it builds.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Factory:     func() any { return &StoredMessage{} },
		Domain:      "storage",
		Category:    "stored",
		Version:     "v1",
		Description: "Message with semantic data and storage reference",
	})
}

// StoredMessage is a message.Storable for content the Component has stored: the entity and
// triples of the original message, and the reference that opens to its stored body. A reader
// resolves the reference through the store registry (ADR-063).
//
// Its fields are unexported, so a received StoredMessage cannot be changed; MarshalJSON and
// UnmarshalJSON spell its wire form.
type StoredMessage struct {
	entityID    string
	triples     []message.Triple
	storageRef  *message.StorageReference
	storedAt    time.Time
	messageType string
}

// NewStoredMessage returns a StoredMessage for graphable's entity and triples, stored at
// storageRef now. messageType is the original message's type.
func NewStoredMessage(graphable graph.Graphable, storageRef *message.StorageReference, messageType string) *StoredMessage {
	return &StoredMessage{
		entityID:    graphable.EntityID(),
		triples:     graphable.Triples(),
		storageRef:  storageRef,
		storedAt:    time.Now(),
		messageType: messageType,
	}
}

// EntityID implements graph.Graphable.
func (s *StoredMessage) EntityID() string {
	return s.entityID
}

// Triples implements graph.Graphable.
func (s *StoredMessage) Triples() []message.Triple {
	return s.triples
}

// StorageRef implements message.Storable.
func (s *StoredMessage) StorageRef() *message.StorageReference {
	return s.storageRef
}

// StoredAt returns when the message was stored.
func (s *StoredMessage) StoredAt() time.Time {
	return s.storedAt
}

// MessageType returns the original message's type.
func (s *StoredMessage) MessageType() string {
	return s.messageType
}

// storedMessageWire is StoredMessage's JSON form.
type storedMessageWire struct {
	EntityID    string                    `json:"entity_id"`
	Triples     []message.Triple          `json:"triples"`
	StorageRef  *message.StorageReference `json:"storage_ref"`
	StoredAt    time.Time                 `json:"stored_at"`
	MessageType string                    `json:"message_type"`
}

// MarshalJSON implements json.Marshaler.
func (s *StoredMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(storedMessageWire{
		EntityID:    s.entityID,
		Triples:     s.triples,
		StorageRef:  s.storageRef,
		StoredAt:    s.storedAt,
		MessageType: s.messageType,
	})
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *StoredMessage) UnmarshalJSON(data []byte) error {
	var wire storedMessageWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*s = StoredMessage{
		entityID:    wire.EntityID,
		triples:     wire.Triples,
		storageRef:  wire.StorageRef,
		storedAt:    wire.StoredAt,
		messageType: wire.MessageType,
	}
	return nil
}

// Schema implements message.Payload: storage.stored.v1.
func (s *StoredMessage) Schema() message.Type {
	return message.Type{Domain: "storage", Category: "stored", Version: "v1"}
}

// Validate implements message.Payload. It refuses an entity ID pkg/types refuses, keeping that
// error's code, and a missing reference.
func (s *StoredMessage) Validate() error {
	if err := semtypes.ValidateEntityID(s.entityID); err != nil {
		return err
	}
	if s.storageRef == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "StoredMessage", "Validate", "storage_ref is required")
	}
	return nil
}

var (
	_ message.Storable = (*StoredMessage)(nil)
	_ message.Payload  = (*StoredMessage)(nil)
)
