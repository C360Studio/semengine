package clustering

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/types"
)

// TestNewNATSCommunityStorageRefusesNil: a nil store is refused at construction rather
// than panicking at the first read.
func TestNewNATSCommunityStorageRefusesNil(t *testing.T) {
	storage, err := NewNATSCommunityStorage(nil)
	if storage != nil || !errs.IsInvalid(err) {
		t.Fatalf("NewNATSCommunityStorage(nil) = %v, %v; want nil and an invalid error", storage, err)
	}
}

// TestGetAllCommunitiesRefusesNilContext: a nil context is refused before any read.
func TestGetAllCommunitiesRefusesNilContext(t *testing.T) {
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	// The bucket is nil: a read through it would panic, so a refusal proves none happened.
	storage, err := NewNATSCommunityStorage(client.NewKVStore(nil))
	if err != nil {
		t.Fatal(err)
	}
	//nolint:staticcheck // SA1012: the nil context is the input under test.
	got, err := storage.GetAllCommunities(nil)
	if got != nil || !errs.IsInvalid(err) {
		t.Fatalf("GetAllCommunities(nil) = %v, %v; want nil and an invalid error", got, err)
	}
}

// TestCommunityKey: a record's key is its level, a dot, and its group ID (pin
// storage.go:518-520).
func TestCommunityKey(t *testing.T) {
	if got, want := communityKey(2, "acme.dep1"), "2.acme.dep1"; got != want {
		t.Fatalf("communityKey = %q, want %q", got, want)
	}
}

// TestLevelPrefix: a level's key prefix is the level and a dot, the prefix every record
// key at that level starts with; a level outside 0 to 2 is refused with an invalid error.
func TestLevelPrefix(t *testing.T) {
	for level, want := range []string{"0.", "1.", "2."} {
		got, err := LevelPrefix(level)
		if err != nil || got != want {
			t.Fatalf("LevelPrefix(%d) = %q, %v; want %q", level, got, err, want)
		}
	}
	for _, level := range []int{-1, 3} {
		got, err := LevelPrefix(level)
		if got != "" || !errs.IsInvalid(err) {
			t.Fatalf("LevelPrefix(%d) = %q, %v; want an invalid error", level, got, err)
		}
	}
}

// TestGroupKey: an entity's group key at a level is the level, a dot, and the ID's first
// four, three or two positions; a level outside 0 to 2 and an ID the canonical parser
// refuses are refused.
func TestGroupKey(t *testing.T) {
	id := "acme.dep1.src.docs.note.n1"
	for level, want := range []string{"0.acme.dep1.src.docs", "1.acme.dep1.src", "2.acme.dep1"} {
		got, err := GroupKey(id, level)
		if err != nil || got != want {
			t.Fatalf("GroupKey(%q, %d) = %q, %v; want %q", id, level, got, err, want)
		}
	}
	for _, tt := range []struct {
		id    string
		level int
	}{
		{id, -1}, {id, 3}, {"acme.dep1.src.docs.note", 0}, {"acme..src.docs.note.n1", 1}, {"", 2},
	} {
		got, err := GroupKey(tt.id, tt.level)
		if got != "" || !errs.IsInvalid(err) {
			t.Fatalf("GroupKey(%q, %d) = %q, %v; want an invalid error", tt.id, tt.level, got, err)
		}
	}
}

// TestDecodeCommunity: the value a real Maintainer stores at an entity's level-1 group
// key decodes to that group's record; a value that is not JSON, whose level or ID does not
// give its key, whose level is not 0 to 2 even at its own key, or whose entity_revision is
// 0 or missing is refused naming the key.
func TestDecodeCommunity(t *testing.T) {
	m, kv := newTestMaintainer(t)
	id := "acme.dep1.src.docs.note.n1"
	mustUpsert(t, m, id, 7, "alpha")
	m.BootstrapDone()
	mustTick(t, m)

	key, err := GroupKey(id, 1)
	if err != nil {
		t.Fatal(err)
	}
	kv.mu.Lock()
	value := kv.data[key]
	kv.mu.Unlock()
	got, err := DecodeCommunity(key, value)
	if err != nil {
		t.Fatalf("DecodeCommunity(%q): %v", key, err)
	}
	want := &Community{
		ID: "acme.dep1.src", Level: 1, MemberCount: 1, EntityRevision: 7, TypeCounts: map[string]int{"note": 1},
		Keywords: []string{"alpha"}, StatisticalSummary: "Community of 1 entities including 1 note. Key themes: alpha.",
		RepEntities: []string{id},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeCommunity(%q) = %+v, want %+v", key, got, want)
	}

	for _, tt := range []struct {
		name  string
		value string
	}{
		{"not JSON", `not a record`},
		{"wrong level", `{"id":"acme.dep1.src","level":0,"member_count":1,"entity_revision":7}`},
		{"wrong ID", `{"id":"acme.dep1.other","level":1,"member_count":1,"entity_revision":7}`},
		{"level 5", `{"id":"acme.dep1.src","level":5,"member_count":1,"entity_revision":7}`},
		{"revision 0", `{"id":"acme.dep1.src","level":1,"member_count":1,"entity_revision":0}`},
		{"no revision", `{"id":"acme.dep1.src","level":1,"member_count":1}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key := key
			if tt.name == "level 5" {
				key = "5.acme.dep1.src" // the record's own key: only the level is wrong
			}
			got, err := DecodeCommunity(key, []byte(tt.value))
			if got != nil || !errs.IsInvalid(err) {
				t.Fatalf("DecodeCommunity = %+v, %v; want nil and an invalid error", got, err)
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("error %q does not name key %s", err, key)
			}
		})
	}
}

// FuzzGroupKey: never panics; a key is the level, a dot and the ID's first 4-level
// positions, split here on '.', for a level 0 to 2 and an ID the canonical parser accepts;
// a refusal is an invalid error.
func FuzzGroupKey(f *testing.F) {
	for _, seed := range []struct {
		id    string
		level int
	}{
		{"acme.dep1.src.docs.note.n1", 0}, {"acme.dep1.src.docs.note.n1", 2}, {"acme.dep1.src.docs.note.n1", 3},
		{"acme.dep1.src.docs.note.n1", -1}, {"acme.dep1.src.docs.note", 1}, {"acme..src.docs.note.n1", 0},
		{"a.b.c.d.e.f.g", 0}, {"", 0}, {"a*.b.c.d.e.f", 1}, {strings.Repeat("a", 251) + ".b.c.d.e.f", 0},
	} {
		f.Add(seed.id, seed.level)
	}
	f.Fuzz(func(t *testing.T, id string, level int) {
		key, err := GroupKey(id, level)
		if err != nil {
			if !errs.IsInvalid(err) {
				t.Fatalf("GroupKey(%q, %d) refusal %v is not an invalid error", id, level, err)
			}
			return
		}
		if _, perr := types.ParseEntityID(id); perr != nil || level < 0 || level > 2 {
			t.Fatalf("GroupKey(%q, %d) = %q, want a refusal", id, level, key)
		}
		want := strconv.Itoa(level) + "." + strings.Join(strings.Split(id, ".")[:4-level], ".")
		if key != want {
			t.Fatalf("GroupKey(%q, %d) = %q, want %q", id, level, key, want)
		}
	})
}

// FuzzDecodeCommunity: never panics; an accepted record's level is 0 to 2, its level and
// ID give its key and its entity_revision is not 0; a refusal is an invalid error naming the key.
func FuzzDecodeCommunity(f *testing.F) {
	for _, seed := range [][2]string{
		{"1.acme.dep1.src", `{"id":"acme.dep1.src","level":1,"member_count":1,"entity_revision":7}`},
		{"1.acme.dep1.src", `{"id":"acme.dep1.src","level":0,"member_count":1,"entity_revision":7}`},
		{"1.acme.dep1.src", `{"id":"acme.dep1.src","level":1,"entity_revision":0}`},
		{"1.acme.dep1.src", `{"id":"acme.dep1.src","level":1,"entity_revision":-1}`},
		{"2.acme.dep1", `{"id":"acme.dep1","level":2,"entity_revision":18446744073709551615}`},
		{"5.x", `{"id":"x","level":5,"entity_revision":1}`}, {"-1.x", `{"id":"x","level":-1,"entity_revision":1}`},
		{"0.a.b.c.d", `null`}, {"0.a.b.c.d", `[]`}, {"", `{}`}, {"x", `not json`},
	} {
		f.Add(seed[0], []byte(seed[1]))
	}
	f.Fuzz(func(t *testing.T, key string, value []byte) {
		c, err := DecodeCommunity(key, value)
		if err != nil {
			if c != nil || !errs.IsInvalid(err) || !strings.Contains(err.Error(), key) {
				t.Fatalf("DecodeCommunity(%q) = %+v, %v; want nil and an invalid error naming the key", key, c, err)
			}
			return
		}
		if own := strconv.Itoa(c.Level) + "." + c.ID; own != key || c.EntityRevision == 0 || c.Level < 0 || c.Level > 2 {
			t.Fatalf("DecodeCommunity(%q) accepted %+v, whose key is %q", key, c, own)
		}
	})
}
