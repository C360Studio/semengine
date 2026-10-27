package composition_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/flowgraph"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/types"
)

// TestValidateRefusesStreamCapturingReservedSubject holds the composition share of #16: a stream
// whose subject filter shares a concrete subject with a request subject graph-ingest serves, or
// with the graph mutation family, would store those requests, so the composition is refused. The
// expected messages are written out here, not built from the implementation's format.
func TestValidateRefusesStreamCapturingReservedSubject(t *testing.T) {
	registry := fakeRegistry(t,
		fakeSpec{name: "crossing", typ: "output", outputs: []component.PortDefinition{jetStreamOut("out", "", "*.mutation.create")}},
		fakeSpec{name: "batching", typ: "output", outputs: []component.PortDefinition{jetStreamOut("out", "EVENTS", "graph.ingest.query.batch")}},
		fakeSpec{name: "quiet", typ: "output", outputs: []component.PortDefinition{jetStreamOut("out", "", "quiet.events")}},
	)
	for _, tc := range []struct {
		name       string
		components config.ComponentConfigs
		streams    config.StreamConfigs
		want       composition.Finding
	}{
		{
			name:    "explicit wide filter",
			streams: config.StreamConfigs{"WIDE": {Subjects: []string{"graph.>"}}},
			want: composition.Finding{Component: "config",
				Message: `stream "WIDE" subject filter "graph.>" captures reserved request subject "graph.ingest.query.entity"`},
		},
		{
			name:    "explicit exact filter",
			streams: config.StreamConfigs{"EXACT": {Subjects: []string{"graph.ingest.query.entity"}}},
			want: composition.Finding{Component: "config",
				Message: `stream "EXACT" subject filter "graph.ingest.query.entity" captures reserved request subject "graph.ingest.query.entity"`},
		},
		{
			// Overlap, not cover: neither pattern covers the other, yet graph.mutation.create
			// matches both. The stream is unnamed, so it is named as provisioning names it.
			name:       "JetStream output crossing filter",
			components: config.ComponentConfigs{"cross": instance("crossing", types.ComponentTypeOutput)},
			want: composition.Finding{Component: "cross", Port: "out",
				Message: `stream "MUTATION" subject filter "*.mutation.create" captures reserved request subject "graph.mutation.>"`},
		},
		{
			name:       "JetStream output with a named stream",
			components: config.ComponentConfigs{"batch": instance("batching", types.ComponentTypeOutput)},
			want: composition.Finding{Component: "batch", Port: "out",
				Message: `stream "EVENTS" subject filter "graph.ingest.query.batch" captures reserved request subject "graph.ingest.query.batch"`},
		},
		{
			// The explicit block is checked beside components that capture nothing.
			name:       "explicit filter beside a clear component",
			components: config.ComponentConfigs{"q": instance("quiet", types.ComponentTypeOutput)},
			streams:    config.StreamConfigs{"MUT": {Subjects: []string{"quiet.>", "graph.mutation.*.create"}}},
			want: composition.Finding{Component: "config",
				Message: `stream "MUT" subject filter "graph.mutation.*.create" captures reserved request subject "graph.mutation.>"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			components := tc.components
			if components == nil {
				components = config.ComponentConfigs{}
			}
			cfg := compositionOf(components)
			cfg.Streams = tc.streams
			result := validateRoundTrip(t, registry, cfg)
			if warnings := findingsOfType(result.Warnings, composition.TypeReservedSubjectCapture); len(warnings) != 0 {
				t.Fatalf("reserved_subject_capture reported as a warning: %+v", warnings)
			}
			got := findingsOfType(result.Errors, composition.TypeReservedSubjectCapture)
			if len(got) != 1 {
				t.Fatalf("reserved_subject_capture errors = %+v, want exactly one", got)
			}
			want := tc.want
			want.Type, want.Severity = composition.TypeReservedSubjectCapture, composition.SeverityError
			if got[0].Type != want.Type || got[0].Severity != want.Severity || got[0].Component != want.Component ||
				got[0].Port != want.Port || got[0].Message != want.Message || len(got[0].Suggestions) == 0 {
				t.Fatalf("finding = %+v\nwant     %+v with suggestions", got[0], want)
			}
			if result.Status != composition.StatusErrors {
				t.Fatalf("status = %q, want %q", result.Status, composition.StatusErrors)
			}
		})
	}
}

// TestValidateDoesNotRefuseNeighbouringSubjects shows the refusal is not a prefix test: filters that
// share leading tokens with a reserved subject but no concrete subject are clear, whether they come
// from the explicit block or from a JetStream output.
func TestValidateDoesNotRefuseNeighbouringSubjects(t *testing.T) {
	neighbours := []string{"graph.ingest.events.>", "graph.ingestion.>", "graph.mutations.created", "graph.ingest.query"}
	outputs := make([]component.PortDefinition, 0, len(neighbours))
	for index, subject := range neighbours {
		outputs = append(outputs, jetStreamOut(fmt.Sprintf("out%d", index), "", subject))
	}
	registry := fakeRegistry(t, fakeSpec{name: "neighbour", typ: "output", outputs: outputs})
	cfg := compositionOf(config.ComponentConfigs{"n": instance("neighbour", types.ComponentTypeOutput)})
	cfg.Streams = config.StreamConfigs{"NEIGHBOURS": {Subjects: neighbours}}

	result := validateRoundTrip(t, registry, cfg)
	all := append(append([]composition.Finding(nil), result.Errors...), result.Warnings...)
	if got := findingsOfType(all, composition.TypeReservedSubjectCapture); len(got) != 0 {
		t.Fatalf("neighbouring subjects refused: %+v", got)
	}
}

// enumerationTokens is the alphabet of the generated check (design "Generated checks"): every token
// of graph-ingest's and the mutation family's reserved subjects, graph-index's tokens up to its first
// verb, and z, a token no reserved subject has. graph-index's later verbs are left out because each
// one widens the enumeration by a power of five; reservedInAlphabet says why the first is enough.
var enumerationTokens = []string{"graph", "ingest", "query", "mutation", "entity", "batch", "prefix", "index", "outgoing", "z"}

// TestReservedSubjectOverlapMatchesEnumeration checks every filter of one to four tokens over the
// alphabet with `*` and a final `>` against an oracle that enumerates concrete subjects. A filter
// overlaps a reserved subject when some concrete subject of at most five tokens matches both; five is
// enough, since a filter of at most four tokens that overlaps graph.mutation.> has a witness of at
// most four. Both SubjectsOverlap and the subject Analyze reports are compared with the oracle.
func TestReservedSubjectOverlapMatchesEnumeration(t *testing.T) {
	reserved := reservedInAlphabet(t)

	concrete := concreteSubjects(5)
	witnesses := make([][]string, len(reserved))
	for index, subject := range reserved {
		for _, candidate := range concrete {
			if oracleMatches(subject, candidate) {
				witnesses[index] = append(witnesses[index], candidate)
			}
		}
		if len(witnesses[index]) == 0 {
			t.Fatalf("no concrete subject over the alphabet matches reserved %q: widen the alphabet", subject)
		}
	}

	filters := enumerateFilters(4)
	streams := config.StreamConfigs{}
	want := map[string]string{} // stream name → first reserved subject captured
	several := 0                // filters capturing more than one reserved subject: the order rule decides
	for index, filter := range filters {
		name := fmt.Sprintf("S%05d", index)
		streams[name] = config.StreamConfig{Subjects: []string{filter}}
		first, hits := "", 0
		for r, subject := range reserved {
			captured := false
			for _, witness := range witnesses[r] {
				if oracleMatches(filter, witness) {
					captured = true
					break
				}
			}
			if got := flowgraph.SubjectsOverlap(filter, subject); got != captured {
				t.Errorf("SubjectsOverlap(%q, %q) = %v, oracle %v", filter, subject, got, captured)
			}
			if got := flowgraph.SubjectsOverlap(subject, filter); got != captured {
				t.Errorf("SubjectsOverlap(%q, %q) = %v, oracle %v", subject, filter, got, captured)
			}
			if captured {
				hits++
			}
			if captured && first == "" {
				first = subject
			}
		}
		if first != "" {
			want[name] = first
		}
		if hits > 1 {
			several++
		}
	}

	result := composition.Analyze(nil, streams)
	got := map[string]string{}
	for _, finding := range result.Errors {
		if finding.Type != composition.TypeReservedSubjectCapture {
			continue
		}
		var stream, filter, subject string
		if _, err := fmt.Sscanf(finding.Message, "stream %q subject filter %q captures reserved request subject %q",
			&stream, &filter, &subject); err != nil {
			t.Fatalf("unreadable finding message %q: %v", finding.Message, err)
		}
		if streams[stream].Subjects[0] != filter {
			t.Errorf("finding names filter %q for stream %s, which holds %q", filter, stream, streams[stream].Subjects[0])
		}
		got[stream] = subject
	}
	for name, subject := range want {
		if got[name] != subject {
			t.Errorf("filter %q: Analyze reports %q, oracle's first captured reserved subject is %q",
				streams[name].Subjects[0], got[name], subject)
		}
	}
	for name, subject := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("filter %q: Analyze reports %q, oracle finds no capture", streams[name].Subjects[0], subject)
		}
	}

	captured, clearFilters := len(want), len(filters)-len(want)
	t.Logf("filters %d: captured %d (of which %d capture several reserved subjects), clear %d; witnesses per reserved subject %v",
		len(filters), captured, several, clearFilters, witnessCounts(witnesses))
	if len(filters) != 17568 || captured == 0 || clearFilters == 0 || several == 0 {
		t.Fatalf("filters %d (want 17568), captured %d, several %d, clear %d: every branch must run",
			len(filters), captured, several, clearFilters)
	}
}

// reservedInAlphabet returns, in reserved order (query verbs in table order, then the mutation
// family), the reserved subjects whose every token is in the alphabet or is a final `>`. The oracle
// checks only these, and that is enough for the subject Analyze reports: Analyze names the first
// reserved subject in that order a filter overlaps (reservedCaptureFindings). A left-out subject
// differs from an earlier kept subject of the same responder only in its last token, which is not in
// the alphabet; so a filter over the alphabet reaches it only through a `*` or `>` at or before that
// position, and the same wildcard reaches the earlier kept sibling. Its first overlap is therefore
// always a kept subject. Both premises are checked here; when one fails, widen the alphabet.
func reservedInAlphabet(t *testing.T) []string {
	t.Helper()
	inAlphabet := map[string]bool{}
	for _, token := range enumerationTokens {
		inAlphabet[token] = true
	}
	type entry struct{ subject, responder string }
	var all []entry
	for _, verb := range graph.QueryVerbs() {
		all = append(all, entry{verb.Subject, verb.Responder})
	}
	all = append(all, entry{graphmutation.SubjectFamily, "graph-mutation"})

	var kept []entry
	keptBy := map[string]bool{}
	for _, candidate := range all {
		tokens := strings.Split(candidate.subject, ".")
		whole := true
		for index, token := range tokens {
			if !inAlphabet[token] && (token != ">" || index != len(tokens)-1) {
				whole = false
				break
			}
		}
		if whole {
			kept = append(kept, candidate)
			keptBy[candidate.responder] = true
			continue
		}
		sibling := false
		for _, earlier := range kept {
			earlierTokens := strings.Split(earlier.subject, ".")
			if earlier.responder == candidate.responder && len(earlierTokens) == len(tokens) &&
				!strings.ContainsAny(tokens[len(tokens)-1], "*>") &&
				strings.Join(earlierTokens[:len(tokens)-1], ".") == strings.Join(tokens[:len(tokens)-1], ".") {
				sibling = true
				break
			}
		}
		if !sibling {
			t.Fatalf("no concrete subject over the alphabet matches reserved %q, and no earlier kept subject of %s differs from it only in its last token: widen the alphabet",
				candidate.subject, candidate.responder)
		}
	}
	for _, candidate := range all {
		if !keptBy[candidate.responder] {
			t.Fatalf("no reserved subject of %s is over the alphabet: widen the alphabet", candidate.responder)
		}
	}
	subjects := make([]string, 0, len(kept))
	for _, candidate := range kept {
		subjects = append(subjects, candidate.subject)
	}
	return subjects
}

// enumerateFilters returns every filter of 1 to maxTokens tokens over the alphabet and `*`, with `>`
// allowed only as the last token.
func enumerateFilters(maxTokens int) []string {
	inner := append(append([]string(nil), enumerationTokens...), "*")
	last := append(append([]string(nil), inner...), ">")
	var out []string
	var walk func(prefix []string)
	walk = func(prefix []string) {
		for _, token := range last {
			out = append(out, strings.Join(append(prefix, token), "."))
		}
		if len(prefix)+1 < maxTokens {
			for _, token := range inner {
				walk(append(append([]string(nil), prefix...), token))
			}
		}
	}
	walk(nil)
	return out
}

// concreteSubjects returns every subject of 1 to maxTokens tokens over the alphabet, no wildcards.
func concreteSubjects(maxTokens int) []string {
	var out []string
	level := []string{""}
	for length := 1; length <= maxTokens; length++ {
		var next []string
		for _, prefix := range level {
			for _, token := range enumerationTokens {
				subject := token
				if prefix != "" {
					subject = prefix + "." + token
				}
				next = append(next, subject)
			}
		}
		out = append(out, next...)
		level = next
	}
	return out
}

// oracleMatches is the test's own NATS matcher of a filter against a concrete subject: `*` matches
// exactly one token, a final `>` one or more.
func oracleMatches(filter, subject string) bool {
	filterTokens, subjectTokens := strings.Split(filter, "."), strings.Split(subject, ".")
	for index, token := range filterTokens {
		if token == ">" {
			return len(subjectTokens) > index
		}
		if index >= len(subjectTokens) || (token != "*" && token != subjectTokens[index]) {
			return false
		}
	}
	return len(filterTokens) == len(subjectTokens)
}

func witnessCounts(witnesses [][]string) []int {
	counts := make([]int, len(witnesses))
	for index, list := range witnesses {
		counts[index] = len(list)
	}
	return counts
}
