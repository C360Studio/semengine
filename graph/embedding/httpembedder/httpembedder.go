// Package httpembedder is an embedding.Embedder for a provider that serves the OpenAI-shaped
// embeddings request over HTTP, such as semembed. It is the tier 1 embedder: a product that runs
// such a provider passes an *Embedder to graph-embedding.
//
// It sends one request per call and trusts only a whole reply: one non-empty vector per text,
// each placed by its index, all of the width the first accepted reply had, from the configured
// model. Any other reply is an error with no vectors.
package httpembedder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/c360studio/semengine/graph/embedding"
)

var _ embedding.Embedder = (*Embedder)(nil)

// Config configures an Embedder.
type Config struct {
	// BaseURL is the provider's API root; requests go to BaseURL + "/embeddings". For semembed
	// it ends in /v1, for example "http://127.0.0.1:8081/v1". Required; http or https.
	BaseURL string

	// Model is the model name sent with every request. A reply naming another model is refused.
	// Required.
	Model string

	// APIKey, when set, is sent as "Authorization: Bearer <APIKey>". A local provider needs none,
	// and then no Authorization header is sent.
	APIKey string

	// QueryPrefix is prepended to each text GenerateQuery embeds, never to documents. Set it to
	// the model's query instruction for an asymmetric model, such as "Represent this sentence for
	// searching relevant passages: " for Snowflake arctic-embed. Empty sends queries raw.
	QueryPrefix string
}

// Embedder calls an embedding provider over HTTP. It is safe for concurrent use. The caller's
// context bounds each request; the Embedder sets no timeout of its own.
type Embedder struct {
	endpoint    string
	model       string
	apiKey      string
	queryPrefix string

	// dimensions is the width of the first accepted reply, or 0 before one. It is read by other
	// goroutines while a Generate call writes it.
	dimensions atomic.Int64
}

// New returns an Embedder for cfg. It refuses an empty base URL or model, and a base URL that is
// not an absolute http or https URL. It makes no request: the width is learned from the first
// reply, so a provider that is down at start degrades graph-embedding rather than failing it.
func New(cfg Config) (*Embedder, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("httpembedder: base URL is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("httpembedder: model is required")
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("httpembedder: base URL: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, fmt.Errorf("httpembedder: base URL %q is not an http or https URL", cfg.BaseURL)
	}
	return &Embedder{
		endpoint:    base.JoinPath("embeddings").String(),
		model:       cfg.Model,
		apiKey:      cfg.APIKey,
		queryPrefix: cfg.QueryPrefix,
	}, nil
}

// Generate embeds documents: one vector per text, in the texts' order. No texts sends no request.
func (e *Embedder) Generate(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, texts)
}

// GenerateQuery embeds search queries, each with the configured query prefix in front. With no
// prefix it sends the same request Generate does.
func (e *Embedder) GenerateQuery(ctx context.Context, texts []string) ([][]float32, error) {
	if e.queryPrefix == "" {
		return e.embed(ctx, texts)
	}
	prefixed := make([]string, len(texts))
	for i, text := range texts {
		prefixed[i] = e.queryPrefix + text
	}
	return e.embed(ctx, prefixed)
}

// Dimensions returns the width of the first accepted reply, or 0 before one.
func (e *Embedder) Dimensions() int {
	return int(e.dimensions.Load())
}

// Model returns the configured model name.
func (e *Embedder) Model() string {
	return e.model
}

// Close returns nil: requests go through the shared default transport, which holds nothing for
// one Embedder.
func (e *Embedder) Close() error {
	return nil
}

type request struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type reply struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
}

type errorReply struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (e *Embedder) embed(ctx context.Context, texts []string) ([][]float32, error) {
	if ctx == nil {
		return nil, errors.New("httpembedder: nil context")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(request{Model: e.model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("httpembedder: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("httpembedder: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpembedder: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var refusal errorReply
		if json.NewDecoder(resp.Body).Decode(&refusal) == nil && refusal.Error.Message != "" {
			return nil, fmt.Errorf("httpembedder: provider answered %s: %s", resp.Status, refusal.Error.Message)
		}
		return nil, fmt.Errorf("httpembedder: provider answered %s", resp.Status)
	}
	var r reply
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("httpembedder: decode reply: %w", err)
	}
	return e.accept(r, len(texts))
}

// accept returns the reply's vectors in input order, or an error if any part of the reply cannot
// be trusted. The width is learned only from a reply accepted whole.
func (e *Embedder) accept(r reply, n int) ([][]float32, error) {
	if r.Model != "" && r.Model != e.model {
		return nil, fmt.Errorf("httpembedder: reply is from model %q, configured %q", r.Model, e.model)
	}
	if len(r.Data) != n {
		return nil, fmt.Errorf("httpembedder: reply has %d vectors for %d texts", len(r.Data), n)
	}
	vectors := make([][]float32, n)
	width := 0
	for _, d := range r.Data {
		switch {
		case d.Index < 0 || d.Index >= n:
			return nil, fmt.Errorf("httpembedder: reply index %d is outside 0..%d", d.Index, n-1)
		case vectors[d.Index] != nil:
			return nil, fmt.Errorf("httpembedder: reply repeats index %d", d.Index)
		case len(d.Embedding) == 0:
			return nil, fmt.Errorf("httpembedder: reply has an empty vector at index %d", d.Index)
		case width != 0 && len(d.Embedding) != width:
			return nil, fmt.Errorf("httpembedder: reply mixes vector dimension %d and %d", width, len(d.Embedding))
		}
		width = len(d.Embedding)
		vectors[d.Index] = d.Embedding
	}
	if !e.dimensions.CompareAndSwap(0, int64(width)) {
		if learned := int(e.dimensions.Load()); learned != width {
			return nil, fmt.Errorf("httpembedder: reply dimension %d, learned %d", width, learned)
		}
	}
	return vectors, nil
}
