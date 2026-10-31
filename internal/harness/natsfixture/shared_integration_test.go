//go:build integration

package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
)

// nats-fixture › "Shared server per package", scenario "A process that never resets starts
// nothing": a Shared on which Reset is never called starts no container, and Stop returns nil
// without a Docker call. A nil context is still refused by name.
func TestSharedNeverResetStartsNothing(t *testing.T) {
	var s Shared
	var nilCtx context.Context
	if err := s.Stop(nilCtx); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("Stop(nil) = %v, want a refusal naming the nil context", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.Stop(cancelled); err != nil {
		t.Fatalf("Stop on an unstarted Shared with a cancelled context = %v, want nil (no Docker call)", err)
	}
	if err := s.Stop(t.Context()); err != nil {
		t.Fatalf("Stop on an unstarted Shared = %v, want nil", err)
	}
	if got := processContainers(t); len(got) != 0 {
		t.Fatalf("containers of this process = %v, want none", got)
	}
}

// nats-fixture › "Shared server per package", scenario "A failed start fails later resets at
// once": when the server's start fails at the first reset, that test fails with the start error,
// the container is rolled back, and the next reset fails at once with the same error without a
// Docker call.
func TestSharedFailedStartFailsLaterResetsAtOnce(t *testing.T) {
	var s Shared
	reads := 0
	s.f.readPort = func(context.Context, testcontainers.Container) (string, error) {
		reads++
		return "", errNotFound
	}
	rec1 := &recordingTB{TB: t, ctx: t.Context()}
	if url := s.Reset(rec1); url != "" {
		t.Fatalf("first Reset returned URL %q on a failed start, want empty", url)
	}
	if reads != 1 {
		t.Fatalf("port reads on first Reset = %d, want 1", reads)
	}
	if len(rec1.fatals) != 1 || !strings.Contains(rec1.fatals[0], "phase mapped-port") || !strings.Contains(rec1.fatals[0], errNotFound.Error()) {
		t.Fatalf("first Reset fatals = %q, want one naming phase mapped-port and %q", rec1.fatals, errNotFound)
	}
	if got := processContainers(t); len(got) != 0 {
		t.Fatalf("containers of this process after failed first Reset = %v, want none", got)
	}

	rec2 := &recordingTB{TB: t, ctx: t.Context()}
	if url := s.Reset(rec2); url != "" {
		t.Fatalf("second Reset returned URL %q after a failed start, want empty", url)
	}
	if reads != 1 {
		t.Fatalf("port reads after second Reset = %d, want 1 (no second Docker start)", reads)
	}
	if !slices.Equal(rec2.fatals, rec1.fatals) {
		t.Fatalf("second Reset fatals = %q, want the same start error %q", rec2.fatals, rec1.fatals)
	}
	if !stopHoldsNothing(t, &s.f) {
		t.Fatalf("after a failed first Reset, Shared still holds %v", s.f.remaining())
	}
}

// nats-fixture › "Shared server per package" ("Container gone after the package") and "Reset to a
// verified-empty server" (all four scenarios): one Shared server runs across the four reset
// scenarios in turn, and Stop then removes its container.
func TestSharedServerAndReset(t *testing.T) {
	var s Shared
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), stopBound)
		defer cancel()
		_ = s.Stop(ctx)
	})
	if s.f.containerID != "" || len(processContainers(t)) != 0 {
		t.Fatalf("before first Reset: containerID = %q, containers = %v; want none", s.f.containerID, processContainers(t))
	}

	var sharedID, sharedURL string

	// Scenario: State does not cross tests
	t.Run("state does not cross tests", func(t *testing.T) {
		ctx := t.Context()
		recA := &recordingTB{TB: t, ctx: ctx}
		urlA := s.Reset(recA)
		if len(recA.fatals) != 0 || !strings.HasPrefix(urlA, "nats://") {
			t.Fatalf("test A Reset = %q, fatals %q; want a nats:// URL and no failure", urlA, recA.fatals)
		}
		sharedID, sharedURL = s.f.containerID, urlA
		if sharedID == "" || !containerExists(t, sharedID) {
			t.Fatalf("shared container %q not running during test A", sharedID)
		}

		// Dial through Open so Open's LIFO cleanup closes the connection before Reset's
		// post-test connection check runs (nats-fixture › "Reset to a verified-empty server").
		ncA := Open(recA, urlA, func(_ context.Context, u string) (*nats.Conn, func(context.Context) error, error) {
			nc, err := nats.Connect(u, nats.MaxReconnects(0))
			if err != nil {
				return nil, nil, err
			}
			return nc, func(context.Context) error {
				nc.Close()
				return nil
			}, nil
		})

		jsA, err := jetstream.New(ncA)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CreateStream(ctx, jsA, jetstream.FileStorage, "ORDERS", "orders.>"); err != nil {
			t.Fatal(err)
		}
		if _, err := jsA.Publish(ctx, "orders.1", []byte("one")); err != nil {
			t.Fatal(err)
		}
		if _, err := jsA.CreateOrUpdateConsumer(ctx, "ORDERS", jetstream.ConsumerConfig{Durable: "worker"}); err != nil {
			t.Fatal(err)
		}
		kv, err := jsA.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "STATE"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := kv.Put(ctx, "k", []byte("v")); err != nil {
			t.Fatal(err)
		}
		obs, err := jsA.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: "BLOBS", Storage: jetstream.MemoryStorage})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := obs.PutBytes(ctx, "blob", make([]byte, 512)); err != nil {
			t.Fatal(err)
		}
		before, err := jsA.AccountInfo(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if before.Streams != 3 || before.Consumers < 1 || before.Memory == 0 || before.Store == 0 {
			t.Fatalf("before reset: streams=%d consumers=%d memory=%d store=%d; want 3, >=1, >0, >0",
				before.Streams, before.Consumers, before.Memory, before.Store)
		}
		recA.runCleanups()
		if len(recA.errors) != 0 {
			t.Fatalf("test A cleanups reported errors: %q", recA.errors)
		}

		recB := &recordingTB{TB: t, ctx: ctx}
		urlB := s.Reset(recB)
		if len(recB.fatals) != 0 || urlB != sharedURL || s.f.containerID != sharedID {
			t.Fatalf("test B Reset = %q (container %s), fatals %q; want %q (%s) and no failure",
				urlB, s.f.containerID, recB.fatals, sharedURL, sharedID)
		}
		ncB, err := nats.Connect(urlB, nats.MaxReconnects(0))
		if err != nil {
			t.Fatalf("dial %s: %v", urlB, err)
		}
		recB.Cleanup(func() { ncB.Close() })
		jsB, err := jetstream.New(ncB)
		if err != nil {
			t.Fatal(err)
		}
		after, err := jsB.AccountInfo(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if after.Streams != 0 || after.Consumers != 0 || after.Memory != 0 || after.Store != 0 {
			t.Fatalf("after B's reset: streams=%d consumers=%d memory=%d store=%d; want all zero",
				after.Streams, after.Consumers, after.Memory, after.Store)
		}
		recB.runCleanups()
		if len(recB.errors) != 0 {
			t.Fatalf("test B cleanups reported errors: %q", recB.errors)
		}
	})

	// Scenario: Fixed names in turn
	t.Run("fixed names in turn", func(t *testing.T) {
		ctx := t.Context()
		rec1 := &recordingTB{TB: t, ctx: ctx}
		url1 := s.Reset(rec1)
		if len(rec1.fatals) != 0 {
			t.Fatalf("first Reset fatals: %q", rec1.fatals)
		}
		nc1, err := nats.Connect(url1, nats.MaxReconnects(0))
		if err != nil {
			t.Fatal(err)
		}
		rec1.Cleanup(func() { nc1.Close() })
		js1, err := jetstream.New(nc1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CreateStream(ctx, js1, jetstream.FileStorage, "ENTITY", "entity.>"); err != nil {
			t.Fatal(err)
		}
		ack, err := js1.Publish(ctx, "entity.1", []byte("from-first"))
		if err != nil {
			t.Fatal(err)
		}
		rec1.runCleanups()
		if len(rec1.errors) != 0 {
			t.Fatalf("first test cleanups: %q", rec1.errors)
		}

		rec2 := &recordingTB{TB: t, ctx: ctx}
		url2 := s.Reset(rec2)
		if len(rec2.fatals) != 0 {
			t.Fatalf("second Reset fatals: %q", rec2.fatals)
		}
		nc2, err := nats.Connect(url2, nats.MaxReconnects(0))
		if err != nil {
			t.Fatal(err)
		}
		rec2.Cleanup(func() { nc2.Close() })
		js2, err := jetstream.New(nc2)
		if err != nil {
			t.Fatal(err)
		}
		stream, err := CreateStream(ctx, js2, jetstream.FileStorage, "ENTITY", "entity.>")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stream.GetMsg(ctx, ack.Sequence); !errors.Is(err, jetstream.ErrMsgNotFound) {
			t.Fatalf("GetMsg(%d) on recreated ENTITY = %v, want ErrMsgNotFound", ack.Sequence, err)
		}
		rec2.runCleanups()
		if len(rec2.errors) != 0 {
			t.Fatalf("second test cleanups: %q", rec2.errors)
		}
	})

	// Scenario: A leaked connection fails its own test
	t.Run("leaked connection fails its own test", func(t *testing.T) {
		rec := &recordingTB{TB: t, ctx: t.Context()}
		url := s.Reset(rec)
		if len(rec.fatals) != 0 {
			t.Fatalf("Reset fatals: %q", rec.fatals)
		}
		leaked, err := nats.Connect(url, nats.MaxReconnects(0), nats.Name("leaked-by-test"))
		if err != nil {
			t.Fatal(err)
		}
		defer leaked.Close()
		flushCtx, cancel := context.WithTimeout(t.Context(), stopBound)
		defer cancel()
		if err := leaked.FlushWithContext(flushCtx); err != nil {
			t.Fatal(err)
		}
		leakedID, err := leaked.GetClientID()
		if err != nil {
			t.Fatal(err)
		}

		rec.runCleanups()
		// The error names the connection left open, so a failure says whose it was (#493).
		if len(rec.errors) != 1 || !strings.Contains(rec.errors[0], "1 client connection") ||
			!strings.Contains(rec.errors[0], fmt.Sprintf("cid=%d name=\"leaked-by-test\"", leakedID)) {
			t.Fatalf("cleanups errors = %q, want one naming \"1 client connection\" and the leaked connection's cid %d and name",
				rec.errors, leakedID)
		}
		leaked.Close()
	})

	// Scenario: A reset while a connection is open fails
	t.Run("reset while a connection is open fails", func(t *testing.T) {
		leaked, err := nats.Connect(sharedURL, nats.MaxReconnects(0))
		if err != nil {
			t.Fatal(err)
		}
		defer leaked.Close()
		flushCtx, cancel := context.WithTimeout(t.Context(), stopBound)
		defer cancel()
		if err := leaked.FlushWithContext(flushCtx); err != nil {
			t.Fatal(err)
		}

		rec := &recordingTB{TB: t, ctx: t.Context()}
		if url := s.Reset(rec); url != "" {
			t.Fatalf("Reset while a connection is open returned %q, want empty", url)
		}
		if len(rec.fatals) != 1 || !strings.Contains(rec.fatals[0], "a connection from an earlier test is still open") || !strings.Contains(rec.fatals[0], "1 other client connection") {
			t.Fatalf("Reset fatals = %q, want one saying a connection from an earlier test is still open (1 other client connection)", rec.fatals)
		}
		if len(rec.cleanups) != 0 {
			t.Fatalf("%d cleanup(s) registered after a failed Reset, want none", len(rec.cleanups))
		}
		leaked.Close()
	})

	// Scenario: Container gone after the package
	if sharedID == "" || !containerExists(t, sharedID) {
		t.Fatalf("shared container %q missing before Stop", sharedID)
	}
	stopCtx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatalf("Shared.Stop: %v", err)
	}
	if containerExists(t, sharedID) {
		t.Fatalf("shared container %s still exists after Stop", sharedID)
	}
	if !stopHoldsNothing(t, &s.f) {
		t.Fatalf("after Stop, Shared still holds %v", s.f.remaining())
	}
	recStopped := &recordingTB{TB: t, ctx: t.Context()}
	if url := s.Reset(recStopped); url != "" || len(recStopped.fatals) != 1 || !strings.Contains(recStopped.fatals[0], "stopped shared server") {
		t.Fatalf("Reset after Stop = %q, fatals %q; want empty URL and a fatal naming stopped shared server", url, recStopped.fatals)
	}
}
