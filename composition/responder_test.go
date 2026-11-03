package composition_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

// declaredInstance is one instance of a responder test: the factory that builds it and the ports it
// declares. Ports are per instance so two instances of a factory can claim different verbs.
type declaredInstance struct {
	name    string
	factory string
	inputs  []component.PortDefinition
	outputs []component.PortDefinition
}

// declareInstances runs each instance through a registry's Declare, the projection Validate and boot
// admission read, and returns the declarations in the order given.
func declareInstances(t *testing.T, instances []declaredInstance) []component.Declaration {
	t.Helper()
	byName := map[string]declaredInstance{}
	registry := component.NewRegistry()
	registered := map[string]bool{}
	for _, inst := range instances {
		byName[inst.name] = inst
		if registered[inst.factory] {
			continue
		}
		registered[inst.factory] = true
		factory := inst.factory
		err := registry.Register(component.Registration{
			Name: factory, Type: types.ComponentTypeProcessor, Protocol: "fake", Domain: "test", Version: "0.0.1",
			Description: "fake " + factory,
			Schema:      component.ConfigSchema{Properties: map[string]component.PropertySchema{"knob": {Type: "string"}}},
			Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
				return nil, errors.New("these tests must not construct " + factory)
			},
			Ports: func(_ json.RawMessage, instanceName string) (component.PortConfig, error) {
				return component.PortConfig{Inputs: byName[instanceName].inputs, Outputs: byName[instanceName].outputs}, nil
			},
		})
		if err != nil {
			t.Fatalf("register %s: %v", factory, err)
		}
	}
	declarations := make([]component.Declaration, 0, len(instances))
	for _, inst := range instances {
		declaration, err := registry.Declare(inst.name, instance(inst.factory, types.ComponentTypeProcessor))
		if err != nil {
			t.Fatalf("declare %s: %v", inst.name, err)
		}
		declarations = append(declarations, declaration)
	}
	return declarations
}

func requestIn(name, subject string) component.PortDefinition {
	return component.PortDefinition{Name: name, Required: true, Config: component.NATSRequestPort{Subject: subject}}
}

func responding(name, factory, subject string) declaredInstance {
	return declaredInstance{name: name, factory: factory, inputs: []component.PortDefinition{requestIn("in", subject)}}
}

// TestAnalyzeReportsSecondResponder: a NATS-request input overlapping a subject of graph.QueryVerbs()
// is refused when its factory is not that verb's responder, or on the later instance when two
// instances of the responder declare the verb. The expected text is written out here, so the first
// conflicting table row each case names is read from the table by hand: the ingest rows come first,
// then index, then graph.query, each starting with the row the message names.
func TestAnalyzeReportsSecondResponder(t *testing.T) {
	type want struct{ component, port, message string }
	for _, tc := range []struct {
		name      string
		instances []declaredInstance
		want      []want
	}{
		{
			name:      "another factory on the exact subject",
			instances: []declaredInstance{responding("sq", "squatter", "graph.query.entity")},
			want: []want{{"sq", "in", `instance "sq" of factory "squatter" declares NATS-request input subject ` +
				`"graph.query.entity", overlapping "graph.query.entity", which factory "graph-query" answers`}},
		},
		{
			name:      "another factory on a single-token wildcard",
			instances: []declaredInstance{responding("sq", "squatter", "graph.query.*")},
			want: []want{{"sq", "in", `instance "sq" of factory "squatter" declares NATS-request input subject ` +
				`"graph.query.*", overlapping "graph.query.entity", which factory "graph-query" answers`}},
		},
		{
			name:      "another factory on a leading wildcard",
			instances: []declaredInstance{responding("sq", "squatter", "*.query.entity")},
			want: []want{{"sq", "in", `instance "sq" of factory "squatter" declares NATS-request input subject ` +
				`"*.query.entity", overlapping "graph.query.entity", which factory "graph-query" answers`}},
		},
		{
			name:      "another factory on the full wildcard names the first table row",
			instances: []declaredInstance{responding("sq", "squatter", ">")},
			want: []want{{"sq", "in", `instance "sq" of factory "squatter" declares NATS-request input subject ` +
				`">", overlapping "graph.ingest.query.entity", which factory "graph-ingest" answers`}},
		},
		{
			name:      "another factory on a mid-table family",
			instances: []declaredInstance{responding("sq", "squatter", "graph.index.query.*")},
			want: []want{{"sq", "in", `instance "sq" of factory "squatter" declares NATS-request input subject ` +
				`"graph.index.query.*", overlapping "graph.index.query.outgoing", which factory "graph-index" answers`}},
		},
		{
			name:      "one responder on another responder's verb",
			instances: []declaredInstance{responding("idx", "graph-index", "graph.query.entity")},
			want: []want{{"idx", "in", `instance "idx" of factory "graph-index" declares NATS-request input subject ` +
				`"graph.query.entity", overlapping "graph.query.entity", which factory "graph-query" answers`}},
		},
		{
			name:      "a responder whose wildcard reaches past its own verbs",
			instances: []declaredInstance{responding("gq", "graph-query", ">")},
			want: []want{{"gq", "in", `instance "gq" of factory "graph-query" declares NATS-request input subject ` +
				`">", overlapping "graph.ingest.query.entity", which factory "graph-ingest" answers`}},
		},
		{
			name: "a port beside the family, a sibling token and a longer subject overlap nothing",
			instances: []declaredInstance{
				responding("beside", "consumer", "graph.query.sources"),
				responding("sibling", "consumer", "graph.queries.entity"),
				responding("longer", "consumer", "graph.query.entity.more"),
			},
		},
		{
			name: "a consumer verb beside the responder's family",
			instances: []declaredInstance{
				responding("gq", "graph-query", "graph.query.*"),
				responding("sources", "consumer", "graph.query.sources"),
			},
		},
		{
			name:      "graph-query alone on its family",
			instances: []declaredInstance{responding("gq", "graph-query", "graph.query.*")},
		},
		{
			name:      "graph-ingest alone on its verb",
			instances: []declaredInstance{responding("gi", "graph-ingest", "graph.ingest.query.entity")},
		},
		{
			name:      "graph-index alone on its family",
			instances: []declaredInstance{responding("ix", "graph-index", "graph.index.query.*")},
		},
		{
			name: "two instances of the responder: only the later",
			instances: []declaredInstance{
				responding("gq-a", "graph-query", "graph.query.*"),
				responding("gq-b", "graph-query", "graph.query.*"),
			},
			want: []want{{"gq-b", "in", `instance "gq-b" of factory "graph-query" declares NATS-request input subject ` +
				`"graph.query.*", overlapping "graph.query.entity", which instance "gq-a" of factory "graph-query" already answers`}},
		},
		{
			name: "three instances of the responder: each later one names the first",
			instances: []declaredInstance{
				responding("gq-a", "graph-query", "graph.query.*"),
				responding("gq-b", "graph-query", "graph.query.*"),
				responding("gq-c", "graph-query", "graph.query.*"),
			},
			want: []want{
				{"gq-b", "in", `instance "gq-b" of factory "graph-query" declares NATS-request input subject ` +
					`"graph.query.*", overlapping "graph.query.entity", which instance "gq-a" of factory "graph-query" already answers`},
				{"gq-c", "in", `instance "gq-c" of factory "graph-query" declares NATS-request input subject ` +
					`"graph.query.*", overlapping "graph.query.entity", which instance "gq-a" of factory "graph-query" already answers`},
			},
		},
		{
			name: "two instances of the responder on different verbs share none",
			instances: []declaredInstance{
				responding("gq-a", "graph-query", "graph.query.entity"),
				responding("gq-b", "graph-query", "graph.query.batch"),
			},
		},
		{
			// gq-b's wildcard first meets a claimed verb at batch, not at the table's first graph.query row.
			name: "the later instance names the verb they share",
			instances: []declaredInstance{
				responding("gq-a", "graph-query", "graph.query.batch"),
				responding("gq-b", "graph-query", "graph.query.*"),
			},
			want: []want{{"gq-b", "in", `instance "gq-b" of factory "graph-query" declares NATS-request input subject ` +
				`"graph.query.*", overlapping "graph.query.batch", which instance "gq-a" of factory "graph-query" already answers`}},
		},
		{
			// gq-c meets gq-b, which holds prefix only through a port already refused for entity and
			// batch: every verb a port overlaps is claimed, past its first conflict and past its second.
			name: "a refused instance still holds the other verbs its port overlaps",
			instances: []declaredInstance{
				{name: "gq-a", factory: "graph-query", inputs: []component.PortDefinition{
					requestIn("entity", "graph.query.entity"), requestIn("batch", "graph.query.batch"),
				}},
				responding("gq-b", "graph-query", "graph.query.*"),
				responding("gq-c", "graph-query", "graph.query.prefix"),
			},
			want: []want{
				{"gq-b", "in", `instance "gq-b" of factory "graph-query" declares NATS-request input subject ` +
					`"graph.query.*", overlapping "graph.query.entity", which instance "gq-a" of factory "graph-query" already answers`},
				{"gq-c", "in", `instance "gq-c" of factory "graph-query" declares NATS-request input subject ` +
					`"graph.query.prefix", overlapping "graph.query.prefix", which instance "gq-b" of factory "graph-query" already answers`},
			},
		},
		{
			// A non-responder claims nothing, so the responder's own instance is not its later party.
			name: "a non-responder sorted first does not displace the responder",
			instances: []declaredInstance{
				responding("a-squatter", "squatter", "graph.query.entity"),
				responding("gq", "graph-query", "graph.query.*"),
			},
			want: []want{{"a-squatter", "in", `instance "a-squatter" of factory "squatter" declares NATS-request input subject ` +
				`"graph.query.entity", overlapping "graph.query.entity", which factory "graph-query" answers`}},
		},
		{
			name: "one instance of the responder with two ports on one verb is not two responders",
			instances: []declaredInstance{{name: "gq", factory: "graph-query", inputs: []component.PortDefinition{
				requestIn("family", "graph.query.*"), requestIn("exact", "graph.query.entity"),
			}}},
		},
		{
			name: "another factory gets one finding per port, one naming the first row each overlaps",
			instances: []declaredInstance{{name: "sq", factory: "squatter", inputs: []component.PortDefinition{
				requestIn("wide", "graph.query.*"), requestIn("narrow", "graph.query.batch"),
			}}},
			want: []want{
				{"sq", "narrow", `instance "sq" of factory "squatter" declares NATS-request input subject ` +
					`"graph.query.batch", overlapping "graph.query.batch", which factory "graph-query" answers`},
				{"sq", "wide", `instance "sq" of factory "squatter" declares NATS-request input subject ` +
					`"graph.query.*", overlapping "graph.query.entity", which factory "graph-query" answers`},
			},
		},
		{
			name: "a core NATS input on a table subject is not a responder",
			instances: []declaredInstance{{name: "tap", factory: "tap", inputs: []component.PortDefinition{
				natsIn("in", "graph.query.entity", false, nil),
			}}},
		},
		{
			name: "a JetStream input on a table subject is not a responder",
			instances: []declaredInstance{{name: "tap", factory: "tap", inputs: []component.PortDefinition{
				jetStreamIn("in", "TAP", "graph.query.entity", false),
			}}},
		},
		{
			name: "a NATS-request output is a caller, not a responder",
			instances: []declaredInstance{{name: "caller", factory: "caller", outputs: []component.PortDefinition{
				{Name: "out", Required: true, Config: component.NATSRequestPort{Subject: "graph.query.entity"}},
			}}},
		},
		{
			// malformed_subject owns this one; a token mixing '*' with text overlaps nothing here.
			name:      "a malformed subject is not also a conflict",
			instances: []declaredInstance{responding("sq", "squatter", "graph.query.ent*")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			declarations := declareInstances(t, tc.instances)
			reversed := slices.Clone(declarations)
			slices.Reverse(reversed)
			forward := composition.Analyze(declarations, nil)
			backward := composition.Analyze(reversed, nil)
			// The input's order must not decide which instance is later or how findings are ordered.
			if !reflect.DeepEqual(forward.Errors, backward.Errors) {
				t.Errorf("findings depend on the input's order:\n forward %+v\nbackward %+v", forward.Errors, backward.Errors)
			}
			var got []want
			for _, finding := range findingsOfType(forward.Errors, composition.TypeRequestSubjectConflict) {
				got = append(got, want{finding.Component, finding.Port, finding.Message})
				if finding.Severity != composition.SeverityError {
					t.Errorf("severity = %q, want error", finding.Severity)
				}
				if len(finding.Suggestions) == 0 {
					t.Errorf("finding %+v gives no suggestion", finding)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("request_subject_conflict findings:\n got %q\nwant %q", got, tc.want)
			}
			if leaked := findingsOfType(forward.Warnings, composition.TypeRequestSubjectConflict); len(leaked) != 0 {
				t.Errorf("reported as a warning: %+v", leaked)
			}
			if len(tc.want) > 0 && forward.Status != composition.StatusErrors {
				t.Errorf("status = %q, want errors", forward.Status)
			}
		})
	}
}

// TestAnalyzeReportsSecondResponderWhenTheGraphCannotBeBuilt: the check reads only port facts, so a
// composition whose graph fails to build, and so returns early, still carries the finding.
func TestAnalyzeReportsSecondResponderWhenTheGraphCannotBeBuilt(t *testing.T) {
	declarations := declareInstances(t, []declaredInstance{
		responding("sq", "squatter", "graph.query.entity"),
		{name: "w1", factory: "writer", outputs: []component.PortDefinition{{Name: "idx", Config: component.KVWritePort{Bucket: "SHARED"}}}},
		{name: "w2", factory: "writer", outputs: []component.PortDefinition{{Name: "idx", Config: component.KVWritePort{Bucket: "SHARED"}}}},
	})
	result := composition.Analyze(declarations, config.StreamConfigs{})
	if got := findingsOfType(result.Errors, composition.TypeConnectionPatternError); len(got) != 1 {
		t.Fatalf("the graph built, so this case proves nothing: %+v", result.Errors)
	}
	if got := findingsOfType(result.Errors, composition.TypeRequestSubjectConflict); len(got) != 1 || got[0].Component != "sq" {
		t.Errorf("request_subject_conflict findings = %+v, want one on sq", got)
	}
}
