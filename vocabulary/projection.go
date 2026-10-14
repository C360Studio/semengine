package vocabulary

import (
	"fmt"
	"strings"
	"unicode"
)

// ProjectionStep describes one hop in a reified ontology projection chain
// (such as a BFO/CCO measurement or act chain) from an entity's flat triple
// to an expanded RDF/SPARQL graph pattern.
type ProjectionStep struct {
	// PredicateIRI is the object-property IRI traversed from the current subject
	// (the entity on step 0, or the previous intermediate node on step > 0).
	PredicateIRI string `json:"predicate_iri" yaml:"predicate_iri"`

	// NodeClassIRI is the optional RDF/OWL class IRI of the intermediate node
	// materialized by this step (for example, cco:ActOfMeasuring). Required on
	// every non-terminal step in a multi-step chain.
	NodeClassIRI string `json:"node_class_iri,omitempty" yaml:"node_class_iri,omitempty"`

	// ValuePredicateIRI is the optional datatype- or object-property IRI that
	// attaches the triple's terminal value to the node reached at the final step
	// (for example, cco:has_decimal_value).
	ValuePredicateIRI string `json:"value_predicate_iri,omitempty" yaml:"value_predicate_iri,omitempty"`
}

// ProjectionSpec declares an ordered sequence of reification steps that
// external projection adapters (such as semsparql) use to expand a flat
// SemEngine triple into a multi-hop ontology pattern without storing blank
// nodes in ENTITY_STATES.
type ProjectionSpec struct {
	Steps []ProjectionStep `json:"steps" yaml:"steps"`
}

// WithProjection attaches a reified ontology projection specification to a
// predicate registration. Passing an empty Steps slice or a malformed step
// panics at registration.
func WithProjection(spec ProjectionSpec) Option {
	return func(m *PredicateMetadata) {
		cloned := cloneProjectionSpec(&spec)
		if cloned == nil {
			cloned = &ProjectionSpec{}
		}
		m.Projection = cloned
	}
}

func cloneProjectionSpec(spec *ProjectionSpec) *ProjectionSpec {
	if spec == nil {
		return nil
	}
	steps := make([]ProjectionStep, len(spec.Steps))
	copy(steps, spec.Steps)
	return &ProjectionSpec{Steps: steps}
}

func validateProjectionSpec(spec *ProjectionSpec) error {
	if spec == nil {
		return nil
	}
	if len(spec.Steps) == 0 {
		return fmt.Errorf("projection must contain at least one step")
	}
	for i, step := range spec.Steps {
		if err := validateNonEmptyIRI(step.PredicateIRI, fmt.Sprintf("projection step %d predicate_iri", i+1)); err != nil {
			return err
		}
		if step.NodeClassIRI != "" {
			if err := validateNonEmptyIRI(step.NodeClassIRI, fmt.Sprintf("projection step %d node_class_iri", i+1)); err != nil {
				return err
			}
		}
		if step.ValuePredicateIRI != "" {
			if err := validateNonEmptyIRI(step.ValuePredicateIRI, fmt.Sprintf("projection step %d value_predicate_iri", i+1)); err != nil {
				return err
			}
		}
		if i < len(spec.Steps)-1 {
			if step.NodeClassIRI == "" {
				return fmt.Errorf("projection step %d requires node_class_iri for intermediate hop", i+1)
			}
			if step.ValuePredicateIRI != "" {
				return fmt.Errorf("projection step %d cannot set value_predicate_iri on non-terminal hop", i+1)
			}
		}
	}
	return nil
}

func validateNonEmptyIRI(iri, field string) error {
	if iri == "" || strings.TrimSpace(iri) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if strings.IndexFunc(iri, unicode.IsSpace) >= 0 {
		return fmt.Errorf("%s %q must not contain whitespace", field, iri)
	}
	return nil
}
