package export

import (
	"fmt"
	"io"

	"github.com/c360studio/semengine/message"
)

// writeNTriples writes triples in N-Triples format.
// Each triple is on its own line: <subject> <predicate> <object> .
// All IRIs are fully expanded (no prefixes).
func writeNTriples(w io.Writer, triples []message.Triple, opts *options) error {
	for _, t := range triples {
		if t.Subject == "" || t.Predicate == "" {
			continue
		}

		obj := classifyObject(t, opts)
		if obj.kind == objectInvalid {
			continue
		}

		subjectIRI := resolveSubjectIRI(t.Subject, opts)
		predicateIRI := resolvePredicateIRI(t.Predicate, opts.baseIRI)

		subjectStr := formatNTriplesSubject(subjectIRI)
		objStr := formatNTriplesObject(obj)

		if _, err := fmt.Fprintf(w, "%s <%s> %s .\n", subjectStr, predicateIRI, objStr); err != nil {
			return err
		}
	}
	return nil
}

func formatNTriplesSubject(subjectIRI string) string {
	if isValidBlankNodeLabel(subjectIRI) {
		return subjectIRI
	}
	return fmt.Sprintf("<%s>", subjectIRI)
}

// formatNTriplesObject formats a classified object for N-Triples output.
// N-Triples always uses full IRIs and explicit datatypes.
func formatNTriplesObject(obj classifiedObject) string {
	switch obj.kind {
	case objectResource:
		return fmt.Sprintf("<%s>", obj.iri)
	case objectBlankNode:
		return obj.iri
	case objectLiteral:
		escaped := escapeTurtleString(obj.lexical)
		if obj.datatype == "" || obj.datatype == xsdString {
			return fmt.Sprintf(`"%s"`, escaped)
		}
		return fmt.Sprintf(`"%s"^^<%s>`, escaped, obj.datatype)
	default:
		return `""`
	}
}
