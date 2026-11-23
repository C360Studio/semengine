package httpembedder_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph/embedding/httpembedder"
)

// FuzzReplyIsWholeOrRefused feeds arbitrary bytes as a provider's 200 reply to a two-text
// request. Whatever the bytes, the call never panics, and it either returns an error with no
// vectors or two non-empty vectors whose width is the one Dimensions reports, each the
// numbers the reply carries at that vector's index and marked truncated only when that entry
// carries "truncated": true.
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
		`{"object":"list","model":"m","data":[{"embedding":[1]},{"index":1,"embedding":[2]}]}`,
		`{"object":"list","model":"m","data":[{"index":null,"embedding":[1]},{"index":1,"embedding":[2]}]}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[null,1]},{"index":1,"embedding":[2,2]}]}`,
		`{"object":"list","model":"m","data":null}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[1,1],"truncated":true},{"index":1,"embedding":[2,2],"truncated":false}]}`,
		`{"object":"list","model":"m","data":[{"index":1,"embedding":[2,2],"truncated":true},{"index":0,"embedding":[1,1],"truncated":null}]}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[1,1],"truncated":"true"},{"index":1,"embedding":[2,2]}]}`,
		`{"object":"list","model":"m","data":[{"index":0,"embedding":[1,1],"truncated":1},{"index":1,"embedding":[2,2]}]}`,
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
		got, err := e.GenerateMarked(t.Context(), []string{"a", "b"})
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
			if len(v.Vector) != width {
				t.Fatalf("vector %d has width %d, Dimensions() says %d", i, len(v.Vector), width)
			}
		}
		want, marks := replyVectors(t, body), replyMarks(t, body)
		for i := range got {
			for j := range got[i].Vector {
				if got[i].Vector[j] != want[i][j] {
					t.Fatalf("accepted %v, but the reply carries %v", got, want)
				}
			}
			if got[i].Truncated != marks[i] {
				t.Fatalf("text %d marked truncated %v, but the reply's mark is %v", i, got[i].Truncated, marks[i])
			}
		}
	})
}

// replyVectors reads an accepted reply without the client's decoder: every entry must carry a
// numeric index in 0..1 and a numeric coordinate in each place, and the result is each entry's
// numbers at its index. An accepted reply that fails any of this fails the test.
func replyVectors(t *testing.T, body string) [2][]float32 {
	t.Helper()
	var r struct {
		Data []struct {
			Index     any   `json:"index"`
			Embedding []any `json:"embedding"`
		} `json:"data"`
	}
	d := json.NewDecoder(strings.NewReader(body))
	d.UseNumber()
	if err := d.Decode(&r); err != nil {
		t.Fatalf("accepted a reply that does not decode: %v", err)
	}
	var out [2][]float32
	for _, entry := range r.Data {
		index, ok := entry.Index.(json.Number)
		if !ok {
			t.Fatalf("accepted an entry whose index is %v, not a number", entry.Index)
		}
		i, err := strconv.Atoi(index.String())
		if err != nil || i < 0 || i > 1 {
			t.Fatalf("accepted index %s", index)
		}
		for _, c := range entry.Embedding {
			n, ok := c.(json.Number)
			if !ok {
				t.Fatalf("accepted a coordinate %v that is not a number", c)
			}
			f, err := strconv.ParseFloat(n.String(), 32)
			if err != nil {
				t.Fatalf("accepted coordinate %s: %v", n, err)
			}
			out[i] = append(out[i], float32(f))
		}
	}
	return out
}

// replyMarks reads each accepted entry's truncated mark at its index without the client's decoder:
// absent or null is false, and an accepted mark that is not a boolean fails the test.
func replyMarks(t *testing.T, body string) [2]bool {
	t.Helper()
	var r struct {
		Data []struct {
			Index     json.Number `json:"index"`
			Truncated any         `json:"truncated"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("accepted a reply that does not decode: %v", err)
	}
	var out [2]bool
	for _, entry := range r.Data {
		i, err := strconv.Atoi(entry.Index.String())
		if err != nil || i < 0 || i > 1 {
			t.Fatalf("accepted index %s", entry.Index)
		}
		switch mark := entry.Truncated.(type) {
		case nil:
		case bool:
			out[i] = mark
		default:
			t.Fatalf("accepted a truncated mark %v that is not a boolean", mark)
		}
	}
	return out
}
