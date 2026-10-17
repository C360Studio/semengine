package graph

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// classifiedLayers counts the *errs.ClassifiedError values on err's unwrap chain above the
// *StateContractError. A refusal of stored graph state carries exactly one (graph-entity-writes,
// "One rule decodes a stored entity"). The walk stops at the state error: its cause is the
// violation itself, and an entity-ID violation from pkg/types is classified on its own.
func classifiedLayers(err error) int {
	layers := 0
	for ; err != nil; err = errors.Unwrap(err) {
		switch err.(type) {
		case *errs.ClassifiedError:
			layers++
		case *StateContractError:
			return layers
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
		`","predicate":"robotics.flight.armed","object":true,"timestamp":"2026-10-16T12:00:00Z"}]},"kv_revision":3}`
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

// An exported struct can be declared without its constructor; that reader refuses every read
// instead of panicking.
func TestZeroExactEntityReaderRefusesReads(t *testing.T) {
	var reader ExactEntityReader
	exact, err := reader.ReadExactEntity(context.Background(), "acme.ops.robotics.gcs.drone.001")
	if err == nil || exact != nil {
		t.Fatalf("zero ExactEntityReader read = (%v, %v), want (nil, error)", exact, err)
	}
}

// storedEntityBytes builds stored bytes by hand, so the test's input is not shaped by the encoder.
// Each statement is {"subject","predicate","source","timestamp"}; an empty source or timestamp is
// left out of the JSON, as a writer that never set it would store it.
func storedEntityBytes(id string, statements ...[4]string) []byte {
	triples := make([]string, 0, len(statements))
	for _, st := range statements {
		triple := `{"subject":"` + st[0] + `","predicate":"` + st[1] + `","object":1`
		if st[2] != "" {
			triple += `,"source":"` + st[2] + `"`
		}
		if st[3] != "" {
			triple += `,"timestamp":"` + st[3] + `"`
		}
		triples = append(triples, triple+"}")
	}
	return []byte(`{"id":"` + id + `","triples":[` + strings.Join(triples, ",") + `]}`)
}

// TestUnmarshalEntityStateRefusesWhatGraphIngestRefuses: the one decoder for a stored entity
// refuses in the spec's order (graph-entity-writes, "One rule decodes a stored entity"): the
// bytes decode, the canonical contract, the key, then statement metadata. One row per reason,
// plus the two adjacent-precedence rows; every refusal is one classified layer naming the key.
// Generated checks are not used: four checks with a fixed precedence make the per-reason rows and
// the adjacent pairs the whole input space (design D1).
func TestUnmarshalEntityStateRefusesWhatGraphIngestRefuses(t *testing.T) {
	const (
		key   = "acme.ops.test.system.widget.001"
		other = "acme.ops.test.system.widget.002"
		src   = "test-producer"
		when  = "2026-10-16T12:00:00Z"
		good  = "test.state.value"
		bad   = "bad.two" // predicate-audit:invalid {"kind":"stored-predicate","value":"bad.two","reason":"arity"}
	)
	cases := []struct {
		name   string
		data   []byte
		reason StateResetReason
	}{
		{"bytes that do not decode", []byte("{"), GraphStateReasonUnreadableEntity},
		{"empty bytes", []byte{}, GraphStateReasonUnreadableEntity},
		{"noncanonical entity ID", storedEntityBytes("bad"), GraphStateReasonNoncanonicalEntityID},
		{"noncanonical predicate", storedEntityBytes(key, [4]string{key, bad, src, when}), GraphStateReasonNoncanonicalPredicate},
		{"another entity under the key", storedEntityBytes(other, [4]string{other, good, src, when}), GraphStateReasonNoncanonicalEntityID},
		{"statement without a source", storedEntityBytes(key, [4]string{key, good, "", when}), GraphStateReasonMissingStatementMetadata},
		{"statement without a timestamp", storedEntityBytes(key, [4]string{key, good, src, ""}), GraphStateReasonMissingStatementMetadata},
		{"key checked before metadata", storedEntityBytes(other, [4]string{other, good, "", when}), GraphStateReasonNoncanonicalEntityID},
		{"contract checked before the key", storedEntityBytes(other, [4]string{other, bad, src, when}), GraphStateReasonNoncanonicalPredicate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var entity EntityState
			requireStateRefusal(t, UnmarshalEntityState(key, tc.data, &entity), tc.reason, key)
		})
	}
	t.Run("a value the rule accepts", func(t *testing.T) {
		var entity EntityState
		if err := UnmarshalEntityState(key, storedEntityBytes(key, [4]string{key, good, src, when}), &entity); err != nil {
			t.Fatalf("UnmarshalEntityState refused a valid value: %v", err)
		}
		if entity.ID != key || len(entity.Triples) != 1 || entity.Triples[0].Source != src {
			t.Fatalf("decoded %+v, want entity %s with its one statement", entity, key)
		}
	})
}

// The aggregate validators apply the stored-value rule's steps that need no key, statement
// metadata included, so a batch or prefix reply is checked like a read of the bucket (design D1;
// owner question 1). The refusal names the candidate's index and has the rule's one shape, with
// no entity ID.
func TestValidateDecodedEntityStatesRefuseStatementWithoutMetadata(t *testing.T) {
	const id = "acme.ops.test.system.widget.001"
	stamped := message.Triple{Subject: id, Predicate: "test.state.value", Object: 1, Source: "test-producer",
		Timestamp: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	unsourced, untimed := stamped, stamped
	unsourced.Source = ""
	untimed.Timestamp = time.Time{}
	for name, triple := range map[string]message.Triple{"source": unsourced, "timestamp": untimed} {
		t.Run("statement without a "+name, func(t *testing.T) {
			entities := []EntityState{
				{ID: id, Triples: []message.Triple{stamped}},
				{ID: id, Triples: []message.Triple{stamped, triple}},
			}
			err := ValidateDecodedEntityStates(entities)
			requireStateRefusal(t, err, GraphStateReasonMissingStatementMetadata, "")
			if err != nil && !strings.HasPrefix(err.Error(), "entity[1]: ") {
				t.Errorf("error %q does not name candidate 1", err)
			}
			if err := ValidateDecodedEntityStatePointers([]*EntityState{&entities[0], &entities[1]}); err == nil {
				t.Error("ValidateDecodedEntityStatePointers accepted the candidate")
			}
		})
	}
}
