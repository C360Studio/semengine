//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

const gateCreateSubject = "graph.mutation.entity.create"

// gateRequestType and gateLessonType stand in for the agentic samples the pin used,
// agentic.request.v1 (trace floor) and agentic.agent_lesson.v1 (content floor): design
// D2 replaces them with test-registered types with the same indexing profile. They are
// registered through payloadregistry.Registration directly, because
// payloadfixture.RegisterTestType registers no floor.
var (
	gateRequestType = message.Type{Domain: "test", Category: "request", Version: "v1"}
	gateLessonType  = message.Type{Domain: "test", Category: "lesson", Version: "v1"}
)

// gateTestRegistry replaces the pin's payloadbuiltins.NewTestRegistry (design D2): the
// per-package RegisterPayloads graph-ingest's fixture registry uses, and the two
// floored stand-ins above.
func gateTestRegistry(t *testing.T) *payloadregistry.Registry {
	t.Helper()
	reg := payloadfixture.NewWithSubset(t, message.RegisterPayloads, inference.RegisterPayloads)
	for _, floored := range []struct {
		mt    message.Type
		floor string
	}{
		{gateRequestType, vocabulary.IndexingProfileTrace},
		{gateLessonType, vocabulary.IndexingProfileContent},
	} {
		require.NoError(t, reg.Register(&payloadregistry.Registration{
			Domain: floored.mt.Domain, Category: floored.mt.Category, Version: floored.mt.Version,
			Description:     "test type registered with the " + floored.floor + " floor",
			IndexingProfile: floored.floor,
			Factory:         func() any { return &struct{}{} },
		}))
	}
	return reg
}

// startGateTestComponent boots graph-ingest over a real NATS testcontainer with
// the supplied payload registry, serving the mutation subjects.
func startGateTestComponent(ctx context.Context, t *testing.T, reg *payloadregistry.Registry, enableHierarchy bool, opts ...testComponentOption) (*Component, *natsclient.Client, *graphIngestTestOwner) {
	t.Helper()

	natsClient := newFixtureClient(t, entityStream)

	config := DefaultConfig()
	config.EnableHierarchy = enableHierarchy
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	deps := testDependencies(t, natsClient, opts...)
	deps.PayloadRegistry = reg // the caller's registry is the point of this fixture
	comp, err := CreateGraphIngest(configJSON, deps)
	require.NoError(t, err)
	c := comp.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.provisionalFinish(ctx, t)
	require.NoError(t, c.Initialize())
	require.NoError(t, c.Start(owner.startContext(ctx)))
	require.NoError(t, natsClient.GetConnection().Flush())
	owner.transfer()
	return c, natsClient, owner
}

func gateMutationClient(t *testing.T, nc *natsclient.Client) *graphmutation.Client {
	t.Helper()
	client, err := graphmutation.NewClient(nc, 5*time.Second)
	require.NoError(t, err)
	return client
}

func gateCreateRequest(id string, mt message.Type) graph.CreateEntityRequest {
	now := time.Now()
	return graph.CreateEntityRequest{
		Entity:  &graph.EntityState{ID: id, MessageType: mt, UpdatedAt: now},
		Triples: withTestMetadata(message.Triple{Subject: id, Predicate: "test.state.value", Object: "born", Timestamp: now, Confidence: 1}),
	}
}

// TestCreateRejectsUnregisteredMessageType: an unregistered stamp never reaches
// ENTITY_STATES — the reply carries the closed code and the key in detail, the
// rejection is metered exactly once, and no key is created.
func TestCreateRejectsUnregisteredMessageType(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	c, nc, owner := startGateTestComponent(ctx, t, gateTestRegistry(t), false, withAuthority("c360", "test"))
	defer owner.finish(ctx, t)
	// The pin read getMutationRejectionsMetric(nil); the counter is the component's
	// own now (design D4), as is the indexing-profile counter below.
	counter := c.mutationRejections.WithLabelValues(gateCreateSubject, graph.ErrorCodeMessageTypeUnregistered)
	before := testutil.ToFloat64(counter)

	const id = "c360.test.gate.system.widget.unregistered"
	unknown := message.Type{Domain: "test", Category: "unknown", Version: "v1"}
	_, err := gateMutationClient(t, nc).Create(ctx, gateCreateRequest(id, unknown))
	require.Error(t, err, "an unregistered stamp must be refused over the wire")

	var ce *errs.ClassifiedError
	require.ErrorAs(t, err, &ce, "the reply decodes into a classified error")
	assert.Equal(t, graph.ErrorCodeMessageTypeUnregistered, ce.Code)
	assert.Equal(t, "test.unknown.v1", ce.Detail["message_type"])
	assert.True(t, errs.IsInvalid(err), "the caller registers the type; it does not retry")

	_, getErr := c.entityBucket.Get(ctx, id)
	require.Error(t, getErr, "nothing may be persisted for the rejected create")
	assert.True(t, errors.Is(getErr, natsclient.ErrKVKeyNotFound))

	assert.InDelta(t, before+1, testutil.ToFloat64(counter), 0.0001,
		"mutation_rejections_total{reason=message_type_unregistered} increments exactly once")
}

// TestCreateAcceptsRegisteredMessageType: a registered stamp is born unchanged.
func TestCreateAcceptsRegisteredMessageType(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	c, nc, owner := startGateTestComponent(ctx, t, gateTestRegistry(t), false, withAuthority("c360", "test"))
	defer owner.finish(ctx, t)

	const id = "c360.test.gate.system.lesson.registered"
	response, err := gateMutationClient(t, nc).Create(ctx, gateCreateRequest(id, gateLessonType))
	require.NoError(t, err)
	assert.Equal(t, graph.MutationApplied, response.Outcome)

	stored := storedEntity(t, c, id)
	assert.Equal(t, gateLessonType, stored.MessageType, "the stamp is persisted verbatim")
}

// TestFloorComesFromRegistration: the indexing-profile floor is read from the
// registered type; a registered type with no floor falls to control and is
// metered, a registered floor is not.
func TestFloorComesFromRegistration(t *testing.T) {
	reg := gateTestRegistry(t)
	payloadfixture.RegisterTestType(t, reg, message.Type{Domain: "test", Category: "nofloor", Version: "v1"})
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	c, nc, owner := startGateTestComponent(ctx, t, reg, false, withAuthority("c360", "test"))
	defer owner.finish(ctx, t)
	client := gateMutationClient(t, nc)

	t.Run("registered floor is stamped without a metric", func(t *testing.T) {
		counter := c.indexingProfileDefault.WithLabelValues(gateRequestType.Key())
		before := testutil.ToFloat64(counter)
		const id = "c360.test.gate.system.request.floor"
		_, err := client.Create(ctx, gateCreateRequest(id, gateRequestType))
		require.NoError(t, err)
		assert.Equal(t, []string{vocabulary.IndexingProfileTrace}, profileValues(storedEntity(t, c, id)))
		assert.InDelta(t, before, testutil.ToFloat64(counter), 0.0001, "a registered floor is not a gap")
	})

	// The pin's subtest was "a framework mutation-lane type takes its registered floor",
	// with agentic.agent_lesson.v1; its stand-in is a test type (design D2).
	t.Run("a registered content floor is stamped without a metric", func(t *testing.T) {
		counter := c.indexingProfileDefault.WithLabelValues(gateLessonType.Key())
		before := testutil.ToFloat64(counter)
		const id = "c360.test.gate.agent.lesson.floor"
		_, err := client.Create(ctx, gateCreateRequest(id, gateLessonType))
		require.NoError(t, err)
		assert.Equal(t, []string{vocabulary.IndexingProfileContent}, profileValues(storedEntity(t, c, id)),
			"a lesson is born with the content floor registered with test.lesson.v1")
		assert.InDelta(t, before, testutil.ToFloat64(counter), 0.0001, "a registered floor is not a gap")
	})

	t.Run("registered type with no floor is metered", func(t *testing.T) {
		counter := c.indexingProfileDefault.WithLabelValues("test.nofloor.v1")
		before := testutil.ToFloat64(counter)
		const id = "c360.test.gate.system.nofloor.001"
		_, err := client.Create(ctx, gateCreateRequest(id, message.Type{Domain: "test", Category: "nofloor", Version: "v1"}))
		require.NoError(t, err)
		assert.Equal(t, []string{vocabulary.IndexingProfileControl}, profileValues(storedEntity(t, c, id)))
		assert.InDelta(t, before+1, testutil.ToFloat64(counter), 0.0001, "a registered type without a floor is the metered gap")
	})
}

// TestHierarchyContainerBirthCarriesRegisteredType (O-16 (a)): a container born
// by graph-ingest's own in-process lane carries graph.hierarchy_container.v1 and
// the unknown-label metric does not fire.
func TestHierarchyContainerBirthCarriesRegisteredType(t *testing.T) {
	reg := gateTestRegistry(t)
	payloadfixture.RegisterTestType(t, reg, message.Type{Domain: "test", Category: "entity", Version: "v1"})
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	c, _, owner := startGateTestComponent(ctx, t, reg, true)
	defer owner.finish(ctx, t)
	unknown := c.indexingProfileDefault.WithLabelValues("unknown")
	before := testutil.ToFloat64(unknown)

	const id = "c360.platform.robotics.mav1.drone.gate001"
	now := time.Now()
	require.NoError(t, c.CreateEntity(ctx, &graph.EntityState{
		ID: id, MessageType: message.Type{Domain: "test", Category: "entity", Version: "v1"},
		Triples:   withTestMetadata(message.Triple{Subject: id, Predicate: "entity.type.class", Object: "test.entity", Timestamp: now, Confidence: 1}),
		UpdatedAt: now,
	}))

	container := storedEntity(t, c, "c360.platform.robotics.mav1.drone.group")
	assert.Equal(t, inference.HierarchyContainerMessageType(), container.MessageType)
	assert.Equal(t, "graph.hierarchy_container.v1", container.MessageType.Key())
	assert.Equal(t, []string{vocabulary.IndexingProfileControl}, profileValues(container), "the container takes its registered floor")
	assert.InDelta(t, before, testutil.ToFloat64(unknown), 0.0001,
		"indexing_profile_default_total{message_type=unknown} must not fire for the framework's own writer")
}
