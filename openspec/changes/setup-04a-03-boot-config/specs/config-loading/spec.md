# Delta: config-loading

## ADDED Requirements

### Requirement: Loading applies one document over the defaults and does not validate

`config.NewLoader().LoadFile(path)` and `LoadFromBytes(data)` SHALL start from the loader's defaults, deep-merge
one JSON document over them, apply the environment overrides, and refuse an authority pair over its declared budget
or a `platform.instance_id` key (ADR-102, ADR-104). They SHALL NOT run `Config.Validate`; a caller that needs a
validated configuration SHALL call `Validate` on the result. `LoadFile` SHALL refuse a path outside the working
directory when the path is relative, a file that is not regular after following links, a file over 10 MB, and a
document nested deeper than 100 levels.

#### Scenario: A document that omits nats keeps the default URL

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"}}`
- **THEN** it returns a `Config` whose `NATS.URLs` are the loader's default and no error

### Requirement: A malformed document is refused

When a document is valid JSON but one of its values does not fit the field it names, `LoadFile` and `LoadFromBytes`
SHALL return an error naming the decode failure, and SHALL NOT return the defaults in its place.

#### Scenario: A URL list written as a string

- **WHEN** `LoadFile` reads `{"platform":{"org":"acme","id":"p1"},"nats":{"urls":"nats://a.invalid:14222"}}`
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
(comma-separated) when set and not empty, and SHALL read no other environment variable.

#### Scenario: URLs from the environment

- **WHEN** `SEMENGINE_NATS_URLS` is `nats://a.invalid:14222,nats://b.invalid:14222` and the document sets other URLs
- **THEN** the loaded `NATS.URLs` are the two URLs from the environment
