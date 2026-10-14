// Package export serializes []message.Triple to standard RDF formats.
//
// Supported formats:
//   - Turtle (.ttl) — compact, human-readable with prefix declarations and subject grouping
//   - N-Triples (.nt) — line-based, one triple per line, no abbreviations
//   - JSON-LD (.jsonld) — JSON with @context and @graph for web APIs
//
// The package uses the existing vocabulary infrastructure for IRI resolution:
//   - Registered predicates map to their StandardIRI via [vocabulary.GetPredicateMetadata]
//   - Entity ID subjects are parsed via [message.ParseEntityID] and converted to hierarchical IRIs
//   - Object values are typed using XSD datatypes based on Go type inference
//
// Basic usage:
//
//	triples := []message.Triple{
//	    {Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: 85.5},
//	    {Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.flight.armed", Object: true},
//	}
//
//	// Write Turtle to stdout
//	err := export.Serialize(os.Stdout, triples, export.Turtle)
//
//	// Get N-Triples as a string
//	s, err := export.SerializeToString(triples, export.NTriples)
//
//	// Custom base IRI
//	err = export.Serialize(w, triples, export.JSONLD, export.WithBaseIRI("https://example.org"))
package export
