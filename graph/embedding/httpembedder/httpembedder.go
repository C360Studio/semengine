// Package httpembedder is an embedding.Embedder for a provider that serves the OpenAI-shaped
// embeddings request over HTTP, such as semembed. It is the tier 1 embedder: a product that runs
// such a provider passes an *Embedder to graph-embedding.
//
// It sends one request per call and trusts only a whole reply: one non-empty vector per text,
// each placed by its index, all of one width, from the configured model. The width is the one set
// in Config.Dimensions, or else the width of the first accepted reply. Any other reply is an error
// with no vectors. A text the provider cut to fit the model's
// input window, which semembed marks "truncated": true on its reply entry, is reported by
// GenerateMarked.
package httpembedder

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/pkg/errs"
)

var (
	_ embedding.Embedder         = (*Embedder)(nil)
	_ embedding.TruncationMarker = (*Embedder)(nil)
)

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

	// QueryPrefix is prepended to each text GenerateQuery embeds, never to documents. It is for a
	// provider that does not add the model's instructions itself: set it to the model's query
	// instruction for an asymmetric model, such as "Represent this sentence for searching relevant
	// passages: " for Snowflake arctic-embed. semembed adds that instruction for every query on its
	// own (see OmitInputType) and sends text that already starts with it unchanged, so a product on
	// semembed leaves this empty. On a provider that adds its own instruction but does not recognise
	// this one, a query would carry both. Empty sends queries raw.
	QueryPrefix string

	// OmitInputType, when true, leaves the input_type field out of every request. By default
	// GenerateQuery sends "input_type": "query" and Generate "input_type": "document", which semembed
	// reads to put the model's own query or document instruction in front of each text. Set it for a
	// provider that refuses the field.
	OmitInputType bool

	// Dimensions is the width of the vectors the model produces, such as 384 for
	// Snowflake/snowflake-arctic-embed-s. Set, it is what Dimensions reports from construction, so
	// graph-embedding can vouch for vectors it stored before a restart without waiting for the
	// provider's first reply, and a reply of any other width is refused. 0 learns the width from the
	// first accepted reply. A negative value is refused.
	Dimensions int
}

// Embedder calls an embedding provider over HTTP. It is safe for concurrent use. The caller's
// context bounds each request; the Embedder sets no timeout of its own.
type Embedder struct {
	endpoint    string
	model       string
	apiKey      string
	queryPrefix string
	inputType   bool // send input_type

	// dimensions is the configured width, or else the width of the first accepted reply, or 0
	// before one. It is read by other goroutines while a Generate call writes it.
	dimensions atomic.Int64
}

// New returns an Embedder for cfg. It refuses an empty base URL or model, a base URL that is not
// an absolute http or https URL with a host, and a negative width. It makes no request, so a
// provider that is down at start degrades graph-embedding rather than failing it: the width is
// cfg.Dimensions when set, and otherwise learned from the first accepted reply.
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
	if base.Hostname() == "" {
		return nil, fmt.Errorf("httpembedder: base URL %q has no host", cfg.BaseURL)
	}
	if cfg.Dimensions < 0 {
		return nil, fmt.Errorf("httpembedder: dimensions %d is negative", cfg.Dimensions)
	}
	e := &Embedder{
		endpoint:    base.JoinPath("embeddings").String(),
		model:       cfg.Model,
		apiKey:      cfg.APIKey,
		queryPrefix: cfg.QueryPrefix,
		inputType:   !cfg.OmitInputType,
	}
	e.dimensions.Store(int64(cfg.Dimensions))
	return e, nil
}

// Generate embeds documents: one vector per text, in the texts' order, sent as input_type
// "document" unless OmitInputType is set. No texts sends no request.
func (e *Embedder) Generate(ctx context.Context, texts []string) ([][]float32, error) {
	return vectorsOf(e.embed(ctx, texts, "document"))
}

// GenerateMarked is Generate, returning with each vector whether the provider marked its text
// truncated. A reply entry without the mark is a text the provider read whole.
func (e *Embedder) GenerateMarked(ctx context.Context, texts []string) ([]embedding.MarkedVector, error) {
	return e.embed(ctx, texts, "document")
}

// GenerateQuery embeds search queries, sent as input_type "query" unless OmitInputType is set, each
// with the configured query prefix in front. With no prefix it sends the same texts Generate does.
func (e *Embedder) GenerateQuery(ctx context.Context, texts []string) ([][]float32, error) {
	if e.queryPrefix == "" {
		return vectorsOf(e.embed(ctx, texts, "query"))
	}
	prefixed := make([]string, len(texts))
	for i, text := range texts {
		prefixed[i] = e.queryPrefix + text
	}
	return vectorsOf(e.embed(ctx, prefixed, "query"))
}

// vectorsOf drops the marks from embed's result.
func vectorsOf(marked []embedding.MarkedVector, err error) ([][]float32, error) {
	if err != nil || marked == nil {
		return nil, err
	}
	vectors := make([][]float32, len(marked))
	for i, m := range marked {
		vectors[i] = m.Vector
	}
	return vectors, nil
}

// Dimensions returns the configured width, or else the width of the first accepted reply, or 0
// before one.
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
	Model     string   `json:"model"`
	Input     []string `json:"input"`
	InputType string   `json:"input_type,omitempty"`
}

// reply's index and coordinates are pointers so that a null or absent value is seen, not read as
// zero.
type reply struct {
	Data []struct {
		Embedding []*float32 `json:"embedding"`
		Index     *int       `json:"index"`
		Truncated bool       `json:"truncated"`
	} `json:"data"`
	Model string `json:"model"`
}

type errorReply struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// embed sends texts as one request, naming inputType unless the Embedder omits it.
func (e *Embedder) embed(ctx context.Context, texts []string, inputType string) ([]embedding.MarkedVector, error) {
	if ctx == nil {
		return nil, errors.New("httpembedder: nil context")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	sent := request{Model: e.model, Input: texts}
	if e.inputType {
		sent.InputType = inputType
	}
	body, err := json.Marshal(sent)
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
		// A certificate the client does not trust will not be trusted on the next try either.
		var certErr *tls.CertificateVerificationError
		if errors.As(err, &certErr) {
			return nil, fmt.Errorf("httpembedder: %w", err)
		}
		// No reply at all: the provider is down, restarting or unreachable, which may clear.
		return nil, transient(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, refused(ctx, resp)
	}
	var r reply
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("httpembedder: decode reply: %w", err)
	}
	return e.accept(r, len(texts))
}

// refused returns the error for a reply outside 2xx, carrying the provider's own message when it
// sent one. A 429 matches errs.ErrRateLimited and is returned at once: Retry-After is not waited
// on and the request is not sent again, because the caller decides when to try again. A 500, 502,
// 503 or 504 is a provider failing in a way it may recover from, so it is marked transient. Every
// other status, 501 among them, refuses the request itself and is left unmarked, because sending
// it again would be refused again.
func refused(ctx context.Context, resp *http.Response) error {
	var err error
	var refusal errorReply
	decodeErr := json.NewDecoder(resp.Body).Decode(&refusal)
	switch {
	case decodeErr == nil && refusal.Error.Message != "":
		err = fmt.Errorf("provider answered %s: %s", resp.Status, refusal.Error.Message)
	case decodeErr != nil && ctx.Err() != nil:
		err = fmt.Errorf("provider answered %s, reading its body: %w", resp.Status, ctx.Err())
	default:
		err = fmt.Errorf("provider answered %s", resp.Status)
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return fmt.Errorf("httpembedder: %w: %w", err, errs.ErrRateLimited)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return transient(err)
	default:
		return fmt.Errorf("httpembedder: %w", err)
	}
}

// transient marks err as a provider failure that may clear, so a caller can retry it by reading
// errs.IsTransient rather than the text. The action word names no reason a caller reads from
// text, such as a timeout, a refused connection or a dimension.
func transient(err error) error {
	return errs.WrapTransient(err, "httpembedder", "embed", "embeddings request")
}

// accept returns the reply's vectors in input order, each with its entry's truncated mark, or an
// error if any part of the reply cannot be trusted. A reply of another width than the configured
// or learned one is refused; with neither, the width is learned only from a reply accepted whole.
func (e *Embedder) accept(r reply, n int) ([]embedding.MarkedVector, error) {
	if r.Model != "" && r.Model != e.model {
		return nil, fmt.Errorf("httpembedder: reply is from model %q, configured %q", r.Model, e.model)
	}
	if len(r.Data) != n {
		return nil, fmt.Errorf("httpembedder: reply has %d vectors for %d texts", len(r.Data), n)
	}
	vectors := make([]embedding.MarkedVector, n)
	width := 0
	for _, d := range r.Data {
		if d.Index == nil {
			return nil, errors.New("httpembedder: reply has a vector with no index")
		}
		i := *d.Index
		switch {
		case i < 0 || i >= n:
			return nil, fmt.Errorf("httpembedder: reply index %d is outside 0..%d", i, n-1)
		case vectors[i].Vector != nil:
			return nil, fmt.Errorf("httpembedder: reply repeats index %d", i)
		case len(d.Embedding) == 0:
			return nil, fmt.Errorf("httpembedder: reply has an empty vector at index %d", i)
		case width != 0 && len(d.Embedding) != width:
			return nil, fmt.Errorf("httpembedder: reply mixes vector dimension %d and %d", width, len(d.Embedding))
		}
		vector := make([]float32, len(d.Embedding))
		for j, c := range d.Embedding {
			if c == nil {
				return nil, fmt.Errorf("httpembedder: reply has a null coordinate at index %d", i)
			}
			vector[j] = *c
		}
		width = len(vector)
		vectors[i] = embedding.MarkedVector{Vector: vector, Truncated: d.Truncated}
	}
	if !e.dimensions.CompareAndSwap(0, int64(width)) {
		if known := int(e.dimensions.Load()); known != width {
			return nil, fmt.Errorf("httpembedder: reply dimension %d, expected %d", width, known)
		}
	}
	return vectors, nil
}
