package rule

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/lifecycle"
	"github.com/c360studio/semengine/vocabulary"
)

// Requirement: rule-matching/"Firing and asking share one match" and
// rule-substitution/"Lifecycle names read the context's lifecycle manager":
// the entity match with condition values filled from the entity and
// lifecycle fields read through the lookup it is given.

// A condition value read from another of the entity's triples gates the
// match on the two being equal; an absent triple fills the value with
// nothing, which is no match and nothing logged.
func TestTripleValueCondition(t *testing.T) {
	log := captureWarnings(t)
	vocabulary.Register("openspec.change.validated")
	vocabulary.Register("openspec.change.revision")
	r := loadExpression(t, "and", `[{"field": "openspec.change.validated", "operator": "eq",
	  "value": "$entity.triple.openspec.change.revision.value", "required": true}]`)
	id := sensorID(t, "010")
	validated := message.Triple{Predicate: "openspec.change.validated", Object: "r42"}

	for _, c := range []struct {
		name    string
		triples []message.Triple
		want    bool
	}{
		{"the revisions are equal", []message.Triple{validated, {Predicate: "openspec.change.revision", Object: "r42"}}, true},
		{"the revision is absent", []message.Triple{validated}, false},
		{"the revisions differ", []message.Triple{validated, {Predicate: "openspec.change.revision", Object: "r43"}}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			fired, err := fire(t, r, entityWith(id, c.triples...))
			require.NoError(t, err)
			assert.Equal(t, c.want, fired, "firing match")

			got, err := r.Matches(t.Context(), entityWith(id, c.triples...))
			require.NoError(t, err)
			assert.Equal(t, c.want, got, "Matches")

			assert.Empty(t, log.take(t), "the match logs nothing")
		})
	}
}

// An ordering operator compares numbers when both sides read as finite
// numbers, text that holds one included, and compares text otherwise.
func TestNumericCompareOfTextObjects(t *testing.T) {
	predicate := semantictest.Predicate(t, "test", "metric", "value")
	vocabulary.Register(predicate)
	id := sensorID(t, "011")

	for _, c := range []struct {
		name     string
		object   any
		operator string
		want     bool
	}{
		// 9.9 and 10 compare one way as numbers and the other as text.
		{"text 9.9 lt 10", "9.9", "lt", true},
		{"text 9.9 gt 10", "9.9", "gt", false},
		{"text 0.4 lt 0.5", "0.4", "lt", true},
		{"text 0.8 gt 0.5", "0.8", "gt", true},
		{"number 9.9 lt 10", 9.9, "lt", true},
		{"number 9.9 gt 10", 9.9, "gt", false},
		{"json.Number 9.9 lt 10", json.Number("9.9"), "lt", true},
		{"json.Number 9.9 gt 10", json.Number("9.9"), "gt", false},
		// Text that is not a finite number is compared as text, and each of
		// these sorts after "10".
		{"text alpha lt 10", "alpha", "lt", false},
		{"text NaN lt 10", "NaN", "lt", false},
		{"text Inf lt 10", "Inf", "lt", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			compareTo := 10.0
			if c.object == "0.4" || c.object == "0.8" {
				compareTo = 0.5
			}
			conditions, err := json.Marshal([]map[string]any{{
				"field": predicate, "operator": c.operator, "value": compareTo, "required": true,
			}})
			require.NoError(t, err)
			r := loadExpression(t, "and", string(conditions))
			state := entityWith(id, message.Triple{Predicate: predicate, Object: c.object})

			fired, err := fire(t, r, state)
			require.NoError(t, err)
			assert.Equal(t, c.want, fired, "firing match")

			got, err := r.Matches(t.Context(), state)
			require.NoError(t, err)
			assert.Equal(t, c.want, got, "Matches")
		})
	}
}

// phaseIsFlying is a condition on the entity's lifecycle phase.
const phaseIsFlying = `[{"field": "$entity.lifecycle.phase", "operator": "eq", "value": "flying"}]`

func TestEntityMatchLifecycle(t *testing.T) {
	t.Run("the phase is read through the lookup", func(t *testing.T) {
		for phase, want := range map[string]bool{"flying": true, "planning": false} {
			id := lifecycleEntityID(t, "001")
			mgr := newFakeManager()
			mgr.seed("mission", &fakeParticipant{entityID: id, phase: phase})

			got, err := loadExpression(t, "and", phaseIsFlying).matchEntity(t.Context(), entityWith(id), mgr)
			require.NoError(t, err, phase)
			assert.Equal(t, want, got, phase)
		}
	})

	// The field is absent, and the condition is not required.
	t.Run("with no lookup the phase is absent and the rule does not match", func(t *testing.T) {
		got, err := fire(t, loadExpression(t, "and", phaseIsFlying), entityWith(lifecycleEntityID(t, "002")))
		require.NoError(t, err)
		assert.False(t, got)
	})

	// Most entities are not lifecycle participants, so the lookup fails for
	// them; a rule that reads no lifecycle field answers as without one.
	t.Run("a lookup that does not know the entity leaves other conditions alone", func(t *testing.T) {
		r := loadExpression(t, "and", `[{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0}]`)
		state := entityWith(lifecycleEntityID(t, "003"), message.Triple{Predicate: "sensor.measurement.fahrenheit", Object: 41.2})

		got, err := r.matchEntity(t.Context(), state, newFakeManager())
		require.NoError(t, err)
		assert.True(t, got)
	})

	// A lookup blocked on its backend returns when the caller's context
	// ends, and the match returns with it; its lifecycle field is then
	// absent.
	t.Run("a blocked lookup returns with the caller's context", func(t *testing.T) {
		r := loadExpression(t, "and", phaseIsFlying)
		state := entityWith(lifecycleEntityID(t, "004"))
		lookup := &blockingLookup{fakeLifecycleManager: newFakeManager(), started: make(chan struct{})}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		type answer struct {
			matched bool
			err     error
		}
		done := make(chan answer, 1)
		go func() {
			matched, err := r.matchEntity(ctx, state, lookup)
			done <- answer{matched, err}
		}()

		<-lookup.started
		cancel()
		got := <-done
		require.NoError(t, got.err)
		assert.False(t, got.matched)
	})

	// The condition's value is filled by a second lookup, in substitution;
	// when that one ends with the caller's context, the value is unfilled
	// and the match is an error, not a verdict.
	t.Run("a condition value's lookup returns with the caller's context", func(t *testing.T) {
		id := lifecycleEntityID(t, "005")
		base := newFakeManager()
		base.seed("mission", &fakeParticipant{entityID: id, phase: "flying"})
		lookup := &secondLookupBlocks{fakeLifecycleManager: base, secondStarted: make(chan struct{})}
		r := loadExpression(t, "and",
			`[{"field": "$entity.lifecycle.phase", "operator": "eq", "value": "$entity.lifecycle.phase"}]`)
		state := entityWith(id)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := r.matchEntity(ctx, state, lookup)
			done <- err
		}()

		<-lookup.secondStarted
		cancel()
		err := <-done
		require.Error(t, err)
		assert.Contains(t, err.Error(), "$entity.lifecycle.phase")
		// The receive from done orders this read after both lookups.
		assert.Equal(t, int64(2), lookup.calls.Load())
	})
}

// blockingLookup blocks every lookup until its context ends, closing started
// when the first begins.
type blockingLookup struct {
	*fakeLifecycleManager
	started chan struct{}
}

func (b *blockingLookup) LookupByEntityID(ctx context.Context, _ string) (lifecycle.Participant, error) {
	close(b.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

// secondLookupBlocks answers the first lookup from the fake manager and
// blocks the second until its context ends, closing secondStarted when it
// begins.
type secondLookupBlocks struct {
	*fakeLifecycleManager
	calls         atomic.Int64
	secondStarted chan struct{}
}

func (m *secondLookupBlocks) LookupByEntityID(ctx context.Context, entityID string) (lifecycle.Participant, error) {
	if m.calls.Add(1) == 1 {
		return m.fakeLifecycleManager.LookupByEntityID(ctx, entityID)
	}
	close(m.secondStarted)
	<-ctx.Done()
	return nil, ctx.Err()
}
