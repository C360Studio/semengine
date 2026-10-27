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
validated configuration SHALL call `Validate` on the result.

#### Scenario: A document that omits nats keeps the default URL

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"}}`
- **THEN** it returns a `Config` whose `NATS.URLs` are the loader's default and no error

### Requirement: Loading refuses oversized documents and unsafe paths

Both loaders SHALL refuse a document over 10 MB or nested deeper than 100 levels. `LoadFile` SHALL also refuse a path
outside the working directory when the path is relative, and a file that is not regular after following links.

#### Scenario: A document over the size or depth bound

- **WHEN** `LoadFromBytes` reads a document over 10 MB, or one nested deeper than 100 levels
- **THEN** it returns an error and no `Config`

#### Scenario: A path the loader will not follow

- **WHEN** `LoadFile` is given a relative path outside the working directory, or a path to a file that is not regular
  after following links
- **THEN** it returns an error and no `Config`

### Requirement: The default services are the heartbeat alone

`LoadFile` and `LoadFromBytes` SHALL default `Services` to one entry, `heartbeat`, enabled, and SHALL default no other
service, so a loaded document holds the services it names and the heartbeat.

#### Scenario: A document that names no services

- **WHEN** `LoadFile` or `LoadFromBytes` reads `{"platform":{"org":"acme","id":"p1","type":"vessel"}}`
- **THEN** the returned `Config.Services` has exactly one key, `heartbeat`, and that entry is enabled

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
SHALL name the key's dotted path as written and as declared.

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

### Requirement: Names a deployment chooses are not checked for letter case

`LoadFile` and `LoadFromBytes` SHALL NOT check map keys a deployment chooses (component, service, stream, archival
and model registry names) or the contents of a component's or service's `config` payload for letter case.

#### Scenario: Names a deployment chooses are not checked

- **WHEN** `LoadFromBytes` reads a document with a component named `Config` whose `config` payload holds `"Port"`
- **THEN** it returns a `Config` holding that component, with the payload unchanged, and no error

### Requirement: An unknown key stays accepted

A key that matches no declared name in any letter case SHALL stay accepted, except where a strictly decoded entry
refuses it.

#### Scenario: An unknown key is still accepted

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"},"tier":"edge"}`
- **THEN** it returns a `Config` and no error

### Requirement: A removed or unapplied key is named before a case variant

When the document also sets a removed or unapplied key, the refusal naming that key SHALL be returned.

#### Scenario: A case variant beside an unapplied key

- **WHEN** `LoadFromBytes` reads a document that sets `NATS` in capitals and also sets `nats.token`
- **THEN** it returns the error naming `nats.token`

### Requirement: Document validation reads no file and is stable

`config.Config.ValidateDocument` SHALL run every check `Config.Validate` runs except the host checks, and SHALL read no
file and write nothing. The host checks are that a configured server certificate, server key and client CA file
exists, and the warning on standard error when `insecure_skip_verify` is set.

#### Scenario: A missing TLS file

- **WHEN** a configuration enables server TLS with a certificate path that does not exist
- **THEN** `ValidateDocument` returns nil and `Validate` refuses it naming `tls.server.cert_file`
  (`TestValidateDocumentReadsNoFile`, `TestConfigValidateRefusesMissingTLSFiles`)

### Requirement: Validate runs the document checks and then the host checks

`Config.Validate` SHALL run `ValidateDocument` and then the host checks.

#### Scenario: A document error is also a Validate error

- **WHEN** a configuration has an invalid component entry
- **THEN** `ValidateDocument` and `Validate` both return an error

### Requirement: Validation visits entries in name order

Both `ValidateDocument` and `Validate` SHALL visit component entries in instance-name order, and the model registry's
endpoints and capabilities in name order, so the first error returned for a given document is the same on every run.

#### Scenario: Several invalid components

- **WHEN** a configuration has invalid component entries `bad3`, `bad1` and `bad2`
- **THEN** every call returns the error naming `bad1` (`TestConfigValidateNamesFirstInvalidComponentByName`)
- **AND** a model registry with several invalid endpoints is refused naming the first endpoint in name order
  (`TestRegistryValidateNamesFirstInvalidEndpointByName`)
