# embedding-index Delta

## MODIFIED Requirements

### Requirement: Only query text carries the query prefix

`GenerateQuery` SHALL send `"input_type": "query"` and `Generate` SHALL send `"input_type": "document"` in every
request body, unless `Config.OmitInputType` is true, when neither SHALL send the field. `GenerateQuery` SHALL prepend
the configured query prefix to each text, and `Generate` SHALL never prepend it; with no prefix configured the two
SHALL send the same texts.

#### Scenario: Asymmetric model

- **WHEN** a prefix is configured and the same text goes through both methods
- **THEN** only the query request carries the prefix (`TestHTTPEmbedder_GenerateQuery_PrependsPrefix`)

#### Scenario: The request names its input type

- **WHEN** a text goes through `GenerateQuery` and then `Generate`
- **THEN** the query request's body carries `"input_type": "query"` and the document request's `"input_type":
  "document"`, and with `OmitInputType` true neither body has the field (`TestInputTypeNamesTheText`)
