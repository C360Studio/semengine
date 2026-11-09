package graphembedding

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// unstartedHandlers returns the query handlers of a graph-embedding built through its factory and
// never started: it has no vector cache, no embedder and no storage, so a request that got past
// the entry checks to the embedder, the bucket or the cache panics rather than returns. A refusal
// from these handlers is therefore made before anything is read or embedded (design D3, "before
// the query is embedded"), which is also why the embedder's statistics cannot have changed.
func unstartedHandlers(t *testing.T) map[string]func(context.Context, []byte) ([]byte, error) {
	t.Helper()
	handlers := mustCreate(t, `{}`).queryHandlers()
	if len(handlers) != 2 {
		t.Fatalf("graph-embedding has %d handlers, want search and similar", len(handlers))
	}
	return handlers
}

// call runs handler on body and returns its reply and error. A handler that got past the entry
// checks panics on what the unstarted component lacks; call reports that as the test's failure.
func call(t *testing.T, handler func(context.Context, []byte) ([]byte, error), body string) (reply []byte, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: got past the entry checks to a read: %v", body, r)
			reply, err = nil, nil
		}
	}()
	return handler(context.Background(), []byte(body))
}

// requireInvalidRequest requires err to be the class invalid, code invalid_request refusal, whose
// message contains each of names.
func requireInvalidRequest(t *testing.T, what string, reply []byte, err error, names ...string) {
	t.Helper()
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorInvalid ||
		classified.Code != graph.ErrorCodeInvalidRequest {
		t.Errorf("%s: reply %q err %v, want class invalid, code %s", what, reply, err, graph.ErrorCodeInvalidRequest)
		return
	}
	for _, name := range names {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("%s: refusal %q does not name %q", what, err, name)
		}
	}
}

// TestHandlersDecodeStrictly: a request holding a key its type does not define, or data after the
// JSON object, is refused invalid_request naming what was refused, on both verbs (design D1;
// graph-embedding spec, "Requests are decoded strictly"). The bodies are otherwise valid, so
// without the strict decoder each would reach the missing cache and panic.
func TestHandlersDecodeStrictly(t *testing.T) {
	handlers := unstartedHandlers(t)
	const id = "acme.ops.test.system.doc.001"
	cases := []struct {
		verb, body string
		names      []string
	}{
		{"search", `{"query":"widget","entity_id":"` + id + `"}`, []string{"entity_id"}},
		{"similar", `{"entity_id":"` + id + `","scope":["acme"]}`, []string{"scope"}},
		{"search", `{"query":"widget"}{"query":"gadget"}`, []string{"data after the request"}},
		{"similar", `{"entity_id":"` + id + `"} 7`, []string{"data after the request"}},
		{"search", `{"query":"widget"} x`, []string{"data after the request"}},
		{"search", ``, nil},
		{"similar", `not json`, nil},
	}
	for _, tc := range cases {
		reply, err := call(t, handlers[tc.verb], tc.body)
		requireInvalidRequest(t, tc.verb+" "+tc.body, reply, err, tc.names...)
	}
}

// TestMinRevisionAboveZeroRefused: a min_revision above 0 is refused class invalid,
// invalid_request, saying the embedding verbs claim no revision, before anything is read (design
// D4). Class invalid is what a retrying caller never retries (natsclient's
// RequestWithRetryClassified retries only an attempt that got no reply). That 0 is answered is
// TestSearchAnswersEnveloped's.
func TestMinRevisionAboveZeroRefused(t *testing.T) {
	handlers := unstartedHandlers(t)
	for verb, body := range map[string]string{
		"search":  `{"query":"widget","min_revision":7}`,
		"similar": `{"entity_id":"acme.ops.test.system.doc.001","min_revision":7}`,
	} {
		reply, err := call(t, handlers[verb], body)
		requireInvalidRequest(t, verb+" "+body, reply, err, "min_revision", "claim no")
	}
}

// TestRefusalsBeforeAnyRead: each request the entry checks refuse is refused invalid_request
// naming what is wrong, before the embedder, the bucket or the cache is reached (design D3). The
// limit is refused, never clamped, on both verbs; a blank query, a malformed scope prefix and a
// malformed entity ID are refused too.
func TestRefusalsBeforeAnyRead(t *testing.T) {
	handlers := unstartedHandlers(t)
	const id = "acme.ops.test.system.doc.001"
	cases := []struct {
		verb, body string
		names      []string
	}{
		{"search", `{"query":"widget","limit":101}`, []string{"limit", "100"}},
		{"search", `{"query":"widget","limit":-1}`, []string{"limit", "100"}},
		{"similar", `{"entity_id":"` + id + `","limit":101}`, []string{"limit", "100"}},
		{"similar", `{"entity_id":"` + id + `","limit":-1}`, []string{"limit", "100"}},
		{"search", `{"query":""}`, []string{"query"}},
		{"search", `{"query":" \t\n "}`, []string{"query"}},
		{"search", `{"query":"widget","scope":["acme.*"]}`, []string{"scope"}},
		{"search", `{"query":"widget","scope":["acme.ops",""," acme"]}`, []string{"scope"}},
		{"search", `{"query":"widget","scope":["acme.ops.test.system.doc.001.extra"]}`, []string{"scope"}},
		{"similar", `{}`, []string{"entity_id"}},
		{"similar", `{"entity_id":"acme.ops.test.system.doc"}`, []string{"entity_id"}},
		{"similar", `{"entity_id":"acme.ops.test.system.doc.*"}`, []string{"entity_id"}},
	}
	for _, tc := range cases {
		reply, err := call(t, handlers[tc.verb], tc.body)
		requireInvalidRequest(t, tc.verb+" "+tc.body, reply, err, tc.names...)
	}
}
