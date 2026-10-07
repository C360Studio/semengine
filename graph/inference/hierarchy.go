package inference

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
)

// errHierarchyAuthorityUnset is returned when an ENABLED HierarchyInference
// holds no deployment authority to compare an entity against.
var errHierarchyAuthorityUnset = errors.New("hierarchy inference has no deployment authority")

// errHierarchyTimeUnset is returned when an ENABLED HierarchyInference is
// given no triggering time to stamp its statements with.
var errHierarchyTimeUnset = errors.New("hierarchy inference has no triggering time")

// HierarchyInference creates membership edges to container entities based on
// the 6-part entity ID structure. It operates synchronously - hierarchy triples
// are computed and returned before the entity is written to storage.
//
// Entity ID format: org.platform.system.domain.type.instance (ADR-102).
// Example: acme.iot.hvac.sensors.temperature.001
//
// Containers are built from the NAMED prefix levels, not fixed indexes:
//   - Type container:     <TypePrefix>.group                 (level 5 + padding)
//   - Taxonomy container: <TaxonomyPrefix>.group.container   (level 4 + padding)
//   - Source container:   <SourcePrefix>.group.container.level (level 3 + padding)
//
// Graph distances via containers:
//   - Same type siblings: 2 hops (entity → type.group ← entity)
//   - Same taxonomy, different type: 4 hops
//   - Same source, different taxonomy: 6 hops
//
// NAMING DEBT (ADR-102 §B.4 / H7, owner item O-6): the config fields
// CreateSystemEdges / CreateDomainEdges and the vocabulary predicates
// hierarchy.system.member / hierarchy.domain.member were named for the
// retired order and now misname their container — the "system" edge is the
// taxonomy container and the "domain" edge is the source container. The
// ruling is to retire containers with gh606 rather than rename an
// operator-facing field and a published predicate for one release; the
// mechanics below are unchanged and correct.
//
// This is a stateless utility - no lifecycle methods (Start/Stop).
type HierarchyInference struct {
	// entityManager handles entity existence checks and creation
	entityManager EntityManager

	// tripleAdder adds triples to entities (used for inverse edges on containers)
	tripleAdder TripleAdder

	// Configuration
	config HierarchyConfig

	// platform is the deployment's own authority, positions 1-2 of every
	// identity this deployment may mint (ADR-102 d5), from graph-ingest's
	// deps.Platform.
	platform types.PlatformMeta

	// Logger for observability
	logger *slog.Logger

	// Cache of known container entities to avoid repeated existence checks
	containerCache   map[string]bool
	containerCacheMu sync.RWMutex
}

// EntityManager provides entity existence checks and creation. Production
// implementations route creation through graph-ingest.
type EntityManager interface {
	ExistsEntity(ctx context.Context, id string) (bool, error)
	CreateEntity(ctx context.Context, entity *gtypes.EntityState) (*gtypes.EntityState, error)
	// ListWithPrefix returns entity IDs matching a prefix (for sibling discovery)
	ListWithPrefix(ctx context.Context, prefix string) ([]string, error)
}

// HierarchyConfig configures hierarchy container inference.
type HierarchyConfig struct {
	// Enabled activates hierarchy inference on entity creation
	Enabled bool `json:"enabled"`

	// CreateTypeEdges enables type membership edges (5-part prefix → type container)
	// StandardIRI: skos:broader
	CreateTypeEdges bool `json:"create_type_edges"`

	// CreateSystemEdges enables system membership edges (4-part prefix → system container)
	// StandardIRI: skos:broader
	CreateSystemEdges bool `json:"create_system_edges"`

	// CreateDomainEdges enables domain membership edges (3-part prefix → domain container)
	// StandardIRI: skos:broader
	CreateDomainEdges bool `json:"create_domain_edges"`

	// CreateTypeSiblings enables sibling edges between entities with the same type (5-part prefix)
	// When enabled, creates bidirectional hierarchy.type.sibling edges
	// Cost: O(N) per new entity where N is existing sibling count
	CreateTypeSiblings bool `json:"create_type_siblings"`
}

// NewHierarchyInference creates a new hierarchy inference component.
//
// Parameters:
//   - entityManager: Component for entity existence checks and creation
//   - tripleAdder: Component that can add triples (for inverse edges on containers)
//   - config: Configuration for edge creation
//   - platform: the deployment's own authority, graph-ingest's deps.Platform.
//     An entity whose positions 1-2 differ is an imported mirror, and
//     GetHierarchyTriples mints NOTHING for it (no container, no membership
//     triple, no inverse sibling edge), because the framework never mints
//     under a foreign authority. An empty pair while config.Enabled is set is
//     refused loudly by GetHierarchyTriples rather than silently skipping
//     every entity.
//   - logger: Logger for observability (can be nil)
func NewHierarchyInference(
	entityManager EntityManager,
	tripleAdder TripleAdder,
	config HierarchyConfig,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *HierarchyInference {
	if logger == nil {
		logger = slog.Default()
	}

	return &HierarchyInference{
		entityManager:  entityManager,
		tripleAdder:    tripleAdder,
		config:         config,
		platform:       platform,
		logger:         logger,
		containerCache: make(map[string]bool),
	}
}

// isContainerEntity returns true if the entityID represents a container entity.
// Container entities carry a reserved padding token in the INSTANCE position
// (group, container, level) and have exactly 6 parts. The token set is owned by
// pkg/types — this asks it via IsReservedInstanceToken rather than re-spelling
// the tokens, so the audit rule and this check can never disagree.
func isContainerEntity(entityID string) bool {
	parsed, err := semtypes.ParseEntityID(entityID)
	if err != nil {
		return false
	}
	return semtypes.IsReservedInstanceToken(parsed.Instance)
}

// GetHierarchyTriples returns hierarchy membership triples for the given entity ID.
//
// IT HAS SIDE EFFECTS ON CONTAINER ENTITIES. It does not write the SUBJECT entity —
// the caller must include the returned triples before writing that — but it DOES
// create missing container entities and append inverse edges to them, as steps 2 and 4
// below say plainly.
//
// The previous comment here opened with "This method has NO side effects", contradicted
// four lines later by its own step 2. That was gh#713's cover story: a reader who
// stopped at the first line had no reason to look for the re-fire the container writes
// cause. A name that says "Get" and a comment that says "no side effects" are not a
// contract; the steps are.
//
// For each enabled level (type, system, domain), it:
// 1. Computes the container entity ID
// 2. Auto-creates the container if it doesn't exist (WRITE — side effect on containers)
// 3. Returns a membership triple from entity to container
// 4. Adds inverse edge to container (container → contains → entity) (WRITE)
//
// Every statement it returns or writes, a new container's type statement
// included, names graph.SourceHierarchy as its source and carries at, the time
// of the write that triggered the inference, never the clock (design D15). A
// zero at is refused.
//
// If any part fails (a container's existence check or birth, an inverse edge,
// the sibling listing or a sibling's inverse edge), it returns every failure
// joined and no statements, so the caller refuses the birth (design D21).
// Containers and inverse edges written before the failure are what the next
// attempt writes again.
//
// Returns empty slice if hierarchy is disabled, the entity ID is invalid, or
// the entity carries a FOREIGN authority (an imported mirror — ADR-102).
func (h *HierarchyInference) GetHierarchyTriples(ctx context.Context, entityID string, at time.Time) ([]message.Triple, error) {
	if !h.config.Enabled {
		return nil, nil
	}

	// An enabled inference with no deployment authority could only answer
	// "everything is foreign" and mint nothing, for every entity, forever. That
	// is the silent shape this check exists to prevent: it is a construction
	// mistake, so it fails the write loudly instead of quietly disabling the
	// feature. graph-ingest cannot reach it — its factory refuses an absent
	// deps.Platform first.
	if h.platform.Org == "" || h.platform.Platform == "" {
		return nil, errs.WrapInvalid(errHierarchyAuthorityUnset, "HierarchyInference", "GetHierarchyTriples",
			"hierarchy inference requires the deployment authority (the platform passed to NewHierarchyInference)")
	}

	// A statement with no time is refused by graph-ingest's write rules; refusing
	// here names the cause instead of minting statements stamped with the zero time.
	if at.IsZero() {
		return nil, errs.WrapInvalid(errHierarchyTimeUnset, "HierarchyInference", "GetHierarchyTriples",
			"hierarchy inference requires the time of the write that triggered it")
	}

	// Skip container entities to prevent infinite cascade
	if isContainerEntity(entityID) {
		return nil, nil
	}

	// Foreign authority: the framework never mints under a peer's org.platform,
	// so an imported entity is persisted with no hierarchy at all — on the
	// import lane as on any other (ADR-102, accepted by ruling on every lane).
	// No warning: for a federated deployment this is the ordinary case, and a
	// per-arrival WARN would be noise, not signal.
	if semtypes.ValidateEntityIDAuthority(entityID, h.platform.Org, h.platform.Platform, false) != nil {
		return nil, nil
	}

	// Parse the entity ID once; every container prefix below is read from a
	// NAMED position on the parsed value, never from a fixed index.
	parsed, err := semtypes.ParseEntityID(entityID)
	if err != nil {
		// Not a valid 6-part EntityID, skip silently
		return nil, nil
	}

	var triples []message.Triple
	var failures []error

	// Create type membership: entity → type.group
	if h.config.CreateTypeEdges {
		typeContainerID := buildTypeContainerID(parsed)
		triple, err := h.ensureContainerAndReturnEdge(ctx, entityID, typeContainerID, vocabulary.HierarchyTypeMember, at)
		if err != nil {
			failures = append(failures, err)
		} else {
			triples = append(triples, triple)
		}
	}

	// Create sibling edges to entities with same type (5-part prefix)
	if h.config.CreateTypeSiblings {
		siblingTriples, err := h.createSiblingEdges(ctx, entityID, parsed, at)
		if err != nil {
			failures = append(failures, err)
		} else {
			triples = append(triples, siblingTriples...)
		}
	}

	// Create taxonomy membership: entity → <TaxonomyPrefix>.group.container.
	// Config field and predicate keep the retired "system" name (H7 above).
	if h.config.CreateSystemEdges {
		systemContainerID := buildTaxonomyContainerID(parsed)
		triple, err := h.ensureContainerAndReturnEdge(ctx, entityID, systemContainerID, vocabulary.HierarchySystemMember, at)
		if err != nil {
			failures = append(failures, err)
		} else {
			triples = append(triples, triple)
		}
	}

	// Create source membership: entity → <SourcePrefix>.group.container.level.
	// Config field and predicate keep the retired "domain" name (H7 above).
	if h.config.CreateDomainEdges {
		domainContainerID := buildSourceContainerID(parsed)
		triple, err := h.ensureContainerAndReturnEdge(ctx, entityID, domainContainerID, vocabulary.HierarchyDomainMember, at)
		if err != nil {
			failures = append(failures, err)
		} else {
			triples = append(triples, triple)
		}
	}

	if len(failures) > 0 {
		return nil, errors.Join(failures...)
	}

	return triples, nil
}

// hierarchyTriple builds one hierarchy statement: graph-ingest's hierarchy
// producer as its source and the triggering time at, never the clock (design
// D15). Structural inference has perfect confidence.
func hierarchyTriple(subject, predicate string, object any, at time.Time) message.Triple {
	return message.Triple{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Source:     gtypes.SourceHierarchy,
		Timestamp:  at,
		Confidence: 1.0,
		Context:    "inference.hierarchy",
	}
}

// buildTypeContainerID creates a 6-part type container ID from the level-5
// type prefix plus one padding token.
// Output: org.platform.system.domain.type.group
func buildTypeContainerID(eid semtypes.EntityID) string {
	return eid.TypePrefix() + ".group"
}

// buildTaxonomyContainerID creates a 6-part container ID from the level-4
// taxonomy prefix plus two padding tokens. Reached through the
// CreateSystemEdges config field and stamped hierarchy.system.member — both
// retired names for this container (H7).
// Output: org.platform.system.domain.group.container
func buildTaxonomyContainerID(eid semtypes.EntityID) string {
	return eid.TaxonomyPrefix() + ".group.container"
}

// buildSourceContainerID creates a 6-part container ID from the level-3 source
// prefix plus three padding tokens. Reached through the CreateDomainEdges
// config field and stamped hierarchy.domain.member — both retired names for
// this container (H7).
// Output: org.platform.system.group.container.level
func buildSourceContainerID(eid semtypes.EntityID) string {
	return eid.SourcePrefix() + ".group.container.level"
}

// createSiblingEdges creates bidirectional sibling edges between the new entity
// and all existing entities sharing its level-5 type prefix.
//
// For each existing sibling:
//   - Returns a forward edge: newEntity → hierarchy.type.sibling → existingSibling
//   - Adds inverse edge directly: existingSibling → hierarchy.type.sibling → newEntity
//
// It stops at the first inverse edge that fails and returns that failure.
//
// Cost: O(N) per new entity where N is existing sibling count.
func (h *HierarchyInference) createSiblingEdges(ctx context.Context, entityID string, eid semtypes.EntityID, at time.Time) ([]message.Triple, error) {
	// The level-5 type prefix is the sibling scope (excludes the instance).
	prefix := eid.TypePrefix()

	// Find existing members with same prefix
	existingMembers, err := h.entityManager.ListWithPrefix(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list hierarchy siblings of %s: %w", entityID, err)
	}

	var triples []message.Triple
	for _, siblingID := range existingMembers {
		// Skip self and container entities
		if siblingID == entityID || isContainerEntity(siblingID) {
			continue
		}

		// Forward edge: new entity → sibling → existing
		triples = append(triples, hierarchyTriple(entityID, vocabulary.HierarchyTypeSibling, siblingID, at))

		// Inverse edge: existing → sibling → new entity (update existing entity)
		// Since HierarchyTypeSibling is symmetric, both directions use same predicate
		inverseTriple := hierarchyTriple(siblingID, vocabulary.HierarchyTypeSibling, entityID, at)
		if err := h.tripleAdder.AddTriple(ctx, inverseTriple); err != nil {
			return nil, fmt.Errorf("add hierarchy sibling edge %s -> %s: %w", siblingID, entityID, err)
		}
	}

	if len(triples) > 0 {
		h.logger.Debug("Created sibling edges",
			"entity_id", entityID,
			"sibling_count", len(triples))
	}

	return triples, nil
}

// ensureContainerAndReturnEdge creates the container entity if needed, then returns
// the membership edge triple WITHOUT adding it to the entity (caller must do that).
// Also adds inverse edge to container (container → contains → entity) for bidirectional
// traversal; a failure of either write is returned.
func (h *HierarchyInference) ensureContainerAndReturnEdge(ctx context.Context, entityID, containerID, predicate string, at time.Time) (message.Triple, error) {
	// Ensure container exists (with caching)
	if err := h.ensureContainerExists(ctx, containerID, at); err != nil {
		return message.Triple{}, err
	}

	// Create forward membership edge: entity → predicate → container.
	// The object is a real 6-part entity ID, so IsRelationship() returns true.
	forwardTriple := hierarchyTriple(entityID, predicate, containerID, at)

	// Create inverse edge: container → contains → entity
	// This enables direct traversal from container to its members without using IncomingIndex
	// NOTE: This IS a side effect - we're updating the container entity
	inversePredicate := vocabulary.GetInversePredicate(predicate)
	if inversePredicate != "" {
		// e.g., hierarchy.type.contains
		inverseTriple := hierarchyTriple(containerID, inversePredicate, entityID, at)
		if err := h.tripleAdder.AddTriple(ctx, inverseTriple); err != nil {
			return message.Triple{}, fmt.Errorf("add hierarchy inverse edge %s %s %s: %w",
				containerID, inversePredicate, entityID, err)
		}
	}

	return forwardTriple, nil
}

// ensureContainerExists creates a minimal container entity if it doesn't exist,
// its type statement carrying the triggering time at.
// Uses an in-memory cache to avoid repeated existence checks.
func (h *HierarchyInference) ensureContainerExists(ctx context.Context, containerID string, at time.Time) error {
	// Check cache first
	h.containerCacheMu.RLock()
	exists := h.containerCache[containerID]
	h.containerCacheMu.RUnlock()

	if exists {
		return nil
	}

	// Check if container exists in storage
	existsInStorage, err := h.entityManager.ExistsEntity(ctx, containerID)
	if err != nil {
		return err
	}

	if existsInStorage {
		// Update cache and return
		h.containerCacheMu.Lock()
		h.containerCache[containerID] = true
		h.containerCacheMu.Unlock()
		return nil
	}

	// Create minimal container entity, stamped with the registered framework
	// type so it passes graph-ingest's registered-type gate (ADR-103, O-16 (a)).
	containerEntity := &gtypes.EntityState{
		ID:          containerID,
		MessageType: HierarchyContainerMessageType(),
		Triples: []message.Triple{
			hierarchyTriple(containerID, "entity.type.class", "hierarchy.container", at),
		},
	}

	_, err = h.entityManager.CreateEntity(ctx, containerEntity)
	if err != nil {
		// Another writer may win the same atomic container birth. That definite
		// conflict means the desired container exists; no string matching needed.
		if errors.Is(err, natsclient.ErrKVKeyExists) {
			h.containerCacheMu.Lock()
			h.containerCache[containerID] = true
			h.containerCacheMu.Unlock()
			return nil
		}
		return err
	}

	// Update cache
	h.containerCacheMu.Lock()
	h.containerCache[containerID] = true
	h.containerCacheMu.Unlock()

	h.logger.Debug("Created hierarchy container entity",
		"container_id", containerID)

	return nil
}
