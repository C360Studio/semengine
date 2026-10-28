//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
)

// TestConfigAckWaitAndHeartbeatInterval: the entity_stream port's ack_wait reaches its
// consumer and its heartbeat_interval is read, each with graph-ingest's default when the
// port declares none: 30 s, and a quarter of the wait in effect. A heartbeat_interval
// above half the wait fails Start before the consumer is created (design D8, "Long
// apply"). graph-ingest starts on the fixture broker, and the wait is read back from the
// server's information on graph-ingest's consumer (fix-301 design D2). Ignored, the
// consumer is created with no wait, so natsclient's own fallback applies, and no pair is
// refused. Until an application signals progress, the interval reaches nothing Start
// hands on, so it is read from ingestDeliveryTiming, the function setup calls, on the
// component's own port.
func TestConfigAckWaitAndHeartbeatInterval(t *testing.T) {
	accepted := []struct {
		name                      string
		ackWait, interval         string
		wantAckWait, wantInterval time.Duration
	}{
		{name: "nothing declared", wantAckWait: 30 * time.Second, wantInterval: 7500 * time.Millisecond},
		{name: "ack_wait alone", ackWait: "2s", wantAckWait: 2 * time.Second, wantInterval: 500 * time.Millisecond},
		{name: "both declared", ackWait: "2s", interval: "1s", wantAckWait: 2 * time.Second, wantInterval: time.Second},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			client, c, owner := newAckWaitIngest(t, tc.ackWait, tc.interval)
			defer owner.finish(ctx, t)
			if err := c.Start(owner.startContext(ctx)); err != nil {
				t.Fatalf("Start: %v", err)
			}
			js, err := client.JetStream()
			if err != nil {
				t.Fatalf("JetStream: %v", err)
			}
			consumer, err := js.Consumer(ctx, entityStream.name, restartConsumer)
			if err != nil {
				t.Fatalf("graph-ingest's consumer %s on %s: %v", restartConsumer, entityStream.name, err)
			}
			info, err := consumer.Info(ctx)
			if err != nil {
				t.Fatalf("consumer info: %v", err)
			}
			if info.Config.AckWait != tc.wantAckWait {
				t.Errorf("the server's ack wait for graph-ingest's consumer = %s, want %s",
					info.Config.AckWait, tc.wantAckWait)
			}
			declared, err := component.GetConsumerConfig(c.InputPorts()[0])
			if err != nil {
				t.Fatalf("GetConsumerConfig(%s): %v", c.InputPorts()[0].Name, err)
			}
			if _, interval := ingestDeliveryTiming(declared); interval != tc.wantInterval {
				t.Errorf("the heartbeat interval = %s, want %s", interval, tc.wantInterval)
			}
		})
	}

	t.Run("interval above half the wait", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()
		client, c, owner := newAckWaitIngest(t, "2s", "1500ms")
		defer owner.finish(ctx, t)
		err := c.Start(owner.startContext(ctx))
		if err == nil || !strings.Contains(err.Error(), "heartbeat interval 1.5s exceeds") {
			t.Errorf("Start error = %v, want the 1.5s interval refused", err)
		}
		js, err := client.JetStream()
		if err != nil {
			t.Fatalf("JetStream: %v", err)
		}
		stream, err := js.Stream(ctx, entityStream.name)
		if err != nil {
			t.Fatalf("stream %s: %v", entityStream.name, err)
		}
		// Start has returned, so any consumer its setup created is on the server.
		names := stream.ConsumerNames(ctx)
		var held []string
		for name := range names.Name() {
			held = append(held, name)
		}
		if err := names.Err(); err != nil {
			t.Fatalf("list consumers of %s: %v", entityStream.name, err)
		}
		if len(held) != 0 {
			t.Errorf("consumers of %s after a refused pair = %v, want none", entityStream.name, held)
		}
	})

	// The port's own validation refuses a negative wait when graph-ingest is built, before
	// Start reads it. The port will not encode one, so the JSON is edited as an operator
	// would write it.
	t.Run("negative wait", func(t *testing.T) {
		raw, err := json.Marshal(withEntityStreamTiming(t, DefaultConfig(), "2s", ""))
		if err != nil {
			t.Fatalf("marshal config: %v", err)
		}
		if strings.Count(string(raw), `"ack_wait":"2s"`) != 1 {
			t.Fatalf("the encoded config does not declare ack_wait 2s once: %s", raw)
		}
		raw = []byte(strings.Replace(string(raw), `"ack_wait":"2s"`, `"ack_wait":"-2s"`, 1))
		natsClient, err := natsclient.NewClient("")
		if err != nil {
			t.Fatalf("natsclient.NewClient: %v", err)
		}
		_, err = CreateGraphIngest(raw, testDependencies(t, natsClient))
		if err == nil || !strings.Contains(err.Error(), `"ack_wait"`) {
			t.Errorf("CreateGraphIngest error = %v, want the negative ack_wait refused", err)
		}
	})
}

// newAckWaitIngest builds graph-ingest on a fixture client holding the ENTITY stream, with
// the entity_stream port declaring ackWait and interval ("" declares nothing), and
// initializes it; the caller starts it under the returned owner.
func newAckWaitIngest(t *testing.T, ackWait, interval string) (
	*natsclient.Client, *Component, *graphIngestTestOwner) {
	t.Helper()
	client := newFixtureClient(t, entityStream)
	raw, err := json.Marshal(withEntityStreamTiming(t, DefaultConfig(), ackWait, interval))
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	created, err := CreateGraphIngest(raw, testDependencies(t, client))
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	c := created.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return client, c, newGraphIngestTestOwner(c)
}
