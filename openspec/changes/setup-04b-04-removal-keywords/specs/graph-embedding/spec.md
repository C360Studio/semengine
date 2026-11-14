# graph-embedding Delta

## ADDED Requirements

### Requirement: graph-embedding runs only with a product's embedder

graph-embedding SHALL embed with the `embedding.Embedder` a product registers through `RegisterWithEmbedder` under a
non-empty name, which SHALL be the type part of the dedup identity. A factory registered by plain `Register` SHALL
refuse construction, class invalid, naming `RegisterWithEmbedder` and tier 1. graph-embedding SHALL refuse at
construction an unknown config key, `embedder_type` among them, naming it, and SHALL import no provider module.

#### Scenario: No embedder registered

- **WHEN** graph-embedding is created from a registry filled by `Register`
- **THEN** construction fails invalid, naming `RegisterWithEmbedder` and tier 1 (`TestRegisterWithoutEmbedderRefuses`)

#### Scenario: A product's embedder

- **WHEN** a product registers graph-embedding with its embedder and an entity with text is written
- **THEN** the entity's record is generated from that embedder's vector, `search` embeds the query with it and
  replies the registered name as `embedder_type` (`TestProductEmbedderIsUsed`)

#### Scenario: A removed key

- **WHEN** the config holds `batch_size` or `embedder_type`
- **THEN** construction fails naming the key (`TestConfigRefusesUnknownKey`)

### Requirement: The embedder lives from Start to Stop

graph-embedding SHALL build its embedder in `Start` and close it once in `Stop`. A `Start` that fails after building
it SHALL close it, or report both errors and keep it for `Stop`.

#### Scenario: The embedder's lifetime

- **WHEN** a component is built and never started, started and stopped, or its `Start` fails after the build
- **THEN** no embedder is built, one is closed once, or it is closed (else kept and closed by `Stop`)
  (`TestEmbedderClosedOnStop`)

## MODIFIED Requirements

### Requirement: The embedding verbs answer in the envelope

Each answer SHALL be a `graph.QueryResponse` with producer `graph-embedding` and `indexed_revision` 0, whose `data` is
a `graph.SemanticData` (with `embedder_type` the name the embedder was registered under) or a `graph.SimilarData`. Its
`results` SHALL be a JSON array, its `unreadable_records` SHALL be the number of `EMBEDDING_INDEX` records the cache
could not decode, and its `failed_records` the number of entities whose latest embedding failed in the Worker.

#### Scenario: An empty index

- **WHEN** a `search` request arrives after the first build with no generated record
- **THEN** the reply's `data.results` is `[]` and `indexed_revision` is 0 (`TestSearchAnswersEnveloped`)

## REMOVED Requirements

### Requirement: Tier-0 config

**Reason**: There is no tier-0 embedder (design D5); "graph-embedding runs only with a product's embedder" carries
the strict-config and provider-import rules.
