package objectstore

import (
	"strings"
	"testing"
)

// The binding-plan tests, carried from the pin's config_ports_test.go:23-148. Each builds the
// Component through the factory from the JSON an operator writes; the pin built it from a Config
// literal. Every configuration now carries the stored output an input needs (design D3).

// plannedConsumers builds the Component from inputs (port definitions as JSON) and returns the
// durable consumer name its plan gives each JetStream input, by subject.
func plannedConsumers(t *testing.T, inputs string) map[string]string {
	t.Helper()
	c, err := buildFromJSON(t, `{"ports": {"inputs": [`+inputs+`], "outputs": [`+storedOutput+`]}}`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	names := make(map[string]string, len(c.bindings))
	for _, binding := range c.bindings {
		names[binding.subject] = binding.consumer
	}
	return names
}

// jetStreamInput is a JetStream input port named name on stream and subject.
func jetStreamInput(name, stream, subject string) string {
	return `{"name": "` + name + `", "config": {"kind": "jetstream", "stream_name": "` + stream +
		`", "subjects": ["` + subject + `"]}}`
}

// TestInputBindingPlanKeepsUniqueLegacyConsumerName: an input whose legacy consumer name collides
// with no other keeps it, so a deployment's existing durable consumer is bound again (pin
// TestObjectStoreInputBindingPlanPreservesUniqueLegacyConsumerName).
func TestInputBindingPlanKeepsUniqueLegacyConsumerName(t *testing.T) {
	got := plannedConsumers(t, jetStreamInput("store_in", "MAPPED", "mapped.messages"))
	if want := "objectstore-objectstore-mapped-messages"; got["mapped.messages"] != want {
		t.Fatalf("consumer name = %q, want the legacy name %q", got["mapped.messages"], want)
	}
}

// TestInputBindingPlanDisambiguatesLegacyCollisionsStably: four subjects whose legacy names
// collide on one stream each get a hashed name that depends on the subject only, not on the
// port's name or order (pin TestObjectStoreInputBindingPlanDisambiguatesLegacyCollisionsStably).
// The digests are the pin's literals.
func TestInputBindingPlanDisambiguatesLegacyCollisionsStably(t *testing.T) {
	forward := strings.Join([]string{
		jetStreamInput("dot", "COLLIDE", "foo.bar"),
		jetStreamInput("dash", "COLLIDE", "foo-bar"),
		jetStreamInput("wild", "COLLIDE", "foo.*"),
		jetStreamInput("word", "COLLIDE", "foo.all"),
	}, ", ")
	reverse := strings.Join([]string{
		jetStreamInput("renamed_word", "COLLIDE", "foo.all"),
		jetStreamInput("renamed_wild", "COLLIDE", "foo.*"),
		jetStreamInput("renamed_dash", "COLLIDE", "foo-bar"),
		jetStreamInput("renamed_dot", "COLLIDE", "foo.bar"),
	}, ", ")
	gotForward, gotReverse := plannedConsumers(t, forward), plannedConsumers(t, reverse)
	want := map[string]string{
		"foo.bar": "objectstore-h-bc749a19c08acd9ba792effb9dd8c7ae0304ee7ac10090326f644f01276c7da8",
		"foo-bar": "objectstore-h-b144089414ccbd79134bee9a337a99ef4dbb992076176b4002615114b42924df",
		"foo.*":   "objectstore-h-2d23635824be8dd4eac9fb12e81f4e01741477990b7343acb72ba87200cf9d24",
		"foo.all": "objectstore-h-cae04769e97a0c8a9db2b2cd75e3846ca95f50f35343fb319ee90c171cfc4127",
	}
	for subject, wantName := range want {
		if gotForward[subject] != wantName {
			t.Errorf("forward consumer for %q = %q, want %q", subject, gotForward[subject], wantName)
		}
		if gotReverse[subject] != wantName {
			t.Errorf("reverse consumer for %q = %q, want %q", subject, gotReverse[subject], wantName)
		}
	}
}

// TestInputBindingPlanRefusesExactDuplicates: two inputs bound to the same subject (core NATS) or
// the same stream and subject (JetStream) are refused at construction, naming both ports in
// sorted order, whatever order they were declared in (pin
// TestObjectStoreInputBindingPlanRejectsExactDuplicatesDeterministically).
func TestInputBindingPlanRefusesExactDuplicates(t *testing.T) {
	for _, tc := range []struct{ name, inputs, want string }{
		{
			"core NATS",
			`{"name": "zeta", "config": {"kind": "nats", "subject": "duplicate.core"}}, ` +
				`{"name": "alpha", "config": {"kind": "nats", "subject": "duplicate.core"}}`,
			`duplicate ObjectStore NATS binding subject="duplicate.core" declared by ports ["alpha" "zeta"]`,
		},
		{
			"JetStream",
			jetStreamInput("zeta", "DUP", "duplicate.js") + ", " + jetStreamInput("alpha", "DUP", "duplicate.js"),
			`duplicate ObjectStore JetStream binding stream="DUP" subject="duplicate.js" declared by ports ["alpha" "zeta"]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildFromJSON(t, `{"ports": {"inputs": [`+tc.inputs+`], "outputs": [`+storedOutput+`]}}`)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("construction error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestInputBindingPlanRefusesOverlengthLegacyName: a subject whose legacy consumer name would be
// longer than NATS allows is refused at construction, not at Start (pin
// TestObjectStoreInputBindingPlanRejectsOverlengthLegacyName).
func TestInputBindingPlanRefusesOverlengthLegacyName(t *testing.T) {
	inputs := jetStreamInput("long", "LONG", strings.Repeat("a", 240))
	_, err := buildFromJSON(t, `{"ports": {"inputs": [`+inputs+`], "outputs": [`+storedOutput+`]}}`)
	if err == nil || !strings.Contains(err.Error(), "maximum is 255") {
		t.Fatalf("construction error = %v, want the planned consumer name refused as too long", err)
	}
}
