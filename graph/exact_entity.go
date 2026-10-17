package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// ExactEntity is one validated authority value and the revision of the same
// ENTITY_STATES entry. KVRevision is the entry's only revision: the stored
// value carries none of its own.
type ExactEntity struct {
	Entity     *EntityState `json:"entity"`
	KVRevision uint64       `json:"kv_revision"`
}

// EntityRequest is the entity verb's request: the ID of the one entity to read.
type EntityRequest struct {
	ID string `json:"id"`
}

// classifiedRequester is the request/reply operation the exact reader sends
// through: natsclient's classified request, which the root does not import.
type classifiedRequester interface {
	RequestClassified(context.Context, string, []byte, time.Duration) ([]byte, error)
}

// ExactEntityReader is the operation-specific embedded authority read: one
// entity by its ID, through graph-ingest's entity verb, refused under the
// stored-value rule (UnmarshalEntityState). It exposes no raw subject, KV
// handle, or aggregate graph-client capability. Build it with
// NewExactEntityReader; the zero value refuses every read.
type ExactEntityReader struct {
	requester classifiedRequester
	timeout   time.Duration
}

// NewExactEntityReader constructs the exact reader over requester. It refuses
// a nil requester and a negative timeout; a zero timeout means the requester's
// default. A typed-nil pointer is not detected.
func NewExactEntityReader(requester classifiedRequester, timeout time.Duration) (*ExactEntityReader, error) {
	if requester == nil {
		return nil, errors.New("exact entity reader requester is required")
	}
	// natsclient replaces only a zero timeout with its default, so a negative
	// one would give every request an already expired context: each read would
	// fail while the caller's context is live.
	if timeout < 0 {
		return nil, fmt.Errorf("exact entity reader timeout must not be negative, got %s", timeout)
	}
	// A zero timeout is passed through: the requester applies its own default
	// (natsclient's RequestClassified uses DefaultRequestTimeout), so the root
	// imports no transport package for it.
	return &ExactEntityReader{requester: requester, timeout: timeout}, nil
}

// ReadExactEntity reads entityID and the KV revision of its stored value. A
// stored value the stored-value rule refuses is returned as that refusal.
func (r *ExactEntityReader) ReadExactEntity(ctx context.Context, entityID string) (*ExactEntity, error) {
	if r == nil || r.requester == nil {
		return nil, errors.New("exact entity reader is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("exact entity context is required")
	}
	if err := semtypes.ValidateEntityID(entityID); err != nil {
		return nil, errs.ClassifiedCode(errs.ErrorInvalid, ErrorCodeInvalidRequest,
			fmt.Errorf("exact entity ID: %w", err))
	}
	request, err := json.Marshal(EntityRequest{ID: entityID})
	if err != nil {
		return nil, fmt.Errorf("marshal exact entity request: %w", err)
	}
	response, err := r.requester.RequestClassified(ctx, entityVerb.Subject, request, r.timeout)
	if err != nil {
		return nil, err
	}
	// The reply is an ExactEntity. Entity shadows the embedded field so the stored
	// value is decoded once, by the stored-value rule; encoding/json takes the
	// shallower entity and the embedded kv_revision.
	var envelope struct {
		ExactEntity
		Entity json.RawMessage `json:"entity"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		return nil, exactResponseError(fmt.Errorf("decode exact entity response: %w", err))
	}
	if len(envelope.Entity) == 0 || string(envelope.Entity) == "null" {
		return nil, exactResponseError(errors.New("exact entity response has no entity"))
	}
	if envelope.KVRevision == 0 {
		return nil, exactResponseError(errors.New("exact entity response has zero KV revision"))
	}
	var entity EntityState
	if err := UnmarshalEntityState(entityID, envelope.Entity, &entity); err != nil {
		return nil, err
	}
	return &ExactEntity{Entity: &entity, KVRevision: envelope.KVRevision}, nil
}

func exactResponseError(err error) error {
	return errs.ClassifiedCode(errs.ErrorFatal, ErrorCodeInternal, err)
}
