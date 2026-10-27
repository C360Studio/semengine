package graphindex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/vocabulary"
)

// reconcileOwnedRows replaces one entity owner's complete membership set in bucket: rows
// the owner filter lists that desired lacks are deleted, desired rows not stored are put
// (stored ones too when overwriteExisting). The listing is deduplicated before the diff,
// since NATS lists through a watcher snapshot and a concurrent Put can show a key twice;
// operations are sorted so retries and logs are deterministic.
func (c *Component) reconcileOwnedRows(
	ctx context.Context,
	indexName string,
	bucket *natsclient.KVStore,
	ownerFilter string,
	desired map[string][]byte,
	overwriteExisting bool,
) error {
	// Validate the filter and every desired key before listing or writing, so a refusal
	// changes nothing.
	if err := natsclient.ValidateKVWildcardFilter(ownerFilter); err != nil {
		return err
	}
	desiredKeys := make([]string, 0, len(desired))
	for key := range desired {
		desiredKeys = append(desiredKeys, key)
	}
	slices.Sort(desiredKeys)
	for _, key := range desiredKeys {
		if err := natsclient.ValidateKVLiteralKey(key); err != nil {
			return err
		}
	}

	existingKeys, err := bucket.KeysByFilter(ctx, ownerFilter)
	c.metrics.kvOperations.WithLabelValues("list", indexName).Inc()
	c.metrics.recordReconcileOperation(indexName, "list", err)
	if err != nil {
		return errs.WrapTransient(err, "Component", "reconcileOwnedRows",
			fmt.Sprintf("list %s rows with %q", indexName, ownerFilter))
	}

	existing := make(map[string]struct{}, len(existingKeys))
	for _, key := range existingKeys {
		if err := natsclient.ValidateKVLiteralKey(key); err != nil {
			return err
		}
		existing[key] = struct{}{}
	}

	stale := make([]string, 0, len(existing))
	for key := range existing {
		if _, keep := desired[key]; !keep {
			stale = append(stale, key)
		}
	}
	slices.Sort(stale)

	var failures []error
	for _, key := range stale {
		delErr := bucket.Delete(ctx, key)
		if natsclient.IsKVNotFoundError(delErr) {
			delErr = nil
		}
		c.metrics.kvOperations.WithLabelValues("delete", indexName).Inc()
		c.metrics.recordReconcileOperation(indexName, "delete", delErr)
		if delErr != nil {
			failures = append(failures, fmt.Errorf("delete %s key %q: %w", indexName, key, delErr))
			c.logger.Debug("failed to retract stale index row",
				slog.String("index", indexName), slog.String("key", key), slog.Any("error", delErr))
		}
	}
	for _, key := range desiredKeys {
		if _, alreadyStored := existing[key]; alreadyStored && !overwriteExisting {
			continue
		}
		_, putErr := bucket.Put(ctx, key, desired[key])
		c.metrics.kvOperations.WithLabelValues("put", indexName).Inc()
		c.metrics.recordReconcileOperation(indexName, "put", putErr)
		if putErr != nil {
			failures = append(failures, fmt.Errorf("put %s key %q: %w", indexName, key, putErr))
			c.logger.Debug("failed to write desired index row",
				slog.String("index", indexName), slog.String("key", key), slog.Any("error", putErr))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return errs.WrapTransient(errIndexWritePartial, "Component", "reconcileOwnedRows",
			fmt.Sprintf("%s owner reconciliation failed: %v", indexName, err))
	}
	return nil
}

// reconcilePredicateIndex replaces entityID's predicate memberships with predicates.
func (c *Component) reconcilePredicateIndex(ctx context.Context, entityID string, predicates map[string]bool) error {
	if err := semtypes.ValidateEntityID(entityID); err != nil {
		return err
	}
	desired := make(map[string][]byte, len(predicates))
	for predicate := range predicates {
		if _, err := vocabulary.ParsePredicate(predicate); err != nil {
			return errs.WrapInvalid(err, "Component", "reconcilePredicateIndex", "invalid predicate")
		}
		desired[predicateIndexKey(predicate, entityID)] = predicateIndexMarker
	}
	return c.reconcileOwnedRows(ctx, "predicate", c.predicateBucket,
		predicateIndexEntityFilter(entityID), desired, false)
}

// reconcileIncomingIndex replaces the incoming edges sourceID owns with incomingByTarget.
func (c *Component) reconcileIncomingIndex(
	ctx context.Context,
	sourceID string,
	incomingByTarget map[string][]graph.IncomingEntry,
) error {
	if err := semtypes.ValidateEntityID(sourceID); err != nil {
		return err
	}
	desired := make(map[string][]byte)
	for targetID, entries := range incomingByTarget {
		if err := semtypes.ValidateEntityID(targetID); err != nil {
			return err
		}
		for _, entry := range entries {
			if !validateIncomingKeyInputs(targetID, sourceID, entry.Predicate, c.logger) {
				return errs.WrapInvalid(errs.ErrInvalidData, "Component", "reconcileIncomingIndex",
					fmt.Sprintf("invalid incoming membership %s -> %s (%s)", sourceID, targetID, entry.Predicate))
			}
			desired[incomingIndexKey(targetID, sourceID, entry.Predicate)] = incomingIndexMarker
		}
	}
	return c.reconcileOwnedRows(ctx, "incoming", c.incomingBucket,
		incomingIndexSourceFilter(sourceID), desired, false)
}
