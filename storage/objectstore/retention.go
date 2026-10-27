package objectstore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// backingStreamRetention reads the age limit (the ObjectStore TTL) and the size
// limit of a bucket's backing stream, OBJ_<bucket>. It is the ObjectStore twin of
// the KV guard's reader in natsclient.
func backingStreamRetention(
	ctx context.Context, js jetstream.JetStream, bucket string,
) (maxAge time.Duration, maxBytes int64, err error) {
	stream, err := js.Stream(ctx, natsclient.ObjectStoreStreamPrefix+bucket)
	if err != nil {
		return 0, 0, errs.WrapTransient(err, "Store", "backingStreamRetention",
			fmt.Sprintf("get backing stream for content store %q", bucket))
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return 0, 0, errs.WrapTransient(err, "Store", "backingStreamRetention",
			fmt.Sprintf("read backing stream info for content store %q", bucket))
	}
	return info.Config.MaxAge, info.Config.MaxBytes, nil
}

// reconcileNoLifecycleRetention keeps age and size eviction off a content
// bucket (ADR-068): a live graph entity can point at any object, and an object
// that expires strands it. It is the ObjectStore twin of
// natsclient.ReconcileNoLifecycleRetention and runs two steps on OBJ_<bucket>:
//
//  1. If the stream has a TTL or a binding MaxBytes, clear both with one stream
//     update and log a warning naming the bucket and what was removed. Clearing
//     deletes no object. A clean stream is read once and left alone.
//  2. Re-read the stream and return a fatal error wrapping
//     natsclient.ErrGraphBucketRetention if anything binding remains (the update
//     was refused, or another writer set it again).
//
// Both steps decide "binding" with natsclient.CheckNoLifecycleRetention.
func reconcileNoLifecycleRetention(
	ctx context.Context, js jetstream.JetStream, bucket string, logger *slog.Logger,
) error {
	maxAge, maxBytes, err := backingStreamRetention(ctx, js, bucket)
	if err != nil {
		return err
	}
	if natsclient.CheckNoLifecycleRetention(bucket, maxAge, maxBytes) == nil {
		return nil
	}

	stream, err := js.Stream(ctx, natsclient.ObjectStoreStreamPrefix+bucket)
	if err != nil {
		return errs.WrapTransient(err, "Store", "reconcileNoLifecycleRetention",
			fmt.Sprintf("get backing stream to reconcile content store %q", bucket))
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return errs.WrapTransient(err, "Store", "reconcileNoLifecycleRetention",
			fmt.Sprintf("read backing stream info to reconcile content store %q", bucket))
	}
	cfg := info.Config
	cfg.MaxAge = 0
	cfg.MaxBytes = -1 // NATS "unlimited", the backing stream's own default
	if _, uerr := js.UpdateStream(ctx, cfg); uerr != nil {
		// Not returned: the re-read below fails closed with the retention error,
		// which is what a refused update means for this bucket.
		logger.Warn("could not strip lifecycle retention from content store; will re-assert fail-closed",
			slog.String("bucket", bucket),
			slog.Duration("max_age", maxAge),
			slog.Int64("max_bytes", maxBytes),
			slog.Any("error", uerr))
	} else {
		logger.Warn("removed lifecycle retention from content store",
			slog.String("bucket", bucket),
			slog.Duration("removed_max_age", maxAge),
			slog.Int64("removed_max_bytes", maxBytes))
	}

	maxAge, maxBytes, err = backingStreamRetention(ctx, js, bucket)
	if err != nil {
		return err
	}
	if err := natsclient.CheckNoLifecycleRetention(bucket, maxAge, maxBytes); err != nil {
		return errs.WrapFatal(err, "Store", "reconcileNoLifecycleRetention",
			fmt.Sprintf("content store %q retains lifecycle eviction after reconcile", bucket))
	}
	return nil
}
