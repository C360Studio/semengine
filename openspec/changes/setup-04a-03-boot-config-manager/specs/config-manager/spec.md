# config-manager

## ADDED Requirements

### Requirement: Start bounds Start and Stop is the only stop

`config.Manager.Start(ctx)` SHALL do its startup work under `ctx` (acquiring the configuration bucket, establishing
the platform identity, version arbitration and opening its watches) and SHALL refuse a nil context, a context that
has already ended, and any Start after the first, before contacting NATS. Ending the context given to Start after
Start returned SHALL stop nothing. `Stop(ctx)` SHALL be the only stop: it SHALL return nil only after every watch is
stopped, every goroutine the Manager started has exited and every channel it returned is closed. If `ctx` ends
first it SHALL return that error, and a later Stop SHALL try again. A channel requested after Stop SHALL be returned
closed. The Manager SHALL pass the `lifecycle-suite` checks through a test-side adapter (ADR-113).

#### Scenario: The Start context ends after Start

- **WHEN** a started Manager's Start context is cancelled and another writer then puts `services.x`
- **THEN** the Manager still delivers the update, and a following Stop returns nil and leaves nothing running
- **AND** the test that verifies this is `TestManagerPassesTheLifecycleSuite`

#### Scenario: A model-registry watcher does not hold Stop

- **WHEN** `model.Watch` is running on a started Manager and Stop is called with a one-second context
- **THEN** Stop returns nil and the channel `WatchModelRegistry` returned is closed
- **AND** the test that verifies this is `TestStopWithAModelRegistryWatcherReturnsPromptly`

### Requirement: The configuration bucket is named by the declared authority pair

`config.BucketName(org, stem)` SHALL be the one derivation of a deployment's configuration bucket,
`semengine_config_<org>_<stem>`, from the declared `platform.org` and `platform.id`. It SHALL refuse an empty part, a
byte outside `[A-Za-z0-9_-]`, and a result longer than 252 bytes. `NewConfigManager` SHALL derive the name at
construction, so a minted identifier never names it, and Start SHALL acquire the bucket through
`kvcatalog.EnsureCatalogBucket` under its context. That refuses a policy that can evict keys and never repairs it.

#### Scenario: Different pairs use different buckets and the same pair shares one

- **WHEN** Managers declaring `acme`/`dep` and `acme`/`other` start on one server, then a second `acme`/`dep`
- **THEN** the first two use `semengine_config_acme_dep` and `semengine_config_acme_other`, and the third adopts the
  first's recorded identifier
- **AND** the tests that verify this are `TestBucketIsNamedByTheDeclaredPair`, `TestBucketNameIsLegalForEveryValidPair`
  and `FuzzBucketName`

### Requirement: Start establishes the platform identity before arbitration

Before arbitration, Start SHALL decide from one read of the bucket's keys (ADR-104):

- The `platform_identity` record is present: Start SHALL adopt its `id` as the effective `platform.id`. It SHALL
  refuse when the record's `org` differs from `platform.org` or its `stem` from the declared `platform.id`, with
  guidance when the declared value is the recorded minted identifier.
- The record is absent and the bucket is otherwise empty: Start SHALL refuse when a sibling
  `semengine_config_<org>_*` bucket records the declared value as its minted identifier. Otherwise it SHALL mint the
  six-hex-byte suffix and write the record with an atomic `Create`, adopting the winner's on conflict.
- The record is absent and the bucket holds other keys: Start SHALL refuse, minting and writing nothing.

The record SHALL carry exactly `org`, `stem` and `id`. Neither arbitration nor sync SHALL apply the stored `platform`
key or write the record (ADR-102 decision 7).

#### Scenario: First boot mints; later boots and co-processes adopt

- **WHEN** a Manager starts on an empty bucket, and later two processes with the same file start together
- **THEN** one record with `id` `<stem>-<six hex bytes>` exists, and every process adopts it
- **AND** the tests that verify this are `TestConfigManagerFirstBootMintsPlatformIdentity`,
  `TestConfigManagerAdoptsPersistedPlatformIdentity`, `TestConfigManagerConcurrentFirstBootConvergesOnOneIdentity`,
  `TestPreIdentityBucketRefusesStartWithoutMinting`, `TestAliasingPairsAreRefusedWithTheAliasMessage`,
  `TestFileDeclaringTheMintedIdentifierIsRefusedWithGuidance` and `TestKVPlatformKeyIsAMirrorNotASource`

### Requirement: A refused Start leaves no writer armed

Every Start that returns an error SHALL leave `PutComponentToKV`, `DeleteComponentFromKV` and every registered key
family's reads and writes returning the not-acquired error, and SHALL leave the bucket's contents unchanged.

#### Scenario: Foreign identity

- **WHEN** Start refuses a bucket whose record names another authority
- **THEN** each writer returns the not-acquired error and the bucket is unchanged key for key
- **AND** the test that verifies this is `TestRefusedStartDisarmsEveryExportedWriter`

### Requirement: Version arbitration selects one source for services and components

On a first boot Start SHALL push the file configuration to the bucket. Otherwise, when the file's `version` is newer
Start SHALL push the file. When it is equal, older, or cannot be compared, Start SHALL select the bucket: the
effective `services` and `components` SHALL then be exactly the bucket's `services.*` and `components.*` entries. A
missing stored `version` SHALL compare as `0.0.0`.

#### Scenario: A deleted file component stays deleted

- **WHEN** a file declares component `a`, `DeleteComponentFromKV("a")` succeeds, and a Manager with the same file
  and version starts
- **THEN** the effective configuration has no component `a`
- **AND** the tests that verify this are `TestDeletedFileComponentStaysDeletedAtNextBoot` and
  `TestKVSelectionReplacesServicesAndComponents`

### Requirement: Start fails closed on the bucket it cannot use

Start SHALL return an error naming the cause, hold nothing and arm no writer when:

- the first-boot push fails;
- listing or reading the bucket during arbitration fails;
- reading the stored `version` fails other than as not-found;
- a stored `services.*`, `components.*`, `nats` or `model_registry` entry does not decode;
- a stored `nats` value sets a key `config-loading` refuses at load, matched in any letter case.

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
returning. They SHALL NOT create, stop or restart a running component (ADR-094). They SHALL refuse a name that is not
one NATS KV literal token after that mapping, naming it. Concurrent writes and watcher applies SHALL each take effect
(serialized through `SafeConfig.Mutate`). A watcher event at or below the Manager's own last write revision SHALL NOT
be re-applied in memory but SHALL still notify subscribers.

#### Scenario: A dotted component name

- **WHEN** `PutComponentToKV` is called with `a.b`
- **THEN** it returns an error naming `a.b` and the bucket is unchanged
- **AND** the test that verifies this is `TestComponentNameThatIsNotOneKeyTokenIsRefused`

#### Scenario: Engine-owned revision still notifies

- **WHEN** the watcher delivers an event at or below the Manager's last write revision
- **THEN** memory is not re-applied and matching subscribers are notified
- **AND** the tests that verify this are `TestHandleUpdate_EngineOwnedRevisionStillNotifies` and
  `TestHandleUpdate_ExternalRevisionAppliesAndNotifies`

### Requirement: A registered key family is delivered to its owner

`NewConfigManager` SHALL accept `WithKeyFamily(family)` and SHALL refuse a nil family or a prefix the Manager owns.
Start SHALL open the family's watch `<prefix>.*` before publishing its handles, and SHALL fail when it cannot. After
Start the handler SHALL receive every entry present when the watch opened, marked initial, then each put and delete
with its member name, value and operation, until Stop returns. The family's `Get`, `Put`, `Create`, `Delete` and
`Names` SHALL be scoped to `<prefix>.<name>`. They SHALL refuse a nil context or a name that is not one KV literal
token before any bucket access, and SHALL return the not-acquired error until Start succeeds. `Names` SHALL omit a
key with more than one token after the prefix, and log it.

#### Scenario: Snapshot then changes

- **WHEN** a bucket holds `rules.a`, the Manager starts with a `rules` family, then `rules.b` is put and `rules.a`
  deleted
- **THEN** the handler receives `a` marked initial, then `b` as a put and `a` as a delete, and nothing outside
  `rules.*`
- **AND** the tests that verify this are `TestKeyFamilyDeliversSnapshotThenChanges`,
  `TestBoundKeyFamilyRefusesBeforeAnyStoreAccess` and `TestNewConfigManagerRefusesAFamilyOnTheManagersOwnKeys`
