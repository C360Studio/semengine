# objectstore

`objectstore.Store` keeps byte objects in one NATS ObjectStore bucket (NATS JetStream's store for large values,
named by a string key). It implements `storage.StreamableStore`: `Put`, `Get`, `Open`, `List` and `Delete`. It
also has `StoreContent`, which stores a payload's body text and returns a `message.StorageReference` that other
code resolves back to this Store.

The package also has `Component`, which wraps the Store with NATS ports so that other components can hand it
messages whose body should be stored. See "The Component" below.

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

## The Component

`objectstore.Register(registry)` adds the factory under the name `objectstore`. A running Component subscribes to its
input ports; for each message it decodes the payload through the payload registry, and when the payload is a
`message.ContentStorable` it stores the body with `StoreContent` and publishes a `StoredMessage` (payload type
`storage.stored.v1`; `RegisterPayloads` registers it) on the `stored` output. The `StoredMessage` carries the entity ID,
its triples and the reference, so a reader opens the body through the store registry. Any other message is refused:
the Component stores bodies only.

### Configuration

Two keys, both optional. Any other key is refused when the Component is built, naming it.

- `bucket_name`: the NATS ObjectStore bucket, default `MESSAGES`.
- `ports`: `inputs` and `outputs`. With no `ports`, the Component has the input `write` (core NATS, subject
  `storage.objectstore.write`) and the output `stored` (core NATS, subject `storage.objectstore.stored`).

```json
{
  "bucket_name": "MESSAGES",
  "ports": {
    "inputs": [
      {"name": "write", "config": {"kind": "jetstream", "stream_name": "STORAGE", "subjects": ["storage.objectstore.write"]}}
    ],
    "outputs": [
      {"name": "stored", "config": {"kind": "nats", "subject": "storage.objectstore.stored"}}
    ]
  }
}
```

- An input is core NATS or JetStream, one subject each. A Component with an input must have the `stored` output, and
  `stored` is the only output name allowed, because a body stored with no reference published reaches nobody. A
  Component with no inputs only offers its Store and needs no output.
- A JetStream input's stream must already exist. Start waits for it and fails if it does not appear; it does not
  create it. The durable consumer is created with a name derived from the subject, so an input that declares
  `consumer_name` is refused. The other consumer settings, `ack_wait` included, reach the consumer
  (`TestJetStreamInputAckWaitReachesConsumer`), and delivery starts at the beginning of the stream unless the port
  says otherwise.
- The Component registers its Store under the name `objectstore` through `ProvidedStores`, and every reference it
  publishes carries that name. A process runs one such Component; starting a second one fails the boot
  (`TestSecondComponentRefusedAtStart`).

### What happens to a message

| Input | Stored and published | Refused or failed |
| --- | --- | --- |
| core NATS (the default) | the message is done | logged at error level, counted in `ErrorCount`, then gone |
| JetStream | acknowledged after the body is stored and its reference published | refused (not decodable, not a `ContentStorable`, or content the Store refuses): terminated, not redelivered; a failed store or publish: NAK, redelivered after 30 seconds (5 seconds if the handler's context ended); each is logged at error level and counted |

The core NATS input is at-most-once: NATS keeps nothing to redeliver, so a refused message is lost after the log line.
The JetStream input is at-least-once. If publishing the reference fails after the body was stored, the redelivery
stores the body again under a new key, so one object can be left that no reference names. A delivery that reaches the
consumer's `MaxDeliver` limit is not delivered again, and the Component reports nothing about it.

### Health, Stop and the Start context

- If a JetStream input's consume loop ends while the Component runs, or its durable consumer cannot be found by name,
  `Health` reports the Component not healthy and names the input (`TestLostJetStreamInputReportsUnhealthy`,
  `TestMemoryInputLostAtBrokerRestartStaysUnhealthy`). A check runs every five seconds. The Component does not bind the
  input again; restarting the process is the recovery. Whether the server deleting a consumer ends the consume loop is
  not promised (observed once with nats.go v1.54.0); the lookup by name is what catches the other cases.
- `Stop` drains every input and waits for its running handlers to return (`TestStopJoinsHandlerAfterConnectionLoss`). If
  `Stop`'s context ends first, the inputs stay held and a later `Stop` finishes the job
  (`TestFailedCleanupRetainsInputsForLaterStop`).
- The input handlers keep running after the context given to `Start` ends; only `Stop` ends them
  (`TestStartContextEndKeepsBothLanesWriting`).

## Not here

These are not in this package: the in-memory data cache and `Close` of the SemStreams original (dropped by the
owner, [PR #432 comment][cache-ruling]), the SemStreams Component's raw lane (storing the wire bytes of any message
under a generated key), its `events` output and its `api` input, and a way to name several Store instances in one
process (ADR-063 left it open; the answer is one name, `objectstore`, per process).

[cache-ruling]: https://github.com/C360Studio/semengine/pull/432#issuecomment-6446982969
