# rule-pack-identity

## ADDED Requirements

### Requirement: A rule-pack ID is one bounded literal token

`rulepack.ValidateID` SHALL accept a rule-pack ID of 1 to 246 bytes in which every byte matches `[A-Za-z0-9_=-]`, and
SHALL refuse every other value without trimming, folding or otherwise normalizing it. A value that is not one literal
NATS KV token SHALL be refused with natsclient's classified `kv_token_invalid` error wrapped. `PackIDCharset` and
`MaxPackIDBytes` SHALL state the same alphabet and bound.

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
