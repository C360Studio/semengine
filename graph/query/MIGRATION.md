# Consumer migration: `graph/query`

For developers of projects moving onto SemEngine's `graph/query`. SemStreams' classifier chain had an example
classifier between the keyword tier and the LLM tier. SemEngine has no example classifier, so the chain is the keyword
tier, then the optional LLM tier.

- **`NewClassifierChain`.** Drop the middle argument. The call was `NewClassifierChain(keyword, example, llm)` and is
  now `NewClassifierChain(keyword, llm)`, where `llm` is one `*LLMClassifier`, or `nil` for the keyword tier alone. A
  call that passed `nil` for the example tier and `nil` for the LLM tier now passes one `nil`. An unchanged
  three-argument call no longer compiles, so no call keeps building while losing its LLM tier.
- **`EmbeddingClassifier`, `NewEmbeddingClassifier`.** Removed, with nothing at tier 0 in their place: a product
  without a language model classifies by keywords only. A product with one can pass its examples to
  `NewLLMClassifier`, which needs an `LLMClient` (`LoadDomainExamples` still reads an example file).
