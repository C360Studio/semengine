// Package lexical is tier 0 word search: an in-memory index that ranks
// entities by field-weighted BM25 over the words of their text.
//
// Tokens is the one tokenizer: text written into an index and the text of a
// query go through it alone, so a query word and an indexed word agree exactly
// when they come from the same written word, its plural, or one part of a
// split identifier.
//
// The package is pure: it imports no NATS client and reads no storage.
package lexical
