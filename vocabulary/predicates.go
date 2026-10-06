package vocabulary

import (
	"fmt"
	"strings"
)

// Predicate vocabulary using three-level dotted notation: domain.category.property
// This maintains consistency with the unified semantic architecture.
//
// Design principles:
//   - Three levels: domain.category.property (e.g., "sensor.temperature.celsius")
//   - Enables NATS wildcard queries: "sensor.temperature.*" finds all temperature predicates
//   - Human readable: "geo.location.latitude" is clear and semantic
//   - Domain scoped: Each domain manages its own predicate categories
//   - Consistent with EntityType.Key(), MessageType.Key(), and EntityID.Key() patterns
//
// Predicate naming conventions:
//   - domain: lowercase, represents business domain (sensors, geo, time, etc.)
//   - category: lowercase, groups related properties (temperature, location, lifecycle, etc.)
//   - property: lowercase, specific property name (celsius, latitude, created, etc.)
//   - No underscores or special characters (dots only for level separation)
//
// Domain applications (like semops for robotics) define their own domain-specific predicates.
// These examples demonstrate the framework's predicate pattern.

// Geospatial Domain Predicates
// These predicates describe location, positioning, and geographic data

const (
	// GeoLocationLatitude is float64, degrees (-90 to 90)
	GeoLocationLatitude = "geo.location.latitude"
	// GeoLocationLongitude is float64, degrees (-180 to 180)
	GeoLocationLongitude = "geo.location.longitude"
	// GeoLocationAltitude is float64, meters above sea level
	GeoLocationAltitude = "geo.location.altitude"
)

// Graph Domain Predicates
// These predicates describe relationships between entities in the semantic graph

const (
	// GraphRelContains represents hierarchical containment (parent contains child)
	// Example: A platform contains sensors, a system contains components
	GraphRelContains = "graph.rel.contains"
)

// Hierarchy Domain Predicates
// These predicates describe relationships derived from 6-part entity ID structure.
// Entity IDs follow the pattern: org.platform.system.domain.type.instance
// Hierarchy inference automatically creates these edges at entity ingestion time.

const (
	// HierarchyDomainMember indicates entity belongs to a domain (3-part prefix match).
	// Subject is the entity, object is the domain prefix (e.g., "acme.dep1.sensor").
	// StandardIRI: skos:broader (entity is narrower than domain)
	// InverseOf: HierarchyDomainContains
	// Example: sensor-temp-001 hierarchy.domain.member acme.dep1.sensor
	HierarchyDomainMember = "hierarchy.domain.member"

	// HierarchyDomainContains indicates domain contains an entity (inverse of HierarchyDomainMember).
	// Subject is the domain, object is the entity.
	// StandardIRI: skos:narrower (domain contains narrower entities)
	// InverseOf: HierarchyDomainMember
	// Example: acme.dep1.sensor hierarchy.domain.contains sensor-temp-001
	HierarchyDomainContains = "hierarchy.domain.contains"

	// HierarchySystemMember indicates entity belongs to a system (4-part prefix match).
	// Subject is the entity, object is the system prefix (e.g., "acme.dep1.document.sensor").
	// StandardIRI: skos:broader (entity is narrower than system)
	// InverseOf: HierarchySystemContains
	// Example: sensor-temp-001 hierarchy.system.member acme.dep1.document.sensor
	HierarchySystemMember = "hierarchy.system.member"

	// HierarchySystemContains indicates system contains an entity (inverse of HierarchySystemMember).
	// Subject is the system, object is the entity.
	// StandardIRI: skos:narrower (system contains narrower entities)
	// InverseOf: HierarchySystemMember
	// Example: acme.dep1.document.sensor hierarchy.system.contains sensor-temp-001
	HierarchySystemContains = "hierarchy.system.contains"

	// HierarchyTypeSibling indicates entities share the same type (5-part prefix match).
	// Bidirectional relationship between entities with same type prefix.
	// StandardIRI: skos:related (symmetric relationship)
	// IsSymmetric: true (if A is sibling of B, B is sibling of A)
	// Example: sensor-temp-001 hierarchy.type.sibling sensor-temp-002
	HierarchyTypeSibling = "hierarchy.type.sibling"

	// HierarchyTypeMember indicates entity belongs to a type container (5-part prefix + .group).
	// Subject is the entity, object is the type container entity ID.
	// StandardIRI: skos:broader (entity is narrower than type container)
	// InverseOf: HierarchyTypeContains
	// Example: acme.iot.sensors.hvac.temperature.001 hierarchy.type.member acme.iot.sensors.hvac.temperature.group
	HierarchyTypeMember = "hierarchy.type.member"

	// HierarchyTypeContains indicates type container contains an entity (inverse of HierarchyTypeMember).
	// Subject is the type container, object is the entity.
	// StandardIRI: skos:narrower (container contains narrower entities)
	// InverseOf: HierarchyTypeMember
	// Example: acme.iot.sensors.hvac.temperature.group hierarchy.type.contains acme.iot.sensors.hvac.temperature.001
	HierarchyTypeContains = "hierarchy.type.contains"
)

// Indexing Eligibility (ADR-054)
// The producer's hint for which semantic substrates should index an entity.
// Single-valued, stamped once at entity creation by graph-ingest. Gates ONLY
// the embedding/community/search substrates — never the structural graph,
// which stays queryable/traversable regardless of profile.

// EntityIndexingProfile is the reserved single-valued predicate carrying an
// entity's indexing profile (one of the IndexingProfile* values below).
// Stamped at entity creation (replace-on-write); read by indexing consumers.
const EntityIndexingProfile = "entity.indexing.profile"

const (
	// IndexingProfileContent is the retrieval corpus (embed yes, community yes):
	// memory, documents, research facts, lessons, prose-bearing entities.
	IndexingProfileContent = "content"
	// IndexingProfileControl is low-cardinality lifecycle/harness/run machinery
	// (embed yes — the cardinality guard excludes high-fan-out control types —
	// community yes). The substrate stays semantically reachable. Default floor.
	IndexingProfileControl = "control"
	// IndexingProfileSignal is telemetry readings (embed only summarized
	// rollups, community aggregate only). Raw readings stay graph-visible, not
	// embedded.
	IndexingProfileSignal = "signal"
	// IndexingProfileTrace is mechanically generated audit/trace entities (embed
	// no, community no). Fully graph-visible and queryable, but never embedded.
	IndexingProfileTrace = "trace"
)

// IsValidIndexingProfile reports whether s is one of the four recognized
// indexing profiles. Empty and unrecognized strings are invalid (callers
// treat invalid as "absent" and fall through to the fallback floor rather
// than failing, per ADR-054 lenient Phase 1 semantics).
func IsValidIndexingProfile(s string) bool {
	switch s {
	case IndexingProfileContent, IndexingProfileControl, IndexingProfileSignal, IndexingProfileTrace:
		return true
	default:
		return false
	}
}

// Predicate Object Datatypes (ADR-107, gh#1267)
// The closed vocabulary a predicate declares for its object values. It names
// the PRAGMATIC type of the object, never the Go type of the value: the Go
// type is something the serializer observes directly, and a value read back
// through the authoritative ENTITY_STATES JSON round trip no longer carries
// the Go type its author predicted.
//
// Per ADR-107 no semantic-web term appears here. The mapping from these
// values to XSD/RDF datatype IRIs belongs to the serializer, at the export
// edge, and lives in vocabulary/export.

const (
	// DataTypeString declares plain text.
	DataTypeString = "string"
	// DataTypeEntityID declares that the object is a canonical 6-part entity
	// ID rather than text that happens to resemble one.
	DataTypeEntityID = "entity_id"
	// DataTypeInt declares that the number is semantically whole, whatever the
	// JSON round trip left in the Go value.
	DataTypeInt = "int"
	// DataTypeFloat declares a real number.
	DataTypeFloat = "float"
	// DataTypeBool declares a truth value.
	DataTypeBool = "bool"
	// DataTypeDateTime declares an instant, whether carried as a time.Time or
	// as an RFC 3339 string.
	DataTypeDateTime = "datetime"
	// DataTypeJSON declares that the string holds a structured document.
	DataTypeJSON = "json"
)

// isValidDataType reports whether s is one of the seven canonical predicate
// object datatypes. The empty string is not one of them: registration accepts
// it as "no datatype declared" rather than as a value.
//
// Unexported deliberately (gh#1267 owner ruling: "in package"). ADR-106 freezes
// this Tier 1 package's surface, so a membership helper adopters never asked
// for is a permanent bill for a fact the seven exported constants already
// carry. Callers outside this package compare against those constants.
func isValidDataType(s string) bool {
	switch s {
	case DataTypeString, DataTypeEntityID, DataTypeInt, DataTypeFloat,
		DataTypeBool, DataTypeDateTime, DataTypeJSON:
		return true
	default:
		return false
	}
}

// validateDataType reports whether a declared datatype is one the registry
// accepts: a canonical value, or absent.
//
// The framework normalizes nothing (gh#1267, owner ruling 2026-09-09, option
// (d) no-legacy). There is deliberately no alias or legacy-spelling map here:
// a normalizer on a Tier 1 frozen package is an undated permanent bridge, and
// the whole adopter story is migration instead —
// docs/operations/migration-predicate-datatype.md carries the family-wide
// site list. So an adopter who declared a legacy spelling or a Go struct name
// is refused at registration and told what to write.
//
// The empty string is accepted: an absent datatype is a legitimate
// registration shape (three sister repositories register nothing but a
// predicate name), and #1277 tracks closing the framework's own bare set.
func validateDataType(declared string) error {
	if declared == "" || isValidDataType(declared) {
		return nil
	}
	return fmt.Errorf("unrecognized data type %q: expected one of %s, or none "+
		"(see docs/operations/migration-predicate-datatype.md)",
		declared, strings.Join(canonicalDataTypes(), ", "))
}

// canonicalDataTypes returns the seven canonical values in a stable order, for
// error messages and for the repository contract guards that walk the closed
// set rather than a hand-copied list of it.
func canonicalDataTypes() []string {
	return []string{
		DataTypeString,
		DataTypeEntityID,
		DataTypeInt,
		DataTypeFloat,
		DataTypeBool,
		DataTypeDateTime,
		DataTypeJSON,
	}
}

// Content Domain Predicates
// Product-neutral content-classification convention. Products that classify
// content emit these; the framework recognizes them for synthesis/enrichment.
const (
	// ContentClassificationTag is a multi-valued string classification tag
	// (one triple per tag) carrying thematic vocabulary for content entities.
	ContentClassificationTag = "content.classification.tag"
)

// NOTE: SemStreams is a framework - applications should define their own domain-specific
// vocabulary in their own packages and register predicates using the vocabulary registry.

// PredicateMetadata provides semantic information about each predicate
// This enables validation, type checking, and documentation generation
type PredicateMetadata struct {
	// Name is the predicate constant (e.g., "sensor.temperature.celsius")
	// Uses dotted notation for NATS stream query compatibility
	Name string

	// Description provides human-readable documentation
	Description string

	// DataType declares the pragmatic type of the object value, as one of
	// the seven canonical DataType* values, or is absent.
	//
	// It is NOT the Go type of the value. The serializer observes the Go type
	// directly, and a value read back through the authoritative ENTITY_STATES
	// JSON round trip no longer carries the Go type its author predicted — a
	// declared whole number arrives as float64 every time. The declaration
	// therefore carries only what observation cannot recover.
	//
	// Any value outside the closed vocabulary is refused at registration, and
	// nothing is normalized — what is declared here is what every reader
	// observes. Absent is legal.
	// RDF export honors the declaration, and ignores it for any triple whose
	// observed value contradicts it (gh#1267, ADR-107).
	DataType string

	// Units carries human-readable measurement units, as free-form
	// DOCUMENTATION on the declaration. Examples: "percent", "celsius".
	//
	// No framework path validates, normalizes, interprets, or honors it. That
	// is stated rather than left implicit because the alternative failure is
	// silent: a field that looks typed, sits beside DataType which IS honored,
	// and is frozen into a released surface reads as a promise the system does
	// not keep. When a consumer is named it arrives with its own change; a
	// closed vocabulary is deliberately not invented ahead of one (gh#1267 Q4,
	// tracked on gh#1264).
	Units string

	// Range carries a human-readable description of valid values, as free-form
	// DOCUMENTATION on the declaration. Examples: "0-100", "-90 to 90",
	// "positive" — three incompatible grammars, which is why no framework path
	// validates, normalizes, interprets, or honors it. See Units (gh#1267 Q4,
	// tracked on gh#1264).
	Range string

	// Domain identifies which domain owns this predicate
	Domain string

	// Category identifies the predicate category within the domain
	Category string

	// StandardIRI provides the W3C/RDF equivalent IRI for standards compliance (optional)
	// Examples: "http://www.w3.org/2002/07/owl#sameAs", "http://www.w3.org/2004/02/skos/core#prefLabel"
	// This enables RDF/JSON-LD export and semantic web interoperability while maintaining
	// dotted notation for NATS compatibility internally.
	// See vocabulary/standards.go for common constants.
	StandardIRI string

	// Alias semantics (for entity resolution and alias indexing)
	// IsAlias marks predicates that represent entity aliases
	IsAlias bool

	// AliasType defines the semantic meaning (identity, label, external, etc.)
	// Only meaningful when IsAlias is true. See AliasType documentation for
	// standard vocabulary mappings (OWL, SKOS, Schema.org).
	AliasType AliasType

	// AliasPriority defines conflict resolution order (lower number = higher priority)
	// Only meaningful when IsAlias is true
	AliasPriority int

	// InverseOf names the predicate that represents the inverse relationship.
	// Example: "hierarchy.type.member" has InverseOf = "hierarchy.type.contains"
	// When entity A → hierarchy.type.member → B, the inverse relationship is
	// B → hierarchy.type.contains → A.
	//
	// Important: Both predicates in an inverse pair should be registered with
	// their InverseOf pointing to each other. The registry stores relationships;
	// it does not auto-generate inverse triples at runtime.
	//
	// StandardIRI equivalent: owl:inverseOf
	InverseOf string

	// IsSymmetric indicates the predicate is its own inverse.
	// Example: "hierarchy.type.sibling" - if A is sibling of B, then B is sibling of A.
	// Symmetric predicates don't need InverseOf set; GetInversePredicate() returns
	// the predicate itself for symmetric cases.
	//
	// StandardIRI equivalent: owl:SymmetricProperty
	IsSymmetric bool

	// RuleOpaque marks the predicate as carrying free-form content that
	// rules MUST NOT predicate on. The rule-validator rejects rule
	// conditions whose `field` names a rule-opaque predicate at
	// config-load time.
	//
	// Used for agent-private observable state (ADR-036): the owning
	// agent is the sole writer and sole interpreter of content; rules
	// may match structural facts (counts, status enums, transitions)
	// but not the content itself. The asymmetric ownership pattern
	// avoids LLM-on-LLM checklist Goodhart by keeping content out of
	// the rule-engine's branching surface.
	RuleOpaque bool

	// Weight is the predicate's salience weight for ranking, SIGNED: positive =
	// more salient (boost), 0 = unweighted (neutral), negative = down-rank
	// (demote, gh#441). Lets a ranker prefer entities carrying salient facts, or
	// push down structurally-identifiable noise (tests, generated code, mocks)
	// that carries the same boosted predicates as the real thing, without
	// hard-coding a per-product table.
	Weight float64
}

// IsValidPredicate reports whether predicate satisfies the canonical lower-kebab
// domain.category.property contract. ParsePredicate returns the typed reason.
func IsValidPredicate(predicate string) bool {
	_, err := ParsePredicate(predicate)
	return err == nil
}
