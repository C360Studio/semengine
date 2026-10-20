# config-loading delta

## ADDED Requirements

### Requirement: A key in another letter case is refused

`LoadFile` and `LoadFromBytes` SHALL refuse a document holding a key that differs only in letter case from the name
of the struct field it is decoded into, at any depth. The error SHALL name the key's dotted path as written and as
declared. They SHALL NOT check map keys a deployment chooses (component, service, stream, override, archival and model
registry names) or the contents of a component's or service's `config` payload. A key that matches no declared name
in any letter case SHALL stay accepted, except where a strictly decoded entry refuses it. When the document also sets
a removed or unapplied key, the refusal naming that key SHALL be returned.

#### Scenario: NATS written in capitals

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"},"NATS":{"urls":["nats://a.invalid:14222"]}}`
- **THEN** it returns an error naming `NATS` and `nats` and no `Config`

#### Scenario: A nested key in another letter case

- **WHEN** `LoadFromBytes` reads `{"platform":{"org":"acme","id":"p1"},"nats":{"URLs":["nats://a.invalid:14222"]}}`
- **THEN** it returns an error naming `nats.URLs` and `nats.urls` and no `Config`

#### Scenario: Both spellings in one document

- **WHEN** `LoadFromBytes` reads a document that sets both `nats` and `NATS`
- **THEN** it returns an error naming `NATS` and `nats` and no `Config`

#### Scenario: Names a deployment chooses are not checked

- **WHEN** `LoadFromBytes` reads a document with a component named `Config` whose `config` payload holds `"Port"`
- **THEN** it returns a `Config` holding that component, with the payload unchanged, and no error

#### Scenario: An unknown key is still accepted

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"},"tier":"edge"}`
- **THEN** it returns a `Config` and no error
