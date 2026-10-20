package component

import (
	"errors"
	"reflect"
	"testing"
)

// conflictPair is two components' ports: first is admitted, then second is
// the candidate.
type conflictPair struct {
	firstInputs, firstOutputs   []Port
	secondInputs, secondOutputs []Port
}

// declarePair declares both components on a registry with nothing admitted,
// through the same Declare a composition check uses.
func declarePair(t *testing.T, pair conflictPair) (registry *Registry, first, second Declaration) {
	t.Helper()
	registry = NewRegistry()
	registerPortsFactory(t, registry, "first-factory", pair.firstInputs, pair.firstOutputs)
	registerPortsFactory(t, registry, "second-factory", pair.secondInputs, pair.secondOutputs)
	first, err := registry.Declare("first", declarationTestConfig("first-factory", `{}`))
	requireNoError(t, err)
	second, err = registry.Declare("second", declarationTestConfig("second-factory", `{}`))
	requireNoError(t, err)
	return registry, first, second
}

// TestDeclarationConflictWithMatchesAdmission: ConflictWith names the same
// clash, in the same words, as CreateComponent's refusal. Expected text is
// written as literals, never built by the code under test.
func TestDeclarationConflictWithMatchesAdmission(t *testing.T) {
	claim := func(name string, direction Direction, protocol, host string, port int) []Port {
		return []Port{networkClaimPort(name, direction, protocol, host, port)}
	}
	tests := []struct {
		name string
		pair conflictPair
		want string
	}{
		{
			name: "http and websocket on one TCP port",
			pair: conflictPair{
				firstOutputs:  claim("listen", DirectionOutput, "http", "0.0.0.0", 8080),
				secondOutputs: claim("listen", DirectionOutput, "websocket", "0.0.0.0", 8080),
			},
			want: "resource conflict: component 'second' claims websocket:0.0.0.0:8080, " +
				"held by component 'first' as http:0.0.0.0:8080",
		},
		{
			name: "two claims each, only the candidate's second clashes with the holder's first",
			pair: conflictPair{
				firstInputs:   claim("accept", DirectionInput, "tcp", "0.0.0.0", 8080),
				firstOutputs:  claim("serve", DirectionOutput, "tcp", "0.0.0.0", 9090),
				secondInputs:  claim("accept", DirectionInput, "tcp", "0.0.0.0", 7070),
				secondOutputs: claim("serve", DirectionOutput, "http", "127.0.0.1", 8080),
			},
			want: "resource conflict: component 'second' claims http:127.0.0.1:8080, " +
				"held by component 'first' as tcp:0.0.0.0:8080",
		},
		{
			// Two clashing pairs: the candidate's first with the holder's
			// second, and the candidate's second with the holder's first. The
			// holder's claims are the outer loop, so the holder's first wins.
			name: "two clashes each way, the holder's claims are visited first",
			pair: conflictPair{
				firstInputs:   claim("accept", DirectionInput, "tcp", "0.0.0.0", 8080),
				firstOutputs:  claim("serve", DirectionOutput, "tcp", "0.0.0.0", 9090),
				secondInputs:  claim("accept", DirectionInput, "tcp", "127.0.0.1", 9090),
				secondOutputs: claim("serve", DirectionOutput, "tcp", "127.0.0.1", 8080),
			},
			want: "resource conflict: component 'second' claims tcp:127.0.0.1:8080, " +
				"held by component 'first' as tcp:0.0.0.0:8080",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, first, second := declarePair(t, tt.pair)

			err := second.ConflictWith(first)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("ConflictWith error = %v, want %q", err, tt.want)
			}
			if errors.Unwrap(err) != nil {
				t.Fatalf("ConflictWith error %v wraps %v, want a plain error", err, errors.Unwrap(err))
			}

			_, err = registry.CreateComponent(
				"first", declarationTestConfig("first-factory", `{}`), declarationTestDeps(), nil)
			requireNoError(t, err)
			_, err = registry.CreateComponent(
				"second", declarationTestConfig("second-factory", `{}`), declarationTestDeps(), nil)
			requireInvalidRefusal(t, err, tt.want)
		})
	}
}

// TestDeclarationConflictWithAdmitsNonClashingClaims is the sensitivity pair of
// the test above: claims that can both be held give no error, either way round,
// and admission takes both.
func TestDeclarationConflictWithAdmitsNonClashingClaims(t *testing.T) {
	registry, first, second := declarePair(t, conflictPair{
		firstOutputs:  []Port{networkClaimPort("listen", DirectionOutput, "udp", "0.0.0.0", 8080)},
		secondOutputs: []Port{networkClaimPort("listen", DirectionOutput, "tcp", "0.0.0.0", 8080)},
	})

	if err := second.ConflictWith(first); err != nil {
		t.Fatalf("second.ConflictWith(first) = %v, want nil", err)
	}
	if err := first.ConflictWith(second); err != nil {
		t.Fatalf("first.ConflictWith(second) = %v, want nil", err)
	}

	_, err := registry.CreateComponent(
		"first", declarationTestConfig("first-factory", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)
	_, err = registry.CreateComponent(
		"second", declarationTestConfig("second-factory", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)
	if got := admittedNames(registry); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("admitted = %v, want [first second]", got)
	}
}
