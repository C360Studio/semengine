package graphindex

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
)

// goldenFixture is the stored encoding of goldenRecords' two records, one hex line each, pending
// first. It changes only together with recordFormat.
var goldenFixture = filepath.Join("testdata", "record-golden.hex")

// goldenRecords is a fixed plan's pending and clean records: rows of three buckets, an empty value
// and a value that carries data, under three generations.
func goldenRecords() []entityRecord {
	id := "acme.ops.test.system.widget.001"
	planned := []indexRow{
		{bucket: graph.BucketIncomingIndex, key: "acme.ops.test.system.widget.002." + id + ".746573742e6c696e6b2e746f", value: []byte{}},
		{bucket: graph.BucketNameIndex, key: "8ed3f6ad685b959ead7022518e1af76cd816f8e8ec7ccdda1ed4018e8f2223f8." + id + ".64632e7465726d732e7469746c65", value: []byte(`{"name":"Alpha","priority":0}`)},
		{bucket: graph.BucketOutgoingIndex, key: id, value: []byte(`[{"to_entity_id":"acme.ops.test.system.widget.002","predicate":"test.link.to"}]`)},
	}
	generations := map[string]uint64{
		graph.BucketEntityStates:  1764000000000000001,
		graph.BucketIncomingIndex: 1764000000000000002,
		graph.BucketOutgoingIndex: 1764000000000000003,
	}
	clean := recordedRows(planned)
	pending := append([]recordRow{{bucket: graph.BucketAliasIndex, key: "aa." + id + ".bb"}}, clean...)
	return []entityRecord{
		{pending: true, generations: generations, rows: pending},
		{pending: false, generations: generations, rows: clean},
	}
}

// TestRecordGoldenFixture builds a fixed plan's records and requires the stored bytes: an encoding
// that moves without a new recordFormat fails here. Each fixture line decodes back to its record.
func TestRecordGoldenFixture(t *testing.T) {
	raw, err := os.ReadFile(goldenFixture)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(string(raw))
	records := goldenRecords()
	if len(lines) != len(records) {
		t.Fatalf("%s holds %d records, want %d", goldenFixture, len(lines), len(records))
	}
	for i, r := range records {
		got := hex.EncodeToString(encodeRecord(r))
		if got != lines[i] {
			t.Errorf("record %d (pending %v) encodes to\n%s\nwant the fixture's\n%s\n"+
				"An encoding change needs a new recordFormat and a new fixture.", i, r.pending, got, lines[i])
		}
		stored, err := hex.DecodeString(lines[i])
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeRecord(stored)
		if err != nil {
			t.Fatalf("decode fixture record %d: %v", i, err)
		}
		if !bytes.Equal(encodeRecord(decoded), stored) || decoded.pending != r.pending {
			t.Errorf("fixture record %d does not decode to the record it encodes", i)
		}
	}
}

// TestRecordDecodeRefusesNonCanonical: every record that is not the canonical encoding of one is
// refused, so the apply lists rather than trusting it.
func TestRecordDecodeRefusesNonCanonical(t *testing.T) {
	valid := encodeRecord(goldenRecords()[0])
	reversed := goldenRecords()[0]
	reversed.rows[0], reversed.rows[1] = reversed.rows[1], reversed.rows[0]
	duplicated := goldenRecords()[1]
	duplicated.rows = append(duplicated.rows, duplicated.rows[len(duplicated.rows)-1])
	otherFormat := bytes.Replace(valid, []byte(recordFormat), []byte("graph-index-record/0"), 1)
	unknownState := bytes.Replace(valid, []byte(recordPending), []byte("waiting"), 1)
	for name, data := range map[string][]byte{
		"empty":          {},
		"truncated":      valid[:len(valid)-1],
		"trailing byte":  append(bytes.Clone(valid), 0),
		"rows unsorted":  encodeRecord(reversed),
		"row twice":      encodeRecord(duplicated),
		"unknown state":  unknownState,
		"another format": otherFormat,
		"huge count":     append(encodeRecord(entityRecord{})[:len(encodeRecord(entityRecord{}))-8], 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff),
	} {
		if _, err := decodeRecord(data); err == nil {
			t.Errorf("%s: decodeRecord accepted it", name)
		}
	}
	if _, err := decodeRecord(otherFormat); !errors.Is(err, errRecordFormat) {
		t.Errorf("another format: error %v, want errRecordFormat", err)
	}
}

// FuzzRecordDecode: decoding never panics, and whatever decodes encodes back to the same bytes, so
// two different stored records never read as one.
func FuzzRecordDecode(f *testing.F) {
	for _, r := range goldenRecords() {
		f.Add(encodeRecord(r))
	}
	f.Add(encodeRecord(entityRecord{}))
	f.Add([]byte{})
	f.Add([]byte("not a record"))
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := decodeRecord(data)
		if err != nil {
			return
		}
		if got := encodeRecord(r); !bytes.Equal(got, data) {
			t.Fatalf("decoded record re-encodes to %x, want %x", got, data)
		}
	})
}

// TestRetryIndexWritesStopsOnRecordConflict: a record another writer changed is not retried in
// place, while a transient failure is retried up to indexWriteMaxAttempts.
func TestRetryIndexWritesStopsOnRecordConflict(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"conflict":  {fmt.Errorf("put: %w: %w", errRecordConflict, natsclient.ErrKVRevisionMismatch), 1},
		"transient": {errors.New("kv put: timeout"), indexWriteMaxAttempts},
	} {
		attempts := 0
		err := retryIndexWrites(t.Context(), func() error { attempts++; return tc.err })
		if !errors.Is(err, tc.err) || attempts != tc.want {
			t.Errorf("%s: %d attempts, error %v; want %d attempts and the attempt's error", name, attempts, err, tc.want)
		}
	}
}
