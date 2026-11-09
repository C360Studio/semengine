//go:build integration

package smoke

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/pkg/errs"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	"github.com/c360studio/semengine/vocabulary"
)

// The fixed inputs (design D5). Expected answers are written from these, never from a reply.
const (
	producerSource = "smoke-producer"
	namePredicate  = "fleet.identity.name" // ends in .name, so graph-embedding embeds it
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
	subject := ""
	for _, v := range graph.QueryVerbs() {
		if v.Responder == graph.ResponderGraphQuery && v.Name == verb {
			subject = v.Subject
		}
	}
	if subject == "" {
		t.Fatalf("graph-query has no verb %s", verb)
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

// checkAnswers is the one oracle (design D5): every answer the stack gives about w, read after the
// ordered waits. deleted are IDs the stack must no longer hold.
func checkAnswers(t *testing.T, o observer, f fixture, w want, deleted ...string) {
	t.Helper()
	lead := f.id(drone1)
	ids := slices.Sorted(maps.Keys(w))
	revisions := map[string]uint64{}

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
		revisions[id] = exact.KVRevision
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

	// localSearch: nothing in this stack writes community records, so its answer is the refusal.
	reply, err := o.ask(t, "localSearch", graph.LocalSearchRequest{EntityID: lead, Query: "falcon"})
	requireRefusal(t, "localSearch", reply, err, graph.ErrorCodeIndexNotReady, graph.BucketCommunityIndex)

	checkEmbeddingRecords(t, o, revisions)
	if t.Failed() {
		t.FailNow()
	}
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

// checkEmbeddingRecords reads EMBEDDING_INDEX, a declared read beside the public paths (design D5):
// one generated record per live entity, made from the entity's current revision as the entity verb
// read it (revisions), and no other record.
func checkEmbeddingRecords(t *testing.T, o observer, revisions map[string]uint64) {
	t.Helper()
	kv, err := o.js.KeyValue(t.Context(), graph.BucketEmbeddingIndex)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketEmbeddingIndex, err)
	}
	keys, err := kv.Keys(t.Context())
	if err != nil {
		t.Fatalf("keys of %s: %v", graph.BucketEmbeddingIndex, err)
	}
	slices.Sort(keys)
	if wantKeys := slices.Sorted(maps.Keys(revisions)); !slices.Equal(keys, wantKeys) {
		t.Errorf("%s holds records for %v, want %v", graph.BucketEmbeddingIndex, keys, wantKeys)
	}
	for _, key := range keys {
		revision, live := revisions[key]
		if !live {
			continue
		}
		entry, err := kv.Get(t.Context(), key)
		if err != nil {
			t.Fatalf("get %s/%s: %v", graph.BucketEmbeddingIndex, key, err)
		}
		var record embedding.Record
		if err := json.Unmarshal(entry.Value(), &record); err != nil {
			t.Fatalf("decode %s/%s: %v", graph.BucketEmbeddingIndex, key, err)
		}
		if record.Status != embedding.StatusGenerated || record.EntityID != key || record.SourceRevision != revision {
			t.Errorf("%s/%s = status %s, entity %s, source revision %d; want generated, %s, %d",
				graph.BucketEmbeddingIndex, key, record.Status, record.EntityID, record.SourceRevision, key, revision)
		}
	}
}

// TestSmoke_OneConfigAnswers holds composed-stack, "Answers from one configuration": the stack boots
// from one configuration, answers the fixed questions, and follows an edit and a delete.
func TestSmoke_OneConfigAnswers(t *testing.T) {
	url := shared.Reset(t)
	o := newObserver(t, url)
	s := startStack(t, o, url)
	f := fixture{platform: s.platform}
	// StartAll has returned, so the composition is fixed: the four components are listed.
	requireComponentsListed(t, readHealth(t, s))

	inputs := f.fixedEntities()
	var last uint64
	for _, e := range inputs {
		last = o.publish(t, e)
	}
	o.awaitAcknowledged(t, s, last)
	o.awaitIndexed(t, s)
	requireComponentsHealthy(t, readHealth(t, s))
	checkAnswers(t, o, f, wantOf(inputs...))

	// An edit on the stream and a delete through graph-ingest's mutation verb.
	o.awaitAcknowledged(t, s, o.publish(t, f.editedDrone3()))
	deleted := f.id(drone8)
	read, err := o.ask(t, "entity", graph.EntityRequest{ID: deleted})
	if err != nil {
		t.Fatalf("entity %s: %v", deleted, err)
	}
	var held graph.ExactEntity
	if err := json.Unmarshal(read, &held); err != nil {
		t.Fatalf("decode %s: %v", read, err)
	}
	mutations, err := graphmutation.NewClient(o.client, requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	// The delete's reply comes after its write, so awaitIndexed's revision read covers it.
	if _, err := mutations.Delete(t.Context(), graph.DeleteEntityRequest{EntityID: deleted,
		ExpectedRevision: held.KVRevision}); err != nil {
		t.Fatalf("delete %s: %v", deleted, err)
	}
	o.awaitIndexed(t, s)
	requireComponentsHealthy(t, readHealth(t, s))
	edited := wantOf(inputs...)
	edited[f.id(drone3)] = f.editedDrone3()
	delete(edited, deleted)
	checkAnswers(t, o, f, edited, deleted)

	stopStack(t, s)
}

// checkWritesAfterRestart publishes one new entity and one edit to a restarted stack, waits on the
// order design D5 sets after them, and checks the answers: before plus the two writes. It returns
// what the stack now answers. The waits read the ENTITY_STATES revision after the two writes, so the
// graph-index and graph-embedding envelopes an earlier process left, at a lower revision, cannot
// pass them; the new entity's record and the edit's record at its new revision are ones no earlier
// process can have written (design D6).
func checkWritesAfterRestart(t *testing.T, o observer, s stack, f fixture, before want) want {
	t.Helper()
	o.publish(t, f.newDrone())
	o.awaitAcknowledged(t, s, o.publish(t, f.editedDrone3()))
	o.awaitIndexed(t, s)
	requireComponentsHealthy(t, readHealth(t, s))
	after := maps.Clone(before)
	after[f.newDrone().id] = f.newDrone()
	after[f.id(drone3)] = f.editedDrone3()
	checkAnswers(t, o, f, after)
	return after
}

// TestSmoke_StackRestartKeepsAnswers holds composed-stack, "Stop and start the stack" (design D6,
// case 1): the stack stops on SIGTERM, a new process boots from the same configuration on the same
// server, and it answers what the first one did plus what is published after the restart.
func TestSmoke_StackRestartKeepsAnswers(t *testing.T) {
	url := shared.Reset(t)
	o := newObserver(t, url)
	first := startStack(t, o, url)
	f := fixture{platform: first.platform}
	requireComponentsListed(t, readHealth(t, first))

	inputs := f.fixedEntities()
	var last uint64
	for _, e := range inputs {
		last = o.publish(t, e)
	}
	o.awaitAcknowledged(t, first, last)
	o.awaitIndexed(t, first)
	requireComponentsHealthy(t, readHealth(t, first))
	before := wantOf(inputs...)
	checkAnswers(t, o, f, before)
	t.Logf("first process, %s: %v", reindexMetric, readReindexCounter(t, first))
	stopStack(t, first)

	second := restartStack(t, o, url, first)
	// The configuration bucket holds the minted ID, so the second process adopts it (ADR-104).
	if second.platform != first.platform {
		t.Fatalf("the restarted stack reports platform %q, want the first process's %q", second.platform, first.platform)
	}
	requireComponentsListed(t, readHealth(t, second))
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
// read beside the public paths (design D5). It reports whether the record exists.
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

// TestSmoke_KillMidIngestKeepsAcknowledgedWrites holds composed-stack, "Kill mid-ingest" (design D6,
// case 2): the first process holds graph-ingest on one input, after its entity write and before its
// applied-sequence record and acknowledgement, and is killed there; a new process boots from the
// same configuration. Every input the ENTITY stream acknowledged then answers with each statement
// once, the held input among them, and the index and embedding records catch up, including the
// writes published after the new process booted.
func TestSmoke_KillMidIngestKeepsAcknowledgedWrites(t *testing.T) {
	url := shared.Reset(t)
	o := newObserver(t, url)
	ingestConfig := envIngestConfig + "=" + killIngestConfig(t)
	first := startStack(t, o, url, ingestConfig, envHold+"="+heldDrone)
	f := fixture{platform: first.platform}
	requireComponentsListed(t, readHealth(t, first))

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
	requireComponentsListed(t, readHealth(t, second))
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
	o.awaitIndexed(t, second)
	requireComponentsHealthy(t, readHealth(t, second))
	before := wantOf(append(inputs, held)...)
	checkAnswers(t, o, f, before)

	checkWritesAfterRestart(t, o, second, f, before)
	stopStack(t, second)
}

// brokerBudget bounds the own server's Start and Restart, which require a bounded context. At an
// hour it is an unreachable fallback: a broker that never comes back ends at go test -timeout
// (docs/testing.md, #500).
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
// stopped" (design D6, case 3): the stack stops on SIGTERM, the NATS server restarts on the same
// URL with its file storage, and a new process boots from the same configuration and answers what
// the first one did plus what is published after the restart.
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
		first  stack
		f      fixture
		before want
	)
	if !t.Run("first boot", func(t *testing.T) {
		o := newObserver(t, url)
		first = startStack(t, o, url)
		f = fixture{platform: first.platform}
		requireComponentsListed(t, readHealth(t, first))

		inputs := f.fixedEntities()
		var last uint64
		for _, e := range inputs {
			last = o.publish(t, e)
		}
		o.awaitAcknowledged(t, first, last)
		o.awaitIndexed(t, first)
		requireComponentsHealthy(t, readHealth(t, first))
		before = wantOf(inputs...)
		checkAnswers(t, o, f, before)
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
	requireComponentsListed(t, readHealth(t, second))
	checkWritesAfterRestart(t, o, second, f, before)
	stopStack(t, second)
}
