package graphmutation

import "testing"

func TestSubjectAdmitsOnlyFourCanonicalOperations(t *testing.T) {
	want := map[Operation]string{
		CreateEntity:        "graph.mutation.entity.create",
		ReconcilePredicates: "graph.mutation.entity.reconcile",
		AppendTriples:       "graph.mutation.triple.append",
		DeleteEntity:        "graph.mutation.entity.delete",
	}
	for operation, subject := range want {
		got, err := Subject(operation)
		if err != nil {
			t.Fatalf("Subject(%q): %v", operation, err)
		}
		if got != subject {
			t.Fatalf("Subject(%q) = %q, want %q", operation, got, subject)
		}
	}

	for _, retired := range []Operation{
		"",
		"entity.create_with_triples",
		"entity.update",
		"entity.update_with_triples",
		"triple.add",
		"triple.add_batch",
		"triple.remove",
	} {
		if subject, err := Subject(retired); err == nil {
			t.Fatalf("retired operation %q resolved to %q", retired, subject)
		}
	}
}

// The port's interface identity is an outward-facing name: flow configs spell it, and #69
// ruled every such name semengine (design D9).
func TestInterfaceTypeNamesSemEngine(t *testing.T) {
	if InterfaceType != "semengine.graph.mutation" {
		t.Fatalf("InterfaceType = %q, want %q", InterfaceType, "semengine.graph.mutation")
	}
}
