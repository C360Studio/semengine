package inference

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
)

// errHierarchyAuthorityUnset is returned when a HierarchyInference is built
// with no deployment authority to compare an entity against.
var errHierarchyAuthorityUnset = errors.New("hierarchy inference has no deployment authority")

// errHierarchyTimeUnset is returned when a HierarchyInference is given no
// triggering time to stamp its statements with.
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
//   - Same type: 2 hops (entity → type.group ← entity)
//   - Same taxonomy, different type: 4 hops
//   - Same source, different taxonomy: 6 hops
//
// A caller with hierarchy off builds none: there is no disabled inference.
// This is a stateless utility - no lifecycle methods (Start/Stop).
type HierarchyInference struct {
	// store checks and creates containers and writes their inverse edges.
	store EntityStore

	// platform is the deployment's own authority, positions 1-2 of every
	// identity this deployment may mint (ADR-102 d5), from graph-ingest's
	// deps.Platform.
	platform types.PlatformMeta

	// Logger for observability
	logger *slog.Logger
}

// EntityStore is the entity storage the inference creates containers in and
// writes their inverse edges to. graph-ingest implements it, and it refuses
// what graph-ingest refuses: CreateEntity of a key the store holds returns
// natsclient.ErrKVKeyExists, and AddTriple to an absent entity fails.
type EntityStore interface {
	TripleAdder

	// ExistsEntity reports whether the store holds an entity under id.
	ExistsEntity(ctx context.Context, id string) (bool, error)

	// CreateEntity writes entity under its ID, refusing an ID the store holds
	// with natsclient.ErrKVKeyExists.
	CreateEntity(ctx context.Context, entity *gtypes.EntityState) error
}

// NewHierarchyInference builds the inference over store for the deployment
// whose authority is platform, graph-ingest's deps.Platform. An entity whose
// positions 1-2 differ is an imported mirror, and AddToContainers mints
// NOTHING for it (no container, no membership triple, no inverse edge),
// because the framework never mints under a foreign authority (ADR-102). An
// empty pair could only read every entity as foreign and mint nothing,
// forever, so it is refused here rather than on every call. A nil logger is
// slog.Default.
func NewHierarchyInference(store EntityStore, platform types.PlatformMeta, logger *slog.Logger) (*HierarchyInference, error) {
	if platform.Org == "" || platform.Platform == "" {
		return nil, errs.WrapInvalid(errHierarchyAuthorityUnset, "HierarchyInference", "NewHierarchyInference",
			"hierarchy inference requires the deployment authority (platform.org and platform.id)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HierarchyInference{store: store, platform: platform, logger: logger}, nil
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

// AddToContainers adds an entity being born to its three containers (type,
// taxonomy, source). For each one it:
//  1. creates the container if storage does not hold it (a write);
//  2. writes the container's inverse edge, container → contains → entity (a
//     write);
//  3. returns the entity's membership statement, entity → member → container,
//     which it does not write: the caller writes the returned statements with
//     the entity.
//
// Every statement it returns or writes, a new container's type statement
// included, names graph.SourceHierarchy as its source and carries at, the time
// of the write that triggered the inference, never the clock (design D15). A
// zero at is refused.
//
// It writes no sibling edge between entities of one type (ruling G, #91
// comment 6062681355); vocabulary.HierarchyTypeSibling stays registered.
//
// If any part fails (a container's existence check or birth, or an inverse
// edge), it returns every failure joined and no statements, so the caller
// refuses the birth (design D21). Containers and inverse edges written before
// the failure are what the next attempt writes again.
//
// It returns no statements, and writes nothing, for an invalid entity ID, a
// container, or an entity carrying a FOREIGN authority (an imported mirror,
// ADR-102).
func (h *HierarchyInference) AddToContainers(ctx context.Context, entityID string, at time.Time) ([]message.Triple, error) {
	// A statement with no time is refused by graph-ingest's write rules; refusing
	// here names the cause instead of minting statements stamped with the zero time.
	if at.IsZero() {
		return nil, errs.WrapInvalid(errHierarchyTimeUnset, "HierarchyInference", "AddToContainers",
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

	// The taxonomy and source containers' membership predicates keep the names
	// of the retired order, hierarchy.system.member and hierarchy.domain.member
	// (ADR-102 §B.4, H7).
	levels := [...]struct {
		containerID string
		predicate   string
	}{
		{buildTypeContainerID(parsed), vocabulary.HierarchyTypeMember},
		{buildTaxonomyContainerID(parsed), vocabulary.HierarchySystemMember},
		{buildSourceContainerID(parsed), vocabulary.HierarchyDomainMember},
	}

	triples := make([]message.Triple, 0, len(levels))
	var failures []error
	for _, level := range levels {
		triple, err := h.ensureContainerAndReturnEdge(ctx, entityID, level.containerID, level.predicate, at)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		triples = append(triples, triple)
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
// taxonomy prefix plus two padding tokens. Its membership predicate is
// hierarchy.system.member, the retired order's name for it (H7).
// Output: org.platform.system.domain.group.container
func buildTaxonomyContainerID(eid semtypes.EntityID) string {
	return eid.TaxonomyPrefix() + ".group.container"
}

// buildSourceContainerID creates a 6-part container ID from the level-3 source
// prefix plus three padding tokens. Its membership predicate is
// hierarchy.domain.member, the retired order's name for it (H7).
// Output: org.platform.system.group.container.level
func buildSourceContainerID(eid semtypes.EntityID) string {
	return eid.SourcePrefix() + ".group.container.level"
}

// ensureContainerAndReturnEdge creates the container entity if needed, then returns
// the membership edge triple WITHOUT adding it to the entity (caller must do that).
// Also adds inverse edge to container (container → contains → entity) for bidirectional
// traversal; a failure of either write is returned.
func (h *HierarchyInference) ensureContainerAndReturnEdge(ctx context.Context, entityID, containerID, predicate string, at time.Time) (message.Triple, error) {
	// Ensure the container exists, asking storage.
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
		if err := h.store.AddTriple(ctx, inverseTriple); err != nil {
			return message.Triple{}, fmt.Errorf("add hierarchy inverse edge %s %s %s: %w",
				containerID, inversePredicate, entityID, err)
		}
	}

	return forwardTriple, nil
}

// ensureContainerExists creates a minimal container entity if storage does not
// hold it, its type statement carrying the triggering time at. It keeps no record
// of which containers exist: each call asks storage, so a container deleted since
// an earlier birth is created again, and a birth delivered again commits what the
// first attempt did (#130, design D21).
func (h *HierarchyInference) ensureContainerExists(ctx context.Context, containerID string, at time.Time) error {
	exists, err := h.store.ExistsEntity(ctx, containerID)
	if err != nil {
		return err
	}
	if exists {
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

	if err := h.store.CreateEntity(ctx, containerEntity); err != nil {
		// Another writer may win the same atomic container birth between the
		// check above and this create. That definite conflict means the desired
		// container exists; no string matching needed.
		if errors.Is(err, natsclient.ErrKVKeyExists) {
			return nil
		}
		return err
	}

	h.logger.Debug("Created hierarchy container entity",
		"container_id", containerID)

	return nil
}
