package httpembedder_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semengine/graph/embedding/httpembedder"
)

const testModel = "Snowflake/snowflake-arctic-embed-s"

func newEmbedder(t *testing.T, cfg httpembedder.Config) *httpembedder.Embedder {
	t.Helper()
	e, err := httpembedder.New(cfg)
	if err != nil {
		t.Fatalf("New(%+v): %v", cfg, err)
	}
	return e
}

func TestRequestShape(t *testing.T) {
	p := newProvider(t, semembed(2))
	e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel})

	got, err := e.Generate(t.Context(), []string{"first text", "second text"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := [][]float32{{1, 1}, {2, 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("vectors = %v, want %v", got, want)
	}

	reqs := p.Requests()
	if len(reqs) != 1 {
		t.Fatalf("provider saw %d requests, want 1", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/v1/embeddings" {
		t.Fatalf("request = %s %s, want POST /v1/embeddings", r.Method, r.Path)
	}
	if r.HasAuth {
		t.Fatalf("Authorization = %q sent with no API key configured", r.Authorization)
	}
	wantBody := map[string]any{"model": testModel, "input": []any{"first text", "second text"}, "input_type": "document"}
	if !reflect.DeepEqual(r.Body, wantBody) {
		t.Fatalf("request body = %v, want exactly %v", r.Body, wantBody)
	}

	keyed := newProvider(t, semembed(2))
	e = newEmbedder(t, httpembedder.Config{BaseURL: keyed.BaseURL(), Model: testModel, APIKey: "secret"})
	if _, err := e.Generate(t.Context(), []string{"text"}); err != nil {
		t.Fatalf("Generate with API key: %v", err)
	}
	if reqs := keyed.Requests(); len(reqs) != 1 || reqs[0].Authorization != "Bearer secret" {
		t.Fatalf("requests with API key = %+v, want one carrying Authorization: Bearer secret", reqs)
	}
}

// TestInputTypeNamesTheText: each request says whether its texts are queries or documents, so a
// provider such as semembed can put the model's own instruction in front; with OmitInputType the
// field is not sent at all, for a provider that refuses it.
func TestInputTypeNamesTheText(t *testing.T) {
	for _, omit := range []bool{false, true} {
		p := newProvider(t, semembed(2))
		e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel, OmitInputType: omit})
		if _, err := e.GenerateQuery(t.Context(), []string{"retry with backoff"}); err != nil {
			t.Fatalf("GenerateQuery: %v", err)
		}
		if _, err := e.Generate(t.Context(), []string{"retry with backoff"}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		reqs := p.Requests()
		if len(reqs) != 2 {
			t.Fatalf("provider saw %d requests, want 2", len(reqs))
		}
		for i, want := range []string{"query", "document"} {
			got, sent := reqs[i].Body["input_type"]
			switch {
			case omit && sent:
				t.Errorf("OmitInputType: request %d sent input_type %v, want no field", i, got)
			case !omit && got != want:
				t.Errorf("request %d input_type = %v (sent %t), want %q", i, got, sent, want)
			}
		}
	}
}

func TestVectorsPlacedByIndex(t *testing.T) {
	// The reply lists the vectors out of input order. It also names no model, which is accepted:
	// only a reply naming another model is refused.
	p := newProvider(t, fixed(http.StatusOK, embedReply{Object: "list", Data: []embedObject{
		{Object: "embedding", Embedding: []float32{3, 3}, Index: 2},
		{Object: "embedding", Embedding: []float32{1, 1}, Index: 0},
		{Object: "embedding", Embedding: []float32{2, 2}, Index: 1},
	}}))
	e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel})

	got, err := e.Generate(t.Context(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := [][]float32{{1, 1}, {2, 2}, {3, 3}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("vectors = %v, want %v: each vector belongs at its index, not at its place in the reply",
			got, want)
	}
}

func TestEmptyInputSendsNothing(t *testing.T) {
	p := newProvider(t, semembed(2))
	e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel, QueryPrefix: "query: "})

	for name, call := range map[string]func() ([][]float32, error){
		"Generate nil":        func() ([][]float32, error) { return e.Generate(t.Context(), nil) },
		"Generate empty":      func() ([][]float32, error) { return e.Generate(t.Context(), []string{}) },
		"GenerateQuery empty": func() ([][]float32, error) { return e.GenerateQuery(t.Context(), []string{}) },
	} {
		got, err := call()
		if err != nil || len(got) != 0 {
			t.Fatalf("%s = %v, %v; want an empty result and no error", name, got, err)
		}
	}
	if reqs := p.Requests(); len(reqs) != 0 {
		t.Fatalf("provider saw %d requests for no texts, want none", len(reqs))
	}
	if d := e.Dimensions(); d != 0 {
		t.Fatalf("Dimensions() = %d after no reply, want 0", d)
	}
}

func TestHTTPEmbedder_GenerateQuery_PrependsPrefix(t *testing.T) {
	p := newProvider(t, semembed(3))
	const prefix = "Represent this sentence for searching relevant passages: "
	e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel, QueryPrefix: prefix})

	if _, err := e.GenerateQuery(t.Context(), []string{"retry with backoff", "jitter"}); err != nil {
		t.Fatalf("GenerateQuery: %v", err)
	}
	if _, err := e.Generate(t.Context(), []string{"retry with backoff"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	reqs := p.Requests()
	if len(reqs) != 2 {
		t.Fatalf("provider saw %d requests, want 2", len(reqs))
	}
	if got, want := reqs[0].Decoded.Input, []string{prefix + "retry with backoff", prefix + "jitter"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("query input = %q, want every text prefixed: %q", got, want)
	}
	// Documents are embedded raw: an asymmetric model is trained that way.
	if got, want := reqs[1].Decoded.Input, []string{"retry with backoff"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("document input = %q, want unprefixed %q", got, want)
	}
}

func TestHTTPEmbedder_GenerateQuery_NoPrefixConfigured(t *testing.T) {
	p := newProvider(t, semembed(3))
	e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel})

	if _, err := e.GenerateQuery(t.Context(), []string{"hello"}); err != nil {
		t.Fatalf("GenerateQuery: %v", err)
	}
	if _, err := e.Generate(t.Context(), []string{"hello"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	reqs := p.Requests()
	// The two differ only in input_type (TestInputTypeNamesTheText): with no prefix a query sends the
	// same model and texts as a document.
	if len(reqs) != 2 || !reflect.DeepEqual(reqs[0].Decoded, reqs[1].Decoded) {
		t.Fatalf("requests = %+v, want two with the same model and input", reqs)
	}
	if got := reqs[0].Decoded.Input; !reflect.DeepEqual(got, []string{"hello"}) {
		t.Fatalf("query input = %q, want unprefixed", got)
	}
}

func TestRefusesUntrustedReply(t *testing.T) {
	vec := func(index int, v ...float32) embedObject {
		return embedObject{Object: "embedding", Embedding: v, Index: index}
	}
	reply := func(model string, data ...embedObject) embedReply {
		return embedReply{Object: "list", Data: data, Model: model}
	}
	cases := []struct {
		name string
		// learn, when set, is a first reply the embedder accepts before the refused one.
		learn func(req embedRequest) (int, any)
		reply func(req embedRequest) (int, any)
		// wantErr lists text the error must carry, in lower case.
		wantErr []string
	}{
		{name: "status_400", reply: fixed(http.StatusBadRequest,
			errorReply{errorDetail{"Model rejected the input", "invalid_request_error"}}),
			wantErr: []string{"400", "model rejected the input"}},
		{name: "status_500", reply: fixed(http.StatusInternalServerError,
			errorReply{errorDetail{"Failed to generate embeddings: x", "internal_error"}}),
			wantErr: []string{"500", "failed to generate embeddings: x"}},
		{name: "status_503_no_body", reply: fixed(http.StatusServiceUnavailable, ""), wantErr: []string{"503"}},
		{name: "too_few", reply: fixed(http.StatusOK, reply(testModel, vec(0, 1, 1)))},
		{name: "too_many", reply: fixed(http.StatusOK, reply(testModel, vec(0, 1, 1), vec(1, 2, 2), vec(2, 3, 3)))},
		{name: "index_out_of_range", reply: fixed(http.StatusOK, reply(testModel, vec(0, 1, 1), vec(2, 2, 2)))},
		{name: "negative_index", reply: fixed(http.StatusOK, reply(testModel, vec(-1, 1, 1), vec(1, 2, 2)))},
		{name: "repeated_index", reply: fixed(http.StatusOK, reply(testModel, vec(0, 1, 1), vec(0, 2, 2)))},
		{name: "empty_vector", reply: fixed(http.StatusOK, reply(testModel, vec(0), vec(1, 2, 2)))},
		{name: "mixed_width", reply: fixed(http.StatusOK, reply(testModel, vec(0, 1, 1), vec(1, 2, 2, 2))),
			wantErr: []string{"dimension"}},
		{name: "width_other_than_learned", learn: semembed(2), reply: semembed(3), wantErr: []string{"dimension"}},
		{name: "other_model", reply: fixed(http.StatusOK, reply("served-other", vec(0, 1, 1), vec(1, 2, 2))),
			wantErr: []string{"served-other"}},
		{name: "not_json", reply: fixed(http.StatusOK, "not json")},
		{name: "null_coordinate", reply: fixed(http.StatusOK,
			`{"data":[{"index":0,"embedding":[null,1]},{"index":1,"embedding":[2,2]}]}`)},
		{name: "absent_index", reply: fixed(http.StatusOK,
			`{"data":[{"embedding":[1,1]},{"index":1,"embedding":[2,2]}]}`)},
		{name: "null_index", reply: fixed(http.StatusOK,
			`{"data":[{"index":null,"embedding":[1,1]},{"index":1,"embedding":[2,2]}]}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			current := tc.learn
			if current == nil {
				current = tc.reply
			}
			p := newProvider(t, func(req embedRequest) (int, any) {
				mu.Lock()
				defer mu.Unlock()
				return current(req)
			})
			e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel})
			wantDims := 0
			if tc.learn != nil {
				if _, err := e.Generate(t.Context(), []string{"a", "b"}); err != nil {
					t.Fatalf("%s: first reply refused: %v", tc.name, err)
				}
				wantDims = e.Dimensions()
				mu.Lock()
				current = tc.reply
				mu.Unlock()
			}

			got, err := e.Generate(t.Context(), []string{"a", "b"})
			if err == nil || got != nil {
				t.Fatalf("%s: Generate = %v, %v; want an error and no vectors", tc.name, got, err)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(strings.ToLower(err.Error()), want) {
					t.Fatalf("%s: error %q does not name %q", tc.name, err, want)
				}
			}
			if d := e.Dimensions(); d != wantDims {
				t.Fatalf("%s: Dimensions() = %d after a refused reply, want %d", tc.name, d, wantDims)
			}
		})
	}
}

func TestContextEndsRequest(t *testing.T) {
	arrived := make(chan struct{})
	var once sync.Once
	// The handler waits on its own request's context, so only the caller's cancel can end it. It
	// reads the body first: without that, net/http's server here never saw the client close the
	// connection and this test hung.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		once.Do(func() { close(arrived) })
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	e := newEmbedder(t, httpembedder.Config{BaseURL: srv.URL + "/v1", Model: testModel})

	ctx, cancel := context.WithCancel(t.Context())
	type result struct {
		vectors [][]float32
		err     error
	}
	done := make(chan result, 1)
	go func() {
		v, err := e.Generate(ctx, []string{"a"})
		done <- result{v, err}
	}()
	<-arrived
	cancel()
	res := <-done
	if res.err == nil || res.vectors != nil {
		t.Fatalf("Generate after cancel = %v, %v; want an error and no vectors", res.vectors, res.err)
	}
	if !errors.Is(res.err, ctx.Err()) {
		t.Fatalf("error %v does not match the caller's context error %v", res.err, ctx.Err())
	}
}

// readSignal is a test transport that closes entered the first time the client reads a
// response body, so a test can end the context while that read is under way.
type readSignal struct {
	entered chan struct{}
	once    sync.Once
}

func (s *readSignal) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err == nil {
		resp.Body = &signalBody{ReadCloser: resp.Body, s: s}
	}
	return resp, err
}

type signalBody struct {
	io.ReadCloser
	s *readSignal
}

func (b *signalBody) Read(p []byte) (int, error) {
	b.s.once.Do(func() { close(b.s.entered) })
	return b.ReadCloser.Read(p)
}

// expiredCtx reports DeadlineExceeded once its parent is cancelled, so a test chooses the moment
// a deadline passes without waiting on a clock.
type expiredCtx struct{ context.Context }

func (c expiredCtx) Err() error {
	if c.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}

func TestContextEndsErrorBodyRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		wrap func(context.Context) context.Context
	}{
		{name: "cancel", wrap: func(ctx context.Context) context.Context { return ctx }},
		{name: "deadline", wrap: func(ctx context.Context) context.Context { return expiredCtx{ctx} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The embedder sends through http.DefaultClient; this test swaps it to see the
			// moment the error body read begins. The package's parallel tests start only after
			// its sequential tests, this one included, have ended.
			signal := &readSignal{entered: make(chan struct{})}
			saved := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: signal}
			t.Cleanup(func() { http.DefaultClient = saved })

			release := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusServiceUnavailable)
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			t.Cleanup(srv.Close)
			t.Cleanup(func() { close(release) })
			e := newEmbedder(t, httpembedder.Config{BaseURL: srv.URL + "/v1", Model: testModel})

			parent, cancel := context.WithCancel(t.Context())
			ctx := tc.wrap(parent)
			type result struct {
				vectors [][]float32
				err     error
			}
			done := make(chan result, 1)
			go func() {
				v, err := e.Generate(ctx, []string{"a"})
				done <- result{v, err}
			}()
			<-signal.entered
			cancel()
			res := <-done
			if res.err == nil || res.vectors != nil {
				t.Fatalf("Generate = %v, %v; want an error and no vectors", res.vectors, res.err)
			}
			if !errors.Is(res.err, ctx.Err()) {
				t.Fatalf("error %v does not match the caller's context error %v", res.err, ctx.Err())
			}
			if !strings.Contains(res.err.Error(), "503") {
				t.Fatalf("error %q does not name the provider's status 503", res.err)
			}
		})
	}
}

func TestNilContextRefused(t *testing.T) {
	p := newProvider(t, semembed(2))
	e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: testModel})
	var nilCtx context.Context
	if got, err := e.Generate(nilCtx, []string{"a"}); err == nil || got != nil {
		t.Fatalf("Generate(nil ctx) = %v, %v; want an error", got, err)
	}
	if got, err := e.GenerateQuery(nilCtx, nil); err == nil || got != nil {
		t.Fatalf("GenerateQuery(nil ctx, no texts) = %v, %v; want an error", got, err)
	}
	if reqs := p.Requests(); len(reqs) != 0 {
		t.Fatalf("provider saw %d requests under a nil context, want none", len(reqs))
	}
}

func TestNewRefusesConfig(t *testing.T) {
	for name, cfg := range map[string]httpembedder.Config{
		"no base URL":     {Model: testModel},
		"no model":        {BaseURL: "http://127.0.0.1:18081/v1"},
		"ftp scheme":      {BaseURL: "ftp://127.0.0.1/v1", Model: testModel},
		"no scheme":       {BaseURL: "localhost:18081/v1", Model: testModel},
		"relative path":   {BaseURL: "/v1", Model: testModel},
		"unparsable":      {BaseURL: "http://[::1", Model: testModel},
		"websocket":       {BaseURL: "ws://127.0.0.1:18081/v1", Model: testModel},
		"whitespace only": {BaseURL: " ", Model: testModel},
		"scheme only":     {BaseURL: "http:", Model: testModel},
		"no host":         {BaseURL: "http:///v1", Model: testModel},
	} {
		if e, err := httpembedder.New(cfg); err == nil {
			t.Errorf("%s: New(%+v) = %v, nil; want an error", name, cfg, e)
		}
	}
	for _, base := range []string{"http://127.0.0.1:18081/v1", "https://api.example.com/v1/"} {
		e, err := httpembedder.New(httpembedder.Config{BaseURL: base, Model: testModel})
		if err != nil {
			t.Fatalf("New(%q): %v", base, err)
		}
		if e.Model() != testModel || e.Dimensions() != 0 || e.Close() != nil {
			t.Fatalf("New(%q): Model %q, Dimensions %d, Close %v; want %q, 0, nil",
				base, e.Model(), e.Dimensions(), e.Close(), testModel)
		}
	}
}
