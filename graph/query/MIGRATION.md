# Consumer migration: `graph/query`

For developers of projects moving onto SemEngine's `graph/query`. SemStreams' classifier chain had an example
classifier between the keyword tier and the LLM tier. SemEngine has no example classifier, so the chain is the keyword
tier, then the optional LLM tier.

- **`NewClassifierChain`.** Drop the middle argument. The call was `NewClassifierChain(keyword, example, llm)` and is
  now `NewClassifierChain(keyword, llm...)`, where `llm` is zero or one `*LLMClassifier`. A call that passed `nil` for
  the example tier and `nil` for the LLM tier now passes one `nil`, or none.
- **`EmbeddingClassifier`, `NewEmbeddingClassifier`.** Removed. A product that matched questions to examples
  supplies the examples to `NewLLMClassifier` instead (`LoadDomainExamples` still reads an example file).
