// Package objectstore is a storage.StreamableStore over one NATS ObjectStore
// bucket.
//
// NewStoreWithConfig creates the bucket, or opens it when it exists. The bucket
// has no TTL and no size limit: graph entities point at its objects for as long
// as they live (ADR-068). If an existing bucket's backing stream (OBJ_<bucket>)
// has either, construction removes it, logs a warning naming the bucket and what
// was removed, and fails with an error wrapping natsclient.ErrGraphBucketRetention
// if it cannot.
//
// Put replaces the object at a key; no earlier version is kept. Get and Open of a
// missing key return an error matching storage.ErrObjectNotFound. Delete of a
// missing key returns nil. List returns the matching names sorted, reading every
// object's metadata to do so. The Store keeps no copy of any object and has
// nothing to close.
//
// StoreContent stores a message.ContentStorable's body as raw bytes and returns a
// message.StorageReference that opens to that body. It refuses any other content.
//
// # Component
//
// Component wraps the Store with NATS ports. Register adds its factory to a
// component.Registry under the name "objectstore". Each message on an input port
// is decoded through the payload registry; if its payload is a
// message.ContentStorable, the body is stored through StoreContent and a
// StoredMessage (payload type storage.stored.v1, which RegisterPayloads registers)
// carrying the entity, its triples and the body's reference is published on the
// "stored" output port. Any other message is refused: the Component stores bodies
// only.
//
// The configuration has two keys, both optional: bucket_name (default MESSAGES) and
// ports. An unknown key is refused at construction, naming it. With no ports the
// Component has one input, "write", a core NATS subscription on
// storage.objectstore.write, and one output, "stored", on
// storage.objectstore.stored. A configured output must be named "stored", and a
// Component with an input must have it. An input may be core NATS or JetStream. A
// JetStream input must name a stream that exists: Start does not create it, waits
// for it, and fails if it never appears. A JetStream input that declares
// consumer_name is refused, because the durable consumer is named from the
// subject; its ack_wait and the other consumer settings are used.
//
// The default core NATS input keeps nothing: a message that is refused, or whose
// write fails, is logged at error level, counted in Health's ErrorCount, and gone.
// A JetStream input is at-least-once: a delivery is acknowledged only after its
// body is stored and its reference published. A refusal ends the delivery (Term); a
// failed store or publish is negatively acknowledged (NAK) for redelivery, after
// 30 seconds, or 5 seconds when the handler's context ended. Every refusal and
// failure is logged and counted. A redelivery after a failed publish stores the
// body again under a new key, so it can leave one stored object that no reference
// names. A delivery that reaches the consumer's MaxDeliver is not delivered again
// and the Component reports nothing about it.
//
// The Component offers its Store, under the instance name "objectstore", through
// ProvidedStores. That one name is stamped into every reference. A process runs one
// such Component: a second is refused when the component manager starts it.
//
// If a JetStream input's consume loop ends while the Component runs, or its durable
// consumer cannot be found on the server, Health reports the Component not healthy,
// naming the input. A lost consume loop is not rebound: the recovery is to restart
// the process. The check runs every five seconds, so the report can take that long.
// Whether the server's deleting a consumer ends the consume loop is not promised
// (observed once with nats.go v1.54.0). The lookup by name catches the cases where
// it does not.
//
// Stop drains every input and waits for its running handlers to return, through
// natsclient's WaitConsumerHandlers. The handlers keep running after the context
// given to Start ends; only Stop ends them.
package objectstore
