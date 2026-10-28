//go:build integration

package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/payloadregistry"
)

// heldDocument is a registered ContentStorable test payload whose RawContent reports that it was
// entered and then waits for release, so a test can hold an input handler in the middle of a
// write. Its channels come from the factory the test registers; they are not on the wire.
type heldDocument struct {
	ID      string `json:"id"`
	entered chan<- struct{}
	release <-chan struct{}
}

func (d *heldDocument) Schema() message.Type {
	return message.Type{Domain: "objectstoretest", Category: "held", Version: "v1"}
}
func (d *heldDocument) Validate() error                       { return nil }
func (d *heldDocument) EntityID() string                      { return d.ID }
func (d *heldDocument) Triples() []message.Triple             { return nil }
func (d *heldDocument) StorageRef() *message.StorageReference { return nil }
func (d *heldDocument) ContentFields() map[string]string {
	return map[string]string{message.ContentRoleBody: "body"}
}
func (d *heldDocument) RawContent() map[string]string {
	select {
	case d.entered <- struct{}{}:
	default:
	}
	<-d.release
	return map[string]string{"body": "held"}
}
func (d *heldDocument) MarshalJSON() ([]byte, error) {
	type alias heldDocument
	return json.Marshal((*alias)(d))
}
func (d *heldDocument) UnmarshalJSON(data []byte) error {
	type alias heldDocument
	return json.Unmarshal(data, (*alias)(d))
}

// TestStopJoinsHandlerAfterConnectionLoss: a Stop never returns nil while an input handler still
// runs, even after the broker went away for good while the handler was mid-write (#439). The
// client does not reconnect, so the broker's restart closes its connection; on the JetStream
// input the consume loop's native Closed() then reports the end while the handler is still held
// (natsclient/stream.go:421-427, #83), which the test checks before it stops. A Stop whose context
// ends first returns that error and keeps the inputs for a later Stop; once the handler returns,
// a later Stop returns nil and holds nothing (lifecycle-suite, retained handles). The core-NATS
// input proves the same through Subscription.Drain.
func TestStopJoinsHandlerAfterConnectionLoss(t *testing.T) {
	for _, lane := range []struct {
		name, raw, subject string
		jetStream          bool
	}{
		{"jetstream", jetStreamConfig, inputSubject, true},
		{"core", coreConfig, "objectstore.test.core", false},
	} {
		t.Run(lane.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()

			// Own server: the test takes the broker away under a running Component.
			f := natsfixture.New(t)
			if err := f.Start(ctx); err != nil {
				t.Fatalf("fixture Start: %v", err)
			}
			if lane.jetStream {
				createInputStream(ctx, t, f.JetStream(), jetstream.FileStorage)
			}
			// The client does not reconnect (openSharedClient), so losing the broker closes its
			// connection for good.
			client := natsfixture.Open(t, f.URL(), openSharedClient)

			entered := make(chan struct{}, 1)
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseHandler()

			reg, err := testRegistry()
			if err == nil {
				err = reg.Register(&payloadregistry.Registration{
					Factory: func() any { return &heldDocument{entered: entered, release: release} },
					Domain:  "objectstoretest", Category: "held", Version: "v1",
					Description: "ContentStorable test document that holds its handler",
				})
			}
			if err != nil {
				t.Fatalf("registry: %v", err)
			}
			built, err := newComponent(json.RawMessage(lane.raw), component.Dependencies{
				NATSClient: client, PayloadRegistry: reg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			c := built.(*Component)
			if err := c.Initialize(); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			if err := c.Start(ctx); err != nil {
				t.Fatalf("Start: %v", err)
			}
			defer func() {
				releaseHandler()
				stopCtx, stopCancel := context.WithTimeout(t.Context(), 30*time.Second)
				defer stopCancel()
				if err := c.Stop(stopCtx); err != nil {
					t.Errorf("cleanup Stop: %v", err)
				}
			}()

			data := wire(t, &heldDocument{ID: "acme.ops.docs.library.document.readme"})
			publish := client.Publish
			if lane.jetStream {
				publish = client.PublishToStream
			}
			if err := publish(ctx, lane.subject, data); err != nil {
				t.Fatalf("publish: %v", err)
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("the input handler never reached RawContent")
			}

			closedConn := client.GetConnection().StatusChanged(nats.CLOSED)
			if err := f.Restart(ctx); err != nil {
				t.Fatalf("fixture Restart: %v", err)
			}
			select {
			case <-closedConn:
			case <-ctx.Done():
				t.Fatal("the client's connection never closed after the broker went away")
			}
			if lane.jetStream {
				// The trigger: the consume loop reports its end while the handler is still held.
				select {
				case <-c.consumers[0].handle.Closed():
				case <-ctx.Done():
					t.Fatal("the consume loop's Closed() never fired; the scenario needs it to")
				}
			}

			// The handler is held until releaseHandler, so this Stop can only end by its context.
			stopCtx, stopCancel := context.WithTimeout(ctx, time.Second)
			err = c.Stop(stopCtx)
			stopCancel()
			if err == nil {
				t.Fatal("Stop returned nil while an input handler is still running")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Stop error = %v, want the expired context's", err)
			}
			if held := (suiteOwner{c}).Observe().Unresolved; len(held) == 0 {
				t.Fatal("a Stop whose context ended released the inputs; cleanup must stay pending for a later Stop")
			}

			releaseHandler()
			retryCtx, retryCancel := context.WithTimeout(ctx, 30*time.Second)
			defer retryCancel()
			if err := c.Stop(retryCtx); err != nil {
				t.Fatalf("Stop after the handler returned: %v", err)
			}
			if held := (suiteOwner{c}).Observe().Unresolved; len(held) != 0 {
				t.Fatalf("after a Stop returned nil the Component still holds %v", held)
			}
		})
	}
}

// TestJetStreamInputAckWaitReachesConsumer: a JetStream input's declared ack_wait is the durable
// consumer's AckWait on the server, and an input that declares none gets natsclient's 30 seconds
// (natsclient/stream.go:897-900) (#438).
func TestJetStreamInputAckWaitReachesConsumer(t *testing.T) {
	for _, tc := range []struct {
		name, declared string
		want           time.Duration
	}{
		{"declared", `"ack_wait": "2m", `, 2 * time.Minute},
		{"unset", "", 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
			js := clientJetStream(t, client)
			createInputStream(ctx, t, js, jetstream.FileStorage)
			raw := strings.Replace(jetStreamConfig, `"subjects":`, tc.declared+`"subjects":`, 1)
			_, stop := startComponent(ctx, t, client, raw)
			defer stop()

			consumer, err := js.Consumer(ctx, inputStream, inputConsumer)
			if err != nil {
				t.Fatalf("consumer %s: %v", inputConsumer, err)
			}
			info, err := consumer.Info(ctx)
			if err != nil {
				t.Fatalf("consumer Info: %v", err)
			}
			if info.Config.AckWait != tc.want {
				t.Fatalf("server AckWait = %s, want %s", info.Config.AckWait, tc.want)
			}
		})
	}
}
