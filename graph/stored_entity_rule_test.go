package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/pkg/errs"
)

// classifiedLayers counts the *errs.ClassifiedError values on err's unwrap chain. A refusal of
// stored graph state carries exactly one (graph-entity-writes, "One rule decodes a stored entity").
func classifiedLayers(err error) int {
	layers := 0
	for ; err != nil; err = errors.Unwrap(err) {
		if _, ok := err.(*errs.ClassifiedError); ok {
			layers++
		}
	}
	return layers
}

// requireStateRefusal checks the one refusal shape for stored graph state: one classified layer,
// fatal graph_state_reset_required, over a *StateContractError with reason and entityID.
func requireStateRefusal(t *testing.T, err error, reason StateResetReason, entityID string) {
	t.Helper()
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorFatal ||
		classified.Code != ErrorCodeGraphStateResetRequired {
		t.Fatalf("error = %T %v, want fatal/%s", err, err, ErrorCodeGraphStateResetRequired)
	}
	if n := classifiedLayers(err); n != 1 {
		t.Errorf("error %v carries %d ClassifiedError layers, want 1", err, n)
	}
	var stateErr *StateContractError
	if !errors.As(err, &stateErr) {
		t.Fatalf("error = %T %v, want a *StateContractError", err, err)
	}
	if stateErr.Reason != reason || stateErr.EntityID != entityID {
		t.Errorf("refusal = %s for entity %q, want %s for entity %q", stateErr.Reason, stateErr.EntityID, reason, entityID)
	}
}

// One reply whose entity holds a statement with no source is refused like the stored value it was
// read from: the exact reader applies the stored-value rule, statement metadata included.
func TestExactEntityReaderRefusesAReplyWithoutStatementMetadata(t *testing.T) {
	const entityID = "acme.ops.robotics.gcs.drone.001"
	reply := `{"entity":{"id":"` + entityID + `","triples":[{"subject":"` + entityID +
		`","predicate":"robotics.flight.armed","object":true,"timestamp":"2026-10-16T12:00:00Z"}]},"kvRevision":3}`
	requester := exactEntityRequesterFunc(func(context.Context, string, []byte, time.Duration) ([]byte, error) {
		return []byte(reply), nil
	})
	exact, err := newTestExactEntityReader(t, requester).ReadExactEntity(context.Background(), entityID)
	if exact != nil {
		t.Fatalf("ReadExactEntity delivered %+v for a statement with no source", exact.Entity)
	}
	requireStateRefusal(t, err, GraphStateReasonMissingStatementMetadata, entityID)
}

// A nil requester can never answer, so the constructor refuses it rather than returning a reader
// that refuses every call (graphmutation.NewClient's shape).
func TestNewExactEntityReaderRefusesNilRequester(t *testing.T) {
	reader, err := NewExactEntityReader(nil, time.Second)
	if err == nil || reader != nil {
		t.Fatalf("NewExactEntityReader(nil) = (%v, %v), want (nil, error)", reader, err)
	}
}

// A refusal of stored bytes is classified once: one fatal graph_state_reset_required layer, not a
// classified error wrapped in another.
func TestUnmarshalEntityStateRefusalIsClassifiedOnce(t *testing.T) {
	var entity EntityState
	err := UnmarshalEntityState([]byte("{"), &entity)
	if n := classifiedLayers(err); n != 1 {
		t.Fatalf("UnmarshalEntityState error %v carries %d ClassifiedError layers, want 1", err, n)
	}
}
