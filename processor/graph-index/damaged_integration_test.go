//go:build integration

package graphindex

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

// damagedValue is an ENTITY_STATES value graph.UnmarshalEntityState refuses: it is not JSON.
var damagedValue = []byte("{not json")

// counted is a decoded successful reply with the count of damaged entities it left out.
type counted[T any] struct {
	Data            T      `json:"data"`
	IndexedRevision uint64 `json:"indexed_revision"`
	DamagedEntities *int   `json:"damaged_entities"`
}

// settled sends body on verb until graph-index answers or refuses it with anything but a transient
// refusal (index_not_ready, revision_not_reached), and returns the reply or the refusal. The
// request carries min_revision, so a reply reflects every write at or below it.
func settled(t *testing.T, h *harness, verb string, body map[string]any, minRevision uint64) ([]byte, error) {
	t.Helper()
	body["min_revision"] = minRevision
	type outcome struct {
		raw []byte
		err error
	}
	got := await(t, verb+" settles",
		func(context.Context) (outcome, error) {
			raw, err := query(t, h.client, verb, body)
			return outcome{raw, err}, nil
		},
		func(o outcome) bool { return o.err == nil || !errs.IsTransient(o.err) })
	return got.raw, got.err
}

// countedAnswer requires a successful reply to verb at minRevision and returns it decoded.
func countedAnswer[T any](t *testing.T, h *harness, verb string, body map[string]any, minRevision uint64) counted[T] {
	t.Helper()
	raw, err := settled(t, h, verb, body, minRevision)
	if err != nil {
		t.Fatalf("%s %v: %v", verb, body, err)
	}
	var reply counted[T]
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("%s reply %s: %v", verb, raw, err)
	}
	return reply
}

// requireDamaged requires reply's damaged_entities to be want, and absent when want is 0.
func requireDamaged[T any](t *testing.T, verb string, reply counted[T], want int) {
	t.Helper()
	switch {
	case want == 0 && reply.DamagedEntities != nil:
		t.Errorf("%s: damaged_entities = %d; want the key absent", verb, *reply.DamagedEntities)
	case want > 0 && (reply.DamagedEntities == nil || *reply.DamagedEntities != want):
		t.Errorf("%s: damaged_entities = %v; want %d", verb, reply.DamagedEntities, want)
	}
}

// requireDamagedRefusal requires err to be the fatal graph_state_reset_required refusal naming id.
func requireDamagedRefusal(t *testing.T, verb, id string, err error) {
	t.Helper()
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorFatal ||
		classified.Code != graph.ErrorCodeGraphStateResetRequired {
		t.Errorf("%s of %s: err = %v; want fatal %s", verb, id, err, graph.ErrorCodeGraphStateResetRequired)
		return
	}
	if !strings.Contains(err.Error(), id) {
		t.Errorf("%s of %s: refusal %v does not name the entity", verb, id, err)
	}
}

// requireRelationship requires entries, rendered as strings, to equal want.
func requireRelationship(t *testing.T, what string, entries []string, want []string) {
	t.Helper()
	if !slices.Equal(entries, want) {
		t.Errorf("%s = %v; want %v", what, entries, want)
	}
}

func outgoingTargets(entries []graph.OutgoingEntry) []string {
	targets := []string{}
	for _, e := range entries {
		targets = append(targets, e.Predicate+">"+e.ToEntityID)
	}
	return targets
}

func incomingSources(entries []graph.IncomingEntry) []string {
	sources := []string{}
	for _, e := range entries {
		sources = append(sources, e.Predicate+"<"+e.FromEntityID)
	}
	return sources
}

func nameIDs(matches []graph.NameMatch) []string {
	ids := []string{}
	for _, m := range matches {
		ids = append(ids, m.EntityID)
	}
	return ids
}

// requireHealthNames reads Health once and requires it not healthy, naming id with the decode
// refusal's reason. One read is enough: the damage is reported before the entity's completion,
// and every reply the caller read before this waited on a min_revision that completion reaches.
func requireHealthNames(t *testing.T, h *harness, id string) {
	t.Helper()
	health := h.c.Health()
	want := id + " (" + string(graph.GraphStateReasonUnreadableEntity) + ")"
	if health.Healthy || !strings.Contains(health.LastError, want) {
		t.Errorf("Health = %+v; want not healthy, naming %s", health, want)
	}
}

// TestDamagedEntityLeftOutAndNamed: B's stored value does not decode. graph-index keeps answering:
// A's rows answer with damaged_entities 1, B's own rows are gone, outgoing of B is refused with
// the recorded refusal, incoming of B answers A's edge, and Health names B. Each request carries
// min_revision B's revision, so its reply reflects B's damage.
func TestDamagedEntityLeftOutAndNamed(t *testing.T) {
	a, b := entityID(1), entityID(2)
	valueA := func(t *testing.T) []byte {
		return entityValue(t, a, stmt{linkTo, b}, stmt{vocabulary.DCTermsTitle, "Alpha"}, stmt{alias, "alpha-1"})
	}

	t.Run("stored before start", func(t *testing.T) {
		client := fixtureClient(t)
		states := ensureEntityStates(t, client)
		if _, err := states.Put(t.Context(), a, valueA(t)); err != nil {
			t.Fatal(err)
		}
		revB, err := states.Put(t.Context(), b, damagedValue)
		if err != nil {
			t.Fatal(err)
		}
		h := startIndex(t, client, `{}`)
		requireAnswersPastDamage(t, h, a, b, revB)
	})

	t.Run("overwritten after start", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		h.put(t, a, valueA(t))
		revValid := h.put(t, b, entityValue(t, b,
			stmt{linkTo, a}, stmt{vocabulary.DCTermsTitle, "Beta"}, stmt{alias, "beta-1"}))
		names := countedAnswer[graph.NameData](t, h, "byName", map[string]any{"name": "Beta"}, revValid)
		requireRelationship(t, "byName Beta before the damage", nameIDs(names.Data.Matches), []string{b})

		revB := h.put(t, b, damagedValue)
		requireAnswersPastDamage(t, h, a, b, revB)
		names = countedAnswer[graph.NameData](t, h, "byName", map[string]any{"name": "Beta"}, revB)
		requireRelationship(t, "byName Beta after the damage", nameIDs(names.Data.Matches), []string{})
		requireDamaged(t, "byName Beta", names, 1)
		aliasB := countedAnswer[graph.AliasData](t, h, "alias", map[string]any{"alias": "beta-1"}, revB)
		if aliasB.Data.CanonicalID != nil {
			t.Errorf("alias beta-1 = %s after the damage; want none", *aliasB.Data.CanonicalID)
		}
		requireDamaged(t, "alias beta-1", aliasB, 1)
		if got := h.c.Health().ErrorCount; got != 1 {
			t.Errorf("ErrorCount = %d after one entity was damaged; want 1", got)
		}
	})
}

// requireAnswersPastDamage checks the replies once B, at revB, is damaged and A links to B.
func requireAnswersPastDamage(t *testing.T, h *harness, a, b string, revB uint64) {
	t.Helper()
	names := countedAnswer[graph.NameData](t, h, "byName", map[string]any{"name": "Alpha"}, revB)
	requireRelationship(t, "byName Alpha", nameIDs(names.Data.Matches), []string{a})
	requireDamaged(t, "byName Alpha", names, 1)

	aliasA := countedAnswer[graph.AliasData](t, h, "alias", map[string]any{"alias": "alpha-1"}, revB)
	if aliasA.Data.CanonicalID == nil || *aliasA.Data.CanonicalID != a {
		t.Errorf("alias alpha-1 = %v; want %s", aliasA.Data.CanonicalID, a)
	}
	requireDamaged(t, "alias alpha-1", aliasA, 1)

	outA := countedAnswer[graph.OutgoingRelationshipsData](t, h, "outgoing", map[string]any{"entity_id": a}, revB)
	requireRelationship(t, "outgoing of A", outgoingTargets(outA.Data.Relationships), []string{linkTo + ">" + b})
	requireDamaged(t, "outgoing of A", outA, 1)

	inB := countedAnswer[graph.IncomingRelationshipsData](t, h, "incoming", map[string]any{"entity_id": b}, revB)
	requireRelationship(t, "incoming of B", incomingSources(inB.Data.Relationships), []string{linkTo + "<" + a})
	requireDamaged(t, "incoming of B", inB, 1)

	// Nothing of B's earlier value links to A.
	inA := countedAnswer[graph.IncomingRelationshipsData](t, h, "incoming", map[string]any{"entity_id": a}, revB)
	requireRelationship(t, "incoming of A", incomingSources(inA.Data.Relationships), []string{})

	_, err := settled(t, h, "outgoing", map[string]any{"entity_id": b}, revB)
	requireDamagedRefusal(t, "outgoing", b, err)

	requireHealthNames(t, h, b)
}

// TestRepairedEntityReturns: B, damaged, is written again with a valid value. With no restart its
// rows answer, replies carry no damaged_entities, and Health is healthy again.
func TestRepairedEntityReturns(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	b := entityID(2)
	revDamaged := h.put(t, b, damagedValue)
	names := countedAnswer[graph.NameData](t, h, "byName", map[string]any{"name": "Beta"}, revDamaged)
	requireDamaged(t, "byName Beta while damaged", names, 1)

	revRepair := h.put(t, b, entityValue(t, b, stmt{vocabulary.DCTermsTitle, "Beta"}, stmt{alias, "beta-1"}))
	names = countedAnswer[graph.NameData](t, h, "byName", map[string]any{"name": "Beta"}, revRepair)
	requireRelationship(t, "byName Beta after the repair", nameIDs(names.Data.Matches), []string{b})
	requireDamaged(t, "byName Beta after the repair", names, 0)
	out := countedAnswer[graph.OutgoingRelationshipsData](t, h, "outgoing", map[string]any{"entity_id": b}, revRepair)
	requireDamaged(t, "outgoing of B after the repair", out, 0)
	// The reply above waited for the repair's revision, whose completion cleared the record.
	if health := h.c.Health(); !health.Healthy {
		t.Errorf("Health after the repair = %+v; want healthy", health)
	}
}

// TestRemovedNoncanonicalKeyIsNoLongerDamaged: a value is stored under a key KV accepts that is not
// a six-part entity ID, so graph-index records the key damaged. Health tells the operator to delete
// such a key; once it is deleted or purged, with no restart, the record is gone: a reply past the
// removal counts no damaged entity and Health is healthy. legacy-key is read back to decide; a key
// longer than the shared KV key validator allows is not read, and its delete decides.
func TestRemovedNoncanonicalKeyIsNoLongerDamaged(t *testing.T) {
	keys := []struct{ name, key string }{
		{"legacy-key", "legacy-key"},
		{"over-long key", strings.Repeat("k", natsclient.MaxKVLiteralKeyBytes+1)},
	}
	removals := []struct {
		name string
		run  func(ctx context.Context, states jetstream.KeyValue, key string) error
	}{
		{"delete", func(ctx context.Context, states jetstream.KeyValue, key string) error { return states.Delete(ctx, key) }},
		{"purge", func(ctx context.Context, states jetstream.KeyValue, key string) error { return states.Purge(ctx, key) }},
	}
	for _, k := range keys {
		for _, remove := range removals {
			t.Run(k.name+" "+remove.name, func(t *testing.T) {
				h := startIndex(t, fixtureClient(t), `{}`)
				h.awaitIndexed(t, h.put(t, k.key, damagedValue))
				if got := h.f.DamagedCount(nil); got != 1 {
					t.Fatalf("damaged entities after a value under %s = %d; want 1", k.name, got)
				}
				if err := remove.run(t.Context(), h.states, k.key); err != nil {
					t.Fatalf("%s %s: %v", remove.name, k.name, err)
				}
				// A write after the removal: a reply at its revision waits for the floor to pass
				// it, so the removal's revision, below it, has completed.
				a := entityID(1)
				rev := h.put(t, a, entityValue(t, a, stmt{vocabulary.DCTermsTitle, "Alpha"}))
				names := countedAnswer[graph.NameData](t, h, "byName", map[string]any{"name": "Alpha"}, rev)
				requireRelationship(t, "byName Alpha", nameIDs(names.Data.Matches), []string{a})
				requireDamaged(t, "byName Alpha after the "+remove.name, names, 0)
				if health := h.c.Health(); !health.Healthy {
					t.Errorf("Health after the %s of %s = %+v; want healthy", remove.name, k.name, health)
				}
			})
		}
	}
}

// TestUnreadableKeyRemovalOutlastsCoalescedWrite: a key longer than the shared KV key validator
// allows cannot be read back, so its delivered events decide whether it is damaged. With
// coalescing on, a write of it taken into a batch must not record it damaged after its delete or
// purge has been applied: no later event for the key would clear that record. The batch is handed
// to the lanes only after the removal's revision has completed, and a reply past a later write
// through the same lane must count no damaged entity.
func TestUnreadableKeyRemovalOutlastsCoalescedWrite(t *testing.T) {
	removals := []struct {
		name string
		run  func(ctx context.Context, states jetstream.KeyValue, key string) error
	}{
		{"delete", func(ctx context.Context, states jetstream.KeyValue, key string) error { return states.Delete(ctx, key) }},
		{"purge", func(ctx context.Context, states jetstream.KeyValue, key string) error { return states.Purge(ctx, key) }},
	}
	for _, remove := range removals {
		t.Run(remove.name, func(t *testing.T) {
			// The window is far past go test's deadline, so only the test fires the coalescer. One
			// lane takes every key, in submit order.
			h := startIndex(t, fixtureClient(t), `{"coalesce_ms":3600000,"workers":1}`)
			bad := strings.Repeat("k", natsclient.MaxKVLiteralKeyBytes+1)
			a := entityID(1)

			// The coalescer's own callback, wrapped: it hands the lanes every key but bad at once, and
			// bad's write only once release is closed, as a batch taken just before the removal and
			// scheduled after it would.
			original := h.c.coalescer.callback
			captured, release := make(chan struct{}), make(chan struct{})
			h.c.coalescer.callback = func(ctx context.Context, batch []coalescedEntity) {
				var held []coalescedEntity
				rest := batch[:0:0]
				for _, e := range batch {
					if e.entityID == bad {
						held = append(held, e)
					} else {
						rest = append(rest, e)
					}
				}
				original(ctx, rest)
				select {
				case <-captured:
				default:
					close(captured)
				}
				<-release
				original(ctx, held)
			}
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			pendingAt := func(key string, rev uint64) {
				await(t, "write of "+key+" handed to the coalescer", func(context.Context) (bool, error) {
					h.c.coalescer.mu.Lock()
					defer h.c.coalescer.mu.Unlock()
					return h.c.coalescer.pending[key] == rev, nil
				}, func(v bool) bool { return v })
			}

			h.put(t, bad, damagedValue)
			// Entries are delivered one at a time in order, so a's pending revision means bad's write
			// was delivered before it.
			pendingAt(a, h.put(t, a, entityValue(t, a, stmt{vocabulary.DCTermsTitle, "Alpha"})))
			returned := make(chan struct{})
			go func() {
				h.c.coalescer.fire(t.Context())
				close(returned)
			}()
			<-captured

			if err := remove.run(t.Context(), h.states, bad); err != nil {
				t.Fatalf("%s: %v", remove.name, err)
			}
			status, err := h.states.Status(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			h.awaitIndexed(t, status.(*jetstream.KeyValueBucketStatus).StreamInfo().State.LastSeq)
			if got := h.f.DamagedCount(nil); got != 0 {
				t.Fatalf("damaged entities once the %s completed = %d; want 0", remove.name, got)
			}

			close(release)
			<-returned
			// The one lane applies this write after whatever the batch handed it for bad.
			rev := h.put(t, a, entityValue(t, a, stmt{vocabulary.DCTermsTitle, "Alpha"}))
			pendingAt(a, rev)
			h.c.coalescer.fire(t.Context())
			names := countedAnswer[graph.NameData](t, h, "byName", map[string]any{"name": "Alpha"}, rev)
			requireRelationship(t, "byName Alpha", nameIDs(names.Data.Matches), []string{a})
			requireDamaged(t, "byName Alpha after the "+remove.name, names, 0)
			// The key is over a thousand bytes, so only the status is printed.
			if health := h.c.Health(); !health.Healthy {
				t.Errorf("Health after the %s is %s with %d errors; want healthy", remove.name, health.Status, health.ErrorCount)
			}
		})
	}
}
