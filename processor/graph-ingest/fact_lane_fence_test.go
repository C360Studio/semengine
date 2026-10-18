package graphingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
)

// A payload that fails validation or panics in its own code on the Graphable lane
// is poison: counted, logged with its payload type and subject, and terminated,
// never returned for redelivery (design D8 Q13; graph-ingest-recovery, "A failing
// payload is poison, not a redelivery loop"). Carried from SemStreams PR #1437 at
// 0ea823a6 (processor/graph-ingest/fact_lane_fence_test.go), on a payload type the
// test registers, as a product registers its own.

// fenceTestMode selects how a decoded fenceTestPayload misbehaves. It travels
// on the wire, so the decoded copy carries it.
const (
	fenceModeConforming   = "conforming"
	fenceModeInvalid      = "invalid"
	fenceModePanicID      = "panic-entity-id"
	fenceModePanicTriples = "panic-triples"
	fenceModePanicValid   = "panic-validate"
	fenceModePanicDecode  = "panic-unmarshal"
)

// fenceEntityID is under c360.test, the authority the fence fixtures build under.
const fenceEntityID = "c360.test.fence.sys.widget.001"

// fenceSubject is the subject the unit tests name a message by.
const fenceSubject = "entity.fence"

// fenceCallRecorder counts the identity calls made on decoded payloads. The
// registry factory hands every decoded instance the same recorder.
type fenceCallRecorder struct {
	entityIDCalls atomic.Int64
	triplesCalls  atomic.Int64
}

type fenceTestPayload struct {
	ID   string `json:"id"`
	Mode string `json:"mode"`

	rec *fenceCallRecorder
}

func fenceTestType() message.Type {
	return message.Type{Domain: "test", Category: "fence", Version: "v1"}
}

func (p *fenceTestPayload) Schema() message.Type { return fenceTestType() }

func (p *fenceTestPayload) Validate() error {
	switch p.Mode {
	case fenceModeInvalid:
		return errors.New("fence test payload: mode invalid")
	case fenceModePanicValid:
		panic("fence test payload: Validate boom")
	}
	return nil
}

func (p *fenceTestPayload) EntityID() string {
	if p.rec != nil {
		p.rec.entityIDCalls.Add(1)
	}
	if p.Mode == fenceModePanicID {
		panic("fence test payload: EntityID boom")
	}
	return p.ID
}

func (p *fenceTestPayload) Triples() []message.Triple {
	if p.rec != nil {
		p.rec.triplesCalls.Add(1)
	}
	if p.Mode == fenceModePanicTriples {
		panic("fence test payload: Triples boom")
	}
	return []message.Triple{{Subject: p.ID, Predicate: "test.fence.mode", Object: p.Mode, Confidence: 1}}
}

func (p *fenceTestPayload) MarshalJSON() ([]byte, error) {
	type alias fenceTestPayload
	return json.Marshal((*alias)(p))
}

func (p *fenceTestPayload) UnmarshalJSON(data []byte) error {
	type alias fenceTestPayload
	if err := json.Unmarshal(data, (*alias)(p)); err != nil {
		return err
	}
	if p.Mode == fenceModePanicDecode {
		panic("fence test payload: UnmarshalJSON boom")
	}
	return nil
}

// withFenceTestPayload registers the fence test type in the deployment's payload
// registry, the one the factory builds the component's decoder from. Every
// instance the registry decodes reports to rec (nil records nothing).
func withFenceTestPayload(tb testing.TB, rec *fenceCallRecorder) testComponentOption {
	return func(deps *component.Dependencies) {
		tb.Helper()
		typ := fenceTestType()
		if err := deps.PayloadRegistry.Register(&payloadregistry.Registration{
			Domain:      typ.Domain,
			Category:    typ.Category,
			Version:     typ.Version,
			Description: "graph-ingest validate-then-fence test payload",
			Factory:     func() any { return &fenceTestPayload{rec: rec} },
		}); err != nil {
			tb.Fatalf("register fence test payload: %v", err)
		}
	}
}

// newFenceTestComponent returns a component whose decoder resolves the fence
// test type, the recorder every decoded instance reports to, and its bucket.
func newFenceTestComponent(t *testing.T) (*Component, *fenceCallRecorder, *mockKVBucket) {
	t.Helper()
	rec := &fenceCallRecorder{}
	comp, bucket := createTestComponentWithMockKVBucket(t, withAuthority("c360", "test"), withFenceTestPayload(t, rec))
	return comp, rec, bucket
}

func storedKeyCount(bucket *mockKVBucket) int {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	return len(bucket.data)
}

// fenceWireJSON hand-writes the BaseMessage wire form. The invalid mode cannot
// be produced through BaseMessage.MarshalJSON (it validates), which is exactly
// the producer that bypasses the envelope this lane must still refuse.
func fenceWireJSON(t *testing.T, mode string) []byte {
	t.Helper()
	typ := fenceTestType()
	wire := map[string]any{
		"id":   "fence-msg-001",
		"type": map[string]string{"domain": typ.Domain, "category": typ.Category, "version": typ.Version},
		"payload": map[string]string{
			"id":   fenceEntityID,
			"mode": mode,
		},
		"meta": map[string]any{"created_at": 1_790_000_000_000, "received_at": 1_790_000_000_000, "source": "fence-test"},
	}
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	return data
}

// requireInvalidClass reads err's class from the ClassifiedError it wraps.
func requireInvalidClass(t *testing.T, err error) {
	t.Helper()
	var classified *errs.ClassifiedError
	require.True(t, errors.As(err, &classified), "the error is not a classified error: %v", err)
	assert.Equal(t, errs.ErrorInvalid, classified.Class, "the error's class: %v", err)
}

func TestDecodeEntity_ValidateFailureRejectsBeforeIdentity(t *testing.T) {
	comp, rec, _ := newFenceTestComponent(t)

	entity, err := comp.decodeEntity(fenceSubject, fenceWireJSON(t, fenceModeInvalid))

	require.Error(t, err)
	assert.Nil(t, entity)
	requireInvalidClass(t, err)
	assert.Contains(t, err.Error(), "payload validation failed")
	assert.Contains(t, err.Error(), "mode invalid")
	assert.Contains(t, err.Error(), fenceTestType().String(), "the error names the payload type")
	assert.Contains(t, err.Error(), fenceSubject, "the error names the subject")
	assert.Zero(t, rec.entityIDCalls.Load(), "EntityID must not run on a payload that failed Validate")
	assert.Zero(t, rec.triplesCalls.Load(), "Triples must not run on a payload that failed Validate")
}

// The fence covers the whole lane entry: the payload's UnmarshalJSON (inside
// Decode), its Validate, and its identity methods. Every row's error names the
// payload type and the subject; the PR's named only the subject for a panic
// inside Decode, which the spec's log requirement does not allow.
func TestDecodeEntity_FencesPayloadPanics(t *testing.T) {
	tests := []struct {
		mode      string
		recovered string
	}{
		{mode: fenceModePanicDecode, recovered: "UnmarshalJSON boom"},
		{mode: fenceModePanicValid, recovered: "Validate boom"},
		{mode: fenceModePanicID, recovered: "EntityID boom"},
		{mode: fenceModePanicTriples, recovered: "Triples boom"},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			comp, _, _ := newFenceTestComponent(t)
			data := fenceWireJSON(t, tt.mode)

			var (
				entity any
				err    error
			)
			require.NotPanics(t, func() {
				entity, err = comp.decodeEntity(fenceSubject, data)
			})
			require.Error(t, err)
			assert.Nil(t, entity)
			requireInvalidClass(t, err)
			assert.Contains(t, err.Error(), fenceTestType().String(), "the error names the payload type")
			assert.Contains(t, err.Error(), fenceSubject, "the error names the subject")
			assert.Contains(t, err.Error(), tt.recovered, "the error names the recovered value")
		})
	}
}

// The poison accounting is the lane's existing path: handleMessage counts the
// rejection and returns; nothing escapes and nothing is persisted. The
// conforming row is the control that makes "nothing persisted" observable.
func TestHandleMessage_RejectionsLandOnPoisonAccounting(t *testing.T) {
	tests := []struct {
		mode       string
		wantErrors int64
		wantStored int
	}{
		{mode: fenceModeConforming, wantErrors: 0, wantStored: 1},
		{mode: fenceModeInvalid, wantErrors: 1, wantStored: 0},
		{mode: fenceModePanicID, wantErrors: 1, wantStored: 0},
		{mode: fenceModePanicTriples, wantErrors: 1, wantStored: 0},
		{mode: fenceModePanicValid, wantErrors: 1, wantStored: 0},
		{mode: fenceModePanicDecode, wantErrors: 1, wantStored: 0},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			comp, _, bucket := newFenceTestComponent(t)
			data := fenceWireJSON(t, tt.mode)

			require.NotPanics(t, func() {
				comp.handleMessage(t.Context(), fenceSubject, data)
			})

			assert.Equal(t, tt.wantErrors, atomic.LoadInt64(&comp.errors), "poison count for mode %s", tt.mode)
			assert.Equal(t, tt.wantStored, storedKeyCount(bucket), "persisted entities for mode %s", tt.mode)
		})
	}
}

// TestStreamLane_FailingPayloadIsTerminated drives the consume closure Start wires
// for the entity_stream port (scenario "Payload panics while giving its entity
// ID", and every other failing payload the requirement names): the message is
// terminated, not acknowledged or returned, the error count `Health()` reports
// rises by one, the WARN line names the payload type and subject,
// and the next message on the stream is applied.
func TestStreamLane_FailingPayloadIsTerminated(t *testing.T) {
	for _, mode := range []string{
		fenceModePanicID, fenceModeInvalid, fenceModePanicTriples, fenceModePanicValid, fenceModePanicDecode,
	} {
		t.Run(mode, func(t *testing.T) {
			c, handler := startStreamLane(t, withFenceTestPayload(t, nil))
			// startStreamLane gives the lane the merge-test decoder; this test's type is in
			// the component's own registry.
			c.decoder = message.NewDecoder(c.payloadRegistry)
			var logs bytes.Buffer
			c.logger = slog.New(slog.NewTextHandler(&logs, nil))
			before := atomic.LoadInt64(&c.errors)

			assert.Equal(t, "term", deliverStreamLane(t, handler, fenceWireJSON(t, mode), 1), "disposition")
			assert.Equal(t, before+1, atomic.LoadInt64(&c.errors), "the poison count rises by one")
			// The subject is the delivered message's (streamLaneMsg.Subject), not the
			// port's filter.
			logLine := logs.String()
			for _, want := range []string{"level=WARN", "subject=entity.statement-metadata", fenceTestType().String()} {
				assert.Contains(t, logLine, want, "the poison log line")
			}
			_, _, stored := storedRevision(t, c, fenceEntityID)
			assert.False(t, stored, "nothing is persisted for the poison message")

			assert.Equal(t, "ack", deliverStreamLane(t, handler, fenceWireJSON(t, fenceModeConforming), 2),
				"the next message's disposition")
			_, _, stored = storedRevision(t, c, fenceEntityID)
			assert.True(t, stored, "the next message on the stream is applied")
			assert.Equal(t, before+1, atomic.LoadInt64(&c.errors), "the poison message is counted once")
		})
	}
}
