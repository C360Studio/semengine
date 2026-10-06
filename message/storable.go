package message

// StorageReference points to where the full message data is stored.
// This enables lightweight message passing where components can reference
// stored data without transmitting the full payload.
//
// The StorageReference pattern supports the "store once, reference everywhere"
// architecture, reducing data duplication and enabling efficient processing
// of large messages.
//
// Example usage:
//   - A storage component stores the full message and returns a StorageReference
//   - A graph consumer receives a Storable with a reference to the full data
//   - Components can fetch full data only when needed
type StorageReference struct {
	// StorageInstance identifies which storage component holds the data.
	// This enables federation across multiple storage instances.
	// Examples: "message-store", "cache-1", "objectstore-primary"
	StorageInstance string `json:"storage_instance"`

	// Key is the storage-specific key to retrieve the data.
	// Format depends on the storage backend but typically includes
	// time-based partitioning for efficient retrieval.
	// Examples: "2025/01/13/14/msg_abc123", "robotics/drone/1/latest"
	Key string `json:"key"`

	// ContentType specifies the MIME type of the stored content.
	// This helps consumers understand how to process the data.
	// Examples: "application/json", "application/protobuf", "application/avro"
	ContentType string `json:"content_type"`

	// Size is an optional hint about the stored data size in bytes.
	// This helps consumers decide whether to fetch the full data.
	// A value of 0 indicates the size is unknown.
	Size int64 `json:"size,omitempty"`
}

// Storable is a payload that names its entity and its facts (EntityID and
// Triples) and carries a reference to where its full data is stored.
// Components that implement Storable can provide both semantic
// information and a reference to their full data.
//
// This interface enables the lightweight message pattern where:
//  1. Domain processors create messages with semantic data
//  2. A storage component stores the full message and adds a StorageReference
//  3. A graph consumer receives the Storable with both semantics and reference
//  4. Consumers can access full data via StorageReference when needed
//
// SemEngine has no graph package yet. In SemStreams, where this file comes
// from, EntityID and Triples are also the methods of graph.Graphable, so a
// Storable there satisfies that interface too.
//
// Example implementation:
//
//	type StoredEntity struct {
//	    entityID string
//	    triples  []Triple
//	    storage  *StorageReference
//	}
//
//	func (s *StoredEntity) EntityID() string { return s.entityID }
//	func (s *StoredEntity) Triples() []Triple { return s.triples }
//	func (s *StoredEntity) StorageRef() *StorageReference { return s.storage }
type Storable interface {
	// EntityID returns deterministic 6-part ID: org.platform.system.domain.type.instance
	EntityID() string

	// Triples returns all facts about this entity
	Triples() []Triple

	// StorageRef returns reference to where full data is stored.
	// May return nil if data is not stored externally.
	StorageRef() *StorageReference
}
