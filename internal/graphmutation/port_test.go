package graphmutation

import (
	"testing"

	"github.com/c360studio/semengine/component"
)

// TestClassifyPort checks membership and shape on facts from real ports. The
// family, interface type and version are literals, never this package's
// constants, so a change to a constant shows here. Each row of the "member,
// not valid" group breaks exactly one condition of the valid port.
func TestClassifyPort(t *testing.T) {
	typed := func(version string) *component.InterfaceContract {
		return &component.InterfaceContract{Type: "semengine.graph.mutation", Version: version}
	}
	request := func(subject string, contract *component.InterfaceContract, required bool) component.Port {
		return component.Port{
			Name: "mutations", Direction: component.DirectionInput, Required: required,
			Config: component.NATSRequestPort{Subject: subject, Interface: contract},
		}
	}
	tests := []struct {
		name          string
		port          component.Port
		member, valid bool
	}{
		{name: "the valid provider", port: request("graph.mutation.>", typed("v1"), true), member: true, valid: true},
		{name: "optional", port: request("graph.mutation.>", typed("v1"), false), member: true},
		{name: "wrong version", port: request("graph.mutation.>", typed("v2"), true), member: true},
		{
			name: "wrong interface type on the family", member: true,
			port: request("graph.mutation.>", &component.InterfaceContract{Type: "other.interface", Version: "v1"}, true),
		},
		{name: "typed, single-token wildcard", port: request("graph.mutation.*", typed("v1"), true), member: true},
		{name: "typed, off the family", port: request("events.write", typed("v1"), true), member: true},
		{name: "legacy untyped subject", port: request("graph.mutation.entity.create", nil, true), member: true},
		{
			name: "typed nats port on the family", member: true,
			port: component.Port{
				Name: "mutations", Direction: component.DirectionInput, Required: true,
				Config: component.NATSPort{Subject: "graph.mutation.>", Interface: typed("v1")},
			},
		},
		{
			name: "jetstream port with one subject on the family", member: true,
			port: component.Port{
				Name: "events", Direction: component.DirectionInput, Required: true,
				Config: component.JetStreamPort{
					StreamName: "EVENTS", Subjects: []string{"events.>", "graph.mutation.entity.create"},
				},
			},
		},
		{name: "untyped, outside the family", port: request("graph.query.entity", nil, true)},
		{name: "untyped, a longer first token", port: request("graph.mutations.entity", nil, true)},
		{
			name: "another interface, outside the family",
			port: request("events.write", &component.InterfaceContract{Type: "other.interface", Version: "v1"}, true),
		},
		{
			name: "kv write port",
			port: component.Port{
				Name: "entity_states", Direction: component.DirectionOutput, Required: true,
				Config: component.KVWritePort{Bucket: "ENTITY_STATES"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := tt.port.Facts()
			if err != nil {
				t.Fatalf("Facts(): %v", err)
			}
			member, valid := ClassifyPort(facts, tt.port.Required)
			if member != tt.member || valid != tt.valid {
				t.Fatalf("ClassifyPort() = member %t, valid %t; want member %t, valid %t",
					member, valid, tt.member, tt.valid)
			}
		})
	}
}
