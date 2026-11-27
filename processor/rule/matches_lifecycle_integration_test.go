//go:build integration

package rule

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/lifecycle"
)

// Requirement: rule-matching/"Firing and asking share one match", against a
// real lifecycle manager: most entities are not lifecycle participants, so
// the manager's lookup fails for them, and a rule that reads no lifecycle
// field must answer as it would with no manager at all.
func TestEntityMatchRealManagerOrdinaryRule(t *testing.T) {
	client := natsfixture.Open(t, shared.Reset(t), openClient)
	mgr := lifecycle.NewManager(client, nil)
	r := loadExpression(t, "and", `[{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0}]`)
	state := entityWith(sensorID(t, "020"), message.Triple{Predicate: "sensor.measurement.fahrenheit", Object: 41.2})

	_, err := mgr.LookupByEntityID(t.Context(), state.ID)
	require.True(t, errors.Is(err, lifecycle.ErrEntityNotFound), "the manager does not know the entity: %v", err)

	got, err := r.matchEntity(t.Context(), state, mgr)
	require.NoError(t, err, "a failed lookup does not refuse a rule that reads no lifecycle field")
	assert.True(t, got)
}
