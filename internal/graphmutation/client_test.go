package graphmutation

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	semerrs "github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go"
)

const clientTestEntity = "acme.ops.test.system.widget.001"

type requesterCall struct {
	subject string
	payload []byte
}

type fakeRequester struct {
	reply []byte
	err   error
	calls []requesterCall
}

func (f *fakeRequester) RequestClassified(
	_ context.Context,
	subject string,
	payload []byte,
	_ time.Duration,
) ([]byte, error) {
	f.calls = append(f.calls, requesterCall{subject: subject, payload: append([]byte(nil), payload...)})
	return f.reply, f.err
}

func TestClientAppendUsesCanonicalSubjectAndOneRequest(t *testing.T) {
	request := graph.AppendTriplesRequest{Triples: []message.Triple{{
		Subject: clientTestEntity, Predicate: "test.value.name", Object: "one",
	}}}
	fake := &fakeRequester{reply: mustJSON(t, graph.AppendTriplesResponse{Results: []graph.AppendSubjectResult{{
		EntityID: clientTestEntity, Outcome: graph.MutationApplied, KVRevision: 12,
	}}})}
	client, err := NewClient(fake, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Append(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].subject != "graph.mutation.triple.append" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	if response.Results[0].KVRevision != 12 {
		t.Fatalf("revision = %d", response.Results[0].KVRevision)
	}
}

func TestClientDoesNotRetryAmbiguousTransportFailure(t *testing.T) {
	want := errors.New("timeout after possible delivery")
	fake := &fakeRequester{err: want}
	client, err := NewClient(fake, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Append(context.Background(), graph.AppendTriplesRequest{Triples: []message.Triple{{
		Subject: clientTestEntity, Predicate: "test.value.name", Object: "one",
	}}})
	if !errors.Is(err, want) || len(fake.calls) != 1 {
		t.Fatalf("error = %v, calls = %d", err, len(fake.calls))
	}
	var transport *TransportError
	if !errors.As(err, &transport) || transport.Outcome != TransportCommitUnknown {
		t.Fatalf("error = %#v, want commit_unknown", err)
	}
}

func TestClientClassifiesDefiniteNonDelivery(t *testing.T) {
	t.Run("no responder", func(t *testing.T) {
		fake := &fakeRequester{err: nats.ErrNoResponders}
		client, _ := NewClient(fake, time.Second)
		_, err := client.Delete(context.Background(), graph.DeleteEntityRequest{
			EntityID: clientTestEntity, ExpectedRevision: 1,
		})
		var transport *TransportError
		if !errors.As(err, &transport) || transport.Outcome != TransportUnavailable ||
			!IsDefinitelyNotCommitted(err) || len(fake.calls) != 1 {
			t.Fatalf("error = %#v, calls = %d", err, len(fake.calls))
		}
	})

	t.Run("context done before request", func(t *testing.T) {
		fake := &fakeRequester{}
		client, _ := NewClient(fake, time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := client.Delete(ctx, graph.DeleteEntityRequest{
			EntityID: clientTestEntity, ExpectedRevision: 1,
		})
		var transport *TransportError
		if !errors.As(err, &transport) || transport.Outcome != TransportDeadline ||
			!IsDefinitelyNotCommitted(err) || len(fake.calls) != 0 {
			t.Fatalf("error = %#v, calls = %d", err, len(fake.calls))
		}
	})
}

// clientWrite is one of the client's four writes, called with a well-formed request.
type clientWrite struct {
	name      string
	operation Operation
	call      func(context.Context, *Client) error
}

func clientWrites() []clientWrite {
	return []clientWrite{
		{"Create", CreateEntity, func(ctx context.Context, c *Client) error {
			_, err := c.Create(ctx, graph.CreateEntityRequest{Entity: &graph.EntityState{ID: clientTestEntity}})
			return err
		}},
		{"Reconcile", ReconcilePredicates, func(ctx context.Context, c *Client) error {
			_, err := c.Reconcile(ctx, graph.ReconcilePredicatesRequest{EntityID: clientTestEntity})
			return err
		}},
		{"Append", AppendTriples, func(ctx context.Context, c *Client) error {
			_, err := c.Append(ctx, graph.AppendTriplesRequest{Triples: []message.Triple{{
				Subject: clientTestEntity, Predicate: "test.value.name", Object: "one",
			}}})
			return err
		}},
		{"Delete", DeleteEntity, func(ctx context.Context, c *Client) error {
			_, err := c.Delete(ctx, graph.DeleteEntityRequest{EntityID: clientTestEntity, ExpectedRevision: 1})
			return err
		}},
	}
}

// TestClientReportsRefusalBeforeSendingAsUnavailable: natsclient returns ErrNotConnected only
// before its one send (natsclient/request.go, RequestWithHeaders), so the client reports it as
// unavailable, not committed, with the sentinel still reachable (design
// D1, #268). The real never-connected natsclient.Client shows the production requester returns
// the sentinel this way; it dials nothing.
func TestClientReportsRefusalBeforeSendingAsUnavailable(t *testing.T) {
	assertUnavailable := func(t *testing.T, err error, operation Operation, sentinel error) {
		t.Helper()
		var transport *TransportError
		if !errors.As(err, &transport) || transport.Operation != operation ||
			transport.Outcome != TransportUnavailable || !IsDefinitelyNotCommitted(err) || !errors.Is(err, sentinel) {
			t.Fatalf("error = %#v; want a %s unavailable transport error reaching %v", err, operation, sentinel)
		}
	}
	for _, sentinel := range []error{natsclient.ErrNotConnected} {
		for _, write := range clientWrites() {
			t.Run(sentinel.Error()+"/"+write.name, func(t *testing.T) {
				fake := &fakeRequester{err: sentinel}
				client, err := NewClient(fake, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				assertUnavailable(t, write.call(context.Background(), client), write.operation, sentinel)
				if len(fake.calls) != 1 {
					t.Fatalf("calls = %d, want 1", len(fake.calls))
				}
			})
		}
	}
	for _, write := range clientWrites() {
		t.Run("never connected natsclient/"+write.name, func(t *testing.T) {
			nc, err := natsclient.NewClient("nats://unused")
			if err != nil {
				t.Fatal(err)
			}
			client, err := NewClient(nc, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			assertUnavailable(t, write.call(context.Background(), client), write.operation, natsclient.ErrNotConnected)
		})
	}
}

// assertInvalidRequest checks that err is the client's own refusal before sending: class invalid,
// code invalid_request (design D2), so projection reads it as not committed.
func assertInvalidRequest(t *testing.T, err error) {
	t.Helper()
	var classified *semerrs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != semerrs.ErrorInvalid ||
		classified.Code != graph.ErrorCodeInvalidRequest || !semerrs.IsInvalid(err) {
		t.Fatalf("error = %#v; want class invalid, code %q", err, graph.ErrorCodeInvalidRequest)
	}
}

// Each method is an exported boundary that returns an error, so a nil context is refused there,
// before any request (developer contract, "Context ownership"), as the client's own refusal:
// invalid, code invalid_request (design D2).
func TestClientRefusesNilContext(t *testing.T) {
	var nilCtx context.Context
	for _, write := range clientWrites() {
		t.Run(write.name, func(t *testing.T) {
			fake := &fakeRequester{}
			client, err := NewClient(fake, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			assertInvalidRequest(t, write.call(nilCtx, client))
			if len(fake.calls) != 0 {
				t.Fatalf("calls = %d; want a refusal before any request", len(fake.calls))
			}
		})
	}
}

// TestClientRefusesUnencodableAndEntitylessRequests: a request the client cannot send (a nil
// client, a value encoding/json refuses, a create with no entity) is refused before sending as
// invalid with code invalid_request, and no request goes out (design D2, #268, #137 item a).
func TestClientRefusesUnencodableAndEntitylessRequests(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		var client *Client
		for _, write := range clientWrites() {
			assertInvalidRequest(t, write.call(context.Background(), client))
		}
	})
	tests := []struct {
		name string
		call func(*Client) error
	}{
		{"append with a NaN object", func(c *Client) error {
			_, err := c.Append(context.Background(), graph.AppendTriplesRequest{Triples: []message.Triple{{
				Subject: clientTestEntity, Predicate: "test.value.score", Object: math.NaN(),
			}}})
			return err
		}},
		{"create with no entity", func(c *Client) error {
			_, err := c.Create(context.Background(), graph.CreateEntityRequest{})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// A reply that would pass every check, so only a refusal before sending fails the call.
			fake := &fakeRequester{reply: mustJSON(t, graph.EntityMutationResponse{
				Outcome: graph.MutationApplied, Entity: &graph.EntityState{ID: clientTestEntity}, KVRevision: 1,
			})}
			client, err := NewClient(fake, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			assertInvalidRequest(t, test.call(client))
			if len(fake.calls) != 0 {
				t.Fatalf("calls = %d; want a refusal before any request", len(fake.calls))
			}
		})
	}
}

func TestClientPreservesDefiniteClassifiedRejection(t *testing.T) {
	rejection := semerrs.ClassifiedCode(semerrs.ErrorInvalid, graph.ErrorCodeEntityNotFound, errors.New("missing"))
	fake := &fakeRequester{err: rejection}
	client, _ := NewClient(fake, time.Second)
	_, err := client.Append(context.Background(), graph.AppendTriplesRequest{Triples: []message.Triple{{
		Subject: clientTestEntity, Predicate: "test.value.name", Object: "one",
	}}})
	var classified *semerrs.ClassifiedError
	var transport *TransportError
	if !errors.As(err, &classified) || classified.Code != graph.ErrorCodeEntityNotFound ||
		errors.As(err, &transport) || len(fake.calls) != 1 {
		t.Fatalf("error = %#v, calls = %d", err, len(fake.calls))
	}
}

func TestClientClassifiesInvalidSuccessReplyAsCommitUnknown(t *testing.T) {
	tests := []struct {
		name  string
		reply []byte
	}{
		{name: "malformed JSON", reply: []byte(`{"outcome":`)},
		{name: "invalid success shape", reply: mustJSON(t, graph.EntityMutationResponse{Outcome: graph.MutationApplied})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{reply: tt.reply}
			client, _ := NewClient(fake, time.Second)
			_, err := client.Create(context.Background(), graph.CreateEntityRequest{
				Entity: &graph.EntityState{ID: clientTestEntity},
			})
			var transport *TransportError
			if !errors.As(err, &transport) || transport.Outcome != TransportCommitUnknown || len(fake.calls) != 1 {
				t.Fatalf("error = %#v, calls = %d", err, len(fake.calls))
			}
		})
	}
}

func TestClientAppendPreservesPartialOutcomeAndFailure(t *testing.T) {
	other := "acme.ops.test.system.widget.002"
	request := graph.AppendTriplesRequest{Triples: []message.Triple{
		{Subject: clientTestEntity, Predicate: "test.value.name", Object: "one"},
		{Subject: other, Predicate: "test.value.name", Object: "two"},
	}}
	want := graph.AppendTriplesResponse{Results: []graph.AppendSubjectResult{
		{EntityID: clientTestEntity, Outcome: graph.MutationApplied, KVRevision: 9},
		{EntityID: other, Outcome: graph.MutationFailed,
			Error: &graph.MutationFailure{Class: "fatal", Code: graph.ErrorCodeGraphStateResetRequired}},
	}}
	fake := &fakeRequester{reply: mustJSON(t, want)}
	client, _ := NewClient(fake, time.Second)
	got, err := client.Append(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("response = %#v, want %#v", *got, want)
	}
}

func TestClientAppendRejectsErrorOnAppliedOutcome(t *testing.T) {
	fake := &fakeRequester{reply: mustJSON(t, graph.AppendTriplesResponse{Results: []graph.AppendSubjectResult{{
		EntityID: clientTestEntity,
		Outcome:  graph.MutationApplied,
		Error:    &graph.MutationFailure{Class: "fatal", Code: graph.ErrorCodeInternal},
	}}})}
	client, _ := NewClient(fake, time.Second)
	_, err := client.Append(context.Background(), graph.AppendTriplesRequest{Triples: []message.Triple{{Subject: clientTestEntity}}})
	if err == nil {
		t.Fatal("Append() accepted error on applied outcome")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestClientAppendRefusesMalformedResponse: an append reply is valid only when it answers the
// request subject for subject: one result for each distinct subject asked, none other and none
// twice, a failure carrying its class and code and no revision, an applied or unchanged result
// carrying a revision, a not-found result carrying none, and no outcome the protocol does not
// name. Any other reply follows a request that reached graph-ingest, so the client returns
// commit_unknown after its one request and never hands the reply on as a success.
func TestClientAppendRefusesMalformedResponse(t *testing.T) {
	other := "acme.ops.test.system.widget.002"
	stranger := "acme.ops.test.system.widget.003"
	// Two statements on one subject and one on another: two distinct subjects.
	request := graph.AppendTriplesRequest{Triples: []message.Triple{
		{Subject: clientTestEntity, Predicate: "test.value.name", Object: "one"},
		{Subject: clientTestEntity, Predicate: "test.value.name", Object: "two"},
		{Subject: other, Predicate: "test.value.name", Object: "three"},
	}}
	applied := graph.AppendSubjectResult{EntityID: clientTestEntity, Outcome: graph.MutationApplied, KVRevision: 3}
	failure := &graph.MutationFailure{Class: "fatal", Code: graph.ErrorCodeGraphStateResetRequired}
	tests := []struct {
		name    string
		results []graph.AppendSubjectResult
		valid   bool
	}{
		{name: "one result per subject", valid: true, results: []graph.AppendSubjectResult{
			applied, {EntityID: other, Outcome: graph.MutationEntityNotFound},
		}},
		{name: "unchanged and failed", valid: true, results: []graph.AppendSubjectResult{
			{EntityID: clientTestEntity, Outcome: graph.MutationUnchanged, KVRevision: 3},
			{EntityID: other, Outcome: graph.MutationFailed, Error: failure},
		}},
		{name: "no results"},
		{name: "fewer results than subjects", results: []graph.AppendSubjectResult{applied}},
		{name: "one result per statement", results: []graph.AppendSubjectResult{
			applied, applied, {EntityID: other, Outcome: graph.MutationApplied, KVRevision: 4},
		}},
		{name: "a subject not asked", results: []graph.AppendSubjectResult{
			applied, {EntityID: stranger, Outcome: graph.MutationApplied, KVRevision: 4},
		}},
		{name: "a subject twice", results: []graph.AppendSubjectResult{
			applied, {EntityID: clientTestEntity, Outcome: graph.MutationApplied, KVRevision: 4},
		}},
		{name: "failure without an error", results: []graph.AppendSubjectResult{
			applied, {EntityID: other, Outcome: graph.MutationFailed},
		}},
		{name: "failure without a class", results: []graph.AppendSubjectResult{
			applied, {EntityID: other, Outcome: graph.MutationFailed, Error: &graph.MutationFailure{Code: failure.Code}},
		}},
		{name: "failure without a code", results: []graph.AppendSubjectResult{
			applied, {EntityID: other, Outcome: graph.MutationFailed, Error: &graph.MutationFailure{Class: "fatal"}},
		}},
		{name: "failure with a revision", results: []graph.AppendSubjectResult{
			applied, {EntityID: other, Outcome: graph.MutationFailed, Error: failure, KVRevision: 4},
		}},
		{name: "applied without a revision", results: []graph.AppendSubjectResult{
			applied, {EntityID: other, Outcome: graph.MutationApplied},
		}},
		{name: "unchanged without a revision", results: []graph.AppendSubjectResult{
			applied, {EntityID: other, Outcome: graph.MutationUnchanged},
		}},
		{name: "not found with a revision", results: []graph.AppendSubjectResult{
			applied, {EntityID: other, Outcome: graph.MutationEntityNotFound, KVRevision: 4},
		}},
		{name: "an outcome append does not report", results: []graph.AppendSubjectResult{
			applied, {EntityID: other, Outcome: graph.MutationOutcome("revision_mismatch"), KVRevision: 4},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reply := graph.AppendTriplesResponse{Results: test.results}
			fake := &fakeRequester{reply: mustJSON(t, reply)}
			client, err := NewClient(fake, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.Append(context.Background(), request)
			if len(fake.calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(fake.calls))
			}
			if test.valid {
				if err != nil || got == nil || !reflect.DeepEqual(*got, reply) {
					t.Fatalf("response = %#v, error = %v; want the reply %#v", got, err, reply)
				}
				return
			}
			assertCommitUnknown(t, got == nil, err, AppendTriples)
		})
	}
}

// TestClientReconcileAndDeleteSendOneRequestAndReadTheReply: Reconcile and Delete each send the
// request they are given, unchanged, as one request on their own subject, and return the reply
// when it answers that request: for a reconcile, applied or unchanged for the same entity at a
// revision; for a delete, applied for the same entity at the same fence. A reply that answers
// something else follows a request that reached graph-ingest, so it is commit_unknown, never a
// success.
func TestClientReconcileAndDeleteSendOneRequestAndReadTheReply(t *testing.T) {
	entity := &graph.EntityState{ID: clientTestEntity}
	otherEntity := &graph.EntityState{ID: "acme.ops.test.system.widget.002"}
	t.Run("reconcile", func(t *testing.T) {
		request := graph.ReconcilePredicatesRequest{
			EntityID: clientTestEntity, ExpectedRevision: 7, Source: "test-source",
			Predicates: []string{"test.value.name"},
			Desired:    []message.Triple{{Subject: clientTestEntity, Predicate: "test.value.name", Object: "one"}},
			TraceID:    "trace-1", RequestID: "request-1",
		}
		tests := []struct {
			name  string
			reply graph.EntityMutationResponse
			valid bool
		}{
			{name: "applied", valid: true,
				reply: graph.EntityMutationResponse{Outcome: graph.MutationApplied, Entity: entity, KVRevision: 8}},
			{name: "unchanged", valid: true,
				reply: graph.EntityMutationResponse{Outcome: graph.MutationUnchanged, Entity: entity, KVRevision: 7}},
			{name: "another outcome",
				reply: graph.EntityMutationResponse{Outcome: graph.MutationOutcome("revision_mismatch"), Entity: entity, KVRevision: 8}},
			{name: "no entity",
				reply: graph.EntityMutationResponse{Outcome: graph.MutationApplied, KVRevision: 8}},
			{name: "another entity",
				reply: graph.EntityMutationResponse{Outcome: graph.MutationApplied, Entity: otherEntity, KVRevision: 8}},
			{name: "no revision",
				reply: graph.EntityMutationResponse{Outcome: graph.MutationApplied, Entity: entity}},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				fake := &fakeRequester{reply: mustJSON(t, test.reply)}
				client, err := NewClient(fake, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				got, err := client.Reconcile(context.Background(), request)
				var sent graph.ReconcilePredicatesRequest
				assertOneRequest(t, fake, "graph.mutation.entity.reconcile", &sent)
				if !reflect.DeepEqual(sent, request) {
					t.Fatalf("sent %#v, want the request %#v", sent, request)
				}
				if test.valid {
					if err != nil || got == nil || !reflect.DeepEqual(*got, test.reply) {
						t.Fatalf("response = %#v, error = %v; want the reply %#v", got, err, test.reply)
					}
					return
				}
				assertCommitUnknown(t, got == nil, err, ReconcilePredicates)
			})
		}
	})
	t.Run("delete", func(t *testing.T) {
		request := graph.DeleteEntityRequest{
			EntityID: clientTestEntity, ExpectedRevision: 7, TraceID: "trace-2", RequestID: "request-2",
		}
		tests := []struct {
			name  string
			reply graph.DeleteEntityResponse
			valid bool
		}{
			{name: "applied", valid: true,
				reply: graph.DeleteEntityResponse{EntityID: clientTestEntity, Outcome: graph.MutationApplied, ExpectedRevision: 7}},
			{name: "another outcome",
				reply: graph.DeleteEntityResponse{EntityID: clientTestEntity, Outcome: graph.MutationUnchanged, ExpectedRevision: 7}},
			{name: "another entity",
				reply: graph.DeleteEntityResponse{EntityID: otherEntity.ID, Outcome: graph.MutationApplied, ExpectedRevision: 7}},
			{name: "another fence",
				reply: graph.DeleteEntityResponse{EntityID: clientTestEntity, Outcome: graph.MutationApplied, ExpectedRevision: 8}},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				fake := &fakeRequester{reply: mustJSON(t, test.reply)}
				client, err := NewClient(fake, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				got, err := client.Delete(context.Background(), request)
				var sent graph.DeleteEntityRequest
				assertOneRequest(t, fake, "graph.mutation.entity.delete", &sent)
				if sent != request {
					t.Fatalf("sent %#v, want the request %#v", sent, request)
				}
				if test.valid {
					if err != nil || got == nil || *got != test.reply {
						t.Fatalf("response = %#v, error = %v; want the reply %#v", got, err, test.reply)
					}
					return
				}
				assertCommitUnknown(t, got == nil, err, DeleteEntity)
			})
		}
	})
}

// assertOneRequest checks that exactly one request was sent, on subject, and decodes it into sent.
func assertOneRequest(t *testing.T, fake *fakeRequester, subject string, sent any) {
	t.Helper()
	if len(fake.calls) != 1 || fake.calls[0].subject != subject {
		t.Fatalf("calls = %#v, want one on %s", fake.calls, subject)
	}
	if err := json.Unmarshal(fake.calls[0].payload, sent); err != nil {
		t.Fatal(err)
	}
}

// assertCommitUnknown checks that a call returned no response and a commit_unknown transport error
// for operation.
func assertCommitUnknown(t *testing.T, noResponse bool, err error, operation Operation) {
	t.Helper()
	var transport *TransportError
	if !noResponse || !errors.As(err, &transport) || transport.Operation != operation ||
		transport.Outcome != TransportCommitUnknown {
		t.Fatalf("response returned = %t, error = %#v; want no response and a %s commit_unknown",
			!noResponse, err, operation)
	}
}
