//go:build integration

package rule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it
// after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(15*time.Second),
		natsclient.WithMaxReconnects(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}

// newStateTracker resets the shared server and returns a StateTracker over a fresh
// RULE_STATE bucket, with the bucket itself for the test to read what is stored
// without going through the tracker.
func newStateTracker(t *testing.T) (*StateTracker, jetstream.KeyValue) {
	t.Helper()
	client := natsfixture.Open(t, shared.Reset(t), openClient)
	bucket, err := client.CreateKeyValueBucket(t.Context(), jetstream.KeyValueConfig{Bucket: "RULE_STATE"})
	require.NoError(t, err)
	return NewStateTracker(client.NewKVStore(bucket)), bucket
}

// storedKeys lists the bucket's live keys, sorted, through nats.go's own lister.
func storedKeys(t *testing.T, bucket jetstream.KeyValue) []string {
	t.Helper()
	lister, err := bucket.ListKeys(t.Context())
	require.NoError(t, err)
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}
	// Once the channel has closed, Stop has been seen to report the lister's
	// subscription as invalid (observed, not promised by nats.go); the keys read are
	// complete either way.
	_ = lister.Stop()
	slices.Sort(keys)
	return keys
}

func TestStateTracker(t *testing.T) {
	entityA := semantictest.EntityID(t, "c360", "platform1", "robotics", "mav1", "drone", "a")
	entityB := semantictest.EntityID(t, "c360", "platform1", "robotics", "mav1", "drone", "b")

	t.Run("a record round-trips under rule ID dot entity key", func(t *testing.T) {
		tracker, bucket := newStateTracker(t)
		want := MatchState{
			RuleID:         "low-battery",
			EntityKey:      entityA,
			IsMatching:     true,
			LastTransition: "entered",
			TransitionAt:   time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC),
			SourceRevision: 42,
			LastChecked:    time.Date(2026, 11, 26, 9, 5, 0, 0, time.UTC),
			Iteration:      2,
			MaxIterations:  3,
			FieldValues:    map[string]string{"workflow.plan.status": "approved"},
		}
		require.NoError(t, tracker.Set(t.Context(), want))

		got, err := tracker.Get(t.Context(), "low-battery", entityA)
		require.NoError(t, err)
		assert.Equal(t, want, got)

		// The stored layout operators read by hand: one key, `<rule ID>.<entity key>`,
		// holding the record's JSON field names.
		require.Equal(t, []string{"low-battery." + entityA}, storedKeys(t, bucket))
		entry, err := bucket.Get(t.Context(), "low-battery."+entityA)
		require.NoError(t, err)
		var stored map[string]any
		require.NoError(t, json.Unmarshal(entry.Value(), &stored))
		assert.Equal(t, "low-battery", stored["rule_id"])
		assert.Equal(t, entityA, stored["entity_key"])
		assert.Equal(t, true, stored["is_matching"])
		assert.Equal(t, "entered", stored["last_transition"])
		assert.InDelta(t, 42, stored["source_revision"], 0)
		assert.InDelta(t, 2, stored["iteration"], 0)
		assert.InDelta(t, 3, stored["max_iterations"], 0)
		assert.Equal(t, map[string]any{"workflow.plan.status": "approved"}, stored["field_values"])
		assert.NotContains(t, stored, "action_iterations")
	})

	t.Run("a pair never saved is not found", func(t *testing.T) {
		tracker, _ := newStateTracker(t)
		_, err := tracker.Get(t.Context(), "low-battery", entityA)
		require.ErrorIs(t, err, ErrStateNotFound)
	})

	t.Run("a wildcard entity key or a dotted rule ID is refused and nothing is written", func(t *testing.T) {
		tracker, bucket := newStateTracker(t)
		for _, key := range []struct{ ruleID, entityKey string }{
			{"low-battery", "c360.platform1.robotics.mav1.drone.*"},
			{"low-battery", "c360.platform1.>"},
			{"low.battery", entityA},
			{"", entityA},
			{"low-battery", ""},
		} {
			err := tracker.Set(t.Context(), MatchState{RuleID: key.ruleID, EntityKey: key.entityKey, IsMatching: true})
			require.Error(t, err, "Set(%q, %q)", key.ruleID, key.entityKey)
			assert.True(t, errs.IsInvalid(err), "Set(%q, %q) error %v is not classified invalid", key.ruleID, key.entityKey, err)

			_, err = tracker.Get(t.Context(), key.ruleID, key.entityKey)
			require.Error(t, err, "Get(%q, %q)", key.ruleID, key.entityKey)
			assert.NotErrorIs(t, err, ErrStateNotFound)
			assert.True(t, errs.IsInvalid(err), "Get(%q, %q) error %v is not classified invalid", key.ruleID, key.entityKey, err)
		}
		assert.Empty(t, storedKeys(t, bucket))
	})

	t.Run("a nil context is refused", func(t *testing.T) {
		tracker, bucket := newStateTracker(t)
		var nilCtx context.Context
		err := tracker.Set(nilCtx, MatchState{RuleID: "low-battery", EntityKey: entityA})
		require.Error(t, err)
		assert.True(t, errs.IsInvalid(err), "Set error %v is not classified invalid", err)
		_, err = tracker.Get(nilCtx, "low-battery", entityA)
		require.Error(t, err)
		assert.True(t, errs.IsInvalid(err), "Get error %v is not classified invalid", err)
		err = tracker.DeleteAllForEntity(nilCtx, entityA)
		require.Error(t, err)
		assert.True(t, errs.IsInvalid(err), "DeleteAllForEntity error %v is not classified invalid", err)
		assert.Empty(t, storedKeys(t, bucket))
	})

	t.Run("deleting an entity's records keeps every other record", func(t *testing.T) {
		tracker, bucket := newStateTracker(t)
		// Neighbours whose keys end like entity A's or extend it: an instance with A's
		// instance as a prefix, one with A's whole ID as a dotted prefix, and a key
		// whose entity key ends with A's whole ID.
		underscoreSibling := entityA + "_x"
		dottedSibling := entityA + "0"
		longerKey := "prefix." + entityA
		for _, key := range []struct{ ruleID, entityKey string }{
			{"r1", entityA}, {"r2", entityA}, {"r1", entityB},
			{"r1", underscoreSibling}, {"r1", dottedSibling}, {"r1", longerKey},
		} {
			require.NoError(t, tracker.Set(t.Context(), MatchState{RuleID: key.ruleID, EntityKey: key.entityKey}))
		}

		require.NoError(t, tracker.DeleteAllForEntity(t.Context(), entityA))

		want := []string{"r1." + entityB, "r1." + dottedSibling, "r1." + underscoreSibling, "r1." + longerKey}
		slices.Sort(want)
		assert.Equal(t, want, storedKeys(t, bucket))
		_, err := tracker.Get(t.Context(), "r1", entityA)
		require.ErrorIs(t, err, ErrStateNotFound)
		_, err = tracker.Get(t.Context(), "r1", entityB)
		require.NoError(t, err)
	})

	t.Run("deleting from an empty bucket is nil", func(t *testing.T) {
		tracker, _ := newStateTracker(t)
		require.NoError(t, tracker.DeleteAllForEntity(t.Context(), entityA))
	})

	t.Run("a wildcard entity ID is refused and every record kept", func(t *testing.T) {
		tracker, bucket := newStateTracker(t)
		for _, key := range []struct{ ruleID, entityKey string }{{"r1", entityA}, {"r2", entityB}} {
			require.NoError(t, tracker.Set(t.Context(), MatchState{RuleID: key.ruleID, EntityKey: key.entityKey}))
		}
		want := storedKeys(t, bucket)
		require.Len(t, want, 2)

		for _, entityID := range []string{">", "c360.platform1.robotics.mav1.drone.*", "*", ""} {
			err := tracker.DeleteAllForEntity(t.Context(), entityID)
			require.Error(t, err, "DeleteAllForEntity(%q)", entityID)
			assert.True(t, errs.IsInvalid(err), "DeleteAllForEntity(%q) error %v is not classified invalid", entityID, err)
		}
		assert.Equal(t, want, storedKeys(t, bucket))
	})
}
