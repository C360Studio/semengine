// Package lexical is tier 0 word search: an in-memory index that ranks
// entities by field-weighted BM25 over the words of their text.
//
// Tokens is the one tokenizer: text written into an index and the text of a
// query go through it alone, so a query word and an indexed word agree
// when they come from the same written word, its plural (an acronym's too:
// "IDs" and "ids" meet "id"), or one part of a split identifier. Folding the
// plural also joins a few words that are not plurals: "its" meets "it", and a
// three-letter acronym ending in "s" meets its first two letters ("TLS" and
// "tl", "CAS" and "CA").
//
// ParseQuery reads a query of plain words, "quoted phrases" and -excluded
// terms. Phrases and exclusions decide which entities match; plain and phrase
// words decide their order.
//
// The package is pure: it imports no NATS client and reads no storage.
package lexical
