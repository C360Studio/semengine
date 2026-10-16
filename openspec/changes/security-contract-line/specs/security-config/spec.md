# security-config

## ADDED Requirements

### Requirement: A product receives the security configuration through component.Dependencies

`pkg/security` SHALL offer products seven configuration types, `Config`, `TLSConfig`, `ServerTLSConfig`,
`ClientTLSConfig`, `ServerMTLSConfig`, `ClientMTLSConfig` and `ACMEConfig`, with the JSON keys listed on the
`pkg/security` row of `docs/admission-ledger.yaml`. A component factory SHALL receive them as the field `Security` of
the `component.Dependencies` it is called with. The zero value SHALL leave server TLS off, and with server mTLS on
SHALL require a client certificate unless `client_cert_optional` is true.

#### Scenario: A product builds a server from the configuration it was given

- **WHEN** a product's `component.Factory` is called with a `component.Dependencies` whose `Security` was decoded from
  a document enabling server TLS and server mTLS, and passes it to `metric.NewServer`
- **THEN** the started server completes a TLS handshake only with a client presenting a certificate the configured
  client CA signed, and answers that client's request

#### Scenario: Client certificates made optional

- **WHEN** the same document also sets `client_cert_optional` to true
- **THEN** a client presenting no certificate is served

### Requirement: The metrics server applies the server TLS block

`metric.NewServer` SHALL apply `TLS.Server` of the `security.Config` it is given: with `enabled` false it SHALL serve
plain HTTP; with `enabled` true it SHALL serve TLS from `cert_file` and `key_file` at `min_version` or above.
`Start` SHALL refuse a `min_version` other than empty, `1.2` or `1.3`, and SHALL refuse server mTLS enabled while
server TLS is not.

#### Scenario: mTLS without TLS

- **WHEN** a server is started with server mTLS enabled and server TLS disabled
- **THEN** `Start` returns an error and serves nothing

#### Scenario: A misspelled TLS version

- **WHEN** a server is started with server TLS enabled and `min_version` `"1.4"`
- **THEN** `Start` returns an error naming `min_version`, and serves nothing

### Requirement: Settings no SemEngine code applies yet are refused

Until SemEngine ports its ACME loaders, the loaders that build a TLS configuration from these types SHALL refuse,
with an invalid-class error naming the key, a `mode` other than empty or `manual` and an `acme` block with `enabled`
true, and `metric.NewServer`'s `Start` SHALL then return an error naming the key and serve nothing. No exported
SemEngine call applies `TLS.Client`; a product that reads it builds its own TLS configuration.

#### Scenario: ACME mode on the metrics server

- **WHEN** a server is started with server TLS enabled and `mode` `"acme"`
- **THEN** `Start` returns an error naming `mode`, and serves nothing
