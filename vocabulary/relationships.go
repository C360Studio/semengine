package vocabulary

// Relationship predicate registration with metadata and IRI mappings.
// This file registers the graph.rel.* predicates defined in predicates.go
// with their semantic metadata and standard vocabulary mappings.

func init() {
	// GraphRelContains - Hierarchical containment
	Register(GraphRelContains,
		WithDescription("Hierarchical containment relationship (parent contains child)"),
		WithIRI(ProvHadMember))
}
