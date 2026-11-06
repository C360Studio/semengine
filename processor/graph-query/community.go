package graphquery

import (
	"context"
	"log/slog"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/pkg/graphview"
)

// communityRecheck is how long the community supervisor waits before it opens COMMUNITY_INDEX again,
// after the bucket was missing or a view ended: the pin's recheck_interval default
// (pin:processor/graph-query/component.go:100-103), not configurable (design D3).
const communityRecheck = 5 * time.Second

// communityView is graph-query's view of COMMUNITY_INDEX, one record per group key.
type communityView = graphview.View[clustering.Community]

// superviseCommunities runs views of COMMUNITY_INDEX until ctx, the run context, ends: it opens the
// bucket through the catalog's reader, which never creates it, runs one view until it returns, waits
// communityRecheck and opens again, as the pin did (pin:component.go:710-750). A missing bucket and a
// lost watch are the same case: the key stays refused until a new view has caught up (design D3).
func (c *Component) superviseCommunities(ctx context.Context) {
	for {
		err := c.runCommunityView(ctx)
		if ctx.Err() != nil {
			return
		}
		c.logger.Warn("community view unavailable; localSearch refuses index_not_ready until it opens again",
			slog.Any("error", err), slog.Duration("retry_in", communityRecheck))
		timer := time.NewTimer(communityRecheck)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// runCommunityView opens COMMUNITY_INDEX, publishes a view of it for the handlers and runs the view
// until ctx ends or its watch is lost. Once Run has returned the view answers nothing, and it is
// withdrawn so the handlers see none.
func (c *Component) runCommunityView(ctx context.Context) error {
	reader, err := kvcatalog.OpenCatalogReader(ctx, c.natsClient, graph.BucketCommunityIndex)
	if err != nil {
		return err
	}
	view, err := graphview.New(reader, decodeCommunityEntry)
	if err != nil {
		return err
	}
	c.communities.Store(view)
	defer c.communities.Store(nil)
	return view.Run(ctx)
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
