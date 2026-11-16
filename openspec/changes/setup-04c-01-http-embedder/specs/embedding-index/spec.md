# embedding-index Delta

## ADDED Requirements

### Requirement: The HTTP embedder sends one request per call

The HTTP embedder SHALL send each `Generate` or `GenerateQuery` call as one `POST` to `<base URL>/embeddings` with the
configured model and the texts, sending an `Authorization` header only when an API key is configured, and SHALL send
nothing for no texts. It SHALL import neither the OpenAI SDK nor the module's `model` package.

#### Scenario: Request shape

- **WHEN** two texts are embedded with no API key configured
- **THEN** one `POST` to `<base URL>/embeddings` carries the model and both texts and no `Authorization` header
  (`TestRequestShape`)

### Requirement: The HTTP embedder trusts only a whole reply

The HTTP embedder SHALL return one vector per text, placed by the reply's `index`. It SHALL return an error and no
vectors when the reply's status is not 2xx, its count differs from the texts', an `index` is out of range or repeated,
a vector is empty or of another width than it learned, or the reply names another model. `Dimensions` SHALL be 0
until the first accepted reply and that reply's width after it.

#### Scenario: Out-of-order reply

- **WHEN** the provider replies with `index` 1 before `index` 0
- **THEN** each vector is returned at its text's position (`TestVectorsPlacedByIndex`)

#### Scenario: An untrusted reply

- **WHEN** the reply has a wrong count, a repeated index, an empty vector, a second width, or another model
- **THEN** the call returns an error and no vectors (`TestRefusesUntrustedReply`)

### Requirement: Only query text carries the query prefix

`GenerateQuery` SHALL prepend the configured query prefix to each text, and `Generate` SHALL never prepend it; with no
prefix configured the two SHALL send the same texts.

#### Scenario: Asymmetric model

- **WHEN** a prefix is configured and the same text goes through both methods
- **THEN** only the query request carries the prefix (`TestHTTPEmbedder_GenerateQuery_PrependsPrefix`)
