package lexical

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/c360studio/semengine/graph"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// NameWeight is how many times a word in a document's Name counts against
// the same word in its Body, both in term frequency and in length.
const NameWeight = 2

// BM25's customary parameters, as Lucene sets them.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

var (
	// ErrInvalidEntityID is returned by Put for an ID that is not a canonical
	// six-part entity ID, so every held entity can be matched by scope.
	ErrInvalidEntityID = errors.New("lexical: invalid entity ID")
	// ErrInvalidScope is returned by Search for a malformed scope prefix.
	ErrInvalidScope = errors.New("lexical: invalid scope prefix")
	// ErrInvalidLimit is returned by Search for a limit below 1.
	ErrInvalidLimit = errors.New("lexical: limit must be at least 1")
)

// Document is the text an entity is found by. Words in Name weigh
// NameWeight times as much as words in Body.
type Document struct {
	Name string
	Body string
}

// Options bound a search. An empty Scope searches every entity; otherwise
// only entities under one of its ID prefixes are matched (an empty prefix
// matches every entity). Limit is the most hits returned, at least 1.
type Options struct {
	Scope []string
	Limit int
}

// Hit is one entity a search found, with its score.
type Hit struct {
	EntityID string
	Score    float64
}

// Result is a search's answer: the best hits, ordered by score and then by
// entity ID, and the number of entities matched in scope before the limit.
type Result struct {
	Hits    []Hit
	Matched int
}

// Index ranks the entities it holds by field-weighted BM25 over their words.
// It is safe for concurrent use: a search sees each Put or Delete whole or
// not at all.
type Index struct {
	mu       sync.RWMutex
	entities map[string]*entity
	// postings maps each word to the entities holding it and the word's
	// weighted term frequency in each.
	postings map[string]map[string]int
	// totalLength is the sum of every held entity's weighted length. It is
	// an integer, so it is exact whatever order the writes came in.
	totalLength int
}

// entity is what the index holds for one entity: its words in order, per
// field, which a write needs to remove the entity's postings and a phrase is
// checked against.
type entity struct {
	name   []string
	body   []string
	length int
}

// NewIndex returns an empty index.
func NewIndex() *Index {
	return &Index{
		entities: make(map[string]*entity),
		postings: make(map[string]map[string]int),
	}
}

// Put replaces everything entityID held with the words of doc. A document
// with no words removes the entity. An ID that is not a canonical entity ID
// is refused with an error matching ErrInvalidEntityID.
func (ix *Index) Put(entityID string, doc Document) error {
	if err := semtypes.ValidateEntityID(entityID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEntityID, err)
	}
	name, body := Tokens(doc.Name), Tokens(doc.Body)

	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.remove(entityID)
	if len(name) == 0 && len(body) == 0 {
		return nil
	}
	e := &entity{name: name, body: body, length: NameWeight*len(name) + len(body)}
	ix.entities[entityID] = e
	ix.totalLength += e.length
	tf := make(map[string]int, len(name)+len(body))
	for _, w := range name {
		tf[w] += NameWeight
	}
	for _, w := range body {
		tf[w]++
	}
	for w, n := range tf {
		holders := ix.postings[w]
		if holders == nil {
			holders = make(map[string]int)
			ix.postings[w] = holders
		}
		holders[entityID] = n
	}
	return nil
}

// Delete removes entityID. It does nothing for an entity the index does not
// hold.
func (ix *Index) Delete(entityID string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.remove(entityID)
}

// remove drops entityID and its postings. The caller holds the write lock.
func (ix *Index) remove(entityID string) {
	e, ok := ix.entities[entityID]
	if !ok {
		return
	}
	for _, field := range [][]string{e.name, e.body} {
		for _, w := range field {
			holders := ix.postings[w]
			delete(holders, entityID)
			if len(holders) == 0 {
				delete(ix.postings, w)
			}
		}
	}
	ix.totalLength -= e.length
	delete(ix.entities, entityID)
}

// Len is the number of entities the index holds.
func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.entities)
}

// Search scores every entity in scope that holds one of the query's plain or
// phrase words, every one of its phrases and none of its excluded terms, and
// returns the best opts.Limit of them. A score sums BM25F over the distinct
// plain and phrase words; excluded words never add to it. The entity count,
// each word's entity count and the average length are those of the entities
// held when the search runs; scope filters before the limit and never changes
// a score.
func (ix *Index) Search(q Query, opts Options) (Result, error) {
	if opts.Limit < 1 {
		return Result{}, fmt.Errorf("%w: got %d", ErrInvalidLimit, opts.Limit)
	}
	for _, prefix := range opts.Scope {
		if prefix == "" {
			continue
		}
		if err := semtypes.ValidateEntityIDPrefix(prefix); err != nil {
			return Result{}, fmt.Errorf("%w %q: %w", ErrInvalidScope, prefix, err)
		}
	}

	ix.mu.RLock()
	defer ix.mu.RUnlock()
	if len(ix.entities) == 0 {
		return Result{}, nil
	}
	n := float64(len(ix.entities))
	avgLength := float64(ix.totalLength) / n
	scores := make(map[string]float64)
	for _, w := range q.scored() {
		holders := ix.postings[w]
		if len(holders) == 0 {
			continue
		}
		df := float64(len(holders))
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for id, weighted := range holders {
			tf := float64(weighted)
			norm := 1 - bm25B + bm25B*float64(ix.entities[id].length)/avgLength
			scores[id] += idf * tf * (bm25K1 + 1) / (tf + bm25K1*norm)
		}
	}

	var hits []Hit
	for id, score := range scores {
		if graph.MatchesAnyIDPrefix(id, opts.Scope) && ix.admits(id, q) {
			hits = append(hits, Hit{EntityID: id, Score: score})
		}
	}
	slices.SortFunc(hits, func(a, b Hit) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return cmp.Compare(a.EntityID, b.EntityID)
	})
	matched := len(hits)
	if len(hits) > opts.Limit {
		hits = hits[:opts.Limit]
	}
	return Result{Hits: hits, Matched: matched}, nil
}

// admits reports whether entity id holds every phrase of q and none of its
// excluded terms. The caller holds the read lock.
func (ix *Index) admits(id string, q Query) bool {
	for _, p := range q.phrases {
		if !ix.holdsRun(id, p) {
			return false
		}
	}
	for _, x := range q.excluded {
		if ix.holdsRun(id, x) {
			return false
		}
	}
	return true
}

// holdsRun reports whether entity id holds run's words next to each other, in
// order, within its Name or within its Body. The postings narrow it first, so
// only an entity holding every word has its fields scanned.
func (ix *Index) holdsRun(id string, run []string) bool {
	for _, w := range run {
		if _, ok := ix.postings[w][id]; !ok {
			return false
		}
	}
	e := ix.entities[id]
	return containsRun(e.name, run) || containsRun(e.body, run)
}

func containsRun(field, run []string) bool {
	for i := 0; i+len(run) <= len(field); i++ {
		if slices.Equal(field[i:i+len(run)], run) {
			return true
		}
	}
	return false
}
