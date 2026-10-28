package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
)

// TestClassifyWriteDisposition is the JetStream settlement table (design D4, the pin's
// classifyWriteDisposition, pin component_ack_test.go): a write that returned nil is acked, even
// under an ended context; any failure under an ended context is NAKed with the shutdown delay,
// even an invalid one, so a shutdown never terminates a message it could still store; an invalid
// failure is terminated; anything else is NAKed with the transient delay.
func TestClassifyWriteDisposition(t *testing.T) {
	invalid := errs.WrapInvalid(errors.New("bad entity id"), "Store", "StoreContent", "validate entity ID")
	transient := errs.WrapTransient(errors.New("backend down"), "Store", "StoreContent", "store content")
	for _, tc := range []struct {
		name            string
		ctxErr, procErr error
		want            writeDisposition
	}{
		{"nil error acks", nil, nil, dispositionAck},
		{"nil error acks under an ended context", context.Canceled, nil, dispositionAck},
		{"invalid terminates", nil, invalid, dispositionTerm},
		{"transient naks with the transient delay", nil, transient, dispositionNakTransient},
		{"unclassified naks with the transient delay", nil, errors.New("kaboom"), dispositionNakTransient},
		{"cancellation owns a transient error", context.Canceled,
			errs.WrapTransient(context.Canceled, "Store", "StoreContent", "store content"), dispositionNakShutdown},
		{"cancellation owns an invalid error", context.Canceled, invalid, dispositionNakShutdown},
		{"a deadline owns the outcome", context.DeadlineExceeded,
			errs.WrapTransient(context.DeadlineExceeded, "Store", "StoreContent", "store content"), dispositionNakShutdown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyWriteDisposition(tc.ctxErr, tc.procErr); got != tc.want {
				t.Fatalf("classifyWriteDisposition(%v, %v) = %v, want %v", tc.ctxErr, tc.procErr, got, tc.want)
			}
		})
	}
}

// TestComponentRefusesInputWithoutPayloadRegistry: an input decodes every message through the
// payload registry, so a Component with an input and no registry is refused at construction,
// naming it, rather than refusing every message it receives; a Component with no input offers
// its Store only and needs none.
func TestComponentRefusesInputWithoutPayloadRegistry(t *testing.T) {
	_, err := newComponent(json.RawMessage(`{}`), component.Dependencies{})
	if err == nil || !strings.Contains(err.Error(), "payload registry") {
		t.Fatalf("default ports without a payload registry: err = %v, want a refusal naming the payload registry", err)
	}
	if _, err := newComponent(json.RawMessage(`{"ports": {}}`), component.Dependencies{}); err != nil {
		t.Fatalf("no inputs without a payload registry: %v", err)
	}
}
