// Package composition validates a component composition — the boot
// configuration's components composed with the catalog a binary registers —
// as a pure function, and projects it as a graph.
//
// Two evidence classes share one interpreter (ADR-100):
//
//   - Validate(catalog, cfg) is the offline judgment: each factory's static
//     port declarer (component.Registration.Ports) predicts the ports the next
//     boot would admit, with no NATS, no process, and no construction. Two
//     components whose exclusive claims clash are judged by
//     component.Declaration.ConflictWith, the rule boot admission applies.
//   - Analyze(declarations, streams) is the graph-level half of the same
//     function over declarations and the configuration's explicit streams.
//     The boot caller is service's component manager, not yet ported: it will
//     run Analyze over what was admitted and refuse to boot on an
//     error-severity finding (ADR-100 decision 3).
//
// Findings carry a closed vocabulary (the Type* constants), one severity table
// (severityOf), non-nil arrays, and a deterministic order, so two runs over
// equal inputs marshal to byte-equal JSON. A product checks a composition in
// its own tests by calling Validate and requiring an empty Errors list.
package composition
