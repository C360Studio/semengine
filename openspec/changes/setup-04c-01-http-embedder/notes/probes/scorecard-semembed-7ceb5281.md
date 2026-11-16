# Probe: the search scorecard through semembed 7ceb5281

This probe asks the word-search scorecard's 500 known-item questions of the same 2,000 sections three ways: by
embedding vectors, by the word index, and by the two ranked lists merged. A *known-item question* is a section's own
heading; its one right answer is the section under it, whose text does not hold the heading
(`graph/lexical/testdata/scorecard/README.md`). It runs outside `task verify`; nothing in verify reaches an outside
service. It is a measurement only: the merged row is not a merging design, and this note proposes no floor.

- **Provider:** semembed at commit `7ceb5281`, the container `semengine-semembed` (image
  `semengine-semembed:7ceb5281`) at `http://127.0.0.1:18081/v1`, built and run as
  [semembed-7ceb5281.md](semembed-7ceb5281.md) records.
- **Model:** `Snowflake/snowflake-arctic-embed-s`, the name semembed's reply reports (read from one raw request in
  the probe); 384 dimensions, from `Dimensions()` after the first reply.
- **Client:** `graph/embedding/httpembedder` through `httpembedder.New`, at commit
  `ae44896391d892816c0d8f42fbee9e1fa3157627` (main `1e4ce1a3` plus an empty commit).
- **Query prefix:** questions go through `GenerateQuery` with `QueryPrefix` set to
  `"Represent this sentence for searching relevant passages: "` (quoted to show the trailing space). Sections go
  through `Generate`, with no prefix. The raw row uses a second embedder with no `QueryPrefix`, so `GenerateQuery`
  sends the question as it is.
- **Text embedded per section:** the section's `name`, one space, then its `body`, from `corpus.jsonl.gz`. The word
  index sees the same two strings, as separate fields (`lexical.Document{Name, Body}`, as `corpusIndex` in
  `graph/lexical/scorecard_test.go` builds it); one space is how graph-embedding joins an entity's text values. No
  text was cut by the probe. Whether semembed cuts a long section to the model's input window was not measured here;
  bodies run to 4,000 bytes.
- **Batch size:** 32 texts per request, for sections and questions alike.
- **Ranking:** vectors rank all 2,000 sections by cosine similarity (each vector normalised to unit length, then a dot
  product). Words rank every hit `Index.Search` returns with `Limit` set to the index size, as `measure` in
  `scorecard_test.go` does; a section with no word in common is not in that list. Ties in either list, and in the
  merged list, break by entity ID.
- **Merged:** reciprocal rank fusion with k = 60. Each section scores the sum, over the two lists, of 1 / (60 + rank),
  rank counted from 1; a section absent from a list gets nothing from it. List depth: the whole of both lists, all
  2,000 vector ranks and every word hit.
- **Scores:** `R@1` is the share of questions whose answer is first, `R@10` the share in the first ten, `MRR` the
  average of 1 / rank (0 when the answer is absent), as the scorecard computes them.
- **Go:** `go version go1.26.11 darwin/arm64`
- **Run on:** 2026-11-16. The probe was a throwaway `main` package at `cmd/probescorecard/main.go` in the worktree,
  deleted after the run (SHA-256 `b103617de36b87666a03b7984681a79c62933b90c690b41822ec44878ba31bf8` before
  deletion); it is not in this repository.

Tabs in the fenced blocks below are replaced with spaces (four per tab), because the markdown lint forbids hard tabs.

## Results

| Row | R@1 | R@10 | MRR |
| --- | --- | --- | --- |
| Vectors alone (question with the prefix) | 22.8% | 47.6% | 0.310 |
| Words alone | 24.6% | 56.0% | 0.358 |
| Merged, reciprocal rank fusion, k = 60, full lists | 26.8% | 58.4% | 0.374 |
| Vectors alone (question raw, no prefix) | 13.8% | 31.2% | 0.200 |

**Sanity check:** the words-alone row reproduces `TestScorecardHoldsItsFloor`'s numbers on this commit (R@1 24.6%,
R@10 56.0%, MRR 0.358), so the probe loads the same corpus and questions and scores them the same way.

**Timings (wall clock, this Mac, one run):** 2,000 sections embedded in 2m6.337s; 500 questions in 2.047s with the
prefix and 1.667s raw. The whole `go run`, build included, took 2m12.57s.

## What the numbers say

The first reading, "this model's vectors lose to words", does not hold. An independent audit re-ran the probe
against the same container with its own HTTP calls and ranking, and reproduced all four rows exactly. It then found
two things holding the vector row back that are not the model
([audit on PR #680](https://github.com/C360Studio/semengine/pull/680#issuecomment-6916898569)).

1. **The file title in front of every section.** In this corpus `name` is the first heading of the section's
   *file*, not of the section, so every section of a file starts with the same words. Embedded with it, sibling
   sections look more alike than they are. Words barely notice. A real entity's name is its own, so this is a
   property of the test set, not of graph-embedding.

   | Row | R@1 | R@10 | MRR |
   | --- | --- | --- | --- |
   | Words alone (committed scorecard) | 24.6% | 56.0% | 0.358 |
   | Words, body only | 26.2% | 55.8% | 0.368 |
   | Vectors, `name + " " + body` (this probe) | 22.8% | 47.6% | 0.310 |
   | Vectors, body only | 28.2% | 52.0% | 0.362 |
   | Merged, this probe's vectors | 26.8% | 58.4% | 0.374 |
   | Merged, body-only vectors | 31.2% | 62.4% | 0.412 |

2. **semembed cuts every text at 512 tokens and says nothing.** Counted with the model's own tokenizer, 475 of the
   2,000 sections (23.8%) are longer than the window, and 23.1% of the corpus's tokens were never read. Split by
   whether the answer fits:

   | Body-only vectors against words | Questions | Vectors R@1 / R@10 / MRR | Words R@1 / R@10 / MRR |
   | --- | --- | --- | --- |
   | Answer fits the window | 389 | 32.6% / 57.6% / 0.409 | 25.4% / 57.3% / 0.369 |
   | Answer is longer | 111 | 12.6% / 32.4% / 0.197 | 21.6% / 51.4% / 0.320 |

   Where the model reads the whole answer, vectors match words in the first ten and are well ahead in first place.
   The whole shortfall is in the long answers. Splitting long bodies into pieces of at most 480 tokens did not win
   it back on this set (26.8% / 50.2% / 0.349 against 28.2% / 52.0% / 0.362 unsplit). The cut is filed on semembed
   as C360Studio/semembed#6, and SemEngine's side of long text is #667.

Leaving out the query prefix still costs a third of the vector score (R@10 47.6% to 31.2%). Merging is the strongest
row in every variant. These numbers hold for `Snowflake/snowflake-arctic-embed-s` on this corpus only; 500 questions
resolve differences of about 2 points, and smaller ones are noise.

## Command

Run from the repository root, with the container running:

```bash
go run ./cmd/probescorecard
```

## Output (verbatim)

```text
go: go1.26.11 darwin/arm64
corpus 2000 sections, 500 known-item questions
reply model: "Snowflake/snowflake-arctic-embed-s"
sections embedded: 2000 in 2m6.337s (batch 32)
dimensions: 384
queries embedded with prefix: 500 in 2.047s
queries embedded raw: 500 in 1.667s
vectors alone (prefixed query): R@1 22.8%  R@10 47.6%  MRR 0.310 over 500 questions
words alone:                    R@1 24.6%  R@10 56.0%  MRR 0.358 over 500 questions
merged, RRF k=60, full lists:   R@1 26.8%  R@10 58.4%  MRR 0.374 over 500 questions
vectors alone (raw query):      R@1 13.8%  R@10 31.2%  MRR 0.200 over 500 questions
```

The shell's `time` added: `go run ./cmd/probescorecard 2>&1  2.18s user 0.77s system 2% cpu 2:12.57 total`.

## Source (`cmd/probescorecard/main.go`, throwaway)

```go
// Command probescorecard is a throwaway probe: it runs the word-search scorecard's known-item
// questions through semembed's model, by vectors, by words, and merged, and prints R@1, R@10 and
// MRR for each. It is deleted after the run.
package main

import (
    "bufio"
    "bytes"
    "compress/gzip"
    "context"
    "encoding/json"
    "fmt"
    "math"
    "net/http"
    "os"
    "path/filepath"
    "runtime"
    "sort"
    "time"

    "github.com/c360studio/semengine/graph/embedding/httpembedder"
    "github.com/c360studio/semengine/graph/lexical"
)

const (
    baseURL   = "http://127.0.0.1:18081/v1"
    model     = "Snowflake/snowflake-arctic-embed-s"
    prefix    = "Represent this sentence for searching relevant passages: "
    batchSize = 32
    rrfK      = 60
)

var dir = filepath.Join("graph", "lexical", "testdata", "scorecard")

type section struct {
    ID   string `json:"id"`
    Path string `json:"path"`
    Name string `json:"name"`
    Body string `json:"body"`
}

type knownItem struct {
    Query string `json:"query"`
    ID    string `json:"id"`
}

type recall struct {
    questions, top1, top10 int
    reciprocal             float64
}

func (r *recall) add(rank int) { // rank is 0-based, -1 when the answer is not in the list
    r.questions++
    if rank < 0 {
        return
    }
    r.reciprocal += 1 / float64(rank+1)
    if rank == 0 {
        r.top1++
    }
    if rank < 10 {
        r.top10++
    }
}

func (r recall) String() string {
    return fmt.Sprintf("R@1 %.1f%%  R@10 %.1f%%  MRR %.3f over %d questions",
        100*float64(r.top1)/float64(r.questions), 100*float64(r.top10)/float64(r.questions),
        r.reciprocal/float64(r.questions), r.questions)
}

func must(err error) {
    if err != nil {
        fmt.Println("error:", err)
        os.Exit(2)
    }
}

func loadCorpus() []section {
    data, err := os.ReadFile(filepath.Join(dir, "corpus.jsonl.gz"))
    must(err)
    zr, err := gzip.NewReader(bytes.NewReader(data))
    must(err)
    var corpus []section
    sc := bufio.NewScanner(zr)
    sc.Buffer(nil, 1<<20)
    for sc.Scan() {
        var s section
        must(json.Unmarshal(sc.Bytes(), &s))
        corpus = append(corpus, s)
    }
    must(sc.Err())
    return corpus
}

// embedAll embeds texts in batches of batchSize through gen.
func embedAll(ctx context.Context, gen func(context.Context, []string) ([][]float32, error), texts []string) [][]float32 {
    out := make([][]float32, 0, len(texts))
    for i := 0; i < len(texts); i += batchSize {
        vs, err := gen(ctx, texts[i:min(i+batchSize, len(texts))])
        must(err)
        out = append(out, vs...)
    }
    return out
}

func normalize(v []float32) []float64 {
    var n float64
    for _, c := range v {
        n += float64(c) * float64(c)
    }
    n = math.Sqrt(n)
    out := make([]float64, len(v))
    for i, c := range v {
        out[i] = float64(c) / n
    }
    return out
}

type scored struct {
    id    string
    score float64
}

// ranked sorts by score, highest first, entity ID breaking ties, and returns the IDs.
func ranked(s []scored) []string {
    sort.Slice(s, func(i, j int) bool {
        if s[i].score != s[j].score {
            return s[i].score > s[j].score
        }
        return s[i].id < s[j].id
    })
    ids := make([]string, len(s))
    for i, x := range s {
        ids[i] = x.id
    }
    return ids
}

// byCosine ranks every section by cosine similarity to q.
func byCosine(q []float64, ids []string, docs [][]float64) []string {
    s := make([]scored, len(docs))
    for i, d := range docs {
        var dot float64
        for j := range d {
            dot += q[j] * d[j]
        }
        s[i] = scored{ids[i], dot}
    }
    return ranked(s)
}

// rrf merges ranked lists by reciprocal rank fusion: each list adds 1/(k + rank), rank from 1.
func rrf(lists ...[]string) []string {
    sum := map[string]float64{}
    for _, l := range lists {
        for i, id := range l {
            sum[id] += 1 / float64(rrfK+i+1)
        }
    }
    s := make([]scored, 0, len(sum))
    for id, v := range sum {
        s = append(s, scored{id, v})
    }
    return ranked(s)
}

func indexOf(l []string, id string) int {
    for i, x := range l {
        if x == id {
            return i
        }
    }
    return -1
}

func main() {
    ctx := context.Background()
    fmt.Println("go:", runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH)

    corpus := loadCorpus()
    var items []knownItem
    data, err := os.ReadFile(filepath.Join(dir, "known.json"))
    must(err)
    must(json.Unmarshal(data, &items))
    fmt.Printf("corpus %d sections, %d known-item questions\n", len(corpus), len(items))

    // The model name semembed's reply reports, read from one raw request.
    body, _ := json.Marshal(map[string]any{"model": model, "input": []string{"probe"}})
    resp, err := http.Post(baseURL+"/embeddings", "application/json", bytes.NewReader(body))
    must(err)
    var meta struct {
        Model string `json:"model"`
    }
    must(json.NewDecoder(resp.Body).Decode(&meta))
    _ = resp.Body.Close()
    fmt.Printf("reply model: %q\n", meta.Model)

    // Words: the scorecard's index, name and body as separate fields.
    ix := lexical.NewIndex()
    for _, s := range corpus {
        must(ix.Put(s.ID, lexical.Document{Name: s.Name, Body: s.Body}))
    }
    words := make([][]string, len(items))
    var wordRecall recall
    for i, it := range items {
        q, err := lexical.ParseQuery(it.Query)
        must(err)
        res, err := ix.Search(q, lexical.Options{Limit: ix.Len()})
        must(err)
        for _, h := range res.Hits {
            words[i] = append(words[i], h.EntityID)
        }
        wordRecall.add(indexOf(words[i], it.ID))
    }

    withPrefix, err := httpembedder.New(httpembedder.Config{BaseURL: baseURL, Model: model, QueryPrefix: prefix})
    must(err)
    raw, err := httpembedder.New(httpembedder.Config{BaseURL: baseURL, Model: model})
    must(err)

    // Sections: name, one space, body, as graph-embedding joins an entity's text values.
    ids := make([]string, len(corpus))
    texts := make([]string, len(corpus))
    for i, s := range corpus {
        ids[i] = s.ID
        texts[i] = s.Name + " " + s.Body
    }
    start := time.Now()
    docVecs := embedAll(ctx, withPrefix.Generate, texts)
    fmt.Printf("sections embedded: %d in %s (batch %d)\n", len(docVecs), time.Since(start).Round(time.Millisecond), batchSize)
    fmt.Printf("dimensions: %d\n", withPrefix.Dimensions())
    docs := make([][]float64, len(docVecs))
    for i, v := range docVecs {
        docs[i] = normalize(v)
    }

    queries := make([]string, len(items))
    for i, it := range items {
        queries[i] = it.Query
    }
    start = time.Now()
    qPrefixed := embedAll(ctx, withPrefix.GenerateQuery, queries)
    fmt.Printf("queries embedded with prefix: %d in %s\n", len(qPrefixed), time.Since(start).Round(time.Millisecond))
    start = time.Now()
    qRaw := embedAll(ctx, raw.GenerateQuery, queries)
    fmt.Printf("queries embedded raw: %d in %s\n", len(qRaw), time.Since(start).Round(time.Millisecond))

    var vecRecall, mergedRecall, rawRecall recall
    for i, it := range items {
        vec := byCosine(normalize(qPrefixed[i]), ids, docs)
        vecRecall.add(indexOf(vec, it.ID))
        mergedRecall.add(indexOf(rrf(vec, words[i]), it.ID))
        rawRecall.add(indexOf(byCosine(normalize(qRaw[i]), ids, docs), it.ID))
    }

    fmt.Println("vectors alone (prefixed query):", vecRecall)
    fmt.Println("words alone:                   ", wordRecall)
    fmt.Println("merged, RRF k=60, full lists:  ", mergedRecall)
    fmt.Println("vectors alone (raw query):     ", rawRecall)
}
```
