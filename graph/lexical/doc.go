// Package lexical holds tier 0 word search's rules for turning text into words.
//
// Tokens is the one tokenizer: text written into an index and the text of a
// query go through it alone, so a query word and an indexed word agree exactly
// when they come from the same written word, its plural, or one part of a
// split identifier.
//
// The package is pure: it imports no NATS client and reads no storage.
package lexical
