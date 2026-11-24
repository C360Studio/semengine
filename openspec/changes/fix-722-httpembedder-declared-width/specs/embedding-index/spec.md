# embedding-index Delta

## MODIFIED Requirements

### Requirement: The HTTP embedder trusts only a whole reply

The HTTP embedder SHALL return one vector per text, placed by the reply's `index`. It SHALL return an error and no
vectors when the reply's status is not 2xx, its count differs from the texts', an `index` is out of range or repeated,
a vector is empty or of another width than it was configured with or learned, or the reply names another model. With
`Config.Dimensions` set, `Dimensions` SHALL be that width from construction, and `New` SHALL send no request; with it
0, `Dimensions` SHALL be 0 until the first accepted reply and that reply's width after it. `New` SHALL refuse a
negative `Config.Dimensions`.

#### Scenario: Out-of-order reply

- **WHEN** the provider replies with `index` 1 before `index` 0
- **THEN** each vector is returned at its text's position (`TestVectorsPlacedByIndex`)

#### Scenario: An untrusted reply

- **WHEN** the reply has a wrong count, a repeated index, an empty vector, a second width, or another model
- **THEN** the call returns an error and no vectors (`TestRefusesUntrustedReply`)

#### Scenario: A declared width

- **WHEN** the embedder is built with `Config.Dimensions` 3
- **THEN** `Dimensions` is 3 before any request, a reply of width 2 is refused with no vectors, and `Dimensions`
  stays 3 (`TestDeclaredWidth`)
