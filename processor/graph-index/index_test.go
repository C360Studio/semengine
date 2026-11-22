package graphindex

import (
	"errors"
	"slices"
	"testing"

	"github.com/c360studio/semengine/graph"
)

// TestNoncanonicalRefusalNamesTheKey: the refusal recorded for a key that is not a literal entity
// ID names that key in its detail, as the decoder's refusal names its key.
func TestNoncanonicalRefusalNamesTheKey(t *testing.T) {
	const key = "acme.ops.test.system.widget"
	err := noncanonicalRefusal(key, errors.New("five parts"))
	if !graph.IsStateContractError(err) {
		t.Fatalf("noncanonicalRefusal = %v; want the state contract refusal", err)
	}
	if got := graph.StateRefusalEntityIDs(err); !slices.Equal(got, []string{key}) {
		t.Errorf("StateRefusalEntityIDs = %v; want [%s]", got, key)
	}
}
