package vocabulary

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"gopkg.in/yaml.v3"
)

// PrefixSpec declares a namespace prefix mapping in a declarative vocabulary pack.
type PrefixSpec struct {
	Prefix    string `json:"prefix" yaml:"prefix"`
	Namespace string `json:"namespace" yaml:"namespace"`
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
	prefixRegistrar func(prefix, namespace string) error
}

// PackOption configures how RegisterPack applies a PackSpec.
type PackOption func(*packConfig)

// WithPrefixRegistrar supplies a callback (such as export.Register) that
// RegisterPack invokes for each PrefixSpec after all pack entries pass
// validation.
func WithPrefixRegistrar(fn func(prefix, namespace string) error) PackOption {
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

// ValidatePack checks that every prefix, predicate, and entity type in pack is
// well-formed without mutating registry state.
func ValidatePack(pack PackSpec) error {
	registryMu.RLock()
	defer registryMu.RUnlock()

	_, _, err := stagePackLocked(pack)
	return err
}

// RegisterPack validates every prefix, predicate, and entity type in pack
// before mutating state. If any entry is invalid or if the optional prefix
// registrar returns an error, RegisterPack leaves the predicate and entity-type
// registries unchanged.
func RegisterPack(pack PackSpec, opts ...PackOption) error {
	var cfg packConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	stagedPredicates, stagedEntityTypes, err := stagePackLocked(pack)
	if err != nil {
		return err
	}

	if cfg.prefixRegistrar != nil {
		for _, pfx := range pack.Prefixes {
			if err := cfg.prefixRegistrar(pfx.Prefix, pfx.Namespace); err != nil {
				return fmt.Errorf("register pack %q prefix %q: %w", pack.Name, pfx.Prefix, err)
			}
		}
	}

	predicateRegistry = stagedPredicates
	entityTypeRegistry = stagedEntityTypes
	return nil
}

func stagePackLocked(pack PackSpec) (map[string]PredicateMetadata, map[string]EntityTypeMetadata, error) {
	seenPrefixes := make(map[string]string, len(pack.Prefixes))
	for i, pfx := range pack.Prefixes {
		if err := validateNonEmptyIRI(pfx.Prefix, fmt.Sprintf("prefixes[%d].prefix", i)); err != nil {
			return nil, nil, err
		}
		if err := validateNonEmptyIRI(pfx.Namespace, fmt.Sprintf("prefixes[%d].namespace", i)); err != nil {
			return nil, nil, err
		}
		if existing, ok := seenPrefixes[pfx.Prefix]; ok && existing != pfx.Namespace {
			return nil, nil, fmt.Errorf("prefixes[%d]: duplicate prefix %q with conflicting namespace", i, pfx.Prefix)
		}
		seenPrefixes[pfx.Prefix] = pfx.Namespace
	}

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

	if err := validateStagedPredicateMetadata(meta, staged); err != nil {
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

func validateStagedPredicateMetadata(meta PredicateMetadata, staged map[string]PredicateMetadata) error {
	if err := validateDataType(meta.DataType); err != nil {
		return err
	}
	if err := validateProjectionSpec(meta.Projection); err != nil {
		return err
	}
	if meta.InverseOf != "" {
		if _, err := ParsePredicate(meta.InverseOf); err != nil {
			return fmt.Errorf("inverse predicate %q: %w", meta.InverseOf, err)
		}
	}
	if meta.IsSymmetric && meta.InverseOf != "" {
		return fmt.Errorf("symmetric predicate cannot also declare inverse %q", meta.InverseOf)
	}
	if meta.IsAlias {
		if !validAliasType(meta.AliasType) {
			return fmt.Errorf("invalid alias type %q", meta.AliasType)
		}
	} else if meta.AliasType != "" || meta.AliasPriority != 0 {
		return fmt.Errorf("alias metadata requires IsAlias=true")
	}
	if inverse, ok := staged[meta.InverseOf]; meta.InverseOf != "" && ok &&
		inverse.InverseOf != "" && inverse.InverseOf != meta.Name {
		return fmt.Errorf("inverse predicate %q points to %q, not %q",
			meta.InverseOf, inverse.InverseOf, meta.Name)
	}
	return nil
}
