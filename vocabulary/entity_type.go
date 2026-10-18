package vocabulary

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

const maxEntityTypeKeyBytes = 2*MaxPredicateSegmentBytes + 1

// EntityTypeMetadata maps a two-segment lower-kebab entity-type key
// (<domain>.<type>, naming the domain and type segments at 0-indexed positions
// 2 and 4 of a 6-part entity ID) to its RDF/OWL class IRI and superclass IRIs.
type EntityTypeMetadata struct {
	// DomainType is the canonical "<domain>.<type>" key (for example, "uxs.uas").
	DomainType string

	// Domain is the first segment of DomainType (0-indexed segment 2 of an entity ID).
	Domain string

	// Type is the second segment of DomainType (0-indexed segment 4 of an entity ID).
	Type string

	// Description provides human-readable documentation for the entity type.
	Description string

	// ClassIRI is the primary RDF/OWL class IRI for entities of this type.
	ClassIRI string

	// Superclasses holds additional RDF/OWL class IRIs (for example, parent
	// CCO/BFO classes) in registration order without duplicates.
	Superclasses []string

	optErr error
}

// EntityTypeOption configures an EntityTypeMetadata registration.
type EntityTypeOption func(*EntityTypeMetadata)

// WithEntityTypeDescription sets the human-readable description of an entity type.
func WithEntityTypeDescription(desc string) EntityTypeOption {
	return func(m *EntityTypeMetadata) {
		m.Description = desc
	}
}

// WithClassIRI sets the primary RDF/OWL class IRI for an entity type.
// Passing an empty or whitespace-containing IRI causes RegisterEntityType to panic.
func WithClassIRI(iri string) EntityTypeOption {
	return func(m *EntityTypeMetadata) {
		if err := validateNonEmptyIRI(iri, "class_iri"); err != nil && m.optErr == nil {
			m.optErr = err
			return
		}
		m.ClassIRI = iri
	}
}

// WithSuperclass appends a superclass IRI to an entity type's Superclasses
// slice if it is not already present. Passing an empty or whitespace-containing
// IRI causes RegisterEntityType to panic.
func WithSuperclass(iri string) EntityTypeOption {
	return func(m *EntityTypeMetadata) {
		if err := validateNonEmptyIRI(iri, "superclass"); err != nil && m.optErr == nil {
			m.optErr = err
			return
		}
		if !slices.Contains(m.Superclasses, iri) {
			m.Superclasses = append(m.Superclasses, iri)
		}
	}
}

// ParseEntityTypeKey validates and decomposes a two-segment lower-kebab ASCII
// entity-type key "<domain>.<type>". Each segment follows the vocabulary
// predicate segment grammar ([a-z][a-z0-9-]* with no trailing '-', at most
// MaxPredicateSegmentBytes bytes), which is deliberately narrower than the
// general segment grammar accepted by pkg/types.ValidateEntityID.
func ParseEntityTypeKey(domainType string) (domain, entityType string, err error) {
	if domainType == "" {
		return "", "", fmt.Errorf("entity type key must not be empty")
	}
	if len(domainType) > maxEntityTypeKeyBytes {
		return "", "", fmt.Errorf("entity type key %q exceeds %d bytes", domainType, maxEntityTypeKeyBytes)
	}
	segments := strings.Split(domainType, ".")
	if len(segments) != 2 {
		return "", "", fmt.Errorf("entity type key %q must have exactly 2 segments (<domain>.<type>)", domainType)
	}
	if err := validatePredicateSegment(domainType, segments[0], 0); err != nil {
		return "", "", fmt.Errorf("entity type key %q domain segment: %w", domainType, err)
	}
	if err := validatePredicateSegment(domainType, segments[1], 1); err != nil {
		return "", "", fmt.Errorf("entity type key %q type segment: %w", domainType, err)
	}
	return segments[0], segments[1], nil
}

// RegisterEntityType registers or amends metadata for a "<domain>.<type>"
// entity-type key. It panics if domainType does not satisfy ParseEntityTypeKey
// or if any option supplies an empty or whitespace-containing IRI.
func RegisterEntityType(domainType string, opts ...EntityTypeOption) {
	domain, entityType, err := ParseEntityTypeKey(domainType)
	if err != nil {
		panic(fmt.Sprintf("register entity type %q: %v", domainType, err))
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	meta := cloneEntityTypeMetadataValue(entityTypeRegistry[domainType])
	meta.DomainType = domainType
	meta.Domain = domain
	meta.Type = entityType
	meta.optErr = nil

	for _, opt := range opts {
		opt(&meta)
	}
	if err := validateEntityTypeMetadata(meta); err != nil {
		panic(fmt.Sprintf("register entity type %q: %v", domainType, err))
	}

	entityTypeRegistry[domainType] = meta
}

func validateEntityTypeMetadata(meta EntityTypeMetadata) error {
	if meta.optErr != nil {
		return meta.optErr
	}
	if meta.ClassIRI != "" {
		if err := validateNonEmptyIRI(meta.ClassIRI, "class_iri"); err != nil {
			return err
		}
	}
	for _, sc := range meta.Superclasses {
		if err := validateNonEmptyIRI(sc, "superclass"); err != nil {
			return err
		}
	}
	return nil
}

func cloneEntityTypeMetadataValue(meta EntityTypeMetadata) EntityTypeMetadata {
	out := meta
	out.optErr = nil
	if len(meta.Superclasses) > 0 {
		out.Superclasses = make([]string, len(meta.Superclasses))
		copy(out.Superclasses, meta.Superclasses)
	}
	return out
}

// GetEntityTypeMetadata returns a defensive copy of the registered metadata for
// domainType, or nil if domainType is not registered.
func GetEntityTypeMetadata(domainType string) *EntityTypeMetadata {
	registryMu.RLock()
	defer registryMu.RUnlock()

	meta, exists := entityTypeRegistry[domainType]
	if !exists {
		return nil
	}
	cloned := cloneEntityTypeMetadataValue(meta)
	return &cloned
}

// ListRegisteredEntityTypes returns all registered "<domain>.<type>" keys
// sorted in lexicographical order.
func ListRegisteredEntityTypes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	keys := make([]string, 0, len(entityTypeRegistry))
	for k := range entityTypeRegistry {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// LookupEntityTypesByClassIRI returns all registered "<domain>.<type>" keys
// whose ClassIRI or Superclasses contain classIRI, sorted in lexicographical
// order, or nil when classIRI is empty or matches no registered entity type.
func LookupEntityTypesByClassIRI(classIRI string) []string {
	if classIRI == "" {
		return nil
	}

	registryMu.RLock()
	defer registryMu.RUnlock()

	var matches []string
	for key, meta := range entityTypeRegistry {
		if meta.ClassIRI == classIRI || slices.Contains(meta.Superclasses, classIRI) {
			matches = append(matches, key)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	return matches
}
