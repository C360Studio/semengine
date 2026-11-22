package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
)

// TestStateRefusalEntityIDs: the helper reads the damaged IDs from a refusal built in this process
// and from one whose detail went through JSON, as a request/reply client rebuilds it, and returns
// nil for every error that is not a damaged-entity refusal.
func TestStateRefusalEntityIDs(t *testing.T) {
	const (
		b = "acme.ops.test.system.widget.002"
		c = "acme.ops.test.system.widget.003"
	)
	var entity EntityState
	inProcess := UnmarshalEntityState(b, []byte("{"), &entity)

	var decoded map[string]any
	if err := json.Unmarshal([]byte(`{"entity_ids":["`+b+`","`+c+`"]}`), &decoded); err != nil {
		t.Fatal(err)
	}
	overTheWire := errs.ClassifiedCodeDetail(errs.ErrorFatal, ErrorCodeGraphStateResetRequired, decoded,
		errors.New("graph_state_reset_required: read encountered 2 poisoned entities"))

	t.Run("refusals", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			err  error
			want []string
		}{
			{"built in process", inProcess, []string{b}},
			{"wrapped", fmt.Errorf("read %s: %w", b, inProcess), []string{b}},
			{"decoded from JSON", overTheWire, []string{b, c}},
		} {
			if got := StateRefusalEntityIDs(tc.err); !slices.Equal(got, tc.want) {
				t.Errorf("%s: StateRefusalEntityIDs = %v, want %v", tc.name, got, tc.want)
			}
		}
	})

	t.Run("the caller's copy", func(t *testing.T) {
		ids := StateRefusalEntityIDs(inProcess)
		if len(ids) != 1 {
			t.Fatalf("StateRefusalEntityIDs = %v, want [%s]", ids, b)
		}
		ids[0] = "changed"
		if got := StateRefusalEntityIDs(inProcess); !slices.Equal(got, []string{b}) {
			t.Errorf("after changing a returned list, StateRefusalEntityIDs = %v, want [%s]", got, b)
		}
	})

	t.Run("other errors", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			err  error
		}{
			{"nil", nil},
			{"unclassified", errors.New("boom")},
			{"another code with entity_ids", errs.ClassifiedCodeDetail(errs.ErrorInvalid, ErrorCodeEntityNotFound,
				map[string]any{"entity_ids": []string{b}}, errors.New("not found"))},
			{"a refusal with no entity", errs.ClassifiedCode(errs.ErrorFatal, ErrorCodeGraphStateResetRequired,
				errors.New("graph_state_reset_required"))},
			{"a list holding a non-string", errs.ClassifiedCodeDetail(errs.ErrorFatal, ErrorCodeGraphStateResetRequired,
				map[string]any{"entity_ids": []any{b, 7.0}}, errors.New("graph_state_reset_required"))},
		} {
			if got := StateRefusalEntityIDs(tc.err); got != nil {
				t.Errorf("%s: StateRefusalEntityIDs = %#v, want nil", tc.name, got)
			}
		}
	})
}
