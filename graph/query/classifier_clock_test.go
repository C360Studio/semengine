package query

import (
	"testing"
	"time"
	_ "time/tzdata" // the zones below load the same on every machine

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimeRangesFromOneInstant: every range of a question comes from the one
// instant the keyword tier is given, in that instant's zone (#479 item 1,
// design D5). Each expected start and end is written out by hand.
func TestTimeRangesFromOneInstant(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	plus14 := time.FixedZone("UTC+14", 14*3600)
	utc := func(y int, mo time.Month, d, h, mi, s, ns int) time.Time {
		return time.Date(y, mo, d, h, mi, s, ns, time.UTC)
	}

	tests := []struct {
		name      string
		query     string
		now       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			// 2026-03-08 is the US change to daylight time: the day is 23 hours,
			// starting at EST (-05:00) and ending at EDT (-04:00).
			name:      "today on a DST day",
			query:     "events from today",
			now:       time.Date(2026, 3, 8, 15, 0, 0, 0, newYork),
			wantStart: utc(2026, 3, 8, 5, 0, 0, 0),
			wantEnd:   utc(2026, 3, 9, 3, 59, 59, 999999999),
		},
		{
			name:      "yesterday is the DST day",
			query:     "events from yesterday",
			now:       time.Date(2026, 3, 9, 10, 0, 0, 0, newYork),
			wantStart: utc(2026, 3, 8, 5, 0, 0, 0),
			wantEnd:   utc(2026, 3, 9, 3, 59, 59, 999999999),
		},
		{
			// 04:30 EDT is 08:30Z; three hours earlier is 00:30 EST, four hours
			// earlier on the wall clock.
			name:      "last 3 hours across the DST change",
			query:     "alarms in the last 3 hours",
			now:       time.Date(2026, 3, 8, 4, 30, 0, 0, newYork),
			wantStart: utc(2026, 3, 8, 5, 30, 0, 0),
			wantEnd:   utc(2026, 3, 8, 8, 30, 0, 0),
		},
		{
			name:      "last 2 days",
			query:     "data from the last 2 days",
			now:       utc(2026, 10, 30, 12, 0, 0, 0),
			wantStart: utc(2026, 10, 28, 12, 0, 0, 0),
			wantEnd:   utc(2026, 10, 30, 12, 0, 0, 0),
		},
		{
			// At 11:00Z it is already 01:00 on the 31st at UTC+14.
			name:      "today in the zone of now, not UTC",
			query:     "events today",
			now:       utc(2026, 10, 30, 11, 0, 0, 0).In(plus14),
			wantStart: utc(2026, 10, 30, 10, 0, 0, 0),
			wantEnd:   utc(2026, 10, 31, 9, 59, 59, 999999999),
		},
		{
			name:      "this week on a Sunday",
			query:     "deliveries this week",
			now:       utc(2026, 3, 1, 12, 0, 0, 0), // a Sunday
			wantStart: utc(2026, 3, 1, 0, 0, 0, 0),
			wantEnd:   utc(2026, 3, 7, 23, 59, 59, 999999999),
		},
		{
			name:      "this week on a Saturday, across a month edge",
			query:     "deliveries this week",
			now:       utc(2026, 2, 28, 23, 59, 59, 0), // a Saturday
			wantStart: utc(2026, 2, 22, 0, 0, 0, 0),
			wantEnd:   utc(2026, 2, 28, 23, 59, 59, 999999999),
		},
		{
			// The month ends one nanosecond before the next starts, like today, this week and this year
			// (the pin cut off its last second).
			name:      "this month in February",
			query:     "readings this month",
			now:       utc(2026, 2, 15, 8, 0, 0, 0),
			wantStart: utc(2026, 2, 1, 0, 0, 0, 0),
			wantEnd:   utc(2026, 2, 28, 23, 59, 59, 999999999),
		},
		{
			name:      "this month in a leap February",
			query:     "readings this month",
			now:       utc(2028, 2, 10, 8, 0, 0, 0),
			wantStart: utc(2028, 2, 1, 0, 0, 0, 0),
			wantEnd:   utc(2028, 2, 29, 23, 59, 59, 999999999),
		},
		{
			// March 2026 in New York starts at EST and ends at EDT.
			name:      "this month across the DST change",
			query:     "readings this month",
			now:       time.Date(2026, 3, 20, 9, 0, 0, 0, newYork),
			wantStart: utc(2026, 3, 1, 5, 0, 0, 0),
			wantEnd:   utc(2026, 4, 1, 3, 59, 59, 999999999),
		},
	}

	k := NewKeywordClassifier()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := k.classify(tt.query, tt.now)
			require.NoError(t, err)
			require.NotNil(t, opts.TimeRange)
			assert.True(t, tt.wantStart.Equal(opts.TimeRange.Start), "start: want %s, got %s", tt.wantStart, opts.TimeRange.Start)
			assert.True(t, tt.wantEnd.Equal(opts.TimeRange.End), "end: want %s, got %s", tt.wantEnd, opts.TimeRange.End)
		})
	}
}

// TestOutOfRangeNumbersAreRefused: a question whose number breaks its bound
// gets no result and an invalid-input error naming the phrase from the chain,
// and no such hint from the keyword tier (#479 item 2, design D6 option A).
// Inventory Q3's cases are the first rows.
func TestOutOfRangeNumbersAreRefused(t *testing.T) {
	refused := []struct {
		query  string
		phrase string // the phrase the error names
		bound  string // text naming the bound it broke
	}{
		{"events in the last 3000000 hours", "last 3000000 hours", "at most 2562047 hours"},
		{"events in the last 120000 days", "last 120000 days", "at most 106751 days"},
		{"sensors within 5km of 95,10", "within 5km of 95,10", "latitude"},
		{"sensors within 5km of 10,500", "within 5km of 10,500", "longitude"},
		// The first number past each bound.
		{"events in the last 2562048 hours", "last 2562048 hours", "at most 2562047 hours"},
		{"events in the last 106752 days", "last 106752 days", "at most 106751 days"},
		{"events in the last 0 hours", "last 0 hours", "at least 1"},
		{"events in the last 0 days", "last 0 days", "at least 1"},
		{"events in the last 99999999999999999999 hours", "last 99999999999999999999 hours", "at most 2562047 hours"},
		{"sensors within 5km of -90.5,10", "within 5km of -90.5,10", "latitude"},
		{"sensors within 5km of 10,-180.01", "within 5km of 10,-180.01", "longitude"},
		// A zero radius gives no box, but a bad coordinate is still refused.
		{"sensors within 0km of 95,10", "within 0km of 95,10", "latitude"},
		{"sensors within 1" + zeros(400) + "km of 0,0", "within 1" + zeros(400) + "km of 0,0", "radius"},
	}

	chain := NewClassifierChain(NewKeywordClassifier(), nil)
	k := NewKeywordClassifier()
	for _, tt := range refused {
		t.Run("refused/"+short(tt.phrase), func(t *testing.T) {
			got, err := chain.ClassifyQuery(t.Context(), tt.query)
			assert.Nil(t, got, "no result")
			require.Error(t, err)
			assert.True(t, errs.IsInvalid(err), "invalid input: %v", err)
			assert.ErrorContains(t, err, `"`+tt.phrase+`"`)
			assert.ErrorContains(t, err, tt.bound)

			opts := k.ClassifyQuery(t.Context(), tt.query)
			require.NotNil(t, opts)
			assert.Nil(t, opts.TimeRange, "the keyword tier gives no time range for %q", tt.phrase)
			assert.Nil(t, opts.GeoBounds, "the keyword tier gives no box for %q", tt.phrase)
		})
	}

	t.Run("both phrases named", func(t *testing.T) {
		got, err := chain.ClassifyQuery(t.Context(), "events in the last 3000000 hours within 5km of 95,10")
		assert.Nil(t, got)
		assert.True(t, errs.IsInvalid(err), "invalid input: %v", err)
		assert.ErrorContains(t, err, `"last 3000000 hours"`)
		assert.ErrorContains(t, err, `"within 5km of 95,10"`)
	})

	t.Run("other hints of a refused question stay with the keyword tier", func(t *testing.T) {
		opts := k.ClassifyQuery(t.Context(), "how many sensors within 5km of 95,10")
		assert.Equal(t, AggregationCount, opts.AggregationType)
		assert.Nil(t, opts.GeoBounds)
	})

	// The largest numbers inside each bound answer: last N hours starts N hours
	// before now.
	t.Run("bounds themselves answer", func(t *testing.T) {
		now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
		for query, span := range map[string]time.Duration{
			"events in the last 1 hours":       time.Hour,
			"events in the last 2562047 hours": 2562047 * time.Hour,
			"events in the last 106751 days":   106751 * 24 * time.Hour,
		} {
			opts, _, err := k.classify(query, now)
			require.NoError(t, err, "%q", query)
			require.NotNil(t, opts.TimeRange, "%q", query)
			assert.True(t, now.Add(-span).Equal(opts.TimeRange.Start), "%q", query)
			assert.True(t, now.Equal(opts.TimeRange.End), "%q", query)
		}
	})

	// Pole clamping is geometry, not a refusal (design D6).
	boxes := []struct {
		query string
		want  SpatialBounds
	}{
		// 5 km is 5/111 degrees of latitude: 0.045045045...
		{"sensors within 5km of 89.9999,10", SpatialBounds{North: 90, South: 89.954854954954955, East: 180, West: -180}},
		{"sensors within 5km of -89.9999,10", SpatialBounds{North: -89.954854954954955, South: -90, East: 180, West: -180}},
		{"sensors within 5km of 90,0", SpatialBounds{North: 90, South: 89.954954954954955, East: 180, West: -180}},
		{"sensors within 50000km of 0,0", SpatialBounds{North: 90, South: -90, East: 180, West: -180}},
		// The antimeridian is not wrapped: East stays above 180 (declared).
		{"sensors within 5km of 0,179.99", SpatialBounds{North: 0.045045045045045, South: -0.045045045045045, East: 180.035045045045045, West: 179.944954954954955}},
		{"sensors within 5km of 0,180", SpatialBounds{North: 0.045045045045045, South: -0.045045045045045, East: 180.045045045045045, West: 179.954954954954955}},
	}
	for _, tt := range boxes {
		t.Run("answers/"+tt.query, func(t *testing.T) {
			got, err := chain.ClassifyQuery(t.Context(), tt.query)
			require.NoError(t, err)
			require.NotNil(t, got)
			box, ok := got.Options["geo_bounds"].(*SpatialBounds)
			require.True(t, ok, "geo_bounds: %#v", got.Options)
			assert.InDelta(t, tt.want.North, box.North, 1e-9, "north")
			assert.InDelta(t, tt.want.South, box.South, 1e-9, "south")
			assert.InDelta(t, tt.want.East, box.East, 1e-9, "east")
			assert.InDelta(t, tt.want.West, box.West, 1e-9, "west")
		})
	}
}

func zeros(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}

// short keeps subtest names readable for the 400-digit radius.
func short(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// The clock window helpers check a range the classifier computed from its own
// reading of the clock, taken after before was read and before after was read
// (the call returning orders the reading before after).

// assertCalendarDay asserts that got is the whole calendar day offset days from
// the classifier's reading, in time.Local. A day boundary can fall between
// before and after, so the day of either reading is accepted.
func assertCalendarDay(t *testing.T, got any, before, after time.Time, offset int) {
	t.Helper()
	tr, ok := got.(*TimeRange)
	require.True(t, ok && tr != nil, "time_range: %#v", got)
	day := func(at time.Time) (time.Time, time.Time) {
		at = at.In(time.Local)
		start := time.Date(at.Year(), at.Month(), at.Day()+offset, 0, 0, 0, 0, time.Local)
		return start, start.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	for _, at := range []time.Time{before, after} {
		start, end := day(at)
		if tr.Start.Equal(start) && tr.End.Equal(end) {
			return
		}
	}
	t.Errorf("time_range %s to %s is not day %+d of %s or %s", tr.Start, tr.End, offset, before, after)
}

// assertSpanEndingNow asserts that got ends at the classifier's reading and
// starts span before it.
func assertSpanEndingNow(t *testing.T, got any, before, after time.Time, span time.Duration) {
	t.Helper()
	tr, ok := got.(*TimeRange)
	require.True(t, ok && tr != nil, "time_range: %#v", got)
	assert.False(t, tr.End.Before(before), "end %s before %s", tr.End, before)
	assert.False(t, tr.End.After(after), "end %s after %s", tr.End, after)
	assert.True(t, tr.End.Add(-span).Equal(tr.Start), "start %s is not %s before end %s", tr.Start, span, tr.End)
}
