package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// TestQueryRefusalsCarryACode: every refusal of the entity, batch and prefix query verbs
// carries a classified code, and giving it one does not change its class (#300 section B;
// design D3). The wanted class and code are spelled out, not taken from the constants the
// handlers use; the entity verb's rows hold its codes as they were.
func TestQueryRefusalsCarryACode(t *testing.T) {
	const (
		stored = "acme.ops.robotics.gcs.drone.001"
		absent = "acme.ops.robotics.gcs.drone.404"
	)
	kvDown := func(context.Context, string) (jetstream.KeyValueEntry, error) {
		return nil, errors.New("injected KV failure")
	}
	listDown := func(context.Context, ...string) (jetstream.KeyLister, error) {
		return nil, errors.New("injected listing failure")
	}
	request := func(t *testing.T, value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		return data
	}
	entity := func(c *Component, data []byte) error {
		_, err := c.handleQueryEntityNATS(context.Background(), data)
		return err
	}
	batch := func(c *Component, data []byte) error {
		_, err := c.handleQueryBatchNATS(context.Background(), data)
		return err
	}
	prefix := func(c *Component, data []byte) error {
		_, err := c.handleQueryPrefixWithMaxPayload(context.Background(), data, 1<<20)
		return err
	}

	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, c *Component, bucket *mockKVBucket)
		call  func(t *testing.T, c *Component) error
		class errs.ErrorClass
		code  string
	}{
		{
			name:  "entity: malformed request",
			call:  func(_ *testing.T, c *Component) error { return entity(c, []byte("{")) },
			class: errs.ErrorInvalid, code: "invalid_request",
		},
		{
			name:  "entity: malformed ID",
			call:  func(t *testing.T, c *Component) error { return entity(c, request(t, graph.EntityRequest{ID: "bad"})) },
			class: errs.ErrorInvalid, code: "invalid_request",
		},
		{
			name:  "entity: absent",
			call:  func(t *testing.T, c *Component) error { return entity(c, request(t, graph.EntityRequest{ID: absent})) },
			class: errs.ErrorInvalid, code: "entity_not_found",
		},
		{
			name:  "entity: read failure",
			setup: func(_ *testing.T, _ *Component, b *mockKVBucket) { b.getFunc = kvDown },
			call:  func(t *testing.T, c *Component) error { return entity(c, request(t, graph.EntityRequest{ID: stored})) },
			class: errs.ErrorTransient, code: "internal",
		},
		{
			name:  "batch: malformed request",
			call:  func(_ *testing.T, c *Component) error { return batch(c, []byte("{")) },
			class: errs.ErrorInvalid, code: "invalid_request",
		},
		{
			name:  "batch: read failure",
			setup: func(_ *testing.T, _ *Component, b *mockKVBucket) { b.getFunc = kvDown },
			call: func(t *testing.T, c *Component) error {
				return batch(c, request(t, graph.EntityBatchRequest{IDs: []string{stored}}))
			},
			class: errs.ErrorTransient, code: "internal",
		},
		{
			name: "batch: refused stored value",
			setup: func(_ *testing.T, _ *Component, b *mockKVBucket) {
				b.data[stored] = mockKVData{value: []byte(`{"id":`), revision: 1}
			},
			call: func(t *testing.T, c *Component) error {
				return batch(c, request(t, graph.EntityBatchRequest{IDs: []string{stored}}))
			},
			class: errs.ErrorFatal, code: "graph_state_reset_required",
		},
		{
			name: "prefix: maximum payload not observable",
			call: func(t *testing.T, c *Component) error {
				_, err := c.handleQueryPrefixNATS(context.Background(), request(t, graph.PrefixQueryRequest{Prefix: "acme"}))
				return err
			},
			class: errs.ErrorTransient, code: "internal",
		},
		{
			name:  "prefix: malformed request",
			call:  func(_ *testing.T, c *Component) error { return prefix(c, []byte("{")) },
			class: errs.ErrorInvalid, code: "invalid_request",
		},
		{
			name: "prefix: prefix and pattern both set",
			call: func(t *testing.T, c *Component) error {
				return prefix(c, request(t, graph.PrefixQueryRequest{Prefix: "acme", Pattern: "acme.*.*.*.*.*"}))
			},
			class: errs.ErrorInvalid, code: "invalid_request",
		},
		{
			name: "prefix: malformed prefix",
			call: func(t *testing.T, c *Component) error {
				return prefix(c, request(t, graph.PrefixQueryRequest{Prefix: "acme.*"}))
			},
			class: errs.ErrorInvalid, code: "entity_id_prefix_invalid",
		},
		{
			name: "prefix: malformed pattern",
			call: func(t *testing.T, c *Component) error {
				return prefix(c, request(t, graph.PrefixQueryRequest{Pattern: "acme.*"}))
			},
			class: errs.ErrorInvalid, code: "entity_id_pattern_invalid",
		},
		{
			name:  "prefix: pattern listing failure",
			setup: func(_ *testing.T, _ *Component, b *mockKVBucket) { b.listFilteredFunc = listDown },
			call: func(t *testing.T, c *Component) error {
				return prefix(c, request(t, graph.PrefixQueryRequest{Pattern: "acme.*.*.*.*.*"}))
			},
			class: errs.ErrorTransient, code: "internal",
		},
		{
			name:  "prefix: prefix listing failure",
			setup: func(_ *testing.T, _ *Component, b *mockKVBucket) { b.listFilteredFunc = listDown },
			call: func(t *testing.T, c *Component) error {
				return prefix(c, request(t, graph.PrefixQueryRequest{Prefix: "acme"}))
			},
			class: errs.ErrorTransient, code: "internal",
		},
		{
			name:  "prefix: exact-key read failure",
			setup: func(_ *testing.T, _ *Component, b *mockKVBucket) { b.getFunc = kvDown },
			call: func(t *testing.T, c *Component) error {
				return prefix(c, request(t, graph.PrefixQueryRequest{Prefix: stored}))
			},
			class: errs.ErrorTransient, code: "internal",
		},
		{
			name: "prefix: malformed cursor",
			call: func(t *testing.T, c *Component) error {
				return prefix(c, request(t, graph.PrefixQueryRequest{Prefix: "acme", Cursor: "%%%"}))
			},
			class: errs.ErrorInvalid, code: "invalid_request",
		},
		{
			name: "prefix: read failure",
			setup: func(t *testing.T, c *Component, b *mockKVBucket) {
				storePrefixEntity(t, c, stored)
				b.getFunc = kvDown
			},
			call: func(t *testing.T, c *Component) error {
				return prefix(c, request(t, graph.PrefixQueryRequest{Prefix: "acme"}))
			},
			class: errs.ErrorTransient, code: "internal",
		},
		{
			name:  "prefix: no entity fits the maximum payload",
			setup: func(t *testing.T, c *Component, _ *mockKVBucket) { storePrefixEntity(t, c, stored) },
			call: func(t *testing.T, c *Component) error {
				_, err := c.handleQueryPrefixWithMaxPayload(context.Background(),
					request(t, graph.PrefixQueryRequest{Prefix: "acme"}), 16)
				return err
			},
			class: errs.ErrorInvalid, code: "response_too_large",
		},
		{
			// The page's own encoding fails: an invariant failure, not the caller's fault and
			// not worth a retry.
			name: "prefix: page cannot be encoded",
			call: func(_ *testing.T, _ *Component) error {
				_, err := marshalFittingPrefixPage([]graph.EntityState{{
					ID: stored, MessageType: testEntityType(), UpdatedAt: time.Now(),
					Triples: []message.Triple{{Subject: stored, Predicate: "test.entity.value", Object: math.Inf(1)}},
				}}, "", 1<<20)
				return err
			},
			class: errs.ErrorFatal, code: "internal",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
			if tt.setup != nil {
				tt.setup(t, c, bucket)
			}
			err := tt.call(t, c)
			if err == nil {
				t.Fatal("the query was answered, want a refusal")
			}
			code := ""
			var classified *errs.ClassifiedError
			if errors.As(err, &classified) {
				code = classified.Code
			}
			if class := errs.Classify(err); class != tt.class || code != tt.code {
				t.Errorf("refusal = class %s, code %q; want class %s, code %q (error: %v)", class, code, tt.class, tt.code, err)
			}
		})
	}
}

// TestQueryBatchEmptyIDsRepliesAnEmptyResponse: a batch request with no IDs is answered with
// an empty graph.EntityBatchResponse, encoded like every other batch reply (design D3), not
// with a hand-written body.
func TestQueryBatchEmptyIDsRepliesAnEmptyResponse(t *testing.T) {
	c := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
	reply, err := c.handleQueryBatchNATS(context.Background(), []byte(`{"ids":[]}`))
	if err != nil {
		t.Fatalf("handleQueryBatchNATS: %v", err)
	}
	if string(reply) != `{"entities":null}` {
		t.Errorf("reply = %s, want {\"entities\":null}", reply)
	}
}
