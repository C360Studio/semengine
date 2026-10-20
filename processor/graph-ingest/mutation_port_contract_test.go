package graphingest

import (
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/pkg/errs"
)

func TestDefaultConfigDeclaresRequiredMutationProvider(t *testing.T) {
	config := DefaultConfig()
	for _, definition := range config.Ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			t.Fatalf("resolve mutation provider: %v", err)
		}
		request, ok := port.Config.(component.NATSRequestPort)
		if !ok || request.Interface == nil || request.Interface.Type != graphmutation.InterfaceType {
			continue
		}
		if !port.Required || request.Subject != graphmutation.SubjectFamily ||
			request.Interface.Version != graphmutation.InterfaceVersion {
			t.Fatalf("mutation provider = %#v", port)
		}
		return
	}
	t.Fatal("default config has no typed graph mutation provider")
}

// TestConfigValidateMutationProviderRefusals pins the refusals Config.Validate
// gives for graph-mutation ports. The ports and the texts are written as
// literals, never built from graphmutation's constants, so a change to either
// shows here.
func TestConfigValidateMutationProviderRefusals(t *testing.T) {
	const malformed = "graph mutation provider must be a required semengine.graph.mutation v1 nats-request " +
		"input on graph.mutation.>"
	entityStream := component.PortDefinition{
		Name: "entity_stream", Config: component.JetStreamPort{StreamName: "ENTITY", Subjects: []string{"entity.>"}},
	}
	provider := func(name, subject, version string, required bool) component.PortDefinition {
		return component.PortDefinition{
			Name: name, Required: required,
			Config: component.NATSRequestPort{
				Subject:   subject,
				Interface: &component.InterfaceContract{Type: "semengine.graph.mutation", Version: version},
			},
		}
	}
	valid := provider("mutations", "graph.mutation.>", "v1", true)
	untyped := func(subject string) component.PortDefinition {
		return component.PortDefinition{
			Name: "legacy", Required: true, Config: component.NATSRequestPort{Subject: subject},
		}
	}
	configWith := func(inputs ...component.PortDefinition) Config {
		return Config{Ports: &component.PortConfig{
			Inputs:  inputs,
			Outputs: []component.PortDefinition{{Name: "entity_states", Config: component.KVWritePort{Bucket: "ENTITY_STATES"}}},
		}}
	}
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "no ports", config: Config{}, want: "ports configuration required"},
		{name: "no candidate", config: configWith(entityStream), want: "graph mutation provider port is required"},
		{
			name:   "two candidates",
			config: configWith(entityStream, valid, provider("mutations_2", "graph.mutation.>", "v1", true)),
			want:   "graph mutation provider port is ambiguous",
		},
		{name: "wrong version", config: configWith(entityStream, provider("mutations", "graph.mutation.>", "v2", true)), want: malformed},
		{name: "not required", config: configWith(entityStream, provider("mutations", "graph.mutation.>", "v1", false)), want: malformed},
		{name: "untyped graph.mutation.*", config: configWith(entityStream, untyped("graph.mutation.*")), want: malformed},
		{
			name: "untyped graph.mutation.entity.create", config: configWith(entityStream, untyped("graph.mutation.entity.create")),
			want: malformed,
		},
		{
			// Ruled on #323 (Q1): any one subject on graph.mutation. makes a
			// port a member, so this second member beside the provider is refused.
			name: "jetstream input with one graph.mutation subject",
			config: configWith(entityStream, valid, component.PortDefinition{
				Name: "events",
				Config: component.JetStreamPort{
					StreamName: "EVENTS", Subjects: []string{"events.>", "graph.mutation.entity.create"},
				},
			}),
			want: "graph mutation provider port is ambiguous",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil || !errs.IsInvalid(err) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want an invalid-class refusal containing %q", err, tt.want)
			}
		})
	}
	t.Run("the valid provider", func(t *testing.T) {
		config := configWith(entityStream, valid)
		if err := config.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})
}
