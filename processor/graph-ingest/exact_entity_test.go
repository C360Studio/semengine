package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

func TestHandleQueryEntityReturnsEntityAndSameEntryRevision(t *testing.T) {
	component := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
	const entityID = "acme.ops.robotics.gcs.drone.001"
	entity := graph.EntityState{ID: entityID}
	data, err := graph.MarshalEntityState(&entity)
	if err != nil {
		t.Fatalf("MarshalEntityState: %v", err)
	}
	revision, err := component.entityBucket.Put(context.Background(), entityID, data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	response, err := component.handleQueryEntityNATS(context.Background(), []byte(`{"id":"`+entityID+`"}`))
	if err != nil {
		t.Fatalf("handleQueryEntityNATS: %v", err)
	}
	var exact graph.ExactEntity
	if err := json.Unmarshal(response, &exact); err != nil {
		t.Fatalf("Unmarshal ExactEntity: %v", err)
	}
	if exact.Entity == nil || exact.Entity.ID != entityID {
		t.Fatalf("entity = %#v", exact.Entity)
	}
	// EntityState has no Version (design D15): the KV revision is the one
	// revision, so the pin's check that a logical Version was not taken for it
	// has nothing left to compare. The same-entry check below is the claim.
	if exact.KVRevision == 0 || exact.KVRevision != revision {
		t.Fatalf("kv_revision = %d, want same-entry revision %d", exact.KVRevision, revision)
	}
}

func TestHandleQueryEntityRejectsMalformedIDBeforeKVLookup(t *testing.T) {
	component := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
	_, err := component.handleQueryEntityNATS(context.Background(), []byte(`{"id":"not-six-parts"}`))
	if err == nil {
		t.Fatal("malformed entity ID was accepted")
	}
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Code != graph.ErrorCodeInvalidRequest {
		t.Fatalf("error = %v, want invalid_request", err)
	}
}

func TestHandleQueryEntityRejectsAuthorityKeyEntityMismatch(t *testing.T) {
	component := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
	const (
		requestedID = "acme.ops.robotics.gcs.drone.001"
		storedID    = "acme.ops.robotics.gcs.drone.002"
	)
	data, err := graph.MarshalEntityState(&graph.EntityState{ID: storedID})
	if err != nil {
		t.Fatalf("MarshalEntityState: %v", err)
	}
	if _, err := component.entityBucket.Put(context.Background(), requestedID, data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	_, err = component.handleQueryEntityNATS(context.Background(), []byte(`{"id":"`+requestedID+`"}`))
	if !graph.IsStateContractError(err) {
		t.Fatalf("error = %v, want graph-state poison", err)
	}
}

// handlerRequester answers the exact reader's request by calling graph-ingest's entity
// handler, keeping each reply so the test can read its wire form.
type handlerRequester struct {
	handler func(context.Context, []byte) ([]byte, error)
	replies [][]byte
}

func (r *handlerRequester) RequestClassified(ctx context.Context, _ string, data []byte, _ time.Duration) ([]byte, error) {
	reply, err := r.handler(ctx, data)
	if err == nil {
		r.replies = append(r.replies, reply)
	}
	return reply, err
}

// TestExactEntityReplyCarriesKVRevision: graph-ingest's real entity reply carries the
// revision it read under the JSON key kv_revision, and the exact reader returns that
// revision (graph-transport-boundary, "An entity read carries its revision"). The reply
// travels through the reader, so a rename on one side only fails.
func TestExactEntityReplyCarriesKVRevision(t *testing.T) {
	component := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
	const entityID = "acme.ops.robotics.gcs.drone.001"
	data, err := graph.MarshalEntityState(&graph.EntityState{ID: entityID})
	if err != nil {
		t.Fatalf("MarshalEntityState: %v", err)
	}
	if _, err := component.entityBucket.Put(context.Background(), entityID, data); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	// A second write makes the stored revision differ from the first.
	stored, err := component.entityBucket.Put(context.Background(), entityID, data)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}

	requester := &handlerRequester{handler: component.handleQueryEntityNATS}
	reader, err := graph.NewExactEntityReader(requester, time.Second)
	if err != nil {
		t.Fatalf("NewExactEntityReader: %v", err)
	}
	exact, err := reader.ReadExactEntity(context.Background(), entityID)
	if err != nil {
		t.Fatalf("ReadExactEntity: %v", err)
	}
	if len(requester.replies) != 1 {
		t.Fatalf("handler replies = %d, want 1", len(requester.replies))
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(requester.replies[0], &wire); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if got := string(wire["kv_revision"]); got != strconv.FormatUint(stored, 10) {
		t.Fatalf("reply kv_revision = %q, want %d; reply %s", got, stored, requester.replies[0])
	}
	if exact.KVRevision != stored || exact.Entity == nil || exact.Entity.ID != entityID {
		t.Fatalf("exact = {%#v, %d}, want entity %s at revision %d", exact.Entity, exact.KVRevision, entityID, stored)
	}
}
