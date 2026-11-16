package httpembedder_test

import (
	"sync"
	"testing"

	"github.com/c360studio/semengine/graph/embedding/httpembedder"
)

// TestHTTPEmbedder_DimensionsIsRaceFree reads Dimensions from many goroutines while the first
// reply is being accepted, as graph-embedding's watcher and workers do. Run under -race.
func TestHTTPEmbedder_DimensionsIsRaceFree(t *testing.T) {
	t.Parallel()

	p := newProvider(t, semembed(768))
	e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: "all-mpnet-base-v2"})

	done := make(chan struct{})
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = e.Dimensions()
				}
			}
		}()
	}

	if _, err := e.Generate(t.Context(), []string{"first call learns the width"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	close(done)
	readers.Wait()

	if got := e.Dimensions(); got != 768 {
		t.Fatalf("Dimensions() = %d after a 768-wide reply, want 768", got)
	}
}

// TestHTTPEmbedder_DimensionsUnresolvedUntilFirstResponse holds Dimensions at 0 until a reply is
// accepted. A placeholder width would be folded into dedup keys and split them in two at the
// first reply.
func TestHTTPEmbedder_DimensionsUnresolvedUntilFirstResponse(t *testing.T) {
	t.Parallel()

	p := newProvider(t, semembed(768))
	e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: "all-mpnet-base-v2"})

	if got := e.Dimensions(); got != 0 {
		t.Fatalf("Dimensions() = %d before any reply, want 0 (not yet known)", got)
	}
	if _, err := e.Generate(t.Context(), []string{"learn me"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := e.Dimensions(); got != 768 {
		t.Fatalf("Dimensions() = %d after a 768-wide reply, want 768", got)
	}
}

// TestHTTPEmbedder_DimensionsResolveOnceAndStick holds the learned width across calls. A model
// that is really 384 wide must report 384 and keep it.
func TestHTTPEmbedder_DimensionsResolveOnceAndStick(t *testing.T) {
	t.Parallel()

	p := newProvider(t, semembed(384))
	e := newEmbedder(t, httpembedder.Config{BaseURL: p.BaseURL(), Model: "all-MiniLM-L6-v2"})

	for i := range 3 {
		if _, err := e.Generate(t.Context(), []string{"call"}); err != nil {
			t.Fatalf("Generate #%d: %v", i, err)
		}
		if got := e.Dimensions(); got != 384 {
			t.Fatalf("Dimensions() = %d after call #%d, want a stable 384", got, i)
		}
	}
}
