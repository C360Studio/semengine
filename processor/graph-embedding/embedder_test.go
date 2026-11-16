package graphembedding

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
)

// testEmbedderType is the name the tests register their embedder under, as a product names its own.
const testEmbedderType = "word-count"

// wordDimensions is the test embedder's vector width: wide enough that the few words a test uses
// land in different positions.
const wordDimensions = 1024

// stopWords are left out of a word-count vector, so a query of them alone embeds to the zero vector.
var stopWords = map[string]bool{"a": true, "an": true, "and": true, "in": true, "is": true, "of": true, "the": true, "to": true}

// wordVector is the test embedder's vector for text: each word other than a stop word, lower-cased,
// adds one at a position its hash picks, and the counts are scaled to unit length. Texts with the
// same words in the same proportions get the same vector; texts sharing no word score 0.
func wordVector(text string) []float32 {
	vector := make([]float32, wordDimensions)
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if stopWords[word] {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(word))
		vector[h.Sum32()%wordDimensions]++
	}
	var sum float64
	for _, v := range vector {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vector
	}
	norm := float32(math.Sqrt(sum))
	for i := range vector {
		vector[i] /= norm
	}
	return vector
}

// wordEmbedder is the embedder the tests register through RegisterWithEmbedder, as a product
// registers its own. It counts its Close calls and keeps the query texts it embedded.
type wordEmbedder struct {
	mu       sync.Mutex
	queries  []string
	closes   int
	closeErr error // returned by the first Close when set
}

func (e *wordEmbedder) Generate(_ context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vectors[i] = wordVector(text)
	}
	return vectors, nil
}

func (e *wordEmbedder) GenerateQuery(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	e.queries = append(e.queries, texts...)
	e.mu.Unlock()
	return e.Generate(ctx, texts)
}

func (e *wordEmbedder) Dimensions() int { return wordDimensions }
func (e *wordEmbedder) Model() string   { return "word-count-v1" }

func (e *wordEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closes++
	if e.closes == 1 && e.closeErr != nil {
		return e.closeErr
	}
	return nil
}

func (e *wordEmbedder) closeCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closes
}

func (e *wordEmbedder) queryTexts() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.queries...)
}

// embedderSource is the newEmbedder a test registers: it records every embedder it builds, and
// each one's first Close returns closeErr when set.
type embedderSource struct {
	mu       sync.Mutex
	built    []*wordEmbedder
	closeErr error
}

func (s *embedderSource) newEmbedder() (embedding.Embedder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := &wordEmbedder{closeErr: s.closeErr}
	s.built = append(s.built, e)
	return e, nil
}

func (s *embedderSource) builds() []*wordEmbedder {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*wordEmbedder(nil), s.built...)
}

// registeredFactory registers graph-embedding with register on a registry of its own at each
// call and creates it through the registry's CreateComponent, the call a product's component
// manager makes, so a test builds the component as a product's composition root does.
func registeredFactory(t *testing.T, register func(*component.Registry) error) component.Factory {
	t.Helper()
	return func(raw json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
		registry := component.NewRegistry()
		if err := register(registry); err != nil {
			t.Fatalf("register graph-embedding: %v", err)
		}
		return registry.CreateComponent(key, types.ComponentConfig{
			Type: "processor", Name: key, Enabled: true, Config: raw,
		}, deps, nil)
	}
}

// productFactory is graph-embedding as a product registers it: with source's embedders, under
// testEmbedderType.
func productFactory(t *testing.T, source *embedderSource) component.Factory {
	t.Helper()
	return registeredFactory(t, func(registry *component.Registry) error {
		return RegisterWithEmbedder(registry, testEmbedderType, source.newEmbedder)
	})
}

// TestRegisterWithoutEmbedderRefuses: graph-embedding has no embedder of its own, so the factory
// plain Register adds refuses construction, class invalid, naming RegisterWithEmbedder and tier 1;
// the configuration it refuses builds through RegisterWithEmbedder. RegisterWithEmbedder refuses an
// empty name and a missing embedder constructor, both invalid, adding no factory.
func TestRegisterWithoutEmbedderRefuses(t *testing.T) {
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	deps := component.Dependencies{NATSClient: client}
	for _, raw := range []string{``, `{}`} {
		built, err := registeredFactory(t, Register)(json.RawMessage(raw), deps)
		if err == nil {
			t.Fatalf("Register's factory built %T from %q", built, raw)
		}
		if !errs.IsInvalid(err) || !strings.Contains(err.Error(), "RegisterWithEmbedder") ||
			!strings.Contains(err.Error(), "tier 1") {
			t.Fatalf("Register's factory refused %q with %v, want invalid naming RegisterWithEmbedder and tier 1", raw, err)
		}
		if _, err := productFactory(t, &embedderSource{})(json.RawMessage(raw), deps); err != nil {
			t.Fatalf("RegisterWithEmbedder's factory refused %q: %v", raw, err)
		}
	}

	source := &embedderSource{}
	for name, register := range map[string]func(*component.Registry) error{
		"empty name": func(r *component.Registry) error { return RegisterWithEmbedder(r, "", source.newEmbedder) },
		"no constructor": func(r *component.Registry) error {
			return RegisterWithEmbedder(r, testEmbedderType, nil)
		},
	} {
		registry := component.NewRegistry()
		if err := register(registry); !errs.IsInvalid(err) {
			t.Errorf("RegisterWithEmbedder with %s = %v, want an invalid refusal", name, err)
		}
		if _, ok := registry.ListFactories()[key]; ok {
			t.Errorf("RegisterWithEmbedder with %s added a factory", name)
		}
	}
	if built := source.builds(); len(built) != 0 {
		t.Errorf("registration built %d embedders, want none", len(built))
	}
}
