# config-loading Specification

## Purpose

`config` loads a deployment's configuration: `config.NewLoader().LoadFile(path)` and `LoadFromBytes(data)` merge one
JSON document over the loader's defaults and apply the `SEMENGINE_` environment overrides. Loading refuses what
SemEngine does not apply or cannot decode, rather than dropping it silently. Validation is a separate call
(`Config.Validate`).

## Requirements

### Requirement: Loading applies one document over the defaults and does not validate

`config.NewLoader().LoadFile(path)` and `LoadFromBytes(data)` SHALL start from the loader's defaults, deep-merge
one JSON document over them, apply the environment overrides, and refuse an authority pair over its declared budget
or a `platform.instance_id` key (ADR-102, ADR-104). They SHALL NOT run `Config.Validate`; a caller that needs a
validated configuration SHALL call `Validate` on the result. Both loaders SHALL refuse a document over 10 MB or
nested deeper than 100 levels. `LoadFile` SHALL also refuse a path outside the working directory when the path is
relative, and a file that is not regular after following links.

#### Scenario: A document that omits nats keeps the default URL

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"}}`
- **THEN** it returns a `Config` whose `NATS.URLs` are the loader's default and no error

### Requirement: A malformed document is refused

When a document is valid JSON but one of its values does not fit the field it names, or a strictly decoded entry
(`services.<name>`) carries a field it refuses, `LoadFile` and `LoadFromBytes` SHALL return an error naming the decode
failure, and SHALL NOT return the defaults in its place.

#### Scenario: A URL list written as a string

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"},"nats":{"urls":"nats://a.invalid:14222"}}`
- **THEN** it returns an error and no `Config`

#### Scenario: A service entry with an unknown field

- **WHEN** `LoadFromBytes` reads `{"platform":{"org":"acme","id":"p1"},"services":{"metrics":{"name":"m","enabled":true}}}`
- **THEN** it returns an error and no `Config`

### Requirement: NATS settings nothing applies are refused at load

Until SemEngine's boot applies them, `LoadFile` and `LoadFromBytes` SHALL refuse a document that sets any of
`nats.username`, `nats.password`, `nats.token`, `nats.tls`, `nats.max_reconnects`, `nats.reconnect_wait`,
`nats.jetstream.domain`, `nats.jetstream.retention_policy` or `nats.jetstream.replication_factor`, with an error that
names the key and says it is not applied. `nats.urls`, `nats.jetstream.enabled`, `nats.jetstream.max_memory` and
`nats.jetstream.max_file_store` SHALL load.

#### Scenario: A token in the file

- **WHEN** `LoadFromBytes` reads a document whose `nats` block sets `"token": "x"`
- **THEN** it returns an error naming `nats.token`

### Requirement: Environment overrides use the SEMENGINE_ prefix

The loader SHALL override `platform.id`, `platform.type`, `platform.region` and `nats.urls` from
`SEMENGINE_PLATFORM_ID`, `SEMENGINE_PLATFORM_TYPE`, `SEMENGINE_PLATFORM_REGION` and `SEMENGINE_NATS_URLS`
(comma-separated) when set and not empty. Until boot applies NATS credentials, the loader SHALL refuse a load when
`SEMENGINE_NATS_USERNAME`, `SEMENGINE_NATS_PASSWORD` or `SEMENGINE_NATS_TOKEN` is set and not empty, with an error
naming the variable.

#### Scenario: URLs from the environment

- **WHEN** `SEMENGINE_NATS_URLS` is `nats://a.invalid:14222,nats://b.invalid:14222` and the document sets other URLs
- **THEN** the loaded `NATS.URLs` are the two URLs from the environment

#### Scenario: A password in the environment

- **WHEN** `SEMENGINE_NATS_PASSWORD` is `x` and `LoadFile` reads a valid document
- **THEN** it returns an error naming `SEMENGINE_NATS_PASSWORD`

### Requirement: A key in another letter case is refused

`LoadFile` and `LoadFromBytes` SHALL refuse a document holding a key that differs only in letter case from the name
of the struct field it is decoded into, at any depth; a key exactly equal to a declared name is accepted. The error
SHALL name the key's dotted path as written and as declared. They SHALL NOT check map keys a deployment chooses
(component, service, stream, override, archival and model registry names) or the contents of a component's or
service's `config` payload. A key that matches no declared name in any letter case SHALL stay accepted, except where a
strictly decoded entry refuses it. When the document also sets a removed or unapplied key, the refusal naming that key
SHALL be returned.

#### Scenario: NATS written in capitals

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"},"NATS":{"urls":["nats://a.invalid:14222"]}}`
- **THEN** it returns an error naming `NATS` and `nats` and no `Config`

#### Scenario: A nested key in another letter case

- **WHEN** `LoadFromBytes` reads `{"platform":{"org":"acme","id":"p1"},"nats":{"URLs":["nats://a.invalid:14222"]}}`
- **THEN** it returns an error naming `nats.URLs` and `nats.urls` and no `Config`

#### Scenario: A case variant inside a service entry

- **WHEN** `LoadFromBytes` reads a document whose `services.heartbeat` entry sets `"Enabled": false`
- **THEN** it returns an error naming `services.heartbeat.Enabled` and `services.heartbeat.enabled` and no `Config`

#### Scenario: Both spellings in one document

- **WHEN** `LoadFromBytes` reads a document that sets both `nats` and `NATS`
- **THEN** it returns an error naming `NATS` and `nats` and no `Config`

#### Scenario: Names a deployment chooses are not checked

- **WHEN** `LoadFromBytes` reads a document with a component named `Config` whose `config` payload holds `"Port"`
- **THEN** it returns a `Config` holding that component, with the payload unchanged, and no error

#### Scenario: An unknown key is still accepted

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"},"tier":"edge"}`
- **THEN** it returns a `Config` and no error
