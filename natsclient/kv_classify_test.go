package natsclient

import (
	"errors"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// IsKVNotFoundError and IsKVConflictError read an error's type, never its text
// (owner ruling N, #91 comment 6080973822; #146): a refusal's text can name a
// stored entity, and an entity ID holding "10071" or "10037" must not decide
// whether a write retries or a read reports not-found.

// textOnlyKVError carries text in an error whose type is none the classifiers
// accept, in the shape of the write seam's key-check refusal (#146).
func textOnlyKVError(text string) error {
	return fmt.Errorf("stored entity c360.ops.robotics.gcs.drone.%s does not match its key", text)
}

func TestIsKVNotFoundErrorClassifiesByType(t *testing.T) {
	typed := []struct {
		name string
		err  error
	}{
		{"ErrKVKeyNotFound", ErrKVKeyNotFound},
		{"jetstream.ErrKeyNotFound", jetstream.ErrKeyNotFound},
		{"jetstream.ErrKeyDeleted", jetstream.ErrKeyDeleted},
		{"APIError 10037", &jetstream.APIError{
			Code: 404, ErrorCode: jetstream.JSErrCodeMessageNotFound, Description: "no message found",
		}},
	}
	for _, tc := range typed {
		if !IsKVNotFoundError(tc.err) {
			t.Errorf("%s: IsKVNotFoundError = false, want true", tc.name)
		}
		if wrapped := fmt.Errorf("kv get c360.ops.robotics.gcs.drone.001: %w", tc.err); !IsKVNotFoundError(wrapped) {
			t.Errorf("%s wrapped with %%w: IsKVNotFoundError = false, want true", tc.name)
		}
	}

	// Each string the pin's text match read (kv.go:700-703 at 8b99efe9).
	for _, text := range []string{"key not found", "key was deleted", "10037"} {
		if err := textOnlyKVError(text); IsKVNotFoundError(err) {
			t.Errorf("IsKVNotFoundError(%q) = true, want false: a text match decided the class", err)
		}
	}
	otherCode := &jetstream.APIError{
		Code: 400, ErrorCode: jetstream.JSErrCodeBadRequest, Description: "key not found, key was deleted, 10037",
	}
	if IsKVNotFoundError(otherCode) {
		t.Errorf("IsKVNotFoundError(%q) = true, want false: a text match decided the class", otherCode)
	}

	for _, err := range []error{nil, ErrKVKeyExists, ErrKVRevisionMismatch, jetstream.ErrKeyExists} {
		if IsKVNotFoundError(err) {
			t.Errorf("IsKVNotFoundError(%v) = true, want false", err)
		}
	}
}

func TestIsKVConflictErrorClassifiesByType(t *testing.T) {
	typed := []struct {
		name string
		err  error
	}{
		{"ErrKVRevisionMismatch", ErrKVRevisionMismatch},
		{"ErrKVKeyExists", ErrKVKeyExists},
		{"jetstream.ErrKeyExists", jetstream.ErrKeyExists},
		{"APIError 10071", &jetstream.APIError{
			Code: 400, ErrorCode: jetstream.JSErrCodeStreamWrongLastSequence, Description: "wrong last sequence: 3",
		}},
		// A replicated stream reports a CAS conflict as 10164, "wrong last sequence",
		// which the pin's text match also read (nats.go v1.54.0 jetstream/errors.go:61-64).
		{"APIError 10164", &jetstream.APIError{
			Code: 400, ErrorCode: jetstream.JSErrCodeStreamWrongLastSequenceConstant, Description: "wrong last sequence",
		}},
		{"APIError 10058", &jetstream.APIError{
			Code: 400, ErrorCode: jetstream.JSErrCodeStreamNameInUse,
			Description: "stream name already in use with a different configuration",
		}},
	}
	for _, tc := range typed {
		if !IsKVConflictError(tc.err) {
			t.Errorf("%s: IsKVConflictError = false, want true", tc.name)
		}
		if wrapped := fmt.Errorf("kv update c360.ops.robotics.gcs.drone.001: %w", tc.err); !IsKVConflictError(wrapped) {
			t.Errorf("%s wrapped with %%w: IsKVConflictError = false, want true", tc.name)
		}
	}

	// Each string the pin's text match read (kv.go:716-720 at 8b99efe9).
	for _, text := range []string{"wrong last sequence", "10071", "key exists", "10058"} {
		if err := textOnlyKVError(text); IsKVConflictError(err) {
			t.Errorf("IsKVConflictError(%q) = true, want false: a text match decided the class", err)
		}
	}
	otherCode := &jetstream.APIError{
		Code: 400, ErrorCode: jetstream.JSErrCodeBadRequest,
		Description: "wrong last sequence, 10071, key exists, 10058",
	}
	if IsKVConflictError(otherCode) {
		t.Errorf("IsKVConflictError(%q) = true, want false: a text match decided the class", otherCode)
	}

	for _, err := range []error{nil, ErrKVKeyNotFound, jetstream.ErrKeyNotFound, errors.New("kv: unrelated")} {
		if IsKVConflictError(err) {
			t.Errorf("IsKVConflictError(%v) = true, want false", err)
		}
	}
}
