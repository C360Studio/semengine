package export

import (
	"fmt"
	"io"
	"sort"

	"github.com/c360studio/semengine/message"
)

// writeTurtle writes triples in Turtle format with prefix declarations
// and subject grouping (multiple predicates for the same subject use ; separator).
func writeTurtle(w io.Writer, triples []message.Triple, opts *options) error {
	pm := newPrefixMap(opts.baseIRI)
	groups, subjectOrder := groupTriples(triples, opts)

	if len(groups) == 0 {
		return nil
	}

	// Determine which prefixes are used by compacting all IRIs
	scanPrefixes(groups, pm)

	if err := writeTurtlePrefixes(w, pm); err != nil {
		return err
	}

	return writeTurtleBody(w, groups, subjectOrder, pm)
}

// writeTurtlePrefixes writes @prefix declarations for used prefixes.
func writeTurtlePrefixes(w io.Writer, pm *prefixMap) error {
	usedPrefixes := pm.usedPrefixes()
	for _, prefix := range usedPrefixes {
		ns := pm.namespaceFor(prefix)
		if _, err := fmt.Fprintf(w, "@prefix %s: <%s> .\n", prefix, ns); err != nil {
			return err
		}
	}
	if len(usedPrefixes) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

// writeTurtleBody writes the triple groups as Turtle statements.
func writeTurtleBody(w io.Writer, groups map[string][]resolvedTriple, subjectOrder []string, pm *prefixMap) error {
	for i, subIRI := range subjectOrder {
		rts := groups[subIRI]
		subjectStr := formatTurtleTerm(subIRI, pm)

		sort.Slice(rts, func(a, b int) bool {
			return rts[a].predicateIRI < rts[b].predicateIRI
		})

		for j, rt := range rts {
			predStr := formatTurtleTerm(rt.predicateIRI, pm)
			objStr := formatTurtleObject(rt.obj, pm)

			if j == 0 {
				if _, err := fmt.Fprintf(w, "%s %s %s", subjectStr, predStr, objStr); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, " ;\n    %s %s", predStr, objStr); err != nil {
					return err
				}
			}
		}

		if _, err := fmt.Fprint(w, " .\n"); err != nil {
			return err
		}
		if i < len(subjectOrder)-1 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	return nil
}

// formatTurtleTerm formats an IRI (or blank-node label) for Turtle, compacting
// with prefix if possible.
func formatTurtleTerm(iri string, pm *prefixMap) string {
	if isValidBlankNodeLabel(iri) {
		return iri
	}
	if compacted, ok := pm.compact(iri); ok {
		return compacted
	}
	return "<" + iri + ">"
}

// formatTurtleObject formats a classified object for Turtle output.
func formatTurtleObject(obj classifiedObject, pm *prefixMap) string {
	switch obj.kind {
	case objectResource:
		return formatTurtleTerm(obj.iri, pm)
	case objectBlankNode:
		return obj.iri
	case objectLiteral:
		return formatTurtleLiteral(obj, pm)
	default:
		return `""`
	}
}

// formatTurtleLiteral formats a literal value in Turtle syntax.
func formatTurtleLiteral(obj classifiedObject, pm *prefixMap) string {
	if obj.bare {
		return obj.lexical
	}

	escaped := escapeTurtleString(obj.lexical)

	if obj.datatype == "" || obj.datatype == xsdString {
		return fmt.Sprintf(`"%s"`, escaped)
	}

	dtStr := formatTurtleTerm(obj.datatype, pm)
	return fmt.Sprintf(`"%s"^^%s`, escaped, dtStr)
}
