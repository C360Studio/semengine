# config-manager Specification

## Purpose

`config.Manager` keeps a deployment's configuration in a NATS KV bucket named by the deployment's authority pair. At
`Start` it establishes the platform identity, chooses between the file's configuration and the bucket's copy by
version, and watches the bucket for changes it delivers to subscribers and registered key families. It is a
lifecycle owner under ADR-113: `Start(ctx)` bounds only the start-up work, and `Stop(ctx)` is the only stop.

## Requirements

### Requirement: Start bounds Start and Stop is the only stop

`config.Manager.Start(ctx)` SHALL do its startup work under `ctx` (acquiring the configuration bucket, establishing the
platform identity, version arbitration and opening its watches) and SHALL refuse a nil context, a context that has
already ended, and any Start after the first, before contacting NATS. Ending the context given to Start after Start
returned SHALL stop nothing.

#### Scenario: The Start context ends after Start

- **WHEN** a started Manager's Start context is cancelled and another writer then puts `services.x`
- **THEN** the Manager still delivers the update, and a following Stop returns nil and leaves nothing running
- **AND** the tests that verify this are `TestStartContextEndingKeepsDelivering` (delivery) and
  `TestManagerPassesTheLifecycleSuite` (held and released handles)

### Requirement: Stop is the only stop

`Stop(ctx)` SHALL be the only stop: it SHALL return nil only after every watch is stopped, every goroutine the Manager
started has exited and every channel it returned is closed. A channel requested after Stop SHALL be returned closed. The
Manager SHALL pass the `lifecycle-suite` checks through a test-side adapter (ADR-113).

#### Scenario: A model-registry watcher does not hold Stop

- **WHEN** `model.Watch` is running on a started Manager and Stop is called with a one-second context
- **THEN** Stop returns nil and the channel `WatchModelRegistry` returned is closed
- **AND** the test that verifies this is `TestStopWithAModelRegistryWatcherReturnsPromptly`

### Requirement: Stop honours its context

If the context given to `Stop` ends first, `Stop` SHALL return that error, including while a watch's teardown is still
waiting on the server, and a later Stop SHALL wait for that teardown again.

#### Scenario: A watch teardown the server does not answer

- **WHEN** the Manager's credentials may open its watches but not delete their consumers, and Stop is called with a
  100 ms context
- **THEN** Stop returns `context.DeadlineExceeded` within a second, and a later Stop with a longer context returns nil
- **AND** the test that verifies this is `TestStopHonoursItsDeadlineWhenAConsumerDeleteIsDenied`

### Requirement: The configuration bucket is named by the declared authority pair

`config.BucketName(org, stem)` SHALL be the one derivation of a deployment's configuration bucket,
`semengine_config_<org>_<stem>`, from the declared `platform.org` and `platform.id`. It SHALL refuse an empty part, a
byte outside `[A-Za-z0-9_-]`, and a result longer than 252 bytes.

#### Scenario: Different pairs use different buckets and the same pair shares one

- **WHEN** Managers declaring `acme`/`dep` and `acme`/`other` start on one server, then a second `acme`/`dep`
- **THEN** the first two use `semengine_config_acme_dep` and `semengine_config_acme_other`, and the third adopts the
  first's recorded identifier
- **AND** the tests that verify this are `TestBucketIsNamedByTheDeclaredPair`, `TestBucketNameIsLegalForEveryValidPair`
  and `FuzzBucketName`

### Requirement: Construction names the bucket and Start acquires it

`NewConfigManager` SHALL derive the name at construction, so a minted identifier never names it, and Start SHALL acquire
the bucket through `kvcatalog.EnsureCatalogBucket` under its context. That call refuses a policy that can evict keys and
never repairs it.

#### Scenario: A bucket whose policy can evict keys

- **WHEN** the configuration bucket exists with a policy that can evict keys
- **THEN** Start refuses it and does not repair the policy

### Requirement: Start establishes the platform identity before arbitration

Before arbitration, Start SHALL decide from one read of the bucket's keys (ADR-104), as the three requirements that
follow state. The record SHALL carry exactly `org`, `stem` and `id`. Neither arbitration nor sync SHALL apply the stored
`platform` key or write the record (ADR-102 decision 7).

#### Scenario: First boot mints; later boots and co-processes adopt

- **WHEN** a Manager starts on an empty bucket, and later two processes with the same file start together
- **THEN** one record with `id` `<stem>-<six hex bytes>` exists, and every process adopts it
- **AND** the tests that verify this are `TestConfigManagerFirstBootMintsPlatformIdentity`,
  `TestConfigManagerAdoptsPersistedPlatformIdentity`, `TestConfigManagerConcurrentFirstBootConvergesOnOneIdentity`,
  `TestPreIdentityBucketRefusesStartWithoutMinting`, `TestAliasingPairsAreRefusedWithTheAliasMessage`,
  `TestFileDeclaringTheMintedIdentifierIsRefusedWithGuidance` and `TestKVPlatformKeyIsAMirrorNotASource`

### Requirement: Start adopts a recorded platform identity

When the `platform_identity` record is present, Start SHALL adopt its `id` as the effective `platform.id`. It SHALL
refuse when the record's `org` differs from `platform.org` or its `stem` from the declared `platform.id`, with guidance
when the declared value is the recorded minted identifier.

#### Scenario: Record of another authority

- **WHEN** the `platform_identity` record names another `org` or `stem` than the declared pair
- **THEN** Start refuses; when the declared `platform.id` is the recorded minted identifier, the error carries guidance

### Requirement: Start mints the platform identity on an empty bucket

When the record is absent and the bucket is otherwise empty, Start SHALL refuse if a sibling `semengine_config_<org>_*`
bucket records the declared value as its minted identifier. Otherwise it SHALL mint the six-hex-byte suffix and write
the record with an atomic `Create`, adopting the winner's on conflict.

#### Scenario: Sibling bucket records the declared value

- **WHEN** the record is absent, the bucket is otherwise empty, and a sibling `semengine_config_<org>_*` bucket records
  the declared value as its minted identifier
- **THEN** Start refuses

### Requirement: Start refuses a bucket that has keys but no identity record

When the record is absent and the bucket holds other keys, Start SHALL refuse, minting and writing nothing.

#### Scenario: Bucket of other keys without a record

- **WHEN** the record is absent and the bucket holds other keys
- **THEN** Start refuses and the bucket is unchanged

### Requirement: A refused Start leaves no writer armed

Every Start that returns an error SHALL leave `PutComponentToKV`, `DeleteComponentFromKV` and every registered key
family's reads and writes returning the not-acquired error. A Start refused before its first write (identity,
retention policy, a refused service or component name, a value that cannot be encoded, a stored entry that does not
decode) SHALL leave the bucket's contents
unchanged.

#### Scenario: Foreign identity

- **WHEN** Start refuses a bucket whose record names another authority
- **THEN** each writer returns the not-acquired error and the bucket is unchanged key for key
- **AND** the test that verifies this is `TestRefusedStartDisarmsEveryExportedWriter`

### Requirement: Version arbitration selects one source for services and components

On a first boot Start SHALL push the file configuration to the bucket. Otherwise, when the file's `version` is newer
Start SHALL push the file. When it is equal, older, or cannot be compared, Start SHALL select the bucket: the effective
`services` and `components` SHALL then be exactly the bucket's `services.*` and `components.*` entries.

#### Scenario: Leftovers of an interrupted push do not select a partial set

- **WHEN** the bucket holds the identity record and some `services.*`/`components.*` keys with no `version`, or with
  a `version` older than the file's, and a Manager starts with a file whose `version` is newer than the stored one (a
  missing stored `version` compares as `0.0.0`; a file with no `version`, or `0.0.0`, is not newer and selects the
  partial bucket)
- **THEN** the effective services and components are the file's full set, and the bucket holds the full push with the
  file's `version`
- **AND** the tests that verify this are `TestRecoveryFromAnInterruptedPush` and `TestPushWritesVersionLast`

#### Scenario: A deleted file component stays deleted

- **WHEN** a file declares component `a`, `DeleteComponentFromKV("a")` succeeds, and a Manager with the same file
  and version starts
- **THEN** the effective configuration has no component `a`
- **AND** the tests that verify this are `TestDeletedFileComponentStaysDeletedAtNextBoot` and
  `TestKVSelectionReplacesServicesAndComponents`

### Requirement: Versions compare as semantic versions

A missing stored `version`, and a stored `version` that fails to decode into a string (with a Warn), SHALL compare as
`0.0.0`; a stored string (including the empty string a stored `null` decodes to) or a file `version` that is not a
semantic version (a file with no `version` among them) cannot be compared, and Start SHALL select the bucket.

#### Scenario: File with no version

- **WHEN** the file has no `version` and the bucket holds `services.*` entries
- **THEN** the version cannot be compared and Start selects the bucket

### Requirement: Start refuses names that are not one key

Before acquiring the bucket, Start SHALL refuse a file configuration holding a service or component name that is not one
NATS KV literal token after the space mapping, or two names that map to one key, naming them. Start SHALL then name each
service and component as its key does, each space written as `_`.

#### Scenario: A name the bucket cannot hold as one key

- **WHEN** the file declares a service or component named `a.b`, or two named `a b` and `a_b`
- **THEN** Start fails naming them, and the bucket is unchanged
- **AND** the test that verifies this is `TestNameThatIsNotOneKeyTokenIsRefused`

### Requirement: A push writes the version last

A push SHALL encode every value before its first write and SHALL write `version` after every other key.

#### Scenario: Version is the last key written

- **WHEN** Start pushes the file configuration
- **THEN** every value is encoded before the first write and `version` is written after every other key;
  `TestPushWritesVersionLast` verifies this

### Requirement: Start fails closed on the bucket it cannot use

Start SHALL return an error naming the cause, roll back what it acquired and arm no writer when any of the following
holds; a rollback that fails SHALL be joined into Start's error and SHALL leave its cleanup for the next Stop:

- the first-boot push fails, or the push when the file's version is newer fails, including a value that cannot be
  encoded (`TestPushEncodingFailureFailsStart`);
- listing or reading the bucket during arbitration fails.

#### Scenario: A value that cannot be encoded

- **WHEN** the first-boot push or the push of a newer file contains a value that cannot be encoded
- **THEN** Start returns an error naming the cause, rolls back what it acquired and arms no writer;
  `TestPushEncodingFailureFailsStart` verifies this

### Requirement: Start fails closed on a stored value it cannot use

Start SHALL fail in the way the requirement "Start fails closed on the bucket it cannot use" states when reading the
stored `version` fails other than as not-found, when a stored `services.*`, `components.*`, `nats` or `model_registry`
entry does not decode, or when a stored `nats` value sets a key `config-loading` refuses at load, matched in any letter
case.

#### Scenario: A malformed stored service

- **WHEN** the selected bucket holds `services.metrics` with an unknown field
- **THEN** Start returns an error naming `services.metrics`
- **AND** the test that verifies this is `TestMalformedStoredServiceFailsStart`

#### Scenario: A stored NATS token

- **WHEN** the selected bucket's `nats` value sets `"token"`
- **THEN** Start returns an error naming `nats.token`
- **AND** the test that verifies this is `TestStoredNATSTokenFailsStart`

### Requirement: Component writes are next-boot desired state

`PutComponentToKV(ctx, name, cfg)` and `DeleteComponentFromKV(ctx, name)` SHALL write or delete the key
`components.<name>`, with each space in `name` written as `_`, and SHALL apply the change to `GetConfig()` before
returning. They SHALL NOT create, stop or restart a running component (ADR-094).

#### Scenario: Deleting a file component named with a space

- **WHEN** the file declares component `a b` and `DeleteComponentFromKV("a b")` succeeds
- **THEN** `components.a_b` is gone from the bucket and `GetConfig()` holds neither `a b` nor `a_b`
- **AND** the test that verifies this is `TestDeletingAFileComponentWithASpaceUpdatesMemory`

### Requirement: Component writes refuse a name that is not one key

`PutComponentToKV` and `DeleteComponentFromKV` SHALL refuse a name that is not one NATS KV literal token after the space
mapping, naming it.

#### Scenario: A dotted component name

- **WHEN** `PutComponentToKV` is called with `a.b`
- **THEN** it returns an error naming `a.b` and the bucket is unchanged
- **AND** the test that verifies this is `TestNameThatIsNotOneKeyTokenIsRefused`

### Requirement: A delete excludes watcher applies until it has removed the entry

No watcher apply SHALL run between a delete's bucket write and its in-memory removal, so a later put of the key is
applied after the removal and the delete's own, older event is not re-applied over it.

#### Scenario: A delete does not erase a later put of its key

- **WHEN** `DeleteComponentFromKV("local")` has written its delete to the bucket, an external writer then puts
  `components.local`, and the watcher delivers that put before the delete has updated `GetConfig()`
- **THEN** the put's apply waits for the delete's in-memory removal, the delete's own event is not re-applied after it,
  and `GetConfig()` holds the external value
- **AND** the test that verifies this is `TestDeleteExcludesAWatcherApplyMidTransition`

### Requirement: Writes and watcher applies are ordered by revision

A put whose key the watcher has already applied at a newer revision SHALL keep that newer value. Concurrent writes and
watcher applies SHALL each take effect (serialized through `SafeConfig.Mutate`). A watcher event at or below the
revision already applied for its key SHALL NOT be re-applied in memory but SHALL still notify subscribers; a write to
one key SHALL NOT suppress another key's event.

#### Scenario: A put keeps a newer revision of its key

- **WHEN** the watcher has already applied an external write of `components.local` at a revision above the one
  `PutComponentToKV` gets for its own write of that key
- **THEN** `GetConfig()` keeps the external value
- **AND** the test that verifies this is `TestPutKeepsANewerRevisionTheWatcherApplied`

#### Scenario: Engine-owned revision still notifies

- **WHEN** the watcher delivers an event at or below the revision already applied for its key
- **THEN** memory is not re-applied and matching subscribers are notified
- **AND** the tests that verify this are `TestHandleUpdate_EngineOwnedRevisionStillNotifies` and
  `TestHandleUpdate_ExternalRevisionAppliesAndNotifies`

#### Scenario: A local write to another key

- **WHEN** an external writer puts `services.external` and, before its event is processed, `PutComponentToKV` writes
  another key at a later revision
- **THEN** the external event is applied in memory
- **AND** the test that verifies this is `TestOtherKeyWriteDoesNotSuppressAnExternalUpdate`

### Requirement: A registered key family is delivered to its owner

`NewConfigManager` SHALL accept `WithKeyFamily(family)` and SHALL refuse a nil family or a prefix the Manager owns.
Start SHALL open the family's watch `<prefix>.*` before publishing its handles, and SHALL fail when it cannot.

#### Scenario: Nil family or a prefix the Manager owns

- **WHEN** `NewConfigManager` is given `WithKeyFamily` with a nil family, or with a prefix the Manager owns
- **THEN** it refuses; `TestNewConfigManagerRefusesAFamilyOnTheManagersOwnKeys` verifies this

### Requirement: A key family handler receives its snapshot and then its changes

After Start the handler SHALL receive every entry present when the watch opened, marked initial, then each put and
delete with its member name, value and operation, until Stop returns.

#### Scenario: Snapshot then changes

- **WHEN** a bucket holds `rules.a`, the Manager starts with a `rules` family, then `rules.b` is put and `rules.a`
  deleted
- **THEN** the handler receives `a` marked initial, then `b` as a put and `a` as a delete, and nothing outside
  `rules.*`
- **AND** the tests that verify this are `TestKeyFamilyDeliversSnapshotThenChanges`,
  `TestBoundKeyFamilyRefusesBeforeAnyStoreAccess` and `TestNewConfigManagerRefusesAFamilyOnTheManagersOwnKeys`

### Requirement: A key family's access is scoped to its prefix

The family's `Get`, `Put`, `Create`, `Delete` and `Names` SHALL be scoped to `<prefix>.<name>`. They SHALL refuse a nil
context or a name that is not one KV literal token before any bucket access, and SHALL return the not-acquired error
until Start succeeds. `Names` SHALL omit a key with more than one token after the prefix, and log it.

#### Scenario: A nested key

- **WHEN** the bucket holds a key with more than one token after a family's prefix and the family calls `Names`
- **THEN** `Names` omits that key and logs it

### Requirement: A key family's values are left to its handler

A family's stored values SHALL be left to its handler: Start SHALL NOT decode them as Manager configuration.

#### Scenario: A family value that is not JSON

- **WHEN** a `rules` family puts `rules.a` as the bytes `{`, and the deployment restarts with the same family
- **THEN** Start succeeds
- **AND** the test that verifies this is `TestOpaqueKeyFamilyValueSurvivesRestart`
