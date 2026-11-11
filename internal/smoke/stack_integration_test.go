//go:build integration

package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	graphgateway "github.com/c360studio/semengine/gateway/graph-gateway"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/internal/harness/prochost"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	graphclustering "github.com/c360studio/semengine/processor/graph-clustering"
	graphembedding "github.com/c360studio/semengine/processor/graph-embedding"
	graphindex "github.com/c360studio/semengine/processor/graph-index"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	graphquery "github.com/c360studio/semengine/processor/graph-query"
	"github.com/c360studio/semengine/service"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

// stackConfig is the one configuration the six-component stack boots from (design D2), as a
// product would write it. The child fills the three placeholders from its environment: the NATS
// URL and the two ports the test picked (design D4), and graph-ingest's configuration, {} (its
// defaults) unless the test passes one. Every other component takes its defaults, except that
// graph-embedding runs BM25 only and graph-clustering publishes the changed groups every second
// rather than every 30 s, so a community wait lasts about a second.
const stackConfig = `{
  "version": "1.0.0",
  "platform": {"org": "c360", "id": "smoke", "type": "test", "environment": "test"},
  "nats": {"urls": ["{{nats_url}}"]},
  "services": {
    "service-manager": {"enabled": true, "config": {"http_port": {{http_port}}}},
    "metrics": {"enabled": true, "config": {"path": "/metrics", "port": {{metrics_port}}}}
  },
  "streams": {
    "ENTITY": {"subjects": ["entity.>"], "storage": "file", "max_age": "24h", "max_bytes": 67108864,
      "discard": "old"}
  },
  "components": {
    "graph-ingest": {"type": "processor", "name": "graph-ingest", "enabled": true, "config": {{ingest_config}}},
    "graph-index": {"type": "processor", "name": "graph-index", "enabled": true, "config": {}},
    "graph-embedding": {"type": "processor", "name": "graph-embedding", "enabled": true,
      "config": {"embedder_type": "bm25"}},
    "graph-clustering": {"type": "processor", "name": "graph-clustering", "enabled": true,
      "config": {"detection_interval": "1s"}},
    "graph-query": {"type": "processor", "name": "graph-query", "enabled": true, "config": {}},
    "graph-gateway": {"type": "gateway", "name": "graph-gateway", "enabled": true, "config": {}}
  }
}`

// graphOnlyConfig is stackConfig with no graph-embedding: the configuration of a product that
// answers by name, prefix and structure and runs no embedder (composed-stack, "The graph stack
// answers fusion without an embedder").
const graphOnlyConfig = `{
  "version": "1.0.0",
  "platform": {"org": "c360", "id": "smoke", "type": "test", "environment": "test"},
  "nats": {"urls": ["{{nats_url}}"]},
  "services": {
    "service-manager": {"enabled": true, "config": {"http_port": {{http_port}}}},
    "metrics": {"enabled": true, "config": {"path": "/metrics", "port": {{metrics_port}}}}
  },
  "streams": {
    "ENTITY": {"subjects": ["entity.>"], "storage": "file", "max_age": "24h", "max_bytes": 67108864,
      "discard": "old"}
  },
  "components": {
    "graph-ingest": {"type": "processor", "name": "graph-ingest", "enabled": true, "config": {{ingest_config}}},
    "graph-index": {"type": "processor", "name": "graph-index", "enabled": true, "config": {}},
    "graph-query": {"type": "processor", "name": "graph-query", "enabled": true, "config": {}}
  }
}`

// composition is one stack the child can boot: its configuration, and what the composition root
// and the test know about it.
type composition struct {
	// name selects the composition in the child's environment (envComposition).
	name   string
	config string
	// components are the component instances config declares, sorted by name. The child
	// registers exactly these factories, and the health reply must list exactly these.
	components []string
	// indexKeys are the GRAPH_STATUS keys awaitIndexed waits on after graph-ingest's: the
	// indexes the composition runs.
	indexKeys []string
	// labels are predicates the composition root registers as display names before StartAll;
	// graph-index reads them once when it starts, and its name lookup indexes only them.
	labels []string
}

// fullStack is the six-component stack every test but the no-embedder one boots. graph-clustering's
// key counts what its maintainer holds, not what it has published (readiness.KeyGraphClustering), so
// awaitIndexed's wait on it orders no community read: checkCommunities waits on the record's own
// entity_revision.
var fullStack = composition{
	name:       "full",
	config:     stackConfig,
	components: []string{"graph-clustering", "graph-embedding", "graph-gateway", "graph-index", "graph-ingest", "graph-query"},
	indexKeys:  []string{readiness.KeyGraphIndex, readiness.KeyGraphEmbedding, readiness.KeyGraphClustering},
}

// graphOnlyStack is the stack with no embedder: graph-embedding neither configured nor registered.
// Its consumer resolves by name, so namePredicate is a display name.
var graphOnlyStack = composition{
	name:       "graph-only",
	config:     graphOnlyConfig,
	components: []string{"graph-index", "graph-ingest", "graph-query"},
	indexKeys:  []string{readiness.KeyGraphIndex},
	labels:     []string{namePredicate},
}

// compositionNamed returns the composition called name; the test always passes one.
func compositionNamed(name string) (composition, error) {
	for _, c := range []composition{fullStack, graphOnlyStack} {
		if c.name == name {
			return c, nil
		}
	}
	return composition{}, fmt.Errorf("no composition named %q", name)
}

// componentFactories are the factories a composition can name, by component name.
var componentFactories = map[string]func(*component.Registry) error{
	"graph-ingest":     graphingest.Register,
	"graph-index":      graphindex.Register,
	"graph-embedding":  graphembedding.Register,
	"graph-clustering": graphclustering.Register,
	"graph-query":      graphquery.Register,
	"graph-gateway":    graphgateway.Register,
}

// What the test passes the child in its environment.
const (
	envNATSURL     = "SMOKE_NATS_URL"
	envHTTPPort    = "SMOKE_HTTP_PORT"
	envMetricsPort = "SMOKE_METRICS_PORT"
	// envComposition is the name of the composition the child boots.
	envComposition = "SMOKE_COMPOSITION"
	// envReady is the file the child creates once StartAll returned.
	envReady = "SMOKE_READY"
	// envIngestConfig is graph-ingest's configuration, JSON; unset means its defaults.
	envIngestConfig = "SMOKE_INGEST_CONFIG"
	// envHold names, by its last two ID positions, the one entity whose application the child
	// holds for good (design D6, case 2); unset holds nothing.
	envHold = "SMOKE_HOLD"
)

// stackHelper is the prochost helper that runs the composition root.
const stackHelper = "smoke-stack"

// stopBudget bounds each of the child's three stops. A stop is terminal work, so it gets its own
// finite authority; at an hour it is an unreachable fallback, so a stop that hangs ends at go test
// -timeout instead of passing as an error the test reads (docs/testing.md, #500).
const stopBudget = time.Hour

// requestTimeout is one request's reply timeout, which natsclient requires. At an hour it is an
// unreachable fallback: a reply that never comes ends at go test -timeout (docs/testing.md, #500).
const requestTimeout = time.Hour

// ingestConsumer is graph-ingest's durable consumer on the ENTITY stream's entity.> subject.
const ingestConsumer = "graph-ingest-entity-wildcard"

// smokeEntityType is the payload type the producer publishes; the composition root registers it with
// smokeIndexingProfile, which graph-ingest stamps on each entity it creates.
var smokeEntityType = message.Type{Domain: "smoke", Category: "entity", Version: "v1"}

// smokeIndexingProfile is the fixed entities' profile: named things, the retrieval corpus.
const smokeIndexingProfile = vocabulary.IndexingProfileContent

// smokeEntity is the producer's payload: one entity and its statements, a graph.Graphable.
type smokeEntity struct {
	ID         string           `json:"id"`
	Statements []message.Triple `json:"triples"`
}

func (e *smokeEntity) EntityID() string          { return e.ID }
func (e *smokeEntity) Triples() []message.Triple { return e.Statements }
func (e *smokeEntity) Schema() message.Type      { return smokeEntityType }
func (e *smokeEntity) Validate() error           { return nil }

// MarshalJSON and UnmarshalJSON marshal only the payload's own fields, through a type alias.
func (e *smokeEntity) MarshalJSON() ([]byte, error) {
	type plain smokeEntity
	return json.Marshal((*plain)(e))
}

func (e *smokeEntity) UnmarshalJSON(data []byte) error {
	type plain smokeEntity
	return json.Unmarshal(data, (*plain)(e))
}

// TestHelperProcess runs the stack in a child the test started with prochost.Start; in a normal run
// the Helper call returns at once.
func TestHelperProcess(t *testing.T) {
	prochost.Helper(stackHelper, func() {
		if err := runStack(t.Context()); err != nil {
			fmt.Fprintf(os.Stderr, "smoke stack: %v\n", err)
			os.Exit(1)
		}
	})
}

// runStack is the composition root (design D2): what a product's main does, in order. It boots
// the composition envComposition names from its configuration, creates the ready file, waits for
// SIGTERM and stops everything it started, each stop under its own budget.
func runStack(ctx context.Context) (result error) {
	terminated := make(chan os.Signal, 1)
	signal.Notify(terminated, syscall.SIGTERM)
	var handler slog.Handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	// Only case 2 sets envHold; every other boot runs with the plain logger.
	if local := os.Getenv(envHold); local != "" {
		handler = &holdOnIngested{inner: handler, suffix: ".fleet.ops." + local}
	}
	logger := slog.New(handler)
	stop := func(what string, fn func(context.Context) error) {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopBudget)
		defer cancel()
		if err := fn(stopCtx); err != nil {
			result = errors.Join(result, fmt.Errorf("%s: %w", what, err))
		}
	}

	stackComposition, err := compositionNamed(os.Getenv(envComposition))
	if err != nil {
		return err
	}
	ingestConfig := os.Getenv(envIngestConfig)
	if ingestConfig == "" {
		ingestConfig = "{}"
	}
	raw := strings.NewReplacer(
		"{{nats_url}}", os.Getenv(envNATSURL),
		"{{http_port}}", os.Getenv(envHTTPPort),
		"{{metrics_port}}", os.Getenv(envMetricsPort),
		"{{ingest_config}}", ingestConfig,
	).Replace(stackComposition.config)
	cfg, err := config.NewLoader().LoadFromBytes([]byte(raw))
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}

	client, err := natsclient.NewClient(strings.Join(cfg.NATS.URLs, ","), natsclient.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("NATS client: %w", err)
	}
	defer stop("close the NATS client", client.Close)
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return fmt.Errorf("wait for the NATS connection: %w", err)
	}

	configManager, err := config.NewConfigManager(cfg, client, logger)
	if err != nil {
		return fmt.Errorf("config manager: %w", err)
	}
	if err := configManager.Start(ctx); err != nil {
		return fmt.Errorf("start the config manager: %w", err)
	}
	defer stop("stop the config manager", configManager.Stop)
	effective := configManager.GetConfig().Get()

	if err := config.NewStreamsManager(client, logger).EnsureStreams(ctx, effective); err != nil {
		return fmt.Errorf("ensure streams: %w", err)
	}

	components := component.NewRegistry()
	for _, name := range stackComposition.components {
		if err := componentFactories[name](components); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}
	for _, predicate := range stackComposition.labels {
		vocabulary.Register(predicate, vocabulary.WithAlias(vocabulary.AliasTypeLabel, 0))
	}

	payloads := payloadregistry.New()
	if err := message.RegisterPayloads(payloads); err != nil {
		return fmt.Errorf("register the message payloads: %w", err)
	}
	if err := payloads.Register(&payloadregistry.Registration{
		Domain: smokeEntityType.Domain, Category: smokeEntityType.Category, Version: smokeEntityType.Version,
		Description:     "the smoke test's entity payload",
		IndexingProfile: smokeIndexingProfile,
		Factory:         func() any { return &smokeEntity{} },
	}); err != nil {
		return fmt.Errorf("register the smoke entity payload: %w", err)
	}

	services := service.NewServiceRegistry()
	if err := service.RegisterAll(services); err != nil {
		return fmt.Errorf("register the services: %w", err)
	}
	manager := service.NewServiceManager(services)
	if err := manager.ConfigureFromServices(effective.Services, &service.Dependencies{
		NATSClient:        client,
		MetricsRegistry:   metric.NewMetricsRegistry(),
		Logger:            logger,
		Platform:          types.PlatformMeta{Org: effective.Platform.Org, Platform: effective.Platform.ID},
		Manager:           configManager,
		ComponentRegistry: components,
		PayloadRegistry:   payloads,
	}); err != nil {
		return fmt.Errorf("configure the services: %w", err)
	}
	if err := manager.StartAll(ctx); err != nil {
		stop("stop the services after a failed start", manager.StopAll)
		return fmt.Errorf("start the services: %w", err)
	}
	defer stop("stop the services", manager.StopAll)

	// The ready file carries the platform ID config.Manager established: minted from the declared
	// stem at the first boot and adopted after (ADR-104), it is position 2 of every entity ID.
	ready := os.Getenv(envReady)
	if err := os.WriteFile(ready+".tmp", []byte(effective.Platform.ID), 0o600); err != nil {
		return err
	}
	if err := os.Rename(ready+".tmp", ready); err != nil {
		return err
	}
	<-terminated
	logger.Info("smoke stack: SIGTERM received, stopping")
	return nil
}

// holdOnIngested is the child's log handler: it passes every record to inner, except that
// graph-ingest's "Entity ingested" record for the entity whose ID ends in suffix, which
// ingestEntity logs after the entity's write and before its applied-sequence record and its
// acknowledgement (processor/graph-ingest/ingest.go, keyed_ingest.go), never returns. The lane
// applying that input stays there until the test kills the process: the hold of design D6, case 2,
// as settlement_integration_test.go's pauseOnRecord, never released. The suffix is the ID's fixed
// positions 3 to 6; positions 1 and 2 are the deployment's, which config.Manager establishes after
// the logger exists. An empty suffix holds nothing.
type holdOnIngested struct {
	inner  slog.Handler
	suffix string
}

// heldRecord is graph-ingest's record the hold waits on.
const heldRecord = "Entity ingested"

// forever is never closed: the held lane receives from it until the process is killed.
var forever = make(chan struct{})

// Enabled lets every record through to Handle, so the hold sees the Debug record inner drops.
func (h *holdOnIngested) Enabled(context.Context, slog.Level) bool { return true }

func (h *holdOnIngested) Handle(ctx context.Context, record slog.Record) error {
	if h.suffix != "" && record.Message == heldRecord {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "entity_id" && strings.HasSuffix(attr.Value.String(), h.suffix) {
				<-forever
			}
			return true
		})
	}
	if !h.inner.Enabled(ctx, record.Level) {
		return nil
	}
	return h.inner.Handle(ctx, record)
}

func (h *holdOnIngested) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &holdOnIngested{inner: h.inner.WithAttrs(attrs), suffix: h.suffix}
}

func (h *holdOnIngested) WithGroup(name string) slog.Handler {
	return &holdOnIngested{inner: h.inner.WithGroup(name), suffix: h.suffix}
}

// stack is one running child, the ports it serves HTTP and metrics on and the platform ID it
// reported.
type stack struct {
	composition composition
	process     *prochost.Process
	httpPort    int
	metricsPort int
	platform    string
	// ingestStatusBefore is the revision of graph-ingest's GRAPH_STATUS entry before this child
	// started, 0 if absent: an entry above it is one this child wrote.
	ingestStatusBefore uint64
}

// freePort picks a port for the ServiceManager or the metrics server and releases it.
func freePort(t *testing.T) int {
	t.Helper()
	// Every address: the ServiceManager and the metrics server bind ":"+port (design D4).
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("pick a port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release port %d: %v", port, err)
	}
	return port
}

// startStack boots the six-component stack in a child against url, on two ports it picks, and
// returns once StartAll has returned in it.
func startStack(t *testing.T, o observer, url string, env ...string) stack {
	t.Helper()
	return startComposition(t, o, url, fullStack, env...)
}

// startComposition is startStack for the composition c.
func startComposition(t *testing.T, o observer, url string, c composition, env ...string) stack {
	t.Helper()
	return startStackOn(t, o, url, c, freePort(t), freePort(t), env...)
}

// restartStack boots a new child against url from the configuration previous booted from, its
// ports included. The configuration bucket the first boot wrote holds those ports, and a later
// boot serves on them whatever the file says ("Synced configuration from KV"), so the same
// configuration is the honest restart. previous must have exited. env is passed to the child as
// startStack passes it.
func restartStack(t *testing.T, o observer, url string, previous stack, env ...string) stack {
	t.Helper()
	if previous.process.Alive() {
		t.Fatal("restartStack: the previous stack is still running")
	}
	return startStackOn(t, o, url, previous.composition, previous.httpPort, previous.metricsPort, env...)
}

// startStackOn boots a child running c on the two ports; env adds to what the child is passed, as
// envIngestConfig and envHold.
func startStackOn(t *testing.T, o observer, url string, c composition, httpPort, metricsPort int, env ...string) stack {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	// Read before the child exists, so an entry above this revision is the child's own write.
	before := o.statusRevision(t, readiness.KeyGraphIngest)
	p, err := prochost.Start(t, stackHelper, append([]string{
		envComposition + "=" + c.name,
		envNATSURL + "=" + url,
		envHTTPPort + "=" + strconv.Itoa(httpPort),
		envMetricsPort + "=" + strconv.Itoa(metricsPort),
		envReady + "=" + ready,
	}, env...)...)
	if err != nil {
		t.Fatalf("start the stack: %v", err)
	}
	s := stack{composition: c, process: p, httpPort: httpPort, metricsPort: metricsPort, ingestStatusBefore: before}
	// The child renames the file into place, so a file that exists is whole.
	s.platform = await(t, s, "the stack's StartAll to return", func(context.Context) (string, error) {
		data, err := os.ReadFile(ready)
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return string(data), err
	}, func(platform string) bool { return platform != "" })
	return s
}

// stopStack ends the child with SIGTERM and requires a clean exit: every stop returned nil.
func stopStack(t *testing.T, s stack) {
	t.Helper()
	if !s.process.Alive() {
		t.Fatal("the stack exited before the test stopped it")
	}
	if err := s.process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM the stack: %v", err)
	}
	status, err := s.process.Wait(t.Context())
	if err != nil {
		t.Fatalf("wait for the stack: %v", err)
	}
	if status.Code != 0 {
		t.Fatalf("the stack exited %+v after SIGTERM, want 0", status)
	}
}

// await polls observe until done accepts what it saw, under the test's context: go test -timeout is
// the one deadline. It fails at once if the child exits first, since an exited child cannot bring
// the condition about.
func await[T any](t *testing.T, s stack, what string, observe func(context.Context) (T, error), done func(T) bool) T {
	t.Helper()
	type observation struct {
		value  T
		exited bool
	}
	seen, err := probe.Await(t.Context(), func(ctx context.Context) (observation, error) {
		if !s.process.Alive() {
			return observation{exited: true}, nil
		}
		value, err := observe(ctx)
		return observation{value: value}, err
	}, func(o observation) bool { return o.exited || done(o.value) })
	if err != nil {
		t.Fatalf("waiting for %s: %v", what, err)
	}
	if seen.exited {
		t.Fatalf("the stack exited before %s", what)
	}
	return seen.value
}

// openClient opens the producer's and observer's connection, with natsclient's defaults.
func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url)
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

// observer is the test's side of the stack: one connection, as producer and as reader.
type observer struct {
	client *natsclient.Client
	js     jetstream.JetStream
}

func newObserver(t *testing.T, url string) observer {
	t.Helper()
	client := natsfixture.Open(t, url, openClient)
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	return observer{client: client, js: js}
}

// publish publishes one input for e on entity.<id> and returns the sequence the ENTITY stream
// acknowledged it at.
func (o observer) publish(t *testing.T, e entityInput) uint64 {
	t.Helper()
	payload := &smokeEntity{ID: e.id}
	for _, s := range e.statements {
		payload.Statements = append(payload.Statements, message.Triple{
			Subject: e.id, Predicate: s.predicate, Object: s.object, Source: producerSource,
			Timestamp: inputTime, Confidence: 1,
		})
	}
	data, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, producerSource))
	if err != nil {
		t.Fatalf("marshal %s: %v", e.id, err)
	}
	ack, err := o.client.PublishToStreamWithAck(t.Context(), "entity."+e.id, data)
	if err != nil {
		t.Fatalf("publish %s: %v", e.id, err)
	}
	return ack.Sequence
}

// consumerState is what the broker reports of graph-ingest's durable consumer, in fields a failed
// wait prints.
type consumerState struct {
	AckFloor        uint64 // every input up to this stream sequence is acknowledged
	AckPending      int    // inputs delivered and not yet acknowledged
	DeliveredStream uint64 // the highest stream sequence delivered
	Deliveries      uint64 // deliveries made, redeliveries included (the consumer sequence)
}

// readConsumer reads graph-ingest's durable consumer from the broker.
func (o observer) readConsumer(ctx context.Context) (consumerState, error) {
	consumer, err := o.js.Consumer(ctx, "ENTITY", ingestConsumer)
	if err != nil {
		return consumerState{}, err
	}
	info, err := consumer.Info(ctx)
	if err != nil {
		return consumerState{}, err
	}
	return consumerState{AckFloor: info.AckFloor.Stream, AckPending: info.NumAckPending,
		DeliveredStream: info.Delivered.Stream, Deliveries: info.Delivered.Consumer}, nil
}

// awaitAcknowledged waits until graph-ingest's consumer has acknowledged every input up to seq:
// graph-ingest's own ordering event for the batch (design D5). It returns what it read last.
func (o observer) awaitAcknowledged(t *testing.T, s stack, seq uint64) consumerState {
	t.Helper()
	return await(t, s, fmt.Sprintf("graph-ingest to acknowledge up to sequence %d", seq), o.readConsumer,
		func(c consumerState) bool { return c.AckFloor >= seq && c.AckPending == 0 })
}

// entityRevision reads the ENTITY_STATES revision: the last sequence of its stream, every write
// and delete included.
func (o observer) entityRevision(t *testing.T) uint64 {
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
	return bucket.StreamInfo().State.LastSeq
}

// readStatus reads one producer's GRAPH_STATUS envelope.
func (o observer) readStatus(ctx context.Context, key string) (graph.IndexStatusResponse, error) {
	var status graph.IndexStatusResponse
	kv, err := o.js.KeyValue(ctx, graph.BucketGraphStatus)
	if err != nil {
		return status, err
	}
	entry, err := kv.Get(ctx, key)
	if err != nil {
		return status, err
	}
	err = json.Unmarshal(entry.Value(), &status)
	return status, err
}

// statusRevision returns the revision of key's GRAPH_STATUS entry, or 0 when the bucket or the
// key does not exist yet.
func (o observer) statusRevision(t *testing.T, key string) uint64 {
	t.Helper()
	kv, err := o.js.KeyValue(t.Context(), graph.BucketGraphStatus)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		return 0
	}
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketGraphStatus, err)
	}
	entry, err := kv.Get(t.Context(), key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return 0
	}
	if err != nil {
		t.Fatalf("get %s/%s: %v", graph.BucketGraphStatus, key, err)
	}
	return entry.Revision()
}

// latched reports a state an envelope holds until the process restarts, or "" for none.
func latched(status graph.IndexStatusResponse) string {
	if status.State == graph.IndexStateResetRequired || status.Code != "" {
		return fmt.Sprintf("state %s, code %q: %s", status.State, status.Code, status.Reason)
	}
	return ""
}

// awaitIndexed is the rest of design D5's order, after the acknowledgement or a mutation's reply:
// the ENTITY_STATES revision, read now; graph-ingest's envelope, once this child has written one,
// not degraded; then the envelope of each index the composition runs (graph-index, and
// graph-embedding and graph-clustering where they are configured) at or above that revision. It returns the revision. A latched
// state fails at once, naming it.
func (o observer) awaitIndexed(t *testing.T, s stack) uint64 {
	t.Helper()
	revision := o.entityRevision(t)
	// graph-ingest writes its first record from a goroutine Start launches, then on a period, so
	// the entry may be absent or, after a restart, the earlier process's: wait for one above the
	// revision read before this child started.
	ingest := await(t, s, fmt.Sprintf("%s/%s written by this process (revision above %d)",
		graph.BucketGraphStatus, readiness.KeyGraphIngest, s.ingestStatusBefore),
		func(ctx context.Context) (graph.IndexStatusResponse, error) {
			kv, err := o.js.KeyValue(ctx, graph.BucketGraphStatus)
			if err != nil {
				return graph.IndexStatusResponse{}, err
			}
			entry, err := kv.Get(ctx, readiness.KeyGraphIngest)
			if err != nil {
				return graph.IndexStatusResponse{}, err
			}
			if entry.Revision() <= s.ingestStatusBefore {
				return graph.IndexStatusResponse{}, fmt.Errorf("revision %d is not above %d", entry.Revision(), s.ingestStatusBefore)
			}
			var status graph.IndexStatusResponse
			return status, json.Unmarshal(entry.Value(), &status)
		}, func(graph.IndexStatusResponse) bool { return true })
	if ingest.State == graph.IndexStateDegraded || latched(ingest) != "" {
		t.Fatalf("%s/%s reads %+v, want not degraded", graph.BucketGraphStatus, readiness.KeyGraphIngest, ingest)
	}
	for _, key := range s.composition.indexKeys {
		status := await(t, s, fmt.Sprintf("%s/%s at revision %d", graph.BucketGraphStatus, key, revision),
			func(ctx context.Context) (graph.IndexStatusResponse, error) { return o.readStatus(ctx, key) },
			func(status graph.IndexStatusResponse) bool {
				return latched(status) != "" || status.IndexedRevision >= revision
			})
		if reason := latched(status); reason != "" {
			t.Fatalf("%s/%s latched: %s", graph.BucketGraphStatus, key, reason)
		}
	}
	return revision
}

// componentsHealth is the reply of GET /components/health.
type componentsHealth struct {
	Overall    health.Status   `json:"overall"`
	Components []health.Status `json:"components"`
	Total      int             `json:"total"`
}

// get reads path from the stack's server on port.
func get(t *testing.T, s stack, port int, path string) (string, int, []byte) {
	t.Helper()
	return send(t, s, port, http.MethodGet, path, nil)
}

// send sends one request to the stack's server on port, dialled at the IPv6 loopback: on a Mac,
// 127.0.0.1:<port> can belong to another program that bound IPv4 only (design D4, #390). It
// returns the URL, the status and the body.
func send(t *testing.T, s stack, port int, method, path string, body []byte) (string, int, []byte) {
	t.Helper()
	if !s.process.Alive() {
		t.Fatalf("the stack exited before %s %s was sent", method, path)
	}
	url := "http://" + net.JoinHostPort("::1", strconv.Itoa(port)) + path
	request, err := http.NewRequestWithContext(t.Context(), method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = response.Body.Close() }()
	reply, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return url, response.StatusCode, reply
}

// readHealth reads the component health the stack serves.
func readHealth(t *testing.T, s stack) componentsHealth {
	t.Helper()
	url, code, body := get(t, s, s.httpPort, "/components/health")
	var reply componentsHealth
	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatalf("decode %s (HTTP %d): %v: %s", url, code, err, body)
	}
	return reply
}

// requireComponentsListed fails unless the stack's health reply names exactly the components its
// composition declares. An empty list reads healthy (health/helpers.go:44-45), so the names and
// the total are checked.
func requireComponentsListed(t *testing.T, s stack) componentsHealth {
	t.Helper()
	reply := readHealth(t, s)
	declared := s.composition.components
	var names []string
	for _, c := range reply.Components {
		names = append(names, c.Component)
	}
	slices.Sort(names)
	if reply.Total != len(declared) || strings.Join(names, ",") != strings.Join(declared, ",") {
		t.Fatalf("/components/health lists %v (total %d), want %v (total %d)",
			names, reply.Total, declared, len(declared))
	}
	return reply
}

// requireComponentsHealthy fails unless the composition's components are listed and each is healthy.
func requireComponentsHealthy(t *testing.T, s stack) {
	t.Helper()
	reply := requireComponentsListed(t, s)
	for _, c := range reply.Components {
		if !c.Healthy {
			t.Errorf("component %s reads %s: %s", c.Component, c.Status, c.Message)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

// reindexMetric is graph-index's re-index counter (#108's instrument), by result.
const reindexMetric = "semengine_graph_index_reindex_events_total"

// readReindexCounter returns the reindexMetric series the stack's metrics server reports, one line
// each. The counter is recorded, never asserted (design D4, #108).
func readReindexCounter(t *testing.T, s stack) []string {
	t.Helper()
	url, code, body := get(t, s, s.metricsPort, "/metrics")
	if code != http.StatusOK {
		t.Fatalf("GET %s: HTTP %d: %s", url, code, body)
	}
	var series []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, reindexMetric+"{") {
			series = append(series, line)
		}
	}
	return series
}
