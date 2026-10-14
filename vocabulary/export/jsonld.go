package export

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/c360studio/semengine/message"
)

// writeJSONLD writes triples as a JSON-LD document with @context and @graph.
func writeJSONLD(w io.Writer, triples []message.Triple, opts *options) error {
	pm := newPrefixMap(opts.baseIRI)
	groups, subjectOrder := groupTriples(triples, opts)

	scanPrefixes(groups, pm)

	graph := buildJSONLDGraph(groups, subjectOrder, pm)
	doc := assembleJSONLDDocument(graph, pm)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("JSON-LD encoding: %w", err)
	}
	return nil
}

// buildJSONLDGraph constructs the @graph array of node objects.
func buildJSONLDGraph(groups map[string][]resolvedTriple, subjectOrder []string, pm *prefixMap) []map[string]any {
	graph := make([]map[string]any, 0, len(subjectOrder))
	for _, subIRI := range subjectOrder {
		node := map[string]any{"@id": subIRI}

		rts := groups[subIRI]
		sort.Slice(rts, func(a, b int) bool {
			return rts[a].predicateIRI < rts[b].predicateIRI
		})

		for _, rt := range rts {
			predKey := rt.predicateIRI
			if compacted, ok := pm.compact(predKey); ok {
				predKey = compacted
			}

			val := jsonldObjectValue(rt.obj, pm)

			if existing, exists := node[predKey]; exists {
				switch ev := existing.(type) {
				case []any:
					node[predKey] = append(ev, val)
				default:
					node[predKey] = []any{ev, val}
				}
			} else {
				node[predKey] = val
			}
		}

		graph = append(graph, node)
	}
	return graph
}

// assembleJSONLDDocument creates the final document with @context and optional @graph.
func assembleJSONLDDocument(graph []map[string]any, pm *prefixMap) map[string]any {
	doc := map[string]any{}

	context := pm.usedPrefixMap()
	if len(context) > 0 {
		doc["@context"] = context
	}

	if len(graph) == 1 {
		for k, v := range graph[0] {
			doc[k] = v
		}
	} else if len(graph) > 1 {
		doc["@graph"] = graph
	}
	return doc
}

// jsonldObjectValue converts a classifiedObject to its JSON-LD representation.
func jsonldObjectValue(obj classifiedObject, pm *prefixMap) any {
	switch obj.kind {
	case objectResource:
		return map[string]any{"@id": obj.iri}
	case objectLiteral:
		return jsonldLiteralValue(obj, pm)
	default:
		return nil
	}
}

// jsonldLiteralValue formats a literal value for JSON-LD.
// Uses native JSON types where possible, typed values with @value/@type otherwise.
func jsonldLiteralValue(obj classifiedObject, pm *prefixMap) any {
	switch obj.datatype {
	case xsdBoolean:
		return obj.lexical == "true"
	case xsdInteger:
		return json.Number(obj.lexical)
	case xsdDouble:
		return json.Number(obj.lexical)
	case "", xsdString:
		return obj.lexical
	default:
		typed := map[string]any{"@value": obj.lexical}
		if compacted, ok := pm.compact(obj.datatype); ok {
			typed["@type"] = compacted
		} else {
			typed["@type"] = obj.datatype
		}
		return typed
	}
}
