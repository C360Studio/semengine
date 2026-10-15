package projection_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/projection"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// TestClassifyMutationErrorMapsEachOutcome holds the mapping to design D1's table, one row per
// table line and per pre-write refusal code. The expected values are written from the table, not
// read from the code: a classified reply whose code graph-ingest sends only before it writes is not
// committed with the reply's class; a request that was not delivered is not committed and
// transient; anything else is commit-unknown and fatal.
func TestClassifyMutationErrorMapsEachOutcome(t *testing.T) {
	detail := map[string]any{"entity_id": "c360.platform.test.system.widget.one"}
	reply := func(class errs.ErrorClass, code string) error {
		classified := errs.ClassifiedCode(class, code, errors.New("graph-ingest reply"))
		classified.Detail = detail
		// The wire client wraps a classified reply this way (graphmutation.Client.request).
		return fmt.Errorf("graph mutation create_entity rejected: %w", classified)
	}
	transport := func(outcome graphmutation.TransportOutcome, err error) error {
		return &graphmutation.TransportError{Operation: graphmutation.CreateEntity, Outcome: outcome, Err: err}
	}
	tests := []struct {
		name   string
		err    error
		commit projection.CommitState
		kind   projection.MutationErrorKind
		class  errs.ErrorClass
		code   string
	}{
		// Pre-write refusals: not committed, the kind by code, the reply's class and code.
		{"invalid request", reply(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest),
			projection.CommitNotCommitted, projection.MutationInvalid, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest},
		{"structural invalid", reply(errs.ErrorInvalid, graph.ErrorCodeStructuralInvalid),
			projection.CommitNotCommitted, projection.MutationInvalid, errs.ErrorInvalid, graph.ErrorCodeStructuralInvalid},
		{"message type unregistered", reply(errs.ErrorInvalid, graph.ErrorCodeMessageTypeUnregistered),
			projection.CommitNotCommitted, projection.MutationInvalid, errs.ErrorInvalid, graph.ErrorCodeMessageTypeUnregistered},
		{"entity ID invalid", reply(errs.ErrorInvalid, semtypes.ErrorCodeEntityIDInvalid),
			projection.CommitNotCommitted, projection.MutationInvalid, errs.ErrorInvalid, semtypes.ErrorCodeEntityIDInvalid},
		{"entity ID authority invalid", reply(errs.ErrorInvalid, semtypes.ErrorCodeEntityIDAuthorityInvalid),
			projection.CommitNotCommitted, projection.MutationInvalid, errs.ErrorInvalid, semtypes.ErrorCodeEntityIDAuthorityInvalid},
		{"revision mismatch", reply(errs.ErrorInvalid, graph.ErrorCodeRevisionMismatch),
			projection.CommitNotCommitted, projection.MutationRevisionConflict, errs.ErrorInvalid, graph.ErrorCodeRevisionMismatch},
		{"entity exists", reply(errs.ErrorInvalid, graph.ErrorCodeEntityExists),
			projection.CommitNotCommitted, projection.MutationConflict, errs.ErrorInvalid, graph.ErrorCodeEntityExists},
		{"entity not found", reply(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound),
			projection.CommitNotCommitted, projection.MutationNotFound, errs.ErrorInvalid, graph.ErrorCodeEntityNotFound},
		{"graph state reset required", reply(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired),
			projection.CommitNotCommitted, projection.MutationInternal, errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired},

		// Not delivered: not committed, unavailable, transient, no code.
		{"transport unavailable", transport(graphmutation.TransportUnavailable, nats.ErrNoResponders),
			projection.CommitNotCommitted, projection.MutationUnavailable, errs.ErrorTransient, ""},
		{"raw no responders", nats.ErrNoResponders,
			projection.CommitNotCommitted, projection.MutationUnavailable, errs.ErrorTransient, ""},
		{"context cancelled before send", transport(graphmutation.TransportDeadline, context.Canceled),
			projection.CommitNotCommitted, projection.MutationUnavailable, errs.ErrorTransient, ""},
		{"deadline passed before send", transport(graphmutation.TransportDeadline, context.DeadlineExceeded),
			projection.CommitNotCommitted, projection.MutationUnavailable, errs.ErrorTransient, ""},

		// natsclient's refusals before sending stay commit-unknown until #268.
		{"not connected, wrapped as commit unknown", transport(graphmutation.TransportCommitUnknown, natsclient.ErrNotConnected),
			projection.CommitUnknown, projection.MutationCommitUnknown, errs.ErrorFatal, ""},
		{"circuit open, wrapped as commit unknown", transport(graphmutation.TransportCommitUnknown, natsclient.ErrCircuitOpen),
			projection.CommitUnknown, projection.MutationCommitUnknown, errs.ErrorFatal, ""},

		// Everything else: commit-unknown and fatal, the reply's code kept.
		{"timeout after possible delivery", transport(graphmutation.TransportCommitUnknown, nats.ErrTimeout),
			projection.CommitUnknown, projection.MutationCommitUnknown, errs.ErrorFatal, ""},
		{"unclassified", errors.New("something else"),
			projection.CommitUnknown, projection.MutationCommitUnknown, errs.ErrorFatal, ""},
		{"transient internal reply", reply(errs.ErrorTransient, graph.ErrorCodeInternal),
			projection.CommitUnknown, projection.MutationCommitUnknown, errs.ErrorFatal, graph.ErrorCodeInternal},
		{"invalid reply with another code", reply(errs.ErrorInvalid, "response_too_large"),
			projection.CommitUnknown, projection.MutationCommitUnknown, errs.ErrorFatal, "response_too_large"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := projection.ClassifyMutationError(projection.MutationOperationDelete, test.err)
			if got == nil {
				t.Fatal("ClassifyMutationError returned nil for a non-nil error")
			}
			if got.Operation != projection.MutationOperationDelete || got.Commit != test.commit ||
				got.Kind != test.kind || got.Class != test.class || got.Code != test.code {
				t.Errorf("operation %q, commit %q, kind %q, class %q, code %q; want %q, %q, %q, %q, %q",
					got.Operation, got.Commit, got.Kind, got.Class, got.Code,
					projection.MutationOperationDelete, test.commit, test.kind, test.class, test.code)
			}
			if !errors.Is(got, test.err) {
				t.Errorf("errors.Is does not reach the cause: %v", got)
			}
			// errs reads a commit-unknown error as fatal, so it is never retried. For a request that
			// was not delivered only the Class field is promised: a raw nats.ErrNoResponders
			// underneath stays unclassified.
			if test.commit == projection.CommitUnknown && (errs.Classify(got) != errs.ErrorFatal || errs.IsTransient(got)) {
				t.Errorf("errs.Classify = %q, IsTransient %t; want fatal, false", errs.Classify(got), errs.IsTransient(got))
			}
			wantDetail := map[string]any(nil)
			if test.code != "" {
				wantDetail = detail
			}
			if !reflect.DeepEqual(got.Detail, wantDetail) {
				t.Errorf("detail %v, want %v", got.Detail, wantDetail)
			}
		})
	}
}

func TestClassifyMutationErrorReturnsNilForNil(t *testing.T) {
	if got := projection.ClassifyMutationError(projection.MutationOperationCreate, nil); got != nil {
		t.Errorf("ClassifyMutationError(nil) = %#v, want nil", got)
	}
}

// The returned detail is a copy: changing it leaves the reply's own detail as it was.
func TestClassifyMutationErrorCopiesTheReplysDetail(t *testing.T) {
	classified := errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeRevisionMismatch, errors.New("stale"))
	classified.Detail = map[string]any{"expected_revision": 3}
	got := projection.ClassifyMutationError(projection.MutationOperationReconcile, classified)
	got.Detail["expected_revision"] = 4
	if classified.Detail["expected_revision"] != 3 {
		t.Errorf("the reply's detail changed to %v; the error must hold a copy", classified.Detail)
	}
}
