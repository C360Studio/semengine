//go:build integration

package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
)

// The large store TestSmoke_WarmRestartOverALargeStore restarts over: cargoEntities entities, each
// carrying one literal of cargoManifestBytes, about 142 MB stored in ENTITY_STATES, twice
// defaultMaxPending. At half that, about 71 MB, the boot's replays on one connection stayed under
// the limit on a Mac, so the test passed over the very fault it is for.
const (
	cargoEntities      = 16000
	cargoManifestBytes = 8 * 1024
	// cargoBatch is how many inputs are published before graph-ingest must acknowledge them all.
	cargoBatch = 1000
	// cargoPredicate ends in no suffix graph/entitytext reads as text, so graph-lexical indexes no
	// words of the manifest: the test measures the replay, not a word index of filler.
	cargoPredicate = "fleet.cargo.manifest"
	// defaultMaxPending is nats-server's default max_pending, 64 MiB: the server closes a
	// connection whose pending bytes pass it as a slow consumer. That is the pinned server's
	// behaviour as this test observes it, not a promise of nats-server's.
	defaultMaxPending = 64 << 20
)

// cargo is the i-th large entity: c360.<platform>.fleet.ops.cargo.<i>, with one manifest literal.
func (f fixture) cargo(i int) entityInput {
	id := f.id(fmt.Sprintf("cargo.%06d", i))
	prefix := fmt.Sprintf("%06d:", i)
	manifest := prefix + strings.Repeat("0123456789abcdef", cargoManifestBytes/16)[len(prefix):]
	return entityInput{id: id, statements: []statement{{cargoPredicate, manifest}}}
}

// publishBatch publishes one input for each of inputs without waiting on each acknowledgement in
// turn, then waits on every one, and returns the highest sequence the ENTITY stream acknowledged.
func (o observer) publishBatch(t *testing.T, inputs []entityInput) uint64 {
	t.Helper()
	acks := make([]jetstream.PubAckFuture, 0, len(inputs))
	for _, e := range inputs {
		ack, err := o.js.PublishAsync("entity."+e.id, encodeInput(t, e))
		if err != nil {
			t.Fatalf("publish %s: %v", e.id, err)
		}
		acks = append(acks, ack)
	}
	var last uint64
	for i, future := range acks {
		select {
		case ack := <-future.Ok():
			last = max(last, ack.Sequence)
		case err := <-future.Err():
			t.Fatalf("publish %s: %v", inputs[i].id, err)
		}
	}
	return last
}

// entityStatesBytes reads the bytes the ENTITY_STATES stream stores.
func (o observer) entityStatesBytes(t *testing.T) uint64 {
	t.Helper()
	kv, err := o.js.KeyValue(t.Context(), graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketEntityStates, err)
	}
	status, err := kv.Status(t.Context())
	if err != nil {
		t.Fatalf("status of %s: %v", graph.BucketEntityStates, err)
	}
	bucket, ok := status.(*jetstream.KeyValueBucketStatus)
	if !ok {
		t.Fatalf("status of %s is %T, want *jetstream.KeyValueBucketStatus", graph.BucketEntityStates, status)
	}
	return bucket.StreamInfo().State.Bytes
}

// statzSubject is the server's account-statistics request, imported into every account.
const statzSubject = "$SYS.REQ.ACCOUNT.PING.STATZ"

// defaultAccount is the account nats-server places every client in when it runs without an
// accounts configuration, as the fixture's server does.
const defaultAccount = "$G"

// slowConsumers reads the count of connections the server has closed as slow consumers in the
// default account. The count is the account's since the server started, so earlier tests on the
// shared server are in it: a caller compares two reads, never one against zero.
func (o observer) slowConsumers(t *testing.T) int {
	t.Helper()
	reply, err := o.client.RequestWithHeaders(t.Context(), statzSubject, nil, nil, requestTimeout)
	if err != nil {
		t.Fatalf("request %s: %v", statzSubject, err)
	}
	var statz struct {
		Data struct {
			Accounts []struct {
				Account       string `json:"acc"`
				SlowConsumers *int   `json:"slow_consumers"`
			} `json:"account_statz"`
		} `json:"data"`
	}
	if err := json.Unmarshal(reply.Data, &statz); err != nil {
		t.Fatalf("decode %s reply %s: %v", statzSubject, reply.Data, err)
	}
	for _, a := range statz.Data.Accounts {
		if a.Account != defaultAccount {
			continue
		}
		// An absent field would read as zero on both sides and hide every close.
		if a.SlowConsumers == nil {
			t.Fatalf("%s reply %s carries no slow_consumers for account %s", statzSubject, reply.Data, defaultAccount)
		}
		return *a.SlowConsumers
	}
	t.Fatalf("%s reply %s lists no account %s", statzSubject, reply.Data, defaultAccount)
	return 0
}

// awaitBootstrapped waits until key's GRAPH_STATUS envelope is one this child wrote (its entry
// revision above before, read before the child started), reports the initial build done
// (BootstrapComplete, which a new process publishes false until its snapshot is applied) and is
// indexed through revision.
func (o observer) awaitBootstrapped(t *testing.T, s stack, key string, before, revision uint64) {
	t.Helper()
	type envelope struct {
		entry  uint64
		status graph.IndexStatusResponse
	}
	await(t, s, fmt.Sprintf("%s/%s written by this process, bootstrapped, at revision %d", graph.BucketGraphStatus, key, revision),
		func(ctx context.Context) (envelope, error) {
			kv, err := o.js.KeyValue(ctx, graph.BucketGraphStatus)
			if err != nil {
				return envelope{}, err
			}
			entry, err := kv.Get(ctx, key)
			if err != nil {
				return envelope{}, err
			}
			var status graph.IndexStatusResponse
			err = json.Unmarshal(entry.Value(), &status)
			return envelope{entry: entry.Revision(), status: status}, err
		}, func(e envelope) bool {
			return e.entry > before && e.status.BootstrapComplete && e.status.IndexedRevision >= revision
		})
}

// TestSmoke_WarmRestartOverALargeStore holds composed-stack, "Restart over 66 MB of entity state":
// a stack seeded through graph-ingest with more than the server's default max_pending in
// ENTITY_STATES, stopped and started again on the same server with default settings, starts, every
// follower and the websocket view apply their snapshot, and the server closes no connection as a
// slow consumer during the boot. The shared server is used: it runs with nats-server's defaults,
// and the slow-consumer count is compared before and after the restart, never against zero.
func TestSmoke_WarmRestartOverALargeStore(t *testing.T) {
	url := shared.Reset(t)
	o := newObserver(t, url)
	first := startStack(t, o, url)
	f := fixture{platform: first.platform}
	requireComponentsListed(t, first)

	seeding := time.Now()
	for start := 0; start < cargoEntities; start += cargoBatch {
		var batch []entityInput
		for i := start; i < min(start+cargoBatch, cargoEntities); i++ {
			batch = append(batch, f.cargo(i))
		}
		last := o.publishBatch(t, batch)
		// The ENTITY stream keeps 64 MiB and discards its oldest inputs, so each batch is
		// acknowledged before the next is published: an input it discards is one already applied.
		o.awaitAcknowledged(t, first, last)
	}
	t.Logf("graph-ingest acknowledged %d inputs in %s", cargoEntities, time.Since(seeding).Round(time.Millisecond))
	revision := o.awaitIndexed(t, first)
	stored := o.entityStatesBytes(t)
	t.Logf("seeded %d entities, %d bytes in %s at revision %d, in %s", cargoEntities, stored,
		graph.BucketEntityStates, revision, time.Since(seeding).Round(time.Millisecond))
	if stored < defaultMaxPending {
		t.Fatalf("%s stores %d bytes, want at least the server's default max_pending, %d", graph.BucketEntityStates,
			stored, defaultMaxPending)
	}
	stopStack(t, first)

	// Read before the second child exists, so an envelope above these revisions is its own.
	before := map[string]uint64{}
	for _, key := range first.composition.indexKeys {
		before[key] = o.statusRevision(t, key)
	}
	slowBefore := o.slowConsumers(t)

	restarting := time.Now()
	// A failed Start exits the child, which fails this wait.
	second := restartStack(t, o, url, first)
	// graph-ingest's envelope from this process, and the websocket view: a client it accepts reads
	// the snapshot through revision, then the output's health reads healthy.
	o.awaitIndexed(t, second)
	for _, key := range second.composition.indexKeys {
		o.awaitBootstrapped(t, second, key, before[key], revision)
	}
	t.Logf("the restarted stack applied every snapshot in %s", time.Since(restarting).Round(time.Millisecond))
	// Ordered by the waits above: the server counts a slow consumer when it closes the connection,
	// and a reader whose connection was closed mid-replay reconnects and resumes before its
	// snapshot is applied, so every close during the boot is counted before the last wait returned.
	// That is the pinned server's behaviour as this test observes it, not a promise of nats-server's.
	if slowAfter := o.slowConsumers(t); slowAfter != slowBefore {
		t.Errorf("the server closed %d connection(s) as slow consumers during the restart (account %s count %d before, %d after)",
			slowAfter-slowBefore, defaultAccount, slowBefore, slowAfter)
	}
	stopStack(t, second)
}
