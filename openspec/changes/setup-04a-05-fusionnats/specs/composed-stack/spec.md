# composed-stack

## ADDED Requirements

### Requirement: The graph stack answers fusion without an embedder

graph-ingest, graph-index and graph-query SHALL start from one configuration with no graph-embedding, and
`fusion.Engine` over `fusionnats` SHALL answer symbol and prefix questions from them. A natural-language question SHALL
fail with graph-query's refusal, never an empty answer. `pkg/fusion/fusionnats` SHALL import neither
`graph/embedding`, `processor/graph-embedding` nor `graph/readiness`, directly or through a package it imports.

#### Scenario: No embedder

- **WHEN** the stack boots without graph-embedding and the producer publishes the fixed entities
- **THEN** the three components are healthy, `GRAPH_STATUS` holds no `graph-embedding` key, a symbol lens and a prefix
  lens return the expected nodes, relations and paths, an unknown name is a miss, and an NL lens's `Fuse` returns a
  transient `internal` refusal naming `graph.embedding.query.search` (`TestSmoke_NoEmbedderAnswers`)

#### Scenario: An embedding import

- **WHEN** a package `fusionnats` imports reaches `graph/embedding`
- **THEN** `TestFusionNATSImportsNoEmbedding` fails naming the import path
