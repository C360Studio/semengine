package graph

// The reserved sources: the names graph-ingest and pkg/lifecycle write their
// own statements under. A stream message may not carry a statement under one
// of them (graph-entity-writes, "Statement metadata is required"), so no
// producer on the stream can replace their sets; the mutation lane accepts
// them, since pkg/lifecycle writes there under its own.
const (
	// SourceIndexingProfile names the indexing-profile statement graph-ingest
	// stamps at an entity's birth (ADR-054).
	SourceIndexingProfile = "graph-ingest-indexing-profile"
	// SourceHierarchy names the hierarchy statements graph-ingest derives, and
	// the statements of the containers it creates for them.
	SourceHierarchy = "graph-ingest-hierarchy"
	// SourceLifecycle names every statement pkg/lifecycle's manager writes.
	SourceLifecycle = "semengine-lifecycle"
)

// IsReservedSource reports whether source is one of the reserved sources,
// compared exactly.
func IsReservedSource(source string) bool {
	switch source {
	case SourceIndexingProfile, SourceHierarchy, SourceLifecycle:
		return true
	}
	return false
}
