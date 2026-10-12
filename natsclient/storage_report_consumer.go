// storage_report_consumer.go READS the published storage report
// (storage-observability). It is the other half of the KV twofer the report
// bucket was chosen for: storage_report.go writes one key per resource plus the
// reserved per-tier account row, and this file turns a Watch over those keys
// into the in-process view every operator surface serves from.
//
// The direction is the point. Prometheus metrics, health status, the operator
// HTTP route, and any alerting are CONSUMERS of the published bucket, never
// second report-producing paths. Growth, projection, and pressure are derived
// once, at publication, from inputs only the collection holds — the retained
// observation series and the account limits. A surface that recomputed any of
// them from what it happened to have in memory would eventually disagree with
// the bucket, and an operator cannot act on two answers.
//
// Watching this process's OWN writes is deliberate rather than wasteful. Any
// number of processes may publish account-wide, so a surface fed from the local
// collection would report the fraction of the account that one process last
// looked at; a surface fed from the bucket reports the account.
//
// Nothing here writes, rejects, throttles, or degrades anything.

package natsclient

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/pkg/errs"
)

// DefaultReportConsumerRetryBackoff is how long the consumer waits before
// re-establishing a watch that could not be created or that ended.
const DefaultReportConsumerRetryBackoff = 5 * time.Second

// ReportWatchStore is the narrow KV capability the report consumer needs.
// jetstream.KeyValue satisfies it.
//
// WatchAll rather than a range read: on restart a consumer needs the current
// report immediately (KV re-delivers every current value, which is correct
// recovery), and several surfaces should each react rather than one dequeuing.
// Deletes are NOT filtered out — a reclaimed row must retract its metric series
// rather than leave a resource reporting forever after it is gone.
type ReportWatchStore interface {
	WatchAll(ctx context.Context, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error)
}

// StorageReportObserver receives each change the consumer applies.
//
// It exists so a metrics surface can update ONE series per event instead of
// rebuilding every series on every tick. ForgetResource carries the whole last
// known row rather than a key, because a resource whose name is not a legal KV
// key is addressed by an opaque token — a bare key cannot say which label set a
// gauge was registered under.
//
// Implementations are called from the consumer's watch goroutine and must not
// block on it.
type StorageReportObserver interface {
	ObserveResource(row ResourceReport)
	ForgetResource(row ResourceReport)
	ObserveAccount(row AccountReport)
}

// StorageReportSnapshot is the whole published report as one in-process view.
//
// The counts are a TALLY of what the rows say, never a re-evaluation: pressure
// is derived at publication, and a consumer that recomputed it would be the
// second producer this design exists to prevent.
type StorageReportSnapshot struct {
	// Resources is every published resource row, sorted by resource name.
	Resources []ResourceReport

	// Account is the per-tier comparison, valid only when AccountKnown.
	Account      AccountReport
	AccountKnown bool

	// Synced reports that a watch has delivered every current value at least
	// once. Before it, the snapshot is empty and means "not read yet" rather
	// than "the account holds nothing" — the same distinction the inventory's
	// Stale flag keeps. It stays true while a replacement watch is syncing,
	// because the view is then the previous watch's complete one.
	Synced bool

	// UpdatedAt is when the last change was applied.
	UpdatedAt time.Time

	// PressureCounts tallies the EVALUATED rows by state.
	PressureCounts map[PressureState]int

	// NotEvaluated counts rows carrying no pressure state at all — unbounded or
	// unknown capacity. Counted rather than folded into normal: a surface that
	// filtered on "state != normal" would make exactly the unbounded resources
	// invisible.
	NotEvaluated int

	// WorstPressure is the worst evaluated state, or empty when nothing was
	// evaluated. Empty is NOT normal: an account whose every row declined to
	// evaluate has no pressure verdict, and reporting one would manufacture it.
	WorstPressure PressureState
}

// StorageReportConsumerConfig configures the report consumer.
type StorageReportConsumerConfig struct {
	// Observer receives each applied change. Optional: a consumer without one
	// still maintains its snapshot.
	Observer StorageReportObserver

	// RetryBackoff is the wait before re-establishing a watch. Defaults to
	// DefaultReportConsumerRetryBackoff.
	RetryBackoff time.Duration

	// Logger receives watch failures. Defaults to slog.Default().
	Logger *slog.Logger
}

// StorageReportConsumer maintains the in-process view of the published report.
type StorageReportConsumer struct {
	store    ReportWatchStore
	observer StorageReportObserver
	backoff  time.Duration
	logger   *slog.Logger

	mu       sync.RWMutex
	rows     map[string]ResourceReport
	account  AccountReport
	hasAcct  bool
	synced   bool
	updated  time.Time
	snapshot StorageReportSnapshot
}

// NewStorageReportConsumer builds a consumer over the report bucket.
func NewStorageReportConsumer(
	store ReportWatchStore, cfg StorageReportConsumerConfig,
) (*StorageReportConsumer, error) {
	if store == nil {
		return nil, errs.WrapInvalid(
			// Seam guidance is deliberately conditional: the storage-observability
			// service is the report bucket's DECLARED OWNER, so it binds through
			// the owner seam (EnsureCatalogBucket). Any other consumer is a
			// reader and must not create — pointing everyone at the reader seam
			// would have misdirected the one caller that legitimately provisions.
			errors.New("a ReportWatchStore is required; the bucket's declared owner binds it through "+
				"kvcatalog.EnsureCatalogBucket, any other consumer through kvcatalog.OpenCatalogReader "+
				"(with graph.BucketStorageReport)"),
			"StorageReportConsumer", "New", "validate configuration")
	}
	consumer := &StorageReportConsumer{
		store:    store,
		observer: cfg.Observer,
		backoff:  cfg.RetryBackoff,
		logger:   cfg.Logger,
		rows:     make(map[string]ResourceReport, 32),
	}
	if consumer.backoff <= 0 {
		consumer.backoff = DefaultReportConsumerRetryBackoff
	}
	if consumer.logger == nil {
		consumer.logger = slog.Default()
	}
	consumer.rebuildLocked()
	return consumer, nil
}

// Snapshot returns the current view without doing any I/O. Safe to call from a
// health check, an HTTP handler, or a metrics scrape. The result shares no
// memory with the consumer, so a caller may edit it.
//
// While a replacement watch delivers its initial values, Snapshot returns the
// previous view unchanged; at that watch's sync marker the view becomes exactly
// the bucket's current values in one step, so a row or account row the bucket no
// longer holds is gone even when no delete marker for it was replayed.
func (c *StorageReportConsumer) Snapshot() StorageReportSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshot.clone()
}

// Run watches the report bucket until ctx ends, re-establishing the watch when
// it cannot be created or when it ends.
//
// The retry is not optional resilience. A watch that dies leaves every operator
// surface frozen on its last values, and a frozen report is indistinguishable
// from a calm account — which is the exact failure this capability exists to
// end. Call Run in its own goroutine.
func (c *StorageReportConsumer) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		c.watchOnce(ctx)

		select {
		case <-ctx.Done():
			return
		case <-time.After(c.backoff):
		}
	}
}

// watchOnce establishes one watch and drains it until it ends.
func (c *StorageReportConsumer) watchOnce(ctx context.Context) {
	watcher, err := c.store.WatchAll(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.logger.Warn("could not watch the storage report; operator surfaces are serving their last values",
				slog.String("error", err.Error()))
		}
		return
	}
	defer func() { _ = watcher.Stop() }()

	// Until this watch's sync marker its entries fill initial, not the view. A
	// Snapshot caller meanwhile sees the previous view whole (or, on the first
	// watch, an empty view with Synced false), never a mix of the old view and
	// part of the new one. At the marker the view becomes exactly this watch's
	// initial values in one step, which is what retracts a row or account the
	// bucket no longer holds: a bucket that was recreated, or whose delete
	// markers were purged, replays no tombstone for it.
	initial := newReportStage()
	updates := watcher.Updates()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, open := <-updates:
			if !open {
				return
			}
			if entry == nil {
				// nats.go sends exactly one nil when every current value has
				// been delivered. It is a marker, not a row.
				if initial != nil {
					c.replaceWith(initial)
					initial = nil
				}
				continue
			}
			if initial != nil {
				initial.apply(c, entry)
				continue
			}
			c.apply(entry)
		}
	}
}

// decodeReportEntry decodes a put entry. The reserved account key carries a
// different row kind; discriminating on the key is sound because a resource key
// can never contain a dot (see StorageAccountReportKey).
func decodeReportEntry(entry jetstream.KeyValueEntry) (row ResourceReport, account AccountReport, err error) {
	if entry.Key() == StorageAccountReportKey {
		err = json.Unmarshal(entry.Value(), &account)
		return row, account, err
	}
	err = json.Unmarshal(entry.Value(), &row)
	return row, account, err
}

// apply folds one live watch entry into the view and fans it out.
func (c *StorageReportConsumer) apply(entry jetstream.KeyValueEntry) {
	key := entry.Key()

	if entry.Operation() != jetstream.KeyValuePut {
		c.remove(key)
		return
	}

	row, account, err := decodeReportEntry(entry)
	if err != nil {
		// One undecodable value must not blank the account's whole view; the
		// next publication of that key repairs it.
		c.logSkip(key, entry.Revision(), err)
		return
	}
	if key == StorageAccountReportKey {
		c.putAccount(account)
		return
	}
	c.putResource(key, row)
}

// reportStage collects one watch's initial values until its sync marker.
type reportStage struct {
	rows map[string]ResourceReport
	// undecodable holds keys present in the bucket whose value could not be
	// decoded. As on the live path, the row held for such a key is kept until
	// the next publication repairs it, rather than retracted as absent.
	undecodable     map[string]struct{}
	account         AccountReport
	hasAcct         bool
	acctUndecodable bool
}

func newReportStage() *reportStage {
	return &reportStage{
		rows:        make(map[string]ResourceReport, 32),
		undecodable: make(map[string]struct{}),
	}
}

func (s *reportStage) apply(c *StorageReportConsumer, entry jetstream.KeyValueEntry) {
	key := entry.Key()
	isAccount := key == StorageAccountReportKey

	if entry.Operation() != jetstream.KeyValuePut {
		if isAccount {
			s.account, s.hasAcct, s.acctUndecodable = AccountReport{}, false, false
			return
		}
		delete(s.rows, key)
		delete(s.undecodable, key)
		return
	}

	row, account, err := decodeReportEntry(entry)
	switch {
	case err != nil && isAccount:
		c.logSkip(key, entry.Revision(), err)
		s.account, s.hasAcct, s.acctUndecodable = AccountReport{}, false, true
	case err != nil:
		c.logSkip(key, entry.Revision(), err)
		delete(s.rows, key)
		s.undecodable[key] = struct{}{}
	case isAccount:
		s.account, s.hasAcct, s.acctUndecodable = account, true, false
	default:
		s.rows[key] = row
		delete(s.undecodable, key)
	}
}

// replaceWith makes a completed watch's initial values the view, retracting
// every held row the bucket no longer holds, and fans the changes out.
func (c *StorageReportConsumer) replaceWith(s *reportStage) {
	keys := make([]string, 0, len(s.rows))
	for key := range s.rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	observed := make([]ResourceReport, 0, len(keys))
	for _, key := range keys {
		observed = append(observed, s.rows[key])
	}

	c.mu.Lock()
	var forgotten []ResourceReport
	for key, held := range c.rows {
		if _, present := s.rows[key]; present {
			continue
		}
		if _, present := s.undecodable[key]; present {
			s.rows[key] = held
			continue
		}
		forgotten = append(forgotten, held)
	}
	sort.Slice(forgotten, func(i, j int) bool { return forgotten[i].Resource.Name < forgotten[j].Resource.Name })
	c.rows = s.rows

	accountRetracted := false
	switch {
	case s.hasAcct:
		c.account, c.hasAcct = s.account, true
	case s.acctUndecodable:
		// Keep the held account row, as the live path does for an undecodable value.
	case c.hasAcct:
		c.account, c.hasAcct = AccountReport{}, false
		accountRetracted = true
	}
	c.synced = true
	c.updated = time.Now()
	c.rebuildLocked()
	c.mu.Unlock()

	if accountRetracted {
		c.logAccountRetracted()
	}
	if c.observer == nil {
		return
	}
	for _, row := range forgotten {
		c.observer.ForgetResource(row)
	}
	for _, row := range observed {
		c.observer.ObserveResource(row)
	}
	if s.hasAcct {
		c.observer.ObserveAccount(s.account)
	}
}

// logAccountRetracted declares the one change an observer cannot be told:
// StorageReportObserver has no method that retracts an account row, so an
// observer keeps the last account it was given while the snapshot reports
// AccountKnown false.
func (c *StorageReportConsumer) logAccountRetracted() {
	c.logger.Warn("the storage report's account row is gone; the snapshot no longer reports an account, "+
		"and observers keep the last account row they were given",
		slog.String("key", StorageAccountReportKey))
}

func (c *StorageReportConsumer) logSkip(key string, revision uint64, err error) {
	c.logger.Warn("skipping an undecodable storage report row",
		slog.String("key", key),
		slog.Uint64("revision", revision),
		slog.String("error", err.Error()))
}

func (c *StorageReportConsumer) putResource(key string, row ResourceReport) {
	c.mu.Lock()
	c.rows[key] = row
	c.updated = time.Now()
	c.rebuildLocked()
	c.mu.Unlock()

	if c.observer != nil {
		c.observer.ObserveResource(row)
	}
}

func (c *StorageReportConsumer) putAccount(report AccountReport) {
	c.mu.Lock()
	c.account = report
	c.hasAcct = true
	c.updated = time.Now()
	c.rebuildLocked()
	c.mu.Unlock()

	if c.observer != nil {
		c.observer.ObserveAccount(report)
	}
}

// remove drops a reclaimed row. A tombstone for a key this process never held
// fans out NOTHING: there is no series to retract and no identity to name.
// A delete or purge of the reserved account key retracts the account row.
func (c *StorageReportConsumer) remove(key string) {
	if key == StorageAccountReportKey {
		c.removeAccount()
		return
	}
	c.mu.Lock()
	row, held := c.rows[key]
	if held {
		delete(c.rows, key)
		c.updated = time.Now()
		c.rebuildLocked()
	}
	c.mu.Unlock()

	if held && c.observer != nil {
		c.observer.ForgetResource(row)
	}
}

func (c *StorageReportConsumer) removeAccount() {
	c.mu.Lock()
	held := c.hasAcct
	if held {
		c.account, c.hasAcct = AccountReport{}, false
		c.updated = time.Now()
		c.rebuildLocked()
	}
	c.mu.Unlock()

	if held {
		c.logAccountRetracted()
	}
}

// rebuildLocked recomputes the published snapshot. Callers hold the write lock,
// except the constructor, which has no concurrent readers yet.
func (c *StorageReportConsumer) rebuildLocked() {
	rows := make([]ResourceReport, 0, len(c.rows))
	counts := make(map[PressureState]int, 4)
	notEvaluated := 0
	worst := PressureState("")

	for _, row := range c.rows {
		rows = append(rows, row)
		if !row.Pressure.Evaluated {
			notEvaluated++
			continue
		}
		counts[row.Pressure.State]++
		if pressureSeverity(row.Pressure.State) > pressureSeverity(worst) || worst == "" {
			worst = row.Pressure.State
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Resource.Name < rows[j].Resource.Name })

	c.snapshot = StorageReportSnapshot{
		Resources:      rows,
		Account:        c.account,
		AccountKnown:   c.hasAcct,
		Synced:         c.synced,
		UpdatedAt:      c.updated,
		PressureCounts: counts,
		NotEvaluated:   notEvaluated,
		WorstPressure:  worst,
	}
}

// clone hands out a copy that shares no memory with the consumer's view: the
// row and tier slices, the count map, and every pointer field are copied, so a
// caller editing a snapshot, through a pointer field included, cannot change
// the consumer's state or a later snapshot.
func (s StorageReportSnapshot) clone() StorageReportSnapshot {
	out := s
	out.Resources = make([]ResourceReport, len(s.Resources))
	for i, row := range s.Resources {
		out.Resources[i] = row.clone()
	}
	out.Account = s.Account.clone()
	out.PressureCounts = make(map[PressureState]int, len(s.PressureCounts))
	for state, count := range s.PressureCounts {
		out.PressureCounts[state] = count
	}
	return out
}

func (r ResourceReport) clone() ResourceReport {
	r.Resource.Bytes = r.Resource.Bytes.clone()
	r.Resource.Messages = r.Resource.Messages.clone()
	r.Growth = r.Growth.clone()
	r.Projection = r.Projection.clone()
	return r
}

func (a AccountReport) clone() AccountReport {
	if a.Tiers == nil {
		return a
	}
	tiers := make([]TierComparison, len(a.Tiers))
	for i, tier := range a.Tiers {
		tier.Limit = tier.Limit.clone()
		tier.Growth = tier.Growth.clone()
		tier.Projection = tier.Projection.clone()
		tiers[i] = tier
	}
	a.Tiers = tiers
	return a
}

func (c Capacity) clone() Capacity {
	c.ConfiguredLimit = clonePointer(c.ConfiguredLimit)
	c.Used = clonePointer(c.Used)
	return c
}

func (g Growth) clone() Growth {
	g.BytesPerSecond = clonePointer(g.BytesPerSecond)
	g.ObservedFrom = clonePointer(g.ObservedFrom)
	return g
}

func (p Projection) clone() Projection {
	p.HeadroomBytes = clonePointer(p.HeadroomBytes)
	p.HeadroomFraction = clonePointer(p.HeadroomFraction)
	p.ThresholdBytes = clonePointer(p.ThresholdBytes)
	p.TimeToThreshold = clonePointer(p.TimeToThreshold)
	return p
}

func clonePointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
