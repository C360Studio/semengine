# object-store

## ADDED Requirements

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

`Store.StoreContent` SHALL refuse, before writing, an invalid entity ID or any non-empty content other than the
`body` role's text, binary fields included. Otherwise it SHALL write the body as raw bytes (empty when there is
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

#### Scenario: A payload with a binary field

- **WHEN** `StoreContent` is called with a payload that implements `message.BinaryStorable` with one binary field
- **THEN** it returns an invalid-input error and the bucket holds no new object
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
