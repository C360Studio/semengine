package graphingest

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
)

// TestIngestHistograms_RecordSamples verifies the gh#480 processing-duration and
// ingest-lag histograms are built with the component and record observations. The
// consume closure (setupJetStreamConsumer) observes both around handleMessage;
// this guards the metric objects themselves (nil registry: built, registered
// nowhere, design D4). Sample count is read via dto.Metric.Write, as at the pin.
func TestIngestHistograms_RecordSamples(t *testing.T) {
	sampleCount := func(t *testing.T, h interface{ Write(*dto.Metric) error }) uint64 {
		t.Helper()
		var m dto.Metric
		if err := h.Write(&m); err != nil {
			t.Fatalf("histogram Write: %v", err)
		}
		return m.GetHistogram().GetSampleCount()
	}

	// The component's own collectors (design D4; the pin read process-wide
	// singletons through getters). The >= before+1 shape is the pin's.
	c, err := newUnstartedGraphIngest(t, nil, nil)
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	proc := c.processingDuration
	before := sampleCount(t, proc)
	proc.Observe(0.0015)
	if got := sampleCount(t, proc); got < before+1 {
		t.Errorf("processing_duration_seconds sample count = %d, want >= %d", got, before+1)
	}

	lag := c.ingestLag
	lbefore := sampleCount(t, lag)
	lag.Observe(0.5)
	if got := sampleCount(t, lag); got < lbefore+1 {
		t.Errorf("ingest_lag_seconds sample count = %d, want >= %d", got, lbefore+1)
	}
}
