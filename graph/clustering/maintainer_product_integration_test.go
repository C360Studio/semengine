//go:build integration

package clustering_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/message"
)

// TestMaintainerAsAProduct drives the package from outside, as change 7's watch loop and a
// product will: build the storage and a Maintainer, feed entity changes, end the
// bootstrap, tick, and read COMMUNITY_INDEX back with no other SemEngine component.
func TestMaintainerAsAProduct(t *testing.T) {
	client := clustering.FixtureClient(t)
	kv, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketCommunityIndex)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := clustering.NewNATSCommunityStorage(client.NewKVStore(kv))
	if err != nil {
		t.Fatal(err)
	}
	m, err := clustering.NewMaintainer(storage)
	if err != nil {
		t.Fatal(err)
	}

	var revision uint64
	upsert := func(id, text string) {
		t.Helper()
		revision++
		state := &graph.EntityState{ID: id, Triples: []message.Triple{{Subject: id, Predicate: "docs.note.body", Object: text}}}
		if err := m.Upsert(id, revision, state); err != nil {
			t.Fatalf("Upsert(%s): %v", id, err)
		}
	}
	// Two sources of one deployment, two domains in the first.
	for i := range 3 {
		upsert(fmt.Sprintf("acme.dep1.gcs1.robotics.drone.d%d", i), "hovering steady")
		upsert(fmt.Sprintf("acme.dep1.gcs1.sensors.camera.c%d", i), "thermal imaging")
		upsert(fmt.Sprintf("acme.dep1.gcs2.robotics.drone.e%d", i), "landing")
	}
	revision++
	if err := m.Delete("acme.dep1.gcs1.sensors.camera.c2", revision); err != nil {
		t.Fatal(err)
	}
	m.BootstrapDone()
	if err := m.Tick(t.Context()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	communities, err := storage.GetAllCommunities(t.Context())
	if err != nil {
		t.Fatalf("GetAllCommunities: %v", err)
	}
	got := map[string]int{}
	for _, c := range communities {
		got[fmt.Sprintf("%d.%s", c.Level, c.ID)] = c.MemberCount
	}
	want := map[string]int{
		"0.acme.dep1.gcs1.robotics": 3,
		"0.acme.dep1.gcs1.sensors":  2,
		"0.acme.dep1.gcs2.robotics": 3,
		"1.acme.dep1.gcs1":          5,
		"1.acme.dep1.gcs2":          3,
		"2.acme.dep1":               8,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("member counts by key = %v, want %v", got, want)
	}
	for _, c := range communities {
		if c.ID == "acme.dep1.gcs1.sensors" {
			if !slices.Equal(c.Keywords, []string{"imaging", "thermal"}) ||
				!strings.HasPrefix(c.StatisticalSummary, "Community of 2 entities including 2 cameras") {
				t.Fatalf("sensors record = %+v", c)
			}
		}
	}
}
