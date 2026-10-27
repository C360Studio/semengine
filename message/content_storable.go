package message

// ContentStorable extends Storable for payloads with large content fields.
//
// This interface enables the "process → store → graph" pattern where:
//  1. Processor creates semantic understanding (triples with metadata only)
//  2. ObjectStore stores raw content and returns StorageReference
//  3. GraphProcessor receives ContentStorable with both semantics and content reference
//  4. EmbeddingWorker uses ContentFields to find text for embedding
//
// The key insight is that ContentStorable separates metadata (in triples) from
// content (in ObjectStore), while providing a semantic map of content structure
// via ContentFields. This avoids bloating triples with large text and enables
// efficient embedding extraction without hardcoded field name coupling.
//
// What the store keeps: objectstore.Store.StoreContent stores only the text of
// the "body" role, as raw bytes, and the StorageReference it returns opens to
// exactly that text. A payload with any other non-empty content (text under a
// field another role names, an undeclared field, or a non-empty binary field of
// a BinaryStorable) is refused with an invalid-input error before anything is
// written. Text that is not the body, such as a title or an abstract, belongs in
// the graph as triples, which the producer emits from Triples().
//
// Example implementation (a document whose only stored content is its body; the
// title and description reach the graph as triples):
//
//	type Document struct {
//	    Title       string
//	    Description string
//	    Body        string
//	    storageRef  *StorageReference
//	}
//
//	func (d *Document) EntityID() string { return d.entityID }
//	func (d *Document) Triples() []Triple { return metadataTriples } // NO body
//	func (d *Document) StorageRef() *StorageReference { return d.storageRef }
//
//	func (d *Document) ContentFields() map[string]string {
//	    return map[string]string{
//	        "body": "body", // semantic role → field name
//	    }
//	}
//
//	func (d *Document) RawContent() map[string]string {
//	    return map[string]string{"body": d.Body}
//	}
type ContentStorable interface {
	Storable // EntityID() + Triples() + StorageRef()

	// ContentFields returns semantic role → field name mapping.
	//
	// This tells consumers how to find content in the stored data without
	// hardcoding field names. Keys are semantic roles understood by consumers
	// (like embedding workers), values are field names in RawContent().
	//
	// Standard semantic roles:
	//   - "body":     Primary text content (full document text)
	//   - "abstract": Brief summary or description
	//   - "title":    Document title
	//
	// Example return value:
	//   {"body": "content", "abstract": "description", "title": "title"}
	//
	// The map should only include roles that have non-empty content.
	ContentFields() map[string]string

	// RawContent returns the content to store in ObjectStore.
	//
	// Field names in this map should match values in ContentFields().
	// objectstore.Store.StoreContent stores the body field's text as raw
	// bytes and refuses any other non-empty field (see the type's doc), so a
	// consumer that opens the StorageRef reads the body and nothing else.
	//
	// Example return value:
	//   {"body": "Full document text..."}
	RawContent() map[string]string
}

// ContentRole constants define standard semantic roles for ContentFields.
// Using these constants ensures consistency across implementations.
const (
	// ContentRoleBody is the primary text content (full document text).
	// Embedding workers prioritize this role for text extraction.
	ContentRoleBody = "body"

	// ContentRoleAbstract is a brief summary or description.
	// Used when body is not available or for additional context.
	ContentRoleAbstract = "abstract"

	// ContentRoleTitle is the document or entity title.
	// Typically included in embeddings for context.
	ContentRoleTitle = "title"

	// ContentRoleMedia is the primary media content (video, image, audio).
	// Used for binary content that may be processed by multimodal embedders.
	ContentRoleMedia = "media"

	// ContentRoleThumbnail is a preview image for media content.
	ContentRoleThumbnail = "thumbnail"
)

// BinaryContent describes a binary field for storage.
// Used by BinaryStorable implementations to provide binary data
// along with its content type for proper handling.
type BinaryContent struct {
	ContentType string // MIME type: "image/jpeg", "video/mp4", "application/pdf"
	Data        []byte // The actual binary data
}

// BinaryStorable extends ContentStorable for payloads with binary content.
//
// This interface enables storage of images, videos, PDFs, and other binary
// files alongside text content. Binary data is stored directly in ObjectStore
// (no base64 encoding) with JSON metadata referencing the binary keys.
//
// Example implementation:
//
//	type VideoDocument struct {
//	    Title       string
//	    Description string
//	    VideoData   []byte
//	    Thumbnail   []byte
//	}
//
//	func (v *VideoDocument) RawContent() map[string]string {
//	    return map[string]string{
//	        "title":       v.Title,
//	        "description": v.Description,
//	    }
//	}
//
//	func (v *VideoDocument) BinaryFields() map[string]BinaryContent {
//	    return map[string]BinaryContent{
//	        "video":     {ContentType: "video/mp4", Data: v.VideoData},
//	        "thumbnail": {ContentType: "image/jpeg", Data: v.Thumbnail},
//	    }
//	}
type BinaryStorable interface {
	ContentStorable

	// BinaryFields returns binary content to store in ObjectStore.
	//
	// Each field is stored as a separate binary blob with its own key.
	// The returned map keys are field names that can be referenced in
	// ContentFields for semantic role mapping.
	//
	// Example return value:
	//   {"video": {ContentType: "video/mp4", Data: videoBytes},
	//    "thumbnail": {ContentType: "image/jpeg", Data: thumbBytes}}
	BinaryFields() map[string]BinaryContent
}
