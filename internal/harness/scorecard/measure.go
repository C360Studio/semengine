package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/embedding/httpembedder"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/lifecycle"
	graphembedding "github.com/c360studio/semengine/processor/graph-embedding"
	graphindex "github.com/c360studio/semengine/processor/graph-index"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	graphlexical "github.com/c360studio/semengine/processor/graph-lexical"
	graphquery "github.com/c360studio/semengine/processor/graph-query"
	"github.com/c360studio/semengine/types"
)

// scorecardDir is the word scorecard's frozen corpus and questions, read from the module root,
// where Task runs the program, so both scorecards ask the same questions of the same text.
var scorecardDir = filepath.Join("graph", "lexical", "testdata", "scorecard")

const (
	// The predicates a section is written with, the word scorecard's: graph-lexical's default rule
	// reads a .title value as the entity's name and a .content value as its body.
	sectionTitle   = "doc.section.title"
	sectionContent = "doc.section.content"
	// embeddedSuffix is the one text suffix graph-embedding embeds. In this test set a section's
	// title is its file's first heading, shared by every section of the file, so embedding it
	// makes sibling sections look more alike than they are; a real entity's name is its own.
	embeddedSuffix = ".content"
	// embedderName is the name the HTTP embedder is registered under, graph-embedding's
	// embedder_type.
	embedderName = "http"
	// requestTimeout bounds each request to the composed components.
	requestTimeout = time.Minute
	// stopBudget bounds each cleanup's Stop or Close: cleanups run after the run's context ended.
	stopBudget = time.Minute
)

// sectionType is the message type the sections are written under; the payload registry knows it
// by a schema-less stub, as graph-ingest needs a registered type for each entity.
var sectionType = message.Type{Domain: "scorecard", Category: "section", Version: "v1"}

type sectionStub struct{}

type section struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
	Body string `json:"body"`
}

// question is a section heading asked as a question; the answer is the section under it, whose
// text does not hold the heading.
type question struct {
	Query string `json:"query"`
	ID    string `json:"id"`
}

// rows are the three ways a question is answered: globalSearch (words), the semantic verb
// (vectors) and searchGraph (merged).
type rows struct {
	words, vectors, merged *tally
}

func loadScorecard() ([]section, []question, error) {
	data, err := os.ReadFile(filepath.Join(scorecardDir, "corpus.jsonl.gz"))
	if err != nil {
		return nil, nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	var corpus []section
	sc := bufio.NewScanner(zr)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var s section
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			return nil, nil, fmt.Errorf("corpus line %d: %w", len(corpus)+1, err)
		}
		corpus = append(corpus, s)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	raw, err := os.ReadFile(filepath.Join(scorecardDir, "known.json"))
	if err != nil {
		return nil, nil, err
	}
	var questions []question
	if err := json.Unmarshal(raw, &questions); err != nil {
		return nil, nil, err
	}
	if len(corpus) != 2000 || len(questions) != 500 {
		return nil, nil, fmt.Errorf("the scorecard holds %d sections and %d questions, want 2000 and 500", len(corpus), len(questions))
	}
	return corpus, questions, nil
}

// replyModel is the model the provider's reply names for one plain request, or "none" when the
// reply names none.
func replyModel(ctx context.Context, opts options) (string, error) {
	body, err := json.Marshal(map[string]any{"model": opts.model, "input": []string{"scorecard"}})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.url+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("provider answered %s: %s", resp.Status, msg)
	}
	var reply struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return "", err
	}
	if reply.Model == "" {
		return "none", nil
	}
	return reply.Model, nil
}

// measure composes graph-ingest, graph-index, graph-lexical, graph-embedding over the provider
// and graph-query (tier 1) on a NATS server of its own, writes the sections, waits until both
// indexes hold every one, and asks each question through globalSearch, the semantic verb and
// searchGraph. What it starts is stopped by tb's cleanups.
func measure(ctx context.Context, opts options, tb *programTB, out io.Writer) (rows, error) {
	corpus, questions, err := loadScorecard()
	if err != nil {
		return rows{}, fmt.Errorf("load the scorecard: %w", err)
	}
	fmt.Fprintln(out, "go:", runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH)
	fmt.Fprintf(out, "commit: %s (tree %s)\n", opts.commit, opts.tree)
	fmt.Fprintf(out, "provider: %s (%s)\n", opts.url, opts.providerNote())
	fmt.Fprintf(out, "model asked for: %q\n", opts.model)
	named, err := replyModel(ctx, opts)
	if err != nil {
		return rows{}, fmt.Errorf("ask the provider which model answers: %w", err)
	}
	fmt.Fprintf(out, "model the reply names: %q\n", named)
	fmt.Fprintf(out, "query prefix: %q\n", opts.queryPrefix)
	fmt.Fprintf(out, "embedded text: values of predicates ending %q (section bodies)\n", embeddedSuffix)
	fmt.Fprintf(out, "corpus: %d sections, %d questions, each asked %d deep\n", len(corpus), len(questions), depth)

	s, err := compose(ctx, opts, tb)
	if err != nil {
		return rows{}, err
	}
	started := time.Now()
	last, err := s.write(ctx, corpus)
	if err != nil {
		return rows{}, err
	}
	if err := s.awaitWords(ctx, last); err != nil {
		return rows{}, err
	}
	words := time.Since(started)
	dims, err := s.awaitVectors(ctx, corpus)
	if err != nil {
		return rows{}, err
	}
	vectors := time.Since(started)
	byBody, err := s.awaitSemanticCopy(ctx, corpus)
	if err != nil {
		return rows{}, err
	}
	fmt.Fprintf(out, "sections: written and in the word index in %s, in the vector index in %s; %d dimensions\n",
		words.Round(time.Millisecond), vectors.Round(time.Millisecond), dims)
	fmt.Fprintf(out, "sections the semantic verb answered only when asked by their own body: %d\n", byBody)

	started = time.Now()
	r, err := s.ask(ctx, questions)
	if err != nil {
		return rows{}, err
	}
	fmt.Fprintf(out, "questions: %d in %s, each through all three verbs\n", len(questions), time.Since(started).Round(time.Millisecond))
	return r, nil
}

// stack is the composed components on one connection.
type stack struct {
	client    *natsclient.Client
	mutations *graphmutation.Client
}

func compose(ctx context.Context, opts options, tb *programTB) (stack, error) {
	// Own server: the program is not a test and has no package's shared server to reset.
	fixture := natsfixture.New(tb)
	if err := fixture.Start(ctx); err != nil {
		return stack{}, fmt.Errorf("start NATS: %w", err)
	}
	client, err := natsclient.NewClient(fixture.URL(),
		natsclient.WithTimeout(15*time.Second), natsclient.WithMaxReconnects(0))
	if err != nil {
		return stack{}, err
	}
	tb.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), stopBudget)
		defer cancel()
		if err := client.Close(stopCtx); err != nil {
			tb.Errorf("close the NATS client: %v", err)
		}
	})
	if err := client.Connect(ctx); err != nil {
		return stack{}, fmt.Errorf("connect to NATS: %w", err)
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return stack{}, err
	}
	js, err := client.JetStream()
	if err != nil {
		return stack{}, err
	}
	if _, err := natsfixture.CreateStream(ctx, js, jetstream.FileStorage, "ENTITY", "entity.>"); err != nil {
		return stack{}, fmt.Errorf("create the ENTITY stream: %w", err)
	}

	payloads := payloadregistry.New()
	for _, register := range []func(*payloadregistry.Registry) error{message.RegisterPayloads, inference.RegisterPayloads, lifecycle.RegisterPayloads} {
		if err := register(payloads); err != nil {
			return stack{}, err
		}
	}
	if err := payloads.Register(&payloadregistry.Registration{
		Domain: sectionType.Domain, Category: sectionType.Category, Version: sectionType.Version,
		Description: "a scorecard section", Factory: func() any { return &sectionStub{} },
	}); err != nil {
		return stack{}, err
	}
	ingestConfig, err := json.Marshal(graphingest.DefaultConfig())
	if err != nil {
		return stack{}, err
	}
	deps := func() component.Dependencies {
		return component.Dependencies{NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry()}
	}
	ingestDeps := deps()
	ingestDeps.PayloadRegistry = payloads
	ingestDeps.Platform = types.PlatformMeta{Org: "c360", Platform: "semengine"}

	embedders := component.NewRegistry()
	if err := graphembedding.RegisterWithEmbedder(embedders, embedderName, func() (embedding.Embedder, error) {
		return httpembedder.New(opts.embedderConfig())
	}); err != nil {
		return stack{}, err
	}
	createEmbedding := func(raw json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
		return embedders.CreateComponent("graph-embedding", types.ComponentConfig{
			Type: "processor", Name: "graph-embedding", Enabled: true, Config: raw,
		}, deps, nil)
	}

	for _, c := range []struct {
		name   string
		create func(json.RawMessage, component.Dependencies) (component.Discoverable, error)
		config string
		deps   component.Dependencies
	}{
		{"graph-ingest", graphingest.CreateGraphIngest, string(ingestConfig), ingestDeps},
		{"graph-index", graphindex.CreateGraphIndex, `{}`, deps()},
		{"graph-lexical", graphlexical.CreateGraphLexical, `{}`, deps()},
		{"graph-embedding", createEmbedding, `{"text_suffixes": ["` + embeddedSuffix + `"]}`, deps()},
		{"graph-query", graphquery.CreateGraphQuery, `{"embedding_enabled": true}`, deps()},
	} {
		built, err := c.create(json.RawMessage(c.config), c.deps)
		if err != nil {
			return stack{}, fmt.Errorf("create %s: %w", c.name, err)
		}
		if err := start(ctx, tb, c.name, built); err != nil {
			return stack{}, err
		}
	}
	mutations, err := graphmutation.NewClient(client, requestTimeout)
	if err != nil {
		return stack{}, err
	}
	return stack{client: client, mutations: mutations}, nil
}

// start initializes and starts one component and registers its Stop as a cleanup, so the
// components stop in the reverse of the order they started.
func start(ctx context.Context, tb *programTB, name string, built component.Discoverable) error {
	owner, ok := built.(component.LifecycleComponent)
	if !ok {
		return fmt.Errorf("%s has no lifecycle", name)
	}
	if err := owner.Initialize(); err != nil {
		return fmt.Errorf("initialize %s: %w", name, err)
	}
	if err := owner.Start(ctx); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	tb.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), stopBudget)
		defer cancel()
		if err := owner.Stop(stopCtx); err != nil {
			tb.Errorf("stop %s: %v", name, err)
		}
	})
	return nil
}

// write creates every section through graph-ingest and returns the last write's revision.
func (s stack) write(ctx context.Context, corpus []section) (uint64, error) {
	var last uint64
	now := time.Now()
	for _, sec := range corpus {
		created, err := s.mutations.Create(ctx, graph.CreateEntityRequest{
			Entity: &graph.EntityState{ID: sec.ID, MessageType: sectionType, UpdatedAt: now},
			Triples: []message.Triple{
				{Subject: sec.ID, Predicate: sectionTitle, Object: sec.Name, Source: "scorecard", Timestamp: now, Confidence: 1},
				{Subject: sec.ID, Predicate: sectionContent, Object: sec.Body, Source: "scorecard", Timestamp: now, Confidence: 1},
			},
		})
		if err != nil {
			return 0, fmt.Errorf("create %s: %w", sec.ID, err)
		}
		last = created.KVRevision
	}
	return last, nil
}

func verbSubject(responder, name string) (string, error) {
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == responder && verb.Name == name {
			return verb.Subject, nil
		}
	}
	return "", fmt.Errorf("the verb table declares no %s for %s", name, responder)
}

// request asks one verb and returns its reply's data decoded into data.
func (s stack) request(ctx context.Context, responder, verb string, request, data any) error {
	subject, err := verbSubject(responder, verb)
	if err != nil {
		return err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	raw, err := s.client.RequestClassified(ctx, subject, body, requestTimeout)
	if err != nil {
		return err
	}
	reply := graph.QueryResponse[json.RawMessage]{}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return fmt.Errorf("%s reply: %w", verb, err)
	}
	if data == nil {
		return nil
	}
	if err := json.Unmarshal(reply.Data, data); err != nil {
		return fmt.Errorf("%s reply data: %w", verb, err)
	}
	return nil
}

// awaitWords waits until graph-index and the word index have applied revision: until then each
// refuses a request naming it as its min_revision, and the first answer is the event. Any other
// refusal ends the run.
func (s stack) awaitWords(ctx context.Context, revision uint64) error {
	waits := []struct {
		responder, verb string
		request         any
	}{
		{graph.ResponderGraphIndex, "alias", map[string]any{"alias": "nobody-holds-this", "min_revision": revision}},
		{graph.ResponderGraphQuery, "globalSearch", graph.GlobalSearchRequest{
			Query: "nothing", Limit: 1, MinRevisionField: graph.MinRevisionField{MinRevision: revision},
		}},
	}
	for _, w := range waits {
		if _, err := awaitAnswer(ctx, readyBudget, func(ctx context.Context) (bool, error) {
			err := s.request(ctx, w.responder, w.verb, w.request, nil)
			return err == nil, err
		}, func(answered bool) bool { return answered }); err != nil {
			return fmt.Errorf("await revision %d in %s: %w", revision, w.responder, err)
		}
	}
	return nil
}

// awaitVectors watches EMBEDDING_INDEX until every section has a record this embedder generated,
// and returns their width. A failed record ends the wait naming its reason.
func (s stack) awaitVectors(ctx context.Context, corpus []section) (int, error) {
	index, err := kvcatalog.OpenCatalogReader(ctx, s.client, graph.BucketEmbeddingIndex)
	if err != nil {
		return 0, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	watcher, err := index.WatchAll(watchCtx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = watcher.Stop() }()

	wanted := make(map[string]bool, len(corpus))
	for _, sec := range corpus {
		wanted[sec.ID] = true
	}
	generated := map[string]bool{}
	dims := 0
	for len(generated) < len(wanted) {
		var entry jetstream.KeyValueEntry
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("await vectors (%d of %d generated): %w", len(generated), len(wanted), ctx.Err())
		case e, ok := <-watcher.Updates():
			if !ok {
				return 0, fmt.Errorf("the %s watch closed with %d of %d generated", graph.BucketEmbeddingIndex, len(generated), len(wanted))
			}
			entry = e
		}
		if entry == nil { // the end of the values held when the watch started
			continue
		}
		if entry.Operation() != jetstream.KeyValuePut {
			delete(generated, entry.Key())
			continue
		}
		var record embedding.Record
		if err := json.Unmarshal(entry.Value(), &record); err != nil {
			return 0, fmt.Errorf("record %s: %w", entry.Key(), err)
		}
		if !wanted[record.EntityID] {
			continue
		}
		switch {
		case record.Status == embedding.StatusFailed:
			return 0, fmt.Errorf("embedding %s failed (%s): %s", record.EntityID, record.Reason, record.ErrorMsg)
		case record.Status == embedding.StatusGenerated && record.EmbedderType == embedderName:
			generated[record.EntityID] = true
			dims = record.Dimensions
		default:
			delete(generated, record.EntityID)
		}
	}
	return dims, nil
}

// awaitSemanticCopy waits until the semantic verb answers every section: it answers from its own
// copy of EMBEDDING_INDEX, kept by a watch of its own, which can trail the one awaitVectors read. A
// hundred sections at a time are asked for by one plain question scoped to their IDs; the verb
// leaves out a vector scoring 0 or less, so a section missing from that answer is then asked for
// by its own body, scoped to it alone, until an answer holds it. It returns how many were. Only an
// index_not_ready refusal or an answer still missing the section is asked again (awaitAnswer); a
// failure of the provider embedding the question ends the run.
func (s stack) awaitSemanticCopy(ctx context.Context, corpus []section) (int, error) {
	byBody := 0
	for first := 0; first < len(corpus); first += depth {
		batch := corpus[first:min(first+depth, len(corpus))]
		scope := make([]string, len(batch))
		for i, sec := range batch {
			scope[i] = sec.ID
		}
		// Until graph-embedding is ready the verb refuses; its first answer is the event.
		request := graph.SemanticRequest{Query: "section", Limit: len(scope), Scope: scope}
		data, err := awaitAnswer(ctx, readyBudget, func(ctx context.Context) (graph.SemanticData, error) {
			var data graph.SemanticData
			return data, s.request(ctx, graph.ResponderGraphQuery, "semantic", request, &data)
		}, func(graph.SemanticData) bool { return true })
		if err != nil {
			return 0, fmt.Errorf("semantic scoped to sections %d to %d: %w", first+1, first+len(batch), err)
		}
		found := make(map[string]bool, len(data.Results))
		for _, hit := range data.Results {
			found[hit.EntityID] = true
		}
		for _, sec := range batch {
			if found[sec.ID] {
				continue
			}
			byBody++
			request := graph.SemanticRequest{Query: sec.Body, Limit: 1, Scope: []string{sec.ID}}
			if _, err := awaitAnswer(ctx, readyBudget, func(ctx context.Context) (int, error) {
				var one graph.SemanticData
				err := s.request(ctx, graph.ResponderGraphQuery, "semantic", request, &one)
				return len(one.Results), err
			}, func(n int) bool { return n == 1 }); err != nil {
				return 0, fmt.Errorf("await %s in the semantic answers: %w", sec.ID, err)
			}
		}
	}
	return byBody, nil
}

// ask puts every question to globalSearch, the semantic verb and searchGraph, depth deep, and
// counts where each put the answer. searchGraph's own reply is scored: a reply whose merge was
// not honoured, or a semantic reply that left records out, ends the run, since its row would not
// measure what it names.
func (s stack) ask(ctx context.Context, questions []question) (rows, error) {
	r := rows{
		words:   &tally{name: "words, globalSearch"},
		vectors: &tally{name: "vectors, semantic"},
		merged:  &tally{name: "merged, searchGraph"},
	}
	for _, q := range questions {
		search := graph.GlobalSearchRequest{Query: q.Query, Limit: depth}
		var global graph.GlobalSearchData
		if err := s.request(ctx, graph.ResponderGraphQuery, "globalSearch", search, &global); err != nil {
			return rows{}, fmt.Errorf("globalSearch %q: %w", q.Query, err)
		}
		if global.Text == nil {
			return rows{}, fmt.Errorf("globalSearch %q: no text in the reply", q.Query)
		}
		r.words.add(matchRank(matchIDs(global.Text.Matches), q.ID))

		var semantic graph.SemanticData
		if err := s.request(ctx, graph.ResponderGraphQuery, "semantic", graph.SemanticRequest{Query: q.Query, Limit: depth}, &semantic); err != nil {
			return rows{}, fmt.Errorf("semantic %q: %w", q.Query, err)
		}
		if semantic.FailedRecords != 0 || semantic.UnreadableRecords != 0 {
			return rows{}, fmt.Errorf("semantic %q: %d failed and %d unreadable records left out", q.Query, semantic.FailedRecords, semantic.UnreadableRecords)
		}
		ids := make([]string, len(semantic.Results))
		for i, hit := range semantic.Results {
			ids[i] = hit.EntityID
		}
		r.vectors.add(matchRank(ids, q.ID))

		var merged graph.GlobalSearchData
		if err := s.request(ctx, graph.ResponderGraphQuery, "searchGraph", search, &merged); err != nil {
			return rows{}, fmt.Errorf("searchGraph %q: %w", q.Query, err)
		}
		if merged.Merge == nil || !merged.Merge.Honoured || merged.Text == nil {
			return rows{}, fmt.Errorf("searchGraph %q did not merge: %+v", q.Query, merged.Merge)
		}
		r.merged.add(matchRank(matchIDs(merged.Text.Matches), q.ID))
	}
	return r, nil
}

func matchIDs(matches []graph.TextSearchMatch) []string {
	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m.ID
	}
	return ids
}
