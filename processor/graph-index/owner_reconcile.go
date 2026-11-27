package graphindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

// Owner reconciliation replaces an entity's complete set of index rows with the rows its current
// state plans, through the entity's record (record.go). One apply is a list of operations, built
// whole before the first write (prepareApply) and run in order (runApply):
//
//   - a clean record equal to the plan: nothing;
//   - a clean record that differs: the pending record, the rows added or whose value changed, the
//     deletes of the rows the plan drops, the clean record;
//   - a pending record: the pending record, every planned row, the deletes of every other row it
//     names, the clean record;
//   - no usable record: the owner listings, then as for a pending record whose rows are the listed
//     ones, every planned row that carries a value put again since a listing returns no values.
//
// Record writes are conditional on the revision read, so a second writer between the read and the
// pending put fails the apply before any row is written.

// errRecordConflict is a record write refused because the record is no longer the one the apply
// read: another writer put it in between.
var errRecordConflict = errors.New("graph-index: the record changed since it was read")

// opKind is what one operation of an apply does.
type opKind int

const (
	opPending opKind = iota // put the pending record, conditional on the revision read
	opPut                   // put an index row
	opDelete                // delete an index row; "not found" counts as done
	opClean                 // put the clean record, conditional on the pending record's revision
)

// indexOp is one operation of an apply. bucket and key name an index row for opPut and opDelete;
// value is the row's value for opPut and the encoded record for opPending and opClean.
type indexOp struct {
	kind   opKind
	bucket string
	key    string
	value  []byte
}

// entityApply is one apply of an entity's planned rows: its ordered operations and the revision of
// the record they were built from, 0 when there was none.
type entityApply struct {
	entityID string
	revision uint64
	ops      []indexOp
}

// unchanged reports whether the apply has nothing to write: the clean record equals the plan.
func (a entityApply) unchanged() bool { return len(a.ops) == 0 }

// writeRows replaces entityID's index rows with planned (nil for a deleted or damaged entity),
// retrying a failed attempt in place. Each attempt reads the record again, since a record put whose
// reply was lost may have landed. A record changed by another writer is not retried in place: the
// entity is failed and the repair pass starts again from the stored record. It reports whether the
// record already held planned, so nothing was written.
func (c *Component) writeRows(ctx context.Context, entityID string, planned []indexRow) (bool, error) {
	var unchanged bool
	err := retryIndexWrites(ctx, func() error {
		apply, err := c.prepareApply(ctx, entityID, planned)
		if err != nil {
			return err
		}
		unchanged = apply.unchanged()
		return c.runApply(ctx, apply)
	})
	return unchanged, err
}

// prepareApply reads entityID's record, lists the owned rows when there is no usable record, and
// builds the apply that takes the stored rows to planned. planned must be sorted by bucket then key,
// each place once, as plannedRows returns it.
func (c *Component) prepareApply(ctx context.Context, entityID string, planned []indexRow) (entityApply, error) {
	apply := entityApply{entityID: entityID}
	entry, err := c.recordBucket.Get(ctx, entityID)
	c.metrics.kvOperations.WithLabelValues("get", "record").Inc()
	var record entityRecord
	usable := false
	switch {
	case err == nil:
		apply.revision = entry.Revision
		decoded, decodeErr := decodeRecord(entry.Value)
		if usable = decodeErr == nil && maps.Equal(decoded.generations, c.generations); usable {
			record = decoded
		} else {
			c.logger.Debug("record unusable; listing the owned rows",
				slog.String("entity", entityID), slog.Any("decode_error", decodeErr))
		}
	case natsclient.IsKVNotFoundError(err):
	default:
		// The record cannot be read. Listing finds the stored rows; the pending put is then a create,
		// which a record that does exist refuses.
		c.logger.Debug("record read failed; listing the owned rows",
			slog.String("entity", entityID), slog.Any("error", err))
	}

	if usable && !record.pending {
		if sameRows(record.rows, planned) {
			return apply, nil
		}
		apply.ops = c.changeOps(record.rows, planned)
		return apply, nil
	}
	union := record.rows
	if !usable {
		listed, err := c.listOwnedRows(ctx, entityID)
		if err != nil {
			return entityApply{}, err
		}
		union = listed
	}
	apply.ops = c.recoveryOps(union, planned, !usable)
	return apply, nil
}

// changeOps takes a clean record's rows to planned: the pending record of both, the rows planned
// adds or whose value changed, the deletes of the rows planned drops, the clean record.
func (c *Component) changeOps(recorded []recordRow, planned []indexRow) []indexOp {
	stored := make(map[rowID]recordRow, len(recorded))
	for _, row := range recorded {
		stored[row.id()] = row
	}
	ops := []indexOp{{kind: opPending, value: c.recordOf(true, unionRows(recorded, planned))}}
	keep := make(map[rowID]bool, len(planned))
	for _, row := range planned {
		recordedRow := row.recorded()
		keep[recordedRow.id()] = true
		if prior, ok := stored[recordedRow.id()]; !ok || prior != recordedRow {
			ops = append(ops, indexOp{kind: opPut, bucket: row.bucket, key: row.key, value: row.value})
		}
	}
	for _, row := range recorded {
		if !keep[row.id()] {
			ops = append(ops, indexOp{kind: opDelete, bucket: row.bucket, key: row.key})
		}
	}
	return append(ops, indexOp{kind: opClean, value: c.recordOf(false, recordedRows(planned))})
}

// recoveryOps takes the rows union names, a pending record's or a listing's, to planned: the pending
// record of both, every planned row, the deletes of every other row of union, the clean record. When
// union is a listing (listed), which knows no values, a planned marker row it found is not put again,
// as its value is the bucket's empty marker; every row that carries a value is.
func (c *Component) recoveryOps(union []recordRow, planned []indexRow, listed bool) []indexOp {
	known := make(map[rowID]bool, len(union))
	for _, row := range union {
		known[row.id()] = true
	}
	ops := []indexOp{{kind: opPending, value: c.recordOf(true, unionRows(union, planned))}}
	keep := make(map[rowID]bool, len(planned))
	for _, row := range planned {
		id := rowID{row.bucket, row.key}
		keep[id] = true
		if listed && known[id] && len(row.value) == 0 {
			continue
		}
		ops = append(ops, indexOp{kind: opPut, bucket: row.bucket, key: row.key, value: row.value})
	}
	for _, row := range union {
		if !keep[row.id()] {
			ops = append(ops, indexOp{kind: opDelete, bucket: row.bucket, key: row.key})
		}
	}
	return append(ops, indexOp{kind: opClean, value: c.recordOf(false, recordedRows(planned))})
}

// unionRows is every place in stored or planned, sorted, each once; a planned row carries its
// planned value's hash. A pending record's hashes are never compared: from a pending record every
// planned row is put.
func unionRows(stored []recordRow, planned []indexRow) []recordRow {
	byID := make(map[rowID]recordRow, len(stored)+len(planned))
	for _, row := range stored {
		byID[row.id()] = row
	}
	for _, row := range planned {
		byID[rowID{row.bucket, row.key}] = row.recorded()
	}
	return slices.SortedFunc(maps.Values(byID), compareRows)
}

// recordedRows is planned as a record names it.
func recordedRows(planned []indexRow) []recordRow {
	rows := make([]recordRow, len(planned))
	for i, row := range planned {
		rows[i] = row.recorded()
	}
	return rows
}

// runApply runs apply's operations in order and stops at the first that fails, so nothing is
// written after a failed or refused record put, and the clean record is put only after every row
// write succeeded.
func (c *Component) runApply(ctx context.Context, apply entityApply) error {
	pendingRevision := uint64(0)
	for _, op := range apply.ops {
		var err error
		switch op.kind {
		case opPending:
			pendingRevision, err = c.putRecord(ctx, apply.entityID, op.value, apply.revision)
		case opClean:
			_, err = c.putRecord(ctx, apply.entityID, op.value, pendingRevision)
		case opPut, opDelete:
			err = c.writeRow(ctx, op)
		}
		if err != nil {
			return errs.WrapTransient(err, "Component", "runApply",
				fmt.Sprintf("index writes for %s stopped", apply.entityID))
		}
	}
	return nil
}

// putRecord puts entityID's record, conditional on revision: a create when it is 0, otherwise an
// update that only revision being current allows. It returns the record's new revision; a refusal
// because the record changed is errRecordConflict.
func (c *Component) putRecord(ctx context.Context, entityID string, value []byte, revision uint64) (uint64, error) {
	var newRevision uint64
	var err error
	if revision == 0 {
		newRevision, err = c.recordBucket.Create(ctx, entityID, value)
	} else {
		newRevision, err = c.recordBucket.Update(ctx, entityID, value, revision)
	}
	c.metrics.kvOperations.WithLabelValues("put", "record").Inc()
	c.metrics.recordReconcileOperation("record", "put", err)
	if errors.Is(err, natsclient.ErrKVKeyExists) || errors.Is(err, natsclient.ErrKVRevisionMismatch) {
		err = fmt.Errorf("%w: %w", errRecordConflict, err)
	}
	if err != nil {
		c.logger.Debug("record write failed", slog.String("entity", entityID), slog.Any("error", err))
		return 0, fmt.Errorf("put the record of %q: %w", entityID, err)
	}
	return newRevision, nil
}

// writeRow puts or deletes one index row. A delete answered "not found" is done.
func (c *Component) writeRow(ctx context.Context, op indexOp) error {
	bucket, label := c.indexBucket(op.bucket)
	if bucket == nil {
		return fmt.Errorf("unknown index bucket %q", op.bucket)
	}
	var err error
	operation := "put"
	if op.kind == opPut {
		_, err = bucket.Put(ctx, op.key, op.value)
	} else {
		operation = "delete"
		if err = bucket.Delete(ctx, op.key); natsclient.IsKVNotFoundError(err) {
			err = nil
		}
	}
	c.metrics.kvOperations.WithLabelValues(operation, label).Inc()
	c.metrics.recordReconcileOperation(label, operation, err)
	if err != nil {
		c.logger.Debug("index row write failed", slog.String("index", label), slog.String("operation", operation),
			slog.String("key", op.key), slog.Any("error", err))
		return fmt.Errorf("%s %s key %q: %w", operation, label, op.key, err)
	}
	return nil
}

// indexBucket is the store of the index bucket named name, and its metric label.
func (c *Component) indexBucket(name string) (*natsclient.KVStore, string) {
	switch name {
	case graph.BucketOutgoingIndex:
		return c.outgoingBucket, "outgoing"
	case graph.BucketIncomingIndex:
		return c.incomingBucket, "incoming"
	case graph.BucketPredicateIndex:
		return c.predicateBucket, "predicate"
	case graph.BucketNameIndex:
		return c.nameBucket, "name"
	case graph.BucketAliasIndex:
		return c.aliasBucket, "alias"
	}
	return nil, ""
}

// listOwnedRows lists the rows entityID owns, with no record to name them: its OUTGOING row, which
// is keyed by the entity and so always named, and every INCOMING, PREDICATE, NAME and ALIAS row its
// owner filter lists. INCOMING rows are owned by their source, though their key begins with the
// target, so a target's delete keeps the edges live sources hold to it. ALIAS rows are owned by the
// entity that carries the alias (ADR-114 §2). Every filter and listed key is validated before it is
// used; a listing deduplicates, since NATS lists through a watcher snapshot and a concurrent put can
// show a key twice. The listed rows carry no value hash.
func (c *Component) listOwnedRows(ctx context.Context, entityID string) ([]recordRow, error) {
	if err := validateEntityLiteralKey(entityID); err != nil {
		return nil, err
	}
	owned := []struct {
		label  string
		bucket string
		store  *natsclient.KVStore
		filter string
	}{
		{"incoming", graph.BucketIncomingIndex, c.incomingBucket, incomingIndexSourceFilter(entityID)},
		{"predicate", graph.BucketPredicateIndex, c.predicateBucket, predicateIndexEntityFilter(entityID)},
		{"name", graph.BucketNameIndex, c.nameBucket, nameIndexEntityFilter(entityID)},
		{"alias", graph.BucketAliasIndex, c.aliasBucket, aliasIndexEntityFilter(entityID)},
	}
	for _, set := range owned {
		if err := natsclient.ValidateKVWildcardFilter(set.filter); err != nil {
			return nil, err
		}
	}
	rows := []recordRow{{bucket: graph.BucketOutgoingIndex, key: entityID}}
	for _, set := range owned {
		keys, err := set.store.KeysByFilter(ctx, set.filter)
		c.metrics.kvOperations.WithLabelValues("list", set.label).Inc()
		c.metrics.recordReconcileOperation(set.label, "list", err)
		if err != nil {
			c.logger.Warn("failed to list owned index rows",
				slog.String("index", set.label), slog.String("entity_id", entityID), slog.Any("error", err))
			return nil, errs.WrapTransient(err, "Component", "listOwnedRows",
				fmt.Sprintf("list %s rows with %q", set.label, set.filter))
		}
		for _, key := range keys {
			if err := natsclient.ValidateKVLiteralKey(key); err != nil {
				c.logger.Warn("invalid owned index key listed",
					slog.String("index", set.label), slog.String("key", key), slog.Any("error", err))
				return nil, err
			}
			rows = append(rows, recordRow{bucket: set.bucket, key: key})
		}
	}
	slices.SortFunc(rows, compareRows)
	return slices.CompactFunc(rows, func(a, b recordRow) bool { return a.id() == b.id() }), nil
}

// plannedRows is every row plan asks for, sorted by bucket then key, each place once. Two name
// statements whose names fold to one key keep the last one's value. Every key is validated.
func (c *Component) plannedRows(plan entityIndexPlan) ([]indexRow, error) {
	entityID := plan.entityID
	if err := validateEntityLiteralKey(entityID); err != nil {
		return nil, err
	}
	values := make(map[rowID][]byte)
	for _, entry := range plan.outgoing {
		if err := validateEntityLiteralKey(entry.ToEntityID); err != nil {
			return nil, err
		}
	}
	// The whole list under the entity's own key: readers see the order the statements give.
	outgoing, err := json.Marshal(plan.outgoing)
	if err != nil {
		return nil, errs.Wrap(err, "Component", "plannedRows", "outgoing entry serialization")
	}
	values[rowID{graph.BucketOutgoingIndex, entityID}] = outgoing
	for targetID, entries := range plan.incomingByTarget {
		for _, entry := range entries {
			if !validateIncomingKeyInputs(targetID, entityID, entry.Predicate, c.logger) {
				return nil, errs.WrapInvalid(errs.ErrInvalidData, "Component", "plannedRows",
					fmt.Sprintf("invalid incoming membership %s -> %s (%s)", entityID, targetID, entry.Predicate))
			}
			values[rowID{graph.BucketIncomingIndex, incomingIndexKey(targetID, entityID, entry.Predicate)}] = incomingIndexMarker
		}
	}
	for predicate := range plan.predicatesUsed {
		if _, err := vocabulary.ParsePredicate(predicate); err != nil {
			return nil, errs.WrapInvalid(err, "Component", "plannedRows", "invalid predicate")
		}
		values[rowID{graph.BucketPredicateIndex, predicateIndexKey(predicate, entityID)}] = predicateIndexMarker
	}
	for _, write := range plan.nameWrites {
		if write.name == "" || !validateNameKeyInputs(entityID, write.predicate, c.logger) {
			return nil, errs.WrapInvalid(errs.ErrInvalidData, "Component", "plannedRows",
				"name membership contains an invalid name, entity ID, or predicate")
		}
		// The original-case spelling and the priority live in the value, so a name whose folded key
		// stays the same can still change the row.
		value, err := json.Marshal(nameCompositeValue{Name: write.name, Priority: write.priority})
		if err != nil {
			return nil, errs.Wrap(err, "Component", "plannedRows", "marshal name value")
		}
		values[rowID{graph.BucketNameIndex, nameCompositeKey(nameIndexKey(write.name), entityID, write.predicate)}] = value
	}
	for _, write := range plan.aliasWrites {
		values[rowID{graph.BucketAliasIndex, aliasIndexKey(write.alias, entityID, write.predicate)}] = []byte{}
	}
	rows := make([]indexRow, 0, len(values))
	for id, value := range values {
		if err := natsclient.ValidateKVLiteralKey(id.key); err != nil {
			return nil, err
		}
		rows = append(rows, indexRow{bucket: id.bucket, key: id.key, value: value})
	}
	slices.SortFunc(rows, func(a, b indexRow) int {
		return compareRows(recordRow{bucket: a.bucket, key: a.key}, recordRow{bucket: b.bucket, key: b.key})
	})
	return rows, nil
}
