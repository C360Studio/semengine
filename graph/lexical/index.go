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
//
// It holds each entity and each word once, under a number, and refers to them
// everywhere else by that number: four bytes in place of a string.
type Index struct {
	mu sync.RWMutex
	// entityNum maps each held entity's ID to its number in entities.
	entityNum map[string]uint32
	entities  numbered[entity]
	// wordNum maps each held word to its number in words. A word is held
	// while at least one entity holds it.
	wordNum map[string]uint32
	words   numbered[word]
	// totalLength is the sum of every held entity's weighted length. It is
	// an integer, so it is exact whatever order the writes came in.
	totalLength int
}

// entity is what the index holds for one entity: its words in order, per
// field, which a write needs to remove the entity's postings and a phrase is
// checked against.
type entity struct {
	id     string
	name   []uint32
	body   []uint32
	length int
}

// word is one word the index holds and its postings: the entities holding it
// and the word's weighted term frequency in each.
type word struct {
	text    string
	holders map[uint32]int32
}

// numbered holds values under small numbers, reusing a removed value's number
// for the next one added, so there are never more numbers than the most values
// held at once.
type numbered[T any] struct {
	// items holds each value at its number; a removed value's place holds
	// the zero value until the number is reused.
	items []T
	free  []uint32
}

func (n *numbered[T]) add(v T) uint32 {
	if last := len(n.free) - 1; last >= 0 {
		i := n.free[last]
		n.free = n.free[:last]
		n.items[i] = v
		return i
	}
	n.items = append(n.items, v)
	return uint32(len(n.items) - 1)
}

func (n *numbered[T]) remove(i uint32) {
	var zero T
	n.items[i] = zero
	n.free = append(n.free, i)
}

// NewIndex returns an empty index.
func NewIndex() *Index {
	return &Index{
		entityNum: make(map[string]uint32),
		wordNum:   make(map[string]uint32),
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
	e := entity{
		id:     entityID,
		name:   ix.number(name),
		body:   ix.number(body),
		length: NameWeight*len(name) + len(body),
	}
	tf := make(map[uint32]int32, len(e.name)+len(e.body))
	for _, w := range e.name {
		tf[w] += NameWeight
	}
	for _, w := range e.body {
		tf[w]++
	}
	num := ix.entities.add(e)
	ix.entityNum[entityID] = num
	ix.totalLength += e.length
	for w, n := range tf {
		ix.words.items[w].holders[num] = n
	}
	return nil
}

// number returns the numbers of words, numbering each word the index does
// not yet hold. The caller holds the write lock.
func (ix *Index) number(words []string) []uint32 {
	nums := make([]uint32, len(words))
	for i, w := range words {
		n, ok := ix.wordNum[w]
		if !ok {
			n = ix.words.add(word{text: w, holders: make(map[uint32]int32)})
			ix.wordNum[w] = n
		}
		nums[i] = n
	}
	return nums
}

// lookup returns the numbers of run's words, and false when the index holds
// no entity with one of them.
func (ix *Index) lookup(run []string) ([]uint32, bool) {
	nums := make([]uint32, len(run))
	for i, w := range run {
		n, ok := ix.wordNum[w]
		if !ok {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
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
	num, ok := ix.entityNum[entityID]
	if !ok {
		return
	}
	e := ix.entities.items[num]
	for _, field := range [][]uint32{e.name, e.body} {
		for _, n := range field {
			w := &ix.words.items[n]
			if w.holders == nil {
				continue // an earlier repeat of this word removed it
			}
			delete(w.holders, num)
			if len(w.holders) == 0 {
				delete(ix.wordNum, w.text)
				ix.words.remove(n)
			}
		}
	}
	ix.totalLength -= e.length
	delete(ix.entityNum, entityID)
	ix.entities.remove(num)
}

// Len is the number of entities the index holds.
func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.entityNum)
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
	if len(ix.entityNum) == 0 {
		return Result{}, nil
	}
	var phrases, excluded [][]uint32
	for _, p := range q.phrases {
		nums, ok := ix.lookup(p)
		if !ok {
			return Result{}, nil // no entity holds every word of this phrase
		}
		phrases = append(phrases, nums)
	}
	for _, x := range q.excluded {
		// An excluded term with a word no entity holds removes nothing.
		if nums, ok := ix.lookup(x); ok {
			excluded = append(excluded, nums)
		}
	}

	n := float64(len(ix.entityNum))
	avgLength := float64(ix.totalLength) / n
	scores := make(map[uint32]float64)
	for _, text := range q.scored() {
		w, ok := ix.wordNum[text]
		if !ok {
			continue
		}
		holders := ix.words.items[w].holders
		df := float64(len(holders))
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for num, weighted := range holders {
			tf := float64(weighted)
			norm := 1 - bm25B + bm25B*float64(ix.entities.items[num].length)/avgLength
			scores[num] += idf * tf * (bm25K1 + 1) / (tf + bm25K1*norm)
		}
	}

	var hits []Hit
	for num, score := range scores {
		id := ix.entities.items[num].id
		if graph.MatchesAnyIDPrefix(id, opts.Scope) && ix.admits(num, phrases, excluded) {
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

// admits reports whether entity num holds every one of phrases and none of
// excluded. The caller holds the read lock.
func (ix *Index) admits(num uint32, phrases, excluded [][]uint32) bool {
	for _, p := range phrases {
		if !ix.holdsRun(num, p) {
			return false
		}
	}
	for _, x := range excluded {
		if ix.holdsRun(num, x) {
			return false
		}
	}
	return true
}

// holdsRun reports whether entity num holds run's words next to each other,
// in order, within its Name or within its Body. The postings narrow it first,
// so only an entity holding every word has its fields scanned.
func (ix *Index) holdsRun(num uint32, run []uint32) bool {
	for _, w := range run {
		if _, ok := ix.words.items[w].holders[num]; !ok {
			return false
		}
	}
	e := &ix.entities.items[num]
	return containsRun(e.name, run) || containsRun(e.body, run)
}

func containsRun(field, run []uint32) bool {
	for i := 0; i+len(run) <= len(field); i++ {
		if slices.Equal(field[i:i+len(run)], run) {
			return true
		}
	}
	return false
}
