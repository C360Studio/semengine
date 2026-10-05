package message

// Behavioral Interfaces
//
// This file defines PURE structural behavioral interfaces that payloads can
// optionally implement to expose additional capabilities.
//
// Semantic interfaces (Graphable, Storable) are defined in separate files.
// RuleReadable is a member of this family too; it lives in rule_readable.go.

// IndexingProfiler lets a Graphable payload declare its indexing profile —
// the producer's hint for which semantic substrates (embedding, community
// clustering, search) should index the entity (ADR-054). It does NOT affect
// the structural graph, which is always queryable/traversable regardless of
// profile ("graph visibility is storage; indexing is policy").
//
// Optional and additive: a payload that does not implement it falls through
// to graph-ingest's resolution order (mutation-envelope field, then the
// fallback floor derived from MessageType + EntityID type-segment). Absence
// means "let the framework decide", never "no profile".
//
// Valid return values are the four indexing profiles in the vocabulary
// package: IndexingProfileContent, IndexingProfileControl,
// IndexingProfileSignal, IndexingProfileTrace. An empty or unrecognized
// return is treated as absent (graph-ingest falls through to the floor).
type IndexingProfiler interface {
	// IndexingProfile returns one of "content" | "control" | "signal" | "trace".
	IndexingProfile() string
}
