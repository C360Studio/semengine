//go:build integration

package smoke

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/fusion"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	"github.com/c360studio/semengine/vocabulary"
)

// The fixed inputs. Expected answers are written from these, never from a reply.
const (
	producerSource = "smoke-producer"
	namePredicate  = "fleet.identity.name" // ends in .name, so graph-lexical indexes its words
	nextPredicate  = "fleet.link.next"
	homePredicate  = "fleet.link.home"

	// leadAlias is drone1's alias.
	leadAlias = "falcon-lead"

	droneType = "fleet.ops.drone"
	baseType  = "fleet.ops.base"
)

// The fixed entities, by their last three ID positions. The first two are the deployment's authority,
// c360 and the platform ID config.Manager minted from the declared stem at the first boot (ADR-104),
// which the child reports; fixture.id puts them together.
const (
	drone1 = "drone.001"
	drone2 = "drone.002"
	drone3 = "drone.003"
	drone4 = "drone.004"
	drone5 = "drone.005"
	drone6 = "drone.006"
	drone7 = "drone.007"
	drone8 = "drone.008"
	base1  = "base.001"
	base2  = "base.002"
	base3  = "base.003"
	base4  = "base.004"
)

// fixture names the fixed entities under one deployment's minted platform ID.
type fixture struct{ platform string }

// id is the six-part entity ID of local: c360.<platform>.fleet.ops.<local>.
func (f fixture) id(local string) string { return "c360." + f.platform + ".fleet.ops." + local }

// inputTime is every published statement's timestamp.
var inputTime = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

// statement is one published fact: a predicate and its object.
type statement struct{ predicate, object string }

// entityInput is one entity the producer publishes, with all its statements.
type entityInput struct {
	id         string
	statements []statement
}

func (f fixture) named(local, name string, links ...statement) entityInput {
	return entityInput{id: f.id(local), statements: append([]statement{{namePredicate, name}}, links...)}
}

func (f fixture) link(predicate, local string) statement { return statement{predicate, f.id(local)} }

// fixedEntities is the first batch: a chain drone1 -> drone2 -> drone3 -> drone4 -> base1 (the
// known path), drones 5 to 8 homed at base2, two bases with no edge, and drone1's alias.
func (f fixture) fixedEntities() []entityInput {
	return []entityInput{
		f.named(drone1, "Falcon", f.link(nextPredicate, drone2), statement{vocabulary.CoreIdentityAlias, leadAlias}),
		f.named(drone2, "Kestrel", f.link(nextPredicate, drone3)),
		f.named(drone3, "Osprey", f.link(nextPredicate, drone4)),
		f.named(drone4, "Harrier", f.link(homePredicate, base1)),
		f.named(drone5, "Merlin", f.link(homePredicate, base2)),
		f.named(drone6, "Hobby", f.link(homePredicate, base2)),
		f.named(drone7, "Lanner", f.link(homePredicate, base2)),
		f.named(drone8, "Saker", f.link(homePredicate, base2)),
		f.named(base1, "North Field"),
		f.named(base2, "South Field"),
		f.named(base3, "East Field"),
		f.named(base4, "West Field"),
	}
}

// editedDrone3 renames drone3 and keeps its edge: the edit TestSmoke_OneConfigAnswers publishes.
func (f fixture) editedDrone3() entityInput {
	return f.named(drone3, "Osprey Two", f.link(nextPredicate, drone4))
}

// newDrone is the entity published after a restart: one no earlier process can have written, homed
// at base3, which had no edge until then.
func (f fixture) newDrone() entityInput {
	return f.named("drone.009", "Peregrine", f.link(homePredicate, base3))
}

// leadPath is pathSearch's answer from drone1, outgoing, written from the chain above.
func (f fixture) leadPath() graph.PathSearchData {
	return graph.PathSearchData{
		Entities: []graph.PathEntity{
			{EntityID: f.id(drone1), Depth: 0}, {EntityID: f.id(drone2), Depth: 1}, {EntityID: f.id(drone3), Depth: 2},
			{EntityID: f.id(drone4), Depth: 3}, {EntityID: f.id(base1), Depth: 4},
		},
		Edges: []graph.RelationshipEdge{
			{Predicate: nextPredicate, FromEntityID: f.id(drone1), ToEntityID: f.id(drone2)},
			{Predicate: nextPredicate, FromEntityID: f.id(drone2), ToEntityID: f.id(drone3)},
			{Predicate: nextPredicate, FromEntityID: f.id(drone3), ToEntityID: f.id(drone4)},
			{Predicate: homePredicate, FromEntityID: f.id(drone4), ToEntityID: f.id(base1)},
		},
	}
}

// want is what the stack must answer: the live entities, by ID.
type want map[string]entityInput

func wantOf(inputs ...entityInput) want {
	w := want{}
	for _, e := range inputs {
		w[e.id] = e
	}
	return w
}

// ask sends request to graph-query's verb and returns the reply bytes or the classified refusal.
func (o observer) ask(t *testing.T, verb string, request any) ([]byte, error) {
	t.Helper()
	return o.askResponder(t, graph.ResponderGraphQuery, verb, request)
}

// askResponder sends request to responder's verb, its subject read from graph's verb table, and
// returns the reply bytes or the classified refusal.
func (o observer) askResponder(t *testing.T, responder, verb string, request any) ([]byte, error) {
	t.Helper()
	subject := ""
	for _, v := range graph.QueryVerbs() {
		if v.Responder == responder && v.Name == verb {
			subject = v.Subject
		}
	}
	if subject == "" {
		t.Fatalf("%s has no verb %s", responder, verb)
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return o.client.RequestClassified(t.Context(), subject, data, requestTimeout)
}

// askData asks an enveloped verb and decodes the envelope's data into out.
func (o observer) askData(t *testing.T, verb string, request, out any) {
	t.Helper()
	reply, err := o.ask(t, verb, request)
	if err != nil {
		t.Fatalf("%s %+v: %v", verb, request, err)
	}
	var envelope graph.QueryResponse[json.RawMessage]
	if err := json.Unmarshal(reply, &envelope); err != nil {
		t.Fatalf("%s: decode envelope %s: %v", verb, reply, err)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		t.Fatalf("%s: decode data %s: %v", verb, envelope.Data, err)
	}
}

// statementsOf returns the entity's statements as sorted predicate/object pairs, every one kept, so
// a statement held twice shows twice.
func statementsOf(entity *graph.EntityState) []string {
	var got []string
	for _, triple := range entity.Triples {
		got = append(got, fmt.Sprintf("%s=%v", triple.Predicate, triple.Object))
	}
	slices.Sort(got)
	return got
}

// statementStrings is what the entity must hold: its published statements, and the indexing profile
// graph-ingest stamps at creation from the payload type's registration.
func (e entityInput) statementStrings() []string {
	out := []string{vocabulary.EntityIndexingProfile + "=" + smokeIndexingProfile}
	for _, s := range e.statements {
		out = append(out, s.predicate+"="+s.object)
	}
	slices.Sort(out)
	return out
}

// edges returns the relationship edges w holds, sorted.
func (w want) edges() []graph.RelationshipEdge {
	var out []graph.RelationshipEdge
	for _, e := range w {
		for _, s := range e.statements {
			if s.predicate == nextPredicate || s.predicate == homePredicate {
				out = append(out, graph.RelationshipEdge{Predicate: s.predicate, FromEntityID: e.id, ToEntityID: s.object})
			}
		}
	}
	sortEdges(out)
	return out
}

func sortEdges(edges []graph.RelationshipEdge) {
	slices.SortFunc(edges, func(a, b graph.RelationshipEdge) int {
		return cmp.Or(strings.Compare(a.FromEntityID, b.FromEntityID), strings.Compare(a.ToEntityID, b.ToEntityID),
			strings.Compare(a.Predicate, b.Predicate))
	})
}

// requireRefusal fails unless err is a classified refusal with code whose message names text.
func requireRefusal(t *testing.T, what string, reply []byte, err error, code, text string) {
	t.Helper()
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) {
		t.Fatalf("%s answered %s (error %v), want the refusal %s", what, reply, err, code)
	}
	if refusal.Code != code || !strings.Contains(refusal.Error(), text) {
		t.Fatalf("%s refused %q: %v, want %q naming %s", what, refusal.Code, refusal, code, text)
	}
}

// checkAnswers is the one oracle: every answer the stack gives about w, read after the ordered
// waits. revision is the ENTITY_STATES revision awaitIndexed read after the last write, which the
// community and word search checks wait on. deleted are IDs the stack must no longer hold. s is the
// running stack, which the search and community checks' waits watch. It returns graph-lexical's
// replies by word (checkLexical), which a restart test compares with the restarted stack's.
func checkAnswers(t *testing.T, o observer, s stack, f fixture, w want, revision uint64, deleted ...string) lexicalReplies {
	t.Helper()
	lead := f.id(drone1)
	ids := slices.Sorted(maps.Keys(w))

	// entity: each live entity with each statement once; a deleted one not found.
	for _, id := range ids {
		reply, err := o.ask(t, "entity", graph.EntityRequest{ID: id})
		if err != nil {
			t.Fatalf("entity %s: %v", id, err)
		}
		var exact graph.ExactEntity
		if err := json.Unmarshal(reply, &exact); err != nil || exact.Entity == nil {
			t.Fatalf("entity %s: decode %s: %v", id, reply, err)
		}
		if got, wantStatements := statementsOf(exact.Entity), w[id].statementStrings(); !slices.Equal(got, wantStatements) {
			t.Errorf("entity %s holds %v, want %v", id, got, wantStatements)
		}
	}
	for _, id := range deleted {
		reply, err := o.ask(t, "entity", graph.EntityRequest{ID: id})
		requireRefusal(t, "entity "+id, reply, err, graph.ErrorCodeEntityNotFound, id)
	}

	// relationships: every edge of every live entity, both directions.
	edges := w.edges()
	for _, id := range ids {
		for _, direction := range []string{"outgoing", "incoming"} {
			var got []graph.RelationshipEdge
			o.askData(t, "relationships", map[string]string{"entity_id": id, "direction": direction}, &got)
			sortEdges(got)
			wantEdges := []graph.RelationshipEdge{}
			for _, e := range edges {
				if (direction == "outgoing" && e.FromEntityID == id) || (direction == "incoming" && e.ToEntityID == id) {
					wantEdges = append(wantEdges, e)
				}
			}
			if !slices.Equal(got, wantEdges) {
				t.Errorf("relationships %s %s = %+v, want %+v", id, direction, got, wantEdges)
			}
		}
	}

	// pathSearch: the known path.
	var path graph.PathSearchData
	o.askData(t, "pathSearch", graph.PathSearchRequest{StartEntity: lead, Direction: "outgoing",
		MaxDepth: graph.MaxPathSearchDepth}, &path)
	if wantPath := f.leadPath(); !reflect.DeepEqual(path, wantPath) {
		t.Errorf("pathSearch from %s = %+v, want %+v", lead, path, wantPath)
	}

	// summary: the whole graph on one page, its types and its predicates.
	var summary graph.SummaryData
	o.askData(t, "summary", graph.SummaryRequest{}, &summary)
	checkSummary(t, summary, w)

	// entityByAlias: the alias's holder, as the entity verb reads it.
	var aliased graph.ExactEntity
	o.askData(t, "entityByAlias", map[string]string{"alias": leadAlias}, &aliased)
	if aliased.Entity == nil || aliased.Entity.ID != lead {
		t.Errorf("entityByAlias %s = %+v, want %s", leadAlias, aliased.Entity, lead)
	} else if got := statementsOf(aliased.Entity); !slices.Equal(got, w[lead].statementStrings()) {
		t.Errorf("entityByAlias %s holds %v, want %v", leadAlias, got, w[lead].statementStrings())
	}

	checkCommunities(t, o, s, f, w, revision, deleted)
	if t.Failed() {
		t.FailNow()
	}
	return checkLexical(t, o, s, f, w, revision, deleted)
}

// checkSummary compares the summary with the counts written from w.
func checkSummary(t *testing.T, summary graph.SummaryData, w want) {
	t.Helper()
	byType := map[string][]string{}
	predicates := map[string]int{}
	for id, e := range w {
		kind := droneType
		if strings.Contains(id, ".base.") {
			kind = baseType
		}
		byType[kind] = append(byType[kind], id)
		predicates[vocabulary.EntityIndexingProfile]++
		seen := map[string]bool{}
		for _, s := range e.statements {
			if !seen[s.predicate] {
				predicates[s.predicate]++
				seen[s.predicate] = true
			}
		}
	}
	if summary.TotalEntities != len(w) || summary.EntitiesScanned != len(w) || summary.Truncated {
		t.Errorf("summary total %d, scanned %d, truncated %v, want %d, %d, false",
			summary.TotalEntities, summary.EntitiesScanned, summary.Truncated, len(w), len(w))
	}
	gotTypes := map[string]int{}
	for _, entityType := range summary.EntityTypes {
		gotTypes[entityType.Type] = entityType.Count
		for _, example := range entityType.Examples {
			if !slices.Contains(byType[entityType.Type], example) {
				t.Errorf("summary type %s gives example %s, not one of its entities", entityType.Type, example)
			}
		}
	}
	wantTypes := map[string]int{}
	for kind, members := range byType {
		wantTypes[kind] = len(members)
	}
	if !maps.Equal(gotTypes, wantTypes) {
		t.Errorf("summary types %v, want %v", gotTypes, wantTypes)
	}
	gotPredicates := map[string]int{}
	for _, p := range summary.Predicates {
		gotPredicates[p.Predicate] = p.EntityCount
	}
	if !maps.Equal(gotPredicates, predicates) {
		t.Errorf("summary predicates %v, want %v", gotPredicates, predicates)
	}
}

// name is e's name, the one statement graph-lexical indexes (namePredicate ends in .name).
func (e entityInput) name() string {
	for _, s := range e.statements {
		if s.predicate == namePredicate {
			return s.object
		}
	}
	return ""
}

// words is e's name as graph-lexical's word index tokenizes these names: lowercased, split on
// spaces. No fixture name holds a stopword, a word under two letters, or a plural the word index
// folds.
func (e entityInput) words() []string { return strings.Fields(strings.ToLower(e.name())) }

// fixedInput is the fixed entity id.
func (f fixture) fixedInput(t *testing.T, id string) entityInput {
	t.Helper()
	for _, e := range f.fixedEntities() {
		if e.id == id {
			return e
		}
	}
	t.Fatalf("%s is not a fixed entity", id)
	return entityInput{}
}

// keywords gives each input the last word of its name that no other input's name holds, so a word
// search for it matches that entity alone. The last word, because the edit appends one: "Osprey
// Two" gives "two", which only drone3's current record holds, so a hit on it is never the record
// from before the edit.
// before the edit.
func keywords(t *testing.T, inputs []entityInput) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, e := range inputs {
		words := e.words()
		for i := len(words) - 1; i >= 0 && out[e.id] == ""; i-- {
			shared := false
			for _, other := range inputs {
				shared = shared || (other.id != e.id && slices.Contains(other.words(), words[i]))
			}
			if !shared {
				out[e.id] = words[i]
			}
		}
		if out[e.id] == "" {
			t.Fatalf("%s's name %q has no word of its own", e.id, e.name())
		}
	}
	return out
}

// lexicalReplies are graph-lexical's search replies, by the word asked.
type lexicalReplies map[string]graph.LexicalSearchData

// lexicalAnswer is one graph-lexical search reply, or the refusal.
type lexicalAnswer struct {
	reply graph.QueryResponse[graph.LexicalSearchData]
	raw   []byte
	err   error
}

// lexicalSearch asks graph-lexical's search verb for word until the reply's indexed_revision reaches
// revision. The word index applies ENTITY_STATES in its own process, so the reply's
// indexed_revision is the one event that orders the read after the writes up to revision.
// index_not_ready before the first build is asked again; a lost feed, which holds until the process
// restarts, and every other refusal end the wait and fail.
func (o observer) lexicalSearch(t *testing.T, s stack, word string, revision uint64) graph.QueryResponse[graph.LexicalSearchData] {
	t.Helper()
	request := graph.LexicalSearchRequest{Query: word}
	what := fmt.Sprintf("graph-lexical search %q at indexed_revision %d or above", word, revision)
	got := await(t, s, what, func(context.Context) (lexicalAnswer, error) {
		var a lexicalAnswer
		a.raw, a.err = o.askResponder(t, graph.ResponderGraphLexical, "search", request)
		if a.err == nil {
			a.err = json.Unmarshal(a.raw, &a.reply)
		}
		return a, nil
	}, func(a lexicalAnswer) bool {
		var refusal *errs.ClassifiedError
		if errors.As(a.err, &refusal) && refusal.Code == graph.ErrorCodeIndexNotReady {
			return strings.Contains(refusal.Error(), "feed is lost")
		}
		return a.err != nil || a.reply.IndexedRevision >= revision
	})
	if got.err != nil {
		t.Fatalf("%s: %s: %v", what, got.raw, got.err)
	}
	if got.reply.Producer != graph.ResponderGraphLexical {
		t.Fatalf("%s: producer %q, want %q", what, got.reply.Producer, graph.ResponderGraphLexical)
	}
	return got.reply
}

// lexicalIDs is the hits' entity IDs, in the order the reply gave them.
func lexicalIDs(hits []graph.LexicalHit) []string {
	out := make([]string, len(hits))
	for i, hit := range hits {
		out[i] = hit.EntityID
	}
	return out
}

// wordHits is every word search's expected answer, written from the fixed names: each entity's
// keyword (keywords), held by its name alone, answers that entity alone while it is live and
// nothing once it is deleted; and "field", which every base's name holds, answers exactly the live
// bases. Every base's name is two words, one of them "field", and holds no other text, so each base
// scores the same and the ties order by entity ID. deleted are IDs the stack must no longer hold.
func wordHits(t *testing.T, f fixture, w want, deleted []string) map[string][]string {
	t.Helper()
	inputs := slices.Collect(maps.Values(w))
	for _, id := range deleted {
		inputs = append(inputs, f.fixedInput(t, id))
	}
	out := map[string][]string{}
	keyword := keywords(t, inputs)
	for _, e := range inputs {
		out[keyword[e.id]] = []string{}
		if _, live := w[e.id]; live {
			out[keyword[e.id]] = []string{e.id}
		}
	}

	bases := []string{}
	for _, id := range slices.Sorted(maps.Keys(w)) {
		if slices.Contains(w[id].words(), baseWord) {
			bases = append(bases, id)
		}
	}
	if len(bases) == 0 {
		t.Fatalf("no live entity's name holds %q, so the search would prove nothing", baseWord)
	}
	out[baseWord] = bases
	return out
}

// baseWord is the word every base's name holds.
const baseWord = "field"

// checkLexical checks graph-lexical's search verb against wordHits (composed-stack, "Answers from
// one configuration"), each reply read once it reflects revision. The fixture has no offloaded
// bodies, so none is counted as not indexed. It returns the replies by word.
func checkLexical(t *testing.T, o observer, s stack, f fixture, w want, revision uint64, deleted []string) lexicalReplies {
	t.Helper()
	replies := lexicalReplies{}
	expected := wordHits(t, f, w, deleted)
	for _, word := range slices.Sorted(maps.Keys(expected)) {
		wantHits := expected[word]
		data := o.lexicalSearch(t, s, word, revision).Data
		replies[word] = data
		hits := lexicalIDs(data.Results)
		if !slices.Equal(hits, wantHits) || data.Matched != len(wantHits) || data.BodiesNotIndexed != 0 {
			t.Errorf("graph-lexical search %q answered %v (matched %d, %d bodies not indexed), want %v (matched %d, 0)",
				word, hits, data.Matched, data.BodiesNotIndexed, wantHits, len(wantHits))
		}
		for _, hit := range data.Results {
			if hit.Score <= 0 || hit.Score != data.Results[0].Score {
				t.Errorf("graph-lexical search %q scored %+v, want every hit the same score above 0", word, data.Results)
				break
			}
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	// fusion's word question: a word lens resolves through graph-lexical's word search, and its
	// nodes are the live entities whose name holds the word, with provenance lexical. fusion's
	// request names no revision; the search for the same word above reflected revision, and nothing
	// is written in between, so this one reads the same index. Only the entity set is checked:
	// relations are checked through graph-query's own verbs.
	resp, err := o.fusionEngine(t).Fuse(t.Context(), fusion.Request{Query: baseWord}, fleetLens{fusion.ResolveModeNL})
	if err != nil {
		t.Fatalf("NL %q: Fuse: %v", baseWord, err)
	}
	handles := []string{}
	for _, n := range resp.Nodes {
		handles = append(handles, n.Handle)
	}
	slices.Sort(handles)
	if resp.Provenance != fusion.ProvenanceLexical || resp.Deferred || len(resp.Misses) > 0 || !slices.Equal(handles, expected[baseWord]) {
		t.Fatalf("NL %q answered %+v (nodes %v), want lexical provenance and nodes %v", baseWord, resp, handles,
			expected[baseWord])
	}
	return replies
}

// requireLexicalKept fails unless graph-lexical answers each word in before as it did then, hits,
// order and scores, once its reply reflects revision. A restarted process rebuilds its word index
// from ENTITY_STATES, and nothing is written between the two replies, so the index it rebuilt must
// score as the one it replaced.
func requireLexicalKept(t *testing.T, o observer, s stack, revision uint64, before lexicalReplies) {
	t.Helper()
	for _, word := range slices.Sorted(maps.Keys(before)) {
		if got := o.lexicalSearch(t, s, word, revision).Data; !reflect.DeepEqual(got, before[word]) {
			t.Errorf("graph-lexical search %q answered %+v after the restart, want %+v as before it", word, got, before[word])
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

// communityAnswer is one localSearch reply, or the refusal.
type communityAnswer struct {
	reply graph.QueryResponse[graph.LocalSearchData]
	raw   []byte
	err   error
}

// matchIDs is the entity IDs of matches, in the order the reply gave them.
func matchIDs(matches []graph.TextSearchMatch) []string {
	out := []string{}
	for _, match := range matches {
		out = append(out, match.ID)
	}
	return out
}

// checkCommunities checks graph-clustering's records and graph-query's two word searches,
// localSearch and global search (composed-stack, "Answers from one configuration"). Every fixed
// entity's ID starts c360.<platform>.fleet.ops, so they make one level-0 group, whose record holds
// w's count.
//
// localSearch attaches the group's record when graph-query's own view of COMMUNITY_INDEX holds it.
// Neither GRAPH_STATUS nor a bucket read orders a read of that view, so localSearch on the lead at
// min_revision revision is asked until it answers with the record of all w's members; a refusal
// that the word index is not ready or below revision is asked again. Then each word of wordHits
// is searched both ways (checkWordSearches).
func checkCommunities(t *testing.T, o observer, s stack, f fixture, w want, revision uint64, deleted []string) {
	t.Helper()
	lead := f.id(drone1)
	group := "c360." + f.platform + ".fleet.ops" // level 0: org.platform.system.domain (ADR-102)
	word := w[lead].words()[0]
	request := graph.LocalSearchRequest{EntityID: lead, Query: word, MinRevisionField: graph.MinRevisionField{MinRevision: revision}}
	what := fmt.Sprintf("localSearch %q on %s at min_revision %d", word, lead, revision)
	got := await(t, s, what+" to answer", func(context.Context) (communityAnswer, error) {
		var answer communityAnswer
		answer.raw, answer.err = o.ask(t, "localSearch", request)
		if answer.err == nil {
			answer.err = json.Unmarshal(answer.raw, &answer.reply)
		}
		return answer, nil
	}, func(a communityAnswer) bool {
		if wordIndexCatchingUp(a.err) {
			return false
		}
		record := a.reply.Data.Record
		return a.err != nil || (record != nil && record.MemberCount == len(w))
	})
	if got.err != nil {
		t.Fatalf("%s: %s: %v", what, got.raw, got.err)
	}
	if local := got.reply.Data; local.RecordState != graph.RecordRead || local.Answer == "" {
		t.Errorf("%s answered %+v; want the record read and an answer", what, local)
	}

	expected := wordHits(t, f, w, deleted)
	for _, word := range slices.Sorted(maps.Keys(expected)) {
		checkWordSearches(t, o, s, lead, group, len(w), word, expected[word], revision)
	}
	if t.Failed() {
		t.FailNow()
	}
}

// wordIndexCatchingUp reports whether err is graph-lexical's refusal, passed on by graph-query,
// that its word index has not reached the request's min_revision: index_not_ready before its first
// build, or revision_not_reached below it. A lost feed, which holds until the process restarts, is
// not catching up.
func wordIndexCatchingUp(err error) bool {
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) {
		return false
	}
	return refusal.Code == graph.ErrorCodeRevisionNotReached ||
		(refusal.Code == graph.ErrorCodeIndexNotReady && !strings.Contains(refusal.Error(), "feed is lost"))
}

// searchAnswer is one graph-query search reply's bytes, or the refusal.
type searchAnswer struct {
	raw []byte
	err error
}

// askSearch asks graph-query's verb until the reply is not the word index catching up, and
// decodes it into out. The request carries min_revision, which graph-query passes to graph-lexical,
// so an answer is read from a word index whose indexed_revision has reached it: that revision is
// the event that orders the read after the writes.
func askSearch(t *testing.T, o observer, s stack, verb string, request, out any) {
	t.Helper()
	what := fmt.Sprintf("%s %+v", verb, request)
	got := await(t, s, what+" to answer", func(context.Context) (searchAnswer, error) {
		var a searchAnswer
		a.raw, a.err = o.ask(t, verb, request)
		return a, nil
	}, func(a searchAnswer) bool { return !wordIndexCatchingUp(a.err) })
	if got.err != nil {
		t.Fatalf("%s: %s: %v", what, got.raw, got.err)
	}
	if err := json.Unmarshal(got.raw, out); err != nil {
		t.Fatalf("%s: decode %s: %v", what, got.raw, err)
	}
}

// checkWordSearches checks that localSearch on the lead and global search on word answer
// wantIDs, the word index's expected hits for it, in that order, read at revision or above. Every
// fixed entity is in the lead's group, so the scoped and the unscoped search find the same
// entities. Global search names that group, holding every match and its record of members
// entities, when anything matches, and no group when nothing does.
func checkWordSearches(t *testing.T, o observer, s stack, lead, group string, members int, word string, wantIDs []string, revision uint64) {
	t.Helper()
	at := graph.MinRevisionField{MinRevision: revision}

	var local graph.QueryResponse[graph.LocalSearchData]
	askSearch(t, o, s, "localSearch", graph.LocalSearchRequest{EntityID: lead, Query: word, MinRevisionField: at}, &local)
	l := local.Data
	if ids := matchIDs(l.Matches); !slices.Equal(ids, wantIDs) || l.Matched != len(wantIDs) ||
		l.CommunityID != group || l.Level != 0 || l.Missing != 0 || l.Truncated || local.IndexedRevision < revision {
		t.Errorf("localSearch %q on %s: matched %v (%d), group %s level %d, %d missing, truncated %v, "+
			"indexed_revision %d; want %v (%d), group %s level 0, 0 missing, not truncated, at %d or above",
			word, lead, ids, l.Matched, l.CommunityID, l.Level, l.Missing, l.Truncated, local.IndexedRevision,
			wantIDs, len(wantIDs), group, revision)
	}

	var global graph.QueryResponse[graph.GlobalSearchData]
	askSearch(t, o, s, "globalSearch", graph.GlobalSearchRequest{Query: word, MinRevisionField: at}, &global)
	text := global.Data.Text
	if text == nil {
		t.Errorf("globalSearch %q answered %+v, want text", word, global.Data)
		return
	}
	var groups []string
	for _, g := range text.Groups {
		count := 0
		if g.Record != nil {
			count = g.Record.MemberCount
		}
		groups = append(groups, fmt.Sprintf("%s/%d matches/%s/%d members", g.CommunityID, g.Matches, g.RecordState, count))
	}
	var wantGroups []string
	if len(wantIDs) > 0 {
		wantGroups = []string{fmt.Sprintf("%s/%d matches/%s/%d members", group, len(wantIDs), graph.RecordRead, members)}
	}
	if ids := matchIDs(text.Matches); !slices.Equal(ids, wantIDs) || text.Matched != len(wantIDs) ||
		!slices.Equal(groups, wantGroups) || text.Level != 0 || text.UnreadableGroups != 0 || text.Missing != 0 ||
		text.Truncated || global.IndexedRevision < revision {
		t.Errorf("globalSearch %q: matched %v (%d), groups %v, level %d, %d unreadable, %d missing, truncated %v, "+
			"indexed_revision %d; want %v (%d), groups %v, level 0, 0, 0, not truncated, at %d or above",
			word, ids, text.Matched, groups, text.Level, text.UnreadableGroups, text.Missing, text.Truncated,
			global.IndexedRevision, wantIDs, len(wantIDs), wantGroups, revision)
	}
}

// TestSmoke_OneConfigAnswers holds composed-stack, "Answers from one configuration": the stack boots
// from one configuration with no graph-embedding, answers the fixed questions over NATS, through
// fusion and over graph-gateway's GraphQL route, streams ENTITY_STATES to a websocket client, and
// follows an edit and a delete.
func TestSmoke_OneConfigAnswers(t *testing.T) {
	url := shared.Reset(t)
	o := newObserver(t, url)
	s := startStack(t, o, url)
	f := fixture{platform: s.platform}
	// StartAll has returned, so the composition is fixed: the seven components are listed.
	requireComponentsListed(t, s)

	inputs := f.fixedEntities()
	var last uint64
	for _, e := range inputs {
		last = o.publish(t, e)
	}
	o.awaitAcknowledged(t, s, last)
	revision := o.awaitIndexed(t, s)
	requireComponentsHealthy(t, s)
	// awaitIndexed read graph-ingest's, graph-index's, graph-clustering's and graph-lexical's
	// entries, written by this child on a server the reset emptied, so the listing is complete for
	// the four; no component runs that writes another, graph-embedding's least of all.
	if keys, want := o.graphStatusKeys(t), []string{readiness.KeyGraphClustering, readiness.KeyGraphIndex,
		readiness.KeyGraphIngest, readiness.KeyGraphLexical}; !slices.Equal(keys, want) {
		t.Fatalf("%s holds keys %v, want %v (no %s)", graph.BucketGraphStatus, keys, want, readiness.KeyGraphEmbedding)
	}
	checkAnswers(t, o, s, f, wantOf(inputs...), revision)
	checkFusion(t, o, f)
	checkGateway(t, s, f, wantOf(inputs...))
	// The client connects after the writes, so its snapshot holds a put for each fixed entity and
	// synced, and any put the view had not yet applied follows as a change.
	ws := dialWebsocket(t, s)
	streamed := map[string]stored{}
	synced := ws.readSnapshot(t, streamed)
	checkWebsocket(t, o, ws, streamed, synced, revision, wantOf(inputs...))

	streamedThrough := max(synced, revision)

	// An edit on the stream and a delete through graph-ingest's mutation verb.
	o.awaitAcknowledged(t, s, o.publish(t, f.editedDrone3()))
	deleted := f.id(drone8)
	o.deleteEntity(t, deleted)
	revision = o.awaitIndexed(t, s)
	requireComponentsHealthy(t, s)
	edited := wantOf(inputs...)
	edited[f.id(drone3)] = f.editedDrone3()
	delete(edited, deleted)
	checkAnswers(t, o, s, f, edited, revision, deleted)
	checkGateway(t, s, f, edited)
	checkWebsocket(t, o, ws, streamed, streamedThrough, revision, edited)

	stopStack(t, s)
}

// deleteEntity deletes id through graph-ingest's mutation verb, at the revision its entity read
// reports. The delete's reply comes after its write, so an awaitIndexed after it covers the delete.
func (o observer) deleteEntity(t *testing.T, id string) {
	t.Helper()
	read, err := o.ask(t, "entity", graph.EntityRequest{ID: id})
	if err != nil {
		t.Fatalf("entity %s: %v", id, err)
	}
	var held graph.ExactEntity
	if err := json.Unmarshal(read, &held); err != nil {
		t.Fatalf("decode %s: %v", read, err)
	}
	mutations, err := graphmutation.NewClient(o.client, requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutations.Delete(t.Context(), graph.DeleteEntityRequest{EntityID: id,
		ExpectedRevision: held.KVRevision}); err != nil {
		t.Fatalf("delete %s: %v", id, err)
	}
}

// checkWritesAfterRestart publishes one new entity and one edit to a restarted stack, waits on the
// ordered waits after them, and checks the answers: before plus the two writes. It returns what the
// stack now answers. The waits read the ENTITY_STATES revision after the two writes, so the
// graph-index, graph-clustering and graph-lexical envelopes and the community record an earlier
// process left, at a lower revision, cannot pass them; the new entity's record
// and the edit's record at its new revision are ones no earlier process can have written.
func checkWritesAfterRestart(t *testing.T, o observer, s stack, f fixture, before want) want {
	t.Helper()
	o.publish(t, f.newDrone())
	o.awaitAcknowledged(t, s, o.publish(t, f.editedDrone3()))
	revision := o.awaitIndexed(t, s)
	requireComponentsHealthy(t, s)
	after := maps.Clone(before)
	after[f.newDrone().id] = f.newDrone()
	after[f.id(drone3)] = f.editedDrone3()
	checkAnswers(t, o, s, f, after, revision)
	return after
}

// TestSmoke_StackRestartKeepsAnswers holds composed-stack, "Stop and start the stack": the stack
// stops on SIGTERM, a new process boots from the same configuration on the same server, and it
// answers what the first one did plus what is published after the restart. Before any new write,
// graph-lexical's replies equal the first process's, hits, order and scores.
func TestSmoke_StackRestartKeepsAnswers(t *testing.T) {
	url := shared.Reset(t)
	o := newObserver(t, url)
	first := startStack(t, o, url)
	f := fixture{platform: first.platform}
	requireComponentsListed(t, first)

	inputs := f.fixedEntities()
	var last uint64
	for _, e := range inputs {
		last = o.publish(t, e)
	}
	o.awaitAcknowledged(t, first, last)
	revision := o.awaitIndexed(t, first)
	requireComponentsHealthy(t, first)
	before := wantOf(inputs...)
	lexical := checkAnswers(t, o, first, f, before, revision)
	t.Logf("first process, %s: %v", reindexMetric, readReindexCounter(t, first))
	stopStack(t, first)

	second := restartStack(t, o, url, first)
	// The configuration bucket holds the minted ID, so the second process adopts it (ADR-104).
	if second.platform != first.platform {
		t.Fatalf("the restarted stack reports platform %q, want the first process's %q", second.platform, first.platform)
	}
	requireComponentsListed(t, second)
	requireLexicalKept(t, o, second, revision, lexical)
	checkWritesAfterRestart(t, o, second, f, before)
	t.Logf("second process, %s: %v", reindexMetric, readReindexCounter(t, second))
	stopStack(t, second)
}

// heldDrone is the input case 2 holds: published last, a new entity homed at base4, which had no
// edge until then, so the index must catch up on an edge the held input brings.
const heldDrone = "drone.010"

func (f fixture) heldInput() entityInput {
	return f.named(heldDrone, "Gyrfalcon", f.link(homePredicate, base4))
}

// killAckWait is graph-ingest's acknowledgement wait in case 2: the broker delivers the held input
// again this long after the killed process last signalled it, so the new process gets it soon.
const killAckWait = "2s"

// killIngestConfig is graph-ingest's configuration in case 2: its defaults, with the entity stream
// port's ack_wait set to killAckWait. A ports block is used as written (graph-ingest's
// ApplyDefaults), so it is built from DefaultConfig's, never spelled by hand.
func killIngestConfig(t *testing.T) string {
	t.Helper()
	cfg := graphingest.DefaultConfig()
	set := 0
	for i, port := range cfg.Ports.Inputs {
		if stream, ok := port.Config.(component.JetStreamPort); ok {
			stream.AckWait = killAckWait
			cfg.Ports.Inputs[i].Config = stream
			set++
		}
	}
	if set != 1 {
		t.Fatalf("graph-ingest's default configuration has %d stream ports, want 1", set)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal graph-ingest's configuration: %v", err)
	}
	return string(data)
}

// appliedRecord reads graph-ingest's applied-sequence record for id on the ENTITY stream, a declared
// read beside the public paths. It reports whether the record exists.
func (o observer) appliedRecord(t *testing.T, id string) bool {
	t.Helper()
	kv, err := o.js.KeyValue(t.Context(), graph.BucketGraphIngestAppliedSeq)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketGraphIngestAppliedSeq, err)
	}
	// The key is the entity ID, "/", the stream name (processor/graph-ingest/keyed_ingest.go, guardKey).
	_, err = kv.Get(t.Context(), id+"/ENTITY")
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false
	}
	if err != nil {
		t.Fatalf("get %s/%s/ENTITY: %v", graph.BucketGraphIngestAppliedSeq, id, err)
	}
	return true
}

// TestSmoke_KillMidIngestKeepsAcknowledgedWrites holds composed-stack, "Kill mid-ingest": the first
// process holds graph-ingest on one input, after its entity write and before its applied-sequence
// record and acknowledgement, and is killed there; a new process boots from the same configuration.
// Every input the ENTITY stream acknowledged then answers with each statement once, the held input
// among them, and the index, word-index and community records catch up, including the writes published after
// the new process booted.
func TestSmoke_KillMidIngestKeepsAcknowledgedWrites(t *testing.T) {
	url := shared.Reset(t)
	o := newObserver(t, url)
	ingestConfig := envIngestConfig + "=" + killIngestConfig(t)
	first := startStack(t, o, url, ingestConfig, envHold+"="+heldDrone)
	f := fixture{platform: first.platform}
	requireComponentsListed(t, first)

	// Each publish below returned the stream's acknowledgement: every one of them must survive.
	inputs := f.fixedEntities()
	for _, e := range inputs {
		o.publish(t, e)
	}
	held := f.heldInput()
	heldSeq := o.publish(t, held)

	// Mid-ingest, shown by three events rather than a time. Lanes are serial per entity and the held
	// input was published last, so nothing waits behind it: every other input is acknowledged, the
	// held one is the one pending, and its entity write has landed.
	entities, err := o.js.KeyValue(t.Context(), graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketEntityStates, err)
	}
	type midIngest struct {
		Consumer   consumerState
		HeldStored bool
	}
	mid := await(t, first, fmt.Sprintf("the held input (sequence %d) written and all before it acknowledged", heldSeq),
		func(ctx context.Context) (midIngest, error) {
			consumer, err := o.readConsumer(ctx)
			if err != nil {
				return midIngest{}, err
			}
			_, err = entities.Get(ctx, held.id)
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return midIngest{Consumer: consumer}, nil
			}
			return midIngest{Consumer: consumer, HeldStored: err == nil}, err
		}, func(m midIngest) bool { return m.HeldStored && m.Consumer.AckFloor >= heldSeq-1 })
	if mid.Consumer.AckFloor != heldSeq-1 || mid.Consumer.AckPending != 1 || mid.Consumer.DeliveredStream < heldSeq {
		t.Fatalf("before the kill graph-ingest's consumer reads %+v, want ack floor %d, 1 pending, delivered through %d",
			mid.Consumer, heldSeq-1, heldSeq)
	}
	// The held application must not have written its applied-sequence record. The record of the
	// input before it is read first, so an absent read cannot pass on a wrong key or bucket.
	if last := inputs[len(inputs)-1].id; !o.appliedRecord(t, last) {
		t.Fatalf("%s has no applied-sequence record, though its input is acknowledged", last)
	}
	if o.appliedRecord(t, held.id) {
		t.Fatalf("%s has an applied-sequence record while its application is held", held.id)
	}

	if !first.process.Alive() {
		t.Fatal("the first stack exited before the test killed it")
	}
	if err := first.process.Kill(); err != nil {
		t.Fatalf("kill the first stack: %v", err)
	}
	status, err := first.process.Wait(t.Context())
	if err != nil {
		t.Fatalf("wait for the first stack: %v", err)
	}
	if status.Signal != syscall.SIGKILL {
		t.Fatalf("the first stack ended %+v, want by SIGKILL", status)
	}

	second := restartStack(t, o, url, first, ingestConfig)
	if second.platform != first.platform {
		t.Fatalf("the restarted stack reports platform %q, want the first process's %q", second.platform, first.platform)
	}
	requireComponentsListed(t, second)
	// The held input comes again after the acknowledgement wait; its acknowledgement follows its
	// applied-sequence record.
	redelivered := o.awaitAcknowledged(t, second, heldSeq)
	if redelivered.Deliveries <= mid.Consumer.Deliveries {
		t.Fatalf("graph-ingest's consumer made %d deliveries, want more than the %d before the kill",
			redelivered.Deliveries, mid.Consumer.Deliveries)
	}
	if !o.appliedRecord(t, held.id) {
		t.Fatalf("%s is acknowledged without its applied-sequence record", held.id)
	}
	revision := o.awaitIndexed(t, second)
	requireComponentsHealthy(t, second)
	before := wantOf(append(inputs, held)...)
	checkAnswers(t, o, second, f, before, revision)

	checkWritesAfterRestart(t, o, second, f, before)
	stopStack(t, second)
}

// brokerBudget bounds the own server's Start and Restart, which require a bounded context. At an
// hour it is an unreachable fallback: a broker that never comes back ends at go test -timeout
// (docs/testing.md).
const brokerBudget = time.Hour

// restartBrokerKeepingURL restarts the NATS server and requires its URL unchanged, as
// graph-ingest's restartKeepingURL does: the restarted stack is booted on the URL the first boot
// was given, and Docker does not promise the host port (natsfixture/restart.go), so a remapped port
// fails here as the fixture's failure, not the stack's.
func restartBrokerKeepingURL(t *testing.T, broker *natsfixture.Fixture) {
	t.Helper()
	before := broker.URL()
	ctx, cancel := context.WithTimeout(t.Context(), brokerBudget)
	defer cancel()
	if err := broker.Restart(ctx); err != nil {
		t.Fatalf("restart the NATS server: %v", err)
	}
	if after := broker.URL(); after != before {
		t.Fatalf("the NATS server's URL changed across Restart, %s to %s: a fixture failure, not the stack's", before, after)
	}
}

// TestSmoke_BrokerRestartBetweenRuns holds composed-stack, "Restart the broker while the stack is
// stopped": the stack stops on SIGTERM, the NATS server restarts on the same URL with its file
// storage, and a new process boots from the same configuration and answers what the first one did
// plus what is published after the restart. Before any new write, graph-lexical's replies equal the
// first process's, hits, order and scores.
func TestSmoke_BrokerRestartBetweenRuns(t *testing.T) {
	// Own server: restarts the NATS server between the two boots, which the shared one must not.
	broker := natsfixture.New(t)
	ctx, cancel := context.WithTimeout(t.Context(), brokerBudget)
	defer cancel()
	if err := broker.Start(ctx); err != nil {
		t.Fatalf("start the NATS server: %v", err)
	}
	url := broker.URL()

	// The first boot runs in a subtest, so its observer's connection is closed before the restart:
	// Restart's caller stops what it built on the URL first (natsfixture/restart.go).
	var (
		first    stack
		f        fixture
		before   want
		revision uint64
		lexical  lexicalReplies
	)
	if !t.Run("first boot", func(t *testing.T) {
		o := newObserver(t, url)
		first = startStack(t, o, url)
		f = fixture{platform: first.platform}
		requireComponentsListed(t, first)

		inputs := f.fixedEntities()
		var last uint64
		for _, e := range inputs {
			last = o.publish(t, e)
		}
		o.awaitAcknowledged(t, first, last)
		revision = o.awaitIndexed(t, first)
		requireComponentsHealthy(t, first)
		before = wantOf(inputs...)
		lexical = checkAnswers(t, o, first, f, before, revision)
		stopStack(t, first)
	}) {
		return
	}

	restartBrokerKeepingURL(t, broker)

	o := newObserver(t, url)
	second := restartStack(t, o, url, first)
	// The configuration bucket, file-backed, kept the minted ID across the broker restart (ADR-104).
	if second.platform != first.platform {
		t.Fatalf("the stack booted after the broker restart reports platform %q, want the first process's %q",
			second.platform, first.platform)
	}
	requireComponentsListed(t, second)
	requireLexicalKept(t, o, second, revision, lexical)
	checkWritesAfterRestart(t, o, second, f, before)
	stopStack(t, second)
}

// TestSmoke_BrokerRestartUnderRunningStack holds composed-stack, "Restart the broker under the
// running stack": the NATS server restarts on the same URL with its file storage while the stack
// keeps running, and the same process, never restarted, applies a new entity, an edit and a delete
// published after it, answers what it did before plus those writes, and lists its seven components
// healthy, with no envelope reporting a lost feed.
func TestSmoke_BrokerRestartUnderRunningStack(t *testing.T) {
	// Own server: restarts the NATS server under the running stack, which the shared one must not.
	broker := natsfixture.New(t)
	ctx, cancel := context.WithTimeout(t.Context(), brokerBudget)
	defer cancel()
	if err := broker.Start(ctx); err != nil {
		t.Fatalf("start the NATS server: %v", err)
	}
	url := broker.URL()

	o := newObserver(t, url)
	s := startStack(t, o, url)
	f := fixture{platform: s.platform}
	requireComponentsListed(t, s)

	inputs := f.fixedEntities()
	var last uint64
	for _, e := range inputs {
		last = o.publish(t, e)
	}
	o.awaitAcknowledged(t, s, last)
	revision := o.awaitIndexed(t, s)
	requireComponentsHealthy(t, s)
	before := wantOf(inputs...)
	checkAnswers(t, o, s, f, before, revision)

	// Unlike case 3, nothing is stopped before the restart: the stack and the observer keep their
	// connections, and both reconnect with natsclient's defaults (unlimited attempts). The observer
	// waits on its own reconnection, counted by nats.go once the connection is re-established, so
	// its first publish goes to the restarted server, not into nats.go's reconnect buffer.
	reconnects := o.client.GetConnection().Stats().Reconnects
	restartBrokerKeepingURL(t, broker)
	restarted := time.Now()
	await(t, s, "the observer's connection to reconnect", func(context.Context) (uint64, error) {
		return o.client.GetConnection().Stats().Reconnects, nil
	}, func(n uint64) bool { return n > reconnects && o.client.IsHealthy() })
	if !s.process.Alive() {
		t.Fatal("the stack exited across the NATS server restart")
	}
	// graph-ingest's readiness record from before the restart must not pass awaitIndexed's
	// not-degraded read: only a record written after the reconnect counts.
	s.ingestStatusBefore = o.statusRevision(t, readiness.KeyGraphIngest)

	// The waits in checkWritesAfterRestart read the ENTITY_STATES revision after the two writes, so
	// the envelopes the stack wrote before the restart cannot pass them.
	after := checkWritesAfterRestart(t, o, s, f, before)
	t.Logf("the writes published after the restart were indexed and answered %v after Restart returned (recorded, not asserted)",
		time.Since(restarted).Round(time.Millisecond))

	// A delete through graph-ingest's mutation verb: its request subscription, too, outlived the
	// restart.
	deleted := f.id(drone8)
	o.deleteEntity(t, deleted)
	revision = o.awaitIndexed(t, s)
	requireComponentsHealthy(t, s)
	delete(after, deleted)
	checkAnswers(t, o, s, f, after, revision, deleted)
	stopStack(t, s)
}

// gatewayRoute is graph-gateway's GraphQL route: the service manager mounts the component's routes
// at /<instance> (service/service_manager_http.go), and the configuration names it graph-gateway.
const gatewayRoute = "/graph-gateway/graphql"

// postGraphQL posts one GraphQL request to graph-gateway through the stack's HTTP port and decodes
// the answer's data.<field> into out. It fails unless the answer is HTTP 200 with no errors.
func postGraphQL(t *testing.T, s stack, field, query string, vars map[string]any, out any) {
	t.Helper()
	request, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		t.Fatal(err)
	}
	url, code, body := send(t, s, s.httpPort, http.MethodPost, gatewayRoute, request)
	var answer struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []json.RawMessage          `json:"errors"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("POST %s %s: HTTP %d, not JSON: %v: %s", url, field, code, err, body)
	}
	if code != http.StatusOK || len(answer.Errors) != 0 {
		t.Fatalf("POST %s %s: HTTP %d %s, want 200 with no errors", url, field, code, body)
	}
	if err := json.Unmarshal(answer.Data[field], out); err != nil {
		t.Fatalf("POST %s %s: decode data.%s %s: %v", url, field, field, body, err)
	}
}

// gatewayEntity is an Entity in graph-gateway's schema, with the triple fields the check selects.
type gatewayEntity struct {
	ID      string `json:"id"`
	Triples []struct {
		Predicate string `json:"predicate"`
		Object    any    `json:"object"`
	} `json:"triples"`
}

// statements returns the entity's triples as statementsOf does.
func (e gatewayEntity) statements() []string {
	var got []string
	for _, triple := range e.Triples {
		got = append(got, fmt.Sprintf("%s=%v", triple.Predicate, triple.Object))
	}
	slices.Sort(got)
	return got
}

// checkGateway checks graph-gateway's four root fields, posted to its route on the process's HTTP
// port, against w (composed-stack, "Answers from one configuration"). The gateway asks graph-query,
// which forwards entity and entitiesByPrefix to graph-ingest's ENTITY_STATES reads and
// entitiesByPredicate and pathSearch to graph-index; the caller's awaitIndexed returned after
// graph-index reported the ENTITY_STATES revision read after the last write, which orders these
// reads after every write w holds.
func checkGateway(t *testing.T, s stack, f fixture, w want) {
	t.Helper()
	ids := slices.Sorted(maps.Keys(w))
	const entityFields = `id triples { predicate object }`

	// entity: each live entity's ID and every triple it holds.
	for _, id := range ids {
		var exact struct {
			Entity     gatewayEntity `json:"entity"`
			KVRevision uint64        `json:"kvRevision"`
		}
		postGraphQL(t, s, "entity", `query($id: String!) { entity(id: $id) { entity { `+entityFields+` } kvRevision } }`,
			map[string]any{"id": id}, &exact)
		if exact.Entity.ID != id || exact.KVRevision == 0 {
			t.Errorf("gateway entity %s answered ID %q at kvRevision %d, want its own ID at a revision", id,
				exact.Entity.ID, exact.KVRevision)
		}
		if got, wantStatements := exact.Entity.statements(), w[id].statementStrings(); !slices.Equal(got, wantStatements) {
			t.Errorf("gateway entity %s holds %v, want %v", id, got, wantStatements)
		}
	}

	// entitiesByPrefix: the four bases, on one page, in key order, each with its triples.
	basePrefix := f.id("base")
	var page struct {
		Entities   []gatewayEntity `json:"entities"`
		Total      int             `json:"total"`
		NextCursor *string         `json:"next_cursor"`
	}
	postGraphQL(t, s, "entitiesByPrefix", `query($p: String!) { entitiesByPrefix(prefix: $p) { entities { `+
		entityFields+` } total next_cursor } }`, map[string]any{"p": basePrefix}, &page)
	wantBases := []string{f.id(base1), f.id(base2), f.id(base3), f.id(base4)}
	var pageIDs []string
	for _, e := range page.Entities {
		pageIDs = append(pageIDs, e.ID)
		if got, wantStatements := e.statements(), w[e.ID].statementStrings(); !slices.Equal(got, wantStatements) {
			t.Errorf("gateway entitiesByPrefix %s: %s holds %v, want %v", basePrefix, e.ID, got, wantStatements)
		}
	}
	if !slices.Equal(pageIDs, wantBases) || page.Total != len(wantBases) || page.NextCursor != nil {
		t.Errorf("gateway entitiesByPrefix %s = %v, total %d, next_cursor %v; want %v, total %d, no cursor",
			basePrefix, pageIDs, page.Total, page.NextCursor, wantBases, len(wantBases))
	}

	// entitiesByPredicate: the entities homed at base2, as a value filter on homePredicate.
	wantHomed := []string{}
	for _, local := range []string{drone5, drone6, drone7, drone8} {
		if _, live := w[f.id(local)]; live {
			wantHomed = append(wantHomed, f.id(local))
		}
	}
	var matches struct {
		Entities []string `json:"entities"`
	}
	postGraphQL(t, s, "entitiesByPredicate", `query($p: String!, $v: String) { entitiesByPredicate(predicate: $p, value: $v) { entities } }`,
		map[string]any{"p": homePredicate, "v": f.id(base2)}, &matches)
	if !slices.Equal(matches.Entities, wantHomed) {
		t.Errorf("gateway entitiesByPredicate %s = %s: %v, want %v", homePredicate, f.id(base2), matches.Entities, wantHomed)
	}

	// pathSearch: the known path, each entity with the edges that lead to it from the lead.
	type edge struct{ From, Predicate, To string }
	var path struct {
		Entities []struct {
			ID    string `json:"id"`
			Depth int    `json:"depth"`
		} `json:"entities"`
		Paths     [][]edge `json:"paths"`
		Truncated bool     `json:"truncated"`
	}
	postGraphQL(t, s, "pathSearch", `query($start: String!, $depth: Int!) { pathSearch(startEntity: $start, direction: "OUTGOING", maxDepth: $depth) {
		entities { id depth } paths { from predicate to } truncated } }`,
		map[string]any{"start": f.id(drone1), "depth": graph.MaxPathSearchDepth}, &path)
	lead := f.leadPath()
	var gotPath, wantPath []string
	for k, e := range path.Entities {
		gotPath = append(gotPath, fmt.Sprintf("%s@%d", e.ID, e.Depth))
		if k < len(path.Paths) {
			gotPath = append(gotPath, fmt.Sprint(path.Paths[k]))
		}
	}
	// The chain is a line, so the path to entity k is the first k edges.
	var steps []edge
	for k, e := range lead.Entities {
		wantPath = append(wantPath, fmt.Sprintf("%s@%d", e.EntityID, e.Depth), fmt.Sprint(steps))
		if k < len(lead.Edges) {
			steps = append(steps, edge{lead.Edges[k].FromEntityID, lead.Edges[k].Predicate, lead.Edges[k].ToEntityID})
		}
	}
	if !slices.Equal(gotPath, wantPath) || len(path.Paths) != len(path.Entities) || path.Truncated {
		t.Errorf("gateway pathSearch from %s = %v (%d paths, truncated %v), want %v (%d paths, not truncated)",
			f.id(drone1), gotPath, len(path.Paths), path.Truncated, wantPath, len(lead.Entities))
	}
}
