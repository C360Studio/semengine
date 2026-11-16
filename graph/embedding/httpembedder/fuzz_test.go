package httpembedder_test

import (
	"net/http"
	"testing"

	"github.com/c360studio/semengine/graph/embedding/httpembedder"
)

// FuzzReplyIsWholeOrRefused feeds arbitrary bytes as a provider's 200 reply to a two-text
// request. Whatever the bytes, the call never panics, and it either returns an error with no
// vectors or two non-empty vectors whose width is the one Dimensions reports.
func FuzzReplyIsWholeOrRefused(f *testing.F) {
	for _, seed := range []string{
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[1,1]},{"index":1,"embedding":[2,2]}]}`,
		`{"object":"list","data":[{"index":1,"embedding":[2,2]},{"index":0,"embedding":[1,1]}]}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[1,1]},{"index":1,"embedding":[1,1,1]}]}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[]},{"index":1,"embedding":[1,1]}]}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":null},{"index":1,"embedding":[1,1]}]}`,
		`{"object":"list","model":"other","data":[{"index":0,"embedding":[1]},{"index":1,"embedding":[1]}]}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[1]},{"index":2,"embedding":[1]}]}`,
		`{"object":"list","model":"m","data":[{"index":-1,"embedding":[1]},{"index":1,"embedding":[1]}]}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[1]},{"index":0,"embedding":[1]}]}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[1]}]}`,
		`{"object":"list","model":"m","data":[{"embedding":[1]},{"embedding":[1]}]}`,
		`{"object":"list","model":"m","data":null}`,
		`{"index":99999999999999999999}`,
		`not json`,
		``,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body string) {
		p := newProvider(t, fixed(http.StatusOK, body))
		e, err := httpembedder.New(httpembedder.Config{BaseURL: p.BaseURL(), Model: "m"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, err := e.Generate(t.Context(), []string{"a", "b"})
		if err != nil {
			if got != nil {
				t.Fatalf("error %v came with vectors %v", err, got)
			}
			if d := e.Dimensions(); d != 0 {
				t.Fatalf("Dimensions() = %d after a refused first reply, want 0", d)
			}
			return
		}
		width := e.Dimensions()
		if len(got) != 2 || width == 0 {
			t.Fatalf("accepted %v with Dimensions() %d; want two vectors of a learned width", got, width)
		}
		for i, v := range got {
			if len(v) != width {
				t.Fatalf("vector %d has width %d, Dimensions() says %d", i, len(v), width)
			}
		}
	})
}
