package vocabulary

import (
	"fmt"
	"strings"
)

// PredicateNamespace is an exact vocabulary namespace delegation. Category is
// empty for a domain-wide delegation and non-empty for a domain.category
// delegation. Wildcards and property-level namespaces are not supported.
type PredicateNamespace struct {
	Domain   string
	Category string
}

// String returns the exact delegated namespace.
func (n PredicateNamespace) String() string {
	if n.Category == "" {
		return n.Domain
	}
	return n.Domain + "." + n.Category
}

// RequireDeclaredPredicate enforces the v1 first-party authoring policy. The
// predicate must be canonical and registered by a trusted composition-root or
// vocabulary package before a configuration, workflow, ownership claim, or
// generated mutation surface may expose it. Runtime graph persistence uses the
// syntax-only final-candidate seam instead; caller-controlled fields never
// manufacture declaration authority.
func RequireDeclaredPredicate(predicate string) error {
	if _, err := ParsePredicate(predicate); err != nil {
		return err
	}
	if GetPredicateMetadata(predicate) == nil {
		return fmt.Errorf("predicate %q is canonical but not declared in the vocabulary registry", predicate)
	}
	return nil
}

// ParsePredicateNamespace validates an exact domain or domain.category
// namespace using the same lower-kebab segment grammar and byte bound as stored
// predicates.
func ParsePredicateNamespace(namespace string) (PredicateNamespace, error) {
	segments := strings.Split(namespace, ".")
	if len(segments) != 1 && len(segments) != 2 {
		return PredicateNamespace{}, fmt.Errorf("predicate namespace %q must be an exact domain or domain.category", namespace)
	}
	for i, segment := range segments {
		if err := validatePredicateSegment(namespace, segment, i); err != nil {
			return PredicateNamespace{}, fmt.Errorf("predicate namespace %q is invalid: %w", namespace, err)
		}
	}

	parsed := PredicateNamespace{Domain: segments[0]}
	if len(segments) == 2 {
		parsed.Category = segments[1]
	}
	return parsed, nil
}
