package graphindex

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// The record. Each entity has one entry in GRAPH_INDEX_RECORDS, under its ID, naming the index
// rows the entity owns: each row's bucket, key and the SHA-256 of its value. Only the entity's lane
// writes it. A clean record names exactly the rows the last completed apply wrote, so an apply whose
// planned rows equal it has nothing to do. A pending record is written before an apply's first row
// write and names every row the entity may own: the rows a clean record or an earlier pending record
// named, or a listing found, plus the planned rows. However many applies in a row are cut off, the
// stored record names every row the entity can own, so its old rows are found without listing.
//
// The encoding is a sequence of fields, each a string or byte slice behind its length as a
// big-endian uint64, and big-endian uint64 numbers:
//
//	format, state ("pending" or "clean"),
//	the generation count, then each generation's bucket and creation time, sorted by bucket,
//	the row count, then each row's bucket, key and 32-byte value hash, sorted by bucket then key.
//
// Length prefixes keep two different records from encoding to the same bytes. A record is decoded
// only in its canonical form (sorted, each row once, nothing after the last row), so a decoded
// record encodes back to the bytes it came from.

// recordFormat names the record encoding. Bump it only when the encoding changes, or with a fix for
// a defect that left rows no record names: every record of another format is unusable, so every
// entity is listed once more. A change to how rows are planned or keyed needs no bump, since the
// record holds each row's literal bucket, key and value hash and the new plan differs from it.
const recordFormat = "graph-index-record/1"

const (
	recordPending = "pending"
	recordClean   = "clean"
)

// errRecordFormat is a record written in another format than recordFormat.
var errRecordFormat = errors.New("graph-index record: another format")

// recordRow is one index row a record names.
type recordRow struct {
	bucket string
	key    string
	hash   [sha256.Size]byte
}

// rowID is a row's place: its bucket and key.
type rowID struct {
	bucket string
	key    string
}

func (r recordRow) id() rowID { return rowID{r.bucket, r.key} }

// compareRows orders rows by bucket, then key: the order a record holds them in.
func compareRows(a, b recordRow) int {
	return cmp.Or(cmp.Compare(a.bucket, b.bucket), cmp.Compare(a.key, b.key))
}

// entityRecord is one decoded record.
type entityRecord struct {
	pending     bool
	generations map[string]uint64
	rows        []recordRow // sorted by compareRows, each place once
}

// encodeRecord is r's stored bytes. r's rows must be sorted by compareRows, each place once.
func encodeRecord(r entityRecord) []byte {
	var buf bytes.Buffer
	writeField := func(b []byte) {
		buf.Write(binary.BigEndian.AppendUint64(nil, uint64(len(b))))
		buf.Write(b)
	}
	writeNumber := func(n uint64) { buf.Write(binary.BigEndian.AppendUint64(nil, n)) }

	writeField([]byte(recordFormat))
	state := recordClean
	if r.pending {
		state = recordPending
	}
	writeField([]byte(state))
	writeNumber(uint64(len(r.generations)))
	for _, bucket := range slices.Sorted(maps.Keys(r.generations)) {
		writeField([]byte(bucket))
		writeNumber(r.generations[bucket])
	}
	writeNumber(uint64(len(r.rows)))
	for _, row := range r.rows {
		writeField([]byte(row.bucket))
		writeField([]byte(row.key))
		buf.Write(row.hash[:])
	}
	return buf.Bytes()
}

// decodeRecord decodes data, refusing another format (errRecordFormat) and anything but the
// canonical encoding of a record.
func decodeRecord(data []byte) (entityRecord, error) {
	d := recordDecoder{data: data}
	if format := d.field(); d.err == nil && string(format) != recordFormat {
		return entityRecord{}, fmt.Errorf("%w %q", errRecordFormat, format)
	}
	var r entityRecord
	switch state := string(d.field()); {
	case d.err != nil:
	case state == recordPending:
		r.pending = true
	case state == recordClean:
	default:
		return entityRecord{}, fmt.Errorf("graph-index record: unknown state %q", state)
	}
	count := d.count(8 + 8)
	r.generations = make(map[string]uint64, count)
	previous := ""
	for i := range count {
		bucket := string(d.field())
		generation := d.number()
		if d.err == nil && i > 0 && bucket <= previous {
			return entityRecord{}, errors.New("graph-index record: generations not sorted")
		}
		previous = bucket
		r.generations[bucket] = generation
	}
	count = d.count(8 + 8 + sha256.Size)
	r.rows = make([]recordRow, 0, count)
	for i := range count {
		row := recordRow{bucket: string(d.field()), key: string(d.field())}
		copy(row.hash[:], d.bytes(sha256.Size))
		if d.err == nil && i > 0 && compareRows(r.rows[i-1], row) >= 0 {
			return entityRecord{}, errors.New("graph-index record: rows not sorted")
		}
		r.rows = append(r.rows, row)
	}
	if d.err != nil {
		return entityRecord{}, d.err
	}
	if len(d.data) != 0 {
		return entityRecord{}, errors.New("graph-index record: bytes after the last row")
	}
	return r, nil
}

// recordDecoder reads a record's fields in order; the first short read sets err, after which
// every read returns nothing.
type recordDecoder struct {
	data []byte
	err  error
}

func (d *recordDecoder) bytes(n uint64) []byte {
	if d.err != nil {
		return nil
	}
	if uint64(len(d.data)) < n {
		d.err = errors.New("graph-index record: truncated")
		return nil
	}
	b := d.data[:n]
	d.data = d.data[n:]
	return b
}

func (d *recordDecoder) number() uint64 {
	b := d.bytes(8)
	if d.err != nil {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

func (d *recordDecoder) field() []byte {
	return d.bytes(d.number())
}

// count reads an item count, refusing one that the remaining bytes cannot hold at minSize bytes
// an item, so a corrupt count never sizes an allocation.
func (d *recordDecoder) count(minSize uint64) uint64 {
	n := d.number()
	if d.err == nil && n > uint64(len(d.data))/minSize {
		d.err = errors.New("graph-index record: count larger than the record")
		return 0
	}
	return n
}

// indexRow is one row an entity's plan asks for.
type indexRow struct {
	bucket string
	key    string
	value  []byte
}

// recorded is row as a record names it.
func (row indexRow) recorded() recordRow {
	return recordRow{bucket: row.bucket, key: row.key, hash: sha256.Sum256(row.value)}
}

// recordOf is the record of rows, sorted by compareRows, with state pending or clean.
func (c *Component) recordOf(pending bool, rows []recordRow) []byte {
	return encodeRecord(entityRecord{pending: pending, generations: c.generations, rows: rows})
}

// sameRows reports whether a record's rows are exactly planned's, values included.
func sameRows(recorded []recordRow, planned []indexRow) bool {
	return slices.EqualFunc(recorded, planned, func(r recordRow, p indexRow) bool { return r == p.recorded() })
}
