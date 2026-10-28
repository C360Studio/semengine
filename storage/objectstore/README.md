# objectstore

`objectstore.Store` keeps byte objects in one NATS ObjectStore bucket (NATS JetStream's store for large values,
named by a string key). It implements `storage.StreamableStore`: `Put`, `Get`, `Open`, `List` and `Delete`. It
also has `StoreContent`, which stores a payload's body text and returns a `message.StorageReference` that other
code resolves back to this Store.

This package has the Store only. The component that would wrap it with NATS ports and a registry
entry is not in this repository yet.

## Use

```go
store, err := objectstore.NewStoreWithConfig(ctx, client, objectstore.Config{BucketName: "MESSAGES"})
if err != nil {
    return err
}
err = store.Put(ctx, "events/a", []byte("hello"))
data, err := store.Get(ctx, "events/a")
keys, err := store.List(ctx, "events/")
err = store.Delete(ctx, "events/a")
```

`ExampleNewStoreWithConfig` in `example_test.go` is this code as a compiled Go example. `client` is a connected
`*natsclient.Client`. `NewStoreWithConfigAndMetrics` also takes a `*metric.MetricsRegistry` and records the Store's
counters and latencies on it, in names that start `semengine_objectstore_`; `NewStoreWithConfig` records nothing.

## What each call does

- `NewStoreWithConfig` creates the bucket, or opens it when it exists. `Config.BucketName` defaults to `MESSAGES`.
  `Config.InstanceName` is the name stamped into every reference the Store returns (`StorageReference.StorageInstance`)
  and defaults to the bucket name; the code that owns the Store sets it.
- `Put` replaces the object at the key. No earlier version is kept. NATS was observed to behave this way with nats.go
  v1.54.0 and nats-server v2.15.1 (probe P4, [PR #432 comment](https://github.com/C360Studio/semengine/pull/432#issuecomment-6445520357));
  the library does not promise it.
- `Get` and `Open` of a missing key return an error that matches `storage.ErrObjectNotFound` and is not transient.
- `Delete` of a key with no object returns nil.
- `List(prefix)` returns the names that start with the prefix, sorted, and an empty slice when none match. It reads
  the metadata of every object in the bucket and filters in the process, so its cost grows with the bucket.
- Every read goes to NATS. The Store keeps no copy of an object, holds no context and starts no goroutine, so it has
  nothing to close. A nil context is refused by every call as invalid input.

## No age or size limit

Graph entities point at stored objects for as long as the entities live, so an object that expires would strand
them (ADR-068, ADR-063). The bucket is created with no TTL and no size limit. If an existing bucket's backing stream
(`OBJ_<bucket>`) has either, construction removes it, logs a warning naming the bucket and what it removed, and
fails with an error wrapping `natsclient.ErrGraphBucketRetention` when it cannot. Removing a limit deletes no
object.

## StoreContent keeps only the body

`StoreContent(ctx, cs)` writes the text under `cs`'s `body` role as raw UTF-8 bytes at a new key
(`content/<yyyy>/<mm>/<dd>/<hh>/<entityID>_<uuid>`) and returns a reference with the Store's instance name, the key,
`text/plain; charset=utf-8` and the byte size. Opening the reference gives the body and nothing else.

It refuses with an invalid-input error, before writing anything, an invalid entity ID and any other non-empty
content: text under a field that is not the body's, text under a field that a non-body role also names, or a
non-empty binary field of a `message.BinaryStorable`. The error names each refused field. Put non-body text in the
graph as triples, which the producer emits. A payload with no content stores an empty object.

## Not here

These are not in this package: the in-memory data cache and `Close` of the SemStreams original (dropped by the
owner, [PR #432 comment][cache-ruling]),
the component, its ports, and a way to name several Store instances in one process (left to the
component's change; ADR-063 leaves it open).

[cache-ruling]: https://github.com/C360Studio/semengine/pull/432#issuecomment-6446982969
