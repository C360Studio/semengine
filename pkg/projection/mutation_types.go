package projection

import (
	"context"
	"fmt"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// MutationClientConfig configures one immutable contract-validating client.
type MutationClientConfig struct {
	NATS      *natsclient.Client
	Contracts []Contract
	Timeout   time.Duration
}

// MutationMetadata carries correlation and triple provenance. Create and Append
// require RequestID and Source; Reconcile requires Source and sends it as the
// request's source. Every statement of a Create, Append or Reconcile needs a
// Timestamp, its own or this one: the client never reads the clock. A caller
// retrying one logical Create or Append must set Timestamp once and reuse the
// complete Metadata; regenerating it would change tuple identity. Other fields
// remain operation-specific.
type MutationMetadata struct {
	RequestID string
	TraceID   string
	Source    string
	Timestamp time.Time
}

// CreateMutation requests strict creation of one entity.
type CreateMutation struct {
	Contract string
	Entity   *graph.EntityState
	Triples  []message.Triple
	Metadata MutationMetadata
}

// ReconcileMutation requests replacement of one declared predicate group.
type ReconcileMutation struct {
	Contract string
	Group    string
	EntityID string
	// ExpectedRevision is the entity's revision the caller read. graph-ingest applies the
	// reconcile only while the entity is still at that revision, and otherwise refuses it with a
	// revision conflict and changes nothing. Zero means the caller names no revision: the client
	// fences at the revision of its own read, so a write between the caller's read and the
	// client's goes undetected.
	ExpectedRevision uint64
	Desired          []message.Triple
	Metadata         MutationMetadata
}

// AppendMutation requests set-valued addition to one declared predicate group.
type AppendMutation struct {
	Contract string
	Group    string
	EntityID string
	Triples  []message.Triple
	Metadata MutationMetadata
}

// DeleteMutation requests revision-fenced deletion of one entity.
type DeleteMutation struct {
	EntityID         string
	ExpectedRevision uint64
	Metadata         MutationMetadata
}

// CommitState reports what the client can prove about mutation commitment.
type CommitState string

const (
	// CommitNotCommitted proves the mutation did not commit.
	CommitNotCommitted CommitState = "not-committed"
	// CommitUnknown reports that the mutation may have committed: it was delivered, and neither
	// its reply nor its absence proves the outcome.
	CommitUnknown CommitState = "unknown"
	// CommitVerified reports a valid authoritative mutation response.
	CommitVerified CommitState = "verified"
)

// MutationReceipt contains the authoritative result of a mutation that committed. A method that
// fails returns a zero receipt, and its *MutationError's Commit reports what is proven.
type MutationReceipt struct {
	Entity     *graph.EntityState
	KVRevision uint64
}

// MutationOperation identifies the client operation that produced a result.
type MutationOperation string

const (
	// MutationOperationCreate identifies strict entity creation.
	MutationOperationCreate MutationOperation = "create"
	// MutationOperationReconcile identifies predicate-set reconciliation.
	MutationOperationReconcile MutationOperation = "reconcile"
	// MutationOperationAppend identifies set-valued triple addition.
	MutationOperationAppend MutationOperation = "append"
	// MutationOperationDelete identifies revision-fenced entity deletion.
	MutationOperationDelete MutationOperation = "delete"
	// MutationOperationReadAuthoritative identifies an exact authority read.
	MutationOperationReadAuthoritative MutationOperation = "read-authoritative"
)

// MutationErrorKind is the stable caller-facing mutation failure category.
type MutationErrorKind string

const (
	// MutationInvalid reports a request rejected before mutation.
	MutationInvalid MutationErrorKind = "invalid"
	// MutationNotFound reports a required entity that does not exist.
	MutationNotFound MutationErrorKind = "not-found"
	// MutationConflict reports strict-create or semantic conflict.
	MutationConflict MutationErrorKind = "conflict"
	// MutationRevisionConflict reports a stale expected authority revision.
	MutationRevisionConflict MutationErrorKind = "revision-conflict"
	// MutationUnavailable reports an operation that could not complete and wrote nothing. For a
	// write, the request was not delivered: no responder took it, or the caller's context ended
	// before it was sent. For a read (operation read-authoritative), the request may have been
	// delivered, but a read stores nothing.
	MutationUnavailable MutationErrorKind = "unavailable"
	// MutationCommitUnknown reports a mutation that may have committed (CommitUnknown).
	MutationCommitUnknown MutationErrorKind = "commit-unknown"
	// MutationInternal reports an unclassified framework failure.
	MutationInternal MutationErrorKind = "internal"
)

// MutationError preserves operation, classification, and commitment evidence.
type MutationError struct {
	Operation MutationOperation
	Kind      MutationErrorKind
	Code      string
	Class     errs.ErrorClass
	Commit    CommitState
	Detail    map[string]any
	Err       error
}

func (e *MutationError) Error() string {
	if e == nil {
		return "projection mutation failed"
	}
	return fmt.Sprintf("projection mutation %s failed (%s, %s): %v", e.Operation, e.Kind, e.Commit, e.Err)
}

func (e *MutationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// PredicateReconciler reconciles one declared predicate group. It is the one type both ends of
// the rule-pack binding name: service.ProjectionBinder receives it and the rule processor that
// binds the pack calls it, so neither names *MutationClient.
type PredicateReconciler interface {
	Reconcile(context.Context, ReconcileMutation) (MutationReceipt, error)
}

var _ PredicateReconciler = (*MutationClient)(nil)
