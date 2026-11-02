//go:build integration

package graphquery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// requestTimeout outlasts every wait these tests expect; a caller hitting it fails loudly.
const requestTimeout = time.Minute

// forward is one row of design D1's table: a verb graph-query serves and the row that answers it.
type forward struct{ verb, downstream graph.QueryVerb }

// forwardRows reads design D1's table, written out here as the test's own statement of it, and finds
// each row in graph.QueryVerbs. The order is the table's: a row of another responder shares a name.
func forwardRows(t *testing.T) []forward {
	t.Helper()
	var rows []forward
	for _, want := range []struct{ name, answeredBy string }{
		{"entity", graph.ResponderGraphIngest},
		{"batch", graph.ResponderGraphIngest},
		{"prefix", graph.ResponderGraphIngest},
		{"byName", graph.ResponderGraphIndex},
	} {
		rows = append(rows, forward{verbRow(t, graph.ResponderGraphQuery, want.name), verbRow(t, want.answeredBy, want.name)})
	}
	return rows
}

func verbRow(t *testing.T, responder, name string) graph.QueryVerb {
	t.Helper()
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == responder && verb.Name == name {
			return verb
		}
	}
	t.Fatalf("the verb table declares no %s for %s", name, responder)
	return graph.QueryVerb{}
}

// serve stands in for a downstream verb. Its subscription ends with the client's Close.
func serve(t *testing.T, client *natsclient.Client, subject string, handler queryHandler) {
	t.Helper()
	if _, err := client.SubscribeForRequests(t.Context(), subject, handler); err != nil {
		t.Fatalf("subscribe %s: %v", subject, err)
	}
}

// startComponent starts graph-query and stops it when the test ends. A held stub's release, if the
// test registers it after this call, runs before the Stop.
func startComponent(t *testing.T, client *natsclient.Client, config string) *Component {
	t.Helper()
	c := newConfigured(t, client, config)
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := c.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	return c
}

// refusalOf returns err's classified refusal, failing the test if it has none.
func refusalOf(t *testing.T, err error) *errs.ClassifiedError {
	t.Helper()
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v (%T), want a classified refusal", err, err)
	}
	return refusal
}

// TestForwardPassesBytesBothWays: each forward hands its downstream verb the request's bytes and its
// caller the reply's, exactly: an unknown field, odd spacing, and bytes that are not JSON at all, in
// each direction. Each downstream verb answers with its own bytes, so a forward wired to the wrong
// row fails as well.
func TestForwardPassesBytesBothWays(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)

	received := make(map[string]chan []byte)
	for _, f := range forwardRows(t) {
		got := make(chan []byte, 1)
		received[f.verb.Name] = got
		serve(t, client, f.downstream.Subject, func(_ context.Context, data []byte) ([]byte, error) {
			got <- data
			return replyBytes(f.downstream.Subject, data), nil
		})
	}

	for _, request := range [][]byte{
		[]byte("{ \"unknown_field\" :  [1, 2.0e0],\t\"id\":\"caf\\u00e9\"  ,\"min_revision\": 007 }\n"),
		[]byte("not json \xff\x00 at all"),
	} {
		for _, f := range forwardRows(t) {
			reply, err := client.RequestClassified(t.Context(), f.verb.Subject, request, requestTimeout)
			if err != nil {
				t.Fatalf("%s: %v", f.verb.Name, err)
			}
			// The stub sends before it returns, so before the reply above; a forward that never
			// reached it has sent nothing, and that is a failure rather than a wait.
			select {
			case got := <-received[f.verb.Name]:
				if !bytes.Equal(got, request) {
					t.Errorf("%s: %s received %q, want %q", f.verb.Name, f.downstream.Subject, got, request)
				}
			default:
				t.Errorf("%s: %s received nothing", f.verb.Name, f.downstream.Subject)
			}
			if want := replyBytes(f.downstream.Subject, request); !bytes.Equal(reply, want) {
				t.Errorf("%s: caller received %q, want %q", f.verb.Name, reply, want)
			}
		}
	}
}

// replyBytes is a stub's reply: not canonical, carrying an unknown field, and different per subject
// and per request.
func replyBytes(subject string, request []byte) []byte {
	return fmt.Appendf(nil, "{\"from\" : %q,  \"unknown_field\":\t[ 1.50 ], \"len\": %d }\n", subject, len(request))
}

// TestForwardsShareNothingBetweenRequests: bytes a forward received or returned stay as they were
// after later requests. Requests run one after another on each verb, longest first, so a reused
// buffer would overwrite an earlier one, and at once across the verbs. The stubs and callers keep
// the slices they were given, uncopied, and every one is compared only when all have finished.
func TestForwardsShareNothingBetweenRequests(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)
	rows := forwardRows(t)
	const rounds = 20

	kept := make(map[string][][]byte)
	var keptMu sync.Mutex
	for _, f := range rows {
		serve(t, client, f.downstream.Subject, func(_ context.Context, data []byte) ([]byte, error) {
			keptMu.Lock()
			defer keptMu.Unlock()
			kept[f.verb.Name] = append(kept[f.verb.Name], data)
			return replyBytes(f.downstream.Subject, data), nil
		})
	}
	requestFor := func(name string, round int) []byte {
		return []byte(fmt.Sprintf("%s-%d-", name, round) + strings.Repeat("x", (rounds-round)*64))
	}

	replies := make([][][]byte, len(rows))
	var callers sync.WaitGroup
	for i, f := range rows {
		replies[i] = make([][]byte, rounds)
		callers.Go(func() {
			for round := range rounds {
				reply, err := client.RequestClassified(t.Context(), f.verb.Subject, requestFor(f.verb.Name, round), requestTimeout)
				if err != nil {
					t.Errorf("%s round %d: %v", f.verb.Name, round, err)
					return
				}
				replies[i][round] = reply
			}
		})
	}
	callers.Wait() // orders the reads below after every stub's and caller's writes
	if t.Failed() {
		t.FailNow()
	}

	for i, f := range rows {
		if got := len(kept[f.verb.Name]); got != rounds {
			t.Fatalf("%s: stub received %d requests, want %d", f.verb.Name, got, rounds)
		}
		for round := range rounds {
			request := requestFor(f.verb.Name, round)
			if got := kept[f.verb.Name][round]; !bytes.Equal(got, request) {
				t.Errorf("%s round %d: the request the stub kept became %q, want %q", f.verb.Name, round, got, request)
			}
			if got, want := replies[i][round], replyBytes(f.downstream.Subject, request); !bytes.Equal(got, want) {
				t.Errorf("%s round %d: the reply the caller kept became %q, want %q", f.verb.Name, round, got, want)
			}
		}
	}
}

// TestForwardCarriesTheRequestContext: a forward's downstream request runs under the handler's
// context. The caller's trace reaches the downstream verb; a context of the forward's own would
// start a new trace.
func TestForwardCarriesTheRequestContext(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)

	f := forwardRows(t)[0]
	seen := make(chan string, 1)
	serve(t, client, f.downstream.Subject, func(ctx context.Context, _ []byte) ([]byte, error) {
		trace, ok := natsclient.TraceContextFromContext(ctx)
		if !ok {
			seen <- ""
			return nil, nil
		}
		seen <- trace.TraceID
		return nil, nil
	})
	sent := natsclient.NewTraceContext()
	ctx := natsclient.ContextWithTrace(t.Context(), sent)
	if _, err := client.RequestClassified(ctx, f.verb.Subject, []byte(`{}`), requestTimeout); err != nil {
		t.Fatalf("request: %v", err)
	}
	// The stub sends before it returns, so before the reply above; a forward that never reached it
	// has sent nothing, and that is a failure rather than a wait.
	select {
	case got := <-seen:
		if got != sent.TraceID {
			t.Errorf("the downstream verb saw trace %q, the caller sent %q", got, sent.TraceID)
		}
	default:
		t.Errorf("%s received nothing", f.downstream.Subject)
	}
}

// TestForwardedRefusalKeepsClassCodeDetail: each forward returns a downstream refusal with its class,
// code, detail and message as the downstream verb gave them, whatever the class, coded or not.
func TestForwardedRefusalKeepsClassCodeDetail(t *testing.T) {
	cases := []struct {
		name    string
		refusal error
		class   errs.ErrorClass
		code    string
		detail  map[string]any // as it decodes from the wire
		message string
	}{
		{
			name: "invalid with detail",
			refusal: errs.ClassifiedCodeDetail(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound,
				map[string]any{"entity_id": "a.b.c.d.e.f"}, errors.New("no such entity")),
			class: errs.ErrorInvalid, code: graph.ErrorCodeEntityNotFound,
			detail: map[string]any{"entity_id": "a.b.c.d.e.f"}, message: "no such entity",
		},
		{
			name: "transient with detail",
			refusal: errs.ClassifiedCodeDetail(errs.ErrorTransient, graph.ErrorCodeRevisionNotReached,
				map[string]any{"indexed_revision": 7}, errors.New("not yet")),
			class: errs.ErrorTransient, code: graph.ErrorCodeRevisionNotReached,
			detail: map[string]any{"indexed_revision": float64(7)}, message: "not yet",
		},
		{
			name:    "fatal with a code",
			refusal: errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeInternal, errors.New("broken")),
			class:   errs.ErrorFatal, code: graph.ErrorCodeInternal, message: "broken",
		},
		{
			name:    "no code",
			refusal: errs.Classified(errs.ErrorInvalid, errors.New("refused without a code")),
			class:   errs.ErrorInvalid, message: "refused without a code",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fixtureClient(t)
			startComponent(t, client, `{}`)
			for _, f := range forwardRows(t) {
				serve(t, client, f.downstream.Subject, func(context.Context, []byte) ([]byte, error) {
					return nil, tc.refusal
				})
				reply, err := client.RequestClassified(t.Context(), f.verb.Subject, []byte(`{}`), requestTimeout)
				if err == nil {
					t.Fatalf("%s: refusal arrived as the success %q", f.verb.Name, reply)
				}
				got := refusalOf(t, err)
				if got.Class != tc.class || got.Code != tc.code {
					t.Errorf("%s: class, code = %v, %q, want %v, %q", f.verb.Name, got.Class, got.Code, tc.class, tc.code)
				}
				if len(got.Detail) != len(tc.detail) || (len(tc.detail) > 0 && !reflect.DeepEqual(got.Detail, tc.detail)) {
					t.Errorf("%s: detail = %v, want %v", f.verb.Name, got.Detail, tc.detail)
				}
				if err.Error() != tc.message {
					t.Errorf("%s: message = %q, want %q", f.verb.Name, err.Error(), tc.message)
				}
			}
		})
	}
}

// TestNoRespondersIsTransient: with no downstream responder the caller receives graph-query's own
// transient internal refusal, naming the subject. The class is graph-query's statement because the
// transport error's is not promised (ADR-112).
func TestNoRespondersIsTransient(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)
	for _, f := range forwardRows(t) {
		reply, err := client.RequestClassified(t.Context(), f.verb.Subject, []byte(`{}`), requestTimeout)
		if err == nil {
			t.Fatalf("%s with no downstream responder succeeded: %q", f.verb.Name, reply)
		}
		if natsclient.IsNoResponders(err) {
			t.Fatalf("%s: graph-query itself did not answer: %v", f.verb.Name, err)
		}
		got := refusalOf(t, err)
		if got.Class != errs.ErrorTransient || got.Code != graph.ErrorCodeInternal {
			t.Errorf("%s: class, code = %v, %q, want transient, %q", f.verb.Name, got.Class, got.Code, graph.ErrorCodeInternal)
		}
		if !strings.Contains(err.Error(), f.downstream.Subject) {
			t.Errorf("%s: refusal %q does not name %s", f.verb.Name, err, f.downstream.Subject)
		}
	}
}

// TestQueryTimeoutIsRead: a downstream verb that never replies ends at query_timeout_ms. The caller's
// refusal is transient and names the subject, and comes from graph-query: the caller's own, longer
// timeout would surface as a transport error, with no class on it. It takes at least the configured
// time, so a shorter bound written in the code fails as well as a longer one.
func TestQueryTimeoutIsRead(t *testing.T) {
	const configured = 300 * time.Millisecond
	client := fixtureClient(t)
	startComponent(t, client, fmt.Sprintf(`{"query_timeout_ms": %d}`, configured.Milliseconds()))

	release := make(chan struct{})
	t.Cleanup(func() { close(release) }) // before the Stop and the fixture's Close
	for _, f := range forwardRows(t) {
		serve(t, client, f.downstream.Subject, func(context.Context, []byte) ([]byte, error) {
			<-release
			return nil, nil
		})
	}
	// The caller's timeout is itself the behaviour under test: below the 15 s default, so a forward
	// that did not read the configuration fails here rather than answering in time.
	const callerTimeout = 10 * time.Second
	for _, f := range forwardRows(t) {
		began := time.Now()
		reply, err := client.RequestClassified(t.Context(), f.verb.Subject, []byte(`{}`), callerTimeout)
		elapsed := time.Since(began)
		if err == nil {
			t.Fatalf("%s: a downstream verb that never replies gave the success %q", f.verb.Name, reply)
		}
		got := refusalOf(t, err)
		if got.Class != errs.ErrorTransient {
			t.Errorf("%s: class = %v, want transient", f.verb.Name, got.Class)
		}
		if !strings.Contains(err.Error(), f.downstream.Subject) {
			t.Errorf("%s: refusal %q does not name %s", f.verb.Name, err, f.downstream.Subject)
		}
		if elapsed < configured {
			t.Errorf("%s: refused after %v, before query_timeout_ms %v", f.verb.Name, elapsed, configured)
		}
	}
}

// TestGraphQueryServesExactlyTheDeclaredVerbs: from another connection, as soon as Start returns,
// every declared verb is answered by graph-query, every other row of the table has no responder
// while its owner is absent, and a verb a consumer serves under graph.query.* is left to that
// consumer: it has no responder until the consumer subscribes (graph-query, "A consumer verb beside
// the component"; graph-transport-boundary, "Routable when Start returns").
func TestGraphQueryServesExactlyTheDeclaredVerbs(t *testing.T) {
	url := shared.Reset(t)
	client := natsfixture.Open(t, url, openClient)
	other := natsfixture.Open(t, url, openClient) // a connection graph-query does not use
	c := startComponent(t, client, `{}`)

	rows := forwardRows(t)
	var declared []string
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphQuery {
			declared = append(declared, verb.Name)
		}
	}
	var want []string
	for _, f := range rows {
		want = append(want, f.verb.Name)
	}
	if !reflect.DeepEqual(declared, want) {
		t.Fatalf("the verb table declares %v for graph-query, want %v", declared, want)
	}
	if got := len(c.subscriptions); got != len(want) {
		t.Errorf("graph-query holds %d request subscriptions, want %d", got, len(want))
	}

	// With nothing downstream, only graph-query can answer, and it refuses naming the downstream row.
	for _, f := range rows {
		_, err := other.RequestClassified(t.Context(), f.verb.Subject, []byte(`{}`), requestTimeout)
		if err == nil || natsclient.IsNoResponders(err) {
			t.Errorf("%s = %v, want graph-query's refusal", f.verb.Subject, err)
			continue
		}
		if !strings.Contains(err.Error(), f.downstream.Subject) {
			t.Errorf("%s answered %q, which does not name %s", f.verb.Subject, err, f.downstream.Subject)
		}
	}
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphQuery {
			continue
		}
		if reply, err := other.RequestClassified(t.Context(), verb.Subject, []byte(`{}`), requestTimeout); !errors.Is(err, nats.ErrNoResponders) {
			t.Errorf("%s of %s = %q, %v, want no responders", verb.Subject, verb.Responder, reply, err)
		}
	}

	// Not in the table; graph-query's port is graph.query.*, so a wildcard subscription would answer.
	const consumerVerb = "graph.query.sources"
	if reply, err := other.RequestClassified(t.Context(), consumerVerb, []byte(`{}`), requestTimeout); !errors.Is(err, nats.ErrNoResponders) {
		t.Fatalf("%s before its consumer subscribed = %q, %v, want no responders", consumerVerb, reply, err)
	}
	serve(t, client, consumerVerb, func(context.Context, []byte) ([]byte, error) { return []byte("the consumer"), nil })
	reply, err := other.RequestClassified(t.Context(), consumerVerb, []byte(`{}`), requestTimeout)
	if err != nil {
		t.Fatalf("%s: %v", consumerVerb, err)
	}
	if string(reply) != "the consumer" {
		t.Errorf("%s answered %q, want the consumer's reply", consumerVerb, reply)
	}
}
