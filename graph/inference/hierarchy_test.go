package inference

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hierarchyTestOrg / hierarchyTestPlatform are the deployment authority every
// HierarchyInference is constructed with (ADR-102): inference mints containers
// from the ingested entity's own prefix, so it must know
// which prefix is this deployment's. They match positions 1-2 of every entity
// ID in this package's hierarchy fixtures — change one and the other must
// follow, or the fixture becomes an import and mints nothing.
//
// The platform is a DEPLOYMENT id, not a product or domain name: position 2 is
// the composition root's own platform.id (ADR-102 d2), and the framework's own
// exemplar should not model the habit the change exists to retire. "logistics"
// here would read as a product; the taxonomy it belongs to lives at position 4,
// where these fixtures already put it.
const (
	hierarchyTestOrg      = "c360"
	hierarchyTestPlatform = "semstreams-hierarchy-test"
)

// hierarchyTestAuthority is the carrier graph-ingest passes from deps.Platform.
var hierarchyTestAuthority = types.PlatformMeta{Org: hierarchyTestOrg, Platform: hierarchyTestPlatform}

// hierarchyTestTime is the time of the write that triggers an inference.
var hierarchyTestTime = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

// newTestInference builds the inference over store for the test deployment.
func newTestInference(t *testing.T, store EntityStore) *HierarchyInference {
	t.Helper()
	hi, err := NewHierarchyInference(store, hierarchyTestAuthority, nil)
	require.NoError(t, err)
	return hi
}

// addHierarchy does what graph-ingest does at a birth: it asks for the entity's
// hierarchy statements and writes the entity with them, so a test can count
// every statement a birth stores in one place.
func addHierarchy(ctx context.Context, hi *HierarchyInference, store *fakeStore, entityID string) error {
	triples, err := hi.AddToContainers(ctx, entityID, hierarchyTestTime)
	if err != nil {
		return err
	}
	store.writeEntity(entityID, triples)
	return nil
}

// statementCount returns how many of triples state predicate with object.
func statementCount(triples []message.Triple, predicate string, object any) int {
	n := 0
	for _, triple := range triples {
		if triple.Predicate == predicate && triple.Object == object {
			n++
		}
	}
	return n
}

// fakeStore is graph-ingest as the inference sees it through EntityStore, and it
// refuses what graph-ingest refuses (#134 item 4): CreateEntity of a key the
// store holds returns natsclient.ErrKVKeyExists as is, as graph-ingest's create
// does (createEntityWithReceipt returns the bucket's conflict sentinel
// unwrapped).
//
// createErr, when set, is what CreateEntity returns; failOn, when set, limits it
// to creates of that ID.
type fakeStore struct {
	mu        sync.Mutex
	entities  map[string]*gtypes.EntityState // what the store holds, by ID
	created   []*gtypes.EntityState          // each create that committed, as given
	creates   []string                       // the ID of every create, refused or not
	triples   []message.Triple               // each statement writeEntity stored
	createErr error
	failOn    string
}

func newFakeStore() *fakeStore {
	return &fakeStore{entities: make(map[string]*gtypes.EntityState)}
}

// injected returns err when it is set and the call is about the ID the store
// fails on (any ID when failOn is empty).
func (s *fakeStore) injected(err error, id string) error {
	if s.failOn != "" && id != s.failOn {
		return nil
	}
	return err
}

func (s *fakeStore) CreateEntity(_ context.Context, entity *gtypes.EntityState) error {
	if err := s.injected(s.createErr, entity.ID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates = append(s.creates, entity.ID)
	if _, exists := s.entities[entity.ID]; exists {
		return natsclient.ErrKVKeyExists
	}
	s.entities[entity.ID] = entity.Clone()
	s.created = append(s.created, entity)
	return nil
}

// writeEntity stores the entity id with triples, as graph-ingest writes a born
// entity with the statements the inference returned, and records each one.
func (s *fakeStore) writeEntity(id string, triples []message.Triple) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entity, exists := s.entities[id]
	if !exists {
		entity = &gtypes.EntityState{ID: id}
		s.entities[id] = entity
	}
	entity.Triples = append(entity.Triples, triples...)
	s.triples = append(s.triples, triples...)
}

func (s *fakeStore) getCreatedEntities() []*gtypes.EntityState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.created)
}

func (s *fakeStore) createCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.creates)
}

func (s *fakeStore) getTriples() []message.Triple {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.triples)
}

// addExistingEntity puts an entity with no statements in the store, unless the
// store already holds it.
func (s *fakeStore) addExistingEntity(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.entities[id]; !exists {
		s.entities[id] = &gtypes.EntityState{ID: id}
	}
}

// deleteEntity removes an entity, as graph-ingest's delete does.
func (s *fakeStore) deleteEntity(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entities, id)
}

// entity returns a copy of what the store holds under id.
func (s *fakeStore) entity(id string) (*gtypes.EntityState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entity, exists := s.entities[id]
	if !exists {
		return nil, false
	}
	return entity.Clone(), true
}

// TestNewHierarchyInferenceRefusesEmptyAuthority pins the third of the three
// LOUD paths the migration note promises (review HIGH-3). Deleting all three
// refusals in one compiling mutant previously left six suites green.
//
// An inference holding no authority pair could only answer "everything is
// foreign" and mint nothing, for every entity, forever — the silent shape this
// whole change exists to remove. It is a construction mistake, so construction
// refuses it (#134 item 1). graph-ingest cannot reach it (its factory refuses
// an absent deps.Platform first), which is exactly why the branch needs its own
// test rather than an integration one.
func TestNewHierarchyInferenceRefusesEmptyAuthority(t *testing.T) {
	for _, tc := range []struct {
		name     string
		org      string
		platform string
	}{
		{"both absent", "", ""},
		{"org absent", "", hierarchyTestPlatform},
		{"platform absent", hierarchyTestOrg, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hi, err := NewHierarchyInference(newFakeStore(),
				types.PlatformMeta{Org: tc.org, Platform: tc.platform}, nil)

			require.Error(t, err,
				"an inference with no deployment authority must be refused, "+
					"not silently mint nothing for every entity")
			assert.ErrorIs(t, err, errHierarchyAuthorityUnset)
			assert.Nil(t, hi)
		})
	}
}

// TestAddToContainersSkipsForeignAuthority is the DISCRIMINATING test for
// the ADR-102 skip, and it lives here rather than at the graph-ingest seam
// because there the skip is not named. Without it, graph-ingest's own authority
// gate refuses every container birth under a peer's pair, AddToContainers
// returns a joined error, and graph-ingest refuses the imported entity's birth
// (design D21): at the seam a deleted check shows as an import that never
// lands, and here as the skip itself.
//
// It also covers the case graph-ingest cannot: this is exported framework
// surface, and a consumer calling AddToContainers directly has no second
// layer behind it.
func TestAddToContainersSkipsForeignAuthority(t *testing.T) {
	store := newFakeStore()
	hi := newTestInference(t, store)

	// A peer deployment's entity: same org, different platform, canonical shape.
	const imported = hierarchyTestOrg + ".dep9.sensor.document.temperature.sensor-001"

	triples, err := hi.AddToContainers(context.Background(), imported, hierarchyTestTime)

	require.NoError(t, err, "a foreign entity is skipped, not rejected")
	assert.Empty(t, triples, "no membership triple may be minted for an imported entity")
	assert.Empty(t, store.createCalls(),
		"no container entity may be born under a peer's authority")

	// The same shape under THIS deployment's authority still mints, so the skip
	// is authority-scoped rather than a blanket disable.
	local := hierarchyTestOrg + "." + hierarchyTestPlatform + ".sensor.document.temperature.sensor-001"
	localTriples, err := hi.AddToContainers(context.Background(), local, hierarchyTestTime)
	require.NoError(t, err)
	assert.NotEmpty(t, localTriples, "a local entity still receives hierarchy triples")
}

func TestHierarchyInference_InvalidEntityID(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	// 5-part entity ID should be skipped
	err := addHierarchy(context.Background(), hi, store, "c360.semstreams-hierarchy-test.sensor.document.temperature")
	require.NoError(t, err)
	assert.Empty(t, store.getTriples())

	// 7-part entity ID should be skipped
	err = addHierarchy(context.Background(), hi, store, "c360.semstreams-hierarchy-test.sensor.document.temperature.zone.sensor")
	require.NoError(t, err)
	assert.Empty(t, store.getTriples())
}

func TestHierarchyInference_AllLevels(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	entityID := "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001"
	err := addHierarchy(context.Background(), hi, store, entityID)
	require.NoError(t, err)

	// Should create 3 containers
	createdEntities := store.getCreatedEntities()
	assert.Len(t, createdEntities, 3)

	containerIDs := make(map[string]bool)
	for _, e := range createdEntities {
		containerIDs[e.ID] = true
	}
	assert.True(t, containerIDs["c360.semstreams-hierarchy-test.sensor.document.temperature.group"]) // Type
	assert.True(t, containerIDs["c360.semstreams-hierarchy-test.sensor.document.group.container"])   // System
	assert.True(t, containerIDs["c360.semstreams-hierarchy-test.sensor.group.container.level"])      // Domain

	// Should store 3 edges, the forward (member) ones, with the entity
	triples := store.getTriples()
	assert.Len(t, triples, 3)
	for _, tr := range triples {
		assert.Equal(t, "inference.hierarchy", tr.Context, "%s %s", tr.Subject, tr.Predicate)
		assert.Equal(t, 1.0, tr.Confidence, "%s %s", tr.Subject, tr.Predicate)
	}

	// Extract forward edges (entity → member → container)
	forwardPredicates := make(map[string]string) // predicate-audit:unrelated {"column":23,"surface":"go-assignment:forwardPredicates","value":"","basis":"reviewed output map populated from inferred triples"}
	for _, tr := range triples {
		if tr.Subject == entityID {
			forwardPredicates[tr.Predicate] = tr.Object.(string)
		}
	}
	assert.Len(t, forwardPredicates, 3)
	assert.Equal(t, "c360.semstreams-hierarchy-test.sensor.document.temperature.group", forwardPredicates[vocabulary.HierarchyTypeMember])
	assert.Equal(t, "c360.semstreams-hierarchy-test.sensor.document.group.container", forwardPredicates[vocabulary.HierarchySystemMember])
	assert.Equal(t, "c360.semstreams-hierarchy-test.sensor.group.container.level", forwardPredicates[vocabulary.HierarchyDomainMember])

	// No inverse edge (ruling M, #91 comment 6080973822): each container holds
	// only the type statement it was born with.
	assertContainersHoldOnlyTheirType(t, store, "c360.semstreams-hierarchy-test.sensor.document.temperature.group",
		"c360.semstreams-hierarchy-test.sensor.document.group.container",
		"c360.semstreams-hierarchy-test.sensor.group.container.level")
}

// assertContainersHoldOnlyTheirType checks that each container is stored with
// the one statement it was born with, its type: a member's birth wrote nothing
// to it.
func assertContainersHoldOnlyTheirType(t *testing.T, store *fakeStore, containerIDs ...string) {
	t.Helper()
	for _, containerID := range containerIDs {
		container, exists := store.entity(containerID)
		require.True(t, exists, "container %s is stored", containerID)
		assert.Equal(t, 1, statementCount(container.Triples, "entity.type.class", "hierarchy.container"), containerID)
		assert.Len(t, container.Triples, 1, "container %s holds a statement besides its type", containerID)
	}
}

func TestHierarchyInference_ContainerReuse(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	// Create first entity
	err := addHierarchy(context.Background(), hi, store, "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// Create second entity with same type prefix
	err = addHierarchy(context.Background(), hi, store, "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-002")
	require.NoError(t, err)

	// Should only create the three containers once (reused)
	createdEntities := store.getCreatedEntities()
	assert.Len(t, createdEntities, 3)

	// Should have 6 edges: 2 x 3 forward (member), stored with each entity; the
	// second birth wrote nothing to the containers the first one created
	triples := store.getTriples()
	assert.Len(t, triples, 6)
	assertContainersHoldOnlyTheirType(t, store, "c360.semstreams-hierarchy-test.sensor.document.temperature.group",
		"c360.semstreams-hierarchy-test.sensor.document.group.container",
		"c360.semstreams-hierarchy-test.sensor.group.container.level")
}

// TestHierarchyInference_ContainerExistsInStorage: the type container is already
// stored, born by an earlier birth or by another writer a moment before this
// one's create. The inference asks storage through the create; graph-ingest
// refuses it with natsclient.ErrKVKeyExists, and that refusal is the container
// existing, so the birth goes on, carries its edge to the container, and writes
// nothing to it (#134 items 4 and 7).
func TestHierarchyInference_ContainerExistsInStorage(t *testing.T) {
	const entityID = "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001"
	const typeContainerID = "c360.semstreams-hierarchy-test.sensor.document.temperature.group"
	store := newFakeStore()
	store.addExistingEntity(typeContainerID)
	hi := newTestInference(t, store)

	triples, err := hi.AddToContainers(context.Background(), entityID, hierarchyTestTime)
	require.NoError(t, err, "a create refused because the container exists is the container existing")

	// Every container's create runs; the type container's is refused, the other
	// two commit.
	assert.Equal(t, []string{
		typeContainerID,
		"c360.semstreams-hierarchy-test.sensor.document.group.container",
		"c360.semstreams-hierarchy-test.sensor.group.container.level",
	}, store.createCalls())
	for _, created := range store.getCreatedEntities() {
		assert.NotEqual(t, typeContainerID, created.ID, "the container the store holds is the earlier one")
	}

	assert.Len(t, triples, 3)
	assert.Equal(t, 1, statementCount(triples, vocabulary.HierarchyTypeMember, typeContainerID),
		"the birth carries its container edge")
	container, exists := store.entity(typeContainerID)
	require.True(t, exists)
	assert.Empty(t, container.Triples, "the birth wrote nothing to the existing container")
}

func TestHierarchyInference_ContainerEntityProperties(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	err := addHierarchy(context.Background(), hi, store, "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// Verify each container entity has correct properties
	createdEntities := store.getCreatedEntities()
	require.Len(t, createdEntities, 3)

	for _, container := range createdEntities {
		require.Len(t, container.Triples, 1, container.ID)

		triple := container.Triples[0]
		assert.Equal(t, container.ID, triple.Subject)
		assert.Equal(t, "entity.type.class", triple.Predicate)
		assert.Equal(t, "hierarchy.container", triple.Object)
	}
}

// TestBuildContainerIDs pins each container to its NAMED prefix level under the
// canonical order org.platform.system.domain.type.instance (ADR-102). The
// fixture deliberately gives positions 3 and 4 distinguishable values ("src"
// and "dom") so a builder that read them in the retired order would produce a
// different string — the previous fixture named them "domain"/"system" in
// position order, which made every assertion here order-blind.
func TestBuildContainerIDs(t *testing.T) {
	eid := semtypes.EntityID{
		Org: "org", Platform: "platform",
		System: "src", Domain: "dom", Type: "type", Instance: "instance",
	}
	require.Equal(t, "org.platform.src.dom.type.instance", eid.Key())

	// Level 5 (type prefix) + one padding token.
	assert.Equal(t, "org.platform.src.dom.type.group", buildTypeContainerID(eid))

	// Level 4 (taxonomy prefix) + two padding tokens. Reached through the
	// retired-name CreateSystemEdges field and hierarchy.system.member (H7).
	assert.Equal(t, "org.platform.src.dom.group.container", buildTaxonomyContainerID(eid))

	// Level 3 (source prefix) + three padding tokens. Reached through the
	// retired-name CreateDomainEdges field and hierarchy.domain.member (H7).
	assert.Equal(t, "org.platform.src.group.container.level", buildSourceContainerID(eid))
}

// TestHierarchyInference_DeletedContainerIsBornAgain holds #130 and design D21:
// the inference keeps no record of which containers exist, so each birth asks
// storage, and a container deleted since an earlier birth of its type is created
// again by the next birth of that type. At the pin a cache answered for the
// deleted container, its inverse edge was refused for an absent subject, and
// every later birth of the type failed until the process restarted.
func TestHierarchyInference_DeletedContainerIsBornAgain(t *testing.T) {
	const first = "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001"
	const second = "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-002"
	const containerID = "c360.semstreams-hierarchy-test.sensor.document.temperature.group"
	ctx := context.Background()
	store := newFakeStore()
	hi := newTestInference(t, store)

	require.NoError(t, addHierarchy(ctx, hi, store, first))
	_, exists := store.entity(containerID)
	require.True(t, exists, "the first birth creates the type container")
	store.deleteEntity(containerID)

	triples, err := hi.AddToContainers(ctx, second, hierarchyTestTime)
	require.NoError(t, err, "the next birth of the type succeeds")

	assert.Equal(t, 1, statementCount(triples, vocabulary.HierarchyTypeMember, containerID),
		"the birth carries its container edge")
	// The container exists again, with only the statement it is born with: the
	// deleted container's statements went with it.
	assertContainersHoldOnlyTheirType(t, store, containerID)
}

// TestHierarchyInference_BirthWritesNoSiblingEdge holds ruling G (#91 comment
// 6062681355) and design D21: a birth of a type that already has a member
// returns no hierarchy.type.sibling statement and writes none on the member
// born before it. The predicate stays registered in vocabulary; the inference
// does not write it.
func TestHierarchyInference_BirthWritesNoSiblingEdge(t *testing.T) {
	const first = "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001"
	const second = "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-002"
	const containerID = "c360.semstreams-hierarchy-test.sensor.document.temperature.group"
	ctx := context.Background()
	store := newFakeStore()
	hi := newTestInference(t, store)
	require.NoError(t, addHierarchy(ctx, hi, store, first))

	triples, err := hi.AddToContainers(ctx, second, hierarchyTestTime)
	require.NoError(t, err)

	assert.Equal(t, 1, statementCount(triples, vocabulary.HierarchyTypeMember, containerID),
		"the second birth ran the inference")
	for _, triple := range append(triples, store.getTriples()...) {
		assert.NotEqual(t, vocabulary.HierarchyTypeSibling, triple.Predicate,
			"%s %s %v: no birth writes a sibling statement", triple.Subject, triple.Predicate, triple.Object)
	}
}
