//go:build integration

package lifecycle

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
)

// graphIngestStopBudget bounds graph-ingest's Stop when a test ends.
const graphIngestStopBudget = 10 * time.Second

// gateMission is the participant this test births; it mirrors the package's
// internal fixture so the production projection path is exercised unchanged.
type gateMission struct {
	ID         string    `json:"entity_id" lifecycle:"id"`
	PhaseF     string    `json:"phase" lifecycle:"phase,predicate=mission.lifecycle.phase"`
	OwnerOrgID string    `json:"owner_org_id,omitempty" lifecycle:"operator_writable,predicate=mission.identity.owner-org-id"`
	LastAt     time.Time `json:"last_at,omitempty" lifecycle:"readonly,predicate=mission.transition.at"`
}

func (m *gateMission) EntityID() string       { return m.ID }
func (m *gateMission) Workflow() string       { return "gate-fixture" }
func (m *gateMission) Phase() string          { return m.PhaseF }
func (m *gateMission) IsTerminal() bool       { return false }
func (m *gateMission) ParentEntityID() string { return "" }

func gateWorkflow() Workflow {
	return Workflow{
		Name:            "gate-fixture",
		EntityIDPattern: "*.*.lifecycle.gcs.mission.*",
		Phases:          []string{"planning", "flying", "completed"},
		Transitions: Transitions{
			"planning":  {"flying"},
			"flying":    {"completed"},
			"completed": {},
		},
		PhasePredicate: "mission.lifecycle.phase",
		Schema:         reflect.TypeOf(gateMission{}),
		OperatorWritablePredicates: []string{
			"mission.identity.owner-org-id",
		},
		AuditPredicates: AuditSpec{ // predicate-audit:unrelated {"column":20,"surface":"go-field:AuditPredicates","value":"","basis":"reviewed:predicate-container-values-audited"}
			Source: "mission.transition.source",
			At:     "mission.transition.at",
			From:   "mission.transition.from",
			Note:   "mission.transition.note",
		},
	}
}

// ingestClient replaces the pin's natsclient.NewTestClient(t, natsclient.WithKV(),
// natsclient.WithStreams(ENTITY on entity.>)) (design D3): a broker of this test's own, the
// ENTITY stream graph-ingest's default input port reads, and a client opened with openClient
// through natsfixture.Open, which bounds the connect and closes the client on t.Cleanup before
// the fixture stops. graph-ingest creates ENTITY_STATES itself.
func ingestClient(t *testing.T) *natsclient.Client {
	t.Helper()
	f := natsfixture.New(t)
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("natsfixture Start: %v", err)
	}
	if _, err := f.CreateStream(t.Context(), "ENTITY", "entity.>"); err != nil {
		t.Fatalf("CreateStream ENTITY: %v", err)
	}
	return natsfixture.Open(t, f, openClient)
}

// startGraphIngest starts a real graph-ingest with its default configuration on client, under
// the c360.platform1 authority, its payload registry holding only lifecycle.RegisterPayloads
// (design D2) and its collectors on metrics (design D4). It returns the function that stops it.
// The caller defers that function, so Stop runs while ctx, Start's authority, is still live and
// before the client closes; the pin's t.Cleanup ran Stop under an unbounded
// context.Background() after the test's context had ended.
func startGraphIngest(ctx context.Context, t *testing.T, client *natsclient.Client, metrics *metric.MetricsRegistry) (stop func()) {
	t.Helper()
	configJSON, err := json.Marshal(graphingest.DefaultConfig())
	require.NoError(t, err)
	created, err := graphingest.CreateGraphIngest(configJSON, component.Dependencies{
		NATSClient:      client,
		PayloadRegistry: payloadfixture.NewWithSubset(t, RegisterPayloads),
		MetricsRegistry: metrics,
		// graph-ingest refuses an absent deployment authority (ADR-102 d5) and
		// rejects any subject outside it, so the fixture pair must match the
		// entity IDs this file uses.
		Platform: component.PlatformMeta{Org: "c360", Platform: "platform1"},
	})
	require.NoError(t, err)
	ingest := created.(*graphingest.Component)
	require.NoError(t, ingest.Initialize())
	require.NoError(t, ingest.Start(ctx))
	return func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), graphIngestStopBudget)
		defer cancel()
		if err := ingest.Stop(stopCtx); err != nil {
			t.Errorf("graph-ingest Stop: %v", err)
		}
	}
}

// unregisteredRejections sums graph-ingest's
// mutation_rejections_total{reason="message_type_unregistered"} across subjects, gathered from
// the registry graph-ingest was built with. The pin gathered from prometheus.DefaultGatherer,
// which graph-ingest no longer registers on (design D4), and matched the semstreams_ name (#69).
func unregisteredRejections(t *testing.T, metrics *metric.MetricsRegistry) float64 {
	t.Helper()
	families, err := metrics.PrometheusRegistry().Gather()
	require.NoError(t, err)
	var total float64
	for _, family := range families {
		if family.GetName() != "semengine_graph_ingest_mutation_rejections_total" {
			continue
		}
		for _, series := range family.GetMetric() {
			for _, label := range series.GetLabel() {
				if label.GetName() == "reason" && label.GetValue() == graph.ErrorCodeMessageTypeUnregistered {
					total += series.GetCounter().GetValue()
				}
			}
		}
	}
	return total
}

// TestHarnessBirthPassesRegisteredTypeGate drives Manager.Create against a
// REAL graph-ingest constructed with lifecycle's own payload registration: the birth is
// admitted, the stored stamp is lifecycle.harness.v1, and the
// message_type_unregistered rejection counter does not move. A
// HarnessMessageType() that RegisterPayloads does not register fails here.
func TestHarnessBirthPassesRegisteredTypeGate(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	client := ingestClient(t)
	metrics := metric.NewMetricsRegistry()
	stop := startGraphIngest(ctx, t, client, metrics)
	defer stop()
	require.NoError(t, client.GetConnection().Flush())

	before := unregisteredRejections(t, metrics)

	mgr := NewManager(client, nil)
	require.NoError(t, mgr.Register(gateWorkflow()))
	const id = "c360.platform1.lifecycle.gcs.mission.gate-001"
	require.NoError(t, mgr.Create(ctx, &gateMission{ID: id, PhaseF: "planning", OwnerOrgID: "acme"}),
		"a harness birth must pass graph-ingest's registered-type gate")

	js, err := client.JetStream()
	require.NoError(t, err)
	kv, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)
	entry, err := kv.Get(ctx, id)
	require.NoError(t, err)
	var stored graph.EntityState
	require.NoError(t, json.Unmarshal(entry.Value(), &stored))
	assert.Equal(t, HarnessMessageType(), stored.MessageType)
	assert.Equal(t, "lifecycle.harness.v1", stored.MessageType.Key())

	assert.Equal(t, before, unregisteredRejections(t, metrics),
		"mutation_rejections_total{reason=message_type_unregistered} must not move for a harness birth")
}
