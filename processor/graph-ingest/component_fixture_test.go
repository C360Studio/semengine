package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/lifecycle"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/stretchr/testify/require"
)

// testStampKeys are the message types this package's tests stamp on the
// create seams (ADR-103: a stamp the registry does not hold is refused).
// Framework types are registered by their own RegisterPayloads;
// these are the test-only types, registered as schema-less stubs with no floor.
var testStampKeys = []message.Type{
	{Domain: "test", Category: "entity", Version: "v1"},
	{Domain: "test", Category: "widget", Version: "v1"},
	{Domain: "test", Category: "fixture", Version: "v1"},
	{Domain: "test", Category: "mutation", Version: "v1"},
	{Domain: "test", Category: "nofloor", Version: "v1"},
	{Domain: "test", Category: "seed", Version: "v1"},
	{Domain: "test", Category: "merge", Version: "v1"},
	{Domain: "test", Category: "graphable", Version: "v1"},
	{Domain: "test", Category: "storable", Version: "v1"},
	{Domain: "test", Category: "poison", Version: "v1"},
	{Domain: "test", Category: "container", Version: "v1"},
	{Domain: "test", Category: "sensor", Version: "v1"},
	{Domain: "test", Category: "decode", Version: "v1"},
	{Domain: "test", Category: "noop", Version: "v1"},
	{Domain: "test", Category: "revision", Version: "v1"},
	{Domain: "metrictest", Category: "widget", Version: "v1"},
	{Domain: "boid", Category: "telemetry", Version: "v1"},
	{Domain: "workflow", Category: "task-unit", Version: "v1"},
	{Domain: "mission", Category: "command", Version: "v1"},
}

// fixtureSource and fixtureTime are the Source and Timestamp withTestMetadata gives a fixture
// statement that carries none: every statement graph-ingest stores carries both, and a write
// carrying a statement without them is refused (design D15, #98).
const fixtureSource = "graph-ingest-fixture"

var fixtureTime = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// withTestMetadata returns a copy of triples in which each empty Source is fixtureSource and
// each zero Timestamp is fixtureTime; a statement that carries its own keeps it. It is for the
// statements of a test that is not about their source or time.
func withTestMetadata(triples ...message.Triple) []message.Triple {
	stamped := slices.Clone(triples)
	for index := range stamped {
		if stamped[index].Source == "" {
			stamped[index].Source = fixtureSource
		}
		if stamped[index].Timestamp.IsZero() {
			stamped[index].Timestamp = fixtureTime
		}
	}
	return stamped
}

// testEntityType is the stamp for test entities born through CreateEntity.
func testEntityType() message.Type {
	return message.Type{Domain: "test", Category: "entity", Version: "v1"}
}

// registerTestStamps adds every test-only stamp through
// payloadfixture.RegisterTestType — the one stub-type spelling.
func registerTestStamps(tb testing.TB, reg *payloadregistry.Registry) {
	tb.Helper()
	for _, mt := range testStampKeys {
		payloadfixture.RegisterTestType(tb, reg, mt)
	}
}

// newTestPayloadRegistry builds the registry graph-ingest tests inject: the
// framework payloads graph-ingest writes or decodes (message's GenericJSON and
// the hierarchy container inference births), lifecycle.harness.v1 (the stamp
// update_preserve_metadata_test.go creates and reconciles) and every test-only
// stamp. At the pin this was payloadbuiltins' whole builtin set plus
// agentic/research; design D2 takes the per-package RegisterPayloads the tests
// need instead.
func newTestPayloadRegistry(tb testing.TB) *payloadregistry.Registry {
	tb.Helper()
	reg := payloadfixture.NewWithSubset(tb, message.RegisterPayloads, inference.RegisterPayloads, lifecycle.RegisterPayloads)
	registerTestStamps(tb, reg)
	return reg
}

// panicTB is the panic-shaped testing.TB for helpers that have no test in
// scope (the shared lifecycle fixture): Fatalf panics, Helper is a no-op,
// everything else is unreachable from RegisterTestType and NewWithSubset.
type panicTB struct{ testing.TB }

func (panicTB) Helper() {}

func (panicTB) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf("graph-ingest test registry: "+format, args...))
}

// mustTestPayloadRegistry is newTestPayloadRegistry for helpers that have no
// testing.TB in scope.
func mustTestPayloadRegistry() *payloadregistry.Registry {
	return newTestPayloadRegistry(panicTB{})
}

// withTestRegistry gives a Component built as a literal (bypassing the
// factory) the registry and decoder the factory would have set, so the
// fail-closed create seam admits registered test stamps (O-15, tasks 5.4).
func withTestRegistry(tb testing.TB, c *Component) *Component {
	tb.Helper()
	c.payloadRegistry = newTestPayloadRegistry(tb)
	c.decoder = message.NewDecoder(c.payloadRegistry)
	return c
}

// testDeploymentOrg / testDeploymentPlatform are the authority the fixtures in
// this package build a component under (ADR-102): graph-ingest refuses every
// candidate subject whose positions 1-2 differ, so a fixture's deployment pair
// and its entity-ID fixtures are one decision. They match the majority of this
// package's IDs; a file whose fixtures sit under a different pair passes
// withAuthority so the gate compares against the pair those IDs actually use.
const (
	testDeploymentOrg      = "c360"
	testDeploymentPlatform = "platform"
)

// testComponentOption customizes the Dependencies a fixture constructs with.
type testComponentOption func(*component.Dependencies)

// withAuthority points the fixture's authority gate at a different deployment
// pair. Naming it at the call site keeps "which deployment is this?" visible in
// the test rather than buried in a shared default.
func withAuthority(org, platform string) testComponentOption {
	return func(deps *component.Dependencies) {
		deps.Platform = component.PlatformMeta{Org: org, Platform: platform}
	}
}

// testDependencies builds the standard fixture Dependencies: a real payload
// registry, the caller's NATS client, and the deployment authority every
// graph-ingest now requires at construction.
func testDependencies(tb testing.TB, natsClient *natsclient.Client, opts ...testComponentOption) component.Dependencies {
	tb.Helper()
	deps := component.Dependencies{
		NATSClient:      natsClient,
		PayloadRegistry: newTestPayloadRegistry(tb),
		Platform:        component.PlatformMeta{Org: testDeploymentOrg, Platform: testDeploymentPlatform},
	}
	for _, opt := range opts {
		opt(&deps)
	}
	return deps
}

// authorityOfFixture is withAuthority read off a fixture entity ID. Read-path
// tests seed through the production write path, so the component must BE the
// deployment that owns the IDs the table declares; deriving the pair from the
// fixture keeps a heterogeneous table's rows and their component in lockstep
// instead of duplicating the pair in every row.
//
// It is a test convenience and nothing else: ADR-102 d2 forbids exactly this
// read-back on any minting path, which is why no production code does it.
func authorityOfFixture(tb testing.TB, entityID string) testComponentOption {
	tb.Helper()
	parsed, err := semtypes.ParseEntityID(entityID)
	if err != nil {
		tb.Fatalf("fixture entity ID %q is not canonical: %v", entityID, err)
	}
	return withAuthority(parsed.Org, parsed.Platform)
}

// seedEntityState writes an entity straight into ENTITY_STATES, as a mirror of
// what an import lane would have produced. A fixture needs it only for an
// entity under a FOREIGN authority: the in-process write paths are
// authority-bound (ADR-102 d5), and correctly so — an in-process create under a
// peer's pair would be the framework minting under a foreign authority. The
// import lane itself is exercised end-to-end by
// TestImportLaneAcceptsForeignRejectsLocalClaim; here it is only a seed.
func seedEntityState(tb testing.TB, c *Component, entity *graph.EntityState) {
	tb.Helper()
	encoded, err := graph.MarshalEntityState(entity)
	if err != nil {
		tb.Fatalf("marshal mirrored entity %q: %v", entity.ID, err)
	}
	if _, err := c.entityBucket.Put(context.Background(), entity.ID, encoded); err != nil {
		tb.Fatalf("seed mirrored entity %q: %v", entity.ID, err)
	}
}

// seedOwnedOrMirrored writes entity through the production create path when it
// carries this component's authority, and as a mirror when it does not — the
// shape a federated deployment actually holds.
func seedOwnedOrMirrored(tb testing.TB, c *Component, entity *graph.EntityState) {
	tb.Helper()
	if c.authorizeSubject(entity.ID, false) == nil {
		if err := c.CreateEntity(context.Background(), entity); err != nil {
			tb.Fatalf("create owned entity %q: %v", entity.ID, err)
		}
		return
	}
	seedEntityState(tb, c, entity)
}

// The helpers below are defined at the pin in test files stage 2b ports; they move
// here with their first ported reader. newTestContext and newTestEntity are
// entity_validation_test.go:467-488 (newTestEntity without Version, design D15);
// guardTestPoisonBytes, guardTestValidBytes, assertIngestResetRequired and
// poisonInventoryEntry are query_contract_guard_test.go:35-42, 459-465 and 375-381.

func newTestContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func newTestEntity(id string) *graph.EntityState {
	return &graph.EntityState{
		ID:          id,
		MessageType: testEntityType(),
		Triples: withTestMetadata(message.Triple{
			Subject:    id,
			Predicate:  "entity.type.class",
			Object:     "test.entity",
			Confidence: 1.0,
			Timestamp:  time.Now(),
		}),
		UpdatedAt: time.Now(),
	}
}

// guardTestPoisonBytes fails the canonical decode on a noncanonical predicate.
func guardTestPoisonBytes(id string) []byte {
	return []byte(`{"id":"` + id + `","triples":[{"subject":"` + id + `","predicate":"legacy.predicate","object":1}]}`) // predicate-audit:invalid {"kind":"stored-predicate","value":"legacy.predicate","reason":"arity"}
}

func guardTestValidBytes(id string) []byte {
	return []byte(`{"id":"` + id + `","triples":[]}`)
}

func assertIngestResetRequired(t *testing.T, err error) {
	t.Helper()
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorFatal || classified.Code != graph.ErrorCodeGraphStateResetRequired {
		t.Fatalf("error = %T %v, want fatal/%s", err, err, graph.ErrorCodeGraphStateResetRequired)
	}
}

// poisonInventoryEntry reads one inventory record under the inventory lock.
func poisonInventoryEntry(c *Component, id string) (entityPoisonRecord, bool) {
	c.entityPoisonMu.Lock()
	defer c.entityPoisonMu.Unlock()
	rec, ok := c.entityPoison[id]
	return rec, ok
}

// mergeTestGraphable is a minimal Graphable payload that stamps a caller-supplied
// triple set on an entity ID. It and registerMergeTestPayload are in the pin's
// merge_entity_integration_test.go:329-378, which a later stage ports. They came here
// from fixture_integration_test.go when a unit test (statement_metadata_test.go) first
// sent one on the stream lane.
type mergeTestGraphable struct {
	entityID string
	triples  []message.Triple
}

func (g *mergeTestGraphable) EntityID() string          { return g.entityID }
func (g *mergeTestGraphable) Triples() []message.Triple { return g.triples }
func (g *mergeTestGraphable) Schema() message.Type {
	return message.Type{Domain: "test", Category: "merge", Version: "v1"}
}

func (g *mergeTestGraphable) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		EntityID string           `json:"entity_id"`
		Triples  []message.Triple `json:"triples"`
	}{g.entityID, g.triples})
}

func (g *mergeTestGraphable) UnmarshalJSON(data []byte) error {
	var v struct {
		EntityID string           `json:"entity_id"`
		Triples  []message.Triple `json:"triples"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	g.entityID = v.EntityID
	g.triples = v.Triples
	return nil
}

func (g *mergeTestGraphable) Validate() error { return nil }

// registerMergeTestPayload gives c a decoder that decodes test.merge.v1 into a
// mergeTestGraphable. The pin started from payloadbuiltins.Register (:369); design D2
// takes the per-package RegisterPayloads graph-ingest's fixture registry uses instead.
func registerMergeTestPayload(t *testing.T, c *Component) {
	t.Helper()
	reg := payloadfixture.NewWithSubset(t, message.RegisterPayloads, inference.RegisterPayloads)
	require.NoError(t, reg.Register(&payloadregistry.Registration{
		Domain:      "test",
		Category:    "merge",
		Version:     "v1",
		Description: "merge-entity integration-test payload",
		Factory:     func() any { return &mergeTestGraphable{} },
	}))
	c.decoder = message.NewDecoder(reg)
}
