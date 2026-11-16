# Probe: the HTTP embedder against semembed 7ceb5281

This probe runs `httpembedder` against a real semembed and checks each point design D5 lists: count and order of a
multi-text reply, the served model, `Dimensions()` before and after the first reply, query versus document vectors,
a cancelled context and a short deadline, and recovery across a provider restart. It runs outside `task verify`;
nothing in verify reaches an outside service.

- **Provider:** semembed at commit `7ceb5281`, in local Docker as container `semengine-semembed`, image
  `semengine-semembed:7ceb5281` (image ID `sha256:a7fe8a48c7ebe1413eb96b39d1e935fa404b320fb36a009676b3359ba999bf4a`,
  created 2026-11-16T07:53:02Z).
- **Image build and run:** from a local semembed checkout, with semembed's own Dockerfile:

  ```bash
  mkdir -p /tmp/semembed-7ceb5281
  git -C <semembed checkout> archive 7ceb5281 | tar -x -C /tmp/semembed-7ceb5281
  docker build -t semengine-semembed:7ceb5281 /tmp/semembed-7ceb5281
  docker run -d --name semengine-semembed -p 127.0.0.1:18081:8081 semengine-semembed:7ceb5281
  ```

  The image history shows semembed's Dockerfile steps, including `ENV SEMEMBED_MODEL=Snowflake/snowflake-arctic-embed-s`
  and `EXPOSE [8081/tcp]`. The model downloads on the container's first start.
- **Container:** port 8081 published on `127.0.0.1:18081` (from `docker inspect`), so the probe uses
  `http://127.0.0.1:18081/v1`.
- **Model:** `Snowflake/snowflake-arctic-embed-s`, 384 dimensions. Query prefix:
  `"Represent this sentence for searching relevant passages: "` (quoted to show the trailing space).
- **Client:** `graph/embedding/httpembedder` at commit `70bd0d78fe0ac845225d6dd051abb82fbaeb7439`, built through
  `httpembedder.New`.
- **Go:** `go version go1.26.11 darwin/arm64`
- **Run on:** 2026-11-16. The probe was a throwaway `main` package at `cmd/probesemembed/main.go` in the worktree,
  deleted after the run; it is not in this repository.

Tabs in the fenced blocks below are replaced with spaces, because the markdown lint forbids hard tabs.

## Command

Run from the repository root, with the container running:

```bash
go run ./cmd/probesemembed
```

The probe stops the container itself (`docker stop semengine-semembed`) for the recovery check, starts it again
(`docker start semengine-semembed`), and retries every half second until a call succeeds.

## Source (`cmd/probesemembed/main.go`, throwaway)

```go
// Command probesemembed is a throwaway probe: it drives httpembedder against a running semembed
// and prints what each check saw. It is deleted after the run.
package main

import (
    "context"
    "errors"
    "fmt"
    "math"
    "os"
    "os/exec"
    "strings"
    "time"

    "github.com/c360studio/semengine/graph/embedding/httpembedder"
)

const (
    baseURL = "http://127.0.0.1:18081/v1"
    model   = "Snowflake/snowflake-arctic-embed-s"
    prefix  = "Represent this sentence for searching relevant passages: "
)

var failed bool

func check(name string, ok bool, detail string) {
    verdict := "PASS"
    if !ok {
        verdict = "FAIL"
        failed = true
    }
    fmt.Printf("%s %s: %s\n", verdict, name, detail)
}

func cosine(a, b []float32) float64 {
    var dot, na, nb float64
    for i := range a {
        dot += float64(a[i]) * float64(b[i])
        na += float64(a[i]) * float64(a[i])
        nb += float64(b[i]) * float64(b[i])
    }
    return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func newEmbedder(m string) *httpembedder.Embedder {
    e, err := httpembedder.New(httpembedder.Config{BaseURL: baseURL, Model: m, QueryPrefix: prefix})
    if err != nil {
        fmt.Println("New:", err)
        os.Exit(2)
    }
    return e
}

func docker(args ...string) {
    out, err := exec.Command("docker", args...).CombinedOutput()
    fmt.Printf("docker %s: %s err=%v\n", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
}

func main() {
    ctx := context.Background()
    e := newEmbedder(model)

    // Readiness: no reply yet, so no width.
    check("dimensions before first reply", e.Dimensions() == 0, fmt.Sprintf("Dimensions()=%d", e.Dimensions()))

    // Count and index order: each vector of a three-text call must match the vector the same
    // text gets on its own.
    texts := []string{"the cat sat on the mat", "graph databases store triples", "rain is expected tomorrow"}
    vs, err := e.Generate(ctx, texts)
    check("multi-text call", err == nil && len(vs) == len(texts), fmt.Sprintf("texts=%d vectors=%d err=%v", len(texts), len(vs), err))
    check("dimensions after first reply", e.Dimensions() == 384, fmt.Sprintf("Dimensions()=%d", e.Dimensions()))
    for i, text := range texts {
        one, err := e.Generate(ctx, []string{text})
        if err != nil {
            check(fmt.Sprintf("index order %d", i), false, err.Error())
            continue
        }
        same := cosine(vs[i], one[0])
        best, bestJ := -2.0, -1
        for j := range vs {
            if c := cosine(vs[j], one[0]); c > best {
                best, bestJ = c, j
            }
        }
        check(fmt.Sprintf("index order %d", i), bestJ == i && same > 0.999,
            fmt.Sprintf("cosine(batch[%d], single)=%.6f, closest batch slot=%d", i, same, bestJ))
    }

    // Served model equals the configured one; a wrong configured model is refused.
    check("served model equals configured", err == nil && e.Model() == model, fmt.Sprintf("Model()=%q, reply accepted", e.Model()))
    wrong := newEmbedder("wrong/model")
    wv, werr := wrong.Generate(ctx, texts)
    check("wrong configured model refused", werr != nil && wv == nil && wrong.Dimensions() == 0,
        fmt.Sprintf("vectors=%v dims=%d err=%v", wv, wrong.Dimensions(), werr))

    // Query versus document.
    raw, err1 := e.Generate(ctx, []string{"what do graph databases store"})
    q, err2 := e.GenerateQuery(ctx, []string{"what do graph databases store"})
    if err1 != nil || err2 != nil {
        check("prefixed query differs", false, fmt.Sprintf("err1=%v err2=%v", err1, err2))
    } else {
        c := cosine(raw[0], q[0])
        check("prefixed query differs", c < 0.9999, fmt.Sprintf("cosine(raw, prefixed)=%.6f", c))
    }

    // Cancelled context.
    cctx, cancel := context.WithCancel(ctx)
    cancel()
    _, err = e.Generate(cctx, texts)
    check("cancelled context", errors.Is(err, cctx.Err()), fmt.Sprintf("err=%v ctx.Err()=%v", err, cctx.Err()))

    // A deadline shorter than one embed: time a large call, then rerun it with a tenth of that.
    big := make([]string, 64)
    for i := range big {
        big[i] = strings.Repeat(fmt.Sprintf("sentence %d about knowledge graphs and durable workflows. ", i), 8)
    }
    start := time.Now()
    _, err = e.Generate(ctx, big)
    took := time.Since(start)
    fmt.Printf("one embed of %d texts took %s err=%v\n", len(big), took.Round(time.Millisecond), err)
    budget := took / 10
    dctx, dcancel := context.WithTimeout(ctx, budget)
    start = time.Now()
    _, err = e.Generate(dctx, big)
    ended := time.Since(start)
    dcancel()
    check("deadline shorter than one embed", errors.Is(err, dctx.Err()) && errors.Is(err, context.DeadlineExceeded),
        fmt.Sprintf("budget=%s returned after %s err=%v ctx.Err()=%v", budget.Round(time.Millisecond), ended.Round(time.Millisecond), err, dctx.Err()))

    // Recovery across a container restart.
    docker("stop", "semengine-semembed")
    _, err = e.Generate(ctx, texts)
    check("call with provider stopped", err != nil && strings.Contains(err.Error(), "connection refused"), fmt.Sprintf("err=%v", err))
    docker("start", "semengine-semembed")
    // Wait until the provider answers again; each probe call is one attempt.
    attempts := 0
    for {
        attempts++
        vs, err = e.Generate(ctx, texts)
        if err == nil {
            break
        }
        if attempts == 1 || attempts%10 == 0 {
            fmt.Printf("attempt %d: %v\n", attempts, err)
        }
        if attempts > 600 {
            break
        }
        time.Sleep(500 * time.Millisecond)
    }
    check("call after restart", err == nil && len(vs) == len(texts), fmt.Sprintf("attempts=%d vectors=%d err=%v", attempts, len(vs), err))
    check("dimensions unchanged after restart", e.Dimensions() == 384, fmt.Sprintf("Dimensions()=%d", e.Dimensions()))

    if failed {
        fmt.Println("RESULT: FAIL")
        os.Exit(1)
    }
    fmt.Println("RESULT: PASS")
}
```

## Output

```text
PASS dimensions before first reply: Dimensions()=0
PASS multi-text call: texts=3 vectors=3 err=<nil>
PASS dimensions after first reply: Dimensions()=384
PASS index order 0: cosine(batch[0], single)=1.000000, closest batch slot=0
PASS index order 1: cosine(batch[1], single)=1.000000, closest batch slot=1
PASS index order 2: cosine(batch[2], single)=1.000000, closest batch slot=2
PASS served model equals configured: Model()="Snowflake/snowflake-arctic-embed-s", reply accepted
PASS wrong configured model refused: vectors=[] dims=0 err=httpembedder: reply is from model "Snowflake/snowflake-arctic-embed-s", configured "wrong/model"
PASS prefixed query differs: cosine(raw, prefixed)=0.855096
PASS cancelled context: err=httpembedder: Post "http://127.0.0.1:18081/v1/embeddings": context canceled ctx.Err()=context canceled
one embed of 64 texts took 596ms err=<nil>
PASS deadline shorter than one embed: budget=60ms returned after 61ms err=httpembedder: Post "http://127.0.0.1:18081/v1/embeddings": context deadline exceeded ctx.Err()=context deadline exceeded
docker stop semengine-semembed: semengine-semembed err=<nil>
PASS call with provider stopped: err=httpembedder: Post "http://127.0.0.1:18081/v1/embeddings": dial tcp 127.0.0.1:18081: connect: connection refused
docker start semengine-semembed: semengine-semembed err=<nil>
attempt 1: httpembedder: Post "http://127.0.0.1:18081/v1/embeddings": EOF
PASS call after restart: attempts=2 vectors=3 err=<nil>
PASS dimensions unchanged after restart: Dimensions()=384
RESULT: PASS
```

Exit status 0.

## What the run shows

- **Count and order.** A three-text call returned three vectors, and each matched (cosine 1.000000) the vector the
  same text got in a call of its own, in the same slot. semembed returned indexes `0, 1` in order for a two-text
  call made with `curl`, so this run does not exercise reordering; the unit tests do that against a fake.
- **Served model.** semembed ignores the `model` field of the request and always answers with the model it serves:
  a `curl` request naming `wrong/model` got vectors and `"model":"Snowflake/snowflake-arctic-embed-s"`. So the
  refusal of a wrongly configured model comes from the client, which compares the reply's `model` with its own; the
  wrong embedder learned no width.
- **Readiness.** `Dimensions()` was 0 before the first reply and 384 after it, and still 384 after the restart.
- **Query versus document.** The prefixed query vector and the raw one had cosine 0.855096.
- **Context.** A context cancelled before the call and a 60 ms deadline on a call that took 596 ms without one both
  ended the call with an error that `errors.Is` matches to `ctx.Err()`.
- **Recovery.** With the container stopped, the call failed with `connect: connection refused`. Right after
  `docker start`, the first call failed with `EOF` rather than `connection refused`: Docker's port forwarder accepted
  the connection before semembed was listening. The second call, half a second later, succeeded.
