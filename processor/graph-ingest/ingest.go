package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/vocabulary"
)

// decodeEntity decodes a message's bytes into an EntityState: unmarshal the
// BaseMessage envelope, validate it, then extract the Graphable payload's
// entity. The consume closure decodes once, then submits to the keyed pool;
// every error it returns is a poison message there (counted, logged and
// terminated), except a refused statement, which is a structural rejection.
//
// Validate is the consumer half of BaseMessage.MarshalJSON: what a producer's
// envelope refuses to emit, the lane refuses to ingest, before any identity
// method runs. One fence covers the whole entry because every step runs
// producer-registered payload code on wire content: the type's UnmarshalJSON
// inside Decode, its Validate, and its EntityID/Triples/StorageRef/
// IndexingProfile during extraction. A panic there becomes a classified error
// on the poison path instead of escaping to the stream handler's Nak and an
// endless redelivery (SemStreams #1112, PR #1437; design D8 Q13).
func (c *Component) decodeEntity(subject string, data []byte) (entity *graph.EntityState, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			entity = nil
			err = errs.WrapInvalid(
				fmt.Errorf("payload %s (subject %s) panicked on the Graphable lane: %v",
					wireMessageType(data), subject, recovered),
				"graph-ingest", "decodeEntity", "fence payload panic")
		}
	}()

	baseMsg, err := c.decoder.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode base message (subject %s): %w", subject, err)
	}
	if err := baseMsg.Validate(); err != nil {
		return nil, fmt.Errorf("validate payload %s (subject %s): %w", baseMsg.Type(), subject,
			errs.WrapInvalid(err, "graph-ingest", "decodeEntity", "payload validation failed"))
	}
	entity, err = extractEntityFromMessage(baseMsg)
	if err != nil {
		return nil, fmt.Errorf("extract entity (subject %s): %w", subject, err)
	}
	return entity, nil
}

// wireMessageType reads the message type from an envelope's wire form, for a
// payload that panicked: a panic inside Decode leaves no decoded message to ask.
func wireMessageType(data []byte) string {
	var envelope struct {
		Type message.Type `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || !envelope.Type.IsValid() {
		return "of unknown type"
	}
	return envelope.Type.String()
}

// ingestEntity merges one Graphable projection into its authority entity.
// importLane is the arrival port's declared lane (ADR-102 d5); the synchronous
// compat entry point and every direct caller pass false.
func (c *Component) ingestEntity(ctx context.Context, entity *graph.EntityState, importLane bool) error {
	if err := c.prepareFactProjection(entity, importLane); err != nil {
		return err
	}

	// Structural-identity predicate gate on the Graphable ingest path
	// (unconditionally fail-closed; the primary vector for product/source-authored
	// predicates). On
	// this lane it is a defense-in-depth backstop: prepareFactProjection above has
	// already rejected any non-canonical predicate via the authoritative
	// ValidateEntityStateContract, so this call cannot fire unless that seam moves.
	if err := validateTriplePredicates(entity.Triples); err != nil {
		return err
	}

	// Store entity in KV bucket — MERGE semantics (SemStreams gh#177). Earlier code
	// used CreateEntity (Put = full-replace) here, which clobbered any
	// pre-existing triples on the entity. The Graphable lane merges while the
	// four explicit mutation operations apply their own strict semantics; the
	// JetStream consumer path previously replaced all state and silently
	// erased lifecycle-managed entity state on every subsequent
	// Graphable arrival.
	if err := c.mergeEntityOnLane(ctx, entity, importLane); err != nil {
		c.logger.Error("Failed to merge entity",
			slog.String("entity_id", entity.ID),
			slog.Any("error", err))
		return err
	}

	c.logger.Debug("Entity ingested",
		slog.String("entity_id", entity.ID),
		slog.Int("triples", len(entity.Triples)))
	return nil
}

// prepareFactProjection applies the Graphable lane's single projection
// convenience and validates the complete candidate. applyIngest calls this
// before its redelivery guard so malformed identity cannot become a guard key
// or trigger guard I/O; ingestEntity calls it as the reusable direct-entry
// safety net. The operation is idempotent.
func (c *Component) prepareFactProjection(entity *graph.EntityState, importLane bool) error {
	if entity == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "ingestEntity", "entity cannot be nil")
	}
	if err := validateEntityID(entity.ID); err != nil {
		return errs.WrapInvalid(&graph.EntityStateContractError{
			Field:       graph.EntityStateContractFieldID,
			TripleIndex: -1,
			Err:         err,
		}, "Component", "ingestEntity", "validate envelope entity ID")
	}
	// Authority gate (ADR-102 d5), before the projection fill and before any KV
	// I/O. importLane is the arrival port's own declaration, carried from the
	// consume closure; a direct caller supplies false.
	if err := c.authorizeSubject(entity.ID, importLane); err != nil {
		return err
	}

	// Graphable projection convenience: an omitted subject means the envelope
	// entity. Fill only this fact-arrival lane, before the authoritative seam;
	// mutation, direct persistence, and replay receive no such fill.
	entity.Triples = fillFactProjectionSubjects(entity.ID, entity.Triples)
	for index, triple := range entity.Triples {
		if triple.Subject != entity.ID {
			return errs.WrapInvalid(
				fmt.Errorf("triple[%d] subject %q does not match Graphable entity %q", index, triple.Subject, entity.ID),
				"Component", "ingestEntity", "validate projected entity subject",
			)
		}
	}

	// Preflight the complete Graphable projection before persistence.
	if err := graph.ValidateEntityStateContract(entity); err != nil {
		return errs.WrapInvalid(err, "Component", "ingestEntity", "validate projected entity state")
	}
	return nil
}

// fillFactProjectionSubjects returns triples with each omitted Subject filled
// from the exact Graphable envelope ID. It copies only when a fill is needed
// and never repairs or rewrites non-empty subject bytes.
func fillFactProjectionSubjects(entityID string, triples []message.Triple) []message.Triple {
	var filled []message.Triple
	for index := range triples {
		if triples[index].Subject != "" {
			continue
		}
		if filled == nil {
			filled = append([]message.Triple(nil), triples...)
		}
		filled[index].Subject = entityID
	}
	if filled != nil {
		return filled
	}
	return triples
}

// extractEntityFromMessage extracts an EntityState from a BaseMessage
func extractEntityFromMessage(msg *message.BaseMessage) (*graph.EntityState, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}

	payload := msg.Payload()
	if payload == nil {
		return nil, fmt.Errorf("message has no payload")
	}

	// Check if payload implements Graphable
	graphable, ok := payload.(graph.Graphable)
	if !ok {
		return nil, fmt.Errorf("payload does not implement Graphable interface")
	}

	// Get entity ID and triples from Graphable
	entityID := graphable.EntityID()
	if entityID == "" {
		return nil, fmt.Errorf("graphable payload returned empty entity ID")
	}

	// Statement metadata (design D15, #98): a payload with no statement refuses the message,
	// a statement without a Source or Timestamp takes the envelope's, and a statement under a
	// reserved source refuses the message. This reads only the payload's statements, before the
	// indexing-profile statement graph-ingest stamps below under its own reserved source.
	own := graphable.Triples()
	if len(own) == 0 {
		return nil, refuseNoStatement()
	}
	triples, err := stampFromEnvelope(own, msg.Meta())
	if err != nil {
		return nil, err
	}

	// Build EntityState
	entity := &graph.EntityState{
		ID:          entityID,
		Triples:     triples,
		MessageType: msg.Type(),
	}

	// SemStreams ADR-055 §5 StorageRef extraction (consumer half): a Graphable payload that
	// also implements message.Storable carries an ObjectStore reference to its
	// offloaded content. Lift it onto the EntityState so mergeEntityOnLane persists it
	// (the create branch marshals it verbatim; the existing-entity branch merges
	// it onto the stored copy). Without this, a ContentStorable producer's offloaded
	// content (tool results, model responses, document bodies) loses its storage
	// pointer at the ingest seam and the embedding worker can never find the
	// text — the "offloaded content stops embedding" gap.
	if storable, ok := payload.(message.Storable); ok {
		if ref := storable.StorageRef(); ref != nil {
			entity.StorageRef = ref
		}
	}

	// SemStreams ADR-054 channel (a): a Graphable payload MAY also declare its indexing
	// profile via the optional IndexingProfiler interface. Stamp it explicitly
	// here so it's present on entity.Triples by the time mergeEntityOnLane reaches
	// its create seam; absence (or an invalid value) falls through to the
	// fallback floor there. It carries the time read from the stamped statements
	// before it is added (design D15), the time mergeEntityOnLane reads again.
	if profiler, ok := payload.(message.IndexingProfiler); ok {
		stampExplicitIndexingProfile(entity, profiler.IndexingProfile(), triggeringTime(entity))
	}

	return entity, nil
}

// hasIndexingProfileTriple reports whether the entity already carries a
// create-time indexing profile. That profile is immutable (SemStreams ADR-054), so a
// later Graphable arrival must not override it through the stream lane's replace.
func hasIndexingProfileTriple(entity *graph.EntityState) bool {
	for _, t := range entity.Triples {
		if t.Predicate == vocabulary.EntityIndexingProfile {
			return true
		}
	}
	return false
}

// triplesWithoutPredicate returns triples with every triple carrying predicate
// removed. Non-mutating; used to drop an incoming immutable predicate before the
// stream lane's replace.
func triplesWithoutPredicate(triples []message.Triple, predicate string) []message.Triple {
	out := make([]message.Triple, 0, len(triples))
	for _, t := range triples {
		if t.Predicate != predicate {
			out = append(out, t)
		}
	}
	return out
}

// removeIndexingProfileTriples drops every entity.indexing.profile triple from
// the entity (the single-valued predicate is replace-on-write, never appended).
func removeIndexingProfileTriples(entity *graph.EntityState) {
	filtered := entity.Triples[:0]
	for _, t := range entity.Triples {
		if t.Predicate != vocabulary.EntityIndexingProfile {
			filtered = append(filtered, t)
		}
	}
	entity.Triples = filtered
}

// triggeringTime is the time every statement graph-ingest derives for a write carries (the
// indexing profile, hierarchy statements, a hierarchy container's statements): the latest
// Timestamp among entity's statements, never the clock (design D15; owner ruling, #91 comment
// 6066791396). Each lane reads it from the write's own statements before graph-ingest adds one
// of its own, and reads all of them, reserved sources included: a hierarchy container's
// statements are all graph-ingest-hierarchy, and pkg/lifecycle writes under its own. A
// statement graph-ingest has added already carries this time, so reading after it gives the
// same answer. Every lane refuses a write that carries no statement before it reads this, so
// the time it returns is never zero.
func triggeringTime(entity *graph.EntityState) time.Time {
	var latest time.Time
	for _, triple := range entity.Triples {
		if triple.Timestamp.After(latest) {
			latest = triple.Timestamp
		}
	}
	return latest
}

// appendIndexingProfileTriple appends one entity.indexing.profile triple carrying at,
// the write's triggeringTime. Callers are responsible for having removed any prior value
// first.
func appendIndexingProfileTriple(entity *graph.EntityState, profile string, at time.Time) {
	entity.Triples = append(entity.Triples, message.Triple{
		Subject:    entity.ID,
		Predicate:  vocabulary.EntityIndexingProfile,
		Object:     profile,
		Source:     graph.SourceIndexingProfile,
		Timestamp:  at,
		Confidence: 1.0,
	})
}

// stampExplicitIndexingProfile sets entity.indexing.profile to an explicitly
// declared profile (replace-on-write, single-valued). Empty or unrecognized
// values are ignored — the entity then falls through to the fallback floor at
// its create seam rather than failing (lenient Phase 1 semantics). Used by the
// Graphable IndexingProfiler channel (extractEntityFromMessage) and the
// mutation-envelope channel (handleEntityCreateWithTriples). The statement carries at, the
// write's triggeringTime.
func stampExplicitIndexingProfile(entity *graph.EntityState, profile string, at time.Time) {
	if entity == nil || !vocabulary.IsValidIndexingProfile(profile) {
		return
	}
	removeIndexingProfileTriples(entity)
	appendIndexingProfileTriple(entity, profile, at)
}

// reconcileIndexingProfile is the entity-CREATION-seam stamp (SemStreams ADR-054 §5). It
// runs at every place an entity is born — createEntity, mergeEntityOnLane's
// first-write branch, and the stub→real upgrade in mergeEntityOnLane's merge branch —
// and never on a plain update of an already-profiled entity, so a profile is
// immutable once set. It enforces the single-valued invariant and applies the
// fallback floor when nothing was declared:
//
//   - ≥1 profile triple present (an explicit declaration was stamped upstream,
//     or an incoming producer is supplying one to an existing unprofiled entity): keep the FIRST and
//     drop any duplicates. No floor, no metric.
//   - 0 profile triples present: apply the floor registered with the type
//     (Registration.IndexingProfile, SemStreams ADR-103) and append it; increment
//     indexing_profile_default_total{message_type} ONLY when the registered type
//     declares no floor (control is the fail-safe default). The create seams
//     refuse an unregistered type before this runs.
//
// The floor statement carries at, the write's triggeringTime.
func (c *Component) reconcileIndexingProfile(entity *graph.EntityState, at time.Time) {
	if entity == nil {
		return
	}
	kept := false
	filtered := entity.Triples[:0]
	for _, t := range entity.Triples {
		if t.Predicate == vocabulary.EntityIndexingProfile {
			if kept {
				continue // single-valued: drop duplicates, keep the first
			}
			kept = true
		}
		filtered = append(filtered, t)
	}
	entity.Triples = filtered
	if kept {
		return
	}
	// No explicit declaration → apply the floor registered with the type
	// (SemStreams ADR-054 channel c, SemStreams ADR-103). The default-fallback metric fires ONLY when
	// the registered type declares no floor and silently took the control
	// default. A registered floor (e.g. agentic.request → trace) is a deliberate
	// classification, not an operator gap; the label now points at a
	// Registration literal.
	profile, floored := c.registeredIndexingProfile(entity.MessageType)
	appendIndexingProfileTriple(entity, profile, at)
	if !floored {
		c.indexingProfileDefault.WithLabelValues(indexingProfileMetricLabel(entity.MessageType)).Inc()
	}
}

// registeredIndexingProfile returns the floor registered with mt and whether
// the type declared one. A registered type with an empty floor and a type the
// registry does not hold both fall to control (fail-safe toward keeping the
// substrate reachable) with floored=false, which fires the default metric.
func (c *Component) registeredIndexingProfile(mt message.Type) (profile string, floored bool) {
	floor, registered := c.payloadRegistry.IndexingProfileFor(mt)
	if !registered || floor == "" {
		return vocabulary.IndexingProfileControl, false
	}
	return floor, true
}

// indexingProfileMetricLabel renders a message.Type as a stable, low-cardinality
// metric label, or "unknown" for any invalid Type. The IsValid guard
// (Type.Validate accepts it) prevents a partial Type from
// producing a non-semantic label like ".widget.v1" or "test.widget." — a
// malformed producer surfaces as "unknown" rather than a junk label key.
func indexingProfileMetricLabel(mt message.Type) string {
	if !mt.IsValid() {
		return "unknown"
	}
	return mt.Key()
}

// ============================================================================
// Entity Operations
// ============================================================================

// validateEntityID validates that an entity ID follows the expected format
func validateEntityID(id string) error {
	return semtypes.ValidateEntityID(id)
}

// CreateEntity atomically creates a new entity. Existing keys are never
// overwritten; callers observe natsclient.ErrKVKeyExists and decide whether
// that conflict is acceptable for their operation. A nil context is refused as
// invalid before anything is read or written.
func (c *Component) CreateEntity(ctx context.Context, entity *graph.EntityState) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "CreateEntity", "nil context")
	}
	_, _, err := c.createEntityWithReceipt(ctx, entity)
	return err
}

// mergeEntityOnLane ingests a streaming-consumer EntityState (typically built
// by extractEntityFromMessage from a Graphable arriving on the
// JetStream input) WITHOUT clobbering pre-existing triples on the
// entity. First write behaves like CreateEntity (the entity didn't
// exist; its fields land as given, a statement repeated equal in every
// field stored once); subsequent writes replace each
// (predicate, source) set the arrival carries, unless it is older than the
// stored set, keeping every other source's statements (graph.ReplaceBySource,
// design D15; the create-time indexing profile is preserved), and take the
// arrival's MessageType and StorageRef when no set was older (UpdatedAt always).
//
// Closes SemStreams gh#177: the prior code called CreateEntity (Put = full-
// replace) from the stream lane, which erased triples written through mutation
// request/reply. The JetStream consumer
// path was the lone outlier. Lifecycle-managed entities surfaced this
// most loudly: Manager.Create stamped the phase triple, then the
// first Graphable arrival via a downstream processor wiped it.
//
// Writes through replaceEntity (entity_writes.go), a CAS read-modify-write,
// so concurrent arrivals on the same Subject converge without racing.
//
// importLane carries the arrival lane so the authority gate can admit a
// foreign subject on a declared import port and refuse it everywhere else
// (ADR-102 d5); only ingestEntity, for such a port, passes true.
func (c *Component) mergeEntityOnLane(ctx context.Context, entity *graph.EntityState, importLane bool) error {
	if entity == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "mergeEntityOnLane", "entity cannot be nil")
	}
	// Entity-ID-only preflight (cheap subset): the ID is the CAS key and the
	// hierarchy-probe key, so reject a malformed ID before any KV I/O.
	//
	// SemStreams gh#562 write-cost: the full ValidateEntityStateContract pass that used to
	// run here was redundant with the MarshalEntityState write gate inside the
	// CAS closure — BOTH closure branches marshal a superset of the incoming
	// candidate (create: candidate + hierarchy/profile stamps; merge: every
	// incoming statement but a set older than the stored one, which is not
	// committed), so an invalid candidate never
	// commits under its own key, and classifyStoredStateRMWError keeps the
	// caller-vs-resident attribution honest. The Graphable ingest lane already
	// runs one caller-blaming full pass before side effects, so the pass here was a
	// third full validation per mutation on the
	// per-key-serialized hot path (SemStreams ADR-072). Two narrow ergonomic changes for
	// un-preflighted direct callers: (1) an incoming entity.indexing.profile
	// triple dropped pre-merge on an already-profiled entity, and a set older
	// than the stored one, are not validated — they are left out, not
	// committed, so the store contract is unaffected; (2) with EnableHierarchy set, an invalid candidate with a
	// valid ID reaches the pre-closure hierarchy step below, whose
	// AddToContainers COMMITS container entities
	// before the write gate rejects the candidate itself — that
	// pre-committed content is contract-valid and identical to what a later
	// legitimate birth of the same ID would create. References to absent entities
	// are valid graph facts and remain observable as unresolved references.
	if err := validateEntityID(entity.ID); err != nil {
		return err
	}
	// The write chokepoint's own authority gate (ADR-102 d5): the fact lane has
	// already run it in prepareFactProjection AND returned, so reaching a
	// rejection here means a DIRECT caller, which is why the recording is the
	// direct lane's and cannot double-count the fact lane's.
	if err := c.authorizeSubject(entity.ID, importLane); err != nil {
		return c.recordDirectAuthorityRejection(err)
	}
	if err := requireOwnStatements(entity.Triples); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "mergeEntityOnLane", "context cancelled")
	}
	// Read over the arrival's statements, stamped from the envelope, and any explicit profile
	// extractEntityFromMessage stamped with this same time: every statement graph-ingest derives
	// for this write carries it.
	at := triggeringTime(entity)

	// Hierarchy is added at birth only: the CAS callback's create branch applies the
	// membership statements, so a merge never appends them again (SemStreams gh#177). The read
	// below decides whether this arrival is a birth before the inference runs, so an
	// update pays no inference. If another writer creates the entity between this
	// read and the CAS, the callback's merge branch does not apply them: that
	// writer's birth is the birth. If the entity is deleted between the two, the CAS
	// finds the key absent after this read found it (readFound), so it writes nothing
	// and returns a transient error; the redelivery reads again and births the entity
	// with its hierarchy (design D21).
	var hierarchyTriples []message.Triple
	readFound := false
	if c.hierarchyInference != nil {
		_, err := c.entityBucket.Get(ctx, entity.ID)
		switch {
		case err == nil:
			// An update: no hierarchy.
			readFound = true
		case natsclient.IsKVNotFoundError(err):
			// A birth: born with its hierarchy statements or not at all (design D21).
			triples, herr := c.addToContainers(ctx, entity.ID, at)
			if herr != nil {
				return herr
			}
			hierarchyTriples = triples
		default:
			// A birth cannot be told from an update, and the CAS below could birth the
			// entity without its hierarchy, so nothing is written and the input is
			// delivered again (design D21).
			return errs.WrapTransient(err, "Component", "mergeEntityOnLane", "read entity before hierarchy")
		}
	}

	revision, err := c.replaceEntity(ctx, entity, hierarchyTriples, readFound, at)
	if err != nil {
		c.lifecycle.CountError()
		return errs.Wrap(err, "Component", "mergeEntityOnLane", "CAS update")
	}

	if revision == 0 {
		// No set applied and nothing was written (design D23): every set was older, or the only
		// statement was the profile of a profiled entity. replaceEntity counted and logged each
		// older set. The message is processed, but no entity was updated.
		return nil
	}
	// replaceEntity cleared the key's poison record and invalidated its cache entry.
	c.entitiesUpdated.Inc()

	c.logger.Debug("entity merged",
		slog.String("entity_id", entity.ID),
		slog.Int("triples_in", len(entity.Triples)))

	return nil
}

// addToContainers adds an entity being born to its containers and returns the
// membership statements its birth carries; with hierarchy off it returns none. The
// stream lane and the in-process create call it, and only for a birth. A failure of
// any part (a container's birth) is returned classified transient, and the
// caller stores nothing for the entity (design D21): the stream lane leaves the
// input unacknowledged, so it is delivered again, and the in-process caller gets the
// error. The class is transient whatever class the store gave its refusal: the
// arrival is not at fault, and the containers born before the failure are found by
// the next attempt, which births only those still absent. The inference's one refusal of its own, a zero time, is
// not reached: both callers have run requireOwnStatements, so at is set.
func (c *Component) addToContainers(ctx context.Context, entityID string, at time.Time) ([]message.Triple, error) {
	if c.hierarchyInference == nil {
		return nil, nil
	}
	triples, err := c.hierarchyInference.AddToContainers(ctx, entityID, at)
	if err != nil {
		return nil, errs.WrapTransient(err, "Component", "addToContainers", "hierarchy inference")
	}
	return triples, nil
}

func (c *Component) createEntityWithReceipt(
	ctx context.Context,
	entity *graph.EntityState,
) (*graph.EntityState, uint64, error) {
	if entity == nil {
		return nil, 0, errs.WrapInvalid(errs.ErrInvalidData, "Component", "CreateEntity", "entity cannot be nil")
	}
	// SemStreams ADR-103 d3: the in-process birth passes the same registered-type gate as
	// the RPC lane. The classified error goes back to the caller (on the hierarchy
	// container path it fails the birth that asked for the container, design D21);
	// it is not metered here.
	if err := c.requireRegisteredMessageType(entity); err != nil {
		return nil, 0, err
	}

	// Validate entity ID format
	if err := validateEntityID(entity.ID); err != nil {
		return nil, 0, err
	}
	// In-process creation is a LOCAL lane: hierarchy container births are minted
	// by this deployment, so a foreign subject here would be the framework
	// minting under a peer's authority (ADR-102 d5). Metered and logged under
	// the `direct` arrival, so the requirement's "each rejection is metered
	// exactly once and loudly logged" holds on this lane too — an in-process
	// birth has no RPC subject, not no destination.
	if err := c.authorizeSubject(entity.ID, false); err != nil {
		return nil, 0, c.recordDirectAuthorityRejection(err)
	}
	if err := graph.ValidateEntityStateContract(entity); err != nil {
		return nil, 0, errs.WrapInvalid(err, "Component", "CreateEntity", "validate entity state contract")
	}
	if err := requireOwnStatements(entity.Triples); err != nil {
		return nil, 0, err
	}
	// Read before graph-ingest adds hierarchy statements or the profile.
	at := triggeringTime(entity)

	// Check context
	if err := ctx.Err(); err != nil {
		return nil, 0, errs.Wrap(err, "Component", "CreateEntity", "context cancelled")
	}

	// Hierarchy before the write, so the entity is written once with its membership
	// statements, or not at all when the inference fails (design D21).
	hierarchyTriples, err := c.addToContainers(ctx, entity.ID, at)
	if err != nil {
		return nil, 0, err
	}
	entity.Triples = append(entity.Triples, hierarchyTriples...)

	// SemStreams ADR-054: stamp the indexing profile at the creation seam — keeps an
	// explicit declaration (envelope/Graphable, already on entity.Triples) and
	// otherwise applies the fallback floor + default metric.
	c.reconcileIndexingProfile(entity, at)
	entity.Triples = distinctStatements(entity.Triples)

	// Serialize entity (now includes hierarchy triples if enabled)
	data, err := graph.MarshalEntityState(entity)
	if err != nil {
		c.lifecycle.CountError()
		return nil, 0, errs.Wrap(err, "Component", "CreateEntity", "entity serialization")
	}

	// Atomic create-or-fail is the only admitted birth primitive. Preserve the
	// conflict sentinel so the calling component can make its own decision.
	committedRev, writeErr := c.createEntity(ctx, entity.ID, data)
	if writeErr != nil && errors.Is(writeErr, natsclient.ErrKVKeyExists) {
		return nil, 0, writeErr
	}
	if writeErr != nil {
		c.lifecycle.CountError()
		return nil, 0, errs.Wrap(writeErr, "Component", "CreateEntity", "KV store")
	}

	// createEntity cleared the key's poison record and invalidated its cache entry.
	c.entitiesUpdated.Inc()

	c.logger.Debug("entity created",
		slog.String("entity_id", entity.ID),
		slog.Int("triples", len(entity.Triples)))

	return entity.Clone(), committedRev, nil
}

func (c *Component) deleteEntityAtRevision(ctx context.Context, entityID string, revision uint64) error {
	if err := validateEntityID(entityID); err != nil {
		return err
	}
	// Deleting a foreign subject is a mutation of an imported mirror, refused
	// on every local lane (ADR-102 d5, ruled O-12(a)). handleCanonicalDelete
	// authorizes and returns before it reaches this body, so a rejection here is
	// a direct in-process delete and is metered as one.
	if err := c.authorizeSubject(entityID, false); err != nil {
		return c.recordDirectAuthorityRejection(err)
	}
	if revision == 0 {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "deleteEntityAtRevision", "revision must be nonzero")
	}
	if c.entityBucket == nil {
		return errors.New("ENTITY_STATES authority bucket unavailable")
	}
	if err := c.deleteEntity(ctx, entityID, revision); err != nil {
		// A revision mismatch is the fence refusing the caller's revision: the caller's
		// outcome, as on the reconcile lane and CreateEntity's key-exists, not a processing
		// error counted in ErrorCount.
		if !errors.Is(err, natsclient.ErrKVRevisionMismatch) {
			c.lifecycle.CountError()
		}
		return err
	}
	return nil
}

// ============================================================================
// Triple Operations
// ============================================================================

// recordSuppressedDuplicates meters triples the append path declined to store
// because the entity already carried them.
func (c *Component) recordSuppressedDuplicates(lane dedupLane, suppressed int) {
	if suppressed <= 0 {
		return
	}
	c.duplicateTriplesSuppressed.WithLabelValues(string(lane)).Add(float64(suppressed))
}

// addTriplesResult is one internal append batch's outcome. Grouped into a struct
// rather than a fifth and sixth return value so the batch handler can ask the
// two questions it actually needs — "did THIS subject commit, and at which
// revision" — without a positional argument pile.
type addTriplesResult struct {
	// Written counts NEWLY appended tuples only, excluding suppressed duplicates.
	Written int
	// Deduplicated counts tuples not appended because the entity already
	// carried them, or because the request repeated them.
	Deduplicated int
	// FailedSubjects maps each subject that rolled back to its per-subject error.
	FailedSubjects map[string]string
	// NotFoundSubjects identifies definite absent targets separately from
	// subject-local internal failures.
	NotFoundSubjects map[string]struct{}
	// SubjectErrors preserves typed subject-local failures for the canonical
	// partial-result wire without erasing earlier receipts.
	SubjectErrors map[string]error
	// UnchangedSubjects maps each subject whose submitted tuples were all already
	// present to the revision its CAS read: nothing was written, and the canonical
	// reply is unchanged at that revision, never at a later read's (design D23).
	UnchangedSubjects map[string]uint64
	// CommittedRevisions holds the EXACT revision each subject's CAS produced,
	// keyed by subject. A subject that was wholly suppressed or that failed is
	// ABSENT — presence in this map is the authoritative answer to "did this
	// call commit a write for that subject", which is what a caller must gate
	// on before attributing a revision to itself (Codex C1/C2).
	CommittedRevisions map[string]uint64
}

// addTriplesLane is AddTriples' body, carrying the lane label for the
// suppressed-duplicate metric and additionally reporting how many submitted
// tuples were suppressed as already-stored duplicates. writtenCount +
// deduplicated accounts for every triple whose subject committed, which is what
// lets a caller distinguish "already present" from "nothing happened" without
// an authoritative read-back.
func (c *Component) addTriplesLane(ctx context.Context, triples []message.Triple, lane dedupLane) (addTriplesResult, error) {
	if len(triples) == 0 {
		return addTriplesResult{}, nil
	}

	// Preserve the established positional diagnostic for an omitted mutation
	// subject. The authoritative candidate validator below remains acceptance
	// authority for all subject syntax, references, and predicates.
	for index := range triples {
		if triples[index].Subject == "" {
			return addTriplesResult{}, errs.WrapInvalid(errs.ErrInvalidData, "Component", "AddTriples", fmt.Sprintf("triple[%d] subject cannot be empty", index))
		}
	}

	// Validate the whole batch before any CAS read. The first subject supplies
	// the synthetic candidate root; every subject/reference/predicate is still
	// validated independently by the authoritative contract.
	if contractErr := graph.ValidateEntityStateContract(&graph.EntityState{
		ID: triples[0].Subject, Triples: triples,
	}); contractErr != nil {
		return addTriplesResult{}, errs.WrapInvalid(contractErr, "Component", "AddTriples", "validate batch contract")
	}
	// EVERY subject in the batch, not just the synthetic root: a batch is
	// rejected whole, so one foreign subject must not ride in beside local ones
	// (ADR-102 d5). One record for the batch, at the first refused subject:
	// the batch is one rejected operation, not one per subject.
	// handleCanonicalAppend authorizes every subject and returns before this
	// body, so the RPC lane never reaches here with a foreign subject.
	for index := range triples {
		if authErr := c.authorizeSubject(triples[index].Subject, false); authErr != nil {
			return addTriplesResult{}, c.recordDirectAuthorityRejection(authErr)
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return addTriplesResult{}, errs.Wrap(ctxErr, "Component", "AddTriples", "context cancelled")
	}

	// Group triples by Subject so each entity sees a single CAS.
	// Map iteration is non-deterministic; we sort subject keys before
	// committing so retries replay in stable order (helpful for tests
	// and for any operator reading a partial-failure trace).
	bySubject := make(map[string][]message.Triple, len(triples))
	for _, t := range triples {
		bySubject[t.Subject] = append(bySubject[t.Subject], t)
	}
	subjects := make([]string, 0, len(bySubject))
	for s := range bySubject {
		subjects = append(subjects, s)
	}
	sort.Strings(subjects)

	var writtenCount, deduplicated int
	failedSubjects := make(map[string]string)
	notFoundSubjects := make(map[string]struct{})
	subjectErrors := make(map[string]error)
	unchangedSubjects := make(map[string]uint64)
	committedRevisions := make(map[string]uint64)
	// allAbsences tracks whether EVERY per-subject failure was an SemStreams ADR-055
	// entity-not-found rejection. When true the aggregated error wraps
	// ErrKVKeyNotFound so the handler can set ErrorCodeEntityNotFound on the
	// response without string-sniffing. Mixed-reason batches (allAbsences=false)
	// stay un-sentineled → handler leaves ErrorCode empty (unclassified).
	allAbsences := true
	// poisonErr preserves the FIRST typed resident-poison failure so the
	// aggregated error can wrap it: without this the typed
	// *graph.StateContractError would die as a FailedSubjects string and the
	// whole-batch reply would misclassify as retryable-internal (design D4).
	var poisonErr error
	for i, subject := range subjects {
		// Short-circuit on context cancellation: don't burn the retry
		// budget for every remaining subject when the caller has
		// already given up. Mark the rest as failed-due-to-cancel so
		// the response shape is honest about what didn't commit, but
		// only count one operational error event for the cancellation
		// rather than N (one per remaining subject).
		if ctxErr := ctx.Err(); ctxErr != nil {
			for _, s := range subjects[i:] {
				failedSubjects[s] = ctxErr.Error()
				subjectErrors[s] = ctxErr
			}
			// A cancelled batch is NOT a pure entity-not-found batch.
			allAbsences = false
			c.lifecycle.CountError()
			break
		}
		group := bySubject[subject]
		appended, casErr := c.appendEntityTriples(ctx, subject, group)
		if casErr != nil {
			c.lifecycle.CountError()
			failedSubjects[subject] = casErr.Error()
			if errors.Is(casErr, natsclient.ErrKVKeyNotFound) {
				notFoundSubjects[subject] = struct{}{}
			} else {
				subjectErrors[subject] = casErr
			}
			var stateErr *graph.StateContractError
			if poisonErr == nil && errors.As(casErr, &stateErr) {
				poisonErr = casErr
			}
			if !errors.Is(casErr, natsclient.ErrKVKeyNotFound) {
				allAbsences = false
			}
			continue
		}
		// appended.suppressed is the count appendEntityTriples took against the
		// state its last CAS attempt read, so a retry never double-counts.
		c.recordSuppressedDuplicates(lane, appended.suppressed)
		deduplicated += appended.suppressed
		if appended.outcome == graph.MutationUnchanged {
			// A wholly-duplicate subject committed nothing but did not FAIL: it
			// stays out of failedSubjects, c.errors and allAbsences, which would
			// otherwise misclassify a mixed batch as a pure entity-not-found batch
			// (wrong ErrorCodeEntityNotFound on the reply).
			unchangedSubjects[subject] = appended.revision
			continue
		}
		// appendEntityTriples cleared the key's poison record and invalidated its
		// cache entry, so the next query reads the appended state.
		// Only NEWLY appended tuples count as written.
		writtenCount += len(group) - appended.suppressed
		// This subject COMMITTED, at exactly this revision.
		committedRevisions[subject] = appended.revision
		c.entitiesUpdated.Inc()
	}

	result := addTriplesResult{
		Written:            writtenCount,
		Deduplicated:       deduplicated,
		CommittedRevisions: committedRevisions,
		NotFoundSubjects:   notFoundSubjects,
		SubjectErrors:      subjectErrors,
		UnchangedSubjects:  unchangedSubjects,
	}
	if len(failedSubjects) == 0 {
		return result, nil
	}
	result.FailedSubjects = failedSubjects

	// Sort failed-subject names for stable error messages.
	failed := make([]string, 0, len(failedSubjects))
	for s := range failedSubjects {
		failed = append(failed, s)
	}
	sort.Strings(failed)
	var innerErr error
	switch {
	case allAbsences:
		// All failures were entity-not-found: wrap the sentinel so the handler
		// can set ErrorCodeEntityNotFound without string-sniffing.
		innerErr = fmt.Errorf("CAS update failed for %d/%d subjects: %v: %w",
			len(failedSubjects), len(subjects), failed, natsclient.ErrKVKeyNotFound)
	case poisonErr != nil:
		// Resident poison in the batch: wrap the typed cause so the aggregated
		// error stays errors.As-able as *graph.StateContractError and the
		// whole-batch reply classifies fatal/graph_state_reset_required instead
		// of dying as a FailedSubjects string.
		innerErr = fmt.Errorf("CAS update failed for %d/%d subjects: %v: %w",
			len(failedSubjects), len(subjects), failed, poisonErr)
	default:
		innerErr = fmt.Errorf("CAS update failed for %d/%d subjects: %v",
			len(failedSubjects), len(subjects), failed)
	}
	return result, errs.Wrap(innerErr, "Component", "AddTriples", "batch CAS partial failure")
}
