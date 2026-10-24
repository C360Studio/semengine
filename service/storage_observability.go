// storage_observability.go hosts the account storage collector as a SERVICE
// (storage-capacity-observability section 4).
//
// It is a service rather than a component because it watches the SUBSTRATE, not
// the flow. A component is a flow participant with ports that processes
// messages; this has no ports and never receives or emits one, and filing it as
// a component would place substrate monitoring in the data-flow graph as though
// it were a pipeline stage.
//
// # Two halves, one bucket
//
// The service runs a PRODUCER and a CONSUMER, and the seam between them is the
// published report bucket rather than a Go value:
//
//	collector (interval) -> publisher (KV: one key per resource + the account
//	row) -> consumer (Watch) -> Prometheus metrics + health status + HTTP route
//
// The detour through KV is the design, not overhead. Growth, projection and
// pressure are derived once, at publication, from inputs only a collection
// holds; every operator surface then reads the same published rows, so no two
// can disagree. Watching this process's own writes also means each surface
// reports the ACCOUNT rather than the fraction of it that one process last
// enumerated: any number of processes may publish account-wide.
//
// # Report-only, and boot-safe
//
// Nothing here rejects a write, throttles a component, fails a readiness or
// health gate, or applies retention. Health reports pressure in its MESSAGE and
// never in its verdict.
//
// A deployment that does not declare this service in its `services` block is
// untouched: the service manager only constructs what the config names. A
// deployment that DOES declare it must still boot when the account is
// unreadable, so the report bucket is acquired lazily and retried rather than
// resolved in the constructor or in Start: a monitoring surface that can take
// down the system it monitors is a worse bug than the blindness it fixes.
//
// # Configuration is applied by RESTART
//
// The collection interval, its timeout, and the pressure thresholds are built
// from rawConfig at construction. A restart is the supported way to change them:
// a stale threshold evaluates old numbers visibly and destroys nothing, unlike a
// stale value reconciling a durable resource to the wrong policy. Restarting is
// cheap precisely because the report and its growth series live in KV, where
// they survive it.

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/natsclient/storagereport"
	"github.com/c360studio/semengine/pkg/errs"
)

// StorageObservabilityConfig is the operator surface for account storage
// observability.
//
// Durations are strings ("1m", "30s") following the repository's service
// configuration convention. Every field is optional and takes its documented
// default; an unusable value is an ERROR at construction rather than a silent
// fallback, because a silently defaulted knob applies a number the operator did
// not choose and is indistinguishable from a working edit.
type StorageObservabilityConfig struct {
	// Interval is how often the account is enumerated. Every process polling
	// account-wide multiplies cost by deployment size, so an operator running
	// many instances turns this down rather than losing the view entirely.
	// Default: 1m.
	Interval string `json:"interval,omitempty"`

	// Timeout bounds ONE collection, both account listings included. It must
	// fit inside Interval. Default: 15s.
	Timeout string `json:"timeout,omitempty"`

	// PressureThresholds are the headroom and time-to-threshold bands pressure
	// is derived from. Report-only: nothing is rejected, throttled, or degraded
	// when a band is crossed.
	PressureThresholds storagereport.StoragePressureThresholds `json:"pressure_thresholds,omitempty"`
}

// appliedStorageObservabilityConfig is the validated form: defaults filled,
// durations parsed, coherence checked. Only applied() produces one, so nothing
// can run against configuration nobody checked.
type appliedStorageObservabilityConfig struct {
	interval   time.Duration
	timeout    time.Duration
	thresholds storagereport.StoragePressureThresholds
}

// applied validates the operator's configuration and fills the documented
// defaults. Defaults have this one home.
func (c StorageObservabilityConfig) applied() (appliedStorageObservabilityConfig, error) {
	interval, err := serviceDuration(StorageObservabilityServiceName, "interval", c.Interval,
		storagereport.DefaultStorageInventoryInterval)
	if err != nil {
		return appliedStorageObservabilityConfig{}, err
	}
	timeout, err := serviceDuration(StorageObservabilityServiceName, "timeout", c.Timeout,
		storagereport.DefaultStorageInventoryTimeout)
	if err != nil {
		return appliedStorageObservabilityConfig{}, err
	}
	// A timeout wider than the interval quietly serializes collections behind
	// each other: the collector holds a lock for the whole of one collection,
	// so every tick would queue rather than sample.
	if timeout > interval {
		return appliedStorageObservabilityConfig{}, fmt.Errorf(
			"invalid storage-observability config: timeout (%s) must not exceed interval (%s)",
			timeout, interval)
	}
	// Resolved here so a malformed band is a loud construction error rather
	// than a warning logged once a minute for the life of the process.
	if _, err := c.PressureThresholds.Resolve(); err != nil {
		return appliedStorageObservabilityConfig{}, fmt.Errorf(
			"invalid storage-observability pressure thresholds: %w", err)
	}
	return appliedStorageObservabilityConfig{
		interval:   interval,
		timeout:    timeout,
		thresholds: c.PressureThresholds,
	}, nil
}

// StorageObservabilityService collects the account storage inventory, publishes
// the report, and serves it as metrics, health status and an HTTP route.
type StorageObservabilityService struct {
	*BaseService

	interval  time.Duration
	collector *storagereport.StorageInventoryCollector
	consumer  *storagereport.Consumer
	logger    *slog.Logger

	// lifecycle admits one Start and makes Stop the only stop (ADR-113).
	lifecycle lifecycleguard.Guard

	// runCancel and loopsDone are written once, by start, before Start returns, and read by
	// cleanup. The guard runs cleanup only after start has returned, so they need no lock.
	// runCancel ends the run context both loops run under.
	runCancel context.CancelFunc
	// loopsDone is closed once the collector's and the consumer's loops have both returned; it is
	// nil when no loop was started.
	loopsDone chan struct{}
}

// NewStorageObservabilityService builds the service from its JSON configuration
// (StorageObservabilityConfig). A key the configuration does not define, or one in another
// letter case, nested ones included, is refused, naming the key. It refuses, as invalid data and
// naming the input, nil dependencies, a nil NATS client, a nil metrics registry, an interval or
// timeout that does not parse or is not positive, a timeout above the interval, and pressure
// thresholds that do not resolve. It does no I/O: the report bucket is acquired by the loops
// Start runs.
func NewStorageObservabilityService(rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	var cfg StorageObservabilityConfig
	if err := decodeStrictServiceJSON(rawConfig, &cfg); err != nil {
		return nil, refuseStorageObservability(err, "parse storage-observability config")
	}
	applied, err := cfg.applied()
	if err != nil {
		return nil, refuseStorageObservability(err, "validate storage-observability config")
	}

	if deps == nil {
		return nil, refuseStorageObservability(
			errors.New("storage-observability requires dependencies"), "check dependencies")
	}
	if deps.NATSClient == nil {
		return nil, refuseStorageObservability(
			errors.New("storage-observability requires a NATS client to enumerate the account"),
			"check dependencies")
	}
	// A service whose series reach no scrape is the phantom signal this capability exists to
	// remove, so a missing registry is refused rather than tolerated.
	if deps.MetricsRegistry == nil {
		return nil, refuseStorageObservability(
			errors.New("storage-observability requires a metrics registry"), "check dependencies")
	}

	logger := slog.Default().With("service", StorageObservabilityServiceName)
	if deps.Logger != nil {
		logger = deps.Logger.With("service", StorageObservabilityServiceName)
	}

	metrics := newStorageObservabilityMetrics()
	if err := metrics.register(deps.MetricsRegistry); err != nil {
		return nil, fmt.Errorf("register storage-observability metrics: %w", err)
	}

	// The bucket is resolved on FIRST USE, not here. A deployment that declares
	// this service must still boot when the account cannot be read.
	bucket := &lazyReportBucket{client: deps.NATSClient}

	// The metrics observer, the health message and the HTTP route all read this
	// one consumer, so the operator surfaces cannot disagree: none recomputes,
	// and all project the rows the collector published.
	consumer, err := storagereport.NewConsumer(bucket, storagereport.ConsumerConfig{
		Observer: metrics,
		Logger:   logger,
	})
	if err != nil {
		return nil, fmt.Errorf("build the storage report consumer: %w", err)
	}

	thresholds := applied.thresholds
	collector, err := storagereport.NewStorageInventoryCollector(
		storagereport.AccountStreamLister(deps.NATSClient),
		storagereport.StorageInventoryConfig{
			Interval:      applied.interval,
			Timeout:       applied.timeout,
			OwnerResolver: kvcatalog.OwnerOf,
			Publisher: &lazyReportPublisher{
				bucket: bucket,
				// Configuration is applied by restart, so the source returns the
				// value built at construction.
				thresholds: func() storagereport.StoragePressureThresholds { return thresholds },
				logger:     logger,
			},
			Logger: logger,
		})
	if err != nil {
		return nil, fmt.Errorf("build the storage inventory collector: %w", err)
	}

	return &StorageObservabilityService{
		BaseService: NewBaseServiceWithOptions(StorageObservabilityServiceName,
			WithLogger(deps.Logger), WithMetrics(deps.MetricsRegistry)),
		interval:  applied.interval,
		collector: collector,
		consumer:  consumer,
		logger:    logger,
	}, nil
}

func refuseStorageObservability(err error, action string) error {
	return errs.WrapInvalid(err, "StorageObservabilityService", "NewStorageObservabilityService", action)
}

// Start starts the base service, then the inventory collector's and the report consumer's loops,
// and returns nil. It does no I/O and never fails on account or bucket state: an unreadable
// account or an unreachable bucket is retried by the loops, each on its own schedule. It refuses
// a nil or ended context, a second Start and a Start after any Stop (errs.ErrAlreadyStarted); a
// refused Start holds nothing. ctx bounds Start only; ending it after Start returned stops nothing.
func (s *StorageObservabilityService) Start(ctx context.Context) error {
	return s.lifecycle.Start(ctx, s.start, s.cleanup)
}

// start is Start's work, run by the lifecycle guard; a failure is rolled back through cleanup.
func (s *StorageObservabilityService) start(ctx context.Context) error {
	if err := s.BaseService.Start(ctx); err != nil {
		return err
	}

	// The run context: Start's values without its cancellation, ended only by Stop (ADR-113).
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.runCancel = cancel

	var loops sync.WaitGroup
	loops.Go(func() { s.collector.Run(runCtx) })
	loops.Go(func() { s.consumer.Run(runCtx) })
	done := make(chan struct{})
	go func() {
		loops.Wait()
		close(done)
	}()
	s.loopsDone = done

	s.logger.Info("storage observability started", "interval", s.interval, "report_only", true)
	return nil
}

// Stop ends the run context, waits for both loops to return and stops the base service. It
// returns nil only once all three are done. If ctx ends first it returns that context's error and
// the next Stop waits again. A Stop after one that returned nil returns nil; a Stop while another
// runs is refused as transient. How soon a collection in progress sees the run context end
// depends on whether the JetStream listing honours its context, which nats.go does not promise;
// Stop stays bounded by its own context either way.
func (s *StorageObservabilityService) Stop(ctx context.Context) error {
	return s.lifecycle.Stop(ctx)
}

// cleanup is the one cleanup the lifecycle guard runs for a failed Start and for every Stop.
// Every step can run again, so a Stop after one whose context ended finishes the job.
func (s *StorageObservabilityService) cleanup(ctx context.Context) error {
	if s.runCancel != nil {
		s.runCancel()
	}
	if s.loopsDone != nil {
		select {
		case <-s.loopsDone:
		case <-ctx.Done():
			return fmt.Errorf("wait for storage observability loops: %w", ctx.Err())
		}
	}
	if err := s.BaseService.Stop(ctx); err != nil {
		return fmt.Errorf("stop storage-observability base service: %w", err)
	}
	s.logger.Info("storage observability stopped")
	return nil
}

// Health reports the storage picture WITHOUT letting it become a verdict.
//
// The lifecycle status comes from BaseService untouched; pressure, capacity and
// over-commitment appear in the message. That separation is the report-only
// guarantee expressed where it would break first: readiness reads Status(), and
// /health aggregates Health(), so a pressure-derived verdict at either point
// would turn observability into admission control. Both reads do no I/O.
func (s *StorageObservabilityService) Health() health.Status {
	return describeStorageHealth(s.BaseService.Health(), s.consumer.Snapshot(), s.collector.Latest())
}

// --- lazy bucket acquisition -------------------------------------------------

// lazyReportBucket acquires the report bucket on first use and caches it.
//
// Acquisition is DEFERRED out of the constructor and out of Start so that a
// deployment declaring this service still boots when JetStream is unreachable,
// the account is unreadable, or the bucket cannot yet be created. Both users
// retry on their own schedule (the collector on its interval, the consumer on
// its watch backoff), so a late-arriving bucket heals without a restart.
type lazyReportBucket struct {
	client *natsclient.Client

	mu     sync.Mutex
	bucket jetstream.KeyValue
}

// resolve acquires the bucket through the catalog OWNER seam. This service is
// the STORAGE_REPORT descriptor's declared owner, so it ensures rather than
// opens: create-or-open, reconcile to the declared policy, verify, or fail.
func (b *lazyReportBucket) resolve(ctx context.Context) (jetstream.KeyValue, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.bucket != nil {
		return b.bucket, nil
	}
	bucket, err := kvcatalog.EnsureCatalogBucket(ctx, b.client, graph.BucketStorageReport)
	if err != nil {
		return nil, err
	}
	b.bucket = bucket
	return bucket, nil
}

// WatchAll makes the lazy bucket a storagereport.ReportWatchStore. A resolution
// failure is returned rather than logged here: the consumer's own retry loop
// owns the backoff and the one warning line.
func (b *lazyReportBucket) WatchAll(
	ctx context.Context, opts ...jetstream.WatchOpt,
) (jetstream.KeyWatcher, error) {
	bucket, err := b.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return bucket.WatchAll(ctx, opts...)
}

// lazyReportPublisher is the collector's InventoryPublisher. It builds the real
// publisher on the first collection that finds the bucket reachable, and keeps
// it: the publisher holds the in-process growth baselines, so rebuilding it per
// collection would re-read the bucket history every time.
type lazyReportPublisher struct {
	bucket     *lazyReportBucket
	thresholds storagereport.ThresholdSource
	logger     *slog.Logger

	mu        sync.Mutex
	publisher *storagereport.Publisher
}

func (p *lazyReportPublisher) Publish(
	ctx context.Context, inv storagereport.StorageInventory,
) (storagereport.PublishResult, error) {
	publisher, err := p.resolve(ctx)
	if err != nil {
		return storagereport.PublishResult{}, err
	}
	return publisher.Publish(ctx, inv)
}

func (p *lazyReportPublisher) resolve(ctx context.Context) (*storagereport.Publisher, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.publisher != nil {
		return p.publisher, nil
	}
	bucket, err := p.bucket.resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire the storage report bucket: %w", err)
	}
	publisher, err := storagereport.NewPublisher(bucket, storagereport.PublisherConfig{
		Thresholds: p.thresholds,
		Logger:     p.logger,
	})
	if err != nil {
		return nil, err
	}
	p.publisher = publisher
	return publisher, nil
}

// --- health rendering --------------------------------------------------------

// describeStorageHealth appends the storage picture to the base verdict WITHOUT replacing it.
//
// It holds no verdict of its own by construction: it only ever rewrites base's Message. Pressure
// never reaches Healthy, Status, or a sub-status, not even a nested one: health.Aggregate does
// not recurse today, so a degraded sub-status would be invisible now and become a readiness
// failure the day it does; leaving the verdict axis untouched is the version of report-only that
// cannot rot.
//
// The base Message is the only field carrying the REASON for a non-healthy lifecycle verdict
// ("Service is unhealthy (failed checks: N)": service/base.go), so overwriting it would delete
// the explanation from the exact surface an operator reads while leaving the status axis saying
// something is wrong.
func describeStorageHealth(
	base health.Status, snapshot storagereport.Snapshot, inventory storagereport.StorageInventory,
) health.Status {
	picture := storageHealthMessage(snapshot, inventory)
	if base.Message == "" {
		base.Message = picture
		return base
	}
	base.Message = base.Message + "; " + picture
	return base
}

// storageHealthMessage renders the storage picture. It is deliberately one deterministic line:
// health is read by operators and by log scrapers, and an unstable rendering is a diff nobody can
// use.
func storageHealthMessage(snapshot storagereport.Snapshot, inventory storagereport.StorageInventory) string {
	var parts []string

	if !snapshot.Synced {
		parts = append(parts, "storage report has not been read yet")
	} else {
		counts := snapshot.PressureCounts
		parts = append(parts, fmt.Sprintf(
			"storage pressure (report-only): critical=%d high=%d warning=%d normal=%d not-evaluated=%d",
			counts[storagereport.PressureCritical], counts[storagereport.PressureHigh],
			counts[storagereport.PressureWarning], counts[storagereport.PressureNormal],
			snapshot.NotEvaluated))

		if worst := worstResources(snapshot); worst != "" {
			parts = append(parts, "worst: "+worst)
		}
	}

	if snapshot.AccountKnown {
		if tiers := accountFindings(snapshot.Account); tiers != "" {
			parts = append(parts, tiers)
		}
	}

	if collected := collectionState(inventory); collected != "" {
		parts = append(parts, collected)
	}
	return strings.Join(parts, "; ")
}

// maxNamedResources bounds the health message. An account can carry hundreds of
// streams, and a message that grew with the account would be unreadable in the
// place operators read it first.
const maxNamedResources = 5

// worstResources names the resources an operator should look at: everything
// above normal, worst first, then the resources carrying no state at all,
// which is what keeps an unbounded resource from being invisible in a summary
// organized by state.
func worstResources(snapshot storagereport.Snapshot) string {
	type named struct {
		name     string
		severity int
		// inherited marks a state that came from the account tier rather than
		// from this resource's own bound. It is a SORT KEY, not decoration; see
		// the ordering below.
		inherited bool
		label     string
	}
	var candidates []named

	for _, row := range snapshot.Resources {
		switch {
		case row.Pressure.Evaluated && row.Pressure.State != storagereport.PressureNormal:
			// A state inherited from the account tier says so. Left unqualified it
			// would read as this resource's own bound filling, sending an operator
			// to raise a bound the resource does not have, and the two findings
			// have different fixes.
			inherited := row.Pressure.EvaluatedAgainst == storagereport.PressureBasisAccountTier
			basis := ""
			if inherited {
				basis = fmt.Sprintf(" vs %s account tier", row.Resource.Tier)
			}
			candidates = append(candidates, named{
				name:      row.Resource.Name,
				severity:  pressureSeverityValue(row.Pressure.State),
				inherited: inherited,
				label: fmt.Sprintf("%s %s (%s)%s",
					row.Resource.Name, row.Pressure.State, row.Pressure.RaisedBy, basis),
			})
		case !row.Pressure.Evaluated:
			candidates = append(candidates, named{
				name:     row.Resource.Name,
				severity: -1,
				label:    fmt.Sprintf("%s not-evaluated (%s)", row.Resource.Name, row.Pressure.Unavailable),
			})
		}
	}
	if len(candidates) == 0 {
		return ""
	}

	// Worst first, then OWN-BOUND before inherited at equal severity, then by name.
	//
	// The middle key is load-bearing. Every framework KV bucket is unbounded on
	// bytes, so a single tier crossing a band makes twenty-odd resources inherit
	// the same state in the same collection. Sorted on severity alone they tie
	// with the genuinely actionable finding and win on alphabetical order,
	// filling all five slots with copies of one tier fact and pushing the one
	// resource whose bound an operator can actually raise into "and N more".
	//
	// Ordering own-bound first does not hide the inherited rows: they still
	// appear, still carry their basis, and the tier itself is named by
	// accountFindings.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].severity != candidates[j].severity {
			return candidates[i].severity > candidates[j].severity
		}
		if candidates[i].inherited != candidates[j].inherited {
			return !candidates[i].inherited
		}
		return candidates[i].name < candidates[j].name
	})

	labels := make([]string, 0, maxNamedResources+1)
	for _, candidate := range candidates {
		if len(labels) == maxNamedResources {
			labels = append(labels, fmt.Sprintf("and %d more", len(candidates)-maxNamedResources))
			break
		}
		labels = append(labels, candidate.label)
	}
	return strings.Join(labels, ", ")
}

// accountFindings names the over-committed tiers and the tiers whose comparison
// could not be made. Both are findings; neither is a gate.
func accountFindings(report storagereport.AccountReport) string {
	var findings []string
	for _, comparison := range report.Tiers {
		switch comparison.State {
		case storagereport.OvercommitmentOver:
			findings = append(findings, fmt.Sprintf("%s over-committed (declared %d bytes)",
				comparison.Tier, comparison.DeclaredBytes))
		case storagereport.OvercommitmentNotApplicable:
			if comparison.Unavailable == storagereport.OvercommitmentUnavailableUnknownLimit {
				findings = append(findings, fmt.Sprintf("%s account limit unknown", comparison.Tier))
			}
		}
	}
	if len(findings) == 0 {
		return ""
	}
	return "account: " + strings.Join(findings, ", ")
}

// collectionState reports FRESHNESS on the answer rather than gating on it. A
// collection that failed leaves last-good in place, and how stale that is
// belongs on the report, not in a decision to withhold it.
func collectionState(inventory storagereport.StorageInventory) string {
	if inventory.Stale {
		return fmt.Sprintf("last collection did not succeed (%s); serving last known result",
			inventory.StaleReason)
	}
	if inventory.CollectedAt.IsZero() {
		return ""
	}
	return fmt.Sprintf("collected %s ago by %s",
		time.Since(inventory.CollectedAt).Round(time.Second), inventory.ProducedBy)
}
