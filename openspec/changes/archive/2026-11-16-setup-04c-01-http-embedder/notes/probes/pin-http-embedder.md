# Probe: the pin's HTTP embedder (`graph/embedding/http_embedder.go`)

This probe shows how the pin's `HTTPEmbedder` handles replies it should not trust. The design's shape-sweep rows
S1 to S6 cite it.

- **Pin:** SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, in a scratch clone checked out at the pin. The clone
  is a local copy only; it is not in this repository.
- **Go:** `go version go1.26.4 darwin/arm64`
- **Run on:** 2026-11-16, with `probe489_test.go` placed in the pin's `graph/embedding/` package. The file was removed
  after the run, and the pin copy has no other changes.

## Command

Run from the root of the pin copy:

```bash
go test -count=1 -v -run TestProbe489 ./graph/embedding/
```

## Source (`graph/embedding/probe489_test.go`, throwaway)

```go
package embedding

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func probeServer(t *testing.T, body string, status int) *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(status)
        _, _ = w.Write([]byte(body))
    }))
}

func TestProbe489(t *testing.T) {
    cases := map[string]struct {
        body   string
        status int
    }{
        "reversed_index": {`{"object":"list","model":"m","data":[{"index":1,"embedding":[2,2]},{"index":0,"embedding":[1,1]}]}`, 200},
        "mixed_width":    {`{"object":"list","model":"m","data":[{"index":0,"embedding":[1,1]},{"index":1,"embedding":[1,1,1]}]}`, 200},
        "empty_vector":   {`{"object":"list","model":"m","data":[{"index":0,"embedding":[]},{"index":1,"embedding":[1,1]}]}`, 200},
        "other_model":    {`{"object":"list","model":"served-other","data":[{"index":0,"embedding":[1]},{"index":1,"embedding":[1]}]}`, 200},
        "semembed_400":   {`{"error":{"message":"Input cannot be empty","type":"invalid_request_error"}}`, 400},
        "semembed_500":   {`{"error":{"message":"Failed to generate embeddings: x","type":"internal_error"}}`, 500},
    }
    for name, c := range cases {
        srv := probeServer(t, c.body, c.status)
        e, _ := NewHTTPEmbedder(HTTPConfig{BaseURL: srv.URL, Model: "m"})
        v, err := e.Generate(context.Background(), []string{"a", "b"})
        b, _ := json.Marshal(v)
        t.Logf("%s: vectors=%s dims=%d err=%v", name, b, e.Dimensions(), err)
        srv.Close()
    }
    // path the pin posts to
    var path string
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        path = r.URL.Path + " auth=" + r.Header.Get("Authorization")
        _, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1]}]}`))
    }))
    e, _ := NewHTTPEmbedder(HTTPConfig{BaseURL: srv.URL, Model: "m"})
    _, err := e.Generate(context.Background(), []string{"a"})
    t.Logf("path=%s err=%v", path, err)
    srv.Close()
    e, _ = NewHTTPEmbedder(HTTPConfig{BaseURL: "http://127.0.0.1:1", Model: "m"})
    _, err = e.Generate(context.Background(), []string{"a"})
    t.Logf("refused: err=%v", err)
}
```

## Output (verbatim but tabs shown as spaces; exit status 0)

```text
=== RUN   TestProbe489
2026/11/16 09:03:46 INFO resolved embedding dimensions from endpoint model=m dimensions=2
    probe489_test.go:36: mixed_width: vectors=[[1,1],[1,1,1]] dims=2 err=<nil>
2026/11/16 09:03:46 INFO resolved embedding dimensions from endpoint model=m dimensions=2
    probe489_test.go:36: empty_vector: vectors=[[],[1,1]] dims=2 err=<nil>
2026/11/16 09:03:46 INFO resolved embedding dimensions from endpoint model=m dimensions=1
    probe489_test.go:36: other_model: vectors=[[1],[1]] dims=1 err=<nil>
    probe489_test.go:36: semembed_400: vectors=null dims=0 err=embedding API call failed: error, status code: 400, status: 400 Bad Request, message: Input cannot be empty
    probe489_test.go:36: semembed_500: vectors=null dims=0 err=embedding API call failed: error, status code: 500, status: 500 Internal Server Error, message: Failed to generate embeddings: x
2026/11/16 09:03:46 INFO resolved embedding dimensions from endpoint model=m dimensions=2
    probe489_test.go:36: reversed_index: vectors=[[2,2],[1,1]] dims=2 err=<nil>
2026/11/16 09:03:46 INFO resolved embedding dimensions from endpoint model=m dimensions=1
    probe489_test.go:47: path=/embeddings auth=Bearer dummy-key err=<nil>
    probe489_test.go:51: refused: err=embedding API call failed: Post "http://127.0.0.1:1/embeddings": dial tcp 127.0.0.1:1: connect: connection refused
--- PASS: TestProbe489 (0.00s)
PASS
ok      github.com/c360studio/semstreams/graph/embedding    0.514s
```

## What it shows

Each row is one shape-sweep finding in `inventory.md`.

| Row | Reply the pin got | What the pin did |
| --- | --- | --- |
| S1 | `index` order reversed | Ignored `index` and returned the vectors in reply order: `[[2,2],[1,1]]` |
| S2 | Vectors of two different widths | Accepted both without error |
| S3 | An empty vector | Accepted it without error |
| S4 | A different `model` name in the reply | Accepted it without error |
| S5 | No API key configured | Sent `Authorization: Bearer dummy-key` |
| S6 | Any request | Posted to `<BaseURL>/embeddings`, while semembed serves `/v1/embeddings` |

- **Errors from semembed:** a 400 or 500 comes back as an error that names the status and semembed's message.
- **Connection refused:** the error text contains `connection refused`, so `classifyEmbedErr` files it as
  `connection_refused`.
