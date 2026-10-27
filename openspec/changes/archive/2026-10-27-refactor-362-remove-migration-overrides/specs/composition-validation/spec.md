# composition-validation delta: refactor-362-remove-migration-overrides

## RENAMED Requirements

- FROM: `### Requirement: Validation reads no host files and one clock`
- TO: `### Requirement: Validation reads no host files and no clock`

## MODIFIED Requirements

### Requirement: Validation reads no host files and no clock

`composition.Validate` SHALL judge the configuration document with `config.Config.ValidateDocument`, so its result does
not depend on files on the host and it writes nothing to standard error. It reads nothing from outside its
arguments.

#### Scenario: Files on the host do not change the result

- **WHEN** a configuration names a client CA file, and validation runs with the file absent and again with it present,
  and once more with `insecure_skip_verify` set
- **THEN** the first two results marshal to byte-equal JSON and nothing is written to standard error
  (`TestValidateReadsNoTLSFile`)
