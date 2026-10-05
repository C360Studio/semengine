package vocabulary

import (
	"testing"
)

func TestParsePredicateNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		want      PredicateNamespace
		wantErr   bool
	}{
		{name: "domain", namespace: "research", want: PredicateNamespace{Domain: "research"}},
		{name: "domain category", namespace: "research.result", want: PredicateNamespace{Domain: "research", Category: "result"}},
		{name: "property is too specific", namespace: "research.result.complete", wantErr: true},
		{name: "wildcard", namespace: "research.*", wantErr: true},
		{name: "underscore", namespace: "research.search_result", wantErr: true},
		{name: "empty", namespace: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParsePredicateNamespace(tt.namespace)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePredicateNamespace(%q) unexpectedly succeeded", tt.namespace)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePredicateNamespace(%q): %v", tt.namespace, err)
			}
			if got != tt.want || got.String() != tt.namespace {
				t.Fatalf("ParsePredicateNamespace(%q) = %#v (%q), want %#v", tt.namespace, got, got.String(), tt.want)
			}
		})
	}
}

func TestRequireDeclaredPredicate(t *testing.T) {
	declared := "authority-test.registered.value"
	Register(declared)
	if err := RequireDeclaredPredicate(declared); err != nil {
		t.Fatalf("registered predicate rejected: %v", err)
	}
	if err := RequireDeclaredPredicate("authority-test.undeclared.value"); err == nil {
		t.Fatal("canonical but undeclared predicate unexpectedly accepted")
	}
	if err := RequireDeclaredPredicate("authority-test.invalid_value"); err == nil {
		t.Fatal("malformed predicate unexpectedly accepted")
	}
}
