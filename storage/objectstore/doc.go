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
package objectstore
