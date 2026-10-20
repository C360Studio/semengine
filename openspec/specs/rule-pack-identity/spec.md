# rule-pack-identity Specification

## Purpose

`pkg/rulepack` holds the identity contract of a composed rule processor: the grammar and byte bound of its `pack_id`,
checked by `ValidateID` and published as `PackIDCharset` and `MaxPackIDBytes`. The rule processor's configuration
and the service manager's rule-pack binding read it (ported later in change 3 and the rule core); duplicate pack
IDs and the static identity across hot reload are their rules, not this package's (pin ADR-076 decision 4).

## Requirements

### Requirement: A rule-pack ID is one bounded literal token

`rulepack.ValidateID` SHALL accept a rule-pack ID of 1 to 246 bytes in which every byte matches `[A-Za-z0-9_=-]`, and
SHALL refuse every other value without trimming, folding or otherwise normalizing it. The checks run in order:
required, then length, then token, then alphabet. A nonempty value within the bound that is not one literal NATS KV
token SHALL be refused with natsclient's classified `kv_token_invalid` error wrapped; an empty or overlength value is
refused by the required or length check first, without that code. `PackIDCharset` and `MaxPackIDBytes` SHALL state
the same alphabet and bound.

#### Scenario: Boundary IDs are accepted unchanged

- **WHEN** `ValidateID` receives a 1-byte ID and a 246-byte ID drawn from the alphabet
- **THEN** both are accepted

#### Scenario: Out-of-grammar IDs are refused

- **WHEN** `ValidateID` receives an empty ID, a 247-byte ID, or an ID with a byte outside the alphabet (slash, colon,
  dot, space, wildcard or non-ASCII)
- **THEN** it returns an error and accepts no normalized form

#### Scenario: A non-token ID carries the KV code

- **WHEN** `ValidateID` receives `pack.one`
- **THEN** the error unwraps to a classified error with code `kv_token_invalid`

#### Scenario: Required and length refusals come first

- **WHEN** `ValidateID` receives an empty ID, or a 247-byte ID made of 246 alphabet bytes and a dot
- **THEN** each is refused, and neither error unwraps to a classified error with code `kv_token_invalid`
