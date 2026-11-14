package graphingest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// Requirement: graph-transport-boundary/A prefix ID listing reads no stored value
//
// TestPrefixIDsListsDamagedEntity: a prefix holding A, B and C, with B's stored value one that does
// not decode, lists all three and records nothing; the exact six-part prefix naming B lists B; and
// the full-entity prefix verb on the same data still refuses the whole page.
func TestPrefixIDsListsDamagedEntity(t *testing.T) {
	const (
		prefix = "acme.ops.test.system.widget"
		idA    = prefix + ".a"
		idB    = prefix + ".b"
		idC    = prefix + ".c"
	)
	c, bucket := poisonScopingTestComponent(t)
	seedValidBytes(bucket, idA, 1)
	seedPoisonBytes(bucket, idB, 2)
	seedValidBytes(bucket, idC, 3)

	reply, err := c.handleQueryPrefixIDsWithMaxPayload(context.Background(),
		prefixIDsRequest(t, graph.PrefixQueryRequest{Prefix: prefix}), 1<<20)
	if err != nil {
		t.Fatalf("prefixIDs refused: %v", err)
	}
	if want := `{"ids":["` + idA + `","` + idB + `","` + idC + `"],"total":3}`; string(reply) != want {
		t.Errorf("reply = %s, want %s", reply, want)
	}
	if _, recorded := poisonInventoryEntry(c, idB); recorded {
		t.Error("listing the prefix recorded B as damaged; a listing reads no stored value")
	}

	reply, err = c.handleQueryPrefixIDsWithMaxPayload(context.Background(),
		prefixIDsRequest(t, graph.PrefixQueryRequest{Prefix: idB}), 1<<20)
	if err != nil {
		t.Fatalf("prefixIDs on the exact ID refused: %v", err)
	}
	if want := `{"ids":["` + idB + `"],"total":1}`; string(reply) != want {
		t.Errorf("exact reply = %s, want %s", reply, want)
	}
	if _, recorded := poisonInventoryEntry(c, idB); recorded {
		t.Error("listing the exact ID recorded B as damaged; the existence check decodes nothing")
	}

	// The full-entity page keeps refusing: only the listing reads around damage.
	_, err = c.handleQueryPrefixWithMaxPayload(context.Background(),
		prefixIDsRequest(t, graph.PrefixQueryRequest{Prefix: prefix}), 1<<20)
	assertIngestResetRequired(t, err)
}

// TestPrefixIDsReadsNoStoredValue: with every value read failing, a prefix listing still answers,
// and asks the bucket for no value. A handler that read the entities would be refused.
func TestPrefixIDsReadsNoStoredValue(t *testing.T) {
	const prefix = "acme.ops.dom.sys.type"
	c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	seedValidBytes(bucket, prefix+".one", 1)
	seedValidBytes(bucket, prefix+".two", 2)
	var gets atomic.Int64
	bucket.getFunc = func(context.Context, string) (jetstream.KeyValueEntry, error) {
		gets.Add(1)
		return nil, errors.New("injected value read failure")
	}

	reply, err := c.handleQueryPrefixIDsWithMaxPayload(context.Background(),
		prefixIDsRequest(t, graph.PrefixQueryRequest{Prefix: "acme"}), 1<<20)
	if err != nil {
		t.Fatalf("prefixIDs refused: %v", err)
	}
	if want := `{"ids":["` + prefix + `.one","` + prefix + `.two"],"total":2}`; string(reply) != want {
		t.Errorf("reply = %s, want %s", reply, want)
	}
	if n := gets.Load(); n != 0 {
		t.Errorf("the listing read %d stored values, want 0", n)
	}
}

// TestPrefixIDsEmptyPageIsAnEmptyList: a prefix with nothing under it answers ids [], never null.
func TestPrefixIDsEmptyPageIsAnEmptyList(t *testing.T) {
	c, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	reply, err := c.handleQueryPrefixIDsWithMaxPayload(context.Background(),
		prefixIDsRequest(t, graph.PrefixQueryRequest{Prefix: "acme"}), 1<<20)
	if err != nil {
		t.Fatalf("prefixIDs refused: %v", err)
	}
	if want := `{"ids":[],"total":0}`; string(reply) != want {
		t.Errorf("reply = %s, want %s", reply, want)
	}
}

// TestPrefixIDsFirstKeyTooLarge: a maximum payload smaller than every reply holding the first key is
// refused as invalid response_too_large, naming the one-key reply's size and the maximum. Both
// replies are measured because the two-key reply carries no cursor and can be the smaller.
func TestPrefixIDsFirstKeyTooLarge(t *testing.T) {
	const (
		first  = "acme.ops.dom.sys.type.aaa"
		second = "acme.ops.dom.sys.type.bbb"
	)
	c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	seedValidBytes(bucket, first, 1)
	seedValidBytes(bucket, second, 2)
	oneKey := encodedPrefixIDs(t, graph.PrefixIDsResponse{IDs: []string{first}, Total: 2, NextCursor: cursorOf(first)})
	bothKeys := encodedPrefixIDs(t, graph.PrefixIDsResponse{IDs: []string{first, second}, Total: 2})
	maxPayload := int64(min(len(oneKey), len(bothKeys))) - 1

	reply, err := c.handleQueryPrefixIDsWithMaxPayload(context.Background(),
		prefixIDsRequest(t, graph.PrefixQueryRequest{Prefix: "acme"}), maxPayload)
	if err == nil {
		t.Fatalf("answered %s, want a refusal", reply)
	}
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorInvalid || classified.Code != "response_too_large" {
		t.Fatalf("refusal = %v, want class invalid, code response_too_large", err)
	}
	if got, want := classified.Detail["response_bytes"], int64(len(oneKey)); got != want {
		t.Errorf("response_bytes = %v (%T), want %d", got, got, want)
	}
	if got := classified.Detail["max_payload"]; got != maxPayload {
		t.Errorf("max_payload = %v (%T), want %d", got, got, maxPayload)
	}
}

// TestPrefixIDsLimitClamp: a limit of zero or less, or over 1000, means 1000; a limit within the
// range is kept. 1001 keys are the fewest that tell a clamp to 1000 from no clamp at all.
func TestPrefixIDsLimitClamp(t *testing.T) {
	c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	for i := range 1001 {
		seedValidBytes(bucket, fmt.Sprintf("acme.ops.dom.sys.type.k%04d", i), uint64(i+1))
	}
	for _, tt := range []struct{ limit, want int }{
		{-1, 1000}, {0, 1000}, {1, 1}, {999, 999}, {1000, 1000}, {1001, 1000},
	} {
		reply, err := c.handleQueryPrefixIDsWithMaxPayload(context.Background(),
			prefixIDsRequest(t, graph.PrefixQueryRequest{Prefix: "acme", Limit: tt.limit}), 1<<20)
		if err != nil {
			t.Fatalf("limit %d: refused: %v", tt.limit, err)
		}
		resp := decodePrefixIDs(t, reply)
		if len(resp.IDs) != tt.want || resp.Total != 1001 || resp.NextCursor == "" {
			t.Errorf("limit %d: %d ids, total %d, cursor %q; want %d ids, total 1001, a cursor",
				tt.limit, len(resp.IDs), resp.Total, resp.NextCursor, tt.want)
		}
	}
}

// walkHead is the type path most stored keys in the cursor-walk tests sit under.
const walkHead = "acme.ops.dom.sys.type"

// Requirement: graph-transport-boundary/A prefix ID listing pages as the prefix verb does
//
// TestPrefixIDsCursorWalkNamedCases runs the boundaries a generated draw might miss through the
// same walk as TestPrefixIDsCursorWalkMatchesSortedKeys, every time. It is its own test because a
// generated check refuses to start in a test that has already failed.
func TestPrefixIDsCursorWalkNamedCases(t *testing.T) {
	const head = walkHead
	witnesses := []struct {
		name       string
		stored     []string
		repeated   []string
		prefix     string
		limit      int
		maxPayload func(want []string) int64
	}{
		{
			// "acme.op" is a different first part from "acme.ops": the dot rule keeps it out.
			name:   "dot rule",
			stored: []string{"acme.op.dom.sys.type.x", "acme.ops.dom.sys.type.y", "acme.opsx.dom.sys.type.z"},
			prefix: "acme.ops",
		},
		{
			// Nothing is listed under the exact ID, so the ID itself is the page.
			name:   "exact key when nothing is under it",
			stored: []string{head + ".a", head + ".b"},
			prefix: head + ".a",
		},
		{
			// A key under the exact ID is listed, so the exact key is not added.
			name:   "no exact key when something is under it",
			stored: []string{head + ".a", head + ".a.child"},
			prefix: head + ".a",
		},
		{
			name:   "absent exact key",
			stored: []string{head + ".a"},
			prefix: head + ".b",
		},
		{
			name:     "a key listed twice is served once",
			stored:   []string{head + ".a", head + ".b", head + ".c"},
			repeated: []string{head + ".b"},
			prefix:   "acme",
			limit:    1,
		},
		{
			// The one-key page carries a long cursor; the two-key last page carries none and is
			// smaller. The longest run that fits is two keys, though one key does not fit.
			name:   "a longer run fits where a shorter one does not",
			stored: []string{head + "." + strings.Repeat("a", 60), head + ".b"},
			prefix: "acme",
			maxPayload: func(want []string) int64 {
				return int64(len(encodedPrefixIDs(t, graph.PrefixIDsResponse{IDs: want, Total: len(want)})))
			},
		},
	}
	for _, w := range witnesses {
		t.Run(w.name, func(t *testing.T) {
			c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
			for i, key := range w.stored {
				seedValidBytes(bucket, key, uint64(i+1))
			}
			listTwice(bucket, w.repeated)
			want := prefixIDsModel(w.stored, w.prefix)
			maxPayload := largestOneKeyReply(t, want)
			if w.maxPayload != nil {
				maxPayload = w.maxPayload(want)
			}
			walkPrefixIDs(t, c, w.prefix, w.limit, maxPayload, want)
		})
	}

}

// Requirement: graph-transport-boundary/A prefix ID listing pages as the prefix verb does
//
// TestPrefixIDsCursorWalkMatchesSortedKeys draws stored keys (some under the requested prefix, some
// beside it, some damaged, some listed twice), a limit from -1 to above 1000, and a maximum payload
// from the largest one-key reply upwards, often within two bytes of a page's reply, then walks the
// cursor to the end through walkPrefixIDs. The expected keys come from prefixIDsModel, which
// restates the spec's rules. Every assertion runs on every page of every case; the boundaries a
// draw might miss are TestPrefixIDsCursorWalkNamedCases.
func TestPrefixIDsCursorWalkMatchesSortedKeys(t *testing.T) {
	const head = walkHead
	rapid.Check(t, func(rt *rapid.T) {
		c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
		heads := []string{head, "acme.op.dom.sys.type", "acme.opsx.dom.sys.type"}
		instances := rapid.SliceOfN(rapid.StringMatching(`[a-z0-9]{1,40}`), 0, 25).Draw(rt, "instances")
		var stored, repeated []string
		for i, inst := range instances {
			key := rapid.SampledFrom(heads).Draw(rt, "head") + "." + inst
			if rapid.Bool().Draw(rt, "under-an-id") {
				key += ".child"
			}
			if _, dup := bucket.data[key]; dup {
				continue
			}
			stored = append(stored, key)
			if rapid.Bool().Draw(rt, "damaged") {
				seedPoisonBytes(bucket, key, uint64(i+1))
			} else {
				seedValidBytes(bucket, key, uint64(i+1))
			}
			if rapid.Bool().Draw(rt, "listed-twice") {
				repeated = append(repeated, key)
			}
		}
		listTwice(bucket, repeated)

		// Half the draws name a stored key's six-part ID, so the exact-key rule is reached as
		// often as the wide prefixes are.
		prefix := rapid.SampledFrom([]string{"", "acme", "acme.op", "acme.ops", head, head + ".absent"}).Draw(rt, "prefix")
		if len(stored) > 0 && rapid.Bool().Draw(rt, "exact-prefix") {
			prefix = strings.TrimSuffix(rapid.SampledFrom(stored).Draw(rt, "exact-key"), ".child")
		}
		limit := rapid.OneOf(rapid.IntRange(-1, 6), rapid.IntRange(998, 1002)).Draw(rt, "limit")
		want := prefixIDsModel(stored, prefix)

		// The maximum is either some spare bytes over the largest one-key reply, or within two
		// bytes of the first page's reply for a drawn run length, where a size counted one or
		// two bytes wrong picks a page that does not fit or stops a page short.
		maxPayload := largestOneKeyReply(rt, want)
		if len(want) > 0 && rapid.Bool().Draw(rt, "near-a-boundary") {
			run := rapid.IntRange(1, len(want)).Draw(rt, "run")
			reply := graph.PrefixIDsResponse{IDs: want[:run], Total: len(want)}
			if run < len(want) {
				reply.NextCursor = cursorOf(want[run-1])
			}
			near := int64(len(encodedPrefixIDs(rt, reply)) + rapid.IntRange(-2, 2).Draw(rt, "offset"))
			maxPayload = max(maxPayload, near)
		} else {
			maxPayload += int64(rapid.IntRange(0, 400).Draw(rt, "spare-bytes"))
		}

		walkPrefixIDs(rt, c, prefix, limit, maxPayload, want)

		c.entityPoisonMu.Lock()
		recorded := len(c.entityPoison)
		c.entityPoisonMu.Unlock()
		if recorded != 0 {
			rt.Fatalf("the walk recorded %d damaged entities, want 0", recorded)
		}
	})
}

// prefixIDsModel restates which keys a prefix selects, in the spec's words and not the handler's
// code: an empty prefix selects every key; otherwise a key is under the prefix when it starts with
// the prefix and a dot; when nothing is under it, a stored key equal to the prefix is the one
// selected. The result is sorted, each key once.
func prefixIDsModel(stored []string, prefix string) []string {
	var selected []string
	for _, key := range stored {
		if prefix == "" || strings.HasPrefix(key, prefix+".") {
			selected = append(selected, key)
		}
	}
	if len(selected) == 0 && prefix != "" && slices.Contains(stored, prefix) {
		selected = []string{prefix}
	}
	sort.Strings(selected)
	return slices.Compact(selected)
}

// modelLimit restates the limit rule: zero or less, or over 1000, means 1000.
func modelLimit(limit int) int {
	if limit <= 0 || limit > 1000 {
		return 1000
	}
	return limit
}

// fatalfer is what walkPrefixIDs needs from a *testing.T or a *rapid.T.
type fatalfer interface {
	Helper()
	Fatalf(format string, args ...any)
}

// walkPrefixIDs follows prefixIDs from no cursor until next_cursor is empty, and checks every reply
// against want, the sorted distinct keys the request selects.
func walkPrefixIDs(tb fatalfer, c *Component, prefix string, limit int, maxPayload int64, want []string) {
	tb.Helper()
	pageLimit := modelLimit(limit)
	var joined []string
	cursor := ""
	for page := 0; ; page++ {
		if page > len(want) {
			tb.Fatalf("the walk did not end after %d pages for %d keys", page, len(want))
		}
		request, err := json.Marshal(graph.PrefixQueryRequest{Prefix: prefix, Limit: limit, Cursor: cursor})
		if err != nil {
			tb.Fatalf("marshal request: %v", err)
		}
		reply, err := c.handleQueryPrefixIDsWithMaxPayload(context.Background(), request, maxPayload)
		if err != nil {
			tb.Fatalf("page %d: refused: %v", page, err)
		}
		if int64(len(reply)) > maxPayload {
			tb.Fatalf("page %d: reply is %d bytes, over the maximum %d", page, len(reply), maxPayload)
		}
		var resp graph.PrefixIDsResponse
		decoder := json.NewDecoder(bytes.NewReader(reply))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&resp); err != nil {
			tb.Fatalf("page %d: decode %s: %v", page, reply, err)
		}
		if resp.IDs == nil {
			tb.Fatalf("page %d: ids is null or missing in %s", page, reply)
		}
		if resp.Total != len(want) {
			tb.Fatalf("page %d: total %d, want %d", page, resp.Total, len(want))
		}
		remaining := want[len(joined):]
		if len(resp.IDs) > len(remaining) || !slices.Equal(resp.IDs, remaining[:len(resp.IDs)]) {
			tb.Fatalf("page %d: ids %v are not the leading run of %v", page, resp.IDs, remaining)
		}
		if len(remaining) > 0 && len(resp.IDs) == 0 {
			tb.Fatalf("page %d: empty page with %d keys left", page, len(remaining))
		}
		if len(resp.IDs) > pageLimit {
			tb.Fatalf("page %d: %d ids, over the limit %d", page, len(resp.IDs), pageLimit)
		}
		// The page is the longest run that fits: every longer run within the limit, encoded by the
		// real encoder with its own cursor, is over the maximum.
		for longer := len(resp.IDs) + 1; longer <= min(pageLimit, len(remaining)); longer++ {
			candidate := graph.PrefixIDsResponse{IDs: remaining[:longer], Total: len(want)}
			if longer < len(remaining) {
				candidate.NextCursor = cursorOf(remaining[longer-1])
			}
			if size := len(encodedPrefixIDs(tb, candidate)); int64(size) <= maxPayload {
				tb.Fatalf("page %d: %d ids answered, but %d ids encode to %d bytes, within the maximum %d",
					page, len(resp.IDs), longer, size, maxPayload)
			}
		}
		joined = append(joined, resp.IDs...)
		wantCursor := ""
		if len(joined) < len(want) {
			wantCursor = cursorOf(joined[len(joined)-1])
		}
		if resp.NextCursor != wantCursor {
			tb.Fatalf("page %d: next_cursor %q, want %q", page, resp.NextCursor, wantCursor)
		}
		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	if !slices.Equal(joined, want) {
		tb.Fatalf("pages joined = %v, want %v", joined, want)
	}
}

// largestOneKeyReply is the size of the largest reply a page holding one key can have, so a
// maximum payload at least this large lets every page hold its first key.
func largestOneKeyReply(tb fatalfer, want []string) int64 {
	tb.Helper()
	largest := len(encodedPrefixIDs(tb, graph.PrefixIDsResponse{IDs: []string{}, Total: len(want)}))
	for i, key := range want {
		resp := graph.PrefixIDsResponse{IDs: []string{key}, Total: len(want)}
		if i < len(want)-1 {
			resp.NextCursor = cursorOf(key)
		}
		largest = max(largest, len(encodedPrefixIDs(tb, resp)))
	}
	return int64(largest)
}

// cursorOf is the cursor a page ending at key carries: the key, base64url without padding. The
// test spells the encoding out to size and check the replies; callers treat it as opaque.
func cursorOf(key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(key))
}

// listTwice makes the bucket's listing name each key in repeated twice, as a listing under
// concurrent overwrites can.
func listTwice(bucket *mockKVBucket, repeated []string) {
	if len(repeated) == 0 {
		return
	}
	bucket.listFilteredFunc = func(_ context.Context, filters ...string) (jetstream.KeyLister, error) {
		bucket.mu.Lock()
		defer bucket.mu.Unlock()
		var listed []string
		for key := range bucket.data {
			if matchesNATSFilters(key, filters) {
				listed = append(listed, key)
				if slices.Contains(repeated, key) {
					listed = append(listed, key)
				}
			}
		}
		ch := make(chan string, len(listed))
		for _, key := range listed {
			ch <- key
		}
		close(ch)
		return &mockKeyLister{ch: ch}, nil
	}
}

func prefixIDsRequest(t *testing.T, req graph.PrefixQueryRequest) []byte {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return data
}

func encodedPrefixIDs(tb fatalfer, resp graph.PrefixIDsResponse) []byte {
	tb.Helper()
	data, err := json.Marshal(resp)
	if err != nil {
		tb.Fatalf("marshal reply: %v", err)
	}
	return data
}

func decodePrefixIDs(t *testing.T, reply []byte) graph.PrefixIDsResponse {
	t.Helper()
	var resp graph.PrefixIDsResponse
	if err := json.Unmarshal(reply, &resp); err != nil {
		t.Fatalf("decode %s: %v", reply, err)
	}
	return resp
}
