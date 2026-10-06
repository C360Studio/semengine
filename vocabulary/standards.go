package vocabulary

// Standard Vocabulary IRIs
//
// These constants provide commonly used W3C and semantic web standard IRIs.
// Use these in PredicateMetadata.StandardIRI to indicate semantic equivalence
// with established vocabularies.
//
// NOTE: SemStreams uses dotted notation internally (e.g., "semantic.identity.alias")
// for NATS stream query compatibility. These IRIs are for documentation, export,
// and interoperability purposes. Processors can translate between formats if needed.
//
// References:
// - OWL: https://www.w3.org/TR/owl2-overview/
// - SKOS: https://www.w3.org/TR/skos-reference/
// - Dublin Core: https://www.dublincore.org/specifications/dublin-core/dcmi-terms/
// - Schema.org: https://schema.org/
// - PROV-O: https://www.w3.org/TR/prov-o/

// SKOS (Simple Knowledge Organization System) Standard IRIs
const (
	// SkosNotation provides a notation (code or identifier) within a concept scheme.
	// Used for: AliasTypeAlternate
	SkosNotation = "http://www.w3.org/2004/02/skos/core#notation"

	// SkosBroader indicates a hierarchical link to a more general concept.
	// Used for: hierarchy.*.member predicates (entity → container membership)
	// Example: sensor-001 skos:broader temperature-group (sensor is narrower than group)
	SkosBroader = "http://www.w3.org/2004/02/skos/core#broader"

	// SkosNarrower indicates a hierarchical link to a more specific concept.
	// Inverse of SkosBroader.
	// Example: temperature-group skos:narrower sensor-001 (group contains sensor)
	SkosNarrower = "http://www.w3.org/2004/02/skos/core#narrower"

	// SkosRelated indicates an associative (non-hierarchical) link between concepts.
	// Used for: hierarchy.type.sibling predicates (symmetric relationship)
	// Example: sensor-001 skos:related sensor-002 (siblings in same type group)
	SkosRelated = "http://www.w3.org/2004/02/skos/core#related"
)

// Dublin Core Metadata Terms Standard IRIs
const (
	// DcIdentifier provides an unambiguous reference to the resource.
	// Used for: AliasTypeExternal
	// Example: ISBN, DOI, serial number
	DcIdentifier = "http://purl.org/dc/terms/identifier"

	// DcTitle provides the name given to the resource.
	// Used for: AliasTypeLabel
	DcTitle = "http://purl.org/dc/terms/title"
)

// Dublin Core Dotted Notation Predicates
// These constants provide dotted notation predicates for use in Triples.
// They map semantically to the Dublin Core IRIs above.
const (
	// DCTermsTitle is the dotted notation predicate for resource title.
	// Maps to: DcTitle (http://purl.org/dc/terms/title)
	DCTermsTitle = "dc.terms.title"
)

// PROV-O (Provenance Ontology) Standard IRIs
// Reference: https://www.w3.org/TR/prov-o/

// PROV-O Namespace
const (
	// ProvNamespace is the base IRI prefix for all PROV-O terms.
	ProvNamespace = "http://www.w3.org/ns/prov#"
)

// PROV-O Derivation Relations
// Relations expressing how entities derive from other entities.
const (
	// ProvWasAttributedTo indicates who an entity was attributed to.
	// Domain: Entity, Range: Agent
	ProvWasAttributedTo = ProvNamespace + "wasAttributedTo"
)

// PROV-O Association Relations
// Relations expressing how agents are associated with activities.
const (
	// ProvHadMember indicates membership in a collection.
	// Domain: Collection, Range: Entity
	// Used for: GraphRelContains
	ProvHadMember = ProvNamespace + "hadMember"
)

// PROV-O Time Properties
// Properties for expressing when activities occurred and entities existed.
const (
	// ProvGeneratedAtTime indicates when an entity was generated.
	// Domain: Entity, Range: xsd:dateTime
	ProvGeneratedAtTime = ProvNamespace + "generatedAtTime"
)
