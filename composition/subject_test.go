package composition_test

import (
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

// malformedSubjects is the hand-written verdict list (#369 design, "Proving tests"): each subject
// fails the subject grammar. The space, form feed, `a.>.b` and `a..b` verdicts come from the #367
// probe (server.IsValidSubject false); `a*` and `a.b*.c` are the grammar's documented stricter
// points; tab, line feed and carriage return come from the spec's grammar text.
var malformedSubjects = []string{"a.foo bar", "a.foo\fbar", "a.>.b", "a*", "a.b*.c", "a..b", "a.\tb", "a.\nb", "a.\rb"}

// validSubjects pass the grammar: the #367 probe for `a.>` and `a.*.c`, the spec's grammar text for
// `>` and the JetStream advisory family.
var validSubjects = []string{"a.>", "a.*.c", ">", "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.>"}

// subjectSite is one place a subject is declared. build returns the registry and configuration
// declaring subject there; want is the expected finding's component, port and message prefix, the
// part before the quoted subject.
type subjectSite struct {
	name      string
	build     func(t *testing.T, subject string) (*component.Registry, *config.Config)
	component string
	port      string
	prefix    string
}

// oneComponent declares one component, instance "c", with the given ports.
func oneComponent(inputs, outputs []component.PortDefinition) func(*testing.T) (*component.Registry, *config.Config) {
	return func(t *testing.T) (*component.Registry, *config.Config) {
		registry := fakeRegistry(t, fakeSpec{name: "site", typ: "processor", inputs: inputs, outputs: outputs})
		return registry, compositionOf(config.ComponentConfigs{"c": instance("site", types.ComponentTypeProcessor)})
	}
}

var subjectSites = []subjectSite{
	{
		name: "NATS input",
		build: func(t *testing.T, subject string) (*component.Registry, *config.Config) {
			return oneComponent([]component.PortDefinition{natsIn("in", subject, false, nil)}, nil)(t)
		},
		component: "c", port: "in", prefix: `input port "in" field "subject": subject `,
	},
	{
		name: "NATS output",
		build: func(t *testing.T, subject string) (*component.Registry, *config.Config) {
			return oneComponent(nil, []component.PortDefinition{natsOut("out", subject, nil)})(t)
		},
		component: "c", port: "out", prefix: `output port "out" field "subject": subject `,
	},
	{
		name: "NATS-request input",
		build: func(t *testing.T, subject string) (*component.Registry, *config.Config) {
			return oneComponent([]component.PortDefinition{{Name: "ask", Config: component.NATSRequestPort{Subject: subject}}}, nil)(t)
		},
		component: "c", port: "ask", prefix: `input port "ask" field "subject": subject `,
	},
	{
		name: "NATS-request output",
		build: func(t *testing.T, subject string) (*component.Registry, *config.Config) {
			return oneComponent(nil, []component.PortDefinition{{Name: "reply", Config: component.NATSRequestPort{Subject: subject}}})(t)
		},
		component: "c", port: "reply", prefix: `output port "reply" field "subject": subject `,
	},
	{
		name: "JetStream input second subject",
		build: func(t *testing.T, subject string) (*component.Registry, *config.Config) {
			return oneComponent([]component.PortDefinition{{Name: "in", Config: component.JetStreamPort{
				StreamName: "IN", Subjects: []string{"ok.first", subject}}}}, nil)(t)
		},
		component: "c", port: "in", prefix: `input port "in" field "subjects[1]": subject `,
	},
	{
		name: "JetStream output second subject",
		build: func(t *testing.T, subject string) (*component.Registry, *config.Config) {
			return oneComponent(nil, []component.PortDefinition{{Name: "out", Config: component.JetStreamPort{
				StreamName: "OUT", Subjects: []string{"ok.first", subject}}}})(t)
		},
		component: "c", port: "out", prefix: `output port "out" field "subjects[1]": subject `,
	},
	{
		name:      "explicit stream second subject",
		build:     explicitStreamSite,
		component: "config", port: "", prefix: `stream "EVENTS" field "subjects[1]": subject `,
	},
}

// explicitStreamSite declares subject as the second subject of explicit stream EVENTS beside one
// clear component.
func explicitStreamSite(t *testing.T, subject string) (*component.Registry, *config.Config) {
	registry, cfg := oneComponent(nil, []component.PortDefinition{natsOut("out", "ok.out", nil)})(t)
	cfg.Streams = config.StreamConfigs{"EVENTS": {Subjects: []string{"ok.first", subject}}}
	return registry, cfg
}

// quoted is the subject as the message quotes it, written by hand for each verdict-list entry.
var quoted = map[string]string{
	"a.foo bar":  `"a.foo bar"`,
	"a.foo\fbar": `"a.foo\fbar"`,
	"a.>.b":      `"a.>.b"`,
	"a*":         `"a*"`,
	"a.b*.c":     `"a.b*.c"`,
	"a..b":       `"a..b"`,
	"a.\tb":      `"a.\tb"`,
	"a.\nb":      `"a.\nb"`,
	"a.\rb":      `"a.\rb"`,
	"":           `""`,
}

// TestAnalyzeReportsMalformedSubject drives every verdict-list subject through every site where a
// subject is declared, through Validate as a product calls it. A malformed subject yields exactly one
// malformed_subject error naming the site; a valid one yields none. Valid cases count
// malformed_subject findings only: `>` on a stream or a JetStream output is also a
// reserved_subject_capture, which is not this test's judgment.
func TestAnalyzeReportsMalformedSubject(t *testing.T) {
	for _, site := range subjectSites {
		malformed := malformedSubjects
		if site.component == "config" {
			// The port codec refuses an empty port subject at declaration; only the stream site
			// reaches Analyze with one.
			malformed = append(append([]string(nil), malformedSubjects...), "")
		}
		for _, subject := range malformed {
			t.Run(site.name+"/malformed "+quoted[subject], func(t *testing.T) {
				registry, cfg := site.build(t, subject)
				result := validateRoundTrip(t, registry, cfg)
				if warnings := findingsOfType(result.Warnings, composition.TypeMalformedSubject); len(warnings) != 0 {
					t.Fatalf("malformed_subject reported as a warning: %+v", warnings)
				}
				got := findingsOfType(result.Errors, composition.TypeMalformedSubject)
				if len(got) != 1 {
					t.Fatalf("malformed_subject errors = %+v, want exactly one (all errors %+v)", got, result.Errors)
				}
				wantMessage := site.prefix + quoted[subject] + " is not a valid subject"
				if got[0].Severity != composition.SeverityError || got[0].Component != site.component ||
					got[0].Port != site.port || got[0].Message != wantMessage || len(got[0].Suggestions) == 0 {
					t.Fatalf("finding = %+v\nwant component %q port %q message %q, error severity, suggestions",
						got[0], site.component, site.port, wantMessage)
				}
				if result.Status != composition.StatusErrors {
					t.Fatalf("status = %q, want %q", result.Status, composition.StatusErrors)
				}
			})
		}
		for _, subject := range validSubjects {
			t.Run(site.name+"/valid "+subject, func(t *testing.T) {
				registry, cfg := site.build(t, subject)
				result := validateRoundTrip(t, registry, cfg)
				all := append(append([]composition.Finding(nil), result.Errors...), result.Warnings...)
				if got := findingsOfType(all, composition.TypeMalformedSubject); len(got) != 0 {
					t.Fatalf("valid subject %q reported malformed: %+v", subject, got)
				}
			})
		}
	}
}

// TestAnalyzeReportsMalformedSubjectBeforeEarlyReturns shows the check runs before Analyze stops
// early: with no enabled component, and when two writers to one KV bucket stop the graph build.
func TestAnalyzeReportsMalformedSubjectBeforeEarlyReturns(t *testing.T) {
	registry := fakeRegistry(t,
		fakeSpec{name: "writer", typ: "processor", outputs: []component.PortDefinition{{Name: "idx", Config: component.KVWritePort{Bucket: "SHARED"}}}},
	)
	for _, tc := range []struct {
		name       string
		components config.ComponentConfigs
		beside     string
	}{
		{name: "no enabled component", components: config.ComponentConfigs{}, beside: composition.TypeEmptyComposition},
		{
			name: "graph build fails",
			components: config.ComponentConfigs{
				"a": instance("writer", types.ComponentTypeProcessor),
				"b": instance("writer", types.ComponentTypeProcessor),
			},
			beside: composition.TypeConnectionPatternError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := compositionOf(tc.components)
			cfg.Streams = config.StreamConfigs{"EVENTS": {Subjects: []string{"a..b"}}}
			result := validateRoundTrip(t, registry, cfg)
			got := findingsOfType(result.Errors, composition.TypeMalformedSubject)
			want := `stream "EVENTS" field "subjects[0]": subject "a..b" is not a valid subject`
			if len(got) != 1 || got[0].Component != "config" || got[0].Port != "" || got[0].Message != want {
				t.Fatalf("malformed_subject errors = %+v, want one from config with message %q", got, want)
			}
			all := append(append([]composition.Finding(nil), result.Errors...), result.Warnings...)
			if len(findingsOfType(all, tc.beside)) == 0 {
				t.Fatalf("no %s beside the malformed subject: %+v", tc.beside, all)
			}
		})
	}
}

// TestAnalyzeMalformedFilterIsNotAReservedCapture: a filter the grammar refuses overlaps nothing, so
// graph.mutation.>.x is one malformed_subject and no reserved_subject_capture.
func TestAnalyzeMalformedFilterIsNotAReservedCapture(t *testing.T) {
	registry, cfg := oneComponent(nil, []component.PortDefinition{natsOut("out", "ok.out", nil)})(t)
	cfg.Streams = config.StreamConfigs{"MUT": {Subjects: []string{"graph.mutation.>.x"}}}
	result := validateRoundTrip(t, registry, cfg)
	all := append(append([]composition.Finding(nil), result.Errors...), result.Warnings...)
	if got := findingsOfType(all, composition.TypeMalformedSubject); len(got) != 1 {
		t.Fatalf("malformed_subject findings = %+v, want exactly one", got)
	}
	if got := findingsOfType(all, composition.TypeReservedSubjectCapture); len(got) != 0 {
		t.Fatalf("malformed filter reported as a reserved capture: %+v", got)
	}
}
