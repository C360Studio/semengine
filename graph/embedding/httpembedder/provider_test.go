package httpembedder_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// The wire types below are written from semembed's OpenAI-shaped API, not from the client, so
// the tests hold the client to what a real provider sends and accepts.

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embedObject's Truncated is semembed's mark on a text it cut to fit the model's input window;
// semembed leaves it out for a text it read whole.
type embedObject struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
	Truncated bool      `json:"truncated,omitempty"`
}

type embedUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type embedReply struct {
	Object string        `json:"object"`
	Data   []embedObject `json:"data"`
	Model  string        `json:"model"`
	Usage  embedUsage    `json:"usage"`
}

type errorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type errorReply struct {
	Error errorDetail `json:"error"`
}

// seenRequest is what the fake provider recorded of one request.
type seenRequest struct {
	Method        string
	Path          string
	Authorization string
	HasAuth       bool
	Body          map[string]any
	Decoded       embedRequest
}

// provider is a fake embedding service shaped like semembed: it serves POST /v1/embeddings
// only, refuses an empty input with 400, and otherwise answers with what reply returns.
type provider struct {
	srv   *httptest.Server
	mu    sync.Mutex
	seen  []seenRequest
	reply func(req embedRequest) (int, any)
}

// BaseURL is the base URL a product configures for this provider, the /v1 prefix included.
func (p *provider) BaseURL() string { return p.srv.URL + "/v1" }

// Requests returns a copy of every request the provider has received so far.
func (p *provider) Requests() []seenRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]seenRequest(nil), p.seen...)
}

func newProvider(t *testing.T, reply func(req embedRequest) (int, any)) *provider {
	t.Helper()
	p := &provider{reply: reply}
	p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *provider) serve(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	var decoded embedRequest
	body := json.NewDecoder(r.Body)
	decodeErr := body.Decode(&raw)
	if decodeErr == nil {
		b, _ := json.Marshal(raw)
		decodeErr = json.Unmarshal(b, &decoded)
	}
	auth, hasAuth := r.Header["Authorization"]
	seen := seenRequest{Method: r.Method, Path: r.URL.Path, HasAuth: hasAuth, Body: raw, Decoded: decoded}
	if hasAuth {
		seen.Authorization = auth[0]
	}
	p.mu.Lock()
	p.seen = append(p.seen, seen)
	p.mu.Unlock()

	if r.Method != http.MethodPost || r.URL.Path != "/v1/embeddings" {
		http.NotFound(w, r)
		return
	}
	status, out := http.StatusOK, any(nil)
	switch {
	case decodeErr != nil:
		status, out = http.StatusUnprocessableEntity, errorReply{errorDetail{decodeErr.Error(), "invalid_request_error"}}
	case len(decoded.Input) == 0:
		status, out = http.StatusBadRequest, errorReply{errorDetail{"Input cannot be empty", "invalid_request_error"}}
	default:
		status, out = p.reply(decoded)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if s, ok := out.(string); ok {
		_, _ = w.Write([]byte(s))
		return
	}
	_ = json.NewEncoder(w).Encode(out)
}

// semembed answers as semembed does: one vector of the given width per input, at the input's
// index, under the requested model. Vector i holds the value i+1 in every position, so a test
// can tell which input a vector belongs to.
func semembed(width int) func(req embedRequest) (int, any) {
	return func(req embedRequest) (int, any) {
		data := make([]embedObject, len(req.Input))
		for i := range req.Input {
			vec := make([]float32, width)
			for j := range vec {
				vec[j] = float32(i + 1)
			}
			data[i] = embedObject{Object: "embedding", Embedding: vec, Index: i}
		}
		return http.StatusOK, embedReply{Object: "list", Data: data, Model: req.Model, Usage: embedUsage{1, 1}}
	}
}

// fixed answers every request with the same status and body.
func fixed(status int, body any) func(req embedRequest) (int, any) {
	return func(embedRequest) (int, any) { return status, body }
}
