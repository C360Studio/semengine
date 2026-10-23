package composition

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/flowgraph"
	"github.com/c360studio/semengine/internal/graphmutation"
)

// Analyze is the graph-level half of composition validation: it connects the
// declarations, analyzes connectivity, stream requirements, and interface
// contracts, and projects the graph. Validate runs it over declared
// components; service's component manager, when it is ported, runs it at boot
// over the admitted ones.
//
// streams is the configuration's explicit `streams` block (nil when it has
// none). Stream provisioning derives streams from JetStream OUTPUT ports and
// from this block; a JetStream subscriber whose subjects an explicit stream
// covers is fed even when its publishers use core NATS, so it is not a
// stream_requirement finding.
func Analyze(declarations []component.Declaration, streams config.StreamConfigs) *Result {
	result := newResult()
	sorted := append([]component.Declaration(nil), declarations...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].InstanceName < sorted[j].InstanceName })
	// The reserved-subject check reads only streams and port facts, so it runs
	// before the early returns: an explicit stream is refused even when no
	// component is enabled or the graph cannot be built.
	for _, finding := range reservedCaptureFindings(sorted, streams) {
		result.add(finding)
	}
	// The subject grammar check also reads only streams and port facts, and
	// runs before the early returns for the same reason.
	for _, finding := range malformedSubjectFindings(sorted, streams) {
		result.add(finding)
	}

	if len(sorted) == 0 {
		result.add(Finding{
			Type: TypeEmptyComposition, Severity: severityOf(TypeEmptyComposition, nil),
			Component: "(none)",
			Message:   "composition declares no enabled components",
			Suggestions: []string{
				"Add at least one enabled component to the configuration",
				"A services-only process boots; this is a warning, not a refusal",
			},
		})
		result.finalize()
		return result
	}

	graph, err := flowgraph.BuildFromDeclarations(sorted)
	if err != nil {
		result.add(Finding{
			Type: TypeConnectionPatternError, Severity: severityOf(TypeConnectionPatternError, nil),
			Component: "(composition)",
			Message:   err.Error(),
			Suggestions: []string{
				"Let exactly one component write each KV bucket",
				"Declare graph-mutation ports with the canonical required nats-request shape",
			},
		})
		result.Graph = graphOf(sorted, nil)
		result.finalize()
		return result
	}

	analysis := graph.AnalyzeConnectivity()
	external := externalInputs(sorted)
	for _, node := range analysis.DisconnectedNodes {
		result.add(Finding{
			Type: TypeDisconnectedNode, Severity: severityOf(TypeDisconnectedNode, nil),
			Component:   node.ComponentName,
			Message:     node.Issue,
			Suggestions: append([]string{}, node.Suggestions...),
		})
	}
	for _, port := range analysis.OrphanedPorts {
		port := port
		if port.Issue == flowgraph.IssueNoPublishers && external[portKey{port.ComponentName, port.PortName}] {
			// The operator declared this input fed from outside the composition
			// (PortDefinition.External): no in-graph publisher is expected, so
			// the no-publisher orphan is not a finding. Every other finding on
			// the port — stream requirement, interface contracts — is unaffected.
			continue
		}
		result.add(orphanedPortFinding(port))
	}
	streamNames := subscriberStreamNames(sorted)
	for _, warning := range graph.ValidateStreamRequirements() {
		if explicitStreamCovers(streams, streamNames[portKey{warning.SubscriberComp, warning.SubscriberPort}], warning.Subjects) {
			continue // the subscriber's named stream is declared and captures its subjects; it is fed
		}
		publishers := append([]string(nil), warning.PublisherComps...)
		sort.Strings(publishers)
		result.add(Finding{
			Type: TypeStreamRequirement, Severity: severityOf(TypeStreamRequirement, nil),
			Component: warning.SubscriberComp,
			Port:      warning.SubscriberPort,
			Message: fmt.Sprintf(
				"JetStream subscriber expects a stream for subjects %v but its publishers [%s] use core NATS (no stream will be created)",
				warning.Subjects, strings.Join(publishers, ", ")),
			Suggestions: []string{
				"Publish through a jetstream output port on: " + strings.Join(publishers, ", "),
				"Or subscribe through a nats input port if durability is not required",
			},
		})
	}
	for _, finding := range interfaceFindings(sorted, graph) {
		result.add(finding)
	}
	result.Graph = graphOf(sorted, graph)
	result.finalize()
	return result
}

// reservedSubjects returns the request subjects no stream may capture: the
// subjects of graph's query verbs in table order, then the graph mutation
// family. Both are read from their declarations, never spelled here.
func reservedSubjects() []string {
	var reserved []string
	for _, verb := range graph.QueryVerbs() {
		reserved = append(reserved, verb.Subject)
	}
	return append(reserved, graphmutation.SubjectFamily)
}

// reservedCaptureFindings reports each stream subject filter that overlaps a
// reserved request subject (#16): a stream on such a filter would store the
// requests, which belong to their responders alone. The streams checked are
// the ones provisioning creates from the composition: the explicit block,
// named by key, and each JetStream output, named as provisioning names it (its
// stream name, else config.DeriveStreamName of its first subject). One finding
// per filter names the first reserved subject it captures.
func reservedCaptureFindings(declarations []component.Declaration, streams config.StreamConfigs) []Finding {
	reserved := reservedSubjects()
	var findings []Finding
	check := func(owner, port, stream string, filters []string) {
		for _, filter := range filters {
			for _, subject := range reserved {
				if !flowgraph.SubjectsOverlap(filter, subject) {
					continue
				}
				findings = append(findings, Finding{
					Type: TypeReservedSubjectCapture, Severity: severityOf(TypeReservedSubjectCapture, nil),
					Component: owner,
					Port:      port,
					Message: fmt.Sprintf("stream %q subject filter %q captures reserved request subject %q",
						stream, filter, subject),
					Suggestions: []string{
						"Narrow the stream's subjects so they share no subject with a graph request subject",
						"Graph request subjects belong to their responders; no stream may store them",
					},
				})
				break
			}
		}
	}
	for name, stream := range streams {
		check("config", "", name, stream.Subjects)
	}
	for _, declaration := range declarations {
		for index, port := range declaration.OutputPorts {
			if index >= len(declaration.OutputFacts) {
				continue
			}
			stream, ok := declaration.OutputFacts[index].Stream()
			if !ok {
				continue
			}
			subjects := stream.Subjects()
			name := stream.Name()
			if name == "" && len(subjects) > 0 {
				name = config.DeriveStreamName(subjects[0])
			}
			check(declaration.InstanceName, port.Name, name, subjects)
		}
	}
	return findings
}

// malformedSubjectFindings reports each subject that fails the subject
// grammar (#369): every subject of every NATS, NATS-request and JetStream
// port, in both lanes, and of every explicit stream. A stream derived from a
// JetStream output holds that port's subjects, so it is not checked again.
// One finding per subject names where it is declared: the lane, port and
// field for a port, the stream key and field for an explicit stream.
func malformedSubjectFindings(declarations []component.Declaration, streams config.StreamConfigs) []Finding {
	var findings []Finding
	check := func(owner, port, where, field, subject string) {
		if flowgraph.ValidSubject(subject) {
			return
		}
		findings = append(findings, Finding{
			Type: TypeMalformedSubject, Severity: severityOf(TypeMalformedSubject, nil),
			Component: owner,
			Port:      port,
			Message:   fmt.Sprintf("%s field %q: subject %q is not a valid subject", where, field, subject),
			Suggestions: []string{
				"Write the subject as '.'-separated tokens, none empty, with no space, tab, line feed, carriage return or form feed",
				"Use '*' and '>' only as whole tokens, '>' only last; a token such as 'a*' is refused here although NATS reads it as a literal",
			},
		})
	}
	for _, name := range slices.Sorted(maps.Keys(streams)) {
		for index, subject := range streams[name].Subjects {
			check("config", "", fmt.Sprintf("stream %q", name), fmt.Sprintf("subjects[%d]", index), subject)
		}
	}
	checkLane := func(declaration component.Declaration, lane string, ports []component.Port, facts []component.PortFacts) {
		for index, port := range ports {
			if index >= len(facts) {
				continue
			}
			where := fmt.Sprintf("%s port %q", lane, port.Name)
			switch facts[index].Kind() {
			case component.PortKindNATS, component.PortKindNATSRequest:
				for _, subject := range facts[index].NATSSubjects() {
					check(declaration.InstanceName, port.Name, where, "subject", subject)
				}
			case component.PortKindJetStream:
				for i, subject := range facts[index].NATSSubjects() {
					check(declaration.InstanceName, port.Name, where, fmt.Sprintf("subjects[%d]", i), subject)
				}
			}
		}
	}
	for _, declaration := range declarations {
		checkLane(declaration, "input", declaration.InputPorts, declaration.InputFacts)
		checkLane(declaration, "output", declaration.OutputPorts, declaration.OutputFacts)
	}
	return findings
}

// explicitStreamCovers reports whether the subscriber is fed by an explicit
// stream declaration: a JetStream consumer binds to its declared stream BY
// NAME, and provisioning creates only explicit and JetStream-output-derived
// streams, so the explicit block must declare exactly that name AND one of
// its subjects must COVER (not merely overlap) each subscriber subject. An
// empty subject list is not covered: nothing a stream could feed.
func explicitStreamCovers(streams config.StreamConfigs, streamName string, subjects []string) bool {
	if len(subjects) == 0 || streamName == "" {
		return false
	}
	stream, declared := streams[streamName]
	if !declared {
		return false
	}
	for _, subject := range subjects {
		covered := false
		for _, filter := range stream.Subjects {
			if flowgraph.SubjectCovers(filter, subject) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

type portKey struct{ instance, port string }

// subscriberStreamNames indexes every JetStream input by the stream name it
// binds to (StreamFacts.Name, required on inputs by the port grammar).
func subscriberStreamNames(declarations []component.Declaration) map[portKey]string {
	names := map[portKey]string{}
	for _, declaration := range declarations {
		for index, port := range declaration.InputPorts {
			if index >= len(declaration.InputFacts) {
				continue
			}
			if stream, ok := declaration.InputFacts[index].Stream(); ok {
				names[portKey{declaration.InstanceName, port.Name}] = stream.Name()
			}
		}
	}
	return names
}

// externalInputs indexes the inputs declared fed from outside the composition.
func externalInputs(declarations []component.Declaration) map[portKey]bool {
	external := map[portKey]bool{}
	for _, declaration := range declarations {
		for _, port := range declaration.InputPorts {
			if port.External {
				external[portKey{declaration.InstanceName, port.Name}] = true
			}
		}
	}
	return external
}

func orphanedPortFinding(port flowgraph.OrphanedPort) Finding {
	severity := severityOf(TypeOrphanedPort, &port)
	var suggestions []string
	switch port.Issue {
	case flowgraph.IssueNoPublishers:
		if severity == SeverityError {
			suggestions = []string{
				"Connect an output from another component",
				"Check that source component is configured correctly",
				"If this input is fed from outside the composition, declare \"external\": true on the port " +
					"(a named override replaces the whole port, so restate it there)",
			}
		} else {
			suggestions = []string{"This port is optional and can remain unconnected"}
		}
	case flowgraph.IssueNoSubscribers:
		if port.Required && port.Pattern == component.PatternStream {
			suggestions = []string{
				"Consider connecting to a processor or output component",
				fmt.Sprintf("Data will be published to %s but not consumed", port.ConnectionID),
			}
		} else {
			suggestions = []string{"This port is optional and can remain unconnected"}
		}
	case flowgraph.IssueOptionalAPIUnused:
		suggestions = []string{"This API port is optional"}
	case flowgraph.IssueOptionalIndexUnwatched:
		suggestions = []string{"KV index ports are optional observation points"}
	default:
		suggestions = []string{}
	}
	return Finding{
		Type: TypeOrphanedPort, Severity: severity,
		Component:   port.ComponentName,
		Port:        port.PortName,
		Message:     fmt.Sprintf("%s port '%s' (%s): %s", port.Direction, port.PortName, port.Pattern, port.Issue),
		Suggestions: suggestions,
	}
}

// interfaceFindings checks the interface contract on every derived edge: two
// declared types that are equal are compatible, two that differ are an error,
// and a source without a contract feeding a target that requires one is a
// warning.
func interfaceFindings(declarations []component.Declaration, graph *flowgraph.FlowGraph) []Finding {
	// Port (instance, name and direction) → interface type. A component may
	// name an input and an output alike, so the direction keeps the two
	// lanes' contracts apart (#346).
	contracts := make(map[flowgraph.ComponentPortRef]string)
	for _, declaration := range declarations {
		for index, port := range declaration.InputPorts {
			contracts[flowgraph.ComponentPortRef{
				ComponentName: declaration.InstanceName, PortName: port.Name, Direction: component.DirectionInput,
			}] = interfaceType(declaration.InputFacts, index)
		}
		for index, port := range declaration.OutputPorts {
			contracts[flowgraph.ComponentPortRef{
				ComponentName: declaration.InstanceName, PortName: port.Name, Direction: component.DirectionOutput,
			}] = interfaceType(declaration.OutputFacts, index)
		}
	}

	var findings []Finding
	for _, edge := range graph.GetEdges() {
		sourceType, ok := contracts[edge.From]
		if !ok {
			continue
		}
		targetType, ok := contracts[edge.To]
		if !ok {
			continue
		}
		componentPair := fmt.Sprintf("%s → %s", edge.From.ComponentName, edge.To.ComponentName)
		portPair := fmt.Sprintf("%s → %s", edge.From.PortName, edge.To.PortName)
		switch {
		case targetType != "" && sourceType != "" && sourceType != targetType:
			findings = append(findings, Finding{
				Type: TypeInterfaceMismatch, Severity: severityOf(TypeInterfaceMismatch, nil),
				Component: componentPair, Port: portPair,
				Message: fmt.Sprintf(
					"Interface mismatch: source port '%s' provides '%s' but target port '%s' requires '%s'",
					edge.From.PortName, sourceType, edge.To.PortName, targetType),
				Suggestions: []string{
					"Check that connected components have compatible interfaces",
					"Verify port interface contracts in component documentation",
					fmt.Sprintf("Source provides: %s", sourceType),
					fmt.Sprintf("Target requires: %s", targetType),
				},
			})
		case targetType != "" && sourceType == "":
			findings = append(findings, Finding{
				Type: TypeMissingInterface, Severity: severityOf(TypeMissingInterface, nil),
				Component: componentPair, Port: portPair,
				Message: fmt.Sprintf(
					"Source port '%s' does not declare an interface, but target port '%s' requires '%s'",
					edge.From.PortName, edge.To.PortName, targetType),
				Suggestions: []string{
					"Verify that source component produces compatible data",
					"Check component documentation for interface contracts",
					fmt.Sprintf("Target requires: %s", targetType),
				},
			})
		}
	}
	return findings
}

func interfaceType(facts []component.PortFacts, index int) string {
	if index >= len(facts) {
		return ""
	}
	if contract, ok := facts[index].Interface(); ok {
		return contract.Type
	}
	return ""
}
