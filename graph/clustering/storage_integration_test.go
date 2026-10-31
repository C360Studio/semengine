//go:build integration

package clustering

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
)

// sampleCommunities is one group per level, every field set.
func sampleCommunities() []*Community {
	return []*Community{
		{
			ID: "acme.dep1.src.robotics", Level: 0, MemberCount: 3,
			TypeCounts:         map[string]int{"drone": 2, "sensor": 1},
			Keywords:           []string{"autonomous", "navigation"},
			StatisticalSummary: "Community of 3 entities including 2 drones, 1 sensor. Key themes: autonomous, navigation.",
			RepEntities:        []string{"acme.dep1.src.robotics.drone.d1", "acme.dep1.src.robotics.sensor.s1"},
		},
		{ID: "acme.dep1.src", Level: 1, MemberCount: 3, TypeCounts: map[string]int{"drone": 2, "sensor": 1}},
		{ID: "acme.dep1", Level: 2, MemberCount: 3, TypeCounts: map[string]int{"drone": 2, "sensor": 1}},
	}
}

// TestSaveAndGetAllCommunities ports the pin's TestIntegration_SaveAndGetCommunity and
// TestIntegration_CommunitySummaryFields: every field of a saved record reads back.
func TestSaveAndGetAllCommunities(t *testing.T) {
	_, storage := communityBucket(t)
	want := sampleCommunities()
	for _, c := range want {
		if err := storage.save(t.Context(), c); err != nil {
			t.Fatalf("save %s: %v", c.ID, err)
		}
	}
	got, err := storage.GetAllCommunities(t.Context())
	if err != nil {
		t.Fatalf("GetAllCommunities: %v", err)
	}
	// Sorted by key: 0.acme.dep1.src.robotics, 1.acme.dep1.src, 2.acme.dep1.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAllCommunities = %+v, want %+v", got, want)
	}
}

// TestCommunityKeyFormat ports the pin's TestIntegration_KeyFormatConsumerCompatibility:
// a record's key is {level}.{group ID}, and no entity mapping key is written.
func TestCommunityKeyFormat(t *testing.T) {
	_, storage := communityBucket(t)
	for _, c := range sampleCommunities() {
		if err := storage.save(t.Context(), c); err != nil {
			t.Fatalf("save %s: %v", c.ID, err)
		}
	}
	keys, err := storage.keys(t.Context())
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	want := []string{"0.acme.dep1.src.robotics", "1.acme.dep1.src", "2.acme.dep1"}
	if !slices.Equal(keys, want) {
		t.Fatalf("keys = %q, want %q", keys, want)
	}
}

// TestRemoveCommunity ports the pin's TestIntegration_DeleteCommunity: a removed record is
// gone, and removing a key that is not there is not an error.
func TestRemoveCommunity(t *testing.T) {
	_, storage := communityBucket(t)
	c := sampleCommunities()[0]
	if err := storage.save(t.Context(), c); err != nil {
		t.Fatalf("save: %v", err)
	}
	key := communityKey(c.Level, c.ID)
	if err := storage.remove(t.Context(), key); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := storage.remove(t.Context(), key); err != nil {
		t.Fatalf("second remove: %v", err)
	}
	got, err := storage.GetAllCommunities(t.Context())
	if err != nil || len(got) != 0 {
		t.Fatalf("GetAllCommunities after remove = %+v, %v; want none, nil", got, err)
	}
}

// TestGetAllCommunitiesEmptyBucket: an empty bucket gives no records and no error.
func TestGetAllCommunitiesEmptyBucket(t *testing.T) {
	_, storage := communityBucket(t)
	got, err := storage.GetAllCommunities(t.Context())
	if err != nil || len(got) != 0 {
		t.Fatalf("GetAllCommunities on an empty bucket = %+v, %v; want none, nil", got, err)
	}
}

// TestGetAllCommunitiesFailsOnCorruptEntry: an entry that is not a group record fails the
// read, naming its key; the pin skipped it (storage.go:291-299).
func TestGetAllCommunitiesFailsOnCorruptEntry(t *testing.T) {
	foreign, err := json.Marshal(sampleCommunities()[1])
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		key   string
		value []byte
	}{
		{"not JSON", "0.acme.dep1.src.zz", []byte("not a record")},
		{"another key's record", "1.acme.dep1.other", foreign},
		{"empty object", "2.acme.dep9", []byte("{}")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv, storage := communityBucket(t)
			if err := storage.save(t.Context(), sampleCommunities()[0]); err != nil {
				t.Fatalf("save: %v", err)
			}
			if _, err := kv.Put(t.Context(), tt.key, tt.value); err != nil {
				t.Fatalf("put corrupt entry: %v", err)
			}
			got, err := storage.GetAllCommunities(t.Context())
			if err == nil {
				t.Fatalf("GetAllCommunities = %+v, nil; want an error naming %s", got, tt.key)
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("error %q does not name key %s", err, tt.key)
			}
			if got != nil {
				t.Fatalf("GetAllCommunities returned records %+v with its error", got)
			}
		})
	}
}

// TestGetAllReportsCancel: a cancelled context returns the context's error, never an
// empty answer; the pin returned nil, nil (storage.go:276-279).
func TestGetAllReportsCancel(t *testing.T) {
	_, storage := communityBucket(t)
	for _, c := range sampleCommunities() {
		if err := storage.save(t.Context(), c); err != nil {
			t.Fatalf("save %s: %v", c.ID, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := storage.GetAllCommunities(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetAllCommunities(cancelled) = %+v, %v; want context.Canceled", got, err)
	}
	if !errs.IsTransient(err) {
		t.Fatalf("cancel error %v is not classified transient", err)
	}
}
