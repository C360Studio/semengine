# security-config Specification

## Purpose

`pkg/security` holds the TLS settings a deployment gives SemEngine: server TLS, client TLS, mutual TLS (both sides
present a certificate) and ACME (automatic certificates). This capability states what a product building on SemEngine
can rely on: how a component receives those settings, what the metrics server does with them today, and which
settings are refused because no SemEngine code applies them yet.

## Requirements

### Requirement: A product receives the security configuration through component.Dependencies

`pkg/security` SHALL offer products seven configuration types, `Config`, `TLSConfig`, `ServerTLSConfig`,
`ClientTLSConfig`, `ServerMTLSConfig`, `ClientMTLSConfig` and `ACMEConfig`, with the JSON keys of the document that
`TestConfigDecodesEveryKey` decodes (`pkg/security/config_test.go`). A component factory SHALL receive them, unchanged,
as the field `Security` of the `component.Dependencies` it is called with. No SemEngine code fills that field yet:
it holds what the code that builds the `Dependencies` set, and is the zero value otherwise. The zero value SHALL leave
server TLS off, so a server built from it serves plain HTTP, and with server mTLS on SHALL require a client
certificate unless `client_cert_optional` is true. `client_cert_optional` admits a client with no certificate only
when `allowed_client_cns` is empty: a server with a CN allowlist SHALL refuse every client whose certificate does not
carry an allowed CN, including a client presenting none.

#### Scenario: Every setting arrives unchanged

- **WHEN** a product's `component.Factory` is called with a `component.Dependencies` whose `Security` was decoded from
  the document that sets every key
- **THEN** the `Security` the factory receives equals the value that document describes, field for field

#### Scenario: A product builds a server from the configuration it was given

- **WHEN** a product's `component.Factory` is called with a `component.Dependencies` whose `Security` was decoded from
  a document enabling server TLS and server mTLS, and passes it to `metric.NewServer`
- **THEN** the started server completes a TLS handshake only with a client presenting a certificate the configured
  client CA signed, and answers that client's request

#### Scenario: Client certificates made optional

- **WHEN** the same document also sets `client_cert_optional` to true, and sets no `allowed_client_cns`
- **THEN** a client presenting no certificate is served

#### Scenario: Optional client certificates under a CN allowlist

- **WHEN** the same document sets `client_cert_optional` to true and `allowed_client_cns` to a nonempty list
- **THEN** a client presenting no certificate is refused

### Requirement: The metrics server applies the server TLS block

`metric.NewServer` SHALL apply `TLS.Server` of the `security.Config` it is given. With `enabled` false it SHALL serve
plain HTTP, SHALL refuse at `Start` server mTLS enabled, and reads no other server setting. With `enabled` true it
SHALL serve TLS from `cert_file` and `key_file` at `min_version` or above, and `Start` SHALL refuse a `min_version`
other than empty, `1.2` or `1.3`.

#### Scenario: mTLS without TLS

- **WHEN** a server is started with server mTLS enabled and server TLS disabled
- **THEN** `Start` returns an error and serves nothing

#### Scenario: A misspelled TLS version

- **WHEN** a server is started with server TLS enabled and `min_version` `"1.4"`
- **THEN** `Start` returns an error naming `min_version`, and serves nothing

### Requirement: Settings no SemEngine code applies yet are refused

Until SemEngine ports its ACME loaders, the loaders that build a TLS configuration from these types SHALL refuse
(for the server loader, only when server TLS is enabled), with an invalid-class error naming the key, a `mode`
other than empty or `manual` and an `acme` block with `enabled` true, and `metric.NewServer`'s `Start` SHALL then
return an error naming the key and serve nothing. No exported SemEngine call applies `TLS.Client`; a product that
reads it builds its own TLS configuration.

#### Scenario: ACME mode on the metrics server

- **WHEN** a server is started with server TLS enabled and `mode` `"acme"`
- **THEN** `Start` returns an error naming `mode`, and serves nothing
