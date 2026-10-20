package graphmutation

import (
	"slices"
	"strings"

	"github.com/c360studio/semengine/component"
)

// ClassifyPort decides, for one component port, whether it is a graph-mutation
// port (member) and whether it has the one valid shape (valid). required is the
// port's Required flag, which PortFacts does not carry.
//
// A port is a member when its interface type is InterfaceType, or when any one
// of its NATS subjects starts with the family's prefix, graph.mutation., whatever
// the port's kind (owner ruling, #323 comment 6279624359). A member is valid
// when it is a required nats-request port whose subjects are exactly
// SubjectFamily and whose interface is InterfaceType at InterfaceVersion. valid
// implies member.
//
// Flowgraph and graph-ingest both call it; each keeps its own direction rule,
// provider count and refusal text.
func ClassifyPort(facts component.PortFacts, required bool) (member, valid bool) {
	prefix := strings.TrimSuffix(SubjectFamily, ">")
	subjects := facts.NATSSubjects()
	contract, typed := facts.Interface()
	member = (typed && contract.Type == InterfaceType) ||
		slices.ContainsFunc(subjects, func(subject string) bool { return strings.HasPrefix(subject, prefix) })
	if !member {
		return false, false
	}
	valid = required && facts.Kind() == component.PortKindNATSRequest &&
		slices.Equal(subjects, []string{SubjectFamily}) &&
		typed && contract.Type == InterfaceType && contract.Version == InterfaceVersion
	return true, valid
}
