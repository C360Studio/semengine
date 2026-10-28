# object-store

## ADDED Requirements

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
