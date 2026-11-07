package graphquery

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/kvwatch"
	"github.com/c360studio/semengine/pkg/graphview"
)

// communityRecheck is how long the community supervisor waits before it opens COMMUNITY_INDEX again,
// after the bucket was missing or a view ended: the pin's recheck_interval default
// (pin:processor/graph-query/component.go:100-103), not configurable (design D3).
const communityRecheck = 5 * time.Second

// communityView is graph-query's view of COMMUNITY_INDEX, one record per group key.
type communityView = graphview.View[clustering.Community]

// communityUnavailable is the supervisor's log message for a failed open or an ended view.
const communityUnavailable = "community view unavailable; localSearch and the text search refuse index_not_ready until it opens again"

// superviseCommunities runs views of COMMUNITY_INDEX until ctx, the run context, ends: it opens the
// bucket through the catalog's reader, which never creates it, runs one view until it returns, waits
// communityRecheck and opens again, as the pin did (pin:component.go:710-750). A missing bucket and a
// lost watch are the same case: the key stays refused until a new view has caught up (design D3).
//
// Each failure is logged, at Warn when it is the first, differs from the last one logged, or follows a
// view that opened, and at Debug otherwise. The pin warned on every retry; until change 7 brings
// COMMUNITY_INDEX's writer, that is one Warn every 5 s in every deployment, for ever.
//
// Once ctx ends it returns nil, or the error of the view's Run when that says more than the
// cancellation (shutdownFailure), which cleanup hands to Stop's caller (#527).
func (c *Component) superviseCommunities(ctx context.Context) error {
	var last string
	for {
		opened, err := c.runCommunityView(ctx)
		if ctx.Err() != nil {
			return shutdownFailure(ctx, opened, err)
		}
		if err == nil { // Run ends a live view only with an error; this keeps the log honest if not
			err = errors.New("the community view returned")
		}
		level := slog.LevelDebug
		if opened || err.Error() != last {
			level = slog.LevelWarn
		}
		last = err.Error()
		c.logger.Log(ctx, level, communityUnavailable, slog.Any("error", err), slog.Duration("retry_in", communityRecheck))
		timer := time.NewTimer(communityRecheck)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// shutdownFailure is what a view's Run returned once ctx, the run context, has ended: nil for a view
// that never opened, one Run ended with the cancellation alone, or one whose watch was lost as ctx
// ended, and Run's error otherwise. Run returns ctx.Err() itself when it ends cleanly, and joins to
// it, or to the loss, a failure to stop its watch (graphview.View.Run, kvwatch.Stop), so errors.Is
// still finds both. A blanket errors.Is on context.Canceled would drop that joined failure.
func shutdownFailure(ctx context.Context, opened bool, err error) error {
	if !opened || err == nil || err == ctx.Err() {
		return nil
	}
	if errors.Is(err, graphview.ErrWatcherLost) && !errors.Is(err, kvwatch.ErrDeliveryOpen) {
		return nil
	}
	return err
}

// runCommunityView opens COMMUNITY_INDEX, publishes a view of it for the handlers and runs the view
// until ctx ends or its watch is lost, reporting whether a view was opened. Once Run has returned the
// view answers nothing, and it is withdrawn so the handlers see none.
func (c *Component) runCommunityView(ctx context.Context) (bool, error) {
	reader, err := kvcatalog.OpenCatalogReader(ctx, c.natsClient, graph.BucketCommunityIndex)
	if err != nil {
		return false, err
	}
	view, err := graphview.New(reader, decodeCommunityEntry)
	if err != nil {
		return false, err
	}
	c.communities.Store(view)
	defer c.communities.Store(nil)
	return true, view.Run(ctx)
}

// decodeCommunityEntry is the view's decode: every key is a group record, and a value that is not the
// record of its key poisons that key until a valid write replaces it.
func decodeCommunityEntry(key string, value []byte, _ graphview.EntryMeta) (clustering.Community, bool, error) {
	record, err := clustering.DecodeCommunity(key, value)
	if err != nil {
		return clustering.Community{}, false, err
	}
	return *record, true, nil
}
