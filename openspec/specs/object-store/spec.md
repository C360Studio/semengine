# object-store Specification

## Purpose
`storage/objectstore` stores message bodies and other content in a NATS JetStream object store bucket and hands
back a reference to it. This spec states how a store opens its bucket, what it writes and refuses, how it reads,
lists and deletes, and which metrics it records. The Component that wires it into a flow is a later change.

## Requirements

### Requirement: A store opens its bucket with no lifecycle eviction

`objectstore.NewStoreWithConfig` SHALL create the configured ObjectStore bucket or open it when it exists, then SHALL
strip a TTL or binding `MaxBytes` from its backing stream `OBJ_<bucket>` with a logged warning, and SHALL fail with a
fatal error wrapping `natsclient.ErrGraphBucketRetention` when either remains. It SHALL refuse a nil context or nil
client before any I/O. The Store SHALL keep no in-memory copy of objects and SHALL have no `Close`.

#### Scenario: A fresh bucket

- **WHEN** a Store is constructed on a bucket that does not exist
- **THEN** the bucket's backing stream has `MaxAge` 0 and `MaxBytes` 0 or less
- **AND** the test that verifies this is `TestFreshBucketHasNoLifecycleRetention`

#### Scenario: A bucket created with a TTL

- **WHEN** a Store is constructed on a bucket whose backing stream has a `MaxAge` and a `MaxBytes`
- **THEN** both are removed, a warning names the bucket and the removed values, and construction succeeds
- **AND** the test that verifies this is `TestRetentionGuardStripsAndAsserts`

#### Scenario: Retention that cannot be removed

- **WHEN** the backing stream still carries a binding retention after the update
- **THEN** construction fails with a fatal error that matches `natsclient.ErrGraphBucketRetention`
- **AND** the test that verifies this is `TestRetentionGuardFailsClosed`

#### Scenario: A nil context

- **WHEN** `NewStoreWithConfig` is called with a nil context
- **THEN** it returns an invalid-input error and creates nothing
- **AND** the test that verifies this is `TestNewStoreRefusesNilContext`

#### Scenario: No cached copy

- **WHEN** one Store gets a key, a second Store on the same bucket deletes it, and the first Store gets it again
- **THEN** the second `Get` returns an error that matches `storage.ErrObjectNotFound`
- **AND** the test that verifies this is `TestGetReadsNATSEveryTime`

### Requirement: Put replaces the object at its key

`Store.Put` SHALL store the given bytes at the key, replacing any object already there; a later `Get` of the key SHALL
return the last bytes put. The Store SHALL NOT keep earlier versions.

#### Scenario: Two puts on one key

- **WHEN** `Put` is called twice on one key with different bytes
- **THEN** `Get` of the key returns the second bytes
- **AND** the test that verifies this is `TestPutReplacesObjectAtKey`

### Requirement: A missing object is not found, not transient

`Store.Get` and `Store.Open` of a key with no object SHALL return an error that matches `storage.ErrObjectNotFound`
and is not classified transient.

#### Scenario: Get and Open of a missing key

- **WHEN** `Get` and then `Open` are called on a key that was never written
- **THEN** each error matches `storage.ErrObjectNotFound` and `errs.IsTransient` is false for it
- **AND** the test that verifies this is `TestGetAndOpenReportMissingAsNotFound`

### Requirement: Delete of a missing key succeeds

`Store.Delete` SHALL return nil for a key that was never written, and SHALL classify any other failure as transient.
Deleting an already-deleted key returns what nats.go returns (nil in the probe, not promised).

#### Scenario: Deleting a missing key

- **WHEN** `Delete` is called on a key that was never written
- **THEN** it returns nil
- **AND** the test that verifies this is `TestDeleteMissingKeySucceeds`

### Requirement: Operation histories match a key-value model

Any sequence of `Put`, `Delete`, `Get` and `List` over one bucket, through one or two Stores, SHALL give the results a
plain key-value map gives: the last put value, not found after a delete, and the sorted keys under a prefix.

#### Scenario: A generated history

- **WHEN** up to 30 generated operations over overlapping keys run on two Stores over one bucket
- **THEN** every result equals the model's
- **AND** the test that verifies this is `TestStoreHistoryMatchesModel`

### Requirement: List returns sorted keys and an empty list for none

`Store.List` SHALL return the names of the objects whose name starts with the prefix, in lexicographic order, and
SHALL return an empty, non-nil slice and a nil error when the bucket is empty or no name matches.

#### Scenario: An empty bucket

- **WHEN** `List` is called on a bucket with no objects
- **THEN** it returns an empty slice and a nil error
- **AND** the test that verifies this is `TestListSortedAndEmpty`

#### Scenario: Names put out of order

- **WHEN** objects `c/2`, `a/1` and `b/9` are put in that order and `List` is called with an empty prefix
- **THEN** it returns `a/1`, `b/9`, `c/2`

### Requirement: StoreContent references the raw body

`Store.StoreContent` SHALL refuse, before writing, an invalid entity ID or any non-empty field or role other than
the `body` role, binary fields included. Otherwise it SHALL write the body as raw bytes (empty when there is
none) under a key with a per-call nonce, and SHALL return a reference with the Store's instance name, that key,
`text/plain; charset=utf-8` and the body's size. Opening the reference SHALL return the body.

#### Scenario: An invalid entity ID

- **WHEN** `StoreContent` is called with a payload whose entity ID is not six parts
- **THEN** it returns an invalid-input error and the bucket holds no new object
- **AND** the test that verifies this is `TestStoreContentRefusesInvalidEntityIDBeforeWriting`

#### Scenario: The reference opens to the body

- **WHEN** `StoreContent` stores a payload with only a body, and the returned reference is resolved through a store
  registry and opened
- **THEN** `Open` returns exactly the body bytes, and the reference's size is the body's length
- **AND** the test that verifies this is `TestStoreContentReferenceOpensToBody`

#### Scenario: A body with a description

- **WHEN** `StoreContent` is called with a payload whose content is a body and a non-empty description
- **THEN** it returns an invalid-input error naming `description`, and the bucket holds no new object
- **AND** the test that verifies this is `TestStoreContentRefusesExtras`

#### Scenario: A description without a body

- **WHEN** `StoreContent` is called with a payload whose only content is a non-empty description
- **THEN** it returns an invalid-input error naming `description`, and the bucket holds no new object
- **AND** the test that verifies this is `TestStoreContentRefusesExtras`

#### Scenario: A non-body role names the body's field

- **WHEN** `StoreContent` is called with a payload whose `body` and `abstract` roles both name one non-empty field
- **THEN** it returns an invalid-input error naming the `abstract` role and that field, and nothing is written
- **AND** the test that verifies this is `TestStoreContentRefusesNonBodyRoleAliasedToBody`

#### Scenario: A payload with a binary field

- **WHEN** `StoreContent` is called with a payload that implements `message.BinaryStorable` with one non-empty binary
  field
- **THEN** it returns an invalid-input error and the bucket holds no new object
- **AND** the test that verifies this is `TestStoreContentRefusesExtras`

#### Scenario: A body with an empty binary field

- **WHEN** `StoreContent` is called with a body and a binary field whose data is empty
- **THEN** it stores the body and returns its reference
- **AND** the test that verifies this is `TestStoreContentRefusesExtras`

#### Scenario: A payload with no content

- **WHEN** `StoreContent` stores a payload with no body and no other content
- **THEN** it returns a reference whose object is empty
- **AND** the test that verifies this is `TestStoreContentWithNoContentStoresEmptyObject`

#### Scenario: Many calls for one entity

- **WHEN** keys are generated for 10,000 calls with one entity ID and field name
- **THEN** no two keys are equal
- **AND** the test that verifies this is `TestStoreContentKeysAreUniquePerCall`

### Requirement: Store calls refuse a nil context

Every exported `Store` method that takes a context SHALL return an invalid-input error for a nil context before any
I/O.

#### Scenario: Each method with a nil context

- **WHEN** `Put`, `Get`, `Open`, `List`, `Delete` and `StoreContent` are each called with a nil context
- **THEN** each returns an invalid-input error
- **AND** the test that verifies this is `TestStoreMethodsRefuseNilContext`

### Requirement: Store metrics live on the injected registry

`NewStoreWithConfigAndMetrics` SHALL register the Store's collectors through `metric.RegisterOrGet` on the registry it
is given, under the `semengine_objectstore_` prefix, with a `bucket` label. `NewStoreWithConfig` SHALL register
nothing. Stores on any buckets SHALL share collectors.

#### Scenario: A store with a registry

- **WHEN** a Store is constructed with a registry and serves one `Put`
- **THEN** the registry holds `semengine_objectstore_write_operations_total` with label `bucket` and one count
- **AND** the test that verifies this is `TestStoreMetricsRegisterOnInjectedRegistry`

#### Scenario: Two stores on one bucket

- **WHEN** two Stores are constructed on one bucket with one registry
- **THEN** both constructions succeed and their counts land on one collector
- **AND** the test that verifies this is `TestTwoStoresOnOneBucketShareCollectors`

#### Scenario: Two buckets on one registry

- **WHEN** Stores on two different buckets are constructed with one registry
- **THEN** both constructions succeed and each bucket's counts carry its own `bucket` label value
- **AND** the test that verifies this is `TestTwoBucketsOnOneRegistryBothConstruct`

### Requirement: The Component's configuration is strict

The objectstore Component SHALL accept only `bucket_name` and `ports`, and SHALL refuse at construction any other
key, `data_cache` included, any output port other than `stored`, an input with no `stored` output, and a JetStream
input declaring `consumer_name`, each with an error naming it.

#### Scenario: A JetStream input declaring consumer_name

- **WHEN** the Component is built with a JetStream input that declares `consumer_name`
- **THEN** construction fails naming the port and `consumer_name`
- **AND** the test that verifies this is `TestComponentRefusesUnsupportedInputs`

#### Scenario: A data_cache key

- **WHEN** the Component is built from `{"data_cache": {"enabled": false}}`
- **THEN** construction fails naming `data_cache`
- **AND** the test that verifies this is `TestComponentConfigRefusesUnknownKey`

#### Scenario: An input with no stored output

- **WHEN** the configuration declares a `write` input and no outputs
- **THEN** construction fails naming the missing `stored` output
- **AND** the test that verifies this is `TestComponentRefusesInputWithoutStoredOutput`

#### Scenario: An events output

- **WHEN** the configured outputs include a port named `events`
- **THEN** construction fails naming `events`
- **AND** the test that verifies this is `TestComponentRefusesOutputOtherThanStored`

### Requirement: The Component stores ContentStorable bodies and emits their reference

For each message on an input whose payload is a `message.ContentStorable`, the Component SHALL store it through
`StoreContent` and, when a `stored` output is configured, SHALL publish a `StoredMessage` carrying the payload's entity,
triples and that reference, which opens to the body.

#### Scenario: A document on the write input

- **WHEN** a `ContentStorable` with only a body is published on the `write` input
- **THEN** a `StoredMessage` on `stored` carries a reference with `StorageInstance` `objectstore` that opens to the body
- **AND** the test that verifies this is `TestWriteLaneStoresBodyAndEmitsReference`

### Requirement: The Component refuses other messages loudly

A message that does not decode, is not `ContentStorable`, or that `StoreContent` refuses SHALL store nothing, SHALL be
logged at error level and counted in `Health().ErrorCount`, and SHALL be terminated on a JetStream input. A core-NATS
input SHALL be at-most-once: such a message is dropped. A JetStream input SHALL be at-least-once. A redelivery after a
failed reference publish leaves one extra, unreferenced object; a delivery past `MaxDeliver` is not reported (#36).

#### Scenario: Each refusal on each lane

- **WHEN** each refused kind is published on a core-NATS input and on a JetStream input
- **THEN** nothing is stored, `ErrorCount` rises by one, and the JetStream delivery is terminated
- **AND** the test that verifies this is `TestWriteLaneRefusals`

#### Scenario: The reference cannot be published

- **WHEN** the body is stored but publishing the `StoredMessage` fails, and the redelivery succeeds
- **THEN** the first delivery is negatively acknowledged and the second acknowledged, leaving two stored objects of
  which one is referenced
- **AND** the test that verifies this is `TestStoredEmitFailureNaksThenRedeliveryStores`

### Requirement: Ending the Start context stops nothing

After `Start` returned, ending the context given to `Start` SHALL NOT stop either input lane: later messages on core-NATS
and JetStream inputs SHALL still be stored. The Component SHALL pass the lifecycle suite.

#### Scenario: Both lanes after the Start context ended

- **WHEN** the Start context is cancelled and a document is then published on each input
- **THEN** both bodies are stored
- **AND** the tests that verify this are `TestStartContextEndKeepsBothLanesWriting` and `TestObjectStoreLifecycleSuite`

### Requirement: A lost JetStream input makes the Component unhealthy

While the Component runs, a JetStream input whose consume loop ended before `Stop` SHALL make `Health` unhealthy,
naming the input and the restart, until the process restarts; an input whose consumer cannot be found SHALL make it
unhealthy, naming the consumer, until a later check finds it.

#### Scenario: The input's consumer is deleted

- **WHEN** the durable consumer of a running Component's JetStream input is deleted while a pull waits at the server
- **THEN** `Health` becomes unhealthy naming that input and the restart
- **AND** the test that verifies this is `TestLostJetStreamInputReportsUnhealthy`

#### Scenario: A memory input stream lost at a broker restart

- **WHEN** a running Component's memory-backed input stream is lost at a broker restart and created again, so the
  consume loop keeps running but its consumer cannot be found
- **THEN** `Health` is unhealthy naming that consumer, before and after a message is published to the stream
- **AND** the test that verifies this is `TestMemoryInputLostAtBrokerRestartStaysUnhealthy`

### Requirement: One objectstore store per process

The Component SHALL stamp and offer its Store as `StorageInstance` `objectstore` from `ProvidedStores`, and nil before
`Start`; a second started Component in one process SHALL be refused at `Start` by the store registry.

#### Scenario: Two Components

- **WHEN** two objectstore Components are started by one component manager
- **THEN** the second Start fails naming the instance `objectstore`
- **AND** the tests that verify this are `TestProvidedStoresUnderObjectstore` and `TestSecondComponentRefusedAtStart`

### Requirement: StoredMessage is a registered payload

`RegisterPayloads` SHALL register `StoredMessage` as `storage.stored.v1`; a `StoredMessage` SHALL round-trip through
`message.NewDecoder` and SHALL refuse validation with an invalid entity ID or no reference.

#### Scenario: Round trip

- **WHEN** a `StoredMessage` is wrapped in a `BaseMessage`, marshalled and decoded
- **THEN** its entity ID, triples and reference are unchanged
- **AND** the tests that verify this are `TestStoredMessageRoundTripsThroughDecoder` and `TestStoredMessageValidate`
