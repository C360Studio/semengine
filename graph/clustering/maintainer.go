package clustering

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/types"
)

// levels is the number of group levels: 0 org.platform.system.domain, 1 the source
// org.platform.system, 2 the deployment org.platform (ADR-102 d6).
const levels = 3

// ErrBootstrapIncomplete is returned by Tick before BootstrapDone: a record built from a
// partial replay would be published as if it were the graph.
var ErrBootstrapIncomplete = errors.New("clustering: bootstrap incomplete")

// IncompleteError is returned by a Tick that published every group while some entities
// stay refused: the records leave those entities out.
type IncompleteError struct {
	// Refused are the refused entity IDs, sorted.
	Refused []string
}

// maxNamedRefusals bounds how many refused IDs Error names; Refused holds them all.
const maxNamedRefusals = 10

func (e *IncompleteError) Error() string {
	named := e.Refused[:min(len(e.Refused), maxNamedRefusals)]
	msg := fmt.Sprintf("clustering: %d refused entities left out of the records: %s", len(e.Refused), strings.Join(named, ", "))
	if rest := len(e.Refused) - len(named); rest > 0 {
		msg += fmt.Sprintf(" and %d more", rest)
	}
	return msg
}

// Maintainer keeps one group per entity-ID prefix at each level current from entity
// changes, and publishes the changed groups' records on each Tick. A caller feeds it one
// ordered watch of the entity bucket: Upsert and Delete for each delivery, in increasing
// revision order per key, then BootstrapDone when the initial replay ends. Its methods
// may be called from the watch goroutine and a ticker goroutine at once.
type Maintainer struct {
	storage *NATSCommunityStorage

	// tickMu makes one Tick run at a time. It is held across storage I/O; mu is not, so
	// Upsert and Delete never wait on a write.
	tickMu sync.Mutex

	mu           sync.Mutex
	members      map[string]*member
	refused      map[string]struct{}
	groups       [levels]map[string]*group
	dirty        map[groupRef]struct{} // touched since their last successful publication
	published    map[string]*Community // by key: the record last written
	stale        map[string]struct{}   // listed keys whose removal is pending
	bootstrapped bool
	listed       bool // the first tick's key listing succeeded
}

// member is one entity's last-applied revision and contribution; contribution is nil
// while the entity is refused.
type member struct {
	revision     uint64
	contribution *contribution
}

type contribution struct {
	id     string
	groups [levels]string
	typ    string
	words  []string
}

type group struct {
	members     int
	types       map[string]int
	words       map[string]int
	textMembers sortedSet // members with at least one word, for representatives
}

type groupRef struct {
	level int
	id    string
}

// NewMaintainer returns a Maintainer publishing to storage. It refuses a nil storage.
func NewMaintainer(storage *NATSCommunityStorage) (*Maintainer, error) {
	if storage == nil {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "Maintainer", "NewMaintainer", "storage is nil")
	}
	m := &Maintainer{
		storage:   storage,
		members:   map[string]*member{},
		refused:   map[string]struct{}{},
		dirty:     map[groupRef]struct{}{},
		published: map[string]*Community{},
		stale:     map[string]struct{}{},
	}
	for level := range m.groups {
		m.groups[level] = map[string]*group{}
	}
	return m, nil
}

// Upsert applies entityID's value at revision, the revision of the exact value state was
// decoded from. A revision at or below the one last applied for entityID changes
// nothing. Revision 0 is refused and changes nothing: a KV entry's revision is its
// stream sequence, which starts at 1 (TestKVRevisionStartsAtOne), so 0 is a caller that
// passed no revision. It refuses, with an invalid error naming the entity, a nil state, an ID the
// canonical parser refuses, or a state whose ID is not entityID; a refused entity
// contributes nothing until a valid Upsert or a Delete for it.
func (m *Maintainer) Upsert(entityID string, revision uint64, state *graph.EntityState) error {
	if revision == 0 {
		return errRevisionZero(entityID, "Upsert")
	}
	next, refusal := newContribution(entityID, state)

	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.members[entityID]; ok {
		if revision <= old.revision {
			return refusal
		}
		m.subtract(old.contribution)
	}
	m.members[entityID] = &member{revision: revision, contribution: next}
	if refusal != nil {
		m.refused[entityID] = struct{}{}
		return refusal
	}
	delete(m.refused, entityID)
	m.add(next)
	return nil
}

// Delete removes entityID's contribution at revision. A revision at or below the one last
// applied changes nothing, and an entity it does not hold has nothing to remove. Revision
// 0 is refused, as in Upsert.
func (m *Maintainer) Delete(entityID string, revision uint64) error {
	if revision == 0 {
		return errRevisionZero(entityID, "Delete")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.members[entityID]
	if !ok || revision <= old.revision {
		return nil
	}
	m.subtract(old.contribution)
	delete(m.members, entityID)
	delete(m.refused, entityID)
	return nil
}

func errRevisionZero(entityID, method string) error {
	return errs.WrapInvalid(fmt.Errorf("entity %q: revision 0", entityID), "Maintainer", method, "check revision")
}

// BootstrapDone records that the initial replay has been applied; Tick publishes only
// after it.
func (m *Maintainer) BootstrapDone() {
	m.mu.Lock()
	m.bootstrapped = true
	m.mu.Unlock()
}

// Tick publishes every group changed since its last successful publication: it writes
// each record that differs from the one last written and, only after every write
// succeeded, removes the records of groups that became empty and, after the first tick's
// one key listing, the keys of groups that do not exist. A failure, or the context ending
// before the pending work is done, returns an error and leaves the work pending for the
// next Tick; with nothing pending, an ended context is not consulted. It returns
// ErrBootstrapIncomplete before BootstrapDone, an *IncompleteError when all is published
// but some entities are refused, and nil otherwise.
func (m *Maintainer) Tick(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errors.New("nil context"), "Maintainer", "Tick", "missing context")
	}
	m.tickMu.Lock()
	defer m.tickMu.Unlock()

	m.mu.Lock()
	bootstrapped, listed := m.bootstrapped, m.listed
	m.mu.Unlock()
	if !bootstrapped {
		return ErrBootstrapIncomplete
	}
	if !listed {
		keys, err := m.storage.keys(ctx)
		if err != nil {
			return err
		}
		m.mu.Lock()
		for _, key := range keys {
			m.stale[key] = struct{}{}
		}
		m.listed = true
		m.mu.Unlock()
	}

	// A failure requeues every taken group, the written ones too: the next tick finds
	// their records equal to the published ones and does not write them again.
	writes, removals, taken := m.plan()
	for _, record := range writes {
		if err := m.write(ctx, record); err != nil {
			m.requeue(taken)
			return err
		}
	}
	for _, key := range removals {
		if err := m.remove(ctx, key); err != nil {
			m.requeue(taken)
			return err
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.refused) > 0 {
		refused := make([]string, 0, len(m.refused))
		for id := range m.refused {
			refused = append(refused, id)
		}
		slices.Sort(refused)
		return &IncompleteError{Refused: refused}
	}
	return nil
}

// plan takes the dirty groups and returns the records to write, the keys to remove, and
// the taken groups, which go back to dirty if the tick fails before publishing them.
func (m *Maintainer) plan() (writes []*Community, removals []string, taken []groupRef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ref := range m.dirty {
		delete(m.dirty, ref)
		key := communityKey(ref.level, ref.id)
		g := m.groups[ref.level][ref.id]
		if g == nil {
			if _, ok := m.published[key]; ok {
				removals = append(removals, key)
				taken = append(taken, ref)
			}
			continue
		}
		record := newCommunity(ref.level, ref.id, g.members, g.types, g.words, g.textMembers.first(MaxRepEntities))
		if reflect.DeepEqual(record, m.published[key]) {
			continue
		}
		writes = append(writes, record)
		taken = append(taken, ref)
	}
	for key := range m.stale {
		if !m.exists(key) { // a live group's write replaces its listed key
			removals = append(removals, key)
		}
	}
	slices.SortFunc(writes, func(a, b *Community) int {
		return strings.Compare(communityKey(a.Level, a.ID), communityKey(b.Level, b.ID))
	})
	slices.Sort(removals)
	removals = slices.Compact(removals)
	return writes, removals, taken
}

func (m *Maintainer) write(ctx context.Context, record *Community) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.storage.save(ctx, record); err != nil {
		return err
	}
	key := communityKey(record.Level, record.ID)
	m.mu.Lock()
	m.published[key] = record
	delete(m.stale, key)
	m.mu.Unlock()
	return nil
}

func (m *Maintainer) remove(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.storage.remove(ctx, key); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.published, key)
	delete(m.stale, key)
	m.mu.Unlock()
	return nil
}

// exists reports whether key is the key of a group with members. The caller holds mu.
func (m *Maintainer) exists(key string) bool {
	for level, groups := range m.groups {
		if id, ok := strings.CutPrefix(key, strconv.Itoa(level)+"."); ok && groups[id] != nil {
			return true
		}
	}
	return false
}

// requeue marks refs dirty again after a failed tick.
func (m *Maintainer) requeue(refs []groupRef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ref := range refs {
		m.dirty[ref] = struct{}{}
	}
}

// add and subtract apply a contribution to its three groups. The caller holds mu; a nil
// contribution (a refused entity) changes nothing.
func (m *Maintainer) add(c *contribution) {
	if c == nil {
		return
	}
	for level, id := range c.groups {
		g := m.groups[level][id]
		if g == nil {
			g = &group{types: map[string]int{}, words: map[string]int{}}
			m.groups[level][id] = g
		}
		g.members++
		g.types[c.typ]++
		for _, w := range c.words {
			g.words[w]++
		}
		if len(c.words) > 0 {
			g.textMembers.insert(c.id)
		}
		m.dirty[groupRef{level, id}] = struct{}{}
	}
}

func (m *Maintainer) subtract(c *contribution) {
	if c == nil {
		return
	}
	for level, id := range c.groups {
		g := m.groups[level][id]
		g.members--
		decrement(g.types, c.typ)
		for _, w := range c.words {
			decrement(g.words, w)
		}
		if len(c.words) > 0 {
			g.textMembers.remove(c.id)
		}
		if g.members == 0 {
			delete(m.groups[level], id)
		}
		m.dirty[groupRef{level, id}] = struct{}{}
	}
}

func decrement(counts map[string]int, key string) {
	if counts[key]--; counts[key] == 0 {
		delete(counts, key)
	}
}

// newContribution returns what state adds to its groups, or nil and the refusal.
func newContribution(entityID string, state *graph.EntityState) (*contribution, error) {
	refuse := func(err error) (*contribution, error) {
		return nil, errs.WrapInvalid(fmt.Errorf("entity %q: %w", entityID, err), "Maintainer", "Upsert", "refuse entity")
	}
	if state == nil {
		return refuse(errors.New("nil state"))
	}
	eid, err := types.ParseEntityID(entityID)
	if err != nil {
		return refuse(err)
	}
	if state.ID != entityID {
		return refuse(fmt.Errorf("state holds entity %q", state.ID))
	}
	return &contribution{
		id:     entityID,
		groups: [levels]string{eid.TaxonomyPrefix(), eid.SourcePrefix(), eid.DeploymentPrefix()},
		typ:    eid.Type,
		words:  entityWords(state),
	}, nil
}

// sortedSet is an ordered set of strings kept as sorted chunks of at most 2*chunkSize, so
// an insert or a removal moves at most one chunk and the smallest members are read from
// the front, never by scanning the group.
type sortedSet struct {
	chunks [][]string
}

const chunkSize = 256

// chunk returns the index of the chunk where v belongs: the first whose last member is
// not below v, or the last chunk.
func (s *sortedSet) chunk(v string) int {
	i, _ := slices.BinarySearchFunc(s.chunks, v, func(c []string, v string) int { return strings.Compare(c[len(c)-1], v) })
	return min(i, len(s.chunks)-1)
}

func (s *sortedSet) insert(v string) {
	if len(s.chunks) == 0 {
		s.chunks = [][]string{{v}}
		return
	}
	ci := s.chunk(v)
	c := s.chunks[ci]
	i, found := slices.BinarySearch(c, v)
	if found {
		return
	}
	c = slices.Insert(c, i, v)
	if len(c) <= 2*chunkSize {
		s.chunks[ci] = c
		return
	}
	s.chunks[ci] = slices.Clone(c[:chunkSize])
	s.chunks = slices.Insert(s.chunks, ci+1, slices.Clone(c[chunkSize:]))
}

func (s *sortedSet) remove(v string) {
	if len(s.chunks) == 0 {
		return
	}
	ci := s.chunk(v)
	c := s.chunks[ci]
	i, found := slices.BinarySearch(c, v)
	if !found {
		return
	}
	if c = slices.Delete(c, i, i+1); len(c) == 0 {
		s.chunks = slices.Delete(s.chunks, ci, ci+1)
		return
	}
	s.chunks[ci] = c
}

// first returns the n smallest members in order.
func (s *sortedSet) first(n int) []string {
	out := make([]string, 0, n)
	for _, c := range s.chunks {
		if len(out) == n {
			break
		}
		out = append(out, c[:min(len(c), n-len(out))]...)
	}
	return out
}
