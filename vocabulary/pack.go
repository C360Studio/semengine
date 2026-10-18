package vocabulary

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// PrefixSpec declares a namespace prefix mapping in a declarative vocabulary pack.
type PrefixSpec struct {
	Prefix    string `json:"prefix" yaml:"prefix"`
	Namespace string `json:"namespace" yaml:"namespace"`
}

// Validate checks that Prefix is a non-empty W3C Turtle PN_PREFIX label
// (starting with a PN_CHARS_BASE letter, containing only PN_CHARS or '.', and
// not ending in '.' or containing ':' or whitespace) and Namespace is a
// non-empty IRI without whitespace.
func (p PrefixSpec) Validate() error {
	if err := validatePNPrefix(p.Prefix); err != nil {
		return err
	}
	return validateNonEmptyIRI(p.Namespace, fmt.Sprintf("namespace for prefix %q", p.Prefix))
}

func validatePNPrefix(prefix string) error {
	if prefix == "" {
		return errors.New("prefix must not be empty")
	}
	var lastRune rune
	for i, r := range prefix {
		if i == 0 {
			if !isPNCharsBase(r) {
				return fmt.Errorf("prefix %q must start with a letter (Turtle PN_CHARS_BASE)", prefix)
			}
		} else if r != '.' && !isPNChars(r) {
			return fmt.Errorf("prefix %q contains invalid Turtle PN_PREFIX character %q", prefix, r)
		}
		lastRune = r
	}
	if lastRune == '.' {
		return fmt.Errorf("prefix %q must not end with '.'", prefix)
	}
	return nil
}

func isPNCharsBase(r rune) bool {
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= 0x00C0 && r <= 0x00D6) ||
		(r >= 0x00D8 && r <= 0x00F6) ||
		(r >= 0x00F8 && r <= 0x02FF) ||
		(r >= 0x0370 && r <= 0x037D) ||
		(r >= 0x037F && r <= 0x1FFF) ||
		(r >= 0x200C && r <= 0x200D) ||
		(r >= 0x2070 && r <= 0x218F) ||
		(r >= 0x2C00 && r <= 0x2FEF) ||
		(r >= 0x3001 && r <= 0xD7FF) ||
		(r >= 0xF900 && r <= 0xFDCF) ||
		(r >= 0xFDF0 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0xEFFFF)
}

func isPNChars(r rune) bool {
	return isPNCharsBase(r) ||
		r == '_' ||
		r == '-' ||
		(r >= '0' && r <= '9') ||
		r == 0x00B7 ||
		(r >= 0x0300 && r <= 0x036F) ||
		(r >= 0x203F && r <= 0x2040)
}

// PredicateSpec declares a canonical 3-part dotted predicate and its metadata
// in a declarative vocabulary pack.
type PredicateSpec struct {
	Name          string          `json:"name" yaml:"name"`
	Description   string          `json:"description,omitempty" yaml:"description,omitempty"`
	DataType      string          `json:"data_type,omitempty" yaml:"data_type,omitempty"`
	Units         string          `json:"units,omitempty" yaml:"units,omitempty"`
	Range         string          `json:"range,omitempty" yaml:"range,omitempty"`
	IRI           string          `json:"iri,omitempty" yaml:"iri,omitempty"`
	IsAlias       bool            `json:"is_alias,omitempty" yaml:"is_alias,omitempty"`
	AliasType     AliasType       `json:"alias_type,omitempty" yaml:"alias_type,omitempty"`
	AliasPriority int             `json:"alias_priority,omitempty" yaml:"alias_priority,omitempty"`
	InverseOf     string          `json:"inverse_of,omitempty" yaml:"inverse_of,omitempty"`
	Symmetric     bool            `json:"symmetric,omitempty" yaml:"symmetric,omitempty"`
	RuleOpaque    bool            `json:"rule_opaque,omitempty" yaml:"rule_opaque,omitempty"`
	Weight        float64         `json:"weight,omitempty" yaml:"weight,omitempty"`
	Projection    *ProjectionSpec `json:"projection,omitempty" yaml:"projection,omitempty"`
}

// EntityTypeSpec declares a canonical 2-part "<domain>.<type>" entity-type key
// and its RDF/OWL class mappings in a declarative vocabulary pack.
type EntityTypeSpec struct {
	DomainType   string   `json:"domain_type" yaml:"domain_type"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	ClassIRI     string   `json:"class_iri,omitempty" yaml:"class_iri,omitempty"`
	Superclasses []string `json:"superclasses,omitempty" yaml:"superclasses,omitempty"`
}

// PackSpec is a declarative JSON or YAML vocabulary pack containing namespace
// prefixes, predicates, and entity-type class mappings (such as BFO, CCO, or
// DIEKM domain packs).
type PackSpec struct {
	Name        string           `json:"name" yaml:"name"`
	Version     string           `json:"version,omitempty" yaml:"version,omitempty"`
	Prefixes    []PrefixSpec     `json:"prefixes,omitempty" yaml:"prefixes,omitempty"`
	Predicates  []PredicateSpec  `json:"predicates,omitempty" yaml:"predicates,omitempty"`
	EntityTypes []EntityTypeSpec `json:"entity_types,omitempty" yaml:"entity_types,omitempty"`
}

type packConfig struct {
	prefixRegistrar func(prefixes []PrefixSpec) error
}

// PackOption configures how RegisterPack applies a PackSpec.
type PackOption func(*packConfig)

// WithPrefixRegistrar supplies an atomic batch callback (such as
// export.RegisterPrefixes) that RegisterPack invokes with the pack's PrefixSpec
// slice after all pack entries pass validation.
func WithPrefixRegistrar(fn func(prefixes []PrefixSpec) error) PackOption {
	return func(c *packConfig) {
		c.prefixRegistrar = fn
	}
}

// LoadPackJSON decodes a PackSpec from JSON, refusing unknown fields,
// malformed syntax, empty input, or trailing JSON values.
func LoadPackJSON(r io.Reader) (PackSpec, error) {
	if r == nil {
		return PackSpec{}, errors.New("load JSON vocabulary pack: nil reader")
	}
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var pack PackSpec
	if err := dec.Decode(&pack); err != nil {
		return PackSpec{}, fmt.Errorf("load JSON vocabulary pack: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return PackSpec{}, errors.New("load JSON vocabulary pack: unexpected trailing JSON data")
		}
		return PackSpec{}, fmt.Errorf("load JSON vocabulary pack: trailing data: %w", err)
	}
	return pack, nil
}

// LoadPackYAML decodes a PackSpec from YAML, refusing unknown fields,
// malformed syntax, empty input, or trailing YAML documents.
func LoadPackYAML(r io.Reader) (PackSpec, error) {
	if r == nil {
		return PackSpec{}, errors.New("load YAML vocabulary pack: nil reader")
	}
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	var pack PackSpec
	if err := dec.Decode(&pack); err != nil {
		return PackSpec{}, fmt.Errorf("load YAML vocabulary pack: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return PackSpec{}, errors.New("load YAML vocabulary pack: unexpected trailing YAML document")
		}
		return PackSpec{}, fmt.Errorf("load YAML vocabulary pack: trailing document: %w", err)
	}
	return pack, nil
}

// Validate checks that p is non-nil with a non-empty Name, and that every
// prefix, predicate (including projections, alias rules, and intra-pack
// inverse symmetry), and entity type is valid and has no intra-pack duplicates.
// Validate is completely pure: it does not acquire registryMu, does not read
// or mutate global predicate/entity-type/prefix registries, and does not
// mutate the caller's PackSpec.
func (p *PackSpec) Validate() error {
	if p == nil {
		return errors.New("vocabulary pack must not be nil")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("vocabulary pack name must not be empty")
	}

	seenPrefixes := make(map[string]struct{}, len(p.Prefixes))
	for i, pfx := range p.Prefixes {
		if err := pfx.Validate(); err != nil {
			return fmt.Errorf("prefixes[%d]: %w", i, err)
		}
		if _, exists := seenPrefixes[pfx.Prefix]; exists {
			return fmt.Errorf("prefixes[%d]: duplicate prefix %q in pack", i, pfx.Prefix)
		}
		seenPrefixes[pfx.Prefix] = struct{}{}
	}

	packPredicates := make(map[string]PredicateMetadata, len(p.Predicates))
	for i, spec := range p.Predicates {
		if _, exists := packPredicates[spec.Name]; exists {
			return fmt.Errorf("predicates[%d] (%q): duplicate predicate in pack", i, spec.Name)
		}
		if err := stagePredicateSpec(i, spec, packPredicates); err != nil {
			return err
		}
	}
	for i, spec := range p.Predicates {
		if err := validatePredicateMetadataIn(packPredicates[spec.Name], packPredicates); err != nil {
			return fmt.Errorf("predicates[%d] (%q): %w", i, spec.Name, err)
		}
	}

	packEntityTypes := make(map[string]EntityTypeMetadata, len(p.EntityTypes))
	for i, spec := range p.EntityTypes {
		if _, exists := packEntityTypes[spec.DomainType]; exists {
			return fmt.Errorf("entity_types[%d] (%q): duplicate entity_type in pack", i, spec.DomainType)
		}
		if err := stageEntityTypeSpec(i, spec, packEntityTypes); err != nil {
			return err
		}
	}

	return nil
}

// RegisterPack validates every prefix, predicate, and entity type in pack
// before mutating state. If any entry is invalid or if the optional batch
// prefix registrar returns an error, RegisterPack leaves the predicate and
// entity-type registries unchanged.
func RegisterPack(pack PackSpec, opts ...PackOption) error {
	var cfg packConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	if err := pack.Validate(); err != nil {
		return err
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	stagedPredicates, stagedEntityTypes, err := stagePackLocked(pack)
	if err != nil {
		return err
	}

	if cfg.prefixRegistrar != nil && len(pack.Prefixes) > 0 {
		if err := cfg.prefixRegistrar(slices.Clone(pack.Prefixes)); err != nil {
			return fmt.Errorf("register pack %q prefixes: %w", pack.Name, err)
		}
	}

	predicateRegistry = stagedPredicates
	entityTypeRegistry = stagedEntityTypes
	return nil
}

func stagePackLocked(pack PackSpec) (map[string]PredicateMetadata, map[string]EntityTypeMetadata, error) {
	stagedPredicates := make(map[string]PredicateMetadata, len(predicateRegistry)+len(pack.Predicates))
	for k, v := range predicateRegistry {
		v.Projection = cloneProjectionSpec(v.Projection)
		stagedPredicates[k] = v
	}
	for i, spec := range pack.Predicates {
		if err := stagePredicateSpec(i, spec, stagedPredicates); err != nil {
			return nil, nil, err
		}
	}
	for i, spec := range pack.Predicates {
		if err := validatePredicateMetadataIn(stagedPredicates[spec.Name], stagedPredicates); err != nil {
			return nil, nil, fmt.Errorf("predicates[%d] (%q): %w", i, spec.Name, err)
		}
	}

	stagedEntityTypes := make(map[string]EntityTypeMetadata, len(entityTypeRegistry)+len(pack.EntityTypes))
	for k, v := range entityTypeRegistry {
		stagedEntityTypes[k] = cloneEntityTypeMetadataValue(v)
	}
	for i, spec := range pack.EntityTypes {
		if err := stageEntityTypeSpec(i, spec, stagedEntityTypes); err != nil {
			return nil, nil, err
		}
	}

	return stagedPredicates, stagedEntityTypes, nil
}

func stagePredicateSpec(i int, spec PredicateSpec, staged map[string]PredicateMetadata) error {
	parts, err := ParsePredicate(spec.Name)
	if err != nil {
		return fmt.Errorf("predicates[%d] (%q): %w", i, spec.Name, err)
	}
	if spec.IRI != "" {
		if err := validateNonEmptyIRI(spec.IRI, fmt.Sprintf("predicates[%d].iri", i)); err != nil {
			return err
		}
	}
	if !spec.IsAlias && spec.AliasType == "" && spec.AliasPriority != 0 {
		return fmt.Errorf("predicates[%d] (%q): alias metadata requires IsAlias=true", i, spec.Name)
	}

	meta := staged[spec.Name]
	meta.Projection = cloneProjectionSpec(meta.Projection)
	meta.Name = spec.Name
	meta.Domain = parts.Domain
	meta.Category = parts.Category

	if spec.Description != "" {
		meta.Description = spec.Description
	}
	if spec.DataType != "" {
		meta.DataType = spec.DataType
	}
	if spec.Units != "" {
		meta.Units = spec.Units
	}
	if spec.Range != "" {
		meta.Range = spec.Range
	}
	if spec.IRI != "" {
		meta.StandardIRI = spec.IRI
	}
	if spec.IsAlias || spec.AliasType != "" {
		meta.IsAlias = true
		meta.AliasType = spec.AliasType
		meta.AliasPriority = spec.AliasPriority
	}
	if spec.InverseOf != "" {
		meta.InverseOf = spec.InverseOf
	}
	if spec.Symmetric {
		meta.IsSymmetric = true
	}
	if spec.RuleOpaque {
		meta.RuleOpaque = true
	}
	if spec.Weight != 0 {
		meta.Weight = spec.Weight
	}
	if spec.Projection != nil {
		cloned := cloneProjectionSpec(spec.Projection)
		if cloned == nil {
			cloned = &ProjectionSpec{}
		}
		meta.Projection = cloned
	}

	if err := validatePredicateMetadataIn(meta, staged); err != nil {
		return fmt.Errorf("predicates[%d] (%q): %w", i, spec.Name, err)
	}
	staged[spec.Name] = meta
	return nil
}

func stageEntityTypeSpec(i int, spec EntityTypeSpec, staged map[string]EntityTypeMetadata) error {
	domain, entityType, err := ParseEntityTypeKey(spec.DomainType)
	if err != nil {
		return fmt.Errorf("entity_types[%d] (%q): %w", i, spec.DomainType, err)
	}
	meta := cloneEntityTypeMetadataValue(staged[spec.DomainType])
	meta.DomainType = spec.DomainType
	meta.Domain = domain
	meta.Type = entityType
	if spec.Description != "" {
		meta.Description = spec.Description
	}
	if spec.ClassIRI != "" {
		if err := validateNonEmptyIRI(spec.ClassIRI, fmt.Sprintf("entity_types[%d].class_iri", i)); err != nil {
			return err
		}
		meta.ClassIRI = spec.ClassIRI
	}
	for j, sc := range spec.Superclasses {
		if err := validateNonEmptyIRI(sc, fmt.Sprintf("entity_types[%d].superclasses[%d]", i, j)); err != nil {
			return err
		}
		if !slices.Contains(meta.Superclasses, sc) {
			meta.Superclasses = append(meta.Superclasses, sc)
		}
	}
	if err := validateEntityTypeMetadata(meta); err != nil {
		return fmt.Errorf("entity_types[%d] (%q): %w", i, spec.DomainType, err)
	}
	staged[spec.DomainType] = meta
	return nil
}
