# Probe: a KV watch across a NATS server restart (case 4)

Run 2026-11-08 on base `d15e6c4`, before the design. Versions: nats.go v1.55.0, nats-server image
`nats:2.15.0-alpine` (`internal/harness/natsfixture/nats-image`), Docker 29.9.2, macOS. The test file below was
copied into `graph/follower/` of a scratch copy of the tree (`cp -R` to `/tmp`), never into the claim worktree, and
run there:

```bash
go test -tags integration -count=1 -run TestProbeWatchAcrossBrokerRestart -v -timeout 6m ./graph/follower/
go test -tags integration -count=2 -run TestProbeWatchAcrossBrokerRestart -v -timeout 6m ./graph/follower/
```

The probe logs what it sees and asserts little; it covers a short outage only (the restart took 189 to 280 ms).

## Result

Three runs, all PASS. The watch did not close, the stream kept its creation time, and the follower read ready again
without a lost feed. Puts after the restart reached the watch 19.53 s, 19.56 s and 19.46 s after it.

## Source

Tabs are expanded to four spaces for the markdown linter; `gofmt` restores them.

```go
//go:build integration

package follower

// Probe for #557 case 4 (scratch, not for the tree): does a KV watch, as the follower holds it,
// survive a NATS server restart (file storage, same URL), and what does the follower report?

import (
    "context"
    "errors"
    "log/slog"
    "os"
    "testing"
    "time"

    "github.com/nats-io/nats.go/jetstream"

    "github.com/c360studio/semengine/graph"
    "github.com/c360studio/semengine/graph/kvcatalog"
    "github.com/c360studio/semengine/graph/readiness"
    "github.com/c360studio/semengine/internal/harness/natsfixture"
    "github.com/c360studio/semengine/metric"
    "github.com/c360studio/semengine/natsclient"
)

func openReconnecting(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
    // Production defaults (infinite reconnects), short reconnect wait.
    client, err := natsclient.NewClient(url, natsclient.WithReconnectWait(250*time.Millisecond), natsclient.WithHealthInterval(0))
    if err != nil {
        return nil, nil, err
    }
    if err := client.Connect(ctx); err != nil {
        return nil, nil, errors.Join(err, client.Close(ctx))
    }
    if err := client.WaitForConnection(ctx); err != nil {
        return nil, nil, errors.Join(err, client.Close(ctx))
    }
    return client, client.Close, nil
}

func TestProbeWatchAcrossBrokerRestart(t *testing.T) {
    ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
    defer cancel()
    // Own server: restarts the server mid-test.
    fx := natsfixture.New(t)
    if err := fx.Start(ctx); err != nil {
        t.Fatal(err)
    }
    client := natsfixture.Open(t, fx.URL(), openReconnecting)
    kv := entityStates(t, client)
    st, _ := kv.Status(ctx)
    info := st.(*jetstream.KeyValueBucketStatus).StreamInfo()
    t.Logf("PROBE ENTITY_STATES storage=%v created=%v lastSeq=%d", info.Config.Storage, info.Created, info.State.LastSeq)
    for i := 1; i <= 3; i++ {
        put(t, kv, entityID(i), storedEntity(entityID(i)))
    }

    // Raw nats.go watch, as follower.go:214 opens it.
    reader, err := kvcatalog.OpenCatalogReader(ctx, client, graph.BucketEntityStates)
    if err != nil {
        t.Fatal(err)
    }
    raw, err := reader.WatchAll(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer raw.Stop()
    rawSeen := map[string]uint64{}
    rawClosed := make(chan struct{})
    rawUpdates := make(chan string, 100)
    go func() {
        for e := range raw.Updates() {
            if e != nil {
                rawUpdates <- e.Key()
            }
        }
        close(rawClosed)
    }()

    var f *Follower
    f, err = Open(ctx, Config{
        Client: client, Key: readiness.KeyGraphIndex, MetricsRegistry: metric.NewMetricsRegistry(),
        Logger:          slog.New(slog.NewTextHandler(os.Stderr, nil)),
        StartupAttempts: 3, StartupInterval: 50 * time.Millisecond,
        Deliver: completing(&f), Failed: noFailures,
    })
    if err != nil {
        t.Fatal(err)
    }
    stop := start(t, f)
    waitReady := func(label string, bound time.Duration) {
        t0 := time.Now()
        deadline := time.After(bound)
        tick := time.NewTicker(250 * time.Millisecond)
        defer tick.Stop()
        for {
            select {
            case <-deadline:
                t.Logf("PROBE %s: NOT ready after %v: QueryReady=%v Stuck=%v Indexed=%d", label, bound, f.QueryReady(ctx), f.Stuck(), f.Indexed())
                return
            case <-tick.C:
                if err := f.QueryReady(ctx); err == nil {
                    t.Logf("PROBE %s: ready after %v Indexed=%d Stuck=%v", label, time.Since(t0).Round(time.Millisecond), f.Indexed(), f.Stuck())
                    return
                }
            }
        }
    }
    waitReady("before restart", 30*time.Second)

    before := fx.URL()
    t0 := time.Now()
    if err := fx.Restart(ctx); err != nil {
        t.Fatal(err)
    }
    t.Logf("PROBE restart took %v, url same=%v", time.Since(t0).Round(time.Millisecond), before == fx.URL())

    // Write after the restart through the fixture's own (new) connection.
    kv2, err := fx.JetStream().KeyValue(ctx, graph.BucketEntityStates)
    if err != nil {
        t.Fatal(err)
    }
    st2, _ := kv2.Status(ctx)
    info2 := st2.(*jetstream.KeyValueBucketStatus).StreamInfo()
    t.Logf("PROBE after restart storage=%v created=%v sameCreated=%v lastSeq=%d", info2.Config.Storage, info2.Created, info2.Created.Equal(info.Created), info2.State.LastSeq)
    for i := 4; i <= 5; i++ {
        rev, err := kv2.Put(ctx, entityID(i), storedEntity(entityID(i)))
        t.Logf("PROBE put %s after restart rev=%d err=%v", entityID(i), rev, err)
    }
    tw := time.Now()
    deadline := time.After(90 * time.Second)
    got := 0
loop:
    for {
        select {
        case k := <-rawUpdates:
            rawSeen[k]++
            if k == entityID(4) || k == entityID(5) {
                got++
                t.Logf("PROBE raw watch delivered %s %v after the restart", k, time.Since(tw).Round(time.Millisecond))
                if got == 2 {
                    break loop
                }
            }
        case <-rawClosed:
            t.Logf("PROBE raw watch CLOSED %v after the restart", time.Since(tw).Round(time.Millisecond))
            break loop
        case <-deadline:
            t.Logf("PROBE raw watch delivered nothing new within 90s")
            break loop
        }
    }
    waitReady("after restart", 90*time.Second)
    t.Logf("PROBE raw per-key deliveries (redelivery check) %v", rawSeen)
    t.Logf("PROBE follower Run returned %v", stop())
}
```

## Output, run 1 (`PROBE` lines and the verdict)

```text
    restart_probe_integration_test.go:53: PROBE ENTITY_STATES storage=File created=2026-11-09 02:46:46.208070178 +0000 UTC lastSeq=0
    restart_probe_integration_test.go:103: PROBE before restart: ready after 250ms Indexed=3 Stuck=[]
    restart_probe_integration_test.go:116: PROBE restart took 220ms, url same=true
    restart_probe_integration_test.go:125: PROBE after restart storage=File created=2026-11-09 02:46:46.208070178 +0000 UTC sameCreated=true lastSeq=3
    restart_probe_integration_test.go:128: PROBE put acme.ops.test.system.widget.004 after restart rev=4 err=<nil>
    restart_probe_integration_test.go:128: PROBE put acme.ops.test.system.widget.005 after restart rev=5 err=<nil>
    restart_probe_integration_test.go:140: PROBE raw watch delivered acme.ops.test.system.widget.004 19.526s after the restart
    restart_probe_integration_test.go:140: PROBE raw watch delivered acme.ops.test.system.widget.005 19.526s after the restart
    restart_probe_integration_test.go:103: PROBE after restart: ready after 251ms Indexed=5 Stuck=[]
    restart_probe_integration_test.go:154: PROBE raw per-key deliveries (redelivery check) map[acme.ops.test.system.widget.001:1 acme.ops.test.system.widget.002:1 acme.ops.test.system.widget.003:1 acme.ops.test.system.widget.004:1 acme.ops.test.system.widget.005:1]
    restart_probe_integration_test.go:155: PROBE follower Run returned <nil>
--- PASS: TestProbeWatchAcrossBrokerRestart (20.63s)
ok      github.com/c360studio/semengine/graph/follower  20.896s
```

## Output, runs 2 and 3

```text
    restart_probe_integration_test.go:53: PROBE ENTITY_STATES storage=File created=2026-11-09 02:47:18.469972096 +0000 UTC lastSeq=0
    restart_probe_integration_test.go:103: PROBE before restart: ready after 252ms Indexed=3 Stuck=[]
    restart_probe_integration_test.go:116: PROBE restart took 189ms, url same=true
    restart_probe_integration_test.go:125: PROBE after restart storage=File created=2026-11-09 02:47:18.469972096 +0000 UTC sameCreated=true lastSeq=3
    restart_probe_integration_test.go:128: PROBE put acme.ops.test.system.widget.004 after restart rev=4 err=<nil>
    restart_probe_integration_test.go:128: PROBE put acme.ops.test.system.widget.005 after restart rev=5 err=<nil>
    restart_probe_integration_test.go:140: PROBE raw watch delivered acme.ops.test.system.widget.004 19.555s after the restart
    restart_probe_integration_test.go:140: PROBE raw watch delivered acme.ops.test.system.widget.005 19.555s after the restart
    restart_probe_integration_test.go:103: PROBE after restart: ready after 251ms Indexed=5 Stuck=[]
    restart_probe_integration_test.go:154: PROBE raw per-key deliveries (redelivery check) map[acme.ops.test.system.widget.001:1 acme.ops.test.system.widget.002:1 acme.ops.test.system.widget.003:1 acme.ops.test.system.widget.004:1 acme.ops.test.system.widget.005:1]
    restart_probe_integration_test.go:155: PROBE follower Run returned <nil>
--- PASS: TestProbeWatchAcrossBrokerRestart (20.59s)
    restart_probe_integration_test.go:53: PROBE ENTITY_STATES storage=File created=2026-11-09 02:47:39.073574175 +0000 UTC lastSeq=0
    restart_probe_integration_test.go:103: PROBE before restart: ready after 252ms Indexed=3 Stuck=[]
    restart_probe_integration_test.go:116: PROBE restart took 280ms, url same=true
    restart_probe_integration_test.go:125: PROBE after restart storage=File created=2026-11-09 02:47:39.073574175 +0000 UTC sameCreated=true lastSeq=3
    restart_probe_integration_test.go:128: PROBE put acme.ops.test.system.widget.004 after restart rev=4 err=<nil>
    restart_probe_integration_test.go:128: PROBE put acme.ops.test.system.widget.005 after restart rev=5 err=<nil>
    restart_probe_integration_test.go:140: PROBE raw watch delivered acme.ops.test.system.widget.004 19.458s after the restart
    restart_probe_integration_test.go:140: PROBE raw watch delivered acme.ops.test.system.widget.005 19.458s after the restart
    restart_probe_integration_test.go:103: PROBE after restart: ready after 250ms Indexed=5 Stuck=[]
    restart_probe_integration_test.go:154: PROBE raw per-key deliveries (redelivery check) map[acme.ops.test.system.widget.001:1 acme.ops.test.system.widget.002:1 acme.ops.test.system.widget.003:1 acme.ops.test.system.widget.004:1 acme.ops.test.system.widget.005:1]
    restart_probe_integration_test.go:155: PROBE follower Run returned <nil>
--- PASS: TestProbeWatchAcrossBrokerRestart (20.60s)
ok      github.com/c360studio/semengine/graph/follower  41.677s
```

Full outputs, Docker lines included, were kept local only (`/tmp/smoke-design/probe/run1.txt`, `run2.txt`).
