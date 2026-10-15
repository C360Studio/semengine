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

	// ValuePredicateIRI is the datatype- or object-property IRI that attaches
	// the triple's terminal value to the node reached at the final step (for
	// example, cco:has_decimal_value). Required on the terminal step and
	// forbidden on non-terminal steps.
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

// Validate checks that Steps is non-empty, every step carries a valid http://
// or https:// PredicateIRI, any non-empty NodeClassIRI is a valid http:// or
// https:// IRI (and is present on every non-terminal step), and
// ValuePredicateIRI is absent on non-terminal steps and set to a valid http://
// or https:// IRI on the terminal step. Validate does not read or mutate global
// registry state.
func (s ProjectionSpec) Validate() error {
	if len(s.Steps) == 0 {
		return fmt.Errorf("projection must contain at least one step")
	}
	for i, step := range s.Steps {
		if err := validateHTTPIRI(step.PredicateIRI, fmt.Sprintf("projection step %d predicate_iri", i+1)); err != nil {
			return err
		}
		if step.NodeClassIRI != "" {
			if err := validateHTTPIRI(step.NodeClassIRI, fmt.Sprintf("projection step %d node_class_iri", i+1)); err != nil {
				return err
			}
		}
		if step.ValuePredicateIRI != "" {
			if err := validateHTTPIRI(step.ValuePredicateIRI, fmt.Sprintf("projection step %d value_predicate_iri", i+1)); err != nil {
				return err
			}
		}
		if i < len(s.Steps)-1 {
			if step.NodeClassIRI == "" {
				return fmt.Errorf("projection step %d requires node_class_iri for intermediate hop", i+1)
			}
			if step.ValuePredicateIRI != "" {
				return fmt.Errorf("projection step %d cannot set value_predicate_iri on non-terminal hop", i+1)
			}
		} else if step.ValuePredicateIRI == "" {
			return fmt.Errorf("projection step %d requires value_predicate_iri on terminal hop", i+1)
		}
	}
	return nil
}

func validateProjectionSpec(spec *ProjectionSpec) error {
	if spec == nil {
		return nil
	}
	return spec.Validate()
}

func validateHTTPIRI(iri, field string) error {
	if err := validateNonEmptyIRI(iri, field); err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(iri, "http://"):
		if len(iri) <= len("http://") {
			return fmt.Errorf("%s %q must use http:// or https:// scheme with a non-empty authority/path", field, iri)
		}
	case strings.HasPrefix(iri, "https://"):
		if len(iri) <= len("https://") {
			return fmt.Errorf("%s %q must use http:// or https:// scheme with a non-empty authority/path", field, iri)
		}
	default:
		return fmt.Errorf("%s %q must use http:// or https:// scheme", field, iri)
	}
	return nil
}

func validateNonEmptyIRI(iri, field string) error {
	if iri == "" || strings.TrimSpace(iri) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	for _, r := range iri {
		if r <= 0x20 || unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("%s %q must not contain whitespace or control characters", field, iri)
		}
		if strings.ContainsRune("<>\"{}|^`\\", r) {
			return fmt.Errorf("%s %q contains invalid Turtle IRIREF character %q", field, iri, r)
		}
	}
	colon := strings.IndexByte(iri, ':')
	if colon <= 0 || colon == len(iri)-1 {
		return fmt.Errorf("%s %q must be an absolute IRI with a scheme", field, iri)
	}
	for i := 0; i < colon; i++ {
		b := iri[i]
		isAlpha := (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
		isDigit := b >= '0' && b <= '9'
		if i == 0 && !isAlpha {
			return fmt.Errorf("%s %q has invalid IRI scheme", field, iri)
		}
		if i > 0 && !isAlpha && !isDigit && b != '+' && b != '-' && b != '.' {
			return fmt.Errorf("%s %q has invalid IRI scheme", field, iri)
		}
	}
	return nil
}
