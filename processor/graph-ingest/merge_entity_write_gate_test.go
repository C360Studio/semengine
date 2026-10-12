package graphingest

// merge_entity_write_gate_test.go — gh#562 write-cost follow-up. MergeEntity
// no longer runs its own full contract pass over the incoming candidate; the
// MarshalEntityState write gate inside the CAS closure is the single
// authoritative full-contract pass per committed candidate (both branches
// marshal a superset of the incoming triples). These tests pin the preserved
// invariant: an invalid candidate NEVER commits, on either closure branch, and
// the failure blames the CALLER's candidate (not resident state — no
// graph-state-reset classification, no poison latch) when the stored entity is
// canonical.

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/stretchr/testify/require"
)

func TestMergeEntity_InvalidCandidateNeverCommits(t *testing.T) {
	validID := "acme.ops.test.system.widget.001"

	badPredicateTriples := withTestMetadata(
		message.Triple{Subject: validID, Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "ok"},
		message.Triple{Subject: validID, Predicate: "not-canonical", Object: "x"}, // predicate-audit:invalid {"kind":"stored-predicate","value":"not-canonical","reason":"arity"}
	)
	badSubjectTriples := withTestMetadata(
		message.Triple{Subject: validID, Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "ok"},
		message.Triple{Subject: "bad", Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "x"},
	)

	cases := []struct {
		name    string
		triples []message.Triple
	}{
		{name: "noncanonical predicate", triples: badPredicateTriples},
		{name: "noncanonical subject", triples: badSubjectTriples},
	}

	for _, tc := range cases {
		t.Run("merge branch "+tc.name, func(t *testing.T) {
			c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
			resident := &graph.EntityState{ID: validID, Triples: withTestMetadata(
				message.Triple{Subject: validID, Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "resident"},
			)}
			require.NoError(t, c.mergeEntityOnLane(context.Background(), resident, false))
			storedBefore, ok := bucket.data[validID]
			require.True(t, ok, "resident seed must exist")

			err := c.mergeEntityOnLane(context.Background(), &graph.EntityState{
				ID: validID, Triples: tc.triples,
			}, false)
			require.Error(t, err, "invalid candidate must not merge")
			require.True(t, errs.IsInvalid(err), "candidate rejection must classify invalid, got: %v", err)
			var stateErr *graph.StateContractError
			require.False(t, errors.As(err, &stateErr),
				"canonical resident state must not be blamed (no reset classification)")
			require.Equal(t, int64(0), c.entityPoisonSize.Load(),
				"caller-invalid candidate must not be inventoried as poison")

			storedAfter := bucket.data[validID]
			require.Equal(t, storedBefore.revision, storedAfter.revision, "no write may commit")
			require.Equal(t, storedBefore.value, storedAfter.value, "stored bytes must be untouched")
		})

		t.Run("create branch "+tc.name, func(t *testing.T) {
			c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))

			err := c.mergeEntityOnLane(context.Background(), &graph.EntityState{
				ID: validID, Triples: tc.triples,
			}, false)
			require.Error(t, err, "invalid candidate must not create")
			require.True(t, errs.IsInvalid(err), "candidate rejection must classify invalid, got: %v", err)

			_, exists := bucket.data[validID]
			require.False(t, exists, "rejected create branch must leave the key absent")
		})
	}
}
