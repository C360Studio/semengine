# config-loading

## ADDED Requirements

### Requirement: Document validation reads no file and is stable

`config.Config.ValidateDocument` SHALL run every check `Config.Validate` runs except the host checks, and SHALL read no
file and write nothing. The host checks are that a configured server certificate, server key and client CA file
exists, and the warning on standard error when `insecure_skip_verify` is set. `Config.Validate` SHALL run
`ValidateDocument` and then the host checks. Both SHALL visit component entries in instance-name order, and the model
registry's endpoints and capabilities in name order, so the first error returned for a given document is the same on
every run.

#### Scenario: A missing TLS file

- **WHEN** a configuration enables server TLS with a certificate path that does not exist
- **THEN** `ValidateDocument` returns nil and `Validate` refuses it naming `tls.server.cert_file`
  (`TestValidateDocumentReadsNoFile`, `TestConfigValidateRefusesMissingTLSFiles`)

#### Scenario: Several invalid components

- **WHEN** a configuration has invalid component entries `bad3`, `bad1` and `bad2`
- **THEN** every call returns the error naming `bad1` (`TestConfigValidateNamesFirstInvalidComponentByName`)
- **AND** a model registry with several invalid endpoints is refused naming the first endpoint in name order
  (`TestRegistryValidateNamesFirstInvalidEndpointByName`)
