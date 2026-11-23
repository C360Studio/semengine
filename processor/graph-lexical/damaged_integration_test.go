//go:build integration

package graphlexical

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// searchPast sends body with min_revision rev until graph-lexical answers or refuses it with
// anything but a transient refusal, and requires an answer.
func searchPast(t *testing.T, client *natsclient.Client, body string, rev uint64) graph.QueryResponse[graph.LexicalSearchData] {
	t.Helper()
	request := strings.TrimSuffix(body, "}") + fmt.Sprintf(`,"min_revision":%d}`, rev)
	type outcome struct {
		raw []byte
		err error
	}
	got := await(t, "search "+request+" settles", func(context.Context) (outcome, error) {
		raw, err := ask(t, client, request)
		return outcome{raw, err}, nil
	}, func(o outcome) bool { return o.err == nil || !errs.IsTransient(o.err) })
	if got.err != nil {
		t.Fatalf("search %s: %v", request, got.err)
	}
	var reply graph.QueryResponse[graph.LexicalSearchData]
	if err := json.Unmarshal(got.raw, &reply); err != nil {
		t.Fatalf("search %s: reply %s: %v", request, got.raw, err)
	}
	return reply
}

// putRaw writes value under key as it is, and returns its revision.
func putRaw(t *testing.T, kv jetstream.KeyValue, key string, value []byte) uint64 {
	t.Helper()
	rev, err := kv.Put(t.Context(), key, value)
	if err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
	return rev
}

// TestSearchPastDamagedEntity: B's stored value does not decode. graph-lexical keeps answering: a
// search for "pump" answers A alone with damaged_entities 1, and one scoped to a prefix B is not
// under counts nothing. Overwritten after start, B stops being a hit; written valid again, it is
// a hit with no count. Each search waits for B's revision through min_revision.
func TestSearchPastDamagedEntity(t *testing.T) {
	const (
		a = "c360.lex.test.north.pump.a"
		b = "c360.lex.test.south.pump.b"
	)
	damaged := []byte("{not json")
	requirePastDamage := func(t *testing.T, client *natsclient.Client, rev uint64) {
		t.Helper()
		reply := searchPast(t, client, `{"query":"pump"}`, rev)
		if ids := resultIDs(reply.Data); !slices.Equal(ids, []string{a}) || reply.DamagedEntities != 1 {
			t.Errorf("search pump = %v, damaged_entities %d; want [%s], 1", ids, reply.DamagedEntities, a)
		}
		scoped := searchPast(t, client, `{"query":"pump","scope":["c360.lex.test.north"]}`, rev)
		if ids := resultIDs(scoped.Data); !slices.Equal(ids, []string{a}) || scoped.DamagedEntities != 0 {
			t.Errorf("search pump in north = %v, damaged_entities %d; want [%s], 0", ids, scoped.DamagedEntities, a)
		}
	}

	t.Run("stored before start", func(t *testing.T) {
		client := fixtureClient(t)
		states := ensureEntityStates(t, client)
		put(t, states, a, text{name: "pump"})
		rev := putRaw(t, states, b, damaged)
		c := startLexical(t, client, metric.NewMetricsRegistry(), nil)
		requirePastDamage(t, client, rev)
		if health := c.Health(); health.Healthy || !strings.Contains(health.LastError, b) {
			t.Errorf("Health = %+v; want not healthy, naming %s", health, b)
		}
	})

	t.Run("overwritten, then repaired", func(t *testing.T) {
		client := fixtureClient(t)
		states := ensureEntityStates(t, client)
		c := startLexical(t, client, metric.NewMetricsRegistry(), nil)
		put(t, states, a, text{name: "pump"})
		rev := put(t, states, b, text{name: "pump"})
		if ids := resultIDs(searchPast(t, client, `{"query":"pump"}`, rev).Data); len(ids) != 2 {
			t.Fatalf("search pump before the damage = %v; want both", ids)
		}
		requirePastDamage(t, client, putRaw(t, states, b, damaged))
		if got := c.Health().ErrorCount; got != 1 {
			t.Errorf("ErrorCount = %d after one damaged entity; want 1", got)
		}

		repaired := searchPast(t, client, `{"query":"pump"}`, put(t, states, b, text{name: "pump"}))
		if ids := resultIDs(repaired.Data); len(ids) != 2 || repaired.DamagedEntities != 0 {
			t.Errorf("search pump after the repair = %v, damaged_entities %d; want both, 0", ids, repaired.DamagedEntities)
		}
		// The reply above waited for the repair's revision, whose completion cleared the record.
		if health := c.Health(); !health.Healthy {
			t.Errorf("Health after the repair = %+v; want healthy", health)
		}
	})
}
