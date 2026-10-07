package inference

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hierarchyTestOrg / hierarchyTestPlatform are the deployment authority every
// enabled HierarchyInference is constructed with (ADR-102): inference mints containers
// and sibling edges from the ingested entity's own prefix, so it must know
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

// addHierarchy does what the pin's OnEntityCreated did: it asks for the
// entity's hierarchy statements and writes them through the adder, so a test
// can count every edge, forward and inverse, in one place.
func addHierarchy(ctx context.Context, hi *HierarchyInference, adder TripleAdder, entityID string) error {
	triples, err := hi.GetHierarchyTriples(ctx, entityID, hierarchyTestTime)
	if err != nil {
		return err
	}
	for _, triple := range triples {
		if err := adder.AddTriple(ctx, triple); err != nil {
			return err
		}
	}
	return nil
}

// cachedContainers counts the containers the inference has cached, which the
// pin's GetCacheStats returned.
func cachedContainers(hi *HierarchyInference) int {
	hi.containerCacheMu.RLock()
	defer hi.containerCacheMu.RUnlock()
	return len(hi.containerCache)
}

// hierarchyMockTripleAdder records added triples for verification
type hierarchyMockTripleAdder struct {
	mu      sync.Mutex
	triples []message.Triple
	err     error
}

func (m *hierarchyMockTripleAdder) AddTriple(_ context.Context, triple message.Triple) error {
	if m.err != nil {
		return m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.triples = append(m.triples, triple)
	return nil
}

func (m *hierarchyMockTripleAdder) getTriples() []message.Triple {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]message.Triple, len(m.triples))
	copy(result, m.triples)
	return result
}

// mockEntityManager implements EntityManager for testing. Each error field,
// when set, is what its method returns.
type mockEntityManager struct {
	mu        sync.Mutex
	entities  map[string]bool // entityID -> exists
	created   []*gtypes.EntityState
	existsErr error
	createErr error
	listErr   error
}

func newMockEntityManager() *mockEntityManager {
	return &mockEntityManager{
		entities: make(map[string]bool),
		created:  make([]*gtypes.EntityState, 0),
	}
}

func (m *mockEntityManager) ExistsEntity(_ context.Context, id string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entities[id], nil
}

func (m *mockEntityManager) CreateEntity(_ context.Context, entity *gtypes.EntityState) (*gtypes.EntityState, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.entities[entity.ID] {
		return nil, errors.New("entity already exists")
	}

	m.entities[entity.ID] = true
	m.created = append(m.created, entity)
	return entity, nil
}

func (m *mockEntityManager) ListWithPrefix(_ context.Context, prefix string) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var matched []string
	prefixDot := prefix + "."
	for id := range m.entities {
		if strings.HasPrefix(id, prefixDot) {
			matched = append(matched, id)
		}
	}
	return matched, nil
}

func (m *mockEntityManager) getCreatedEntities() []*gtypes.EntityState {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*gtypes.EntityState, len(m.created))
	copy(result, m.created)
	return result
}

func (m *mockEntityManager) addExistingEntity(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entities[id] = true
}

// TestGetHierarchyTriplesRefusesWithoutDeploymentAuthority pins the third of
// the three LOUD paths the migration note promises (review HIGH-3). Deleting
// all three refusals in one compiling mutant previously left six suites green.
//
// An ENABLED inference holding no authority pair could only answer "everything
// is foreign" and mint nothing, for every entity, forever — the silent shape
// this whole change exists to remove. It is a construction mistake, so it fails
// the write loudly instead of quietly disabling the feature. graph-ingest
// cannot reach it (its factory refuses an absent deps.Platform first), which is
// exactly why the branch needs its own test rather than an integration one.
func TestGetHierarchyTriplesRefusesWithoutDeploymentAuthority(t *testing.T) {
	const entityID = hierarchyTestOrg + "." + hierarchyTestPlatform + ".sensor.document.temperature.sensor-001"

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
			entityManager := newMockEntityManager()
			tripleAdder := &hierarchyMockTripleAdder{}
			hi := NewHierarchyInference(entityManager, tripleAdder, HierarchyConfig{
				Enabled:         true,
				CreateTypeEdges: true,
			}, types.PlatformMeta{Org: tc.org, Platform: tc.platform}, nil)

			triples, err := hi.GetHierarchyTriples(context.Background(), entityID, hierarchyTestTime)

			require.Error(t, err,
				"an enabled inference with no deployment authority must fail loudly, "+
					"not silently mint nothing for every entity")
			assert.ErrorIs(t, err, errHierarchyAuthorityUnset)
			assert.Empty(t, triples)
			assert.Empty(t, entityManager.getCreatedEntities(),
				"the refusal happens before any container is born")
		})
	}
}

// TestGetHierarchyTriplesDisabledIsSilentWithoutAuthority is the boundary: the
// refusal is scoped to an ENABLED inference. A disabled one is a deliberate
// no-op and must stay quiet whether or not it carries an authority.
func TestGetHierarchyTriplesDisabledIsSilentWithoutAuthority(t *testing.T) {
	hi := NewHierarchyInference(newMockEntityManager(), &hierarchyMockTripleAdder{},
		HierarchyConfig{Enabled: false, CreateTypeEdges: true}, types.PlatformMeta{}, nil)

	triples, err := hi.GetHierarchyTriples(context.Background(),
		hierarchyTestOrg+"."+hierarchyTestPlatform+".sensor.document.temperature.sensor-001", hierarchyTestTime)

	require.NoError(t, err, "a disabled inference is a no-op, not a misconfiguration")
	assert.Empty(t, triples)
}

// TestGetHierarchyTriplesSkipsForeignAuthority is the DISCRIMINATING test for
// the ADR-102 skip, and it lives here rather than at the graph-ingest seam for a
// measured reason: at that seam the skip is shadowed. graph-ingest's own
// authority gate refuses every container birth under a peer's pair, which makes
// GetHierarchyTriples return a joined error, and the merge path then discards
// the WHOLE triple set on any error (component.go, "Failed to get hierarchy
// triples") — so an imported entity ends up with no hierarchy triples whether
// this check exists or not. Deleting the check is invisible there and visible
// here.
//
// It also covers the case graph-ingest cannot: this is exported framework
// surface, and a consumer calling GetHierarchyTriples directly has no second
// layer behind it.
func TestGetHierarchyTriplesSkipsForeignAuthority(t *testing.T) {
	entityManager := newMockEntityManager()
	tripleAdder := &hierarchyMockTripleAdder{}
	hi := NewHierarchyInference(entityManager, tripleAdder, HierarchyConfig{
		Enabled:            true,
		CreateTypeEdges:    true,
		CreateSystemEdges:  true,
		CreateDomainEdges:  true,
		CreateTypeSiblings: true,
	}, hierarchyTestAuthority, nil)

	// A peer deployment's entity: same org, different platform, canonical shape.
	const imported = hierarchyTestOrg + ".dep9.sensor.document.temperature.sensor-001"

	triples, err := hi.GetHierarchyTriples(context.Background(), imported, hierarchyTestTime)

	require.NoError(t, err, "a foreign entity is skipped, not rejected")
	assert.Empty(t, triples, "no membership or sibling triple may be minted for an imported entity")
	assert.Empty(t, entityManager.getCreatedEntities(),
		"no container entity may be born under a peer's authority")
	assert.Empty(t, tripleAdder.getTriples(),
		"no inverse edge may be written for an imported entity")

	// The same shape under THIS deployment's authority still mints, so the skip
	// is authority-scoped rather than a blanket disable.
	local := hierarchyTestOrg + "." + hierarchyTestPlatform + ".sensor.document.temperature.sensor-001"
	localTriples, err := hi.GetHierarchyTriples(context.Background(), local, hierarchyTestTime)
	require.NoError(t, err)
	assert.NotEmpty(t, localTriples, "a local entity still receives hierarchy triples")
}

func TestHierarchyInference_Disabled(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:         false, // Disabled
		CreateTypeEdges: true,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, config, hierarchyTestAuthority, nil)

	err := addHierarchy(context.Background(), hi, tripleAdder, "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// No triples should be added when disabled
	assert.Empty(t, tripleAdder.getTriples())
	assert.Empty(t, entityManager.getCreatedEntities())
}

func TestHierarchyInference_InvalidEntityID(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:         true,
		CreateTypeEdges: true,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, config, hierarchyTestAuthority, nil)

	// 5-part entity ID should be skipped
	err := addHierarchy(context.Background(), hi, tripleAdder, "c360.semstreams-hierarchy-test.sensor.document.temperature")
	require.NoError(t, err)
	assert.Empty(t, tripleAdder.getTriples())

	// 7-part entity ID should be skipped
	err = addHierarchy(context.Background(), hi, tripleAdder, "c360.semstreams-hierarchy-test.sensor.document.temperature.zone.sensor")
	require.NoError(t, err)
	assert.Empty(t, tripleAdder.getTriples())
}

func TestHierarchyInference_TypeEdgeOnly(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, config, hierarchyTestAuthority, nil)

	entityID := "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001"
	containerID := "c360.semstreams-hierarchy-test.sensor.document.temperature.group"
	err := addHierarchy(context.Background(), hi, tripleAdder, entityID)
	require.NoError(t, err)

	// Should create 1 container
	createdEntities := entityManager.getCreatedEntities()
	assert.Len(t, createdEntities, 1)
	assert.Equal(t, containerID, createdEntities[0].ID)

	// Should create 2 edges: forward (member) + inverse (contains)
	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 2)

	// Find forward and inverse triples
	var forwardTriple, inverseTriple *message.Triple
	for i := range triples {
		if triples[i].Subject == entityID {
			forwardTriple = &triples[i]
		} else if triples[i].Subject == containerID {
			inverseTriple = &triples[i]
		}
	}

	// Verify forward edge: entity → member → container
	require.NotNil(t, forwardTriple, "forward triple not found")
	assert.Equal(t, vocabulary.HierarchyTypeMember, forwardTriple.Predicate)
	assert.Equal(t, containerID, forwardTriple.Object)
	assert.Equal(t, "inference.hierarchy", forwardTriple.Context)
	assert.Equal(t, 1.0, forwardTriple.Confidence)

	// Verify inverse edge: container → contains → entity
	require.NotNil(t, inverseTriple, "inverse triple not found")
	assert.Equal(t, vocabulary.HierarchyTypeContains, inverseTriple.Predicate)
	assert.Equal(t, entityID, inverseTriple.Object)
	assert.Equal(t, "inference.hierarchy", inverseTriple.Context)
}

func TestHierarchyInference_AllLevels(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: true,
		CreateDomainEdges: true,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, config, hierarchyTestAuthority, nil)

	entityID := "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001"
	err := addHierarchy(context.Background(), hi, tripleAdder, entityID)
	require.NoError(t, err)

	// Should create 3 containers
	createdEntities := entityManager.getCreatedEntities()
	assert.Len(t, createdEntities, 3)

	containerIDs := make(map[string]bool)
	for _, e := range createdEntities {
		containerIDs[e.ID] = true
	}
	assert.True(t, containerIDs["c360.semstreams-hierarchy-test.sensor.document.temperature.group"]) // Type
	assert.True(t, containerIDs["c360.semstreams-hierarchy-test.sensor.document.group.container"])   // System
	assert.True(t, containerIDs["c360.semstreams-hierarchy-test.sensor.group.container.level"])      // Domain

	// Should create 6 edges: 3 forward (member) + 3 inverse (contains)
	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 6)

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

	// Extract inverse edges (container → contains → entity)
	inversePredicates := make(map[string]string) // predicate-audit:unrelated {"column":23,"surface":"go-assignment:inversePredicates","value":"","basis":"reviewed output map populated from inferred triples"}
	for _, tr := range triples {
		if tr.Object == entityID {
			inversePredicates[tr.Predicate] = tr.Subject
		}
	}
	assert.Len(t, inversePredicates, 3)
	assert.Equal(t, "c360.semstreams-hierarchy-test.sensor.document.temperature.group", inversePredicates[vocabulary.HierarchyTypeContains])
	assert.Equal(t, "c360.semstreams-hierarchy-test.sensor.document.group.container", inversePredicates[vocabulary.HierarchySystemContains])
	assert.Equal(t, "c360.semstreams-hierarchy-test.sensor.group.container.level", inversePredicates[vocabulary.HierarchyDomainContains])
}

func TestHierarchyInference_ContainerReuse(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, config, hierarchyTestAuthority, nil)

	// Create first entity
	err := addHierarchy(context.Background(), hi, tripleAdder, "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// Create second entity with same type prefix
	err = addHierarchy(context.Background(), hi, tripleAdder, "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-002")
	require.NoError(t, err)

	// Should only create 1 container (reused)
	createdEntities := entityManager.getCreatedEntities()
	assert.Len(t, createdEntities, 1)

	// Should have 4 edges: 2 forward (member) + 2 inverse (contains)
	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 4)
}

func TestHierarchyInference_ContainerExistsInStorage(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	// Pre-existing container in storage
	entityManager.addExistingEntity("c360.semstreams-hierarchy-test.sensor.document.temperature.group")

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, config, hierarchyTestAuthority, nil)

	err := addHierarchy(context.Background(), hi, tripleAdder, "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// Should NOT create container (already exists)
	createdEntities := entityManager.getCreatedEntities()
	assert.Empty(t, createdEntities)

	// Should create 2 edges: forward (member) + inverse (contains)
	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 2)
}

func TestHierarchyInference_ContainerEntityProperties(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, config, hierarchyTestAuthority, nil)

	err := addHierarchy(context.Background(), hi, tripleAdder, "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// Verify container entity has correct properties
	createdEntities := entityManager.getCreatedEntities()
	require.Len(t, createdEntities, 1)

	container := createdEntities[0]
	assert.Equal(t, "c360.semstreams-hierarchy-test.sensor.document.temperature.group", container.ID)
	require.Len(t, container.Triples, 1)

	triple := container.Triples[0]
	assert.Equal(t, container.ID, triple.Subject)
	assert.Equal(t, "entity.type.class", triple.Predicate)
	assert.Equal(t, "hierarchy.container", triple.Object)
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

func TestHierarchyInference_RaceConditionOnContainerCreate(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	// Simulate race: container "exists" error during create
	entityManager.addExistingEntity("c360.semstreams-hierarchy-test.sensor.document.temperature.group")

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, config, hierarchyTestAuthority, nil)

	// Even if container exists, edges should still be created
	err := addHierarchy(context.Background(), hi, tripleAdder, "c360.semstreams-hierarchy-test.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// Should create 2 edges: forward (member) + inverse (contains)
	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 2)
}
